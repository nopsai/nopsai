package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"
	"nopsai/services/nopsai/internal/credentials"

	"gopkg.in/yaml.v3"
)

// knowledgeConnectionsGitOpsDirectory is the reserved segment inside the
// knowledge directory that owns Notion, Confluence, and wiki connections. It is
// not a knowledge kind, so connection files and knowledge documents never
// compete for the same path.
const knowledgeConnectionsGitOpsDirectory = "connections"

// knowledgeConnectionGitOpsFile is the reviewable definition of a knowledge
// connection. Runtime state such as reachability status, last error, last check
// time, and document counts stays in the database and is never exported.
type knowledgeConnectionGitOpsFile struct {
	Name          string         `json:"name" yaml:"name"`
	DisplayName   string         `json:"display_name,omitempty" yaml:"display_name,omitempty"`
	Provider      string         `json:"provider" yaml:"provider"`
	BaseURL       string         `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	CredentialRef string         `json:"credential_ref,omitempty" yaml:"credential_ref,omitempty"`
	Disabled      *bool          `json:"disabled,omitempty" yaml:"disabled,omitempty"`
	Scopes        map[string]any `json:"scopes,omitempty" yaml:"scopes,omitempty"`
	Config        map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

type storedKnowledgeConnection struct {
	team          string
	name          string
	displayName   string
	provider      string
	baseURL       string
	credentialRef string
	disabled      bool
	scopes        map[string]any
	config        map[string]any
	sourcePath    string
}

func isKnowledgeConnectionGitOpsPath(rel string) bool {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	parts := strings.Split(rel, "/")
	if len(parts) < 2 || !strings.EqualFold(parts[0], knowledgeConnectionsGitOpsDirectory) {
		return false
	}
	switch strings.ToLower(filepath.Ext(parts[len(parts)-1])) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func parseKnowledgeConnectionGitOpsPath(rel string, binding models.ConfigRepository, boundTeam string) (string, string, error) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	parts := strings.Split(rel, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("knowledge connection path must use connections/team/name or connections/name in a team repository")
	}
	name, err := normalizeKnowledgeConnectionName(strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(parts[len(parts)-1])))
	if err != nil {
		return "", "", err
	}
	team := strings.Trim(strings.Join(parts[1:len(parts)-1], "/"), "/")
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		team, err = configsync.NormalizePathForTeam(boundTeam, team)
		if err != nil {
			return "", "", err
		}
	}
	team, err = normalizeKnowledgeConnectionTeam(team)
	if err != nil {
		return "", "", err
	}
	return team, name, nil
}

func parseGitOpsKnowledgeConnections(files map[string]string, root string, binding models.ConfigRepository, boundTeam string) (map[string]storedKnowledgeConnection, error) {
	connections := make(map[string]storedKnowledgeConnection)
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, root)
		if !ok || rel == "" || strings.HasSuffix(rel, "/") || !isKnowledgeConnectionGitOpsPath(rel) {
			continue
		}
		team, name, err := parseKnowledgeConnectionGitOpsPath(rel, binding, boundTeam)
		if err != nil {
			return nil, fmt.Errorf("invalid knowledge connection path '%s': %w", normalized, err)
		}
		var file knowledgeConnectionGitOpsFile
		if err := yaml.Unmarshal([]byte(content), &file); err != nil {
			return nil, fmt.Errorf("failed to parse knowledge connection '%s': %w", normalized, err)
		}
		if declared := strings.TrimSpace(file.Name); declared != "" {
			declaredName, err := normalizeKnowledgeConnectionName(declared)
			if err != nil {
				return nil, fmt.Errorf("invalid knowledge connection name in '%s': %w", normalized, err)
			}
			if declaredName != name {
				return nil, fmt.Errorf("knowledge connection '%s' declares name %q but the file name implies %q", normalized, declaredName, name)
			}
		}
		provider, err := normalizeKnowledgeConnectionProvider(file.Provider)
		if err != nil {
			return nil, fmt.Errorf("invalid knowledge connection provider in '%s': %w", normalized, err)
		}
		credentialRef := strings.TrimSpace(file.CredentialRef)
		if credentialRef != "" {
			if _, err := credentials.ParseReference(credentialRef); err != nil {
				return nil, fmt.Errorf("invalid knowledge connection credential reference in '%s': %w", normalized, err)
			}
		}
		key := buildKnowledgeConnectionIdentifier(team, name)
		if _, exists := connections[key]; exists {
			return nil, fmt.Errorf("duplicate knowledge connection '%s' detected in config repository", key)
		}
		disabled := false
		if file.Disabled != nil {
			disabled = *file.Disabled
		}
		connections[key] = storedKnowledgeConnection{
			team:          team,
			name:          name,
			displayName:   strings.TrimSpace(file.DisplayName),
			provider:      provider,
			baseURL:       strings.TrimSpace(file.BaseURL),
			credentialRef: credentialRef,
			disabled:      disabled,
			scopes:        mapOrEmpty(file.Scopes),
			config:        mapOrEmpty(file.Config),
			sourcePath:    normalized,
		}
	}
	return connections, nil
}

func (a *App) exportConfigRepositoryKnowledgeConnections(ctx context.Context, repo models.ConfigRepository, delegatedScopes []string, files map[string]string) error {
	rows, err := a.db.Query(ctx, `
			SELECT team_path, name, display_name, provider, base_url, credential_ref, disabled, scopes, config,
			       COALESCE(source, 'database'), config_repo_id, managed_by_config_repo, config_source_path
			FROM knowledge_context_connections
			ORDER BY team_path ASC, name ASC
		`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var teamPath, name, displayName, provider, baseURL, credentialRef, source, sourcePath string
		var disabled, managed bool
		var configRepoID sql.NullInt64
		var scopesJSON, configJSON []byte
		if err := rows.Scan(
			&teamPath, &name, &displayName, &provider, &baseURL, &credentialRef, &disabled, &scopesJSON, &configJSON,
			&source, &configRepoID, &managed, &sourcePath,
		); err != nil {
			return err
		}
		if !configRepositoryIncludesResource(repo, teamPath, source, configRepoID, managed, delegatedScopes) {
			continue
		}
		filePath, ok := configRepositoryKnowledgeConnectionExportPath(repo, teamPath, name, sourcePath, managed, configRepoID)
		if !ok {
			continue
		}
		if !isConfigRepositoryDriftPath(filePath) {
			continue
		}
		doc := knowledgeConnectionGitOpsFile{
			Name:          name,
			DisplayName:   strings.TrimSpace(displayName),
			Provider:      provider,
			BaseURL:       strings.TrimSpace(baseURL),
			CredentialRef: strings.TrimSpace(credentialRef),
			Scopes:        knowledgeConnectionExportMap(scopesJSON),
			Config:        knowledgeConnectionExportMap(configJSON),
		}
		if disabled {
			doc.Disabled = boolPtr(true)
		}
		content, err := marshalConfigRepositoryYAML(doc)
		if err != nil {
			return err
		}
		files[filePath] = string(content)
	}
	return rows.Err()
}

func configRepositoryKnowledgeConnectionExportPath(repo models.ConfigRepository, teamPath, name, sourcePath string, managed bool, configRepoID sql.NullInt64) (string, bool) {
	if _, ok := configRepositoryRelativeResourceIdentifier(repo, teamPath); !ok {
		return "", false
	}
	normalizedTeam := strings.Trim(strings.TrimSpace(teamPath), "/")
	normalizedName := strings.Trim(strings.TrimSpace(name), "/")
	if normalizedTeam == "" || normalizedName == "" {
		return "", false
	}
	canonicalPath := filepath.ToSlash(filepath.Join("knowledge", knowledgeConnectionsGitOpsDirectory, normalizedTeam, normalizedName+".yaml"))
	if managed && configRepoID.Valid && configRepoID.Int64 == repo.ID && strings.TrimSpace(sourcePath) != "" {
		if managedPath, ok := configsync.ManagedSourcePathForCanonical(repo, sourcePath, canonicalPath, configRepositoryDriftPathOptions()); ok {
			return managedPath, true
		}
	}
	return canonicalPath, true
}

// knowledgeConnectionExportMap keeps empty provider scope and config objects out
// of the exported file so an unset value does not read as a deliberate one.
func knowledgeConnectionExportMap(raw []byte) map[string]any {
	decoded := decodeKnowledgeConnectionJSONMap(raw)
	if len(decoded) == 0 {
		return nil
	}
	return decoded
}
