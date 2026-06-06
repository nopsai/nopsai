package service

import (
	"fmt"
	"strings"

	"nopsai/pkg/proto"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
