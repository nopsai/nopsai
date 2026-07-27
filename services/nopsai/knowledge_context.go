package nopsai

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

const (
	knowledgeSourceDatabase = "database"
	knowledgeSourceGitOps   = "git"
	knowledgeSourceRepo     = "repo"
)

var supportedKnowledgeContextKinds = map[string]struct{}{
	"architecture": {},
	"guardrail":    {},
	"policy":       {},
	"adr":          {},
	"guideline":    {},
	"runbook":      {},
	"reference":    {},
	"example":      {},
}

type storedKnowledgeContext struct {
	kind        string
	team        string
	name        string
	description string
	content     string
	sourcePath  string
}

type knowledgeContextListItem struct {
	ID                string     `json:"id"`
	UUID              string     `json:"uuid,omitempty"`
	Kind              string     `json:"kind"`
	Team              string     `json:"team"`
	Name              string     `json:"name"`
	Description       string     `json:"description,omitempty"`
	Visibility        string     `json:"visibility"`
	Source            string     `json:"source"`
	UpdatedAt         time.Time  `json:"updated_at"`
	Access            string     `json:"access"`
	UsedByCount       int        `json:"used_by_count"`
	UsedBy            []string   `json:"used_by,omitempty"`
	GitOpsPath        string     `json:"config_source_path,omitempty"`
	GitOpsCommit      string     `json:"config_source_commit_sha,omitempty"`
	ConnectionID      string     `json:"connection_id,omitempty"`
	ConnectionRef     string     `json:"connection_ref,omitempty"`
	ExternalProvider  string     `json:"external_provider,omitempty"`
	ExternalPageID    string     `json:"external_page_id,omitempty"`
	ExternalPageURL   string     `json:"external_page_url,omitempty"`
	ExternalPageTitle string     `json:"external_page_title,omitempty"`
	SyncMode          string     `json:"sync_mode,omitempty"`
	SyncInterval      int        `json:"sync_interval_minutes,omitempty"`
	FailureMode       string     `json:"failure_mode,omitempty"`
	SyncStatus        string     `json:"sync_status,omitempty"`
	LastSyncedAt      *time.Time `json:"last_synced_at,omitempty"`
	SyncError         string     `json:"sync_error,omitempty"`
	SourceModifiedAt  *time.Time `json:"source_modified_at,omitempty"`
	ContentHash       string     `json:"content_hash,omitempty"`
}

type knowledgeContextDetail struct {
	knowledgeContextListItem
	Content      string                               `json:"content"`
	ManagedByGit bool                                 `json:"managed_by_config_repo"`
	Connection   *knowledgeContextConnectionSummary   `json:"connection,omitempty"`
	ExternalPage *knowledgeContextExternalPageSummary `json:"external_page,omitempty"`
	Sync         *knowledgeContextSyncSummary         `json:"sync,omitempty"`
	Assets       []knowledgeContextAssetSummary       `json:"assets,omitempty"`
}

type knowledgeContextConnectionSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type knowledgeContextExternalPageSummary struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type knowledgeContextSyncSummary struct {
	Mode         string     `json:"mode"`
	IntervalMins int        `json:"interval_minutes,omitempty"`
	FailureMode  string     `json:"failure_mode"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	Status       string     `json:"status"`
	Error        *string    `json:"error"`
}

type upsertKnowledgeContextRequest struct {
	Kind              string `json:"kind"`
	Team              string `json:"team"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	Content           string `json:"content"`
	ContentSource     string `json:"content_source"`
	ConnectionID      string `json:"connection_id"`
	ExternalPageID    string `json:"external_page_id"`
	ExternalPageURL   string `json:"external_page_url"`
	ExternalPageTitle string `json:"external_page_title"`
	SyncMode          string `json:"sync_mode"`
	SyncInterval      int    `json:"sync_interval_minutes"`
	FailureMode       string `json:"failure_mode"`
}

type knowledgeFrontMatter struct {
	Name        string                      `yaml:"name"`
	Title       string                      `yaml:"title"`
	Kind        string                      `yaml:"kind"`
	Description string                      `yaml:"description"`
	Visibility  string                      `yaml:"visibility"`
	Access      *embeddedResourceAccessFile `yaml:"access"`
	Content     string                      `yaml:"content"`
}

func normalizeKnowledgeContextKind(raw string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(raw))
	if kind == "" {
		return "", fmt.Errorf("kind is required")
	}
	if _, ok := supportedKnowledgeContextKinds[kind]; !ok {
		return "", fmt.Errorf("unsupported knowledge context kind %q", raw)
	}
	return kind, nil
}

func normalizeKnowledgeContextName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	name = strings.TrimSuffix(name, ".yaml")
	name = strings.TrimSuffix(name, ".yml")
	name = strings.TrimSuffix(name, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	name = strings.Trim(name, "/")
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return "", fmt.Errorf("name contains invalid path segments")
	}
	return name, nil
}

func normalizeKnowledgeContextTeam(raw string) (string, error) {
	team := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	if team == "." {
		return "", nil
	}
	if team == "" {
		return "", nil
	}
	for _, segment := range strings.Split(team, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("team contains invalid path segments")
		}
	}
	return team, nil
}

func buildKnowledgeContextIdentifier(kind, team, name string) string {
	parts := []string{strings.Trim(strings.TrimSpace(kind), "/")}
	if team = strings.Trim(strings.TrimSpace(team), "/"); team != "" {
		parts = append(parts, team)
	}
	parts = append(parts, strings.Trim(strings.TrimSpace(name), "/"))
	return strings.Join(parts, "/")
}

func splitKnowledgeContextIdentifier(raw string) (string, string, string, error) {
	value := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("knowledge context id must use kind/team/name")
	}
	kind, err := normalizeKnowledgeContextKind(parts[0])
	if err != nil {
		return "", "", "", err
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	team, err := normalizeKnowledgeContextTeam(strings.Join(parts[1:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	if team == "" {
		return "", "", "", fmt.Errorf("knowledge context id must include a team")
	}
	return kind, team, name, nil
}

func knowledgeContextRefToParts(kind, ref string) (string, string, string, error) {
	kind, err := normalizeKnowledgeContextKind(kind)
	if err != nil {
		return "", "", "", err
	}
	ref = strings.Trim(strings.TrimSpace(strings.ReplaceAll(ref, "\\", "/")), "/")
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("knowledge context ref must use team/document")
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	team, err := normalizeKnowledgeContextTeam(strings.Join(parts[:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	if team == "" {
		return "", "", "", fmt.Errorf("knowledge context ref must include a team")
	}
	return kind, team, name, nil
}

func normalizeKnowledgeContextPath(raw string) (string, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if filepath.IsAbs(value) || strings.HasPrefix(value, "~") {
		return "", fmt.Errorf("path must be relative")
	}
	value = strings.Trim(value, "/")
	if value == "" || value == "." {
		return "", fmt.Errorf("path is required")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("path contains invalid path segments")
		}
	}
	return value, nil
}

func isKnowledgeContextGitOpsFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".md", ".markdown":
		return true
	default:
		return false
	}
}

func parseKnowledgeContextGitOpsPath(rel string, binding models.ConfigRepository, boundTeam string) (string, string, string, error) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	parts := strings.Split(rel, "/")
	minParts := 3
	if binding.ScopeType == models.ConfigRepositoryScopeTeam {
		minParts = 2
	}
	if len(parts) < minParts {
		return "", "", "", fmt.Errorf("knowledge document path must use kind/team/document")
	}
	kind, err := normalizeKnowledgeContextKind(parts[0])
	if err != nil {
		return "", "", "", err
	}
	if !isKnowledgeContextGitOpsFile(parts[len(parts)-1]) {
		return "", "", "", fmt.Errorf("knowledge document must be a YAML or Markdown file")
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	var team string
	if binding.ScopeType == models.ConfigRepositoryScopeTeam && len(parts) == 2 {
		team, err = configsync.NormalizePathForTeam(boundTeam, "")
		if err != nil {
			return "", "", "", err
		}
	} else {
		team, err = normalizeKnowledgeContextTeam(strings.Join(parts[1:len(parts)-1], "/"))
		if err != nil {
			return "", "", "", err
		}
	}
	if binding.ScopeType == models.ConfigRepositoryScopeTeam && len(parts) > 2 {
		if team == "" {
			team = boundTeam
		} else {
			team, err = configsync.NormalizePathForTeam(boundTeam, team)
			if err != nil {
				return "", "", "", err
			}
		}
	}
	if team == "" {
		return "", "", "", fmt.Errorf("knowledge document path must include a team")
	}
	return kind, team, name, nil
}

func parseKnowledgeContextDocument(content string) (knowledgeFrontMatter, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")

	if doc, body, ok, err := parseMarkdownKnowledgeContextDocument(normalized); ok || err != nil {
		return doc, body, err
	}

	contentIndex := topLevelYAMLKeyIndex(normalized, "content")
	if contentIndex < 0 {
		return knowledgeFrontMatter{}, "", fmt.Errorf("content is required")
	}

	doc, body, err := parseLooseKnowledgeContextDocument(normalized, contentIndex)
	if err != nil {
		return doc, "", err
	}
	return doc, body, nil
}

func parseMarkdownKnowledgeContextDocument(content string) (knowledgeFrontMatter, string, bool, error) {
	if !strings.HasPrefix(content, "---\n") {
		return knowledgeFrontMatter{}, "", false, nil
	}

	headerStart := len("---\n")
	offset := headerStart
	for _, line := range strings.SplitAfter(content[headerStart:], "\n") {
		if strings.TrimSpace(strings.TrimRight(line, "\n")) == "---" {
			header := content[headerStart:offset]
			body := content[offset+len(line):]
			var doc knowledgeFrontMatter
			if err := yaml.Unmarshal([]byte(header), &doc); err != nil {
				sanitized := quoteLooseKnowledgeScalars(header)
				if sanitized == header {
					return doc, "", true, err
				}
				if retryErr := yaml.Unmarshal([]byte(sanitized), &doc); retryErr != nil {
					return doc, "", true, retryErr
				}
			}
			if strings.TrimSpace(doc.Content) == "" {
				return doc, "", true, fmt.Errorf("content is required")
			}
			if strings.TrimSpace(body) != "" {
				return doc, "", true, fmt.Errorf("markdown body outside content field is not supported; use content")
			}
			return doc, doc.Content, true, nil
		}
		offset += len(line)
	}

	return knowledgeFrontMatter{}, "", true, fmt.Errorf("front matter is not closed")
}

func topLevelYAMLKeyIndex(content, key string) int {
	prefix := key + ":"
	offset := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		trimmedLine := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(trimmedLine, prefix) {
			if len(trimmedLine) == len(prefix) || trimmedLine[len(prefix)] == ' ' || trimmedLine[len(prefix)] == '\t' {
				return offset
			}
		}
		offset += len(line)
	}
	return -1
}

func parseLooseKnowledgeContextDocument(content string, contentIndex int) (knowledgeFrontMatter, string, error) {
	var doc knowledgeFrontMatter
	header := strings.TrimSpace(content[:contentIndex])
	if header != "" {
		if err := yaml.Unmarshal([]byte(header), &doc); err != nil {
			sanitized := quoteLooseKnowledgeScalars(header)
			if sanitized == header {
				return doc, "", err
			}
			if retryErr := yaml.Unmarshal([]byte(sanitized), &doc); retryErr != nil {
				return doc, "", retryErr
			}
		}
	}

	contentBlock := content[contentIndex:]
	lineEnd := strings.IndexByte(contentBlock, '\n')
	if lineEnd < 0 {
		var inline struct {
			Content string `yaml:"content"`
		}
		if err := yaml.Unmarshal([]byte(contentBlock), &inline); err != nil {
			return doc, "", err
		}
		doc.Content = inline.Content
		return doc, inline.Content, nil
	}

	contentLine := strings.TrimSpace(contentBlock[:lineEnd])
	rawBody := contentBlock[lineEnd+1:]
	suffix := strings.TrimSpace(strings.TrimPrefix(contentLine, "content:"))
	if suffix != "" && !strings.HasPrefix(suffix, "|") && !strings.HasPrefix(suffix, ">") {
		var inline struct {
			Content string `yaml:"content"`
		}
		if err := yaml.Unmarshal([]byte(contentBlock[:lineEnd]), &inline); err != nil {
			return doc, "", err
		}
		doc.Content = inline.Content
		return doc, inline.Content, nil
	}

	body := removeCommonIndent(rawBody)
	doc.Content = body
	return doc, body, nil
}

func quoteLooseKnowledgeScalars(content string) string {
	scalarKeys := map[string]struct{}{
		"name":        {},
		"kind":        {},
		"description": {},
		"visibility":  {},
	}
	lines := strings.Split(content, "\n")
	changed := false
	for i, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if _, supported := scalarKeys[key]; !supported {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" || !strings.Contains(value, ":") || strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
			continue
		}
		lines[i] = key + ": " + strconv.Quote(value)
		changed = true
	}
	if !changed {
		return content
	}
	return strings.Join(lines, "\n")
}

func removeCommonIndent(content string) string {
	lines := strings.Split(content, "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := 0
		for indent < len(line) && (line[indent] == ' ' || line[indent] == '\t') {
			indent++
		}
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return content
	}
	for i, line := range lines {
		cut := 0
		for cut < len(line) && cut < minIndent && (line[cut] == ' ' || line[cut] == '\t') {
			cut++
		}
		lines[i] = line[cut:]
	}
	return strings.Join(lines, "\n")
}

func parseGitOpsKnowledgeContexts(files map[string]string, root string, binding models.ConfigRepository, boundTeam string, accessPlan accessSyncPlan) (map[string]storedKnowledgeContext, error) {
	contexts := make(map[string]storedKnowledgeContext)
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := configsync.RelativePath(normalized, root)
		if !ok || rel == "" || strings.HasSuffix(rel, "/") || !isKnowledgeContextGitOpsFile(rel) {
			continue
		}

		kind, team, _, err := parseKnowledgeContextGitOpsPath(rel, binding, boundTeam)
		if err != nil {
			return nil, fmt.Errorf("invalid knowledge context path '%s': %w", normalized, err)
		}
		frontMatter, body, err := parseKnowledgeContextDocument(content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse knowledge context document '%s': %w", normalized, err)
		}
		if declaredKind := strings.TrimSpace(frontMatter.Kind); declaredKind != "" {
			normalizedKind, err := normalizeKnowledgeContextKind(declaredKind)
			if err != nil {
				return nil, fmt.Errorf("invalid knowledge context kind in '%s': %w", normalized, err)
			}
			if normalizedKind != kind {
				return nil, fmt.Errorf("knowledge context '%s' declares kind %q but path implies %q", normalized, normalizedKind, kind)
			}
		}
		declaredName := strings.TrimSpace(frontMatter.Name)
		if declaredName == "" {
			return nil, fmt.Errorf("knowledge context '%s' must declare name", normalized)
		}
		name, err := normalizeKnowledgeContextName(declaredName)
		if err != nil {
			return nil, fmt.Errorf("invalid knowledge context name in '%s': %w", normalized, err)
		}
		if strings.TrimSpace(frontMatter.Title) != "" {
			return nil, fmt.Errorf("knowledge context '%s' must not declare title; use a heading inside content instead", normalized)
		}
		if strings.TrimSpace(frontMatter.Visibility) != "" {
			return nil, fmt.Errorf("knowledge context '%s' must not declare visibility; use access.visibility instead", normalized)
		}

		visibility := resourceVisibilityTeam
		if frontMatter.Access != nil {
			rawGrants := embeddedResourceAccessGrants(*frontMatter.Access)
			visibility = firstNonEmptyString(frontMatter.Access.Visibility, embeddedResourceUseAccessMode(frontMatter.Access.UseAccess))
			if visibility == "" && len(rawGrants) > 0 {
				visibility = resourceVisibilityRestricted
			}
			if visibility == "" {
				visibility = resourceVisibilityTeam
			}
		}
		normalizedVisibility, err := normalizeResourceVisibilityUpdate(visibility)
		if err != nil {
			return nil, fmt.Errorf("invalid knowledge context visibility in '%s': %w", normalized, err)
		}
		if err := validateResourceVisibilityPolicy(grantResourceKnowledgeContext, normalizedVisibility); err != nil {
			return nil, fmt.Errorf("invalid knowledge context visibility in '%s': %w", normalized, err)
		}
		key := buildKnowledgeContextIdentifier(kind, team, name)
		if _, exists := contexts[key]; exists {
			return nil, fmt.Errorf("duplicate knowledge context '%s' detected in config repository", key)
		}

		if err := addKnowledgeContextEmbeddedAccess(accessPlan, frontMatter, normalized, key, binding, boundTeam); err != nil {
			return nil, fmt.Errorf("invalid knowledge context access '%s': %w", normalized, err)
		}
		contexts[key] = storedKnowledgeContext{
			kind:        kind,
			team:        team,
			name:        name,
			description: strings.TrimSpace(frontMatter.Description),
			content:     strings.TrimSpace(body),
			sourcePath:  normalized,
		}
	}
	return contexts, nil
}

func addKnowledgeContextEmbeddedAccess(plan accessSyncPlan, fm knowledgeFrontMatter, sourcePath, resourceID string, binding models.ConfigRepository, boundTeam string) error {
	if fm.Access == nil {
		return nil
	}
	access := embeddedResourceAccessFile{}
	if fm.Access != nil {
		access = *fm.Access
	}
	docBytes, err := yaml.Marshal(embeddedResourceAccessDocument{Access: &access})
	if err != nil {
		return err
	}
	return plan.addEmbeddedResourceAccess(string(docBytes), sourcePath, grantResourceKnowledgeContext, resourceID, binding, boundTeam)
}

func (a *App) handleListKnowledgeContexts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		SELECT k.id::text, k.kind, k.team_path, k.name, k.description, k.source,
		       k.managed_by_config_repo, k.config_source_path, k.config_source_commit_sha, k.updated_at,
		       k.connection_id::text, c.team_path, c.name, k.external_provider, k.external_page_id, k.external_page_url,
		       k.external_page_title, k.sync_mode, k.sync_interval_minutes, k.failure_mode, k.sync_status, k.last_synced_at, k.sync_error,
		       k.source_modified_at, k.content_hash
		FROM knowledge_contexts k
		LEFT JOIN knowledge_context_connections c ON c.id = k.connection_id
		ORDER BY k.kind ASC, k.team_path ASC, k.name ASC
	`)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query knowledge contexts")
		http.Error(w, "Failed to retrieve knowledge contexts", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	usage := a.knowledgeContextUsage(r.Context())
	var items []knowledgeContextListItem
	var resources []aaamodel.ResourceRef
	for rows.Next() {
		var item knowledgeContextListItem
		var managed bool
		var connectionID sql.NullString
		var connectionTeam sql.NullString
		var connectionName sql.NullString
		var lastSyncedAt sql.NullTime
		var sourceModifiedAt sql.NullTime
		if err := rows.Scan(
			&item.UUID, &item.Kind, &item.Team, &item.Name, &item.Description, &item.Source,
			&managed, &item.GitOpsPath, &item.GitOpsCommit, &item.UpdatedAt,
			&connectionID, &connectionTeam, &connectionName, &item.ExternalProvider, &item.ExternalPageID, &item.ExternalPageURL,
			&item.ExternalPageTitle, &item.SyncMode, &item.SyncInterval, &item.FailureMode, &item.SyncStatus, &lastSyncedAt, &item.SyncError,
			&sourceModifiedAt, &item.ContentHash,
		); err != nil {
			log.Error().Err(err).Msg("Failed to scan knowledge context")
			http.Error(w, "Failed to process knowledge contexts", http.StatusInternalServerError)
			return
		}
		item.ID = buildKnowledgeContextIdentifier(item.Kind, item.Team, item.Name)
		if connectionID.Valid {
			item.ConnectionID = connectionID.String
			if connectionTeam.Valid && connectionName.Valid {
				item.ConnectionRef = buildKnowledgeConnectionIdentifier(connectionTeam.String, connectionName.String)
			}
		}
		if lastSyncedAt.Valid {
			item.LastSyncedAt = &lastSyncedAt.Time
		}
		if sourceModifiedAt.Valid {
			item.SourceModifiedAt = &sourceModifiedAt.Time
		}
		visibility, err := a.resourceVisibility(r.Context(), grantResourceKnowledgeContext, item.ID)
		if err != nil {
			log.Error().Err(err).Str("knowledge_context", item.ID).Msg("Failed to load knowledge context visibility")
			http.Error(w, "Failed to process knowledge contexts", http.StatusInternalServerError)
			return
		}
		item.Visibility = visibility
		if managed {
			item.Source = knowledgeSourceGitOps
		}
		item.Access = item.Visibility
		item.UsedBy = usage[item.ID]
		item.UsedByCount = len(item.UsedBy)
		items = append(items, item)
		resources = append(resources, aaamodel.ResourceRef{Type: grantResourceKnowledgeContext, ID: item.ID})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to process knowledge contexts", http.StatusInternalServerError)
		return
	}

	allowedSet, err := a.allowedResourceSet(r, "knowledge_context.read", resources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	filtered := make([]knowledgeContextListItem, 0, len(items))
	for _, item := range items {
		if _, ok := allowedSet[resourceKey(aaamodel.ResourceRef{Type: grantResourceKnowledgeContext, ID: item.ID})]; ok {
			filtered = append(filtered, item)
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (a *App) handleGetKnowledgeContext(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("knowledgeID")
	kind, team, name, err := splitKnowledgeContextIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	detail, err := a.loadKnowledgeContextDetail(r.Context(), kind, team, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "knowledge context not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("knowledge_context", identifier).Msg("Failed to load knowledge context")
		http.Error(w, "Failed to load knowledge context", http.StatusInternalServerError)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_context.read", aaamodel.ResourceRef{Type: grantResourceKnowledgeContext, ID: detail.ID}) {
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *App) handleUpsertKnowledgeContext(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("knowledgeID")
	pathKind, pathTeam, pathName, err := splitKnowledgeContextIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req upsertKnowledgeContextRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	kind := firstNonEmptyString(req.Kind, pathKind)
	team := firstNonEmptyString(req.Team, pathTeam)
	name := firstNonEmptyString(req.Name, pathName)
	kind, err = normalizeKnowledgeContextKind(kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	team, err = normalizeKnowledgeContextTeam(team)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name, err = normalizeKnowledgeContextName(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if kind != pathKind || team != pathTeam || name != pathName {
		http.Error(w, "request body kind, team, and name must match the URL", http.StatusBadRequest)
		return
	}
	resourceID := buildKnowledgeContextIdentifier(kind, team, name)
	var exists int
	lookupErr := a.db.QueryRow(r.Context(), `
		SELECT 1
		FROM knowledge_contexts
		WHERE kind = $1 AND team_path = $2 AND name = $3
		LIMIT 1
	`, kind, team, name).Scan(&exists)
	if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) && !errors.Is(lookupErr, sql.ErrNoRows) {
		log.Error().Err(lookupErr).Str("knowledge_context", resourceID).Msg("Failed to inspect existing knowledge context")
		http.Error(w, "Failed to save knowledge context", http.StatusInternalServerError)
		return
	}
	action := "knowledge_context.update"
	if errors.Is(lookupErr, pgx.ErrNoRows) || errors.Is(lookupErr, sql.ErrNoRows) {
		action = "knowledge_context.create"
	}
	if !a.requireAAADecision(w, r, action, aaamodel.ResourceRef{Type: grantResourceKnowledgeContext, ID: resourceID}) {
		return
	}

	source := knowledgeSourceDatabase
	var connectionUUID any
	externalProvider := ""
	externalPageID := ""
	externalPageURL := ""
	externalPageTitle := ""
	syncMode := knowledgeSyncModeManual
	syncInterval := 0
	failureMode := knowledgeFailureModeFail
	syncStatus := "not_synced"
	contentSource := "inline"
	contentHash := ""
	var lastSyncedAt any
	isExternal := strings.EqualFold(strings.TrimSpace(req.ContentSource), "external") ||
		strings.EqualFold(strings.TrimSpace(req.ContentSource), "external_page") ||
		strings.TrimSpace(req.ConnectionID) != "" ||
		strings.TrimSpace(req.ExternalPageID) != "" ||
		strings.TrimSpace(req.ExternalPageURL) != ""
	if isExternal {
		connection, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), req.ConnectionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "knowledge connection not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if connection.Team != team {
			http.Error(w, "external page connection must belong to the same team as the knowledge context", http.StatusBadRequest)
			return
		}
		if !a.requireAAADecision(w, r, "knowledge_connection.use", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: connection.ID}) {
			return
		}
		externalPageID = strings.TrimSpace(req.ExternalPageID)
		externalPageURL = strings.TrimSpace(req.ExternalPageURL)
		if externalPageID == "" && externalPageURL == "" {
			http.Error(w, "external_page_id or external_page_url is required", http.StatusBadRequest)
			return
		}
		syncMode, err = normalizeKnowledgeSyncMode(req.SyncMode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		syncInterval, err = normalizeKnowledgeSyncIntervalMinutes(req.SyncInterval, syncMode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		failureMode, err = normalizeKnowledgeFailureMode(req.FailureMode)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if failureMode == knowledgeFailureModeSkip && (kind == "guardrail" || kind == "policy") {
			http.Error(w, "skip failure behavior is not allowed for guardrail or policy knowledge contexts", http.StatusBadRequest)
			return
		}
		source = connection.Provider
		contentSource = "external_page"
		parsedConnectionUUID, parseErr := uuid.Parse(connection.UUID)
		if parseErr != nil {
			http.Error(w, "knowledge connection has invalid UUID", http.StatusInternalServerError)
			return
		}
		connectionUUID = parsedConnectionUUID
		externalProvider = connection.Provider
		externalPageTitle = strings.TrimSpace(req.ExternalPageTitle)
		if strings.TrimSpace(req.Content) != "" {
			syncStatus = "cached"
			now := time.Now().UTC()
			lastSyncedAt = now
			contentHash = hashKnowledgeText(req.Content)
		}
	}

	_, err = a.db.Exec(r.Context(), `
		INSERT INTO knowledge_contexts (
			kind, team_path, name, description, content, source, managed_by_config_repo,
			content_source, connection_id, external_provider, external_page_id, external_page_url,
				external_page_title, sync_mode, sync_interval_minutes, failure_mode, sync_failure_mode, sync_status,
				last_sync_status, last_synced_at, sync_error, last_sync_error, synced_content, content_hash, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, FALSE, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15, $16, $16, $17, '', '', $5, $18, NOW())
		ON CONFLICT (kind, team_path, name) DO UPDATE SET
			description = EXCLUDED.description,
			content = EXCLUDED.content,
			source = EXCLUDED.source,
			content_source = EXCLUDED.content_source,
			config_repo_id = NULL,
			config_source_path = '',
			config_source_commit_sha = '',
			managed_by_config_repo = FALSE,
			connection_id = EXCLUDED.connection_id,
			external_provider = EXCLUDED.external_provider,
			external_page_id = EXCLUDED.external_page_id,
				external_page_url = EXCLUDED.external_page_url,
				external_page_title = EXCLUDED.external_page_title,
				sync_mode = EXCLUDED.sync_mode,
				sync_interval_minutes = EXCLUDED.sync_interval_minutes,
				failure_mode = EXCLUDED.failure_mode,
			sync_failure_mode = EXCLUDED.sync_failure_mode,
			sync_status = EXCLUDED.sync_status,
			last_sync_status = EXCLUDED.last_sync_status,
			last_synced_at = EXCLUDED.last_synced_at,
			sync_error = EXCLUDED.sync_error,
			last_sync_error = EXCLUDED.last_sync_error,
			synced_content = EXCLUDED.synced_content,
			content_hash = EXCLUDED.content_hash,
			updated_at = NOW()
		`, kind, team, name, strings.TrimSpace(req.Description), req.Content, source,
		contentSource, connectionUUID, externalProvider, externalPageID, externalPageURL, externalPageTitle,
		syncMode, syncInterval, failureMode, syncStatus, lastSyncedAt, contentHash)
	if err != nil {
		log.Error().Err(err).Str("knowledge_context", identifier).Msg("Failed to save knowledge context")
		http.Error(w, "Failed to save knowledge context", http.StatusInternalServerError)
		return
	}
	detail, err := a.loadKnowledgeContextDetail(r.Context(), kind, team, name)
	if err != nil {
		http.Error(w, "Failed to load saved knowledge context", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *App) handleKnowledgeContextPost(w http.ResponseWriter, r *http.Request) {
	identifier := strings.Trim(strings.TrimSpace(r.PathValue("knowledgeID")), "/")
	if !strings.HasSuffix(identifier, "/sync") {
		http.NotFound(w, r)
		return
	}
	identifier = strings.Trim(strings.TrimSuffix(identifier, "/sync"), "/")
	a.handleSyncKnowledgeContext(w, r, identifier)
}

func (a *App) handleSyncKnowledgeContext(w http.ResponseWriter, r *http.Request, identifier string) {
	kind, team, name, err := splitKnowledgeContextIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	detail, err := a.loadKnowledgeContextDetail(r.Context(), kind, team, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "knowledge context not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("knowledge_context", identifier).Msg("Failed to load knowledge context for sync")
		http.Error(w, "Failed to load knowledge context", http.StatusInternalServerError)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_context.update", aaamodel.ResourceRef{Type: grantResourceKnowledgeContext, ID: detail.ID}) {
		return
	}
	if strings.TrimSpace(detail.ConnectionID) == "" || (strings.TrimSpace(detail.ExternalPageID) == "" && strings.TrimSpace(detail.ExternalPageURL) == "") {
		http.Error(w, "knowledge context is not configured for an external page", http.StatusBadRequest)
		return
	}
	connection, err := a.loadKnowledgeConnectionByIdentifier(r.Context(), detail.ConnectionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "knowledge connection not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to load knowledge connection", http.StatusInternalServerError)
		return
	}
	if !a.requireAAADecision(w, r, "knowledge_connection.use", aaamodel.ResourceRef{Type: grantResourceKnowledgeConnection, ID: connection.ID}) {
		return
	}
	claimed, err := a.markKnowledgeContextSyncing(r.Context(), detail)
	if err != nil {
		log.Error().Err(err).Str("knowledge_context", detail.ID).Msg("Failed to claim manual Knowledge Context sync")
		http.Error(w, "Failed to start knowledge context sync", http.StatusInternalServerError)
		return
	}
	if !claimed {
		http.Error(w, "knowledge context synchronization is already in progress", http.StatusConflict)
		return
	}

	page, syncErr := a.syncExternalKnowledgePage(r.Context(), detail, connection, knowledgeSyncModeManual)
	if syncErr != nil {
		log.Warn().
			Err(syncErr).
			Str("knowledge_context", detail.ID).
			Str("knowledge_connection", connection.ID).
			Msg("Knowledge context manual sync failed")
	} else {
		log.Info().
			Str("knowledge_context", detail.ID).
			Str("knowledge_connection", connection.ID).
			Str("content_hash", page.Hash).
			Msg("Knowledge context manual sync completed from provider page")
	}
	a.auditKnowledgeContextSync(r.Context(), r, detail, connection, "manual", syncErr)
	updated, err := a.loadKnowledgeContextDetail(r.Context(), kind, team, name)
	if err != nil {
		http.Error(w, "Failed to load synced knowledge context", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *App) handleDeleteKnowledgeContext(w http.ResponseWriter, r *http.Request) {
	kind, team, name, err := splitKnowledgeContextIdentifier(r.PathValue("knowledgeID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resourceID := buildKnowledgeContextIdentifier(kind, team, name)
	if !a.requireAAADecision(w, r, "knowledge_context.delete", aaamodel.ResourceRef{Type: grantResourceKnowledgeContext, ID: resourceID}) {
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM knowledge_contexts WHERE kind = $1 AND team_path = $2 AND name = $3`, kind, team, name)
	if err != nil {
		http.Error(w, "Failed to delete knowledge context", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "knowledge context not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) loadKnowledgeContextDetail(ctx context.Context, kind, team, name string) (knowledgeContextDetail, error) {
	var detail knowledgeContextDetail
	var managed bool
	var connectionID sql.NullString
	var connectionTeam sql.NullString
	var connectionName sql.NullString
	var connectionDisplayName sql.NullString
	var connectionStatus sql.NullString
	var lastSyncedAt sql.NullTime
	var sourceModifiedAt sql.NullTime
	err := a.db.QueryRow(ctx, `
		SELECT k.id::text, k.kind, k.team_path, k.name, k.description, k.content,
		       k.source, k.managed_by_config_repo, k.config_source_path, k.config_source_commit_sha, k.updated_at,
		       k.connection_id::text, c.team_path, c.name, c.display_name, c.status,
		       k.external_provider, k.external_page_id, k.external_page_url,
		       k.external_page_title, k.sync_mode, k.sync_interval_minutes, k.failure_mode, k.sync_status, k.last_synced_at, k.sync_error,
		       k.source_modified_at, k.content_hash
		FROM knowledge_contexts k
		LEFT JOIN knowledge_context_connections c ON c.id = k.connection_id
		WHERE k.kind = $1 AND k.team_path = $2 AND k.name = $3
	`, kind, team, name).Scan(
		&detail.UUID, &detail.Kind, &detail.Team, &detail.Name, &detail.Description,
		&detail.Content, &detail.Source, &managed,
		&detail.GitOpsPath, &detail.GitOpsCommit, &detail.UpdatedAt,
		&connectionID, &connectionTeam, &connectionName, &connectionDisplayName, &connectionStatus,
		&detail.ExternalProvider, &detail.ExternalPageID, &detail.ExternalPageURL,
		&detail.ExternalPageTitle, &detail.SyncMode, &detail.SyncInterval, &detail.FailureMode, &detail.SyncStatus, &lastSyncedAt, &detail.SyncError,
		&sourceModifiedAt, &detail.ContentHash,
	)
	if err != nil {
		return detail, err
	}
	detail.ID = buildKnowledgeContextIdentifier(detail.Kind, detail.Team, detail.Name)
	if connectionID.Valid {
		detail.ConnectionID = connectionID.String
		if connectionTeam.Valid && connectionName.Valid {
			detail.ConnectionRef = buildKnowledgeConnectionIdentifier(connectionTeam.String, connectionName.String)
			detail.Connection = &knowledgeContextConnectionSummary{
				ID:       detail.ConnectionRef,
				Name:     firstNonEmptyString(connectionDisplayName.String, connectionName.String),
				Provider: detail.ExternalProvider,
				Status:   connectionStatus.String,
			}
		}
	}
	if lastSyncedAt.Valid {
		detail.LastSyncedAt = &lastSyncedAt.Time
	}
	if sourceModifiedAt.Valid {
		detail.SourceModifiedAt = &sourceModifiedAt.Time
	}
	visibility, err := a.resourceVisibility(ctx, grantResourceKnowledgeContext, detail.ID)
	if err != nil {
		return detail, err
	}
	detail.Visibility = visibility
	detail.Access = detail.Visibility
	detail.ManagedByGit = managed
	if managed {
		detail.Source = knowledgeSourceGitOps
	}
	if detail.ExternalPageID != "" || detail.ExternalPageURL != "" {
		assets, err := a.loadKnowledgeContextAssets(ctx, detail.UUID)
		if err != nil {
			return detail, err
		}
		detail.Assets = assets
		detail.ExternalPage = &knowledgeContextExternalPageSummary{
			ID:    detail.ExternalPageID,
			Title: detail.ExternalPageTitle,
			URL:   detail.ExternalPageURL,
		}
		errorText := strings.TrimSpace(detail.SyncError)
		var errorPtr *string
		if errorText != "" {
			errorPtr = &errorText
		}
		detail.Sync = &knowledgeContextSyncSummary{
			Mode:         firstNonEmptyString(detail.SyncMode, knowledgeSyncModeManual),
			IntervalMins: detail.SyncInterval,
			FailureMode:  firstNonEmptyString(detail.FailureMode, knowledgeFailureModeFail),
			LastSyncedAt: detail.LastSyncedAt,
			Status:       firstNonEmptyString(detail.SyncStatus, "not_synced"),
			Error:        errorPtr,
		}
	}
	usage := a.knowledgeContextUsage(ctx)
	detail.UsedBy = usage[detail.ID]
	detail.UsedByCount = len(detail.UsedBy)
	return detail, nil
}

func (a *App) knowledgeContextUsage(ctx context.Context) map[string][]string {
	usage := map[string][]string{}
	if a == nil || a.db == nil {
		return usage
	}
	rows, err := a.db.Query(ctx, `SELECT path, name, definition FROM pipelines`)
	if err != nil {
		return usage
	}
	defer rows.Close()
	for rows.Next() {
		var path, name, definition string
		if err := rows.Scan(&path, &name, &definition); err != nil {
			continue
		}
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(definition), &pipeline); err != nil {
			continue
		}
		pipelineID := configsync.BuildPipelineIdentifier(path, name)
		for _, ref := range collectPipelineKnowledgeContextRefs(pipeline) {
			if strings.TrimSpace(ref.Ref.Ref) == "" {
				continue
			}
			kind, team, name, err := knowledgeContextRefToParts(ref.Ref.Kind, ref.Ref.Ref)
			if err != nil {
				continue
			}
			id := buildKnowledgeContextIdentifier(kind, team, name)
			usage[id] = appendUniqueString(usage[id], pipelineID)
		}
	}
	for id := range usage {
		sort.Strings(usage[id])
	}
	return usage
}

type pipelineKnowledgeContextRef struct {
	Ref      models.KnowledgeContextRef
	Location string
}

func collectPipelineKnowledgeContextRefs(pipeline models.Pipeline) []pipelineKnowledgeContextRef {
	var refs []pipelineKnowledgeContextRef
	for _, ref := range pipeline.KnowledgeContext {
		refs = append(refs, pipelineKnowledgeContextRef{Ref: ref, Location: "pipeline"})
	}
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			stepName = "unknown"
		}
		for _, ref := range step.GetKnowledgeContext() {
			refs = append(refs, pipelineKnowledgeContextRef{Ref: ref, Location: fmt.Sprintf("step %q", stepName)})
		}
		for _, task := range step.GetTasks() {
			taskName := task.Name
			if taskName == "" {
				taskName = "unknown"
			}
			for _, ref := range task.KnowledgeContext {
				refs = append(refs, pipelineKnowledgeContextRef{Ref: ref, Location: fmt.Sprintf("task %q in step %q", taskName, stepName)})
			}
		}
	}
	return refs
}

func (a *App) resolveKnowledgeContextsForRun(ctx context.Context, runID uuid.UUID, callerType, callerID, triggerSource string, gitContext map[string]string, pipeline models.Pipeline) ([]models.KnowledgeContextSnapshot, []ResourceUseAuthResult, error) {
	if !models.PipelineLLMEnabled(&pipeline) {
		return nil, nil, nil
	}
	refs := collectPipelineKnowledgeContextRefs(pipeline)
	if len(refs) == 0 {
		return nil, nil, nil
	}

	type keyedRef struct {
		ref      models.KnowledgeContextRef
		location string
	}
	unique := map[string]keyedRef{}
	order := make([]string, 0, len(refs))
	for _, entry := range refs {
		kind, err := normalizeKnowledgeContextKind(entry.Ref.Kind)
		if err != nil {
			if entry.Ref.Required {
				return nil, nil, fmt.Errorf("invalid knowledge context in %s: %w", entry.Location, err)
			}
			continue
		}
		ref := entry.Ref
		ref.Kind = kind
		key := ""
		if strings.TrimSpace(ref.Ref) != "" {
			_, team, name, err := knowledgeContextRefToParts(kind, ref.Ref)
			if err != nil {
				if ref.Required {
					return nil, nil, fmt.Errorf("invalid knowledge context ref in %s: %w", entry.Location, err)
				}
				continue
			}
			ref.Ref = strings.Trim(strings.TrimSpace(ref.Ref), "/")
			key = "ref:" + buildKnowledgeContextIdentifier(kind, team, name)
		} else if strings.TrimSpace(ref.Path) != "" {
			path, err := normalizeKnowledgeContextPath(ref.Path)
			if err != nil {
				if ref.Required {
					return nil, nil, fmt.Errorf("invalid knowledge context path in %s: %w", entry.Location, err)
				}
				continue
			}
			ref.Path = path
			key = "path:" + kind + ":" + path
		} else {
			if ref.Required {
				return nil, nil, fmt.Errorf("invalid knowledge context in %s: ref or path is required", entry.Location)
			}
			continue
		}
		if existing, ok := unique[key]; ok {
			if ref.Required {
				existing.ref.Required = true
			}
			unique[key] = existing
			continue
		}
		unique[key] = keyedRef{ref: ref, location: entry.Location}
		order = append(order, key)
	}

	var snapshots []models.KnowledgeContextSnapshot
	var authChecks []ResourceUseAuthResult
	for _, key := range order {
		entry := unique[key]
		if strings.HasPrefix(key, "ref:") {
			snapshot, authResult, err := a.resolveManagedKnowledgeContext(ctx, callerType, callerID, triggerSource, gitContext, entry.ref)
			if authResult.Action != "" {
				authChecks = append(authChecks, authResult)
			}
			if err != nil {
				if entry.ref.Required {
					return snapshots, authChecks, err
				}
				log.Warn().Err(err).Str("run_id", runID.String()).Str("location", entry.location).Msg("Skipping optional knowledge context")
				continue
			}
			snapshots = append(snapshots, snapshot)
			continue
		}
		snapshot, err := a.resolveRepositoryKnowledgeContext(gitContext, entry.ref)
		if err != nil {
			if entry.ref.Required {
				return snapshots, authChecks, err
			}
			log.Warn().Err(err).Str("run_id", runID.String()).Str("location", entry.location).Msg("Skipping optional repository knowledge context")
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := a.persistRunKnowledgeContextSnapshots(ctx, runID, snapshots); err != nil {
		return snapshots, authChecks, err
	}
	return snapshots, authChecks, nil
}

func (a *App) resolveManagedKnowledgeContext(ctx context.Context, callerType, callerID, triggerSource string, gitContext map[string]string, ref models.KnowledgeContextRef) (models.KnowledgeContextSnapshot, ResourceUseAuthResult, error) {
	kind, team, name, err := knowledgeContextRefToParts(ref.Kind, ref.Ref)
	if err != nil {
		return models.KnowledgeContextSnapshot{}, ResourceUseAuthResult{}, err
	}
	resourceID := buildKnowledgeContextIdentifier(kind, team, name)
	authResult, err := a.AuthorizeResourceUse(ctx, ResourceUseAuthInput{
		CallerType:   callerType,
		CallerID:     callerID,
		Action:       "knowledge_context.use",
		ResourceType: grantResourceKnowledgeContext,
		ResourceID:   resourceID,
		EventType:    triggerSource,
		Ref:          gitContext["ref"],
		Repo:         repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"]),
	})
	if err != nil {
		return models.KnowledgeContextSnapshot{}, authResult, fmt.Errorf("knowledge context authorization unavailable for %s: %w", resourceID, err)
	}
	if !authResult.Allowed {
		return models.KnowledgeContextSnapshot{}, authResult, fmt.Errorf("%s", resourceUseDeniedMessage(callerType, callerID, authResult))
	}

	var snapshot models.KnowledgeContextSnapshot
	var resolvedAt time.Time
	var externalPageID string
	var externalPageURL string
	var syncStatus string
	var failureMode string
	var syncMode string
	var connectionID sql.NullString
	err = a.db.QueryRow(ctx, `
		SELECT id::text, kind, team_path, name, description, content,
		       source, config_source_path, config_source_commit_sha, updated_at,
		       external_page_id, external_page_url, sync_status, failure_mode, sync_mode, connection_id::text
		FROM knowledge_contexts
		WHERE kind = $1 AND team_path = $2 AND name = $3
	`, kind, team, name).Scan(
		&snapshot.KnowledgeContextID, &snapshot.Kind, &snapshot.Team, &snapshot.Name,
		&snapshot.Description, &snapshot.Content, &snapshot.Source,
		&snapshot.ConfigSourcePath, &snapshot.ConfigSourceCommitSHA, &resolvedAt,
		&externalPageID, &externalPageURL, &syncStatus, &failureMode, &syncMode, &connectionID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.KnowledgeContextSnapshot{}, authResult, fmt.Errorf("knowledge context %s was not found", resourceID)
		}
		return models.KnowledgeContextSnapshot{}, authResult, err
	}
	isExternal := connectionID.Valid || strings.TrimSpace(externalPageID) != "" || strings.TrimSpace(externalPageURL) != ""
	if isExternal && syncMode == knowledgeSyncModeBeforeRun {
		detail, loadErr := a.loadKnowledgeContextDetail(ctx, kind, team, name)
		if loadErr != nil {
			return models.KnowledgeContextSnapshot{}, authResult, loadErr
		}
		connection, loadErr := a.loadKnowledgeConnectionByIdentifier(ctx, detail.ConnectionID)
		if loadErr != nil {
			if resolved, ok := applyExternalKnowledgeFailureMode(snapshot, ref, failureMode, fmt.Errorf("knowledge connection unavailable for %s: %w", resourceID, loadErr)); ok {
				snapshot = resolved
			} else {
				a.knowledgeSyncMetrics.recordBeforeRunBlock(snapshot.Source)
				return models.KnowledgeContextSnapshot{}, authResult, fmt.Errorf("knowledge connection unavailable for %s: %w", resourceID, loadErr)
			}
		} else if page, fetchErr := a.syncExternalKnowledgePage(ctx, detail, connection, knowledgeSyncModeBeforeRun); fetchErr != nil {
			if resolved, ok := applyExternalKnowledgeFailureMode(snapshot, ref, failureMode, fetchErr); ok {
				snapshot = resolved
			} else {
				a.knowledgeSyncMetrics.recordBeforeRunBlock(connection.Provider)
				return models.KnowledgeContextSnapshot{}, authResult, fmt.Errorf("knowledge context %s could not fetch external page: %w", resourceID, fetchErr)
			}
		} else {
			snapshot.Content = page.Text
			snapshot.Source = connection.Provider
			resolvedAt = time.Now().UTC()
		}
	}
	if isExternal && strings.TrimSpace(snapshot.Content) == "" {
		err := fmt.Errorf("knowledge context %s is not synchronized yet (status: %s, failure_mode: %s)", resourceID, firstNonEmptyString(syncStatus, "not_synced"), firstNonEmptyString(failureMode, knowledgeFailureModeFail))
		if resolved, ok := applyExternalKnowledgeFailureMode(snapshot, ref, failureMode, err); ok {
			snapshot = resolved
		} else {
			return models.KnowledgeContextSnapshot{}, authResult, err
		}
	}
	snapshot.Ref = strings.Trim(strings.TrimSpace(ref.Ref), "/")
	snapshot.Required = ref.Required
	snapshot.ResolvedAt = resolvedAt
	return snapshot, authResult, nil
}

func (a *App) resolveRepositoryKnowledgeContext(gitContext map[string]string, ref models.KnowledgeContextRef) (models.KnowledgeContextSnapshot, error) {
	path, err := normalizeKnowledgeContextPath(ref.Path)
	if err != nil {
		return models.KnowledgeContextSnapshot{}, err
	}
	owner := strings.TrimSpace(gitContext["repo_owner"])
	repo := strings.TrimSpace(gitContext["repo_name"])
	commit := strings.TrimSpace(gitContext["commit_sha"])
	if owner == "" || repo == "" || commit == "" {
		return models.KnowledgeContextSnapshot{}, fmt.Errorf("repository knowledge context %s requires git owner, repo, and commit", path)
	}
	content, err := a.requestGitBotFile(owner, repo, commit, path, errKnowledgeContextNotFound)
	if err != nil {
		return models.KnowledgeContextSnapshot{}, fmt.Errorf("repository knowledge context %s could not be loaded: %w", path, err)
	}
	kind, _ := normalizeKnowledgeContextKind(ref.Kind)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	return models.KnowledgeContextSnapshot{
		Kind:       kind,
		Name:       name,
		Path:       path,
		Required:   ref.Required,
		Source:     knowledgeSourceRepo,
		Content:    content,
		ResolvedAt: time.Now(),
	}, nil
}

var errKnowledgeContextNotFound = errors.New("knowledge context not found")

func (a *App) persistRunKnowledgeContextSnapshots(ctx context.Context, runID uuid.UUID, snapshots []models.KnowledgeContextSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, snapshot := range snapshots {
		var knowledgeID any
		if strings.TrimSpace(snapshot.KnowledgeContextID) != "" {
			knowledgeID = snapshot.KnowledgeContextID
		}
		batch.Queue(`
			INSERT INTO pipeline_run_knowledge_contexts (
				run_id, knowledge_context_id, kind, team_path, name, description,
				ref, path, required, source, content,
				config_source_path, config_source_commit_sha, resolved_at
			) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, COALESCE($14, NOW()))
		`, runID, knowledgeID, snapshot.Kind, snapshot.Team, snapshot.Name, snapshot.Description,
			snapshot.Ref, snapshot.Path, snapshot.Required, snapshot.Source, snapshot.Content,
			snapshot.ConfigSourcePath, snapshot.ConfigSourceCommitSHA, nullableTime(snapshot.ResolvedAt))
	}
	br := a.db.SendBatch(ctx, batch)
	defer br.Close()
	for range snapshots {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("failed to persist knowledge context snapshot: %w", err)
		}
	}
	return nil
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func (a *App) loadRunKnowledgeContextSnapshots(ctx context.Context, runID string) ([]models.KnowledgeContextSnapshot, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id::text, COALESCE(knowledge_context_id::text, ''), kind, team_path, name, description,
		       ref, path, required, source, content,
		       config_source_path, config_source_commit_sha, resolved_at
		FROM pipeline_run_knowledge_contexts
		WHERE run_id::text = $1
		ORDER BY id ASC
	`, strings.TrimSpace(runID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []models.KnowledgeContextSnapshot
	for rows.Next() {
		var snapshot models.KnowledgeContextSnapshot
		if err := rows.Scan(&snapshot.ID, &snapshot.KnowledgeContextID, &snapshot.Kind, &snapshot.Team, &snapshot.Name,
			&snapshot.Description, &snapshot.Ref, &snapshot.Path, &snapshot.Required, &snapshot.Source, &snapshot.Content,
			&snapshot.ConfigSourcePath, &snapshot.ConfigSourceCommitSHA, &snapshot.ResolvedAt); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func snapshotsJSONBase64(snapshots []models.KnowledgeContextSnapshot) (string, error) {
	if len(snapshots) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(snapshots)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}
