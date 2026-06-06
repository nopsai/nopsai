package gitbot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFileReturnsContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/github/file" {
			t.Fatalf("path = %q, want /v1/github/file", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["owner"] != "acme" || payload["repo"] != "app" || payload["ref"] != "main" || payload["path"] != ".nopsai/pipeline.yaml" {
			t.Fatalf("payload = %#v", payload)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"content": "name: ci\n"})
	}))
	defer server.Close()

	got, err := (Client{BaseURL: server.URL}).File("acme", "app", "main", ".nopsai/pipeline.yaml", errors.New("not found"))
	if err != nil {
		t.Fatalf("File() error = %v", err)
	}
	if got != "name: ci\n" {
		t.Fatalf("File() = %q, want content", got)
	}
}

func TestFileReturnsSuppliedNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	wantErr := errors.New("pipeline not found")
	_, err := (Client{BaseURL: server.URL}).File("acme", "app", "main", ".nopsai/missing.yaml", wantErr)
	if !errors.Is(err, wantErr) {
		t.Fatalf("File() error = %v, want supplied not found error", err)
	}
}

func TestCreateCheckRunDecodesID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/checks/create" {
			t.Fatalf("path = %q, want /v1/checks/create", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]int64{"check_run_id": 42})
	}))
	defer server.Close()

	got, err := (Client{BaseURL: server.URL}).CreateCheckRun("acme", "app", "abc123", []byte("name: ci"), "git")
	if err != nil {
		t.Fatalf("CreateCheckRun() error = %v", err)
	}
	if got != 42 {
		t.Fatalf("CreateCheckRun() = %d, want 42", got)
	}
}
