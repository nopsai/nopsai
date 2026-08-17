package app

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"nopsai/config"
	"nopsai/pkg/credentialbroker"
	"nopsai/pkg/serviceauth"
	"nopsai/services/git-bot/internal/service"
)

const testServiceSigningKey = "git-bot-app-test-signing-key-0123456789"

func TestGitHubRuntimeStartsDegradedWhenCredentialsUnavailable(t *testing.T) {
	gitBot, _ := newTestGitBot(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	gitBot.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", rec.Code, http.StatusOK)
	}

	req = httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"action":"opened"}`))
	rec = httptest.NewRecorder()
	gitBot.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("webhook status = %d, want %d: %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

// A GitHub App connected after git-bot started must be picked up by a refresh,
// so the operator does not have to restart the container.
func TestGitHubRuntimeRefreshLoadsCredentialsAfterStartup(t *testing.T) {
	bootstrap := &fakeBootstrap{}
	gitBot, runtime := newTestGitBot(t, bootstrap)

	if err := runtime.refresh(context.Background()); err == nil {
		t.Fatal("refresh() error = nil, want failure while no GitHub App is configured")
	}
	if runtime.configured() {
		t.Fatal("configured() = true before a GitHub App exists")
	}

	bootstrap.set(gitHubBootstrap{
		GitHubAppID:         "424242",
		GitHubPrivateKey:    testPrivateKeyPEM(t),
		GitHubWebhookSecret: "first-secret",
	})
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatalf("refresh() error = %v", err)
	}
	if runtime.AppID() != 424242 {
		t.Fatalf("AppID() = %d, want 424242", runtime.AppID())
	}
	if !runtime.configured() {
		t.Fatal("configured() = false after credentials were loaded")
	}

	body := []byte(`{"action":"created","installation":{"id":7}}`)
	rec := httptest.NewRecorder()
	gitBot.Handler().ServeHTTP(rec, signedWebhookRequest(body, "first-secret"))
	if rec.Code == http.StatusServiceUnavailable || rec.Code == http.StatusUnauthorized {
		t.Fatalf("webhook status = %d, want the request to pass signature verification", rec.Code)
	}
}

// Rotating the App must take effect in place: the old webhook secret stops
// verifying and the new one starts.
func TestGitHubRuntimeRefreshRotatesWebhookSecret(t *testing.T) {
	privateKey := testPrivateKeyPEM(t)
	bootstrap := &fakeBootstrap{}
	bootstrap.set(gitHubBootstrap{
		GitHubAppID:         "111",
		GitHubPrivateKey:    privateKey,
		GitHubWebhookSecret: "old-secret",
	})
	gitBot, runtime := newTestGitBot(t, bootstrap)
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatalf("refresh() error = %v", err)
	}

	bootstrap.set(gitHubBootstrap{
		GitHubAppID:         "222",
		GitHubPrivateKey:    privateKey,
		GitHubWebhookSecret: "new-secret",
	})
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatalf("refresh() error = %v", err)
	}
	if runtime.AppID() != 222 {
		t.Fatalf("AppID() = %d, want 222", runtime.AppID())
	}
	if runtime.WebhookSecret() != "new-secret" {
		t.Fatalf("WebhookSecret() = %q, want %q", runtime.WebhookSecret(), "new-secret")
	}

	body := []byte(`{"action":"created","installation":{"id":9}}`)
	rec := httptest.NewRecorder()
	gitBot.Handler().ServeHTTP(rec, signedWebhookRequest(body, "old-secret"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("webhook status with superseded secret = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestGitHubRuntimeRefreshKeepsCredentialsWhenNopsaiIsUnreachable(t *testing.T) {
	bootstrap := &fakeBootstrap{}
	bootstrap.set(gitHubBootstrap{
		GitHubAppID:         "555",
		GitHubPrivateKey:    testPrivateKeyPEM(t),
		GitHubWebhookSecret: "keep-me",
	})
	_, runtime := newTestGitBot(t, bootstrap)
	if err := runtime.refresh(context.Background()); err != nil {
		t.Fatalf("refresh() error = %v", err)
	}

	bootstrap.fail(true)
	if err := runtime.refresh(context.Background()); err == nil {
		t.Fatal("refresh() error = nil, want the upstream failure to surface")
	}
	if runtime.AppID() != 555 || runtime.WebhookSecret() != "keep-me" {
		t.Fatalf("credentials changed after a failed refresh: app=%d secret=%q", runtime.AppID(), runtime.WebhookSecret())
	}
}

// fakeBootstrap stands in for the NopsAI credential broker endpoint.
type fakeBootstrap struct {
	mu        sync.Mutex
	payload   *gitHubBootstrap
	failing   bool
	serviceID string
}

func (f *fakeBootstrap) set(payload gitHubBootstrap) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payload = &payload
}

func (f *fakeBootstrap) fail(failing bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failing = failing
}

func (f *fakeBootstrap) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	payload, failing, serviceID := f.payload, f.failing, f.serviceID
	f.mu.Unlock()
	if failing || payload == nil {
		http.Error(w, "GitHub integration is incomplete", http.StatusServiceUnavailable)
		return
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sealed, err := credentialbroker.Seal(testServiceSigningKey, serviceID, plaintext)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(gitHubBootstrapEnvelope{Sealed: sealed})
}

func newTestGitBot(t *testing.T, bootstrap *fakeBootstrap) (*service.GitBotApp, *githubRuntime) {
	t.Helper()
	cfg := &config.Config{ServiceJWTSigningKey: testServiceSigningKey}
	if bootstrap != nil {
		bootstrap.serviceID = cfg.EffectiveGitBotServiceID()
		server := httptest.NewServer(bootstrap)
		t.Cleanup(server.Close)
		cfg.NopsaiAPIURL = server.URL
	}
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
		Role:       serviceauth.RoleGitBot,
		ServiceID:  cfg.EffectiveGitBotServiceID(),
	})
	if err != nil {
		t.Fatalf("NewCredentials() error = %v", err)
	}
	runtime := newGitHubRuntime(cfg, http.DefaultClient, credentials)
	gitBot := service.NewGitBotAppWithCredentials(cfg, runtime.resolver, http.DefaultClient, runtime, nil, credentials)
	return gitBot, runtime
}

func signedWebhookRequest(body []byte, secret string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(string(body)))
	req.Header.Set("X-GitHub-Event", "installation")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}
