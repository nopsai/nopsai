package nopsai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/configsync"

	"gopkg.in/yaml.v3"
)

type configRepositoryDriftResponse struct {
	BaseBranch  string                      `json:"base_branch"`
	PushBranch  string                      `json:"push_branch"`
	Items       []configRepositoryDriftItem `json:"items"`
	Summary     map[string]int              `json:"summary"`
	CanPush     bool                        `json:"can_push"`
	PushMessage string                      `json:"push_message"`
}

type configRepositoryDriftItem = configsync.DriftItem

func marshalConfigRepositoryYAML(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *App) handleGetGlobalConfigRepositoryDrift(w http.ResponseWriter, r *http.Request) {
	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeSystem, models.ConfigRepositorySystemGlobalID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load global config repository")
		return
	}
	a.handleGetConfigRepositoryDrift(w, r, repo)
}

func (a *App) handleGetTeamConfigRepositoryDrift(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}

	repo, err := a.store.GetConfigRepositoryByScope(r.Context(), models.ConfigRepositoryScopeTeam, resource.ID)
	if err != nil {
		writeConfigRepositoryStoreError(w, err, "failed to load config repository")
		return
	}
	a.handleGetConfigRepositoryDrift(w, r, repo)
}

func (a *App) handleGetConfigRepositoryDrift(w http.ResponseWriter, r *http.Request, repo models.ConfigRepository) {
	desired, err := a.exportConfigRepositoryFiles(r.Context(), repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	gitFiles, err := a.loadConfigRepositoryGitFiles(r.Context(), repo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	items := diffConfigRepositoryFiles(gitFiles, desired)
	summary := map[string]int{"added": 0, "modified": 0, "deleted": 0, "unchanged": 0}
	for _, item := range items {
		summary[item.Status]++
	}

	writeJSON(w, http.StatusOK, configRepositoryDriftResponse{
		BaseBranch:  repo.Branch,
		PushBranch:  strings.TrimSpace(repo.WriteBranch),
		Items:       items,
		Summary:     summary,
		CanPush:     repo.WriteEnabled && strings.TrimSpace(repo.WriteBranch) != "",
		PushMessage: defaultConfigRepositoryPushMessage(repo),
	})
}

func (a *App) loadConfigRepositoryGitFiles(ctx context.Context, repo models.ConfigRepository) (map[string]string, error) {
	client, _, err := a.newConfigRepositoryGitContentClient(ctx, repo)
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	dirs := configRepositoryGitDirsForBasePath(repo.BasePath)
	branch := configRepositoryBranch(repo.Branch)
	for _, directoryPath := range []string{
		dirs.pipeline,
		dirs.step,
		dirs.trigger,
		dirs.externalTrigger,
		dirs.gitWebhookSource,
		dirs.dashboard,
		dirs.dashboardTemplate,
		dirs.schedule,
		dirs.scope,
		dirs.knowledge,
		dirs.configRepository,
		dirs.access,
		dirs.setting,
	} {
		files, err := client.Directory(ctx, branch, directoryPath)
		if err != nil {
			return nil, err
		}
		for filePath, content := range files {
			rel, ok := configRepositoryRelativeGitPath(repo.BasePath, filePath)
			if !ok || !isConfigRepositoryDriftPath(rel) {
				continue
			}
			result[rel] = normalizeConfigRepositoryFileContent(content)
		}
	}
	if repo.ScopeType == models.ConfigRepositoryScopeTeam {
		rootPath := configsync.RepoJoinPath(repo.BasePath, "notifications.yaml")
		content, err := client.File(ctx, branch, rootPath, errNotificationGitOpsNotFound)
		if err == nil {
			if rel, ok := configRepositoryRelativeGitPath(repo.BasePath, rootPath); ok && isConfigRepositoryDriftPath(rel) {
				result[rel] = normalizeConfigRepositoryFileContent(content)
			}
		} else if !errors.Is(err, errNotificationGitOpsNotFound) {
			return nil, err
		}
		for _, rootPath := range []string{
			configsync.RepoJoinPath(repo.BasePath, "ai-profiles.yaml"),
			configsync.RepoJoinPath(repo.BasePath, "ai-profiles.yml"),
		} {
			content, err := client.File(ctx, branch, rootPath, errTeamAIProfilesGitOpsNotFound)
			if err == nil {
				if rel, ok := configRepositoryRelativeGitPath(repo.BasePath, rootPath); ok && isConfigRepositoryDriftPath(rel) {
					result[rel] = normalizeConfigRepositoryFileContent(content)
				}
				continue
			}
			if !errors.Is(err, errTeamAIProfilesGitOpsNotFound) {
				return nil, err
			}
		}
	}
	return result, nil
}

func diffConfigRepositoryFiles(gitFiles, desiredFiles map[string]string) []configRepositoryDriftItem {
	return configsync.DiffFiles(gitFiles, desiredFiles, configRepositoryFileContentsEqual)
}

func configRepositoryFileContentsEqual(filePath, gitContent, desiredContent string) bool {
	if gitContent == desiredContent {
		return true
	}
	if !strings.HasPrefix(strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/"), "knowledge/") {
		return false
	}
	gitDoc, ok := canonicalKnowledgeContextDriftDocument(filePath, gitContent)
	if !ok {
		return false
	}
	desiredDoc, ok := canonicalKnowledgeContextDriftDocument(filePath, desiredContent)
	if !ok {
		return false
	}
	return gitDoc == desiredDoc
}

type configRepositoryKnowledgeDriftDocument struct {
	Name        string
	Kind        string
	Description string
	Access      string
	Content     string
}

func canonicalKnowledgeContextDriftDocument(filePath, content string) (configRepositoryKnowledgeDriftDocument, bool) {
	doc, body, err := parseKnowledgeContextDocument(content)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	pathKind, _, pathName, err := parseKnowledgeContextDriftPath(filePath)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	kind := strings.TrimSpace(doc.Kind)
	if kind == "" {
		kind = pathKind
	}
	name := strings.TrimSpace(doc.Name)
	if name == "" {
		name = pathName
	}
	kind, err = normalizeKnowledgeContextKind(kind)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	name, err = normalizeKnowledgeContextName(name)
	if err != nil {
		return configRepositoryKnowledgeDriftDocument{}, false
	}
	return configRepositoryKnowledgeDriftDocument{
		Name:        name,
		Kind:        kind,
		Description: strings.TrimSpace(doc.Description),
		Access:      canonicalConfigRepositoryAccessString(doc.Access),
		Content:     strings.TrimSpace(body),
	}, true
}

func parseKnowledgeContextDriftPath(filePath string) (string, string, string, error) {
	rel := strings.Trim(strings.TrimSpace(filepath.ToSlash(filePath)), "/")
	rel = strings.TrimPrefix(rel, "knowledge/")
	parts := strings.Split(rel, "/")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("knowledge document path must use kind/team/document")
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
	return kind, team, name, nil
}

func (a *App) exportConfigRepositoryFiles(ctx context.Context, repo models.ConfigRepository) (map[string]string, error) {
	delegatedScopes, err := a.configRepositoryDelegatedScopes(ctx, repo)
	if err != nil {
		return nil, err
	}

	resourceAccess, err := a.configRepositoryResourceAccess(ctx, repo, delegatedScopes)
	if err != nil {
		return nil, err
	}

	files := map[string]string{}
	if err := a.exportConfigRepositoryPipelines(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositorySteps(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryTriggers(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryExternalTriggers(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryGitWebhookSources(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositorySchedules(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryDashboards(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryNotificationRoutes(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryScopes(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryKnowledge(ctx, repo, delegatedScopes, resourceAccess, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryTeamStructure(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryAccess(ctx, repo, delegatedScopes, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryLLMProfiles(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryAgentProfiles(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryMCPRegistry(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryTeamAIProfiles(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryAuthSettings(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryCredentials(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryRuntimeSettings(repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryMailSettings(ctx, repo, files); err != nil {
		return nil, err
	}
	if err := a.exportConfigRepositoryDataManagement(ctx, repo, files); err != nil {
		return nil, err
	}
	for filePath, content := range files {
		files[filePath] = normalizeConfigRepositoryFileContent(content)
	}
	return files, nil
}

func (a *App) configRepositoryDelegatedScopes(ctx context.Context, repo models.ConfigRepository) ([]string, error) {
	scopeSet := map[string]struct{}{}
	addScope := func(scope string) {
		scope = strings.Trim(strings.TrimSpace(scope), "/")
		if scope == "" {
			return
		}
		if repo.ScopeType == models.ConfigRepositoryScopeTeam {
			boundScope := strings.Trim(strings.TrimSpace(repo.ScopeID), "/")
			if scope == boundScope || !configsync.ResourceUnderScope(scope, boundScope) {
				return
			}
		}
		scopeSet[scope] = struct{}{}
	}

	rows, err := a.db.Query(ctx, `
		SELECT scope_id
		FROM config_repositories
		WHERE scope_type = $1
		  AND enabled = TRUE
		  AND id <> $2
	`, models.ConfigRepositoryScopeTeam, repo.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load delegated config repository scopes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var scopeID string
		if err := rows.Scan(&scopeID); err != nil {
			return nil, err
		}
		addScope(scopeID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	scopes := make([]string, 0, len(scopeSet))
	for scope := range scopeSet {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes, nil
}
