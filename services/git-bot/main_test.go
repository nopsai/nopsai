package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
