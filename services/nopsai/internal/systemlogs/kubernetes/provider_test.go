package kubernetes

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"nopsai/services/nopsai/internal/systemlogs"

	corev1 "k8s.io/api/core/v1"
)

type fakeAPI struct {
	pods      []PodSummary
	logs      string
	listCalls []listCall
	logCalls  []logCall
	logErr    error
}

type listCall struct {
	namespace     string
	labelSelector string
}

type logCall struct {
	namespace string
	podName   string
	options   corev1.PodLogOptions
}

func (f *fakeAPI) ListPods(_ context.Context, namespace, labelSelector string) ([]PodSummary, error) {
	f.listCalls = append(f.listCalls, listCall{namespace: namespace, labelSelector: labelSelector})
	return append([]PodSummary(nil), f.pods...), nil
}

func (f *fakeAPI) PodLogs(_ context.Context, namespace, podName string, options corev1.PodLogOptions) (io.ReadCloser, error) {
	f.logCalls = append(f.logCalls, logCall{namespace: namespace, podName: podName, options: options})
	if f.logErr != nil {
		return nil, f.logErr
	}
	return io.NopCloser(strings.NewReader(f.logs)), nil
}

func TestProviderListsSourcesFromKubernetesComponents(t *testing.T) {
	api := &fakeAPI{pods: []PodSummary{
		{
			Namespace:         "nopsai",
			Name:              "nopsai-api-old",
			Labels:            map[string]string{"app.kubernetes.io/component": "api"},
			Phase:             "Running",
			Ready:             true,
			CreationTimestamp: time.Unix(100, 0),
		},
		{
			Namespace:         "nopsai",
			Name:              "nopsai-api-new",
			Labels:            map[string]string{"app.kubernetes.io/component": "api"},
			Phase:             "Running",
			Ready:             true,
			CreationTimestamp: time.Unix(200, 0),
		},
		{
			Namespace:         "nopsai",
			Name:              "nopsai-dispatcher",
			Labels:            map[string]string{"app.kubernetes.io/component": "dispatcher"},
			Phase:             "Pending",
			Reason:            "ContainersNotReady",
			CreationTimestamp: time.Unix(150, 0),
		},
	}}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{
		Namespace:     "nopsai",
		LabelSelector: "app.kubernetes.io/name=nopsai,app.kubernetes.io/instance=pre",
	})

	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	byID := map[string]systemlogs.SourceStatus{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	if got := byID["nopsai"]; !got.Available || got.ContainerInstance != "nopsai/nopsai-api-new" || got.Health != "ready" {
		t.Fatalf("nopsai source = %#v", got)
	}
	if got := byID["dispatcher"]; !got.Available || got.Health != "not-ready" || got.Status != "ContainersNotReady" {
		t.Fatalf("dispatcher source = %#v", got)
	}
	if got := byID["git-bot"]; got.Available || got.State != "unavailable" {
		t.Fatalf("git-bot source = %#v", got)
	}
	if len(api.listCalls) != 1 || api.listCalls[0].namespace != "nopsai" || api.listCalls[0].labelSelector != "app.kubernetes.io/name=nopsai,app.kubernetes.io/instance=pre" {
		t.Fatalf("list calls = %#v", api.listCalls)
	}
}

func TestProviderTailUsesSourceContainerAndParsesTimestamps(t *testing.T) {
	emitted := "2026-06-23T10:15:30.123456789Z"
	api := &fakeAPI{
		pods: []PodSummary{{
			Namespace: "nopsai",
			Name:      "nopsai-api",
			Labels:    map[string]string{"app.kubernetes.io/component": "api"},
			Phase:     "Running",
		}},
		logs: emitted + " hello from api\nuntimestamped line\n",
	}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{Namespace: "nopsai"})
	provider.now = func() time.Time { return time.Unix(999, 0).UTC() }

	entries, err := provider.Tail(context.Background(), "nopsai", 25)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Line != "hello from api" || entries[0].ContainerInstance != "nopsai/nopsai-api" {
		t.Fatalf("first entry = %#v", entries[0])
	}
	if entries[0].EmittedAt.Format(time.RFC3339Nano) != emitted {
		t.Fatalf("timestamp = %s, want %s", entries[0].EmittedAt.Format(time.RFC3339Nano), emitted)
	}
	if entries[1].Line != "untimestamped line" || !entries[1].EmittedAt.Equal(time.Unix(999, 0).UTC()) {
		t.Fatalf("fallback entry = %#v", entries[1])
	}
	if len(api.logCalls) != 1 || api.logCalls[0].options.Container != "api" || api.logCalls[0].options.TailLines == nil || *api.logCalls[0].options.TailLines != 25 {
		t.Fatalf("log call = %#v", api.logCalls)
	}
}

func TestProviderFollowStartsAtCursorTimestamp(t *testing.T) {
	after := time.Unix(100, 10).UTC()
	api := &fakeAPI{
		pods: []PodSummary{{
			Namespace: "nopsai",
			Name:      "dispatcher-0",
			Labels:    map[string]string{"app.kubernetes.io/component": "dispatcher"},
			Phase:     "Running",
		}},
		logs: after.Format(time.RFC3339Nano) + " next line\n",
	}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{Namespace: "nopsai"})
	var entries []systemlogs.Entry

	if err := provider.Follow(context.Background(), "dispatcher", systemlogs.Cursor{EmittedAt: after}, func(entry systemlogs.Entry) {
		entries = append(entries, entry)
	}); err != nil {
		t.Fatalf("Follow() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Line != "next line" {
		t.Fatalf("entries = %#v", entries)
	}
	if len(api.logCalls) != 1 {
		t.Fatalf("log calls = %#v", api.logCalls)
	}
	call := api.logCalls[0]
	if !call.options.Follow || call.options.Container != "dispatcher" || call.options.SinceTime == nil || call.options.TailLines != nil {
		t.Fatalf("follow log options = %#v", call.options)
	}
	if !call.options.SinceTime.Time.Equal(after.Add(-time.Nanosecond)) {
		t.Fatalf("since time = %s, want cursor overlap", call.options.SinceTime.Time.Format(time.RFC3339Nano))
	}
}
