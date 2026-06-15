package nopsai

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

func (a *App) exportConfigRepositoryGroupStructure(ctx context.Context, repo models.ConfigRepository, files map[string]string) error {
	records, err := loadGroupPathRecords(ctx, a.db)
	if err != nil {
		return err
	}
	structure := map[string]*configsync.GroupStructureExportNode{}
	for _, record := range records {
		path := strings.Trim(strings.TrimSpace(record.Path), "/")
		if path == "" || !configsync.GroupStructureIncludesPath(repo, path) {
			continue
		}
		if record.Kind == "app" || record.RepositoryFullName != "" {
			parentPath := groupItemParentPath(path)
			if parentPath == "" || !configsync.GroupStructureIncludesPath(repo, parentPath) {
				continue
			}
			parent := configsync.EnsureGroupStructureExportPath(structure, parentPath)
			if app, ok := configsync.BuildGroupStructureAppExport(record.Name, record.RepoURL, record.RepositoryFullName); ok {
				parent.Apps = append(parent.Apps, app)
			}
			continue
		}
		node := configsync.EnsureGroupStructureExportPath(structure, path)
		node.Description = strings.TrimSpace(record.Description)
	}

	rows, err := a.db.Query(ctx, `
		SELECT scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch,
		       config_repo_id, managed_by_config_repo
		FROM config_repositories
		WHERE scope_type = $1
		  AND id <> $2
		ORDER BY scope_id ASC
	`, models.ConfigRepositoryScopeFolder, repo.ID)
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
		if scopeID == "" || !configsync.GroupStructureIncludesPath(repo, scopeID) {
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
		node := configsync.EnsureGroupStructureExportPath(structure, scopeID)
		node.Config = &configsync.GroupStructureBindingExport{
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
	structureFiles, err := configRepositoryGroupStructureFiles(repo, structure)
	if err != nil {
		return err
	}
	for filePath, content := range structureFiles {
		files[filePath] = content
	}
	return nil
}

func configRepositoryGroupStructureFiles(repo models.ConfigRepository, structure map[string]*configsync.GroupStructureExportNode) (map[string]string, error) {
	files := map[string]string{}
	switch repo.ScopeType {
	case models.ConfigRepositoryScopeFolder:
		scope := strings.Trim(strings.TrimSpace(repo.ScopeID), "/")
		if scope == "" {
			return files, nil
		}
		node := configRepositoryGroupStructureNodeAtPath(structure, scope)
		if node == nil {
			return files, nil
		}
		content, err := marshalConfigRepositoryGroupStructureNode(node)
		if err != nil {
			return nil, err
		}
		files[configRepositoryGroupStructurePathForScope(scope)] = content
	default:
		for name, node := range structure {
			scope := strings.Trim(strings.TrimSpace(name), "/")
			if scope == "" {
				continue
			}
			content, err := marshalConfigRepositoryGroupStructureNode(node)
			if err != nil {
				return nil, err
			}
			files[configRepositoryGroupStructurePathForScope(scope)] = content
		}
	}
	return files, nil
}

func configRepositoryGroupStructureNodeAtPath(structure map[string]*configsync.GroupStructureExportNode, scope string) *configsync.GroupStructureExportNode {
	parts := strings.Split(strings.Trim(strings.TrimSpace(scope), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return nil
	}
	var node *configsync.GroupStructureExportNode
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

func marshalConfigRepositoryGroupStructureNode(node *configsync.GroupStructureExportNode) (string, error) {
	content, err := marshalConfigRepositoryYAML(configsync.GroupStructureExportNodeMap(node))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func configRepositoryGroupStructurePathForScope(scope string) string {
	return filepath.ToSlash(filepath.Join("config-repositories", "groups", strings.Trim(strings.TrimSpace(scope), "/"), "structure.yaml"))
}
