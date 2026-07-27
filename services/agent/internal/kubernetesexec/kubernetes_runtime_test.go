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
}
