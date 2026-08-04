package kubernetes

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"nopsai/services/nopsai/internal/systemlogs"

	corev1 "k8s.io/api/core/v1"
)

type fakeAPI struct {
	pods       []PodSummary
	podsByCall map[listCall][]PodSummary
	logs       string
	listCalls  []listCall
	logCalls   []logCall
	logErr     error
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
	call := listCall{namespace: namespace, labelSelector: labelSelector}
	f.listCalls = append(f.listCalls, call)
	if f.podsByCall != nil {
		return append([]PodSummary(nil), f.podsByCall[call]...), nil
	}
	return append([]PodSummary(nil), f.pods...), nil
}

func (f *fakeAPI) PodLogs(_ context.Context, namespace, podName string, options corev1.PodLogOptions) (io.ReadCloser, error) {
	f.logCalls = append(f.logCalls, logCall{namespace: namespace, podName: podName, options: options})
	if f.logErr != nil {
		return nil, f.logErr
	}
	return io.NopCloser(strings.NewReader(f.logs)), nil
}

type fakeRunnerSourceResolver struct {
	hints []RunnerSourceHint
	err   error
}

func (f fakeRunnerSourceResolver) RunnerSourceHints(context.Context) ([]RunnerSourceHint, error) {
	return append([]RunnerSourceHint(nil), f.hints...), f.err
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
	if len(api.listCalls) != 2 || api.listCalls[0].namespace != "nopsai" || api.listCalls[0].labelSelector != "app.kubernetes.io/name=nopsai,app.kubernetes.io/instance=pre" || api.listCalls[1].labelSelector != "app.kubernetes.io/name=nopsai-k8s-runner" {
		t.Fatalf("list calls = %#v", api.listCalls)
	}
}

func TestProviderListsAndTailsLabeledRunnerPod(t *testing.T) {
	api := &fakeAPI{
		pods: []PodSummary{{
			Namespace: "nopsai-runs",
			Name:      "nopsai-k8s-runner-prod-1-7f4d",
			Labels: map[string]string{
				"app.kubernetes.io/name":      "nopsai-k8s-runner",
				"app.kubernetes.io/component": "runner",
				"nopsai.io/runner-id":         "k8s-runner-prod-1",
			},
			Phase: "Running",
			Ready: true,
		}},
		logs: "2026-06-23T10:15:30Z runner ready\n",
	}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{Namespace: "nopsai-runs"})

	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	found := false
	for _, source := range sources {
		if source.ID != "runner:k8s-runner-prod-1" {
			continue
		}
		found = source.Available && source.DisplayName == "Runner k8s-runner-prod-1" && source.ContainerName == "runner"
	}
	if !found {
		t.Fatalf("runner source not found in %#v", sources)
	}

	entries, err := provider.Tail(context.Background(), "runner:k8s-runner-prod-1", 10)
	if err != nil {
		t.Fatalf("Tail(runner) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Line != "runner ready" || entries[0].SourceID != "runner:k8s-runner-prod-1" {
		t.Fatalf("runner entries = %#v", entries)
	}
	if len(api.logCalls) != 1 || api.logCalls[0].options.Container != "runner" || api.logCalls[0].podName != "nopsai-k8s-runner-prod-1-7f4d" {
		t.Fatalf("log call = %#v", api.logCalls)
	}
}

func TestProviderListsAndTailsRegisteredRunnerHintOutsideDefaultNamespace(t *testing.T) {
	selector := "app.kubernetes.io/name=nopsai-k8s-runner,app.kubernetes.io/instance=nopsai-k8s-runner-prod,nopsai.io/runner-id=runner-prod"
	runnerPod := PodSummary{
		Namespace: "runner-ns",
		Name:      "nopsai-k8s-runner-prod-6f4d",
		Labels: map[string]string{
			"app.kubernetes.io/name":      "nopsai-k8s-runner",
			"app.kubernetes.io/component": "runner",
			"nopsai.io/runner-id":         "runner-prod",
		},
		Phase:             "Running",
		Ready:             true,
		CreationTimestamp: time.Unix(300, 0),
	}
	api := &fakeAPI{
		podsByCall: map[listCall][]PodSummary{
			{namespace: "nopsai", labelSelector: "app.kubernetes.io/name=nopsai"}:            {},
			{namespace: "nopsai", labelSelector: "app.kubernetes.io/name=nopsai-k8s-runner"}: {},
			{namespace: "runner-ns", labelSelector: selector}:                                {runnerPod},
		},
		logs: "2026-06-23T10:15:30Z registered runner ready\n",
	}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{
		Namespace:     "nopsai",
		LabelSelector: "app.kubernetes.io/name=nopsai",
		RunnerSourceResolver: fakeRunnerSourceResolver{hints: []RunnerSourceHint{{
			RunnerID:      "runner-prod",
			SourceID:      systemlogs.RunnerSourceID("runner-prod"),
			Namespace:     "runner-ns",
			LabelSelector: selector,
		}}},
	})

	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	found := false
	for _, source := range sources {
		if source.ID != systemlogs.RunnerSourceID("runner-prod") {
			continue
		}
		found = source.Available && source.ContainerInstance == "runner-ns/nopsai-k8s-runner-prod-6f4d"
	}
	if !found {
		t.Fatalf("hinted runner source not found in %#v", sources)
	}

	entries, err := provider.Tail(context.Background(), systemlogs.RunnerSourceID("runner-prod"), 10)
	if err != nil {
		t.Fatalf("Tail(hinted runner) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Line != "registered runner ready" || entries[0].ContainerInstance != "runner-ns/nopsai-k8s-runner-prod-6f4d" {
		t.Fatalf("hinted runner entries = %#v", entries)
	}
	if len(api.logCalls) != 1 || api.logCalls[0].namespace != "runner-ns" || api.logCalls[0].podName != "nopsai-k8s-runner-prod-6f4d" || api.logCalls[0].options.Container != "runner" {
		t.Fatalf("log calls = %#v", api.logCalls)
	}
}

func TestProviderResolvesRegisteredRunnerHintByPlatformID(t *testing.T) {
	selector := "nopsai.io/runner-id=runner-prod"
	ownedPod := PodSummary{
		Namespace: "runner-ns",
		Name:      "runner-prod-owned",
		Labels: map[string]string{
			"app.kubernetes.io/name":      "nopsai-k8s-runner",
			"app.kubernetes.io/component": "runner",
			"nopsai.io/runner-id":         "runner-prod",
			"nopsai.io/platform-id":       "platform-a",
		},
		Phase:             "Running",
		Ready:             true,
		CreationTimestamp: time.Unix(100, 0),
	}
	otherPlatformPod := PodSummary{
		Namespace: "runner-ns",
		Name:      "runner-prod-other-platform",
		Labels: map[string]string{
			"app.kubernetes.io/name":      "nopsai-k8s-runner",
			"app.kubernetes.io/component": "runner",
			"nopsai.io/runner-id":         "runner-prod",
			"nopsai.io/platform-id":       "platform-b",
		},
		Phase:             "Running",
		Ready:             true,
		CreationTimestamp: time.Unix(200, 0),
	}
	api := &fakeAPI{
		podsByCall: map[listCall][]PodSummary{
			{namespace: "nopsai", labelSelector: "app.kubernetes.io/name=nopsai"}:            {},
			{namespace: "nopsai", labelSelector: "app.kubernetes.io/name=nopsai-k8s-runner"}: {},
			{namespace: "runner-ns", labelSelector: selector}:                                {otherPlatformPod, ownedPod},
		},
		logs: "2026-06-23T10:15:30Z owned runner ready\n",
	}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{
		Namespace:     "nopsai",
		LabelSelector: "app.kubernetes.io/name=nopsai",
		PlatformID:    "platform-a",
		RunnerSourceResolver: fakeRunnerSourceResolver{hints: []RunnerSourceHint{{
			RunnerID:      "runner-prod",
			SourceID:      systemlogs.RunnerSourceID("runner-prod"),
			PlatformID:    "platform-a",
			Namespace:     "runner-ns",
			LabelSelector: selector,
		}}},
	})

	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	var found systemlogs.SourceStatus
	for _, source := range sources {
		if source.ID == systemlogs.RunnerSourceID("runner-prod") {
			found = source
			break
		}
	}
	if !found.Available || found.ContainerInstance != "runner-ns/runner-prod-owned" {
		t.Fatalf("runner source = %#v, want owned pod available", found)
	}

	entries, err := provider.Tail(context.Background(), systemlogs.RunnerSourceID("runner-prod"), 10)
	if err != nil {
		t.Fatalf("Tail() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ContainerInstance != "runner-ns/runner-prod-owned" || entries[0].Line != "owned runner ready" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestProviderDoesNotResolveOtherPlatformRunnerPod(t *testing.T) {
	selector := "nopsai.io/runner-id=runner-prod"
	api := &fakeAPI{
		podsByCall: map[listCall][]PodSummary{
			{namespace: "nopsai", labelSelector: "app.kubernetes.io/name=nopsai"}:            {},
			{namespace: "nopsai", labelSelector: "app.kubernetes.io/name=nopsai-k8s-runner"}: {},
			{namespace: "runner-ns", labelSelector: selector}: {{
				Namespace: "runner-ns",
				Name:      "runner-prod-other-platform",
				Labels: map[string]string{
					"app.kubernetes.io/name":      "nopsai-k8s-runner",
					"app.kubernetes.io/component": "runner",
					"nopsai.io/runner-id":         "runner-prod",
					"nopsai.io/platform-id":       "platform-b",
				},
				Phase: "Running",
				Ready: true,
			}},
		},
	}
	provider := NewProvider(api, systemlogs.DefaultRegistry(), Options{
		Namespace:     "nopsai",
		LabelSelector: "app.kubernetes.io/name=nopsai",
		PlatformID:    "platform-a",
		RunnerSourceResolver: fakeRunnerSourceResolver{hints: []RunnerSourceHint{{
			RunnerID:      "runner-prod",
			SourceID:      systemlogs.RunnerSourceID("runner-prod"),
			Namespace:     "runner-ns",
			LabelSelector: selector,
		}}},
	})

	sources, err := provider.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	for _, source := range sources {
		if source.ID == systemlogs.RunnerSourceID("runner-prod") && source.Available {
			t.Fatalf("runner source = %#v, want other platform pod unavailable", source)
		}
	}
	if _, err := provider.Tail(context.Background(), systemlogs.RunnerSourceID("runner-prod"), 10); !errors.Is(err, systemlogs.ErrSourceNotFound) {
		t.Fatalf("Tail() error = %v, want ErrSourceNotFound", err)
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
