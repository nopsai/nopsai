package nopsai

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/registryauth"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/internal/systemconfig"

	"gopkg.in/yaml.v3"
)

const runtimeSettingsGitOpsPath = "system/runner.yaml"

var runtimeSettingsForbiddenGitHubKeys = []string{
	"git_bot_api_url",
	"github_app_id",
	"github_installation_id",
	"github_private_key_credential_ref",
	"github_webhook_credential_ref",
}

type gitOpsRuntimeSettingsDirectory struct {
	root  string
	files map[string]string
}

type gitOpsRuntimeSettingsFileCandidate struct {
	sourcePath string
	content    string
}

type gitOpsRuntimeSettingsPlan struct {
	payload                   systemConfigPayload
	runnerRegistryCredentials map[string][]credentials.Reference
	sourcePath                string
}

type runtimeSettingsGitOpsFile struct {
	LogLevel                      *string                       `json:"log_level" yaml:"log_level,omitempty"`
	LogFormat                     *string                       `json:"log_format" yaml:"log_format,omitempty"`
	Environment                   *string                       `json:"environment" yaml:"environment,omitempty"`
	PublicURL                     *string                       `json:"public_url" yaml:"public_url,omitempty"`
	NotificationMailLogoURL       *string                       `json:"notification_mail_logo_url" yaml:"notification_mail_logo_url,omitempty"`
	NotificationMailWebsiteURL    *string                       `json:"notification_mail_website_url" yaml:"notification_mail_website_url,omitempty"`
	NotificationMailSupportURL    *string                       `json:"notification_mail_support_url" yaml:"notification_mail_support_url,omitempty"`
	NotificationMailFooterAddress *string                       `json:"notification_mail_footer_address" yaml:"notification_mail_footer_address,omitempty"`
	RequireProductionGates        *bool                         `json:"require_production_gates" yaml:"require_production_gates,omitempty"`
	NopsaiAPIURL                  *string                       `json:"nopsai_api_url" yaml:"nopsai_api_url,omitempty"`
	DispatcherAddress             *string                       `json:"dispatcher_grpc_address" yaml:"dispatcher_grpc_address,omitempty"`
	AgentImage                    *string                       `json:"agent_image" yaml:"agent_image,omitempty"`
	DockerNetworkName             *string                       `json:"docker_network_name" yaml:"docker_network_name,omitempty"`
	AutoRemovalAgentContainer     *bool                         `json:"auto_removal_agent_container" yaml:"auto_removal_agent_container,omitempty"`
	DefaultPipelineTimeout        *string                       `json:"default_pipeline_timeout" yaml:"default_pipeline_timeout,omitempty"`
	LLMAgentTimeout               *string                       `json:"llm_agent_timeout" yaml:"llm_agent_timeout,omitempty"`
	DispatcherRouting             map[string][]string           `json:"dispatcher_routing" yaml:"dispatcher_routing,omitempty"`
	EjectedRunnerIDs              []string                      `json:"ejected_runner_ids,omitempty" yaml:"-"`
	RunnerID                      *string                       `json:"runner_id" yaml:"runner_id,omitempty"`
	RunnerScopes                  *string                       `json:"runner_scopes" yaml:"runner_scopes,omitempty"`
	RunnerCapacity                *int                          `json:"runner_capacity" yaml:"runner_capacity,omitempty"`
	RunnerRegistryCredentials     map[string][]string           `json:"runner_registry_credentials" yaml:"runner_registry_credentials,omitempty"`
	Runtime                       *string                       `json:"runtime" yaml:"runtime,omitempty"`
	Kubernetes                    *config.KubernetesConfig      `json:"kubernetes" yaml:"kubernetes,omitempty"`
	Limits                        *config.RunnerLimits          `json:"limits" yaml:"limits,omitempty"`
	RuntimePools                  map[string]config.RuntimePool `json:"runtime_pools" yaml:"runtime_pools,omitempty"`
	Assistant                     *config.AssistantConfig       `json:"assistant" yaml:"assistant,omitempty"`
}

type runtimeSettingsSnapshotFile struct {
	runtimeSettingsGitOpsFile `yaml:",inline"`
	githubSettingsGitOpsFile  `yaml:",inline"`
}

func parseGitOpsRuntimeSettingsPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsRuntimeSettingsPlan, error) {
	candidates := []gitOpsRuntimeSettingsFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsRuntimeSettingsRelativePath(rel) {
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
		return nil, fmt.Errorf("runtime settings can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple runtime settings GitOps files found: %s", strings.Join(paths, ", "))
	}

	return parseGitOpsRuntimeSettingsFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsRuntimeSettingsRelativePath(rel string) bool {
	return strings.Trim(filepath.ToSlash(rel), "/") == runtimeSettingsGitOpsPath
}

func parseGitOpsRuntimeSettingsFile(content, sourcePath string) (*gitOpsRuntimeSettingsPlan, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse runtime settings GitOps file '%s': %w", sourcePath, err)
	}
	for _, key := range runtimeSettingsForbiddenGitHubKeys {
		if _, exists := raw[key]; exists {
			return nil, fmt.Errorf("runtime settings GitOps file '%s' contains GitHub setting %q; move GitHub settings to setting/system/github.yaml", sourcePath, key)
		}
	}

	var file runtimeSettingsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse runtime settings GitOps file '%s': %w", sourcePath, err)
	}
	runnerRegistryCredentials, err := parseRunnerRegistryCredentialsGitOps(file.RunnerRegistryCredentials, sourcePath)
	if err != nil {
		return nil, err
	}
	payload := systemConfigPayload{
		LogLevel:                      file.LogLevel,
		LogFormat:                     file.LogFormat,
		Environment:                   file.Environment,
		PublicURL:                     file.PublicURL,
		NotificationMailLogoURL:       file.NotificationMailLogoURL,
		NotificationMailWebsiteURL:    file.NotificationMailWebsiteURL,
		NotificationMailSupportURL:    file.NotificationMailSupportURL,
		NotificationMailFooterAddress: file.NotificationMailFooterAddress,
		RequireProductionGates:        file.RequireProductionGates,
		NopsaiAPIURL:                  file.NopsaiAPIURL,
		DispatcherAddress:             file.DispatcherAddress,
		AgentImage:                    file.AgentImage,
		DockerNetworkName:             file.DockerNetworkName,
		AutoRemovalAgentContainer:     file.AutoRemovalAgentContainer,
		DefaultPipelineTimeout:        file.DefaultPipelineTimeout,
		LLMAgentTimeout:               file.LLMAgentTimeout,
		DispatcherRouting:             file.DispatcherRouting,
		EjectedRunnerIDs:              file.EjectedRunnerIDs,
		RunnerID:                      file.RunnerID,
		RunnerScopes:                  file.RunnerScopes,
		RunnerCapacity:                file.RunnerCapacity,
		Runtime:                       file.Runtime,
		Kubernetes:                    file.Kubernetes,
		Limits:                        file.Limits,
		RuntimePools:                  file.RuntimePools,
		Assistant:                     file.Assistant,
	}
	if payload.RunnerCapacity != nil && *payload.RunnerCapacity <= 0 {
		return nil, fmt.Errorf("runtime settings GitOps file '%s' has invalid runner_capacity", sourcePath)
	}
	if payload.Limits != nil {
		if err := systemconfig.ValidateRunnerLimits(*payload.Limits); err != nil {
			return nil, fmt.Errorf("runtime settings GitOps file '%s' has invalid limits: %w", sourcePath, err)
		}
	}
	return &gitOpsRuntimeSettingsPlan{
		payload:                   payload,
		runnerRegistryCredentials: runnerRegistryCredentials,
		sourcePath:                sourcePath,
	}, nil
}

func parseRunnerRegistryCredentialsGitOps(raw map[string][]string, sourcePath string) (map[string][]credentials.Reference, error) {
	if raw == nil {
		return nil, nil
	}
	parsed := make(map[string][]credentials.Reference, len(raw))
	for rawRunnerID, rawRefs := range raw {
		runnerID := strings.TrimSpace(rawRunnerID)
		if runnerID == "" {
			return nil, fmt.Errorf("runtime settings GitOps file '%s' has empty runner_registry_credentials runner id", sourcePath)
		}
		seen := map[string]struct{}{}
		for _, rawRef := range rawRefs {
			ref, err := credentials.ParseReference(strings.TrimSpace(rawRef))
			if err != nil {
				return nil, fmt.Errorf("runtime settings GitOps file '%s' has invalid registry credential ref for runner %q: %w", sourcePath, runnerID, err)
			}
			if _, exists := seen[ref.String()]; exists {
				continue
			}
			seen[ref.String()] = struct{}{}
			parsed[runnerID] = append(parsed[runnerID], ref)
		}
	}
	return parsed, nil
}

func runnerRegistryHostsFromMetadata(metadata map[string]any) []string {
	raw := metadata["registry_hosts"]
	values, ok := raw.([]any)
	if ok {
		hosts := make([]string, 0, len(values))
		for _, value := range values {
			if host := registryauth.NormalizeRegistryHost(fmt.Sprint(value)); host != "" {
				hosts = append(hosts, host)
			}
		}
		return hosts
	}
	if typed, ok := raw.([]string); ok {
		hosts := make([]string, 0, len(typed))
		for _, value := range typed {
			if host := registryauth.NormalizeRegistryHost(value); host != "" {
				hosts = append(hosts, host)
			}
		}
		return hosts
	}
	return nil
}

func buildRuntimeSettingsGitOpsFile(cfg config.Config) runtimeSettingsGitOpsFile {
	runnerCapacity := cfg.RunnerCapacity
	if runnerCapacity <= 0 {
		runnerCapacity = 1
	}
	dispatcherRouting, _ := systemconfig.RemoveRunnersFromDispatcherRouting(cfg.DispatcherRouting, cfg.EjectedRunnerIDs)
	return runtimeSettingsGitOpsFile{
		LogLevel:                      stringPtr(cfg.LogLevel),
		LogFormat:                     stringPtr(cfg.LogFormat),
		Environment:                   stringPtr(cfg.Environment),
		PublicURL:                     stringPtr(cfg.PublicURL),
		NotificationMailLogoURL:       stringPtr(cfg.NotificationMailLogoURL),
		NotificationMailWebsiteURL:    stringPtr(cfg.NotificationMailWebsiteURL),
		NotificationMailSupportURL:    stringPtr(cfg.NotificationMailSupportURL),
		NotificationMailFooterAddress: stringPtr(cfg.NotificationMailFooterAddress),
		RequireProductionGates:        boolPtr(cfg.RequireProductionGates),
		NopsaiAPIURL:                  stringPtr(cfg.EffectiveNopsaiAPIURL()),
		DispatcherAddress:             stringPtr(cfg.DispatcherAddress),
		AgentImage:                    stringPtr(cfg.AgentImage),
		DockerNetworkName:             stringPtr(cfg.DockerNetworkName),
		AutoRemovalAgentContainer:     boolPtr(cfg.AutoRemovalAgentContainer),
		DefaultPipelineTimeout:        stringPtr(cfg.DefaultPipelineTimeout),
		LLMAgentTimeout:               stringPtr(cfg.LLMAgentTimeout),
		DispatcherRouting:             dispatcherRouting,
		EjectedRunnerIDs:              config.NormalizeRunnerIDs(cfg.EjectedRunnerIDs),
		RunnerID:                      stringPtr(cfg.RunnerID),
		RunnerScopes:                  stringPtr(cfg.RunnerScopes),
		RunnerCapacity:                intPtr(runnerCapacity),
		Runtime:                       stringPtr(config.NormalizeRuntime(cfg.Runtime)),
		Kubernetes:                    kubernetesConfigPtr(config.NormalizeKubernetesConfig(cfg.Kubernetes)),
		Limits:                        runnerLimitsPtr(cfg.Limits),
		RuntimePools:                  config.NormalizeRuntimePools(cfg.RuntimePools),
		Assistant:                     assistantConfigPtr(cfg.EffectiveAssistantConfig()),
	}
}

func buildRuntimeSettingsSnapshotFile(cfg config.Config) runtimeSettingsSnapshotFile {
	return runtimeSettingsSnapshotFile{
		runtimeSettingsGitOpsFile: buildRuntimeSettingsGitOpsFile(cfg),
		githubSettingsGitOpsFile:  buildGitHubSettingsGitOpsFile(cfg),
	}
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func intPtr(value int) *int {
	return &value
}

func kubernetesConfigPtr(value config.KubernetesConfig) *config.KubernetesConfig {
	return &value
}

func runnerLimitsPtr(value config.RunnerLimits) *config.RunnerLimits {
	return &value
}

func assistantConfigPtr(value config.AssistantConfig) *config.AssistantConfig {
	return &value
}
