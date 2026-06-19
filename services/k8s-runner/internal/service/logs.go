package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"nopsai/pkg/logforward"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type podLogStreamFunc func(context.Context, string, *corev1.PodLogOptions) (io.ReadCloser, error)

type podLogCursor struct {
	lastTimestamp            time.Time
	linesAtLastTimestamp     map[string]struct{}
	sawTimestampedKubernetes bool
}

func (r *kubernetesRunner) streamPodLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, podName string) {
	if dispatcher == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	cursor := &podLogCursor{}
	for {
		if ctx.Err() != nil {
			return
		}

		reader, err := r.openPodLogStream(ctx, podName, cursor.options(true))
		if err != nil {
			if r.handlePodLogAttachError(ctx, dispatcher, runID, podName, cursor, err) {
				return
			}
			if !sleepWithContext(ctx, r.effectivePodLogRetryDelay()) {
				return
			}
			continue
		}

		r.forwardPodLogReader(ctx, dispatcher, runID, podName, reader, cursor)
		terminal, terminalErr := r.podTerminal(ctx, podName)
		if terminalErr != nil {
			log.Warn().Err(terminalErr).Str("run_id", runID).Str("pod", podName).Msg("failed to inspect pod after log stream ended")
		}
		if terminal {
			r.fetchFinalPodLogs(ctx, dispatcher, runID, podName, cursor)
			return
		}

		log.Warn().
			Str("run_id", runID).
			Str("pod", podName).
			Msg("pod log stream ended before pod completed; reattaching")
		if !sleepWithContext(ctx, r.effectivePodLogRetryDelay()) {
			return
		}
	}
}

func (r *kubernetesRunner) handlePodLogAttachError(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, podName string, cursor *podLogCursor, attachErr error) bool {
	terminal, terminalErr := r.podTerminal(ctx, podName)
	if terminalErr != nil {
		log.Warn().Err(terminalErr).Str("run_id", runID).Str("pod", podName).Msg("failed to inspect pod after log attach error")
	}
	if !terminal {
		log.Debug().Err(attachErr).Str("run_id", runID).Str("pod", podName).Msg("pod logs are not ready yet")
		return false
	}

	if r.fetchFinalPodLogs(ctx, dispatcher, runID, podName, cursor) {
		return true
	}
	log.Error().Err(attachErr).Str("run_id", runID).Str("pod", podName).Msg("failed to attach to pod logs")
	r.emitRunLog(context.Background(), dispatcher, runID, fmt.Sprintf("Kubernetes runner could not attach to logs for agent pod %s: %v", podName, attachErr))
	return true
}

func (r *kubernetesRunner) fetchFinalPodLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, podName string, cursor *podLogCursor) bool {
	reader, err := r.openPodLogStream(ctx, podName, cursor.options(false))
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("pod", podName).Msg("failed to fetch final pod logs")
		return false
	}
	r.forwardPodLogReader(ctx, dispatcher, runID, podName, reader, cursor)
	return true
}

func (r *kubernetesRunner) forwardPodLogReader(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, podName string, reader io.ReadCloser, cursor *podLogCursor) {
	defer reader.Close()
	logforward.Forward(ctx, reader, func(sendCtx context.Context, lines []string) {
		r.flushLogs(sendCtx, dispatcher, runID, lines)
	}, logforward.Options{
		FilterLine: cursor.filter,
		OnScannerError: func(err error) {
			log.Error().Err(err).Str("run_id", runID).Str("pod", podName).Msg("pod log scanner error")
		},
	})
}

func (r *kubernetesRunner) openPodLogStream(ctx context.Context, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	if r.podLogStream != nil {
		return r.podLogStream(ctx, podName, opts)
	}
	return r.client.CoreV1().Pods(r.namespace).GetLogs(podName, opts).Stream(ctx)
}

func (r *kubernetesRunner) podTerminal(ctx context.Context, podName string) (bool, error) {
	pod, err := r.client.CoreV1().Pods(r.namespace).Get(ctx, podName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	switch pod.Status.Phase {
	case corev1.PodSucceeded, corev1.PodFailed:
		return true, nil
	default:
		return false, nil
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

func (r *kubernetesRunner) emitRunLog(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, line string) {
	if dispatcher == nil || strings.TrimSpace(runID) == "" || strings.TrimSpace(line) == "" {
		return
	}
	r.flushLogs(ctx, dispatcher, runID, []string{line})
}

func (c *podLogCursor) options(follow bool) *corev1.PodLogOptions {
	opts := &corev1.PodLogOptions{
		Container:  kubernetesAgentContainerName,
		Follow:     follow,
		Timestamps: true,
	}
	if since := c.sinceTime(); since != nil {
		opts.SinceTime = since
	}
	return opts
}

func (c *podLogCursor) filter(line string) (string, bool) {
	timestamp, ok := parseKubernetesPodLogTimestamp(line)
	if !ok {
		return line, true
	}
	c.sawTimestampedKubernetes = true
	if c.lastTimestamp.IsZero() || timestamp.After(c.lastTimestamp) {
		c.lastTimestamp = timestamp
		c.linesAtLastTimestamp = map[string]struct{}{line: {}}
		return line, true
	}
	if timestamp.Before(c.lastTimestamp) {
		return "", false
	}
	if _, exists := c.linesAtLastTimestamp[line]; exists {
		return "", false
	}
	c.linesAtLastTimestamp[line] = struct{}{}
	return line, true
}

func (c *podLogCursor) sinceTime() *metav1.Time {
	if c == nil || c.lastTimestamp.IsZero() || !c.sawTimestampedKubernetes {
		return nil
	}
	// Ask for a tiny overlap because Kubernetes log filtering is documented as
	// "after" the timestamp. The cursor drops duplicates from the overlap.
	since := metav1.NewTime(c.lastTimestamp.Add(-time.Nanosecond))
	return &since
}

func parseKubernetesPodLogTimestamp(line string) (time.Time, bool) {
	timestampField, _, ok := strings.Cut(line, " ")
	if !ok {
		return time.Time{}, false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, timestampField)
	if err != nil {
		return time.Time{}, false
	}
	return timestamp, true
}

func (r *kubernetesRunner) effectivePodLogRetryDelay() time.Duration {
	if r.podLogRetryDelay > 0 {
		return r.podLogRetryDelay
	}
	return kubernetesPodLogRetryDelay
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
