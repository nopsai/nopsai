package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nopsai/config"
	"nopsai/pkg/correlation"
	"nopsai/pkg/credentialbroker"
	"nopsai/pkg/serviceauth"
)

type gitHubBootstrap struct {
	GitHubAppID         string `json:"github_app_id"`
	GitHubPrivateKey    string `json:"github_private_key"`
	GitHubWebhookSecret string `json:"github_webhook_secret"`
}

type gitHubBootstrapEnvelope struct {
	Sealed string `json:"sealed"`
}

var errInvalidGitHubAppID = errors.New("GitHub App ID is not a positive integer")

// gitHubBootstrapURL is the NopsAI endpoint that hands git-bot the GitHub App
// credentials it is allowed to use.
func gitHubBootstrapURL(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.EffectiveNopsaiAPIURL()), "/")
	if base == "" {
		return ""
	}
	return base + "/v1/internal/git-bot/bootstrap"
}

func requestGitHubBootstrap(
	ctx context.Context,
	cfg *config.Config,
	httpClient *http.Client,
	credentials *serviceauth.Credentials,
	url string,
) (gitHubBootstrap, error) {
	var result gitHubBootstrap
	if strings.TrimSpace(url) == "" {
		return result, fmt.Errorf("NOPSAI_API_URL is required")
	}
	if credentials == nil {
		return result, fmt.Errorf("git-bot service credentials are required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	ctx, _ = correlation.EnsureRequestID(ctx)
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
	correlation.SetHTTPHeaders(ctx, req.Header)
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
		strings.TrimSpace(result.GitHubPrivateKey) == "" ||
		strings.TrimSpace(result.GitHubWebhookSecret) == "" {
		return gitHubBootstrap{}, fmt.Errorf("credential broker returned incomplete GitHub configuration")
	}
	return result, nil
}
