package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/services/git-bot/internal/checkrender"
)

func TestNormalizeGitHubBranchRef(t *testing.T) {
	got, err := normalizeGitHubBranchRef("refs/heads/nopsai/ui-changes")
	if err != nil {
		t.Fatalf("normalizeGitHubBranchRef() error = %v", err)
	}
	if got != "heads/nopsai/ui-changes" {
		t.Fatalf("ref = %q, want heads/nopsai/ui-changes", got)
	}

	if _, err := normalizeGitHubBranchRef("bad branch"); err == nil {
		t.Fatal("normalizeGitHubBranchRef() accepted branch with spaces")
	}
}

func TestCleanCommitFilePath(t *testing.T) {
	got, err := cleanCommitFilePath("/setting/system/mcp.yaml")
	if err != nil {
		t.Fatalf("cleanCommitFilePath() error = %v", err)
	}
	if got != "setting/system/mcp.yaml" {
		t.Fatalf("path = %q, want setting/system/mcp.yaml", got)
	}

	if _, err := cleanCommitFilePath("../outside.yaml"); err == nil {
		t.Fatal("cleanCommitFilePath() accepted escaping path")
	}
}

func TestHandleHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("body = %q, want ok", rec.Body.String())
	}
}

func TestHandleWebhookDelegatesToForwarder(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	forwarder := &recordingWebhookForwarder{}
	app := &GitBotApp{
		webhookSecret:    "test-webhook-secret",
		webhookForwarder: forwarder,
	}

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", testGitHubSignature("test-webhook-secret", body))
	rec := httptest.NewRecorder()

	app.handleWebhook(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if string(forwarder.body) != string(body) {
		t.Fatalf("forwarded body = %q, want %q", string(forwarder.body), string(body))
	}
}

func TestHandleFetchFileUsesRepositoryProvider(t *testing.T) {
	app := &GitBotApp{
		repositoryProvider: fakeRepositoryProvider{fileContent: "name: build\n"},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/github/file", strings.NewReader(`{
		"owner": "acme",
		"repo": "widgets",
		"ref": "main",
		"path": ".nopsai/pipeline.yaml"
	}`))
	rec := httptest.NewRecorder()

	app.handleFetchFile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response FileContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Content != "name: build\n" {
		t.Fatalf("content = %q, want provider content", response.Content)
	}
}

func TestHandleFetchFileReturnsUnavailableWithoutGitHubClient(t *testing.T) {
	app := NewGitBotApp(nil, nil, nil, 0, "", nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/github/file", strings.NewReader(`{
		"owner": "acme",
		"repo": "widgets",
		"ref": "main",
		"path": ".nopsai/pipeline.yaml"
	}`))
	rec := httptest.NewRecorder()

	app.handleFetchFile(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), githubIntegrationUnavailableMessage) {
		t.Fatalf("body = %q, want unavailable message", rec.Body.String())
	}
}

func TestHandleCreateCheckRunUsesChecksProvider(t *testing.T) {
	checks := &fakeChecksProvider{nextID: 42}
	app := &GitBotApp{
		checkRunStates: make(map[int64]*checkrender.State),
		checksProvider: checks,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/checks/create", strings.NewReader(`{
		"owner": "acme",
		"repo": "widgets",
		"ref": "abc123",
		"pipeline_definition": "name: build\nsteps:\n  - name: test\n    script: go test ./...\n"
	}`))
	rec := httptest.NewRecorder()

	app.handleCreateCheckRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if len(checks.created) != 1 {
		t.Fatalf("created calls = %d, want 1", len(checks.created))
	}
	if got := checks.created[0].Name; got != "build" {
		t.Fatalf("check name = %q, want build", got)
	}
	if len(checks.progressUpdates) != 1 {
		t.Fatalf("progress calls = %d, want 1", len(checks.progressUpdates))
	}
	if checks.progressUpdates[0].Summary != "Pipeline is starting..." {
		t.Fatalf("progress summary = %q, want startup summary", checks.progressUpdates[0].Summary)
	}

	var response CreateCheckRunResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.CheckRunID != 42 {
		t.Fatalf("check_run_id = %d, want 42", response.CheckRunID)
	}
}

func TestHandleCreateCheckRunReturnsUnavailableWithoutGitHubClient(t *testing.T) {
	app := NewGitBotApp(nil, nil, nil, 0, "", nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/checks/create", strings.NewReader(`{
		"owner": "acme",
		"repo": "widgets",
		"ref": "abc123",
		"pipeline_definition": "name: build\nsteps:\n  - name: test\n    script: go test ./...\n"
	}`))
	rec := httptest.NewRecorder()

	app.handleCreateCheckRun(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), githubIntegrationUnavailableMessage) {
		t.Fatalf("body = %q, want unavailable message", rec.Body.String())
	}
}

func TestHandleFindSuiteCheckRunUsesChecksProvider(t *testing.T) {
	checks := &fakeChecksProvider{
		listed: []checkRunSummary{
			{ID: 10, Name: "other", HeadSHA: "abc123", HasCheckSuite: true, CheckSuiteID: 111},
			{ID: 20, Name: "build", HeadSHA: "abc123", HeadBranch: "main", PullRequestHeadRef: "feature", HasCheckSuite: true, CheckSuiteID: 222},
		},
	}
	app := &GitBotApp{
		githubAppID:    999,
		checksProvider: checks,
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/checks/find-suite-run", strings.NewReader(`{
		"owner": "acme",
		"repo": "widgets",
		"suite_id": 222,
		"commit_sha": "abc123"
	}`))
	rec := httptest.NewRecorder()

	app.handleFindSuiteCheckRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response FindSuiteCheckRunResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.CheckRunID != 20 {
		t.Fatalf("check_run_id = %d, want 20", response.CheckRunID)
	}
	if response.HeadBranch != "main" || response.PullRequestHeadRef != "feature" {
		t.Fatalf("head refs = (%q, %q), want (main, feature)", response.HeadBranch, response.PullRequestHeadRef)
	}
}

func testGitHubSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type recordingWebhookForwarder struct {
	body []byte
}

func (f *recordingWebhookForwarder) ForwardWebhook(w http.ResponseWriter, _ *http.Request, body []byte) {
	f.body = append([]byte(nil), body...)
	w.WriteHeader(http.StatusAccepted)
}

type fakeRepositoryProvider struct {
	fileContent string
}

func (p fakeRepositoryProvider) FetchFile(context.Context, FileContentRequest) (string, error) {
	return p.fileContent, nil
}

func (p fakeRepositoryProvider) FetchDirectory(context.Context, DirectoryContentsRequest) (map[string]string, error) {
	return nil, nil
}

func (p fakeRepositoryProvider) CheckAccess(context.Context, RepositoryAccessRequest) (RepositoryAccessResponse, error) {
	return RepositoryAccessResponse{}, nil
}

func (p fakeRepositoryProvider) BranchHasOpenPR(context.Context, BranchPROpenRequest) (BranchPROpenResponse, error) {
	return BranchPROpenResponse{}, nil
}

func (p fakeRepositoryProvider) ListInstalled(context.Context) ([]InstalledRepository, error) {
	return nil, nil
}

func (p fakeRepositoryProvider) FetchPipeline(context.Context, PipelineContentRequest) (string, error) {
	return "", nil
}

type fakeChecksProvider struct {
	nextID          int64
	created         []createQueuedCheckRunRequest
	progressUpdates []checkRunProgressUpdate
	conclusions     []checkRunConclusionUpdate
	listed          []checkRunSummary
}

func (p *fakeChecksProvider) CreateQueued(_ context.Context, req createQueuedCheckRunRequest) (int64, error) {
	p.created = append(p.created, req)
	return p.nextID, nil
}

func (p *fakeChecksProvider) MarkInProgress(_ context.Context, update checkRunProgressUpdate) error {
	p.progressUpdates = append(p.progressUpdates, update)
	return nil
}

func (p *fakeChecksProvider) Conclude(_ context.Context, update checkRunConclusionUpdate) error {
	p.conclusions = append(p.conclusions, update)
	return nil
}

func (p *fakeChecksProvider) ListForRef(context.Context, string, string, string) ([]checkRunSummary, error) {
	return p.listed, nil
}
