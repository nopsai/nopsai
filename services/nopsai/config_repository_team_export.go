package nopsai

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

func (a *App) exportConfigRepositoryTeamStructure(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	records, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return err
	}
	structure := map[string]*configsync.TeamStructureExportNode{}
	for _, record := range records {
		path := strings.Trim(strings.TrimSpace(record.Path), "/")
		if path == "" || !configsync.TeamStructureIncludesPath(repo, path) {
			continue
		}
		if record.Kind == "app" || record.RepositoryFullName != "" {
			parentPath := teamItemParentPath(path)
			if parentPath == "" || !configsync.TeamStructureIncludesPath(repo, parentPath) {
				continue
			}
			parent := configsync.EnsureTeamStructureExportPath(structure, parentPath)
			if app, ok := configsync.BuildTeamStructureAppExport(record.Name, record.RepoURL, record.RepositoryFullName); ok {
				parent.Apps = append(parent.Apps, app)
			}
			continue
		}
		node := configsync.EnsureTeamStructureExportPath(structure, path)
		node.Description = strings.TrimSpace(record.Description)
	}

	rows, err := a.db.Query(ctx, `
		SELECT scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch,
		       config_repo_id, managed_by_config_repo
		FROM config_repositories
		WHERE scope_type = $1
		  AND id <> $2
		ORDER BY scope_id ASC
	`, models.ConfigRepositoryScopeTeam, repo.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var scopeID, repoURL, branch, basePath, writeBranch string
		var enabled, writeEnabled, managed bool
		var configRepoID sql.NullInt64
		if err := rows.Scan(&scopeID, &repoURL, &branch, &basePath, &enabled, &writeEnabled, &writeBranch, &configRepoID, &managed); err != nil {
			return err
		}
		scopeID = strings.Trim(strings.TrimSpace(scopeID), "/")
		if scopeID == "" || !configsync.TeamStructureIncludesPath(repo, scopeID) {
			continue
		}
		if managed && (!configRepoID.Valid || configRepoID.Int64 != repo.ID) {
			continue
		}
		if !managed && repo.ScopeType != models.ConfigRepositoryScopeSystem && !configsync.ResourceUnderScope(scopeID, repo.ScopeID) {
			continue
		}
		enabledValue := enabled
		writeEnabledValue := writeEnabled
		node := configsync.EnsureTeamStructureExportPath(structure, scopeID)
		node.Config = &configsync.TeamStructureBindingExport{
			RepoURL:      strings.TrimSpace(repoURL),
			Branch:       strings.TrimSpace(branch),
			BasePath:     strings.TrimSpace(basePath),
			Enabled:      &enabledValue,
			WriteEnabled: &writeEnabledValue,
			WriteBranch:  strings.TrimSpace(writeBranch),
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(structure) == 0 {
		return nil
	}
	structureFiles, err := configRepositoryTeamStructureFiles(repo, structure)
	if err != nil {
		return err
	}
	for filePath, content := range structureFiles {
		files[filePath] = content
	}
	return nil
}

func configRepositoryTeamStructureFiles(repo models.ConfigRepository, structure map[string]*configsync.TeamStructureExportNode) (map[string]string, error) {
	files := map[string]string{}
	switch repo.ScopeType {
	case models.ConfigRepositoryScopeTeam:
		scope := strings.Trim(strings.TrimSpace(repo.ScopeID), "/")
		if scope == "" {
			return files, nil
		}
		node := configRepositoryTeamStructureNodeAtPath(structure, scope)
		if node == nil {
			return files, nil
		}
		content, err := marshalConfigRepositoryTeamStructureNode(node)
		if err != nil {
			return nil, err
		}
		files[configRepositoryTeamStructurePathForScope(scope)] = content
	default:
		for name, node := range structure {
			scope := strings.Trim(strings.TrimSpace(name), "/")
			if scope == "" {
				continue
			}
			content, err := marshalConfigRepositoryTeamStructureNode(node)
			if err != nil {
				return nil, err
			}
			files[configRepositoryTeamStructurePathForScope(scope)] = content
		}
	}
	return files, nil
}

func configRepositoryTeamStructureNodeAtPath(structure map[string]*configsync.TeamStructureExportNode, scope string) *configsync.TeamStructureExportNode {
	parts := strings.Split(strings.Trim(strings.TrimSpace(scope), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return nil
	}
	var node *configsync.TeamStructureExportNode
	children := structure
	for _, part := range parts {
		node = children[part]
		if node == nil {
			return nil
		}
		children = node.Children
	}
	return node
}

func marshalConfigRepositoryTeamStructureNode(node *configsync.TeamStructureExportNode) (string, error) {
	content, err := marshalConfigRepositoryYAML(configsync.TeamStructureExportNodeMap(node))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func configRepositoryTeamStructurePathForScope(scope string) string {
	return filepath.ToSlash(filepath.Join("config-repositories", "teams", strings.Trim(strings.TrimSpace(scope), "/"), "structure.yaml"))
}
