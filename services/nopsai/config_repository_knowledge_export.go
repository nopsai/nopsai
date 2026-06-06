package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
)

func (a *App) exportConfigRepositoryKnowledge(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, resourceAccess map[resourceAccessPlanKey]configRepositoryResourceAccessState, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
		SELECT kind, group_path, name, description, content, COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
		FROM knowledge_contexts
		ORDER BY kind ASC, group_path ASC, name ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var kind, groupPath, name, description, content, source, sourcePath string
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(&kind, &groupPath, &name, &description, &content, &source, &configRepoID, &managed, &sourcePath); err != nil {
			return err
		}
		identifier := buildKnowledgeContextIdentifier(kind, groupPath, name)
		if !configRepositoryIncludesResource(repo, groupPath, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath := strings.TrimSpace(sourcePath)
		if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && filePath != "" {
			var ok bool
			filePath, ok = configRepositoryManagedSourcePath(repo, filePath)
			if !ok {
				continue
			}
		} else {
			relGroup, ok := configRepositoryRelativeResourceIdentifier(repo, groupPath)
			if !ok {
				continue
			}
			relID := strings.Trim(strings.Trim(relGroup, "/")+"/"+strings.Trim(name, "/"), "/")
			if relID == "" {
				continue
			}
			filePath = filepath.ToSlash(filepath.Join("knowledge", kind, relID+".md"))
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
