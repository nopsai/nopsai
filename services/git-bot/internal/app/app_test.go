package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGitBotAppFromBootstrapStartsDegradedWhenCredentialsUnavailable(t *testing.T) {
	gitBot := newGitBotAppFromBootstrap(nil, nil, nil, nil, gitHubBootstrap{}, errors.New("not configured"))
	if gitBot == nil {
		t.Fatal("newGitBotAppFromBootstrap() = nil, want degraded app")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	gitBot.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestNewGitBotAppFromBootstrapDegradedWebhookReturnsUnavailable(t *testing.T) {
	gitBot := newGitBotAppFromBootstrap(nil, nil, nil, nil, gitHubBootstrap{}, errors.New("not configured"))
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"action":"opened"}`))
	rec := httptest.NewRecorder()

	gitBot.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("webhook status = %d, want %d: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}
