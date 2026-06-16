package nopsai

import (
	"encoding/json"
	"net/http"
	"strings"

	"nopsai/pkg/credentialbroker"
	"nopsai/pkg/serviceauth"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/pkg/auth"
)

type gitBotBootstrapResponse struct {
	GitHubAppID          string `json:"github_app_id"`
	GitHubInstallationID string `json:"github_installation_id"`
	GitHubPrivateKey     string `json:"github_private_key"`
	GitHubWebhookSecret  string `json:"github_webhook_secret"`
}

type gitBotBootstrapEnvelope struct {
	Sealed string `json:"sealed"`
}

func (a *App) handleGitBotBootstrap(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.ClaimsFromContext(r.Context())
	if !ok || claims == nil ||
		!strings.EqualFold(strings.TrimSpace(claims.Provider), serviceauth.ProviderInternalService) ||
		!containsFold(claims.Roles, serviceauth.RoleGitBot) {
		http.Error(w, "git-bot service identity required", http.StatusForbidden)
		return
	}
	if a == nil || a.cfg == nil {
		http.Error(w, "GitHub integration is not configured", http.StatusServiceUnavailable)
		return
	}
	cfg := a.getConfigSnapshot()
	privateKey, err := a.resolveCredentialText(r.Context(), cfg.GitHubPrivateKeyCredentialRef, credentials.Purpose{
		ConsumerService: serviceauth.RoleGitBot,
		Operation:       "github.app_authenticate",
		SubjectType:     "service",
		SubjectID:       claims.Sub,
	})
	if err != nil {
		http.Error(w, "GitHub private key credential is unavailable", http.StatusServiceUnavailable)
		return
	}
	webhookSecret, err := a.resolveCredentialText(r.Context(), cfg.GitHubWebhookCredentialRef, credentials.Purpose{
		ConsumerService: serviceauth.RoleGitBot,
		Operation:       "github.verify_webhook",
		SubjectType:     "service",
		SubjectID:       claims.Sub,
	})
	if err != nil {
		http.Error(w, "GitHub webhook credential is unavailable", http.StatusServiceUnavailable)
		return
	}
	response := gitBotBootstrapResponse{
		GitHubAppID:          strings.TrimSpace(cfg.GitHubAppID),
		GitHubInstallationID: strings.TrimSpace(cfg.GitHubInstallID),
		GitHubPrivateKey:     privateKey,
		GitHubWebhookSecret:  webhookSecret,
	}
	if response.GitHubAppID == "" || response.GitHubInstallationID == "" ||
		response.GitHubPrivateKey == "" || response.GitHubWebhookSecret == "" {
		http.Error(w, "GitHub integration is incomplete", http.StatusServiceUnavailable)
		return
	}
	plaintext, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to encode GitHub integration", http.StatusInternalServerError)
		return
	}
	sealed, err := credentialbroker.Seal(cfg.EffectiveServiceJWTSigningKey(), claims.Sub, plaintext)
	if err != nil {
		http.Error(w, "failed to protect GitHub integration", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, gitBotBootstrapEnvelope{Sealed: sealed})
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(expected)) {
			return true
		}
	}
	return false
}
