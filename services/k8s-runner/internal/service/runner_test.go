package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"nopsai/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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

func TestCreateAgentPodUsesRunnerServiceAccountAndPullSecrets(t *testing.T) {
	automount := false
	client := fake.NewSimpleClientset()
	r := &kubernetesRunner{
		id:               "k8s-runner-1",
		client:           client,
		namespace:        "runs",
		serviceAccount:   "nopsai-runner",
		workloadSA:       "nopsai-runner-workload",
		workloadSAToken:  &automount,
		imagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}, {Name: "agent-regcred"}},
		imagePullPolicy:  corev1.PullIfNotPresent,
		workspaceMode:    kubernetesWorkspaceExistingPVC,
	}

	pod, err := r.createAgentPod(context.Background(), "agent-run-123", "agent:private", "workspace-pvc", nil, &proto.JobRequest{
		RunId:        "run-123",
		PipelineName: "deploy",
	})
	if err != nil {
		t.Fatalf("createAgentPod() error = %v", err)
	}
	if pod.Spec.ServiceAccountName != "nopsai-runner" {
		t.Fatalf("service account = %q, want runner account", pod.Spec.ServiceAccountName)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || !*pod.Spec.AutomountServiceAccountToken {
		t.Fatalf("automount token = %#v, want true", pod.Spec.AutomountServiceAccountToken)
	}
	if len(pod.Spec.ImagePullSecrets) != 2 || pod.Spec.ImagePullSecrets[0].Name != "regcred" || pod.Spec.ImagePullSecrets[1].Name != "agent-regcred" {
		t.Fatalf("image pull secrets = %#v", pod.Spec.ImagePullSecrets)
	}
	assertRunnerPodSecurityDefaults(t, pod)
	assertRunnerContainerSecurityDefaults(t, pod.Spec.Containers[0].SecurityContext)
}

func TestAgentRuntimeVarsCarryWorkloadKubernetesSettings(t *testing.T) {
	automount := false
	r := &kubernetesRunner{
		id:               "k8s-runner-1",
		namespace:        "runs",
		serviceAccount:   "nopsai-runner",
		workloadSA:       "nopsai-runner-workload",
		workloadSAToken:  &automount,
		imagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}, {Name: "agent-regcred"}},
		imagePullPolicy:  corev1.PullIfNotPresent,
		workspaceSize:    "5Gi",
		workspaceAccess:  corev1.ReadWriteOnce,
		workspaceMode:    kubernetesWorkspaceVolumePVC,
		taskTimeout:      time.Minute,
		cleanupPods:      true,
	}

	vars := runtimeVarMap(r.agentRuntimeVars(&proto.JobRequest{}, "workspace-pvc"))
	if vars["KUBERNETES_WORKLOAD_SERVICE_ACCOUNT"] != "nopsai-runner-workload" {
		t.Fatalf("workload service account env = %q", vars["KUBERNETES_WORKLOAD_SERVICE_ACCOUNT"])
	}
	if vars["KUBERNETES_WORKLOAD_AUTOMOUNT_SERVICE_ACCOUNT_TOKEN"] != "false" {
		t.Fatalf("automount env = %q", vars["KUBERNETES_WORKLOAD_AUTOMOUNT_SERVICE_ACCOUNT_TOKEN"])
	}
	if vars["KUBERNETES_IMAGE_PULL_SECRETS"] != "regcred,agent-regcred" {
		t.Fatalf("pull secrets env = %q", vars["KUBERNETES_IMAGE_PULL_SECRETS"])
	}
}

func TestEmitRunLogSendsDiagnosticToDispatcher(t *testing.T) {
	dispatcher := &recordingDispatcherClient{}
	r := &kubernetesRunner{}

	r.emitRunLog(context.Background(), dispatcher, "run-123", "Kubernetes runner diagnostic")

	if len(dispatcher.batches) != 1 {
		t.Fatalf("log batches = %d, want 1", len(dispatcher.batches))
	}
	got := dispatcher.batches[0]
	if got.RunId != "run-123" {
		t.Fatalf("run id = %q, want run-123", got.RunId)
	}
	if len(got.Lines) != 1 || got.Lines[0] != "Kubernetes runner diagnostic" {
		t.Fatalf("lines = %#v, want diagnostic line", got.Lines)
	}
}

func runtimeVarMap(entries []string) map[string]string {
	out := map[string]string{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			out[key] = value
		}
	}
	return out
}

func assertRunnerPodSecurityDefaults(t *testing.T, pod *corev1.Pod) {
	t.Helper()
	if pod.Spec.SecurityContext == nil || pod.Spec.SecurityContext.SeccompProfile == nil {
		t.Fatalf("pod security context missing: %#v", pod.Spec.SecurityContext)
	}
	if pod.Spec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Fatalf("pod seccomp = %q, want RuntimeDefault", pod.Spec.SecurityContext.SeccompProfile.Type)
	}
}

func assertRunnerContainerSecurityDefaults(t *testing.T, security *corev1.SecurityContext) {
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
	if security.Capabilities == nil || len(security.Capabilities.Drop) != 1 || security.Capabilities.Drop[0] != "ALL" {
		t.Fatalf("capabilities = %#v, want drop ALL", security.Capabilities)
	}
}

func TestStreamPodLogsReattachesWhenFollowStreamEndsBeforePodCompletes(t *testing.T) {
	dispatcher := &recordingDispatcherClient{}
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-run-123", Namespace: "runs"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	const firstLine = "2026-06-19T10:00:00Z first agent line"
	const secondLine = "2026-06-19T10:00:01Z second agent line"
	var calls []corev1.PodLogOptions

	r := &kubernetesRunner{
		client:           client,
		namespace:        "runs",
		podLogRetryDelay: time.Millisecond,
		podLogStream: func(_ context.Context, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
			if podName != "agent-run-123" {
				t.Fatalf("pod name = %q, want agent-run-123", podName)
			}
			calls = append(calls, *opts)
			switch len(calls) {
			case 1:
				if !opts.Follow || opts.SinceTime != nil {
					t.Fatalf("first log request = %#v, want initial follow without since_time", opts)
				}
				return io.NopCloser(strings.NewReader(firstLine + "\n")), nil
			case 2:
				if !opts.Follow || opts.SinceTime == nil {
					t.Fatalf("second log request = %#v, want reattached follow with since_time", opts)
				}
				setTestPodPhase(t, client, "runs", "agent-run-123", corev1.PodSucceeded)
				return io.NopCloser(strings.NewReader(secondLine + "\n")), nil
			case 3:
				if opts.Follow || opts.SinceTime == nil {
					t.Fatalf("third log request = %#v, want final non-follow with since_time", opts)
				}
				return io.NopCloser(strings.NewReader(secondLine + "\n")), nil
			default:
				t.Fatalf("unexpected log stream call %d", len(calls))
				return nil, errors.New("unexpected call")
			}
		},
	}

	r.streamPodLogs(context.Background(), dispatcher, "run-123", "agent-run-123")

	got := flattenLogBatches(dispatcher.batches)
	want := []string{firstLine, secondLine}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("forwarded logs = %#v, want %#v", got, want)
	}
	if len(calls) != 3 {
		t.Fatalf("log stream calls = %d, want follow, reattach, final fetch", len(calls))
	}
}

func TestStreamPodLogsFetchesFinalLogsWhenFollowAttachFailsAfterPodCompletes(t *testing.T) {
	dispatcher := &recordingDispatcherClient{}
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-run-456", Namespace: "runs"},
		Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
	})
	const finalLine = "2026-06-19T10:00:02Z final agent line"
	var calls []corev1.PodLogOptions

	r := &kubernetesRunner{
		client:           client,
		namespace:        "runs",
		podLogRetryDelay: time.Millisecond,
		podLogStream: func(_ context.Context, _ string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
			calls = append(calls, *opts)
			switch len(calls) {
			case 1:
				if !opts.Follow {
					t.Fatalf("first log request = %#v, want follow", opts)
				}
				return nil, errors.New("container is terminated")
			case 2:
				if opts.Follow {
					t.Fatalf("second log request = %#v, want final non-follow fetch", opts)
				}
				return io.NopCloser(strings.NewReader(finalLine + "\n")), nil
			default:
				t.Fatalf("unexpected log stream call %d", len(calls))
				return nil, errors.New("unexpected call")
			}
		},
	}

	r.streamPodLogs(context.Background(), dispatcher, "run-456", "agent-run-456")

	got := flattenLogBatches(dispatcher.batches)
	if len(got) != 1 || got[0] != finalLine {
		t.Fatalf("forwarded logs = %#v, want final line", got)
	}
}

type recordingDispatcherClient struct {
	proto.DispatcherServiceClient
	batches []*proto.LogBatch
}

func (c *recordingDispatcherClient) IngestLogs(_ context.Context, batch *proto.LogBatch, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	c.batches = append(c.batches, batch)
	return &emptypb.Empty{}, nil
}

func setTestPodPhase(t *testing.T, client *fake.Clientset, namespace, podName string, phase corev1.PodPhase) {
	t.Helper()
	pod, err := client.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	pod.Status.Phase = phase
	if _, err := client.CoreV1().Pods(namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("update pod status: %v", err)
	}
}

func flattenLogBatches(batches []*proto.LogBatch) []string {
	var out []string
	for _, batch := range batches {
		out = append(out, batch.Lines...)
	}
	return out
}
