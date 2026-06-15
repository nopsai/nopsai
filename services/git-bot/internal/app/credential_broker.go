package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/credentialbroker"
	"nopsai/pkg/serviceauth"
)

type gitHubBootstrap struct {
	GitHubAppID          string `json:"github_app_id"`
	GitHubInstallationID string `json:"github_installation_id"`
	GitHubPrivateKey     string `json:"github_private_key"`
	GitHubWebhookSecret  string `json:"github_webhook_secret"`
}

type gitHubBootstrapEnvelope struct {
	Sealed string `json:"sealed"`
}

func fetchGitHubBootstrap(
	ctx context.Context,
	cfg *config.Config,
	httpClient *http.Client,
	credentials *serviceauth.Credentials,
) (gitHubBootstrap, error) {
	var result gitHubBootstrap
	if cfg == nil || strings.TrimSpace(cfg.GitBotNopsaiAPIURL) == "" {
		return result, fmt.Errorf("GIT_BOT_NOPSAI_API_URL is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if credentials == nil {
		return result, fmt.Errorf("git-bot service credentials are required")
	}
	url := strings.TrimRight(strings.TrimSpace(cfg.GitBotNopsaiAPIURL), "/") + "/v1/internal/git-bot/bootstrap"
	var lastErr error
	for attempt := 1; attempt <= 20; attempt++ {
		result, lastErr = requestGitHubBootstrap(ctx, cfg, httpClient, credentials, url)
		if lastErr == nil {
			return result, nil
		}
		select {
		case <-ctx.Done():
			return gitHubBootstrap{}, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return gitHubBootstrap{}, fmt.Errorf("retrieve GitHub credentials from nopsai: %w", lastErr)
}

func requestGitHubBootstrap(
	ctx context.Context,
	cfg *config.Config,
	httpClient *http.Client,
	credentials *serviceauth.Credentials,
	url string,
) (gitHubBootstrap, error) {
	var result gitHubBootstrap
	token, err := credentials.MintToken(ctx)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("credential broker returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope gitHubBootstrapEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return result, err
	}
	plaintext, err := credentialbroker.Open(
		cfg.EffectiveServiceJWTSigningKey(),
		cfg.EffectiveGitBotServiceID(),
		envelope.Sealed,
	)
	if err != nil {
		return result, fmt.Errorf("open credential broker response: %w", err)
	}
	if err := json.Unmarshal(plaintext, &result); err != nil {
		return result, err
	}
	if strings.TrimSpace(result.GitHubAppID) == "" ||
		strings.TrimSpace(result.GitHubInstallationID) == "" ||
		strings.TrimSpace(result.GitHubPrivateKey) == "" ||
		strings.TrimSpace(result.GitHubWebhookSecret) == "" {
		return gitHubBootstrap{}, fmt.Errorf("credential broker returned incomplete GitHub configuration")
	}
	return result, nil
}
