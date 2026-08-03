package nopsai

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/internal/gitwebhook"
)

const (
	repositoryTriggerProviderGitHub = "github"

	repositoryTriggerManagementNopsAI     = "nopsai"
	repositoryTriggerManagementRepository = "repository"

	repositoryTriggerAllowlistAutomatic    = "automatic"
	repositoryTriggerAllowlistAllowed      = "allowed"
	repositoryTriggerAllowlistDenied       = "denied"
	repositoryTriggerAllowlistMissing      = "missing_source"
	repositoryTriggerAllowlistNotAssigned  = "not_assigned"
	repositoryTriggerAllowlistNotRequired  = "not_required"
	repositoryTriggerAllowlistUnknown      = "unknown"
	repositoryTriggerAllowlistUnconfigured = "no_trigger"
)

type repositoryTriggerRecord struct {
	RepositoryName       string    `json:"repository_name"`
	Definition           string    `json:"-"`
	Source               string    `json:"source"`
	Visibility           string    `json:"visibility"`
	Provider             string    `json:"provider"`
	TeamPath             string    `json:"team_path"`
	Management           string    `json:"management"`
	WebhookSourceID      string    `json:"webhook_source_id,omitempty"`
	ConfigSourcePath     string    `json:"config_source_path,omitempty"`
	ManagedByConfigRepo  bool      `json:"managed_by_config_repo"`
	CreatedAt            time.Time `json:"created_at"`
	WebhookSourceName    string    `json:"webhook_source_name,omitempty"`
	RepositoryForWebhook string    `json:"repository_for_webhook,omitempty"`
	AllowlistStatus      string    `json:"allowlist_status,omitempty"`
	Ingress              string    `json:"ingress,omitempty"`
}

type repositoryTriggerListItem struct {
	Name                  string   `json:"name"`
	Source                string   `json:"source"`
	Provider              string   `json:"provider"`
	TeamPath              string   `json:"team_path"`
	Management            string   `json:"management"`
	WebhookSourceID       string   `json:"webhook_source_id,omitempty"`
	WebhookSourceName     string   `json:"webhook_source_name,omitempty"`
	Ingress               string   `json:"ingress,omitempty"`
	AllowlistStatus       string   `json:"allowlist_status,omitempty"`
	RepositoryForWebhook  string   `json:"repository_for_webhook,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	ManagedByConfigRepo   bool     `json:"managed_by_config_repo"`
	ConfigSourcePath      string   `json:"config_source_path,omitempty"`
	RepositoryManagedHint string   `json:"repository_managed_hint,omitempty"`
}

type repositoryTriggerDetailResponse struct {
	Slug                 string `json:"slug"`
	Source               string `json:"source"`
	Provider             string `json:"provider"`
	TeamPath             string `json:"team_path"`
	Management           string `json:"management"`
	WebhookSourceID      string `json:"webhook_source_id,omitempty"`
	WebhookSourceName    string `json:"webhook_source_name,omitempty"`
	Ingress              string `json:"ingress"`
	AllowlistStatus      string `json:"allowlist_status"`
	RepositoryForWebhook string `json:"repository_for_webhook,omitempty"`
	RawYAML              string `json:"raw_yaml"`
}

type repositoryTriggerQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func repositoryTriggerRecordFromManifest(repositoryName, definition, source, visibility string, manifest models.Manifest, fallbackTeamPath string) (repositoryTriggerRecord, error) {
	provider, err := normalizeRepositoryTriggerProvider(manifest.Provider)
	if err != nil {
		return repositoryTriggerRecord{}, err
	}
	management, err := normalizeRepositoryTriggerManagement(manifest.Management)
	if err != nil {
		return repositoryTriggerRecord{}, err
	}
	teamPath, err := normalizeRepositoryTriggerTeamPath(firstNonEmptyString(manifest.TeamPath, manifest.Team, fallbackTeamPath))
	if err != nil {
		return repositoryTriggerRecord{}, err
	}
	webhookSourceID := strings.TrimSpace(manifest.WebhookSource)
	return repositoryTriggerRecord{
		RepositoryName:       strings.Trim(strings.TrimSpace(repositoryName), "/"),
		Definition:           definition,
		Source:               normalizeTriggerSource(source),
		Visibility:           normalizeResourceVisibility(visibility),
		Provider:             provider,
		TeamPath:             teamPath,
		Management:           management,
		WebhookSourceID:      webhookSourceID,
		RepositoryForWebhook: repositoryTriggerProviderRepository(repositoryName, teamPath),
	}, nil
}

func repositoryTriggerScopesFromDefinition(definition string) []string {
	var manifest models.Manifest
	if err := yaml.Unmarshal([]byte(definition), &manifest); err != nil {
		return nil
	}
	scopes := make(map[string]struct{})
	for _, trigger := range manifest.Triggers {
		scopes[runtimeScopeForDisplay(trigger.Scope)] = struct{}{}
	}
	out := make([]string, 0, len(scopes))
	for scope := range scopes {
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i] == defaultRuntimeScope {
			return true
		}
		if out[j] == defaultRuntimeScope {
			return false
		}
		return out[i] < out[j]
	})
	return out
}

func normalizeRepositoryTriggerProvider(raw string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	provider = strings.ReplaceAll(provider, " ", "_")
	provider = strings.ReplaceAll(provider, "-", "_")
	switch provider {
	case "", repositoryTriggerProviderGitHub, "github_app":
		return repositoryTriggerProviderGitHub, nil
	case gitwebhook.ProviderGeneric, gitwebhook.ProviderGitLab, gitwebhook.ProviderBitbucket, gitwebhook.ProviderGitea:
		return provider, nil
	default:
		return "", fmt.Errorf("provider must be github, generic, gitlab, bitbucket, or gitea")
	}
}

func normalizeRepositoryTriggerManagement(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, "-", "_"))) {
	case "", repositoryTriggerManagementNopsAI, "nops_ai":
		return repositoryTriggerManagementNopsAI, nil
	case repositoryTriggerManagementRepository, "repo", "repository_file":
		return repositoryTriggerManagementRepository, nil
	default:
		return "", fmt.Errorf("management must be nopsai or repository")
	}
}

func normalizeRepositoryTriggerTeamPath(raw string) (string, error) {
	teamPath, err := normalizeRunTeamPath(raw)
	if err != nil {
		return "", fmt.Errorf("invalid team: %w", err)
	}
	return teamPath, nil
}

func normalizeTriggerSource(raw string) string {
	source := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case source == "" || source == "db" || source == "database":
		return "database"
	case strings.Contains(source, "git"):
		return "git"
	case strings.Contains(source, "setup"):
		return "setup"
	case strings.Contains(source, "draft"):
		return "draft"
	case strings.Contains(source, "local") || strings.Contains(source, "repository"):
		return "local"
	default:
		return source
	}
}

func fallbackRepositoryTriggerTeamPath(repositoryName string) string {
	if parent := repositoryParentPath(repositoryName); parent != "" {
		return parent
	}
	return globalGrantID
}

func repositoryTriggerConfigScope(record repositoryTriggerRecord) string {
	if teamPath := strings.Trim(strings.TrimSpace(record.TeamPath), "/"); teamPath != "" {
		return teamPath
	}
	return fallbackRepositoryTriggerTeamPath(record.RepositoryName)
}

func repositoryTriggerProviderRepository(repositoryName, teamPath string) string {
	repositoryName = strings.Trim(strings.TrimSpace(repositoryName), "/")
	teamPath = strings.Trim(strings.TrimSpace(teamPath), "/")
	if repositoryName == "" {
		return ""
	}
	if teamPath != "" && !isGlobalGrantResourceID(teamPath) && teamPath != rootGrantID && strings.HasPrefix(repositoryName, teamPath+"/") {
		return strings.Trim(strings.TrimPrefix(repositoryName, teamPath+"/"), "/")
	}
	return repositoryName
}

func validateRepositoryTriggerForNopsAI(record repositoryTriggerRecord) error {
	if record.Management == repositoryTriggerManagementRepository {
		return fmt.Errorf("repository-managed GitHub triggers are read from .nopsai/triggers.yaml; NopsAI trigger overrides must use management: nopsai")
	}
	if record.Provider == repositoryTriggerProviderGitHub && strings.TrimSpace(record.WebhookSourceID) != "" {
		return fmt.Errorf("GitHub App triggers use automatic ingress and must not set webhook_source")
	}
	if record.Provider != repositoryTriggerProviderGitHub && strings.TrimSpace(record.WebhookSourceID) == "" {
		return fmt.Errorf("webhook_source is required for non-GitHub repository triggers")
	}
	return nil
}

func validateRepositoryTriggerWebhookSource(ctx context.Context, queryer repositoryTriggerQueryer, record repositoryTriggerRecord) error {
	if err := validateRepositoryTriggerForNopsAI(record); err != nil {
		return err
	}
	if record.Provider == repositoryTriggerProviderGitHub {
		return nil
	}

	var provider, teamPath, visibility string
	var allowlistJSON []byte
	err := queryer.QueryRow(ctx, `
		SELECT provider, COALESCE(team_path, ''), COALESCE(visibility, 'team'), repository_allowlist
		FROM git_webhook_sources
		WHERE id = $1
	`, record.WebhookSourceID).Scan(&provider, &teamPath, &visibility, &allowlistJSON)
	if err != nil {
		return fmt.Errorf("webhook_source %q was not found", record.WebhookSourceID)
	}
	provider, err = normalizeRepositoryTriggerProvider(provider)
	if err != nil {
		return err
	}
	if provider != record.Provider {
		return fmt.Errorf("webhook_source %q provider is %s, but trigger provider is %s", record.WebhookSourceID, provider, record.Provider)
	}
	sourceTeam, err := normalizeRepositoryTriggerTeamPath(teamPath)
	if err != nil {
		return err
	}
	if normalizeGitWebhookSourceVisibility(visibility) != gitWebhookSourceVisibilityWorkspace {
		triggerTeam := strings.Trim(strings.TrimSpace(record.TeamPath), "/")
		if sourceTeam != triggerTeam && !IsSameTeamBoundary(sourceTeam, triggerTeam) {
			return fmt.Errorf("webhook_source %q is team-owned by %s and cannot be assigned to trigger team %s", record.WebhookSourceID, sourceTeam, triggerTeam)
		}
	}
	var allowlist []string
	_ = decodeJSONWithDefault(allowlistJSON, &allowlist, []string{})
	if !gitWebhookRepositoryAllowed(record.RepositoryForWebhook, allowlist) {
		return fmt.Errorf("webhook_source %q does not allow repository %s", record.WebhookSourceID, record.RepositoryForWebhook)
	}
	return nil
}

func repositoryTriggerIngress(provider, sourceName, sourceID string) string {
	if normalizeTriggerProviderString(provider) == repositoryTriggerProviderGitHub {
		return "GitHub App - automatic"
	}
	return firstNonEmptyString(sourceName, sourceID)
}

func normalizeTriggerProviderString(provider string) string {
	normalized, err := normalizeRepositoryTriggerProvider(provider)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(provider))
	}
	return normalized
}

func repositoryTriggerAllowlistStatus(ctx context.Context, queryer repositoryTriggerQueryer, record repositoryTriggerRecord) (string, string) {
	if record.Provider == repositoryTriggerProviderGitHub {
		return repositoryTriggerAllowlistAutomatic, ""
	}
	if strings.TrimSpace(record.WebhookSourceID) == "" {
		return repositoryTriggerAllowlistNotAssigned, ""
	}
	var sourceName string
	var allowlistJSON []byte
	err := queryer.QueryRow(ctx, `
		SELECT name, repository_allowlist
		FROM git_webhook_sources
		WHERE id = $1
	`, record.WebhookSourceID).Scan(&sourceName, &allowlistJSON)
	if err != nil {
		return repositoryTriggerAllowlistMissing, ""
	}
	var allowlist []string
	_ = decodeJSONWithDefault(allowlistJSON, &allowlist, []string{})
	if gitWebhookRepositoryAllowed(record.RepositoryForWebhook, allowlist) {
		return repositoryTriggerAllowlistAllowed, sourceName
	}
	return repositoryTriggerAllowlistDenied, sourceName
}
