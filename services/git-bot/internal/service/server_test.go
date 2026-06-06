package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"context"
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
