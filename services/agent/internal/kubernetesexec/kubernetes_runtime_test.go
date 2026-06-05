package kubernetesexec

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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
