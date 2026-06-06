package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/proto"
)

func TestHealthURL(t *testing.T) {
	got, err := healthURL(" http://aaa:8082/ ", "healthz")
	if err != nil {
		t.Fatalf("healthURL() error = %v", err)
	}
	if got != "http://aaa:8082/healthz" {
		t.Fatalf("healthURL() = %q, want http://aaa:8082/healthz", got)
	}

	if _, err := healthURL("aaa:8082", "/healthz"); err == nil {
		t.Fatal("healthURL() accepted URL without scheme and host")
	}
	if _, err := healthURL("", "/healthz"); err == nil {
		t.Fatal("healthURL() accepted empty base URL")
	}
}

func TestCheckHTTPHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/fail":
			http.Error(w, "not ready", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	app := App{httpClient: server.Client()}
	status, message := app.checkHTTPHealth(context.Background(), server.URL, "/healthz")
	if status != "ok" || message != "Reachable." {
		t.Fatalf("checkHTTPHealth() = (%q, %q), want ok reachable", status, message)
	}

	status, message = app.checkHTTPHealth(context.Background(), server.URL, "/fail")
	if status != "error" || !strings.Contains(message, "HTTP 503") || !strings.Contains(message, "not ready") {
		t.Fatalf("checkHTTPHealth() failure = (%q, %q), want HTTP 503 detail", status, message)
	}

	status, message = app.checkHTTPHealth(context.Background(), "not-a-url", "/healthz")
	if status != "error" || message != "Health endpoint URL is invalid." {
		t.Fatalf("checkHTTPHealth() invalid URL = (%q, %q)", status, message)
	}
}

func TestBuildSystemServiceStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	app := App{
		cfg: &config.Config{
			AAAAPIURL:          server.URL,
			NopsaiGitBotAPIURL: server.URL,
		},
		httpClient: server.Client(),
	}
	statuses := app.buildSystemServiceStatuses(context.Background(), &proto.DispatcherStatus{
		Runners: []*proto.RunnerInfo{
			{RunnerId: "runner-a"},
			{RunnerId: "runner-b"},
		},
	}, nil)

	byID := systemServiceStatusesByID(statuses)
	assertServiceStatus(t, byID, "nopsai-api", "ok", "Serving this status request.")
	assertServiceStatus(t, byID, "database", "error", "Database pool is unavailable.")
	assertServiceStatus(t, byID, "dispatcher", "ok", "Connected.")
	assertServiceStatus(t, byID, "runners", "ok", "2 runner(s) registered.")
	assertServiceStatus(t, byID, "aaa", "ok", "Reachable.")
	assertServiceStatus(t, byID, "git-bot", "ok", "Reachable.")
	for _, status := range statuses {
		if status.CheckedAt.IsZero() {
			t.Fatalf("%s CheckedAt is zero", status.ID)
		}
	}
}

func TestBuildSystemServiceStatusesHandlesDispatcherAndGitBotWarnings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	app := App{
		cfg:        &config.Config{AAAAPIURL: server.URL},
		httpClient: server.Client(),
	}
	statuses := app.buildSystemServiceStatuses(context.Background(), nil, nil)

	byID := systemServiceStatusesByID(statuses)
	assertServiceStatus(t, byID, "dispatcher", "warning", "Dispatcher status has not been loaded.")
	assertServiceStatus(t, byID, "runners", "warning", "Runner capacity has not been loaded.")
	assertServiceStatus(t, byID, "git-bot", "warning", "NopsAI to git-bot URL is not configured.")
}

func systemServiceStatusesByID(statuses []systemServiceStatus) map[string]systemServiceStatus {
	byID := make(map[string]systemServiceStatus, len(statuses))
	for _, status := range statuses {
		byID[status.ID] = status
	}
	return byID
}

func assertServiceStatus(t *testing.T, statuses map[string]systemServiceStatus, id, wantStatus, wantMessage string) {
	t.Helper()
	status, ok := statuses[id]
	if !ok {
		t.Fatalf("missing service status %q", id)
	}
	if status.Status != wantStatus || status.Message != wantMessage {
		t.Fatalf("%s status = (%q, %q), want (%q, %q)", id, status.Status, status.Message, wantStatus, wantMessage)
	}
}
