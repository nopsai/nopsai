package service

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicelog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

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
	kubernetesPlatformIDEnv        = "NOPSAI_PLATFORM_ID"
	kubernetesPodLogDrainTimeout   = 15 * time.Second
	kubernetesPodLogStopTimeout    = 2 * time.Second
	kubernetesPodLogRetryDelay     = time.Second
)

var nameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

type Runner interface {
	ServeForever()
}

type RunnerOptions struct {
	Config          *config.Config
	RunnerID        string
	DispatcherAddr  string
	Capacity        int32
	DispatcherCreds *serviceauth.Credentials
	TransportCreds  credentials.TransportCredentials
	Client          kubernetes.Interface
}

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
	workloadSA       string
	workloadSAToken  *bool
	imagePullSecrets []corev1.LocalObjectReference
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
	podLogStream     podLogStreamFunc
	podLogRetryDelay time.Duration
}

func NewKubernetesRunner(options RunnerOptions) Runner {
	cfg := options.Config
	if cfg == nil {
		cfg = &config.Config{}
	}
	kcfg := config.NormalizeKubernetesConfig(cfg.Kubernetes)
	namespace := firstNonEmpty(kcfg.Namespace, readServiceAccountNamespace(), defaultRunnerNamespace)
	serviceAccount := strings.TrimSpace(kcfg.ServiceAccount)
	workloadSA := firstNonEmpty(kcfg.WorkloadServiceAccount, serviceAccount)
	workspaceSize := firstNonEmpty(kcfg.DefaultWorkspaceSize, defaultWorkspaceSize)
	workspaceMode := firstNonEmpty(kcfg.WorkspaceVolumeMode, kubernetesWorkspaceVolumePVC)
	runtimePoolsYAML := encodeRuntimePoolsYAML(cfg.RuntimePools)

	capacity := options.Capacity
	if capacity <= 0 {
		capacity = 1
	}

	return &kubernetesRunner{
		id:               firstNonEmpty(options.RunnerID, "k8s-runner"),
		scopes:           parseScopes(cfg.RunnerScopes),
		capacity:         capacity,
		dispatcherAddr:   firstNonEmpty(options.DispatcherAddr, "dispatcher:9090"),
		dispatcherCreds:  options.DispatcherCreds,
		transportCreds:   options.TransportCreds,
		client:           options.Client,
		namespace:        namespace,
		serviceAccount:   serviceAccount,
		workloadSA:       workloadSA,
		workloadSAToken:  cloneBoolPtr(kcfg.WorkloadAutomountSAToken),
		imagePullSecrets: imagePullSecretRefs(kcfg.ImagePullSecrets),
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
}

func (r *kubernetesRunner) ServeForever() {
	log.Info().
		Str("runner_id", r.id).
		Str("runtime", kubernetesRuntimeName).
		Str("namespace", r.namespace).
		Str("dispatcher_addr", r.dispatcherAddr).
		Strs("scopes", r.scopes).
		Int("capacity", int(r.capacity)).
		Bool("affinity_enabled", r.affinityEnabled).
		Msg("kubernetes runner starting")

	for {
		if err := r.connectAndServe(); err != nil {
			log.Error().Err(err).Msg("dispatcher stream ended, retrying")
			time.Sleep(3 * time.Second)
		}
	}
}

func (r *kubernetesRunner) connectAndServe() error {
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(r.transportCreds),
		grpc.WithBlock(),
		grpc.WithChainUnaryInterceptor(servicelog.GRPCUnaryClientInterceptor()),
		grpc.WithChainStreamInterceptor(servicelog.GRPCStreamClientInterceptor()),
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
		"log_source_id":              "runner:" + r.id,
	}
	if host, err := os.Hostname(); err == nil {
		metadata["hostname"] = strings.TrimSpace(host)
	}
	if selector := strings.TrimSpace(os.Getenv("KUBERNETES_RUNNER_LABEL_SELECTOR")); selector != "" {
		metadata["kubernetes_label_selector"] = selector
	}
	if runnerName := strings.TrimSpace(os.Getenv("RUNNER_NAME")); runnerName != "" && runnerName != r.id {
		metadata["runner_name"] = runnerName
	}
	if platformID := strings.TrimSpace(os.Getenv(kubernetesPlatformIDEnv)); platformID != "" {
		metadata["nopsai_platform_id"] = platformID
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
			r.emitRunLog(context.Background(), dispatcher, job.RunId, "Kubernetes runner failed to prepare workspace: "+err.Error())
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}

		runtimeVars := r.agentRuntimeVars(job, workspacePVC)
		pod, err := r.createAgentPod(context.Background(), podName, agentImage, workspacePVC, runtimeVars, job)
		if err != nil {
			r.emitRunLog(context.Background(), dispatcher, job.RunId, "Kubernetes runner failed to start agent pod: "+err.Error())
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}
		if r.cleanupPods {
			defer r.deletePod(context.Background(), podName)
		}

		log.Info().Str("run_id", job.RunId).Str("pod", pod.Name).Str("workspace_pvc", workspacePVC).Msg("started agent pod")
		r.emitRunLog(context.Background(), dispatcher, job.RunId, fmt.Sprintf("Kubernetes runner started agent pod %s in namespace %s.", pod.Name, r.namespace))

		runCtx, cancelRun := context.WithCancel(context.Background())
		defer cancelRun()
		go r.monitorRunCancellation(runCtx, dispatcher, job.RunId, podName)

		logCtx, cancelLogs := context.WithCancel(context.Background())
		defer cancelLogs()
		logDone := make(chan struct{})
		go func() {
			defer close(logDone)
			r.streamPodLogs(logCtx, dispatcher, job.RunId, podName)
		}()

		phase, err := r.waitForPodCompletion(context.Background(), podName)
		cancelRun()
		if err != nil {
			stopPodLogForwarder(logDone, cancelLogs, job.RunId, podName)
			r.emitRunLog(context.Background(), dispatcher, job.RunId, "Kubernetes runner failed while waiting for agent pod completion: "+err.Error())
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}
		if !waitForPodLogDrain(logDone, cancelLogs, job.RunId, podName) {
			r.emitRunLog(context.Background(), dispatcher, job.RunId, fmt.Sprintf("Kubernetes runner timed out waiting for logs from agent pod %s to drain.", podName))
		}
		if phase != corev1.PodSucceeded {
			r.emitRunLog(context.Background(), dispatcher, job.RunId, fmt.Sprintf("Kubernetes runner agent pod %s completed with phase %s.", podName, phase))
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("agent pod completed with phase %s", phase))
			return
		}
		sendJobResult(sendCh, job.RunId, "completed", "")
	}()
}

func (r *kubernetesRunner) agentRuntimeVars(job *proto.JobRequest, workspacePVC string) []string {
	runtimeVars := append([]string(nil), job.Env...)
	runtimeVars = upsertRuntimeVar(runtimeVars, "DISPATCHER_GRPC_ADDRESS", strings.TrimSpace(r.dispatcherAddr))
	runtimeVars = upsertRuntimeVar(runtimeVars, "RUNNER_ID", r.id)
	runtimeVars = upsertRuntimeVar(runtimeVars, "NOPSAI_RUNTIME", kubernetesRuntimeName)
	runtimeVars = upsertRuntimeVar(runtimeVars, "SHARED_VOLUME_NAME", workspacePVC)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_NAMESPACE", r.namespace)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_SERVICE_ACCOUNT", r.serviceAccount)
	runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_WORKLOAD_SERVICE_ACCOUNT", r.effectiveWorkloadServiceAccount())
	if r.workloadSAToken != nil {
		runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_WORKLOAD_AUTOMOUNT_SERVICE_ACCOUNT_TOKEN", strconv.FormatBool(*r.workloadSAToken))
	}
	if secrets := imagePullSecretNames(r.imagePullSecrets); len(secrets) > 0 {
		runtimeVars = upsertRuntimeVar(runtimeVars, "KUBERNETES_IMAGE_PULL_SECRETS", strings.Join(secrets, ","))
	}
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

func waitForPodLogDrain(done <-chan struct{}, cancel context.CancelFunc, runID, podName string) bool {
	select {
	case <-done:
		return true
	case <-time.After(kubernetesPodLogDrainTimeout):
		log.Warn().
			Str("run_id", runID).
			Str("pod", podName).
			Dur("timeout", kubernetesPodLogDrainTimeout).
			Msg("timed out waiting for pod logs to drain")
		cancel()
		select {
		case <-done:
		case <-time.After(kubernetesPodLogStopTimeout):
		}
		return false
	}
}

func (r *kubernetesRunner) effectiveWorkloadServiceAccount() string {
	if r == nil {
		return ""
	}
	return firstNonEmpty(r.workloadSA, r.serviceAccount)
}

func stopPodLogForwarder(done <-chan struct{}, cancel context.CancelFunc, runID, podName string) {
	cancel()
	select {
	case <-done:
	case <-time.After(kubernetesPodLogStopTimeout):
		log.Warn().
			Str("run_id", runID).
			Str("pod", podName).
			Dur("timeout", kubernetesPodLogStopTimeout).
			Msg("timed out stopping pod log forwarder")
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

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func imagePullSecretRefs(names []string) []corev1.LocalObjectReference {
	if len(names) == 0 {
		return nil
	}
	refs := make([]corev1.LocalObjectReference, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" {
			refs = append(refs, corev1.LocalObjectReference{Name: name})
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func imagePullSecretNames(refs []corev1.LocalObjectReference) []string {
	if len(refs) == 0 {
		return nil
	}
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
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
