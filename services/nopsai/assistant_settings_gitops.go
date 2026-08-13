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

// assistantSettingsGitOpsPath is the only GitOps location that owns Assistant
// settings. Runner settings stay in system/runner.yaml so runtime operations and
// Assistant configuration can be reviewed and delegated separately.
const assistantSettingsGitOpsPath = "system/assistant.yaml"

type gitOpsAssistantSettingsPlan struct {
	payload    systemConfigPayload
	sourcePath string
}

type assistantSettingsGitOpsFile struct {
	Assistant *config.AssistantConfig `json:"assistant" yaml:"assistant,omitempty"`
}

func parseGitOpsAssistantSettingsPlan(binding models.ConfigRepository, directories ...gitOpsRuntimeSettingsDirectory) (*gitOpsAssistantSettingsPlan, error) {
	candidates := []gitOpsRuntimeSettingsFileCandidate{}
	for _, directory := range directories {
		root := filepath.ToSlash(strings.Trim(directory.root, "/"))
		for path, content := range directory.files {
			normalized := filepath.ToSlash(path)
			rel, ok := configsync.RelativePath(normalized, root)
			if !ok || !isGitOpsAssistantSettingsRelativePath(rel) {
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
		return nil, fmt.Errorf("assistant settings can only be configured from a system config repository")
	}
	if len(candidates) > 1 {
		paths := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			paths = append(paths, candidate.sourcePath)
		}
		sort.Strings(paths)
		return nil, fmt.Errorf("multiple assistant settings GitOps files found: %s", strings.Join(paths, ", "))
	}

	return parseGitOpsAssistantSettingsFile(candidates[0].content, candidates[0].sourcePath)
}

func isGitOpsAssistantSettingsRelativePath(rel string) bool {
	return strings.Trim(filepath.ToSlash(rel), "/") == assistantSettingsGitOpsPath
}

func parseGitOpsAssistantSettingsFile(content, sourcePath string) (*gitOpsAssistantSettingsPlan, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("failed to parse assistant settings GitOps file '%s': %w", sourcePath, err)
	}
	for key := range raw {
		if key != "assistant" {
			return nil, fmt.Errorf("assistant settings GitOps file '%s' contains unsupported setting %q; this file only owns the assistant block", sourcePath, key)
		}
	}

	var file assistantSettingsGitOpsFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return nil, fmt.Errorf("failed to parse assistant settings GitOps file '%s': %w", sourcePath, err)
	}
	if file.Assistant == nil {
		return nil, fmt.Errorf("assistant settings GitOps file '%s' is missing the assistant block", sourcePath)
	}
	normalized := config.NormalizeAssistantConfig(*file.Assistant)
	return &gitOpsAssistantSettingsPlan{
		payload:    systemConfigPayload{Assistant: &normalized},
		sourcePath: sourcePath,
	}, nil
}

func buildAssistantSettingsGitOpsFile(cfg config.Config) assistantSettingsGitOpsFile {
	return assistantSettingsGitOpsFile{Assistant: assistantConfigPtr(cfg.EffectiveAssistantConfig())}
}
