package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/config"
	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

type gitOpsRuntimeSettingsDirectory struct {
	root  string
	files map[string]string
}

type gitOpsRuntimeSettingsFileCandidate struct {
	sourcePath string
	content    string
}

type gitOpsRuntimeSettingsPlan struct {
	payload    systemConfigPayload
	sourcePath string
}

type runtimeSettingsGitOpsFile struct {
	AgentNopsaiAPIURL         *string             `json:"agent_nopsai_api_url" yaml:"agent_nopsai_api_url,omitempty"`
	GitBotNopsaiAPIURL        *string             `json:"git_bot_nopsai_api_url" yaml:"git_bot_nopsai_api_url,omitempty"`
	NopsaiGitBotAPIURL        *string             `json:"nopsai_git_bot_api_url" yaml:"nopsai_git_bot_api_url,omitempty"`
	DispatcherAddress         *string             `json:"dispatcher_address" yaml:"dispatcher_address,omitempty"`
	AgentImage                *string             `json:"agent_image" yaml:"agent_image,omitempty"`
	DockerNetworkName         *string             `json:"docker_network_name" yaml:"docker_network_name,omitempty"`
	AutoRemovalAgentContainer *bool               `json:"auto_removal_agent_container" yaml:"auto_removal_agent_container,omitempty"`
	DefaultPipelineTimeout    *string             `json:"default_pipeline_timeout" yaml:"default_pipeline_timeout,omitempty"`
	LLMAgentTimeout           *string             `json:"llm_agent_timeout" yaml:"llm_agent_timeout,omitempty"`
	DispatcherRouting         map[string][]string `json:"dispatcher_routing" yaml:"dispatcher_routing,omitempty"`
	RunnerID                  *string             `json:"runner_id" yaml:"runner_id,omitempty"`
	RunnerScopes              *string             `json:"runner_scopes" yaml:"runner_scopes,omitempty"`
	RunnerCapacity            *int                `json:"runner_capacity" yaml:"runner_capacity,omitempty"`
}

func parseGitOpsRuntimeSettingsPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsRuntimeSettingsPlan, error) {
	candidates := []gitOpsRuntimeSettingsFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := relativeConfigPath(normalized, root)
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
	return strings.Trim(filepath.ToSlash(rel), "/") == "system/runner.yaml"
}

func parseGitOpsRuntimeSettingsFile(content, sourcePath string) (*gitOpsRuntimeSettingsPlan, error) {
	var file runtimeSettingsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse runtime settings GitOps file '%s': %w", sourcePath, err)
	}
	payload := systemConfigPayload{
		AgentNopsaiAPIURL:         file.AgentNopsaiAPIURL,
		GitBotNopsaiAPIURL:        file.GitBotNopsaiAPIURL,
		NopsaiGitBotAPIURL:        file.NopsaiGitBotAPIURL,
		DispatcherAddress:         file.DispatcherAddress,
		AgentImage:                file.AgentImage,
		DockerNetworkName:         file.DockerNetworkName,
		AutoRemovalAgentContainer: file.AutoRemovalAgentContainer,
		DefaultPipelineTimeout:    file.DefaultPipelineTimeout,
		LLMAgentTimeout:           file.LLMAgentTimeout,
		DispatcherRouting:         file.DispatcherRouting,
		RunnerID:                  file.RunnerID,
		RunnerScopes:              file.RunnerScopes,
		RunnerCapacity:            file.RunnerCapacity,
	}
	if payload.RunnerCapacity != nil && *payload.RunnerCapacity <= 0 {
		return nil, fmt.Errorf("runtime settings GitOps file '%s' has invalid runner_capacity", sourcePath)
	}
	return &gitOpsRuntimeSettingsPlan{payload: payload, sourcePath: sourcePath}, nil
}

func buildRuntimeSettingsGitOpsFile(cfg config.Config) runtimeSettingsGitOpsFile {
	runnerCapacity := cfg.RunnerCapacity
	if runnerCapacity <= 0 {
		runnerCapacity = 1
	}
	return runtimeSettingsGitOpsFile{
		AgentNopsaiAPIURL:         stringPtr(cfg.AgentNopsaiAPIURL),
		GitBotNopsaiAPIURL:        stringPtr(cfg.GitBotNopsaiAPIURL),
		NopsaiGitBotAPIURL:        stringPtr(cfg.NopsaiGitBotAPIURL),
		DispatcherAddress:         stringPtr(cfg.DispatcherAddress),
		AgentImage:                stringPtr(cfg.AgentImage),
		DockerNetworkName:         stringPtr(cfg.DockerNetworkName),
		AutoRemovalAgentContainer: boolPtr(cfg.AutoRemovalAgentContainer),
		DefaultPipelineTimeout:    stringPtr(cfg.DefaultPipelineTimeout),
		LLMAgentTimeout:           stringPtr(cfg.LLMAgentTimeout),
		DispatcherRouting:         cloneDispatcherRouting(cfg.DispatcherRouting),
		RunnerID:                  stringPtr(cfg.RunnerID),
		RunnerScopes:              stringPtr(cfg.RunnerScopes),
		RunnerCapacity:            intPtr(runnerCapacity),
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
