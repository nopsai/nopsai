package main

import (
	"testing"

	"nopsai/pkg/proto"

	corev1 "k8s.io/api/core/v1"
)

func TestWorkspaceClaimNameUsesAgentEphemeralPVCName(t *testing.T) {
	r := &kubernetesRunner{workspaceMode: kubernetesWorkspaceVolumePVC}
	claimName, err := r.workspaceClaimName("agent-run-123", &proto.JobRequest{SharedVolumeName: "legacy-volume"})
	if err != nil {
		t.Fatalf("workspaceClaimName() error = %v", err)
	}
	if claimName != "agent-run-123-workspace" {
		t.Fatalf("workspaceClaimName() = %q, want agent-owned ephemeral PVC name", claimName)
	}
}

func TestWorkspaceVolumeSourceUsesEphemeralPVCByDefault(t *testing.T) {
	r := &kubernetesRunner{
		id:              "k8s-runner-1",
		workspaceMode:   kubernetesWorkspaceVolumePVC,
		workspaceSize:   "5Gi",
		workspaceAccess: corev1.ReadWriteOnce,
		storageClass:    "fast-rwo",
	}

	source, err := r.workspaceVolumeSource("agent-run-123-workspace", &proto.JobRequest{RunId: "run-123"})
	if err != nil {
		t.Fatalf("workspaceVolumeSource() error = %v", err)
	}
	if source.Ephemeral == nil || source.Ephemeral.VolumeClaimTemplate == nil {
		t.Fatalf("workspaceVolumeSource() did not use an ephemeral PVC: %#v", source)
	}
	if source.PersistentVolumeClaim != nil {
		t.Fatalf("workspaceVolumeSource() should not reference a runner-created PVC")
	}
	if got := source.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName; got == nil || *got != "fast-rwo" {
		t.Fatalf("storage class = %v, want fast-rwo", got)
	}
}

func TestWorkspaceVolumeSourceUsesExistingPVCWhenConfigured(t *testing.T) {
	r := &kubernetesRunner{workspaceMode: kubernetesWorkspaceExistingPVC}
	source, err := r.workspaceVolumeSource("shared-workspace", &proto.JobRequest{RunId: "run-123"})
	if err != nil {
		t.Fatalf("workspaceVolumeSource() error = %v", err)
	}
	if source.PersistentVolumeClaim == nil || source.PersistentVolumeClaim.ClaimName != "shared-workspace" {
		t.Fatalf("workspaceVolumeSource() did not reference existing PVC: %#v", source)
	}
	if source.Ephemeral != nil {
		t.Fatalf("workspaceVolumeSource() should not create an ephemeral PVC for existing mode")
	}
}
