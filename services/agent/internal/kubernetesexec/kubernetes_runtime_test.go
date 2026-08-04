package kubernetesexec

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSameNodeAffinityTargetsAgentNodeThroughScheduler(t *testing.T) {
	affinity := sameNodeAffinity("worker-a")
	if affinity == nil || affinity.NodeAffinity == nil || affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatalf("sameNodeAffinity() did not build required node affinity: %#v", affinity)
	}

	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(terms) != 1 || len(terms[0].MatchFields) != 1 {
		t.Fatalf("sameNodeAffinity() terms = %#v", terms)
	}
	requirement := terms[0].MatchFields[0]
	if requirement.Key != "metadata.name" || requirement.Operator != corev1.NodeSelectorOpIn {
		t.Fatalf("sameNodeAffinity() requirement = %#v", requirement)
	}
	if len(requirement.Values) != 1 || requirement.Values[0] != "worker-a" {
		t.Fatalf("sameNodeAffinity() values = %#v", requirement.Values)
	}
}

func TestResolveKubernetesAffinityEnabledPrefersPipelineDirective(t *testing.T) {
	enabled := true
	if !resolveKubernetesAffinityEnabled("false", &enabled) {
		t.Fatal("pipeline affinity directive should override runner default")
	}

	disabled := false
	if resolveKubernetesAffinityEnabled("true", &disabled) {
		t.Fatal("pipeline affinity directive should be able to disable affinity")
	}
}

func TestResolveKubernetesAffinityEnabledDefaultsTrue(t *testing.T) {
	if !resolveKubernetesAffinityEnabled("", nil) {
		t.Fatal("affinity should default to enabled")
	}
}

func TestNormalizeKubernetesWorkingDirectoryAllowsDockerCompatibleAbsolutePath(t *testing.T) {
	got, err := normalizeKubernetesWorkingDirectory("/tmp/test")
	if err != nil {
		t.Fatalf("normalizeKubernetesWorkingDirectory() error = %v", err)
	}
	if got != "/tmp/test" {
		t.Fatalf("normalizeKubernetesWorkingDirectory() = %q, want /tmp/test", got)
	}
}

func TestKubernetesWorkspaceVolumeMountsUsePipelineWorkingDirectory(t *testing.T) {
	mounts := kubernetesWorkspaceVolumeMounts("/tmp/test")
	if len(mounts) != 1 {
		t.Fatalf("mount count = %d, want 1", len(mounts))
	}
	if mounts[0].Name != "workspace" || mounts[0].MountPath != "/tmp/test" {
		t.Fatalf("workspace mount = %#v, want workspace at /tmp/test", mounts[0])
	}
}

func TestCreateStepPodUsesWorkloadServiceAccountAndPullSecrets(t *testing.T) {
	automount := false
	runtime := &Runtime{
		client:           fake.NewSimpleClientset(),
		namespace:        "runs",
		workspacePVC:     "workspace-pvc",
		serviceAccount:   "nopsai-runner",
		workloadSA:       "nopsai-runner-workload",
		workloadSAToken:  &automount,
		imagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}, {Name: "step-regcred"}},
		imagePullPolicy:  corev1.PullIfNotPresent,
		taskTimeout:      time.Millisecond,
	}

	_, err := runtime.CreateStepPod(context.Background(), StepPodRequest{
		RunID:            "run-123",
		PipelineName:     "deploy",
		StepName:         "build",
		Image:            "alpine:private",
		WorkingDirectory: "/workspace",
	})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for pod") {
		t.Fatalf("CreateStepPod() error = %v, want wait timeout after pod creation", err)
	}
	podName := kubernetesObjectName("nopsai-step", "run-123", "build")
	pod, getErr := runtime.client.CoreV1().Pods("runs").Get(context.Background(), podName, metav1.GetOptions{})
	if getErr != nil {
		t.Fatalf("created pod get error = %v", getErr)
	}
	if pod.Spec.ServiceAccountName != "nopsai-runner-workload" {
		t.Fatalf("service account = %q, want workload account", pod.Spec.ServiceAccountName)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("automount token = %#v, want false", pod.Spec.AutomountServiceAccountToken)
	}
	if len(pod.Spec.ImagePullSecrets) != 2 || pod.Spec.ImagePullSecrets[0].Name != "regcred" || pod.Spec.ImagePullSecrets[1].Name != "step-regcred" {
		t.Fatalf("image pull secrets = %#v", pod.Spec.ImagePullSecrets)
	}
	assertKubernetesPodSecurityDefaults(t, pod)
	assertKubernetesContainerSecurityDefaults(t, pod.Spec.Containers[0].SecurityContext)
}

func TestCreateStepPodRejectsExistingPodName(t *testing.T) {
	existingName := kubernetesObjectName("nopsai-step", "run-123", "build")
	runtime := &Runtime{
		client: fake.NewSimpleClientset(&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: existingName, Namespace: "runs"},
		}),
		namespace:    "runs",
		workspacePVC: "workspace-pvc",
		taskTimeout:  time.Millisecond,
	}

	_, err := runtime.CreateStepPod(context.Background(), StepPodRequest{
		RunID:            "run-123",
		PipelineName:     "deploy",
		StepName:         "build",
		Image:            "alpine:latest",
		WorkingDirectory: "/workspace",
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to reuse") {
		t.Fatalf("CreateStepPod() error = %v, want refusal to reuse existing pod", err)
	}
}

func TestKubernetesStepVolumesCreateNamedPVCs(t *testing.T) {
	runtime := &Runtime{
		client:        fake.NewSimpleClientset(),
		namespace:     "runs",
		workspaceSize: "1Gi",
		accessMode:    corev1.ReadWriteOnce,
		podLabels:     map[string]string{"custom": "label"},
	}

	mounts, volumes, err := runtime.kubernetesStepVolumes(context.Background(), []string{"cache:/cache"}, "run-123")
	if err != nil {
		t.Fatalf("kubernetesStepVolumes() error = %v", err)
	}
	if len(mounts) != 1 || mounts[0].MountPath != "/cache" || len(volumes) != 1 {
		t.Fatalf("mounts/volumes = %#v/%#v", mounts, volumes)
	}
	claimName := "cache"
	if volumes[0].PersistentVolumeClaim == nil || volumes[0].PersistentVolumeClaim.ClaimName != claimName {
		t.Fatalf("step volume claim = %#v, want %q", volumes[0].PersistentVolumeClaim, claimName)
	}
	pvc, err := runtime.client.CoreV1().PersistentVolumeClaims("runs").Get(context.Background(), claimName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("created pvc get error = %v", err)
	}
	if pvc.Labels[kubernetesManagedLabel] != "true" ||
		pvc.Labels["app.kubernetes.io/component"] != kubernetesStepVolumeComponent ||
		pvc.Labels[kubernetesStepVolumeLogicalLabel] != "cache" {
		t.Fatalf("created pvc labels = %#v, want pipeline volume labels", pvc.Labels)
	}
}

func TestKubernetesStepVolumesReuseNamedPVCAcrossRuns(t *testing.T) {
	runtime := &Runtime{
		client:        fake.NewSimpleClientset(),
		namespace:     "runs",
		workspaceSize: "1Gi",
		accessMode:    corev1.ReadWriteOnce,
	}

	if _, _, err := runtime.kubernetesStepVolumes(context.Background(), []string{"cache:/cache"}, "run-123"); err != nil {
		t.Fatalf("first kubernetesStepVolumes() error = %v", err)
	}
	if _, _, err := runtime.kubernetesStepVolumes(context.Background(), []string{"cache:/cache"}, "run-123"); err != nil {
		t.Fatalf("second kubernetesStepVolumes() error = %v, want same run/logical volume to be reusable", err)
	}
	if _, _, err := runtime.kubernetesStepVolumes(context.Background(), []string{"cache:/cache"}, "run-456"); err != nil {
		t.Fatalf("other run kubernetesStepVolumes() error = %v", err)
	}
	pvcs, err := runtime.client.CoreV1().PersistentVolumeClaims("runs").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list pvcs error = %v", err)
	}
	if len(pvcs.Items) != 1 || pvcs.Items[0].Name != "cache" {
		t.Fatalf("pvcs = %#v, want one named cache", pvcs.Items)
	}
}

func TestKubernetesStepVolumesAllowExistingUnownedPVC(t *testing.T) {
	runtime := &Runtime{
		client: fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "cache", Namespace: "runs"},
		}),
		namespace:     "runs",
		workspaceSize: "1Gi",
		accessMode:    corev1.ReadWriteOnce,
	}

	mounts, volumes, err := runtime.kubernetesStepVolumes(context.Background(), []string{"cache:/cache"}, "run-123")
	if err != nil {
		t.Fatalf("kubernetesStepVolumes() error = %v", err)
	}
	if len(mounts) != 1 || len(volumes) != 1 || volumes[0].PersistentVolumeClaim == nil || volumes[0].PersistentVolumeClaim.ClaimName != "cache" {
		t.Fatalf("mounts/volumes = %#v/%#v, want existing cache pvc", mounts, volumes)
	}
}

func assertKubernetesPodSecurityDefaults(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	if pod.Spec.SecurityContext == nil || pod.Spec.SecurityContext.SeccompProfile == nil {
		t.Fatalf("pod security context missing: %#v", pod.Spec.SecurityContext)
	}
	if pod.Spec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Fatalf("pod seccomp = %q, want RuntimeDefault", pod.Spec.SecurityContext.SeccompProfile.Type)
	}
}

func assertKubernetesContainerSecurityDefaults(t *testing.T, security *corev1.SecurityContext) {
	t.Helper()
	if security == nil {
		t.Fatal("container security context missing")
	}
	if security.AllowPrivilegeEscalation == nil || *security.AllowPrivilegeEscalation {
		t.Fatalf("allow privilege escalation = %#v, want false", security.AllowPrivilegeEscalation)
	}
	if security.SeccompProfile == nil || security.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Fatalf("container seccomp = %#v, want RuntimeDefault", security.SeccompProfile)
	}
	if security.Capabilities != nil && len(security.Capabilities.Drop) != 0 {
		t.Fatalf("capabilities = %#v, want runtime/image default capabilities", security.Capabilities)
	}
}
