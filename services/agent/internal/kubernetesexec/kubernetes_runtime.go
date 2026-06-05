package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	k8sexec "k8s.io/client-go/util/exec"
)

const (
	kubernetesRuntimeName             = "kubernetes"
	kubernetesDefaultNamespace        = "default"
	kubernetesDefaultWorkspaceSize    = "5Gi"
	kubernetesDefaultAccessMode       = corev1.ReadWriteOnce
	kubernetesDefaultTaskTimeout      = 30 * time.Minute
	kubernetesDefaultImagePullPolicy  = corev1.PullIfNotPresent
	kubernetesDefaultWorkspaceMount   = "/workspace"
	kubernetesStepContainerName       = "step"
	kubernetesDefaultRuntimePool      = "default"
	kubernetesServiceAccountNamespace = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

var kubernetesNameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

type kubernetesAgentRuntime struct {
	client          kubernetes.Interface
	restConfig      *rest.Config
	namespace       string
	workspacePVC    string
	nodeName        string
	affinityEnabled bool
	serviceAccount  string
	imagePullPolicy corev1.PullPolicy
	storageClass    string
	workspaceSize   string
	workspaceMode   string
	accessMode      corev1.PersistentVolumeAccessMode
	taskTimeout     time.Duration
	cleanupPods     bool
	podLabels       map[string]string
	podAnnotations  map[string]string
	runtimePools    map[string]appconfig.RuntimePool
}

type kubernetesStepPodRequest struct {
	RunID            string
	PipelineName     string
	StepName         string
	Image            string
	WorkingDirectory string
	Env              []string
	Volumes          []string
	RuntimePool      string
}

func newKubernetesAgentRuntimeFromEnv(runID, pipelineName, workspacePVC string, pipelineAffinityEnabled *bool) (*kubernetesAgentRuntime, error) {
	restConfig, err := kubernetesRESTConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	namespace := firstNonEmpty(os.Getenv("KUBERNETES_NAMESPACE"), readServiceAccountNamespace(), kubernetesDefaultNamespace)
	taskTimeout := parseDurationDefault(os.Getenv("KUBERNETES_DEFAULT_TASK_TIMEOUT"), kubernetesDefaultTaskTimeout)
	accessMode := kubernetesAccessMode(os.Getenv("KUBERNETES_DEFAULT_WORKSPACE_ACCESS_MODE"))
	imagePullPolicy := kubernetesPullPolicy(os.Getenv("KUBERNETES_DEFAULT_IMAGE_PULL_POLICY"))
	workspaceSize := strings.TrimSpace(os.Getenv("KUBERNETES_DEFAULT_WORKSPACE_SIZE"))
	if workspaceSize == "" {
		workspaceSize = kubernetesDefaultWorkspaceSize
	}
	affinityEnabled := resolveKubernetesAffinityEnabled(os.Getenv("KUBERNETES_AFFINITY_ENABLED"), pipelineAffinityEnabled)

	runtime := &kubernetesAgentRuntime{
		client:          clientset,
		restConfig:      restConfig,
		namespace:       namespace,
		workspacePVC:    strings.TrimSpace(workspacePVC),
		nodeName:        strings.TrimSpace(os.Getenv("KUBERNETES_NODE_NAME")),
		affinityEnabled: affinityEnabled,
		serviceAccount:  strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_ACCOUNT")),
		imagePullPolicy: imagePullPolicy,
		storageClass:    strings.TrimSpace(os.Getenv("KUBERNETES_STORAGE_CLASS")),
		workspaceSize:   workspaceSize,
		workspaceMode:   normalizeKubernetesWorkspaceMode(os.Getenv("KUBERNETES_WORKSPACE_VOLUME_MODE")),
		accessMode:      accessMode,
		taskTimeout:     taskTimeout,
		cleanupPods:     parseBoolDefault(os.Getenv("KUBERNETES_CLEANUP_FINISHED_PODS"), true),
		podLabels:       parseStringMapEnv("KUBERNETES_POD_LABELS"),
		podAnnotations:  parseStringMapEnv("KUBERNETES_POD_ANNOTATIONS"),
		runtimePools:    parseRuntimePoolsEnv(),
	}
	if runtime.workspacePVC == "" {
		return nil, fmt.Errorf("SHARED_VOLUME_NAME must be set for kubernetes runtime")
	}
	if err := runtime.prepareWorkspacePVC(context.Background()); err != nil {
		return nil, fmt.Errorf("prepare workspace pvc: %w", err)
	}
	agentLog(runID, pipelineName).Info().
		Str("namespace", runtime.namespace).
		Str("workspace_pvc", runtime.workspacePVC).
		Str("workspace_mode", runtime.workspaceMode).
		Str("node", runtime.nodeName).
		Bool("affinity_enabled", runtime.affinityEnabled).
		Msg("Kubernetes task runtime initialized")
	return runtime, nil
}

func kubernetesRESTConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	kubeconfig := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	if kubeconfig == "" {
		return nil, fmt.Errorf("kubernetes config is unavailable")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("load kubernetes config: %w", err)
	}
	return cfg, nil
}

func (r *kubernetesAgentRuntime) createStepPod(ctx context.Context, req kubernetesStepPodRequest) (string, error) {
	if r == nil {
		return "", fmt.Errorf("kubernetes runtime is not initialized")
	}
	image := strings.TrimSpace(req.Image)
	if image == "" {
		return "", fmt.Errorf("step image is required for kubernetes runtime")
	}
	workingDirectory, err := normalizeKubernetesWorkingDirectory(req.WorkingDirectory)
	if err != nil {
		return "", err
	}

	volumeMounts := kubernetesWorkspaceVolumeMounts(workingDirectory)
	volumes := []corev1.Volume{{
		Name: "workspace",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: r.workspacePVC},
		},
	}}

	extraMounts, extraVolumes, err := r.kubernetesStepVolumes(ctx, req.Volumes)
	if err != nil {
		return "", err
	}
	volumeMounts = append(volumeMounts, extraMounts...)
	volumes = append(volumes, extraVolumes...)

	runtimePool := strings.TrimSpace(req.RuntimePool)
	nodeSelector, resources, err := r.runtimePoolScheduling(runtimePool)
	if err != nil {
		return "", err
	}

	podName := kubernetesObjectName("nopsai-step", req.RunID, req.StepName)
	labels := mergeKubernetesStringMaps(r.podLabels, map[string]string{
		"app.kubernetes.io/name":      "nopsai",
		"app.kubernetes.io/component": "pipeline-step",
		"nopsai.io/run-id":            kubernetesLabelValue(req.RunID),
		"nopsai.io/pipeline":          kubernetesLabelValue(req.PipelineName),
		"nopsai.io/step":              kubernetesLabelValue(req.StepName),
	})

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        podName,
			Namespace:   r.namespace,
			Labels:      labels,
			Annotations: cloneKubernetesStringMap(r.podAnnotations),
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			NodeSelector:       nodeSelector,
			ServiceAccountName: r.serviceAccount,
			Volumes:            volumes,
			Containers: []corev1.Container{{
				Name:            kubernetesStepContainerName,
				Image:           image,
				ImagePullPolicy: r.imagePullPolicy,
				WorkingDir:      workingDirectory,
				Command:         []string{"sh", "-c", fmt.Sprintf("mkdir -p %s && tail -f /dev/null", shellQuote(workingDirectory))},
				Env:             kubernetesEnv(req.Env),
				VolumeMounts:    volumeMounts,
				Resources:       resources,
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						Exec: &corev1.ExecAction{Command: []string{"sh", "-c", "true"}},
					},
					InitialDelaySeconds: 1,
					PeriodSeconds:       5,
					TimeoutSeconds:      2,
					SuccessThreshold:    1,
					FailureThreshold:    3,
				},
			}},
		},
	}
	if r.affinityEnabled && r.nodeName != "" {
		pod.Spec.NodeSelector = nil
		pod.Spec.Affinity = sameNodeAffinity(r.nodeName)
	}

	if _, err := r.client.CoreV1().Pods(r.namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return "", fmt.Errorf("create step pod: %w", err)
		}
	}
	if err := r.waitForPodRunning(ctx, podName); err != nil {
		return "", err
	}
	return podName, nil
}

func (r *kubernetesAgentRuntime) executeAction(ctx context.Context, podName string, action *proto.Action, runtimeVars []string, workingDirectory string) (string, string, int) {
	workingDirectory, err := normalizeKubernetesWorkingDirectory(workingDirectory)
	if err != nil {
		return "", err.Error(), 1
	}

	var cmdStr string
	switch action.Type {
	case "EXECUTE_COMMAND":
		cmdStr = action.GetCommandAction().Command
	case "REPLACE_FILE":
		content := action.GetFileAction().Content
		encodedContent := base64Encode(content)
		filePath, err := resolveActionFilePath(workingDirectory, action.GetFileAction().Path)
		if err != nil {
			return "", err.Error(), 1
		}
		cmdStr = fmt.Sprintf("printf %%s %s | base64 -d > %s", shellQuote(encodedContent), shellQuote(filePath))
	case "RETURN_ANSWER":
		ansAction := action.GetAnswerAction()
		if ansAction == nil {
			return "", "Invalid answer action payload", 1
		}
		return ansAction.Answer, "", 0
	default:
		return "", "Unknown action type", 1
	}

	command := []string{"sh", "-c", prefixRuntimeEnv(runtimeVars) + "cd " + shellQuote(workingDirectory) + " && " + cmdStr}
	execReq := r.client.CoreV1().RESTClient().
		Post().
		Namespace(r.namespace).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: kubernetesStepContainerName,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(r.restConfig, "POST", execReq.URL())
	if err != nil {
		return "", fmt.Sprintf("failed to create kubernetes exec: %v", err), 1
	}

	var stdout, stderr bytes.Buffer
	streamCtx := ctx
	if streamCtx == nil {
		streamCtx = context.Background()
	}
	err = executor.StreamWithContext(streamCtx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	if err == nil {
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), 0
	}

	exitCode := 1
	var exitErr k8sexec.CodeExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitStatus()
	}
	if strings.TrimSpace(stderr.String()) == "" {
		stderr.WriteString(err.Error())
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), exitCode
}

func (r *kubernetesAgentRuntime) cleanupPod(ctx context.Context, podName string) {
	if r == nil || strings.TrimSpace(podName) == "" || !r.cleanupPods {
		return
	}
	grace := int64(1)
	deleteCtx := ctx
	if deleteCtx == nil {
		var cancel context.CancelFunc
		deleteCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}
	err := r.client.CoreV1().Pods(r.namespace).Delete(deleteCtx, podName, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Warn().Err(err).Str("pod", podName).Msg("failed to delete kubernetes step pod")
	}
}

func (r *kubernetesAgentRuntime) kubernetesStepVolumes(ctx context.Context, volumeSpecs []string) ([]corev1.VolumeMount, []corev1.Volume, error) {
	if len(volumeSpecs) == 0 {
		return nil, nil, nil
	}
	mounts := make([]corev1.VolumeMount, 0, len(volumeSpecs))
	volumes := make([]corev1.Volume, 0, len(volumeSpecs))
	for _, spec := range volumeSpecs {
		parts := strings.Split(spec, ":")
		if len(parts) != 2 {
			return nil, nil, fmt.Errorf("invalid volume format %q; must be 'pvc-name:mount-path'", spec)
		}
		claimName := kubernetesPVCName(parts[0])
		mountPath := strings.TrimSpace(parts[1])
		if claimName == "" || mountPath == "" || !strings.HasPrefix(mountPath, "/") {
			return nil, nil, fmt.Errorf("invalid kubernetes volume %q", spec)
		}
		if err := r.ensurePVC(ctx, claimName, r.workspaceSize, r.accessMode); err != nil {
			return nil, nil, fmt.Errorf("ensure pvc %s: %w", claimName, err)
		}
		volumeName := kubernetesObjectName("vol", claimName)
		mounts = append(mounts, corev1.VolumeMount{Name: volumeName, MountPath: mountPath})
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
			},
		})
	}
	return mounts, volumes, nil
}

func (r *kubernetesAgentRuntime) prepareWorkspacePVC(ctx context.Context) error {
	switch r.workspaceMode {
	case "emptyDir":
		return fmt.Errorf("emptyDir workspace mode is not compatible with multi-pod pipeline execution")
	case "existing":
		if _, err := r.client.CoreV1().PersistentVolumeClaims(r.namespace).Get(ctx, r.workspacePVC, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("workspace pvc %s is not available: %w", r.workspacePVC, err)
		}
		return nil
	default:
		if _, err := r.client.CoreV1().PersistentVolumeClaims(r.namespace).Get(ctx, r.workspacePVC, metav1.GetOptions{}); err == nil {
			return nil
		} else if !apierrors.IsNotFound(err) {
			return err
		}
		return r.ensurePVC(ctx, r.workspacePVC, r.workspaceSize, r.accessMode)
	}
}

func (r *kubernetesAgentRuntime) ensurePVC(ctx context.Context, name, size string, accessMode corev1.PersistentVolumeAccessMode) error {
	name = kubernetesPVCName(name)
	if name == "" {
		return fmt.Errorf("pvc name is required")
	}
	if _, err := r.client.CoreV1().PersistentVolumeClaims(r.namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	quantity, err := resource.ParseQuantity(firstNonEmpty(size, kubernetesDefaultWorkspaceSize))
	if err != nil {
		return fmt.Errorf("parse pvc size: %w", err)
	}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.namespace,
			Labels: mergeKubernetesStringMaps(r.podLabels, map[string]string{
				"app.kubernetes.io/name":      "nopsai",
				"app.kubernetes.io/component": "pipeline-workspace",
			}),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: quantity},
			},
		},
	}
	if r.storageClass != "" {
		pvc.Spec.StorageClassName = &r.storageClass
	}
	if _, err := r.client.CoreV1().PersistentVolumeClaims(r.namespace).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (r *kubernetesAgentRuntime) waitForPodRunning(ctx context.Context, podName string) error {
	waitCtx := ctx
	if waitCtx == nil {
		waitCtx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(waitCtx, r.taskTimeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timed out waiting for pod %s to run", podName)
		case <-ticker.C:
			pod, err := r.client.CoreV1().Pods(r.namespace).Get(waitCtx, podName, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			switch pod.Status.Phase {
			case corev1.PodRunning:
				return nil
			case corev1.PodFailed, corev1.PodSucceeded:
				return fmt.Errorf("pod %s finished before it was ready: %s", podName, pod.Status.Phase)
			}
			if reason := firstContainerWaitingReason(pod); reason != "" {
				log.Debug().Str("pod", podName).Str("reason", reason).Msg("waiting for kubernetes step pod")
			}
		}
	}
}

func (r *kubernetesAgentRuntime) runtimePoolScheduling(poolName string) (map[string]string, corev1.ResourceRequirements, error) {
	resources := corev1.ResourceRequirements{}
	if len(r.runtimePools) == 0 {
		return nil, resources, nil
	}
	poolName = strings.TrimSpace(poolName)
	if poolName == "" {
		poolName = kubernetesDefaultRuntimePool
	}
	pool, ok := r.runtimePools[poolName]
	if !ok {
		return nil, resources, fmt.Errorf("runtime pool %q is not configured", poolName)
	}
	requests, err := kubernetesResourceList(pool.Resources.Requests)
	if err != nil {
		return nil, resources, fmt.Errorf("runtime pool %s requests: %w", poolName, err)
	}
	limits, err := kubernetesResourceList(pool.Resources.Limits)
	if err != nil {
		return nil, resources, fmt.Errorf("runtime pool %s limits: %w", poolName, err)
	}
	resources.Requests = requests
	resources.Limits = limits
	return cloneKubernetesStringMap(pool.NodeSelector), resources, nil
}

func kubernetesResourceList(values map[string]string) (corev1.ResourceList, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := corev1.ResourceList{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		qty, err := resource.ParseQuantity(values[key])
		if err != nil {
			return nil, fmt.Errorf("%s=%s: %w", key, values[key], err)
		}
		out[corev1.ResourceName(key)] = qty
	}
	return out, nil
}

func normalizeKubernetesWorkspaceMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "pvc", "dynamic", "create":
		return "pvc"
	case "existing", "existing_pvc", "existing-pvc":
		return "existing"
	case "emptydir", "empty_dir", "empty-dir":
		return "emptyDir"
	default:
		return strings.TrimSpace(raw)
	}
}

func sameNodeAffinity(nodeName string) *corev1.Affinity {
	nodeName = strings.TrimSpace(nodeName)
	if nodeName == "" {
		return nil
	}
	return &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchFields: []corev1.NodeSelectorRequirement{{
						Key:      "metadata.name",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{nodeName},
					}},
				}},
			},
		},
	}
}

func normalizeKubernetesWorkingDirectory(workingDirectory string) (string, error) {
	return models.NormalizePipelineWorkingDirectory(workingDirectory)
}

func kubernetesWorkspaceVolumeMounts(workingDirectory string) []corev1.VolumeMount {
	return []corev1.VolumeMount{{
		Name:      "workspace",
		MountPath: workingDirectory,
	}}
}

func kubernetesEnv(entries []string) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0, len(entries))
	for _, entry := range entries {
		key, value, ok := splitRuntimeEntry(entry)
		if !ok {
			continue
		}
		env = append(env, corev1.EnvVar{Name: key, Value: value})
	}
	return env
}

func prefixRuntimeEnv(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, entry := range entries {
		key, value, ok := splitRuntimeEntry(entry)
		if !ok {
			continue
		}
		builder.WriteString("export ")
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(shellQuote(value))
		builder.WriteString("; ")
	}
	return builder.String()
}

func parseRuntimePoolsEnv() map[string]appconfig.RuntimePool {
	raw := strings.TrimSpace(os.Getenv("KUBERNETES_RUNTIME_POOLS"))
	if raw == "" {
		return nil
	}
	var pools map[string]appconfig.RuntimePool
	if err := yaml.Unmarshal([]byte(raw), &pools); err != nil {
		log.Warn().Err(err).Msg("failed to parse KUBERNETES_RUNTIME_POOLS")
		return nil
	}
	return appconfig.NormalizeRuntimePools(pools)
}

func parseStringMapEnv(name string) map[string]string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	values := map[string]string{}
	if err := yaml.Unmarshal([]byte(raw), &values); err != nil {
		log.Warn().Err(err).Str("env", name).Msg("failed to parse string map env")
		return nil
	}
	return values
}

func firstContainerWaitingReason(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			return status.State.Waiting.Reason
		}
	}
	return ""
}

func kubernetesAccessMode(raw string) corev1.PersistentVolumeAccessMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "readwritemany", "rwx":
		return corev1.ReadWriteMany
	case "readonlymany", "rox":
		return corev1.ReadOnlyMany
	default:
		return kubernetesDefaultAccessMode
	}
}

func kubernetesPullPolicy(raw string) corev1.PullPolicy {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "always":
		return corev1.PullAlways
	case "never":
		return corev1.PullNever
	default:
		return kubernetesDefaultImagePullPolicy
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

func parseBoolDefault(raw string, fallback bool) bool {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return value
}

func resolveKubernetesAffinityEnabled(defaultRaw string, pipelineAffinityEnabled *bool) bool {
	if pipelineAffinityEnabled != nil {
		return *pipelineAffinityEnabled
	}
	return parseBoolDefault(defaultRaw, true)
}

func readServiceAccountNamespace() string {
	data, err := os.ReadFile(kubernetesServiceAccountNamespace)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func kubernetesPVCName(raw string) string {
	return kubernetesObjectName("", raw)
}

func kubernetesObjectName(parts ...string) string {
	joined := strings.Join(parts, "-")
	joined = strings.ToLower(strings.TrimSpace(joined))
	joined = kubernetesNameInvalidChars.ReplaceAllString(joined, "-")
	joined = strings.Trim(joined, "-")
	if joined == "" {
		joined = "nopsai"
	}
	if len(joined) <= 63 {
		return joined
	}
	hash := shortHash(joined)
	return strings.Trim(joined[:54], "-") + "-" + hash
}

func kubernetesLabelValue(raw string) string {
	value := kubernetesObjectName(raw)
	if len(value) <= 63 {
		return value
	}
	return value[:63]
}

func shortHash(value string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%08x", h.Sum32())
}

func mergeKubernetesStringMaps(maps ...map[string]string) map[string]string {
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

func cloneKubernetesStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
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

func base64Encode(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
