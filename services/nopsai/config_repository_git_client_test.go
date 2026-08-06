package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nopsai/pkg/models"
)

func TestConfigRepositoryGitClientUsesGitBotForGitHubWithoutCredential(t *testing.T) {
	app := &App{gitProvider: &fakeGitProvider{}}
	client, identity, err := app.newConfigRepositoryGitContentClient(context.Background(), models.ConfigRepository{
		Provider: models.ConfigRepositoryProviderGitHub,
		RepoURL:  "https://github.com/acme/configs.git",
	})
	if err != nil {
		t.Fatalf("newConfigRepositoryGitContentClient() error = %v", err)
	}
	if _, ok := client.(gitBotConfigRepositoryClient); !ok {
		t.Fatalf("client = %T, want gitBotConfigRepositoryClient", client)
	}
	if identity.Provider != models.ConfigRepositoryProviderGitHub || identity.Owner != "acme" || identity.Repo != "configs" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestGitLabConfigRepositoryClientReadsDirectoryAndCommits(t *testing.T) {
	var commitPayload gitlabCommitRequest
	var sawTree, sawRaw bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "gitlab-token" {
			t.Fatalf("PRIVATE-TOKEN = %q, want gitlab-token", got)
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.EscapedPath(), "/repository/tree"):
			sawTree = true
			if got := r.URL.Query().Get("path"); got != "pipelines" {
				t.Fatalf("tree path = %q, want pipelines", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"type":"blob","path":"pipelines/deploy.yaml"}]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/repository/files/pipelines%2Fdeploy.yaml/raw"):
			sawRaw = true
			_, _ = w.Write([]byte("name: deploy\n"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/repository/branches/feature%2Fconfig-sync"):
			http.NotFound(w, r)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.EscapedPath(), "/repository/commits"):
			if err := json.NewDecoder(r.Body).Decode(&commitPayload); err != nil {
				t.Fatalf("commit payload decode error = %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"abc123","web_url":"https://gitlab.example/acme/platform/configs/-/commit/abc123"}`))
		default:
			t.Fatalf("unexpected GitLab API request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer server.Close()

	repo := models.ConfigRepository{
		ID:            42,
		Provider:      models.ConfigRepositoryProviderGitLab,
		RepoURL:       server.URL + "/acme/platform/configs.git",
		Branch:        "main",
		CredentialRef: "credential://system/gitops/gitlab",
		WriteBranch:   "feature/config-sync",
	}
	app := &App{
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{repo.CredentialRef: "gitlab-token"},
	}
	client, identity, err := app.newConfigRepositoryGitContentClient(context.Background(), repo)
	if err != nil {
		t.Fatalf("newConfigRepositoryGitContentClient() error = %v", err)
	}
	if identity.ProjectPath != "acme/platform/configs" || identity.Scheme != "http" {
		t.Fatalf("identity = %#v", identity)
	}

	files, err := client.Directory(context.Background(), "main", "pipelines")
	if err != nil {
		t.Fatalf("Directory() error = %v", err)
	}
	if files["pipelines/deploy.yaml"] != "name: deploy\n" || !sawTree || !sawRaw {
		t.Fatalf("files = %#v, sawTree=%v sawRaw=%v", files, sawTree, sawRaw)
	}

	out, err := client.CommitFiles(context.Background(), "main", "feature/config-sync", "Update config", []GitCommitFile{
		{Path: "pipelines/deploy.yaml", Content: "name: deploy\n"},
		{Path: "pipelines/old.yaml", Delete: true},
	})
	if err != nil {
		t.Fatalf("CommitFiles() error = %v", err)
	}
	if out.CommitSHA != "abc123" || out.FilesChanged != 2 {
		t.Fatalf("commit response = %#v", out)
	}
	if commitPayload.Branch != "feature/config-sync" || commitPayload.StartBranch != "main" || commitPayload.CommitMessage != "Update config" {
		t.Fatalf("commit payload = %#v", commitPayload)
	}
	if len(commitPayload.Actions) != 2 || commitPayload.Actions[0].Action != "update" || commitPayload.Actions[1].Action != "delete" {
		t.Fatalf("commit actions = %#v", commitPayload.Actions)
	}
}

func TestGitLabConfigRepositoryClientFetchesDirectoryFilesConcurrently(t *testing.T) {
	var mu sync.Mutex
	activeRawRequests := 0
	maxActiveRawRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.EscapedPath(), "/repository/tree"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"type":"blob","path":"pipelines/deploy.yaml"},{"type":"blob","path":"pipelines/test.yaml"}]`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.EscapedPath(), "/repository/files/"):
			mu.Lock()
			activeRawRequests++
			if activeRawRequests > maxActiveRawRequests {
				maxActiveRawRequests = activeRawRequests
			}
			mu.Unlock()

			time.Sleep(15 * time.Millisecond)

			mu.Lock()
			activeRawRequests--
			mu.Unlock()
			if strings.Contains(r.URL.EscapedPath(), "deploy.yaml") {
				_, _ = w.Write([]byte("name: deploy\n"))
				return
			}
			_, _ = w.Write([]byte("name: test\n"))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	repo := models.ConfigRepository{
		ID:            42,
		Provider:      models.ConfigRepositoryProviderGitLab,
		RepoURL:       server.URL + "/acme/platform/configs.git",
		Branch:        "main",
		CredentialRef: "credential://system/gitops/gitlab",
	}
	app := &App{
		httpClient:         server.Client(),
		credentialResolver: staticCredentialResolver{repo.CredentialRef: "gitlab-token"},
	}
	client, _, err := app.newConfigRepositoryGitContentClient(context.Background(), repo)
	if err != nil {
		t.Fatalf("newConfigRepositoryGitContentClient() error = %v", err)
	}

	files, err := client.Directory(context.Background(), "main", "pipelines")
	if err != nil {
		t.Fatalf("Directory() error = %v", err)
	}
	if files["pipelines/deploy.yaml"] != "name: deploy\n" || files["pipelines/test.yaml"] != "name: test\n" {
		t.Fatalf("files = %#v", files)
	}

	mu.Lock()
	maxActive := maxActiveRawRequests
	mu.Unlock()
	if maxActive < 2 {
		t.Fatalf("raw file fetch max concurrency = %d, want at least 2", maxActive)
	}
}
