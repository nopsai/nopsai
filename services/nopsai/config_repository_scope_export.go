package nopsai

import (
	"context"
	"database/sql"
	"strings"

	"nopsai/pkg/models"
)

type configRepositoryScopeExport struct {
	Access    *configRepositoryEmbeddedAccessFile
	Variables map[string]string
	Secrets   map[string]any
}

type configRepositoryScopeDocument struct {
	Access    *configRepositoryEmbeddedAccessFile `yaml:"access,omitempty"`
	Variables map[string]string                   `yaml:"variables,omitempty"`
	Secrets   map[string]any                      `yaml:"secrets,omitempty"`
}

func (a *App) exportConfigRepositoryScopes(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
	scopeFiles := map[string]*configRepositoryScopeExport{}
	scopeAccessAdded := map[resourceAccessPlanKey]struct{}{}
	addFile := func(filePath string) *configRepositoryScopeExport {
		if _, ok := scopeFiles[filePath]; !ok {
			scopeFiles[filePath] = &configRepositoryScopeExport{
				Variables: map[string]string{},
				Secrets:   map[string]any{},
			}
		}
		return scopeFiles[filePath]
	}
	addScopedResourceAccess := func(scope string, sourcePath string, managed bool, configRepoID sql.NullInt64) {
		key := resourceAccessPlanKey{resourceType: grantResourceScope, resourceID: runtimeScopeForResource(scope)}
		access, ok := resourceAccess[key]
		if !ok {
			return
		}
		filePath, ok := configRepositoryScopeFilePath(repo, scope, sourcePath, managed, configRepoID)
		if !ok {
			return
		}
		addFile(filePath).Access = access.exportFile()
		scopeAccessAdded[key] = struct{}{}
	}

	if err := a.exportConfigRepositoryScopeVariables(ctx, repo, delegatedScopes, addFile, addScopedResourceAccess); err != nil {
		return err
	}
	if err := a.exportConfigRepositoryScopeSecrets(ctx, repo, delegatedScopes, addFile, addScopedResourceAccess); err != nil {
		return err
	}
	for key := range resourceAccess {
		if key.resourceType != grantResourceScope {
			continue
		}
		if _, ok := scopeAccessAdded[key]; ok {
			continue
		}
		addScopedResourceAccess(runtimeScopeForDisplay(key.resourceID), "", false, sql.NullInt64{})
	}

	for filePath, payload := range scopeFiles {
		doc := configRepositoryScopeDocument{}
		if payload.Access != nil {
			doc.Access = payload.Access
		}
		if len(payload.Variables) > 0 {
			doc.Variables = payload.Variables
		}
		if len(payload.Secrets) > 0 {
			doc.Secrets = payload.Secrets
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return nil
}

func (a *App) exportConfigRepositoryScopeVariables(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, addFile func(string) *configRepositoryScopeExport, addAccess func(string, string, bool, sql.NullInt64)) error {
	rows, err := a.db.Query(ctx, `
		SELECT name, value, COALESCE(repository_name, ''), scope, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM variables
		ORDER BY scope ASC, repository_name ASC NULLS FIRST, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, value, repositoryName, scope, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&name, &value, &repositoryName, &scope, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, scope, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryScopeFilePath(repo, scope, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		key := strings.TrimSpace(name)
		if repositoryName != "" {
			key = strings.Trim(strings.TrimSpace(repositoryName)+"/"+key, "/")
		}
		addFile(filePath).Variables[key] = value
		if addAccess != nil {
			addAccess(scope, sourcePath, managed, configRepoID)
		}
	}
	return rows.Err()
}

func (a *App) exportConfigRepositoryScopeSecrets(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, addFile func(string) *configRepositoryScopeExport, addAccess func(string, string, bool, sql.NullInt64)) error {
	rows, err := a.db.Query(ctx, `
		SELECT name, value, COALESCE(repository_name, ''), scope, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM secrets
		ORDER BY scope ASC, repository_name ASC NULLS FIRST, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name, repositoryName, scope, source, sourcePath string
		var value sql.NullString
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&name, &value, &repositoryName, &scope, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, scope, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryScopeFilePath(repo, scope, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		key := strings.TrimSpace(name)
		if repositoryName != "" {
			key = strings.Trim(strings.TrimSpace(repositoryName)+"/"+key, "/")
		}
		if value.Valid && strings.TrimSpace(value.String) != "" {
			addFile(filePath).Secrets[key] = value.String
		} else {
			addFile(filePath).Secrets[key] = nil
		}
		if addAccess != nil {
			addAccess(scope, sourcePath, managed, configRepoID)
		}
	}
	return rows.Err()
}
