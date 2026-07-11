package nopsai

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

const externalTriggersGitOpsDirectory = "external-triggers"

type externalTriggerGitOpsDocument struct {
	ID              string                         `yaml:"id,omitempty"`
	Name            string                         `yaml:"name,omitempty"`
	Description     string                         `yaml:"description,omitempty"`
	Enabled         *bool                          `yaml:"enabled,omitempty"`
	Pipeline        string                         `yaml:"pipeline"`
	Scope           string                         `yaml:"scope,omitempty"`
	RunTeamPath     string                         `yaml:"run_team_path,omitempty"`
	AllowedCallers  []externalTriggerAllowedCaller `yaml:"allowed_callers,omitempty"`
	VariableMapping map[string]string              `yaml:"variable_mapping,omitempty"`
	PayloadSchema   map[string]any                 `yaml:"payload_schema,omitempty"`
	RateLimit       map[string]any                 `yaml:"rate_limit,omitempty"`
}

type storedExternalTrigger struct {
	input      externalTriggerRecord
	sourcePath string
}

func parseGitOpsExternalTriggers(files map[string]string, triggerDir string, binding models.ConfigRepository, boundTeam string) (map[string]storedExternalTrigger, error) {
	triggers := map[string]storedExternalTrigger{}
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, triggerDir)
		if !ok {
			continue
		}
		if rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}

		var doc externalTriggerGitOpsDocument
		if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
			return nil, fmt.Errorf("failed to parse external trigger '%s': %w", normalized, err)
		}

		id, err := externalTriggerGitOpsID(doc.ID, rel, binding, boundTeam)
		if err != nil {
			return nil, fmt.Errorf("invalid external trigger path '%s': %w", normalized, err)
		}
		pipeline, pipelineRootQualified := normalizeExternalTriggerPipelineReference(doc.Pipeline)
		scope := strings.Trim(strings.TrimSpace(doc.Scope), "/")
		runTeamPath := strings.Trim(strings.TrimSpace(doc.RunTeamPath), "/")
		if strings.EqualFold(scope, defaultRuntimeScope) {
			scope = ""
		}
		if normalizedScope, rootOnly := stripRootPathPrefix(scope); rootOnly {
			scope = ""
		} else {
			scope = normalizedScope
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			if pipeline != "" && !pipelineRootQualified {
				pipeline, err = configsync.NormalizePathForTeam(boundTeam, pipeline)
				if err != nil {
					return nil, fmt.Errorf("invalid team-scoped external trigger pipeline '%s': %w", normalized, err)
				}
			}
			if scope != "" {
				scope, err = configsync.NormalizePathForTeam(boundTeam, scope)
				if err != nil {
					return nil, fmt.Errorf("invalid team-scoped external trigger scope '%s': %w", normalized, err)
				}
			}
			if runTeamPath != "" {
				if _, rootOnly := stripRootPathPrefix(runTeamPath); rootOnly {
					runTeamPath = rootGrantID
				} else {
					runTeamPath, err = configsync.NormalizePathForTeam(boundTeam, runTeamPath)
					if err != nil {
						return nil, fmt.Errorf("invalid team-scoped external trigger run_team_path '%s': %w", normalized, err)
					}
				}
			}
		}

		trigger, err := normalizeExternalTriggerInput(externalTriggerInput{
			ID:              id,
			Name:            doc.Name,
			Description:     doc.Description,
			Enabled:         doc.Enabled,
			Pipeline:        pipeline,
			Scope:           scope,
			RunTeamPath:     runTeamPath,
			AllowedCallers:  doc.AllowedCallers,
			VariableMapping: doc.VariableMapping,
			PayloadSchema:   doc.PayloadSchema,
			RateLimit:       doc.RateLimit,
		}, "")
		if err != nil {
			return nil, fmt.Errorf("invalid external trigger '%s': %w", normalized, err)
		}
		if _, exists := triggers[trigger.ID]; exists {
			return nil, fmt.Errorf("duplicate external trigger '%s' detected in config repository", trigger.ID)
		}
		triggers[trigger.ID] = storedExternalTrigger{input: trigger, sourcePath: normalized}
	}
	return triggers, nil
}

func externalTriggerGitOpsID(explicitID, rel string, binding models.ConfigRepository, boundTeam string) (string, error) {
	id := strings.TrimSpace(explicitID)
	if id != "" {
		return id, nil
	}
	rel = strings.TrimSuffix(strings.Trim(filepath.ToSlash(rel), "/"), filepath.Ext(rel))
	if rel == "" || strings.Contains(rel, "..") {
		return "", fmt.Errorf("file path does not specify an external trigger id")
	}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		normalized, err := configsync.NormalizePathForTeam(boundTeam, rel)
		if err != nil {
			return "", err
		}
		rel = normalized
	}
	return externalTriggerGitOpsSlug(rel), nil
}

func externalTriggerGitOpsSlug(value string) string {
	value = strings.Trim(strings.TrimSpace(filepath.ToSlash(value)), "/")
	value = strings.ReplaceAll(value, "/", "-")
	value = slugifyExternalTriggerID(value)
	return value
}

func externalTriggerConfigScope(trigger externalTriggerRecord) string {
	runTeamPath := strings.Trim(strings.TrimSpace(trigger.RunTeamPath), "/")
	if runTeamPath != "" {
		return runTeamPath
	}
	return rootGrantID
}

func externalTriggerExportPath(repo models.ConfigRepository, trigger externalTriggerRecord, sourcePath string, managed bool, configRepoIDValid bool, configRepoID int64) (string, bool) {
	if managed && configRepoIDValid && configRepoID == repo.ID && strings.TrimSpace(sourcePath) != "" {
		return configRepositoryManagedSourcePath(repo, sourcePath)
	}
	id := externalTriggerGitOpsSlug(trigger.ID)
	if id == "" {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(externalTriggersGitOpsDirectory, id+".yaml")), true
}
