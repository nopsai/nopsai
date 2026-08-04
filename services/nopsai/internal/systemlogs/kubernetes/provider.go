package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"nopsai/services/nopsai/internal/systemlogs"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientkubernetes "k8s.io/client-go/kubernetes"
)

const maxKubernetesLogLineBytes = 32 * 1024 * 1024

type Options struct {
	Namespace            string
	LabelSelector        string
	Container            string
	PlatformID           string
	RunnerSourceResolver RunnerSourceResolver
}

type PodSummary struct {
	Namespace         string
	Name              string
	UID               string
	Labels            map[string]string
	Phase             string
	Reason            string
	Ready             bool
	CreationTimestamp time.Time
}

type RunnerSourceHint struct {
	RunnerID      string
	SourceID      string
	PlatformID    string
	Namespace     string
	LabelSelector string
	ContainerName string
}

type RunnerSourceResolver interface {
	RunnerSourceHints(context.Context) ([]RunnerSourceHint, error)
}

type API interface {
	ListPods(ctx context.Context, namespace, labelSelector string) ([]PodSummary, error)
	PodLogs(ctx context.Context, namespace, podName string, options corev1.PodLogOptions) (io.ReadCloser, error)
}

type Provider struct {
	api           API
	registry      *systemlogs.Registry
	namespace     string
	labelSelector string
	container     string
	platformID    string
	runnerSources RunnerSourceResolver
	now           func() time.Time
}

func NewProvider(api API, registry *systemlogs.Registry, options Options) *Provider {
	if registry == nil {
		registry = systemlogs.DefaultRegistry()
	}
	labelSelector := strings.TrimSpace(options.LabelSelector)
	if labelSelector == "" {
		labelSelector = "app.kubernetes.io/name=nopsai"
	}
	return &Provider{
		api:           api,
		registry:      registry,
		namespace:     strings.TrimSpace(options.Namespace),
		labelSelector: labelSelector,
		container:     strings.TrimSpace(options.Container),
		platformID:    strings.TrimSpace(options.PlatformID),
		runnerSources: options.RunnerSourceResolver,
		now:           time.Now,
	}
}

func NewClientProvider(client clientkubernetes.Interface, registry *systemlogs.Registry, options Options) *Provider {
	return NewProvider(&clientAPI{client: client}, registry, options)
}

func (p *Provider) ListSources(ctx context.Context) ([]systemlogs.SourceStatus, error) {
	pods, err := p.pods(ctx)
	if err != nil {
		return nil, err
	}
	sources := p.registry.Sources()
	out := make([]systemlogs.SourceStatus, 0, len(sources))
	podsBySource := podsByStaticSource(pods, sources)
	for _, source := range sources {
		status := systemlogs.SourceStatus{
			ID: source.ID, DisplayName: source.DisplayName, ContainerName: source.ContainerName,
			State: "unavailable",
		}
		if pod, ok := podsBySource[source.ID]; ok {
			status.ContainerInstance = podInstance(pod)
			status.Available = true
			status.State = strings.ToLower(strings.TrimSpace(pod.Phase))
			if status.State == "" {
				status.State = "unknown"
			}
			if pod.Ready {
				status.Health = "ready"
			} else {
				status.Health = "not-ready"
			}
			status.Status = strings.TrimSpace(pod.Reason)
		}
		out = append(out, status)
	}
	runnerStatuses := p.runnerSourceStatuses(pods)
	seenRunnerSources := sourceStatusIDSet(runnerStatuses)
	if hintedRunnerStatuses, err := p.runnerSourceStatusesFromHints(ctx, seenRunnerSources); err == nil {
		runnerStatuses = append(runnerStatuses, hintedRunnerStatuses...)
	}
	out = append(out, runnerStatuses...)
	return out, nil
}

func (p *Provider) Tail(ctx context.Context, sourceID string, lines int) ([]systemlogs.Entry, error) {
	source, pod, err := p.resolve(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if lines <= 0 {
		lines = 500
	}
	tailLines := int64(lines)
	reader, err := p.api.PodLogs(ctx, pod.Namespace, pod.Name, corev1.PodLogOptions{
		Container:  p.containerForSource(source),
		Timestamps: true,
		TailLines:  &tailLines,
	})
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := make([]systemlogs.Entry, 0, lines)
	err = scanKubernetesLogs(reader, func(line string) {
		entries = append(entries, p.entry(source, pod, line))
	})
	return entries, err
}

func (p *Provider) Follow(ctx context.Context, sourceID string, after systemlogs.Cursor, emit func(systemlogs.Entry)) error {
	source, pod, err := p.resolve(ctx, sourceID)
	if err != nil {
		return err
	}
	tailLines := int64(0)
	options := corev1.PodLogOptions{
		Container:  p.containerForSource(source),
		Follow:     true,
		Timestamps: true,
		TailLines:  &tailLines,
	}
	if !after.EmittedAt.IsZero() {
		since := metav1.NewTime(after.EmittedAt.UTC().Add(-time.Nanosecond))
		options.SinceTime = &since
		options.TailLines = nil
	}
	reader, err := p.api.PodLogs(ctx, pod.Namespace, pod.Name, options)
	if err != nil {
		return err
	}
	defer reader.Close()
	return scanKubernetesLogs(reader, func(line string) {
		emit(p.entry(source, pod, line))
	})
}

func (p *Provider) resolve(ctx context.Context, sourceID string) (systemlogs.Source, PodSummary, error) {
	source, ok := p.registry.Resolve(sourceID)
	pods, err := p.pods(ctx)
	if err != nil {
		return systemlogs.Source{}, PodSummary{}, err
	}
	if ok {
		pod, ok := podsByStaticSource(pods, []systemlogs.Source{source})[source.ID]
		if !ok {
			return systemlogs.Source{}, PodSummary{}, systemlogs.ErrSourceNotFound
		}
		return source, pod, nil
	}
	runnerID, ok := systemlogs.ParseRunnerSourceID(sourceID)
	if !ok {
		return systemlogs.Source{}, PodSummary{}, systemlogs.ErrSourceNotFound
	}
	for _, pod := range pods {
		if runnerIDFromLabels(pod.Labels) != runnerID {
			continue
		}
		source, ok := p.runnerSourceFromPod(pod)
		if !ok {
			continue
		}
		return source, pod, nil
	}
	if source, pod, ok, err := p.resolveRunnerHint(ctx, sourceID, runnerID); err != nil {
		return systemlogs.Source{}, PodSummary{}, err
	} else if ok {
		return source, pod, nil
	}
	return systemlogs.Source{}, PodSummary{}, systemlogs.ErrSourceNotFound
}

func (p *Provider) pods(ctx context.Context) ([]PodSummary, error) {
	if p == nil || p.api == nil {
		return nil, fmt.Errorf("kubernetes system log API is not configured")
	}
	pods, err := p.api.ListPods(ctx, p.namespace, p.labelSelector)
	if err != nil {
		return nil, err
	}
	if p.labelSelector != "app.kubernetes.io/name=nopsai-k8s-runner" {
		runnerPods, err := p.api.ListPods(ctx, p.namespace, "app.kubernetes.io/name=nopsai-k8s-runner")
		if err != nil {
			return nil, err
		}
		pods = appendPodsUnique(pods, runnerPods)
	}
	sort.SliceStable(pods, func(i, j int) bool {
		if !pods[i].CreationTimestamp.Equal(pods[j].CreationTimestamp) {
			return pods[i].CreationTimestamp.After(pods[j].CreationTimestamp)
		}
		return pods[i].Name < pods[j].Name
	})
	return pods, nil
}

func podsByStaticSource(pods []PodSummary, sources []systemlogs.Source) map[string]PodSummary {
	out := make(map[string]PodSummary, len(sources))
	for _, pod := range pods {
		component := strings.TrimSpace(pod.Labels["app.kubernetes.io/component"])
		if component == "" {
			continue
		}
		for _, source := range sources {
			if _, exists := out[source.ID]; exists {
				continue
			}
			if component == kubernetesComponentForSource(source.ID) {
				out[source.ID] = pod
			}
		}
	}
	return out
}

func appendPodsUnique(base, extra []PodSummary) []PodSummary {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, pod := range base {
		seen[podInstance(pod)] = struct{}{}
	}
	for _, pod := range extra {
		key := podInstance(pod)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		base = append(base, pod)
	}
	return base
}

func (p *Provider) runnerSourceStatuses(pods []PodSummary) []systemlogs.SourceStatus {
	seen := map[string]struct{}{}
	out := make([]systemlogs.SourceStatus, 0)
	for _, pod := range pods {
		source, ok := p.runnerSourceFromPod(pod)
		if !ok {
			continue
		}
		if _, exists := seen[source.ID]; exists {
			continue
		}
		seen[source.ID] = struct{}{}
		out = append(out, runnerSourceStatusForPod(source, pod))
	}
	return out
}

func (p *Provider) runnerSourceStatusesFromHints(ctx context.Context, seen map[string]struct{}) ([]systemlogs.SourceStatus, error) {
	hints, err := p.runnerSourceHints(ctx)
	if err != nil || len(hints) == 0 {
		return nil, err
	}
	out := make([]systemlogs.SourceStatus, 0, len(hints))
	for _, raw := range hints {
		hint, source, ok := normalizeRunnerSourceHint(raw)
		if !ok {
			continue
		}
		if _, exists := seen[source.ID]; exists {
			continue
		}
		seen[source.ID] = struct{}{}
		pod, found, err := p.podForRunnerHint(ctx, hint)
		if err != nil {
			status := runnerSourceStatusUnavailable(source, err.Error())
			out = append(out, status)
			continue
		}
		if !found {
			status := runnerSourceStatusUnavailable(source, "runner pod not found")
			out = append(out, status)
			continue
		}
		out = append(out, runnerSourceStatusForPod(source, pod))
	}
	return out, nil
}

func (p *Provider) resolveRunnerHint(ctx context.Context, sourceID, runnerID string) (systemlogs.Source, PodSummary, bool, error) {
	hints, err := p.runnerSourceHints(ctx)
	if err != nil || len(hints) == 0 {
		return systemlogs.Source{}, PodSummary{}, false, err
	}
	for _, raw := range hints {
		hint, source, ok := normalizeRunnerSourceHint(raw)
		if !ok {
			continue
		}
		if source.ID != sourceID && hint.RunnerID != runnerID {
			continue
		}
		pod, found, err := p.podForRunnerHint(ctx, hint)
		if err != nil {
			return systemlogs.Source{}, PodSummary{}, false, err
		}
		if !found {
			continue
		}
		return source, pod, true, nil
	}
	return systemlogs.Source{}, PodSummary{}, false, nil
}

func (p *Provider) podForRunnerHint(ctx context.Context, hint RunnerSourceHint) (PodSummary, bool, error) {
	if p == nil || p.api == nil {
		return PodSummary{}, false, fmt.Errorf("kubernetes system log API is not configured")
	}
	if hint.PlatformID == "" {
		hint.PlatformID = p.platformID
	}
	pods, err := p.api.ListPods(ctx, hint.Namespace, hint.LabelSelector)
	if err != nil {
		return PodSummary{}, false, err
	}
	sort.SliceStable(pods, func(i, j int) bool {
		if !pods[i].CreationTimestamp.Equal(pods[j].CreationTimestamp) {
			return pods[i].CreationTimestamp.After(pods[j].CreationTimestamp)
		}
		return pods[i].Name < pods[j].Name
	})
	for _, pod := range pods {
		if podMatchesRunnerHint(pod, hint) {
			return pod, true, nil
		}
	}
	return PodSummary{}, false, nil
}

func (p *Provider) runnerSourceHints(ctx context.Context) ([]RunnerSourceHint, error) {
	if p == nil || p.runnerSources == nil {
		return nil, nil
	}
	return p.runnerSources.RunnerSourceHints(ctx)
}

func normalizeRunnerSourceHint(raw RunnerSourceHint) (RunnerSourceHint, systemlogs.Source, bool) {
	hint := RunnerSourceHint{
		RunnerID:      strings.TrimSpace(raw.RunnerID),
		SourceID:      strings.TrimSpace(raw.SourceID),
		PlatformID:    strings.TrimSpace(raw.PlatformID),
		Namespace:     strings.TrimSpace(raw.Namespace),
		LabelSelector: strings.TrimSpace(raw.LabelSelector),
		ContainerName: strings.TrimSpace(raw.ContainerName),
	}
	if hint.ContainerName == "" {
		hint.ContainerName = "runner"
	}
	if hint.SourceID == "" {
		hint.SourceID = systemlogs.RunnerSourceID(hint.RunnerID)
	}
	sourceRunnerID, ok := systemlogs.ParseRunnerSourceID(hint.SourceID)
	if !ok {
		return RunnerSourceHint{}, systemlogs.Source{}, false
	}
	if hint.RunnerID == "" {
		hint.RunnerID = sourceRunnerID
	}
	if hint.RunnerID != sourceRunnerID {
		return RunnerSourceHint{}, systemlogs.Source{}, false
	}
	if hint.RunnerID == "" || hint.Namespace == "" || hint.LabelSelector == "" {
		return RunnerSourceHint{}, systemlogs.Source{}, false
	}
	source, ok := systemlogs.NewRunnerSource(hint.RunnerID, hint.ContainerName)
	if !ok {
		return RunnerSourceHint{}, systemlogs.Source{}, false
	}
	return hint, source, true
}

func runnerSourceStatusForPod(source systemlogs.Source, pod PodSummary) systemlogs.SourceStatus {
	status := systemlogs.SourceStatus{
		ID:                source.ID,
		DisplayName:       source.DisplayName,
		ContainerName:     source.ContainerName,
		ContainerInstance: podInstance(pod),
		Available:         true,
		State:             strings.ToLower(strings.TrimSpace(pod.Phase)),
		Status:            strings.TrimSpace(pod.Reason),
	}
	if status.State == "" {
		status.State = "unknown"
	}
	if pod.Ready {
		status.Health = "ready"
	} else {
		status.Health = "not-ready"
	}
	return status
}

func runnerSourceStatusUnavailable(source systemlogs.Source, reason string) systemlogs.SourceStatus {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "runner pod unavailable"
	}
	return systemlogs.SourceStatus{
		ID:            source.ID,
		DisplayName:   source.DisplayName,
		ContainerName: source.ContainerName,
		Available:     false,
		State:         "unavailable",
		Status:        reason,
	}
}

func sourceStatusIDSet(statuses []systemlogs.SourceStatus) map[string]struct{} {
	out := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		if status.ID == "" {
			continue
		}
		out[status.ID] = struct{}{}
	}
	return out
}

func (p *Provider) runnerSourceFromPod(pod PodSummary) (systemlogs.Source, bool) {
	runnerID := runnerIDFromLabels(pod.Labels)
	if runnerID == "" || !isNopsaiKubernetesRunner(pod.Labels) || !podMatchesPlatform(pod.Labels, p.platformID) {
		return systemlogs.Source{}, false
	}
	return systemlogs.NewRunnerSource(runnerID, "runner")
}

func runnerIDFromLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	return strings.TrimSpace(labels["nopsai.io/runner-id"])
}

func runnerPlatformIDFromLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	return strings.TrimSpace(labels["nopsai.io/platform-id"])
}

func podMatchesPlatform(labels map[string]string, platformID string) bool {
	platformID = strings.TrimSpace(platformID)
	if platformID == "" {
		return true
	}
	return runnerPlatformIDFromLabels(labels) == platformID
}

func podMatchesRunnerHint(pod PodSummary, hint RunnerSourceHint) bool {
	if runnerIDFromLabels(pod.Labels) != hint.RunnerID {
		return false
	}
	return podMatchesPlatform(pod.Labels, hint.PlatformID)
}

func isNopsaiKubernetesRunner(labels map[string]string) bool {
	if len(labels) == 0 {
		return false
	}
	name := strings.TrimSpace(labels["app.kubernetes.io/name"])
	component := strings.TrimSpace(labels["app.kubernetes.io/component"])
	return (name == "nopsai-k8s-runner" && component == "runner") || (name == "nopsai" && component == "k8s-runner")
}

func (p *Provider) containerForSource(source systemlogs.Source) string {
	if p.container != "" {
		return p.container
	}
	if _, ok := systemlogs.ParseRunnerSourceID(source.ID); ok {
		return "runner"
	}
	switch source.ID {
	case "nopsai":
		return "api"
	case "k8s-runner", "docker-runner":
		return "runner"
	default:
		return source.ID
	}
}

func (p *Provider) entry(source systemlogs.Source, pod PodSummary, raw string) systemlogs.Entry {
	emittedAt, line := splitTimestamp(raw, p.now().UTC())
	return systemlogs.Entry{
		SourceID:          source.ID,
		ContainerName:     source.ContainerName,
		ContainerInstance: podInstance(pod),
		EmittedAt:         emittedAt,
		Stream:            systemlogs.StreamStdout,
		Line:              line,
	}
}

func kubernetesComponentForSource(sourceID string) string {
	switch sourceID {
	case "nopsai":
		return "api"
	default:
		return sourceID
	}
}

func podInstance(pod PodSummary) string {
	if pod.Namespace == "" {
		return pod.Name
	}
	return pod.Namespace + "/" + pod.Name
}

func splitTimestamp(raw string, fallback time.Time) (time.Time, string) {
	timestamp, line, found := strings.Cut(strings.TrimRight(raw, "\r\n"), " ")
	if !found {
		return fallback, timestamp
	}
	emittedAt, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return fallback, strings.TrimRight(raw, "\r\n")
	}
	return emittedAt.UTC(), line
}

func scanKubernetesLogs(reader io.Reader, emit func(string)) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxKubernetesLogLineBytes)
	for scanner.Scan() {
		emit(scanner.Text())
	}
	return scanner.Err()
}

type clientAPI struct {
	client clientkubernetes.Interface
}

func (c *clientAPI) ListPods(ctx context.Context, namespace, labelSelector string) ([]PodSummary, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("kubernetes client is not configured")
	}
	items, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	out := make([]PodSummary, 0, len(items.Items))
	for _, pod := range items.Items {
		out = append(out, PodSummary{
			Namespace:         pod.Namespace,
			Name:              pod.Name,
			UID:               string(pod.UID),
			Labels:            copyStringMap(pod.Labels),
			Phase:             string(pod.Status.Phase),
			Reason:            pod.Status.Reason,
			Ready:             podReady(pod),
			CreationTimestamp: pod.CreationTimestamp.Time,
		})
	}
	return out, nil
}

func (c *clientAPI) PodLogs(ctx context.Context, namespace, podName string, options corev1.PodLogOptions) (io.ReadCloser, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("kubernetes client is not configured")
	}
	return c.client.CoreV1().Pods(namespace).GetLogs(podName, &options).Stream(ctx)
}

func podReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
