package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"nopsai/config"
	"nopsai/pkg/correlation"
	"nopsai/pkg/serviceauth"
	"nopsai/services/git-bot/internal/service"
)

func newNopsaiInstallationFetcher(
	cfg *config.Config,
	httpClient *http.Client,
	credentials *serviceauth.Credentials,
) service.GitHubInstallationFetcher {
	return func(ctx context.Context) ([]service.GitHubInstallation, error) {
		if cfg == nil || strings.TrimSpace(cfg.EffectiveNopsaiAPIURL()) == "" {
			return nil, fmt.Errorf("NOPSAI_API_URL is required")
		}
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		if credentials == nil {
			return nil, fmt.Errorf("git-bot service credentials are required")
		}
		ctx, _ = correlation.EnsureRequestID(ctx)
		token, err := credentials.MintToken(ctx)
		if err != nil {
			return nil, err
		}
		url := strings.TrimRight(strings.TrimSpace(cfg.EffectiveNopsaiAPIURL()), "/") + "/v1/internal/git-bot/installations"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		correlation.SetHTTPHeaders(ctx, req.Header)
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("installation registry returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload []struct {
			InstallationID string `json:"installation_id"`
			AccountLogin   string `json:"account_login"`
			AccountType    string `json:"account_type"`
			Enabled        bool   `json:"enabled"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}
		installations := make([]service.GitHubInstallation, 0, len(payload))
		for _, item := range payload {
			id, err := strconv.ParseInt(strings.TrimSpace(item.InstallationID), 10, 64)
			if err != nil || id <= 0 {
				continue
			}
			installations = append(installations, service.GitHubInstallation{
				InstallationID: id,
				AccountLogin:   strings.TrimSpace(item.AccountLogin),
				AccountType:    strings.TrimSpace(item.AccountType),
				Enabled:        item.Enabled,
			})
		}
		return installations, nil
	}
}
