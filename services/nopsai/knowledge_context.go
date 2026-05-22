package main

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
	group       string
	name        string
	description string
	content     string
	sourcePath  string
}

type knowledgeContextListItem struct {
	ID           string    `json:"id"`
	UUID         string    `json:"uuid,omitempty"`
	Kind         string    `json:"kind"`
	Group        string    `json:"group"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Visibility   string    `json:"visibility"`
	Source       string    `json:"source"`
	UpdatedAt    time.Time `json:"updated_at"`
	Access       string    `json:"access"`
	UsedByCount  int       `json:"used_by_count"`
	UsedBy       []string  `json:"used_by,omitempty"`
	GitOpsPath   string    `json:"config_source_path,omitempty"`
	GitOpsCommit string    `json:"config_source_commit_sha,omitempty"`
}

type knowledgeContextDetail struct {
	knowledgeContextListItem
	Content      string `json:"content"`
	ManagedByGit bool   `json:"managed_by_config_repo"`
}

type upsertKnowledgeContextRequest struct {
	Kind        string `json:"kind"`
	Group       string `json:"group"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
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

func normalizeKnowledgeContextGroup(raw string) (string, error) {
	group := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	if group == "." {
		return "", nil
	}
	if group == "" {
		return "", nil
	}
	for _, segment := range strings.Split(group, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("group contains invalid path segments")
		}
	}
	return group, nil
}

func buildKnowledgeContextIdentifier(kind, group, name string) string {
	parts := []string{strings.Trim(strings.TrimSpace(kind), "/")}
	if group = strings.Trim(strings.TrimSpace(group), "/"); group != "" {
		parts = append(parts, group)
	}
	parts = append(parts, strings.Trim(strings.TrimSpace(name), "/"))
	return strings.Join(parts, "/")
}

func splitKnowledgeContextIdentifier(raw string) (string, string, string, error) {
	value := strings.Trim(strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/")), "/")
	parts := strings.Split(value, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("knowledge context id must use kind/group/name")
	}
	kind, err := normalizeKnowledgeContextKind(parts[0])
	if err != nil {
		return "", "", "", err
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	group, err := normalizeKnowledgeContextGroup(strings.Join(parts[1:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	if group == "" {
		return "", "", "", fmt.Errorf("knowledge context id must include a group")
	}
	return kind, group, name, nil
}

func knowledgeContextRefToParts(kind, ref string) (string, string, string, error) {
	kind, err := normalizeKnowledgeContextKind(kind)
	if err != nil {
		return "", "", "", err
	}
	ref = strings.Trim(strings.TrimSpace(strings.ReplaceAll(ref, "\\", "/")), "/")
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("knowledge context ref must use group/document")
	}
	name, err := normalizeKnowledgeContextName(parts[len(parts)-1])
	if err != nil {
		return "", "", "", err
	}
	group, err := normalizeKnowledgeContextGroup(strings.Join(parts[:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	if group == "" {
		return "", "", "", fmt.Errorf("knowledge context ref must include a group")
	}
	return kind, group, name, nil
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

func parseKnowledgeContextGitOpsPath(rel string, binding models.ConfigRepository, boundFolder string) (string, string, string, error) {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("knowledge document path must use kind/group/document")
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
	group, err := normalizeKnowledgeContextGroup(strings.Join(parts[1:len(parts)-1], "/"))
	if err != nil {
		return "", "", "", err
	}
	if binding.ScopeType == models.ConfigRepositoryScopeFolder {
		if group == "" {
			group = boundFolder
		} else {
			group, err = normalizeConfigPathForFolder(boundFolder, group)
			if err != nil {
				return "", "", "", err
			}
		}
	}
	if group == "" {
		return "", "", "", fmt.Errorf("knowledge document path must include a group")
	}
	return kind, group, name, nil
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
				return doc, "", true, fmt.Errorf("markdown body outside content field is not supported; use content:")
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

func parseGitOpsKnowledgeContexts(files map[string]string, root string, binding models.ConfigRepository, boundFolder string, accessPlan accessSyncPlan) (map[string]storedKnowledgeContext, error) {
	contexts := make(map[string]storedKnowledgeContext)
	for path, content := range files {
		normalized := filepath.ToSlash(path)
		rel, ok := relativeConfigPath(normalized, root)
		if !ok || rel == "" || strings.HasSuffix(rel, "/") || !isKnowledgeContextGitOpsFile(rel) {
			continue
		}

		kind, group, _, err := parseKnowledgeContextGitOpsPath(rel, binding, boundFolder)
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

		visibility := resourceVisibilityGroup
		if frontMatter.Access != nil {
			rawGrants := embeddedResourceAccessGrants(*frontMatter.Access)
			visibility = firstNonEmptyString(frontMatter.Access.Visibility, embeddedResourceUseAccessMode(frontMatter.Access.UseAccess))
			if visibility == "" && len(rawGrants) > 0 {
				visibility = resourceVisibilityRestricted
			}
			if visibility == "" {
				visibility = resourceVisibilityGroup
			}
		}
		normalizedVisibility, err := normalizeResourceVisibilityUpdate(visibility)
		if err != nil {
			return nil, fmt.Errorf("invalid knowledge context visibility in '%s': %w", normalized, err)
		}
		if err := validateResourceVisibilityPolicy(grantResourceKnowledgeContext, normalizedVisibility); err != nil {
			return nil, fmt.Errorf("invalid knowledge context visibility in '%s': %w", normalized, err)
		}
		key := buildKnowledgeContextIdentifier(kind, group, name)
		if _, exists := contexts[key]; exists {
			return nil, fmt.Errorf("duplicate knowledge context '%s' detected in config repository", key)
		}

		if err := addKnowledgeContextEmbeddedAccess(accessPlan, frontMatter, normalized, key, binding, boundFolder); err != nil {
			return nil, fmt.Errorf("invalid knowledge context access '%s': %w", normalized, err)
		}
		contexts[key] = storedKnowledgeContext{
			kind:        kind,
			group:       group,
			name:        name,
			description: strings.TrimSpace(frontMatter.Description),
			content:     strings.TrimSpace(body),
			sourcePath:  normalized,
		}
	}
	return contexts, nil
}

func addKnowledgeContextEmbeddedAccess(plan accessSyncPlan, fm knowledgeFrontMatter, sourcePath, resourceID string, binding models.ConfigRepository, boundFolder string) error {
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
	return plan.addEmbeddedResourceAccess(string(docBytes), sourcePath, grantResourceKnowledgeContext, resourceID, binding, boundFolder)
}

func (a *App) handleListKnowledgeContexts(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, kind, group_path, name, description, source,
		       managed_by_config_repo, config_source_path, config_source_commit_sha, updated_at
		FROM knowledge_contexts
		ORDER BY kind ASC, group_path ASC, name ASC
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
		if err := rows.Scan(&item.UUID, &item.Kind, &item.Group, &item.Name, &item.Description, &item.Source, &managed, &item.GitOpsPath, &item.GitOpsCommit, &item.UpdatedAt); err != nil {
			log.Error().Err(err).Msg("Failed to scan knowledge context")
			http.Error(w, "Failed to process knowledge contexts", http.StatusInternalServerError)
			return
		}
		item.ID = buildKnowledgeContextIdentifier(item.Kind, item.Group, item.Name)
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
	kind, group, name, err := splitKnowledgeContextIdentifier(identifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	detail, err := a.loadKnowledgeContextDetail(r.Context(), kind, group, name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "knowledge context not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("knowledge_context", identifier).Msg("Failed to load knowledge context")
		http.Error(w, "Failed to load knowledge context", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *App) handleUpsertKnowledgeContext(w http.ResponseWriter, r *http.Request) {
	identifier := r.PathValue("knowledgeID")
	pathKind, pathGroup, pathName, err := splitKnowledgeContextIdentifier(identifier)
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
	group := firstNonEmptyString(req.Group, pathGroup)
	name := firstNonEmptyString(req.Name, pathName)
	kind, err = normalizeKnowledgeContextKind(kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	group, err = normalizeKnowledgeContextGroup(group)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name, err = normalizeKnowledgeContextName(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if kind != pathKind || group != pathGroup || name != pathName {
		http.Error(w, "request body kind, group, and name must match the URL", http.StatusBadRequest)
		return
	}
	resourceID := buildKnowledgeContextIdentifier(kind, group, name)
	var exists int
	lookupErr := a.db.QueryRow(r.Context(), `
		SELECT 1
		FROM knowledge_contexts
		WHERE kind = $1 AND group_path = $2 AND name = $3
		LIMIT 1
	`, kind, group, name).Scan(&exists)
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

	_, err = a.db.Exec(r.Context(), `
		INSERT INTO knowledge_contexts (
			kind, group_path, name, description, content, source, managed_by_config_repo, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, FALSE, NOW())
		ON CONFLICT (kind, group_path, name) DO UPDATE SET
			description = EXCLUDED.description,
			content = EXCLUDED.content,
			source = EXCLUDED.source,
			config_repo_id = NULL,
			config_source_path = '',
			config_source_commit_sha = '',
			managed_by_config_repo = FALSE,
			updated_at = NOW()
	`, kind, group, name, strings.TrimSpace(req.Description), req.Content, knowledgeSourceDatabase)
	if err != nil {
		log.Error().Err(err).Str("knowledge_context", identifier).Msg("Failed to save knowledge context")
		http.Error(w, "Failed to save knowledge context", http.StatusInternalServerError)
		return
	}
	detail, err := a.loadKnowledgeContextDetail(r.Context(), kind, group, name)
	if err != nil {
		http.Error(w, "Failed to load saved knowledge context", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *App) handleDeleteKnowledgeContext(w http.ResponseWriter, r *http.Request) {
	kind, group, name, err := splitKnowledgeContextIdentifier(r.PathValue("knowledgeID"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tag, err := a.db.Exec(r.Context(), `DELETE FROM knowledge_contexts WHERE kind = $1 AND group_path = $2 AND name = $3`, kind, group, name)
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

func (a *App) loadKnowledgeContextDetail(ctx context.Context, kind, group, name string) (knowledgeContextDetail, error) {
	var detail knowledgeContextDetail
	var managed bool
	err := a.db.QueryRow(ctx, `
		SELECT id::text, kind, group_path, name, description, content,
		       source, managed_by_config_repo, config_source_path, config_source_commit_sha, updated_at
		FROM knowledge_contexts
		WHERE kind = $1 AND group_path = $2 AND name = $3
	`, kind, group, name).Scan(
		&detail.UUID, &detail.Kind, &detail.Group, &detail.Name, &detail.Description,
		&detail.Content, &detail.Source, &managed,
		&detail.GitOpsPath, &detail.GitOpsCommit, &detail.UpdatedAt,
	)
	if err != nil {
		return detail, err
	}
	detail.ID = buildKnowledgeContextIdentifier(detail.Kind, detail.Group, detail.Name)
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
		pipelineID := buildPipelineIdentifier(path, name)
		for _, ref := range collectPipelineKnowledgeContextRefs(pipeline) {
			if strings.TrimSpace(ref.Ref.Ref) == "" {
				continue
			}
			kind, group, name, err := knowledgeContextRefToParts(ref.Ref.Kind, ref.Ref.Ref)
			if err != nil {
				continue
			}
			id := buildKnowledgeContextIdentifier(kind, group, name)
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
			_, group, name, err := knowledgeContextRefToParts(kind, ref.Ref)
			if err != nil {
				if ref.Required {
					return nil, nil, fmt.Errorf("invalid knowledge context ref in %s: %w", entry.Location, err)
				}
				continue
			}
			ref.Ref = strings.Trim(strings.TrimSpace(ref.Ref), "/")
			key = "ref:" + buildKnowledgeContextIdentifier(kind, group, name)
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
		snapshot, err := a.resolveRepositoryKnowledgeContext(ctx, gitContext, entry.ref)
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
	kind, group, name, err := knowledgeContextRefToParts(ref.Kind, ref.Ref)
	if err != nil {
		return models.KnowledgeContextSnapshot{}, ResourceUseAuthResult{}, err
	}
	resourceID := buildKnowledgeContextIdentifier(kind, group, name)
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
	err = a.db.QueryRow(ctx, `
		SELECT id::text, kind, group_path, name, description, content,
		       source, config_source_path, config_source_commit_sha, updated_at
		FROM knowledge_contexts
		WHERE kind = $1 AND group_path = $2 AND name = $3
	`, kind, group, name).Scan(
		&snapshot.KnowledgeContextID, &snapshot.Kind, &snapshot.Group, &snapshot.Name,
		&snapshot.Description, &snapshot.Content, &snapshot.Source,
		&snapshot.ConfigSourcePath, &snapshot.ConfigSourceCommitSHA, &resolvedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			return models.KnowledgeContextSnapshot{}, authResult, fmt.Errorf("knowledge context %s was not found", resourceID)
		}
		return models.KnowledgeContextSnapshot{}, authResult, err
	}
	snapshot.Ref = strings.Trim(strings.TrimSpace(ref.Ref), "/")
	snapshot.Required = ref.Required
	snapshot.ResolvedAt = resolvedAt
	return snapshot, authResult, nil
}

func (a *App) resolveRepositoryKnowledgeContext(ctx context.Context, gitContext map[string]string, ref models.KnowledgeContextRef) (models.KnowledgeContextSnapshot, error) {
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
				run_id, knowledge_context_id, kind, group_path, name, description,
				ref, path, required, source, content,
				config_source_path, config_source_commit_sha, resolved_at
			) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, COALESCE($14, NOW()))
		`, runID, knowledgeID, snapshot.Kind, snapshot.Group, snapshot.Name, snapshot.Description,
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
		SELECT id::text, COALESCE(knowledge_context_id::text, ''), kind, group_path, name, description,
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
		if err := rows.Scan(&snapshot.ID, &snapshot.KnowledgeContextID, &snapshot.Kind, &snapshot.Group, &snapshot.Name,
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
