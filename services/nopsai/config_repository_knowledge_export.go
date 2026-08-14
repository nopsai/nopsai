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
			SELECT k.kind, k.team_path, k.name, k.description, k.content, COALESCE(k.source, 'database'), k.content_source,
			       k.config_repo_id, k.managed_by_config_repo, k.config_source_path,
			       COALESCE(c.name, ''), COALESCE(k.external_provider, ''), COALESCE(k.external_page_id, ''),
			       COALESCE(k.external_page_url, ''), COALESCE(k.external_page_title, ''),
			       COALESCE(k.sync_mode, 'manual'), COALESCE(k.sync_interval_minutes, 0), COALESCE(k.sync_failure_mode, 'fail')
			FROM knowledge_contexts k
			LEFT JOIN knowledge_context_connections c ON c.id = k.connection_id
			ORDER BY k.kind ASC, k.team_path ASC, k.name ASC
		`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var kind, teamPath, name, description, content, source, contentSource, sourcePath string
		var connectionName, externalProvider, externalPageID, externalPageURL, externalPageTitle string
		var syncMode, syncFailureMode string
		var syncIntervalMinutes int
		var configRepoID sql.NullInt64
		var managed bool
		if err := rows.Scan(
			&kind, &teamPath, &name, &description, &content, &source, &contentSource, &configRepoID, &managed, &sourcePath,
			&connectionName, &externalProvider, &externalPageID, &externalPageURL, &externalPageTitle,
			&syncMode, &syncIntervalMinutes, &syncFailureMode,
		); err != nil {
			return err
		}
		var external *knowledgeSourceFrontMatter
		if contentSource == knowledgeContentSourceExternalPage {
			// A connected page is still Git-owned configuration: Git decides
			// which page is attached and how it syncs. Only the mirrored page
			// body stays out of Git so an upstream edit is not config drift.
			if strings.TrimSpace(connectionName) == "" {
				continue
			}
			external = &knowledgeSourceFrontMatter{
				Type:       knowledgeContentSourceExternalPage,
				Connection: connectionName,
				Provider:   externalProvider,
				PageID:     externalPageID,
				PageURL:    externalPageURL,
				PageTitle:  externalPageTitle,
				Sync: &knowledgeSyncFrontMatter{
					Mode:            syncMode,
					IntervalMinutes: syncIntervalMinutes,
					FailureMode:     syncFailureMode,
				},
			}
			content = ""
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
		files[filePath] = renderKnowledgeContextGitOpsDocument(kind, name, description, content, identifier, access, external)
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
	Source      *knowledgeSourceFrontMatter         `yaml:"source,omitempty"`
	Content     string                              `yaml:"content,omitempty"`
}

func renderKnowledgeContextGitOpsDocument(
	kind, name, description, content, fallbackName string,
	access *configRepositoryEmbeddedAccessFile,
	external *knowledgeSourceFrontMatter,
) string {
	doc := configRepositoryKnowledgeDocument{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(description),
		Access:      access,
		Source:      external,
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
