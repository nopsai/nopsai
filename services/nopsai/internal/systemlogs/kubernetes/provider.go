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
	Namespace     string
	LabelSelector string
	Container     string
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
		now:           time.Now,
	}
}

func NewClientProvider(client clientkubernetes.Interface, registry *systemlogs.Registry, options Options) *Provider {
	return NewProvider(&clientAPI{client: client}, registry, options)
}

func (p *Provider) ListSources(ctx context.Context) ([]systemlogs.SourceStatus, error) {
	pods, err := p.podsBySource(ctx)
	if err != nil {
		return nil, err
	}
	sources := p.registry.Sources()
	out := make([]systemlogs.SourceStatus, 0, len(sources))
	for _, source := range sources {
		status := systemlogs.SourceStatus{
			ID: source.ID, DisplayName: source.DisplayName, ContainerName: source.ContainerName,
			State: "unavailable",
		}
		if pod, ok := pods[source.ID]; ok {
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
	if !ok {
		return systemlogs.Source{}, PodSummary{}, systemlogs.ErrSourceNotFound
	}
	pods, err := p.podsBySource(ctx)
	if err != nil {
		return systemlogs.Source{}, PodSummary{}, err
	}
	pod, ok := pods[source.ID]
	if !ok {
		return systemlogs.Source{}, PodSummary{}, systemlogs.ErrSourceNotFound
	}
	return source, pod, nil
}

func (p *Provider) podsBySource(ctx context.Context) (map[string]PodSummary, error) {
	if p == nil || p.api == nil {
		return nil, fmt.Errorf("kubernetes system log API is not configured")
	}
	pods, err := p.api.ListPods(ctx, p.namespace, p.labelSelector)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(pods, func(i, j int) bool {
		if !pods[i].CreationTimestamp.Equal(pods[j].CreationTimestamp) {
			return pods[i].CreationTimestamp.After(pods[j].CreationTimestamp)
		}
		return pods[i].Name < pods[j].Name
	})
	sources := p.registry.Sources()
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
	return out, nil
}

func (p *Provider) containerForSource(source systemlogs.Source) string {
	if p.container != "" {
		return p.container
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
