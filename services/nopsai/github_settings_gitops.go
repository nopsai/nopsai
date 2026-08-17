package nopsai

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

const (
	githubSettingsGitOpsPath       = "git-apps/github.yaml"
	legacyGitHubSettingsGitOpsPath = "system/github.yaml"
)

var githubSettingsForbiddenRuntimeKeys = []string{
	"log_level",
	"log_format",
	"environment",
	"public_url",
	"notification_mail_logo_url",
	"notification_mail_website_url",
	"notification_mail_support_url",
	"notification_mail_footer_address",
	"require_production_gates",
	"nopsai_api_url",
	"dispatcher_grpc_address",
	"agent_image",
	"docker_network_name",
	"auto_removal_agent_container",
	"default_pipeline_timeout",
	"llm_agent_timeout",
	"dispatcher_routing",
	"runner_id",
	"runner_scopes",
	"runner_capacity",
	"runtime",
	"kubernetes",
	"limits",
	"runtime_pools",
}

type gitOpsGitHubSettingsPlan struct {
	payload    systemConfigPayload
	sourcePath string
}

type githubSettingsGitOpsFile struct {
	Provider                *string                           `json:"provider" yaml:"provider,omitempty"`
	GitBotAPIURL            *string                           `json:"git_bot_api_url" yaml:"git_bot_api_url,omitempty"`
	AppID                   *string                           `json:"app_id" yaml:"app_id,omitempty"`
	AppSlug                 *string                           `json:"app_slug" yaml:"app_slug,omitempty"`
	WebhookURL              *string                           `json:"webhook_url" yaml:"webhook_url,omitempty"`
	GitHubAppID             *string                           `json:"github_app_id" yaml:"github_app_id,omitempty"`
	GitHubInstallationID    *string                           `json:"github_installation_id" yaml:"github_installation_id,omitempty"`
	PrivateKeyCredentialRef *string                           `json:"private_key_credential_ref" yaml:"private_key_credential_ref,omitempty"`
	WebhookCredentialRef    *string                           `json:"webhook_credential_ref" yaml:"webhook_credential_ref,omitempty"`
	GitHubPrivateKeyRef     *string                           `json:"github_private_key_credential_ref" yaml:"github_private_key_credential_ref,omitempty"`
	GitHubWebhookRef        *string                           `json:"github_webhook_credential_ref" yaml:"github_webhook_credential_ref,omitempty"`
	Installations           []config.GitHubInstallationConfig `json:"installations" yaml:"installations,omitempty"`
	GitHubInstallations     []config.GitHubInstallationConfig `json:"github_installations" yaml:"github_installations,omitempty"`
}

func parseGitOpsGitHubSettingsPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsGitHubSettingsPlan, error) {
	candidates := []gitOpsRuntimeSettingsFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsGitHubSettingsRelativePath(rel) {
				continue
			}
			candidates = append(candidates, gitOpsRuntimeSettingsFileCandidate{
				sourcePath: normalized,
				content:    content,
			})
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}
	if binding.ScopeType != models.ConfigRepositoryScopeSystem {
		return nil, fmt.Errorf("GitHub settings can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple GitHub settings GitOps files found: %s", strings.Join(paths, ", "))
	}

	return parseGitOpsGitHubSettingsFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsGitHubSettingsRelativePath(rel string) bool {
	normalized := strings.Trim(filepath.ToSlash(rel), "/")
	return normalized == githubSettingsGitOpsPath || normalized == legacyGitHubSettingsGitOpsPath
}

func parseGitOpsGitHubSettingsFile(content, sourcePath string) (*gitOpsGitHubSettingsPlan, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub settings GitOps file '%s': %w", sourcePath, err)
	}
	for _, key := range githubSettingsForbiddenRuntimeKeys {
		if _, exists := raw[key]; exists {
			return nil, fmt.Errorf("GitHub settings GitOps file '%s' contains runtime setting %q; move runner and dispatcher settings to setting/system/runner.yaml", sourcePath, key)
		}
	}
	if isCanonicalGitHubAppResourcePath(sourcePath) {
		for _, key := range []string{"git_bot_api_url", "github_installation_id", "github_installations", "team_path", "visibility", "repository_allowlist"} {
			if _, exists := raw[key]; exists {
				return nil, fmt.Errorf("GitHub App GitOps file '%s' contains unsupported field %q", sourcePath, key)
			}
		}
	}

	var file githubSettingsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub settings GitOps file '%s': %w", sourcePath, err)
	}
	payload := gitHubSettingsPayloadFromFile(file)
	return &gitOpsGitHubSettingsPlan{payload: payload, sourcePath: sourcePath}, nil
}

func gitHubSettingsPayloadFromFile(file githubSettingsGitOpsFile) systemConfigPayload {
	appID := firstStringPtr(file.AppID, file.GitHubAppID)
	appSlug := firstStringPtr(file.AppSlug)
	privateKeyRef := firstStringPtr(file.PrivateKeyCredentialRef, file.GitHubPrivateKeyRef)
	webhookRef := firstStringPtr(file.WebhookCredentialRef, file.GitHubWebhookRef)
	installations := file.Installations
	if len(installations) == 0 {
		installations = file.GitHubInstallations
	}
	installations = stripGitHubInstallationRuntimeMetadata(installations)
	return systemConfigPayload{
		GitBotAPIURL:         file.GitBotAPIURL,
		GitHubAppID:          appID,
		GitHubAppSlug:        appSlug,
		GitHubWebhookURL:     file.WebhookURL,
		GitHubInstallationID: file.GitHubInstallationID,
		GitHubInstallations:  &installations,
		GitHubPrivateKeyRef:  privateKeyRef,
		GitHubWebhookRef:     webhookRef,
	}
}

func buildGitHubSettingsGitOpsFile(cfg config.Config) githubSettingsGitOpsFile {
	return githubSettingsGitOpsFile{
		Provider:                stringPtr("github"),
		AppID:                   stringPtr(cfg.GitHubAppID),
		AppSlug:                 stringPtr(cfg.GitHubAppSlug),
		WebhookURL:              stringPtr(cfg.GitHubWebhookURL),
		PrivateKeyCredentialRef: stringPtr(cfg.GitHubPrivateKeyCredentialRef),
		WebhookCredentialRef:    stringPtr(cfg.GitHubWebhookCredentialRef),
		Installations:           gitHubSettingsInstallationsForGitOps(cfg),
	}
}

func buildGitHubSettingsRuntimeSnapshotFile(cfg config.Config) githubSettingsGitOpsFile {
	return githubSettingsGitOpsFile{
		Provider:                stringPtr("github"),
		AppID:                   stringPtr(cfg.GitHubAppID),
		AppSlug:                 stringPtr(cfg.GitHubAppSlug),
		WebhookURL:              stringPtr(cfg.GitHubWebhookURL),
		PrivateKeyCredentialRef: stringPtr(cfg.GitHubPrivateKeyCredentialRef),
		WebhookCredentialRef:    stringPtr(cfg.GitHubWebhookCredentialRef),
		Installations:           config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID),
	}
}

func gitHubSettingsInstallationsForGitOps(cfg config.Config) []config.GitHubInstallationConfig {
	installations := config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
	return stripGitHubInstallationRuntimeMetadata(installations)
}

func stripGitHubInstallationRuntimeMetadata(installations []config.GitHubInstallationConfig) []config.GitHubInstallationConfig {
	out := make([]config.GitHubInstallationConfig, 0, len(installations))
	for _, installation := range installations {
		out = append(out, config.GitHubInstallationConfig{
			InstallationID: strings.TrimSpace(installation.InstallationID),
			AccountLogin:   strings.TrimSpace(installation.AccountLogin),
			AccountType:    config.NormalizeGitHubAccountType(installation.AccountType),
			Enabled:        installation.Enabled,
		})
	}
	return out
}

func firstStringPtr(values ...*string) *string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return value
		}
	}
	return nil
}

func isCanonicalGitHubAppResourcePath(sourcePath string) bool {
	return strings.Trim(filepath.ToSlash(sourcePath), "/") == "setting/"+githubSettingsGitOpsPath ||
		strings.Trim(filepath.ToSlash(sourcePath), "/") == githubSettingsGitOpsPath
}
