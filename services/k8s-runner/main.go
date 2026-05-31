package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/yaml.v3"
)

const (
	defaultAgentImage              = "nopsai-agent:latest"
	defaultRunnerNamespace         = "default"
	defaultWorkspaceSize           = "5Gi"
	defaultWorkspaceAccessMode     = corev1.ReadWriteOnce
	defaultTaskTimeout             = 30 * time.Minute
	defaultRunTimeout              = 2 * time.Hour
	defaultImagePullPolicy         = corev1.PullIfNotPresent
	serviceAccountNamespaceFile    = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	kubernetesWorkspaceMountPath   = "/workspace"
	kubernetesAgentContainerName   = "agent"
	kubernetesRuntimeName          = "kubernetes"
	defaultRuntimePoolName         = "default"
	defaultCleanupFinishedPods     = true
	defaultKubernetesAffinity      = true
	kubernetesWorkspaceVolumePVC   = "pvc"
	kubernetesWorkspaceExistingPVC = "existing"
	kubernetesWorkspaceEmptyDir    = "emptyDir"
)

var nameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

type kubernetesRunner struct {
	id               string
	scopes           []string
	capacity         int32
	dispatcherAddr   string
	dispatcherCreds  *serviceauth.Credentials
	transportCreds   credentials.TransportCredentials
	client           kubernetes.Interface
	namespace        string
	serviceAccount   string
	active           atomic.Int32
	stopMu           sync.Mutex
	stoppedRuns      map[string]struct{}
	affinityEnabled  bool
	imagePullPolicy  corev1.PullPolicy
	workspaceSize    string
	workspaceAccess  corev1.PersistentVolumeAccessMode
	workspaceMode    string
	existingPVC      string
	storageClass     string
	taskTimeout      time.Duration
	runTimeout       time.Duration
	cleanupPods      bool
	podLabels        map[string]string
	podAnnotations   map[string]string
	runtimePoolsYAML string
	runtimePools     map[string]config.RuntimePool
	limits           config.RunnerLimits
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)

	dispatcherAddr := strings.TrimSpace(cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "dispatcher:9090"
	}
	runnerID := strings.TrimSpace(cfg.RunnerID)
	if runnerID == "" {
		if host, err := os.Hostname(); err == nil {
			runnerID = host
		} else {
			runnerID = "k8s-runner"
		}
	}
	capacity := int32(cfg.RunnerCapacity)
	if capacity <= 0 {
		capacity = 1
	}

	dispatcherCreds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleRunner,
		ServiceID:  cfg.EffectiveRunnerServiceID(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher client authentication")
	}
	transportCreds, err := servicetls.ClientCredentials(servicetls.Config{
		Mode:       cfg.EffectiveDispatcherTLSMode(),
		Secret:     cfg.EffectiveDispatcherTLSSecret(),
		Role:       serviceauth.RoleRunner,
		ServiceID:  cfg.EffectiveRunnerServiceID(),
		ServerName: cfg.EffectiveDispatcherTLSServerName(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher transport security")
	}

	restConfig, err := kubernetesRESTConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load kubernetes config")
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create kubernetes client")
	}

	kcfg := config.NormalizeKubernetesConfig(cfg.Kubernetes)
	namespace := firstNonEmpty(kcfg.Namespace, readServiceAccountNamespace(), defaultRunnerNamespace)
	workspaceSize := firstNonEmpty(kcfg.DefaultWorkspaceSize, defaultWorkspaceSize)
	workspaceMode := firstNonEmpty(kcfg.WorkspaceVolumeMode, kubernetesWorkspaceVolumePVC)
	runtimePoolsYAML := encodeRuntimePoolsYAML(cfg.RuntimePools)

	r := &kubernetesRunner{
		id:               runnerID,
		scopes:           parseScopes(cfg.RunnerScopes),
		capacity:         capacity,
		dispatcherAddr:   dispatcherAddr,
		dispatcherCreds:  dispatcherCreds,
		transportCreds:   transportCreds,
		client:           clientset,
		namespace:        namespace,
		serviceAccount:   strings.TrimSpace(kcfg.ServiceAccount),
		stoppedRuns:      make(map[string]struct{}),
		affinityEnabled:  boolPtrValue(kcfg.AffinityEnabled, defaultKubernetesAffinity),
		imagePullPolicy:  imagePullPolicy(kcfg.DefaultImagePullPolicy),
		workspaceSize:    workspaceSize,
		workspaceAccess:  accessMode(kcfg.DefaultWorkspaceAccessMode),
		workspaceMode:    workspaceMode,
		existingPVC:      strings.TrimSpace(kcfg.ExistingWorkspacePVC),
		storageClass:     strings.TrimSpace(kcfg.StorageClass),
		taskTimeout:      parseDurationDefault(kcfg.DefaultTaskTimeout, defaultTaskTimeout),
		runTimeout:       parseDurationDefault(kcfg.DefaultRunTimeout, defaultRunTimeout),
		cleanupPods:      boolPtrValue(kcfg.CleanupFinishedPods, defaultCleanupFinishedPods),
		podLabels:        cloneMap(kcfg.PodLabels),
		podAnnotations:   cloneMap(kcfg.PodAnnotations),
		runtimePoolsYAML: runtimePoolsYAML,
		runtimePools:     config.NormalizeRuntimePools(cfg.RuntimePools),
		limits:           cfg.Limits,
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("runtime", kubernetesRuntimeName).
		Str("namespace", namespace).
		Str("dispatcher_addr", dispatcherAddr).
		Strs("scopes", r.scopes).
		Int("capacity", int(capacity)).
		Bool("affinity_enabled", r.affinityEnabled).
		Msg("kubernetes runner starting")

	for {
		if err := r.connectAndServe(); err != nil {
			log.Error().Err(err).Msg("dispatcher stream ended, retrying")
			time.Sleep(3 * time.Second)
		}
	}
}

func kubernetesRESTConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if kubeconfig == "" {
		if home, err := os.UserHomeDir(); err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func (r *kubernetesRunner) connectAndServe() error {
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(r.transportCreds),
		grpc.WithBlock(),
	}
	if r.dispatcherCreds != nil {
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(r.dispatcherCreds))
	}
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 20*time.Second)
	conn, err := grpc.DialContext(dialCtx, r.dispatcherAddr, dialOptions...)
	dialCancel()
	if err != nil {
		return fmt.Errorf("failed to dial dispatcher: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dispatcherClient := proto.NewDispatcherServiceClient(conn)
	stream, err := dispatcherClient.Register(ctx)
	if err != nil {
		return fmt.Errorf("failed to open register stream: %w", err)
	}

	sendCh := make(chan *proto.RunnerMessage, 64)
	sendErrCh := make(chan error, 1)
	go func() {
		for {
			select {
			case msg, ok := <-sendCh:
				if !ok {
					return
				}
				if err := stream.Send(msg); err != nil {
					sendErrCh <- err
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	sendCh <- &proto.RunnerMessage{
		Message: &proto.RunnerMessage_Register{
			Register: &proto.RunnerRegistration{
				RunnerId: r.id,
				Scopes:   r.scopes,
				Capacity: r.capacity,
				Metadata: r.registrationMetadata(),
			},
		},
	}

	hbStop := make(chan struct{})
	defer close(hbStop)
	go r.heartbeatLoop(sendCh, hbStop)

	for {
		select {
		case err := <-sendErrCh:
			return err
		default:
		}

		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		switch body := msg.Message.(type) {
		case *proto.DispatcherMessage_Job:
			if body.Job != nil {
				go r.handleJob(context.Background(), dispatcherClient, body.Job, sendCh)
			}
		case *proto.DispatcherMessage_Note:
			log.Info().Str("note", body.Note).Msg("dispatcher message")
		default:
			log.Warn().Msg("received unknown message from dispatcher")
		}
	}
}

func (r *kubernetesRunner) registrationMetadata() map[string]string {
	metadata := map[string]string{
		"version":                    "v1",
		"runtime":                    kubernetesRuntimeName,
		"kubernetes_namespace":       r.namespace,
		"kubernetes_service_account": r.serviceAccount,
		"kubernetes_affinity":        strconv.FormatBool(r.affinityEnabled),
		"kubernetes_workspace_mode":  r.workspaceMode,
		"kubernetes_workspace_size":  r.workspaceSize,
		"kubernetes_access_mode":     string(r.workspaceAccess),
		"kubernetes_storage_class":   r.storageClass,
		"dispatcher_addr":            r.dispatcherAddr,
	}
	if host, err := os.Hostname(); err == nil {
		metadata["hostname"] = strings.TrimSpace(host)
	}
	if r.limits.MaxConcurrentRuns > 0 {
		metadata["max_concurrent_runs"] = strconv.Itoa(r.limits.MaxConcurrentRuns)
	}
	if r.limits.MaxConcurrentTasks > 0 {
		metadata["max_concurrent_tasks"] = strconv.Itoa(r.limits.MaxConcurrentTasks)
	}
	if r.limits.MaxConcurrentTasksPerRun > 0 {
		metadata["max_concurrent_tasks_per_run"] = strconv.Itoa(r.limits.MaxConcurrentTasksPerRun)
	}
	if r.limits.MaxPendingTasks > 0 {
		metadata["max_pending_tasks"] = strconv.Itoa(r.limits.MaxPendingTasks)
	}
	return metadata
}

func (r *kubernetesRunner) heartbeatLoop(sendCh chan<- *proto.RunnerMessage, stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			sendCh <- &proto.RunnerMessage{
				Message: &proto.RunnerMessage_Heartbeat{
					Heartbeat: &proto.RunnerHeartbeat{
						RunnerId:   r.id,
						ActiveJobs: r.active.Load(),
					},
				},
			}
		}
	}
}

func (r *kubernetesRunner) handleJob(ctx context.Context, dispatcher proto.DispatcherServiceClient, job *proto.JobRequest, sendCh chan<- *proto.RunnerMessage) {
	if job == nil {
		return
	}
	r.active.Add(1)
	sendJobResult(sendCh, job.RunId, "accepted", "")

	go func() {
		defer r.active.Add(-1)
		defer r.clearRunStopRequested(job.RunId)

		agentImage := strings.TrimSpace(job.AgentImage)
		if agentImage == "" {
			agentImage = defaultAgentImage
		}

		podName := kubernetesObjectName(firstNonEmpty(job.ContainerName, "agent-"+job.RunId))
		workspacePVC, err := r.workspaceClaimName(podName, job)
		if err != nil {
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}

		runtimeVars := r.agentRuntimeVars(job, workspacePVC)
		pod, err := r.createAgentPod(context.Background(), podName, agentImage, workspacePVC, runtimeVars, job)
		if err != nil {
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}
		if r.cleanupPods {
			defer r.deletePod(context.Background(), podName)
		}

		log.Info().Str("run_id", job.RunId).Str("pod", pod.Name).Str("workspace_pvc", workspacePVC).Msg("started agent pod")

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go r.monitorRunCancellation(runCtx, dispatcher, job.RunId, podName)
		go r.streamPodLogs(runCtx, dispatcher, job.RunId, podName)

		phase, err := r.waitForPodCompletion(context.Background(), podName)
		if err != nil {
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}
		if phase != corev1.PodSucceeded {
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("agent pod completed with phase %s", phase))
			return
		}
		sendJobResult(sendCh, job.RunId, "completed", "")
	}()
}

func (r *kubernetesRunner) agentRuntimeVars(job *proto.JobRequest, workspacePVC string) []string {
	runtimeVars := append([]string(nil), job.Env...)
	runtimeVars = upsertRuntimeVar(runtimeVars, "DISPATCHER_ADDRESS", strings.TrimSpace(r.dispatcherAddr))
	runtimeVars = upsertRuntimeVar(runtimeVars, "RUNNER_ID", r.id)
	runtimeVars = upsertRuntimeVar(runtimeVars, "NOPSAI_RUNTIME", kubernetesRuntimeName)
	runtimeVars = upsertRuntimeVar(runtimeVars, "SHARED_VOLUME_NAME", workspacePVC)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_NAMESPACE", r.namespace)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_SERVICE_ACCOUNT", r.serviceAccount)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_DEFAULT_IMAGE_PULL_POLICY", string(r.imagePullPolicy))
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_DEFAULT_WORKSPACE_SIZE", r.workspaceSize)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_DEFAULT_WORKSPACE_ACCESS_MODE", string(r.workspaceAccess))
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_DEFAULT_TASK_TIMEOUT", r.taskTimeout.String())
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_WORKSPACE_VOLUME_MODE", r.workspaceMode)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_AFFINITY_ENABLED", strconv.FormatBool(r.affinityEnabled))
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_CLEANUP_FINISHED_PODS", strconv.FormatBool(r.cleanupPods))
	if r.storageClass != "" {
		runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_STORAGE_CLASS", r.storageClass)
	}
	if r.runtimePoolsYAML != "" {
		runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_RUNTIME_POOLS", r.runtimePoolsYAML)
	}
	return runtimeVars
}

func (r *kubernetesRunner) workspaceClaimName(podName string, job *proto.JobRequest) (string, error) {
	if r.workspaceMode == kubernetesWorkspaceEmptyDir {
		return "", fmt.Errorf("emptyDir workspace mode is not compatible with multi-pod pipeline execution")
	}
	if r.workspaceMode == kubernetesWorkspaceExistingPVC || r.existingPVC != "" {
		claimName := strings.TrimSpace(r.existingPVC)
		if claimName == "" {
			claimName = strings.TrimSpace(job.SharedVolumeName)
		}
		claimName = kubernetesObjectName(claimName)
		if claimName == "" {
			return "", fmt.Errorf("existing workspace pvc is required")
		}
		return claimName, nil
	}

	podName = strings.TrimSpace(podName)
	if podName == "" {
		return "", fmt.Errorf("workspace pvc name is required")
	}
	return podName + "-workspace", nil
}

func (r *kubernetesRunner) workspaceVolumeSource(workspacePVC string, job *proto.JobRequest) (corev1.VolumeSource, error) {
	if r.workspaceMode == kubernetesWorkspaceExistingPVC || r.existingPVC != "" {
		return corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: workspacePVC},
		}, nil
	}
	if r.workspaceMode == kubernetesWorkspaceEmptyDir {
		return corev1.VolumeSource{}, fmt.Errorf("emptyDir workspace mode is not compatible with multi-pod pipeline execution")
	}

	size, err := resource.ParseQuantity(r.workspaceSize)
	if err != nil {
		return corev1.VolumeSource{}, fmt.Errorf("parse workspace size: %w", err)
	}
	claimTemplate := &corev1.PersistentVolumeClaimTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Labels: mergeMaps(r.podLabels, map[string]string{
				"app.kubernetes.io/name":      "nopsai",
				"app.kubernetes.io/component": "pipeline-workspace",
				"nopsai.io/runner-id":         kubernetesLabelValue(r.id),
				"nopsai.io/run-id":            kubernetesLabelValue(job.RunId),
			}),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{r.workspaceAccess},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		},
	}
	if r.storageClass != "" {
		claimTemplate.Spec.StorageClassName = &r.storageClass
	}
	return corev1.VolumeSource{
		Ephemeral: &corev1.EphemeralVolumeSource{VolumeClaimTemplate: claimTemplate},
	}, nil
}

func (r *kubernetesRunner) createAgentPod(ctx context.Context, podName, image, workspacePVC string, env []string, job *proto.JobRequest) (*corev1.Pod, error) {
	nodeSelector, resources, err := r.defaultPoolScheduling()
	if err != nil {
		return nil, err
	}
	workspaceVolumeSource, err := r.workspaceVolumeSource(workspacePVC, job)
	if err != nil {
		return nil, err
	}
	labels := mergeMaps(r.podLabels, map[string]string{
		"app.kubernetes.io/name":      "nopsai",
		"app.kubernetes.io/component": "pipeline-agent",
		"nopsai.io/runner-id":         kubernetesLabelValue(r.id),
		"nopsai.io/run-id":            kubernetesLabelValue(job.RunId),
		"nopsai.io/pipeline":          kubernetesLabelValue(job.PipelineName),
	})
	agentEnv := upsertEnvVarSource(envVars(env), corev1.EnvVar{
		Name: "KUBERNETES_NODE_NAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"},
		},
	})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   r.namespace,
			Labels:      labels,
			Annotations: cloneMap(r.podAnnotations),
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: r.serviceAccount,
			NodeSelector:       nodeSelector,
			Volumes: []corev1.Volume{{
				Name:         "workspace",
				VolumeSource: workspaceVolumeSource,
			}},
			Containers: []corev1.Container{{
				Name:            kubernetesAgentContainerName,
				Image:           image,
				ImagePullPolicy: r.imagePullPolicy,
				Env:             agentEnv,
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "workspace",
					MountPath: kubernetesWorkspaceMountPath,
				}},
				Resources: resources,
			}},
		},
	}
	created, err := r.client.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create agent pod: %w", err)
	}
	return created, nil
}

func (r *kubernetesRunner) defaultPoolScheduling() (map[string]string, corev1.ResourceRequirements, error) {
	resources := corev1.ResourceRequirements{}
	if len(r.runtimePools) == 0 {
		return nil, resources, nil
	}
	pool, ok := r.runtimePools[defaultRuntimePoolName]
	if !ok {
		return nil, resources, nil
	}
	requests, err := resourceList(pool.Resources.Requests)
	if err != nil {
		return nil, resources, err
	}
	limits, err := resourceList(pool.Resources.Limits)
	if err != nil {
		return nil, resources, err
	}
	resources.Requests = requests
	resources.Limits = limits
	return cloneMap(pool.NodeSelector), resources, nil
}

func resourceList(values map[string]string) (corev1.ResourceList, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := corev1.ResourceList{}
	for name, value := range values {
		qty, err := resource.ParseQuantity(value)
		if err != nil {
			return nil, fmt.Errorf("parse resource %s=%s: %w", name, value, err)
		}
		out[corev1.ResourceName(name)] = qty
	}
	return out, nil
}

func (r *kubernetesRunner) waitForPodCompletion(ctx context.Context, podName string) (corev1.PodPhase, error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.runTimeout)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return "", fmt.Errorf("timed out waiting for agent pod %s", podName)
		case <-ticker.C:
			pod, err := r.client.CoreV1().Pods(r.namespace).Get(waitCtx, podName, metav1.GetOptions{})
			if err != nil {
				return "", err
			}
			switch pod.Status.Phase {
			case corev1.PodSucceeded, corev1.PodFailed:
				return pod.Status.Phase, nil
			}
		}
	}
}

func (r *kubernetesRunner) streamPodLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, podName string) {
	if dispatcher == nil {
		return
	}
	var reader io.ReadCloser
	var err error
	for attempt := 0; attempt < 30; attempt++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		reader, err = r.client.CoreV1().Pods(r.namespace).GetLogs(podName, &corev1.PodLogOptions{
			Container:  kubernetesAgentContainerName,
			Follow:     true,
			Timestamps: true,
		}).Stream(ctx)
		if err == nil {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Str("pod", podName).Msg("failed to attach to pod logs")
		return
	}
	defer reader.Close()

	logChan := make(chan string, 100)
	go func() {
		defer close(logChan)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			logChan <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("pod log scanner error")
		}
	}()

	const batchSize = 50
	const batchTimeout = 500 * time.Millisecond
	var batchLines []string
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()
	for {
		select {
		case line, ok := <-logChan:
			if !ok {
				r.flushLogs(ctx, dispatcher, runID, batchLines)
				return
			}
			batchLines = append(batchLines, line)
			if len(batchLines) >= batchSize {
				r.flushLogs(ctx, dispatcher, runID, batchLines)
				batchLines = nil
			}
		case <-ticker.C:
			if len(batchLines) > 0 {
				r.flushLogs(ctx, dispatcher, runID, batchLines)
				batchLines = nil
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *kubernetesRunner) flushLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID string, lines []string) {
	if len(lines) == 0 {
		return
	}
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := dispatcher.IngestLogs(sendCtx, &proto.LogBatch{RunId: runID, Lines: lines}); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("failed to send log batch to dispatcher")
	}
}

func (r *kubernetesRunner) monitorRunCancellation(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, podName string) {
	if dispatcher == nil || strings.TrimSpace(runID) == "" || strings.TrimSpace(podName) == "" {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			resp, err := dispatcher.GetRunStatus(reqCtx, &proto.RunStatusRequest{RunId: runID})
			cancel()
			if err != nil {
				log.Warn().Err(err).Str("run_id", runID).Msg("failed to poll run status for cancellation")
				continue
			}
			if strings.EqualFold(strings.TrimSpace(resp.GetStatus()), "cancelled") {
				if !r.markRunStopRequested(runID) {
					return
				}
				log.Warn().Str("run_id", runID).Str("pod", podName).Msg("run cancelled; deleting agent pod")
				r.deletePod(context.Background(), podName)
				return
			}
		}
	}
}

func (r *kubernetesRunner) deletePod(ctx context.Context, podName string) {
	grace := int64(1)
	err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, podName, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Warn().Err(err).Str("pod", podName).Msg("failed to delete pod")
	}
}

func (r *kubernetesRunner) markRunStopRequested(runID string) bool {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()
	if _, exists := r.stoppedRuns[runID]; exists {
		return false
	}
	r.stoppedRuns[runID] = struct{}{}
	return true
}

func (r *kubernetesRunner) clearRunStopRequested(runID string) {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()
	delete(r.stoppedRuns, runID)
}

func sendJobResult(sendCh chan<- *proto.RunnerMessage, runID, status, errMsg string) {
	msg := &proto.RunnerMessage{
		Message: &proto.RunnerMessage_JobResult{
			JobResult: &proto.JobResult{
				RunId:  runID,
				Status: status,
				Error:  errMsg,
			},
		},
	}
	select {
	case sendCh <- msg:
	default:
		log.Warn().Str("run_id", runID).Msg("send buffer full while reporting job status")
	}
}

func parseScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var scopes []string
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			scopes = append(scopes, v)
		}
	}
	return scopes
}

func upsertRuntimeVar(runtimeVars []string, key, val string) []string {
	prefix := key + "="
	for i, e := range runtimeVars {
		if strings.HasPrefix(e, prefix) {
			runtimeVars[i] = prefix + val
			return runtimeVars
		}
	}
	return append(runtimeVars, prefix+val)
}

func envVars(entries []string) []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(entries))
	for _, entry := range entries {
		key, value, ok := splitRuntimeEntry(entry)
		if !ok {
			continue
		}
		out = append(out, corev1.EnvVar{Name: key, Value: value})
	}
	return out
}

func upsertEnvVarSource(env []corev1.EnvVar, value corev1.EnvVar) []corev1.EnvVar {
	for i := range env {
		if env[i].Name == value.Name {
			env[i] = value
			return env
		}
	}
	return append(env, value)
}

func splitRuntimeEntry(entry string) (string, string, bool) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false
	}
	return key, parts[1], true
}

func readServiceAccountNamespace() string {
	data, err := os.ReadFile(serviceAccountNamespaceFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func imagePullPolicy(raw string) corev1.PullPolicy {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "always":
		return corev1.PullAlways
	case "never":
		return corev1.PullNever
	default:
		return defaultImagePullPolicy
	}
}

func accessMode(raw string) corev1.PersistentVolumeAccessMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "readwritemany", "rwx":
		return corev1.ReadWriteMany
	case "readonlymany", "rox":
		return corev1.ReadOnlyMany
	default:
		return defaultWorkspaceAccessMode
	}
}

func parseDurationDefault(raw string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func boolPtrValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func kubernetesObjectName(parts ...string) string {
	joined := strings.Join(parts, "-")
	joined = strings.ToLower(strings.TrimSpace(joined))
	joined = nameInvalidChars.ReplaceAllString(joined, "-")
	joined = strings.Trim(joined, "-")
	if joined == "" {
		joined = "nopsai"
	}
	if len(joined) <= 63 {
		return joined
	}
	return strings.Trim(joined[:54], "-") + "-" + shortHash(joined)
}

func kubernetesLabelValue(raw string) string {
	value := kubernetesObjectName(raw)
	if len(value) <= 63 {
		return value
	}
	return value[:63]
}

func shortHash(value string) string {
	h := fnv32a(value)
	return fmt.Sprintf("%08x", h)
}

func fnv32a(value string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(value); i++ {
		hash ^= uint32(value[i])
		hash *= 16777619
	}
	return hash
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	return out
}

func mergeMaps(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, values := range maps {
		for key, value := range values {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func encodeRuntimePoolsYAML(pools map[string]config.RuntimePool) string {
	if len(pools) == 0 {
		return ""
	}
	data, err := yaml.Marshal(config.NormalizeRuntimePools(pools))
	if err != nil {
		log.Warn().Err(err).Msg("failed to encode runtime pools")
		return ""
	}
	return string(data)
}
