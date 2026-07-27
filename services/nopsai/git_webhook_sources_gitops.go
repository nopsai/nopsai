package nopsai

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

const gitWebhookSourcesGitOpsDirectory = "git-webhook-sources"

type gitWebhookSourceGitOpsDocument = gitWebhookSourceInput

type storedGitWebhookSource struct {
	input      gitWebhookSourceRecord
	sourcePath string
}

func parseGitOpsGitWebhookSources(
	files map[string]string,
	sourceDir string,
	binding models.ConfigRepository,
	boundTeam string,
) (map[string]storedGitWebhookSource, error) {
	sources := map[string]storedGitWebhookSource{}
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, sourceDir)
		if !ok || rel == "" || strings.HasSuffix(rel, "/") || !isYAMLFile(rel) {
			continue
		}
		var doc gitWebhookSourceGitOpsDocument
		if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
			return nil, fmt.Errorf("failed to parse git webhook source %q: %w", normalized, err)
		}
		id := strings.TrimSpace(doc.ID)
		if id == "" {
			id = strings.TrimSuffix(strings.Trim(filepath.ToSlash(rel), "/"), filepath.Ext(rel))
			if binding.ScopeType == models.ConfigRepositoryScopeTeam {
				id = externalTriggerGitOpsSlug(strings.Trim(boundTeam, "/") + "/" + id)
			} else {
				id = externalTriggerGitOpsSlug(id)
			}
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			teamPath := strings.Trim(strings.TrimSpace(doc.TeamPath), "/")
			if teamPath == "" {
				doc.TeamPath = strings.Trim(strings.TrimSpace(boundTeam), "/")
			} else if _, rootOnly := stripRootPathPrefix(teamPath); rootOnly {
				doc.TeamPath = rootGrantID
			} else {
				normalizedTeam, err := configsync.NormalizePathForTeam(boundTeam, teamPath)
				if err != nil {
					return nil, fmt.Errorf("invalid team-scoped git webhook source team_path %q: %w", normalized, err)
				}
				doc.TeamPath = normalizedTeam
			}
		}
		source, err := normalizeGitWebhookSourceInput(doc, id)
		if err != nil {
			return nil, fmt.Errorf("invalid git webhook source %q: %w", normalized, err)
		}
		if _, exists := sources[source.ID]; exists {
			return nil, fmt.Errorf("duplicate git webhook source %q detected in config repository", source.ID)
		}
		sources[source.ID] = storedGitWebhookSource{input: source, sourcePath: normalized}
	}
	return sources, nil
}

func gitWebhookSourceExportPath(
	repo models.ConfigRepository,
	source gitWebhookSourceRecord,
	sourcePath string,
	managed bool,
	configRepoIDValid bool,
	configRepoID int64,
) (string, bool) {
	id := externalTriggerGitOpsSlug(source.ID)
	if id == "" {
		return "", false
	}
	canonicalPath := filepath.ToSlash(filepath.Join(gitWebhookSourcesGitOpsDirectory, id+".yaml"))
	if managed && configRepoIDValid && configRepoID == repo.ID && strings.TrimSpace(sourcePath) != "" {
		if managedPath, ok := configsync.ManagedSourcePathForCanonical(repo, sourcePath, canonicalPath, configRepositoryDriftPathOptions()); ok {
			return managedPath, true
		}
	}
	return canonicalPath, true
}

func effectiveGitWebhookSourceTeamPath(source gitWebhookSourceRecord) string {
	if teamPath := strings.Trim(strings.TrimSpace(source.TeamPath), "/"); teamPath != "" {
		return teamPath
	}
	return rootGrantID
}
