package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRequestAllowsOnlyRequiredReadEndpoints(t *testing.T) {
	allowed := []string{
		"GET /_ping",
		"HEAD /v1.51/_ping",
		"GET /version",
		"GET /v1.51/containers/json?all=1&filters=%7B%7D",
		"GET /containers/nopsai-dispatcher/json",
		"GET /v1.51/containers/nopsai-dispatcher/logs?follow=1&stdout=1&stderr=1&tail=500&timestamps=1",
		"GET /v1.51/containers/runner-prod-1/logs?follow=1&stdout=1&stderr=1&tail=500&timestamps=1",
	}
	for _, raw := range allowed {
		method, target, _ := strings.Cut(raw, " ")
		req := httptest.NewRequest(method, target, nil)
		if err := validateRequest(req, allowedContainerSet()); err != nil {
			t.Errorf("validateRequest(%s) error = %v", raw, err)
		}
	}

	blocked := []string{
		"POST /containers/create",
		"DELETE /containers/abc",
		"GET /containers/abc/archive?path=/",
		"GET /containers/abc/stats",
		"GET /containers/untrusted/json",
		"GET /containers/untrusted/logs?stdout=1",
		"GET /containers/runnerprod/logs?stdout=1",
		"GET /events",
		"GET /containers/json?unknown=1",
		"GET /containers/abc/logs?follow=1&evil=1",
	}
	for _, raw := range blocked {
		method, target, _ := strings.Cut(raw, " ")
		req := httptest.NewRequest(method, target, nil)
		if err := validateRequest(req, allowedContainerSet()); err == nil {
			t.Errorf("validateRequest(%s) error = nil, want blocked", raw)
		}
	}
}

func allowedContainerSet() map[string]struct{} {
	return map[string]struct{}{"nopsai-dispatcher": {}}
}

func TestHandlerProxiesAllowedRequestToDockerSocket(t *testing.T) {
	tempDir, err := os.MkdirTemp("/tmp", "nopsai-proxy-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	socketPath := filepath.Join(tempDir, "docker.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"path":%q}`, r.URL.Path)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	recorder := httptest.NewRecorder()
	New(socketPath).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1.51/version", nil))
	response := recorder.Result()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "/v1.51/version") {
		t.Fatalf("response = %d %s", response.StatusCode, body)
	}
}

func TestHandlerRejectsBlockedRequestWithoutDockerSocket(t *testing.T) {
	recorder := httptest.NewRecorder()
	New("/missing/docker.sock").ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/containers/abc/start", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}
