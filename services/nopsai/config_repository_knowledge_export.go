package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
)

func (a *App) exportConfigRepositoryKnowledge(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
			SELECT kind, team_path, name, description, content, COALESCE(source, 'database'), content_source, config_repo_id, managed_by_config_repo, config_source_path
			FROM knowledge_contexts
			ORDER BY kind ASC, team_path ASC, name ASC
		`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var kind, teamPath, name, description, content, source, contentSource, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&kind, &teamPath, &name, &description, &content, &source, &contentSource, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		if contentSource == "external_page" {
			continue
		}
		identifier := buildKnowledgeContextIdentifier(kind, teamPath, name)
		if !configRepositoryIncludesResource(repo, teamPath, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryKnowledgeExportPath(repo, kind, teamPath, name, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		if !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		var access *configRepositoryEmbeddedAccessFile
		if currentAccess, ok := resourceAccess[resourceAccessPlanKey{resourceType: grantResourceKnowledgeContext, resourceID: identifier}]; ok {
			access = currentAccess.exportFile()
		}
		files[filePath] = renderKnowledgeContextGitOpsDocument(kind, name, description, content, identifier, access)
	}
	return rows.Err()
}

func configRepositoryKnowledgeExportPath(repo models.ConfigRepository, kind, teamPath, name, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	if _, ok := configRepositoryRelativeResourceIdentifier(repo, teamPath); !ok {
		return "", false
	}
	normalizedTeam := strings.Trim(strings.TrimSpace(teamPath), "/")
	if normalizedTeam == "" {
		return "", false
	}
	relID := strings.Trim(normalizedTeam+"/"+strings.Trim(name, "/"), "/")
	if relID == "" {
		return "", false
	}
	canonicalPath := filepath.ToSlash(filepath.Join("knowledge", kind, relID+".md"))
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		if managedPath, ok := configsync.ManagedSourcePathForCanonical(repo, sourcePath, canonicalPath, configRepositoryDriftPathOptions()); ok {
			return managedPath, true
		}
	}
	return canonicalPath, true
}

type configRepositoryKnowledgeDocument struct {
	Name        string                              `yaml:"name"`
	Kind        string                              `yaml:"kind"`
	Description string                              `yaml:"description,omitempty"`
	Access      *configRepositoryEmbeddedAccessFile `yaml:"access,omitempty"`
	Content     string                              `yaml:"content"`
}

func renderKnowledgeContextGitOpsDocument(kind, name, description, content, fallbackName string, access *configRepositoryEmbeddedAccessFile) string {
	doc := configRepositoryKnowledgeDocument{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(description),
		Access:      access,
		Content:     content,
	}
	if strings.TrimSpace(doc.Name) == "" {
		doc.Name = fallbackName
	}
	encoded, err := marshalConfigRepositoryYAML(doc)
	if err != nil {
		return fmt.Sprintf("---\nname: %s\nkind: %s\ncontent: |\n%s\n---\n", name, kind, indentConfigRepositoryBlock(content))
	}
	return "---\n" + string(encoded) + "---\n"
}

func indentConfigRepositoryBlock(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
