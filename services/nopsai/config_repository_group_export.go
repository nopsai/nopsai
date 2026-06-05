package main

import (
	"context"
	"database/sql"
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
	content, err := marshalConfigRepositoryYAML(configsync.GroupStructureExportMap(structure))
	if err != nil {
		return err
	}
	files[configRepositoryGroupStructurePath] = string(content)
	return nil
}
