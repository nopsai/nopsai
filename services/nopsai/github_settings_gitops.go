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

const githubSettingsGitOpsPath = "system/github.yaml"

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
	"agent_nopsai_api_url",
	"dispatcher_address",
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
	GitBotNopsaiAPIURL   *string `json:"git_bot_nopsai_api_url" yaml:"git_bot_nopsai_api_url,omitempty"`
	NopsaiGitBotAPIURL   *string `json:"nopsai_git_bot_api_url" yaml:"nopsai_git_bot_api_url,omitempty"`
	GitHubAppID          *string `json:"github_app_id" yaml:"github_app_id,omitempty"`
	GitHubInstallationID *string `json:"github_installation_id" yaml:"github_installation_id,omitempty"`
	GitHubPrivateKeyRef  *string `json:"github_private_key_credential_ref" yaml:"github_private_key_credential_ref,omitempty"`
	GitHubWebhookRef     *string `json:"github_webhook_credential_ref" yaml:"github_webhook_credential_ref,omitempty"`
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
	return strings.Trim(filepath.ToSlash(rel), "/") == githubSettingsGitOpsPath
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

	var file githubSettingsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub settings GitOps file '%s': %w", sourcePath, err)
	}
	payload := gitHubSettingsPayloadFromFile(file)
	return &gitOpsGitHubSettingsPlan{payload: payload, sourcePath: sourcePath}, nil
}

func gitHubSettingsPayloadFromFile(file githubSettingsGitOpsFile) systemConfigPayload {
	return systemConfigPayload{
		GitBotNopsaiAPIURL:   file.GitBotNopsaiAPIURL,
		NopsaiGitBotAPIURL:   file.NopsaiGitBotAPIURL,
		GitHubAppID:          file.GitHubAppID,
		GitHubInstallationID: file.GitHubInstallationID,
		GitHubPrivateKeyRef:  file.GitHubPrivateKeyRef,
		GitHubWebhookRef:     file.GitHubWebhookRef,
	}
}

func buildGitHubSettingsGitOpsFile(cfg config.Config) githubSettingsGitOpsFile {
	return githubSettingsGitOpsFile{
		GitBotNopsaiAPIURL:   stringPtr(cfg.GitBotNopsaiAPIURL),
		NopsaiGitBotAPIURL:   stringPtr(cfg.NopsaiGitBotAPIURL),
		GitHubAppID:          stringPtr(cfg.GitHubAppID),
		GitHubInstallationID: stringPtr(cfg.GitHubInstallID),
		GitHubPrivateKeyRef:  stringPtr(cfg.GitHubPrivateKeyCredentialRef),
		GitHubWebhookRef:     stringPtr(cfg.GitHubWebhookCredentialRef),
	}
}
