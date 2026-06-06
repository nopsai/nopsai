package service

import (
	"context"
	"fmt"
	"time"

	"nopsai/pkg/proto"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rs/zerolog/log"
)

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

func (r *kubernetesRunner) deletePod(ctx context.Context, podName string) {
	grace := int64(1)
	err := r.client.CoreV1().Pods(r.namespace).Delete(ctx, podName, metav1.DeleteOptions{GracePeriodSeconds: &grace})
	if err != nil && !apierrors.IsNotFound(err) {
		log.Warn().Err(err).Str("pod", podName).Msg("failed to delete pod")
	}
}
