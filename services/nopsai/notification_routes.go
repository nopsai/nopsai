package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
)

var (
	errNotificationGitOpsNotFound = errors.New("notification GitOps file not found")
	notificationEventTypes        = []string{
		"failure",
		"success",
		"pending",
		"running",
		"waiting_approval",
		"approval_requested",
		"approval_approved",
		"approval_rejected",
		"cancelled",
		"timed_out",
		"skipped",
	}
)

type notificationRecipientSetFile struct {
	Teams []string `json:"teams" yaml:"teams,omitempty"`
	Users []string `json:"users" yaml:"users,omitempty"`
}

type notificationRecipientsFile struct {
	Include notificationRecipientSetFile `json:"include" yaml:"include,omitempty"`
	Exclude notificationRecipientSetFile `json:"exclude" yaml:"exclude,omitempty"`
}

type notificationRouteFiltersFile struct {
	Pipelines notificationPatternFilter `json:"pipelines" yaml:"pipelines,omitempty"`
	Repos     notificationPatternFilter `json:"repos" yaml:"repos,omitempty"`
	Branches  notificationPatternFilter `json:"branches" yaml:"branches,omitempty"`
}

type notificationPatternFilter struct {
	Include []string `json:"include" yaml:"include,omitempty"`
	Exclude []string `json:"exclude" yaml:"exclude,omitempty"`
}

type notificationDeliveryFile struct {
	Channels []string                   `json:"channels" yaml:"channels,omitempty"`
	Throttle notificationThrottleConfig `json:"throttle" yaml:"throttle,omitempty"`
}

type notificationThrottleConfig struct {
	DedupeWindow string `json:"dedupe_window" yaml:"dedupe_window,omitempty"`
	MaxPerRun    int    `json:"max_per_run" yaml:"max_per_run,omitempty"`
}

type notificationRouteRuleFile struct {
	Name       string                       `json:"name" yaml:"name,omitempty"`
	Enabled    *bool                        `json:"enabled" yaml:"enabled,omitempty"`
	Recipients notificationRecipientsFile   `json:"recipients" yaml:"recipients,omitempty"`
	Events     map[string]bool              `json:"events" yaml:"events,omitempty"`
	Filters    notificationRouteFiltersFile `json:"filters" yaml:"filters,omitempty"`
	Delivery   notificationDeliveryFile     `json:"delivery" yaml:"delivery,omitempty"`
}

type notificationRouteDefinitionFile struct {
	Enabled    *bool                        `json:"enabled" yaml:"enabled,omitempty"`
	Recipients notificationRecipientsFile   `json:"recipients" yaml:"recipients,omitempty"`
	Events     map[string]bool              `json:"events" yaml:"events,omitempty"`
	Filters    notificationRouteFiltersFile `json:"filters" yaml:"filters,omitempty"`
	Delivery   notificationDeliveryFile     `json:"delivery" yaml:"delivery,omitempty"`
	Routes     []notificationRouteRuleFile  `json:"routes" yaml:"routes,omitempty"`
}

type notificationRouteRule struct {
	Name       string                       `json:"name" yaml:"name"`
	Enabled    bool                         `json:"enabled" yaml:"enabled"`
	Recipients notificationRecipientsFile   `json:"recipients" yaml:"recipients,omitempty"`
	Events     map[string]bool              `json:"events" yaml:"events"`
	Filters    notificationRouteFiltersFile `json:"filters" yaml:"filters,omitempty"`
	Delivery   notificationDeliveryFile     `json:"delivery" yaml:"delivery,omitempty"`
}

type notificationRouteDefinition struct {
	Enabled    bool                         `json:"enabled" yaml:"enabled"`
	Recipients notificationRecipientsFile   `json:"recipients" yaml:"recipients,omitempty"`
	Events     map[string]bool              `json:"events" yaml:"events"`
	Filters    notificationRouteFiltersFile `json:"filters" yaml:"filters,omitempty"`
	Delivery   notificationDeliveryFile     `json:"delivery" yaml:"delivery,omitempty"`
	Routes     []notificationRouteRule      `json:"routes" yaml:"routes,omitempty"`
}

type notificationRouteRecord struct {
	ID                    int64                       `json:"id,omitempty"`
	TeamID                int                         `json:"team_id"`
	TeamPath              string                      `json:"team_path"`
	Definition            notificationRouteDefinition `json:"definition"`
	Source                string                      `json:"source"`
	ConfigRepoID          *int64                      `json:"config_repo_id,omitempty"`
	ConfigSourcePath      string                      `json:"config_source_path,omitempty"`
	ConfigSourceCommitSHA string                      `json:"config_source_commit_sha,omitempty"`
	ManagedByConfigRepo   bool                        `json:"managed_by_config_repo"`
	UpdatedBy             string                      `json:"updated_by,omitempty"`
	UpdatedAt             time.Time                   `json:"updated_at"`
}

type storedNotificationRoute struct {
	teamPath   string
	definition notificationRouteDefinition
	sourcePath string
}

func defaultNotificationRouteDefinition() notificationRouteDefinition {
	route := defaultNotificationRouteRule("default")
	return notificationRouteDefinition{
		Enabled:    true,
		Recipients: route.Recipients,
		Events:     route.Events,
		Filters:    route.Filters,
		Delivery:   route.Delivery,
		Routes:     []notificationRouteRule{route},
	}
}

func defaultNotificationRouteRule(name string) notificationRouteRule {
	return notificationRouteDefinition{
		Enabled: true,
		Recipients: notificationRecipientsFile{
			Include: notificationRecipientSetFile{Teams: []string{"same_team"}},
		},
		Events: defaultNotificationEvents(),
		Filters: notificationRouteFiltersFile{
			Pipelines: notificationPatternFilter{Include: []string{"*"}},
			Repos:     notificationPatternFilter{Include: []string{"*"}},
			Branches:  notificationPatternFilter{Include: []string{"*"}},
		},
		Delivery: notificationDeliveryFile{
			Channels: []string{"mail"},
			Throttle: notificationThrottleConfig{
				DedupeWindow: "10m",
				MaxPerRun:    5,
			},
		},
	}.asRouteRule(name)
}

func defaultNotificationEvents() map[string]bool {
	events := map[string]bool{}
	for _, eventType := range notificationEventTypes {
		events[eventType] = false
	}
	for _, eventType := range []string{"failure", "waiting_approval", "approval_requested", "approval_rejected", "cancelled", "timed_out"} {
		events[eventType] = true
	}
	return events
}

func normalizeNotificationRouteDefinition(input notificationRouteDefinitionFile) (notificationRouteDefinition, error) {
	if len(input.Routes) > 0 {
		policyEnabled := true
		if input.Enabled != nil {
			policyEnabled = *input.Enabled
		}
		routes := make([]notificationRouteRule, 0, len(input.Routes))
		seenNames := map[string]struct{}{}
		for idx, routeInput := range input.Routes {
			route, err := normalizeNotificationRouteRule(routeInput, fmt.Sprintf("route-%d", idx+1))
			if err != nil {
				return notificationRouteDefinition{}, fmt.Errorf("route %d is invalid: %w", idx+1, err)
			}
			key := strings.ToLower(route.Name)
			if _, exists := seenNames[key]; exists {
				return notificationRouteDefinition{}, fmt.Errorf("duplicate notification route name %q", route.Name)
			}
			seenNames[key] = struct{}{}
			routes = append(routes, route)
		}
		definition := notificationRouteDefinition{
			Enabled: policyEnabled,
			Routes:  routes,
		}
		applyNotificationLegacyFieldsFromRule(&definition, routes[0])
		return definition, nil
	}

	routeInput := notificationRouteRuleFile{
		Name:       "default",
		Enabled:    input.Enabled,
		Recipients: input.Recipients,
		Events:     input.Events,
		Filters:    input.Filters,
		Delivery:   input.Delivery,
	}
	route, err := normalizeNotificationRouteRule(routeInput, "default")
	if err != nil {
		return notificationRouteDefinition{}, err
	}
	definition := notificationRouteDefinition{
		Enabled: route.Enabled,
		Routes:  []notificationRouteRule{route},
	}
	applyNotificationLegacyFieldsFromRule(&definition, route)
	return definition, nil
}

func normalizeNotificationRouteRule(input notificationRouteRuleFile, defaultName string) (notificationRouteRule, error) {
	route := defaultNotificationRouteRule(defaultName)
	route.Name = normalizeNotificationRouteName(firstNonEmptyString(input.Name, defaultName))
	if route.Name == "" {
		return notificationRouteRule{}, fmt.Errorf("name is required")
	}
	if input.Enabled != nil {
		route.Enabled = *input.Enabled
	}
	route.Recipients = notificationRecipientsFile{
		Include: normalizeNotificationRecipientSet(input.Recipients.Include),
		Exclude: normalizeNotificationRecipientSet(input.Recipients.Exclude),
	}
	if len(input.Events) > 0 {
		events := map[string]bool{}
		allowed := notificationEventTypeSet()
		for _, eventType := range notificationEventTypes {
			events[eventType] = false
		}
		for raw, enabled := range input.Events {
			eventType := normalizeNotificationEventType(raw)
			if _, ok := allowed[eventType]; !ok {
				return notificationRouteRule{}, fmt.Errorf("unsupported notification event %q", raw)
			}
			events[eventType] = enabled
		}
		route.Events = events
	}
	filters, err := normalizeNotificationFilters(input.Filters)
	if err != nil {
		return notificationRouteRule{}, err
	}
	route.Filters = filters
	delivery, err := normalizeNotificationDelivery(input.Delivery)
	if err != nil {
		return notificationRouteRule{}, err
	}
	route.Delivery = delivery
	return route, nil
}

func (definition notificationRouteDefinition) asRouteRule(name string) notificationRouteRule {
	return notificationRouteRule{
		Name:       normalizeNotificationRouteName(name),
		Enabled:    definition.Enabled,
		Recipients: definition.Recipients,
		Events:     definition.Events,
		Filters:    definition.Filters,
		Delivery:   definition.Delivery,
	}
}

func applyNotificationLegacyFieldsFromRule(definition *notificationRouteDefinition, route notificationRouteRule) {
	definition.Recipients = route.Recipients
	definition.Events = route.Events
	definition.Filters = route.Filters
	definition.Delivery = route.Delivery
}

func notificationRouteRules(definition notificationRouteDefinition) []notificationRouteRule {
	if len(definition.Routes) > 0 {
		return definition.Routes
	}
	return []notificationRouteRule{definition.asRouteRule("default")}
}

func normalizeNotificationRouteName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}

func notificationRouteDefinitionFileFromDefinition(definition notificationRouteDefinition) notificationRouteDefinitionFile {
	enabled := definition.Enabled
	return notificationRouteDefinitionFile{
		Enabled:    &enabled,
		Recipients: definition.Recipients,
		Events:     definition.Events,
		Filters:    definition.Filters,
		Delivery:   definition.Delivery,
		Routes:     notificationRouteRuleFilesFromRules(definition.Routes),
	}
}

func notificationRouteRuleFilesFromRules(routes []notificationRouteRule) []notificationRouteRuleFile {
	if len(routes) == 0 {
		return nil
	}
	out := make([]notificationRouteRuleFile, 0, len(routes))
	for _, route := range routes {
		enabled := route.Enabled
		out = append(out, notificationRouteRuleFile{
			Name:       route.Name,
			Enabled:    &enabled,
			Recipients: route.Recipients,
			Events:     route.Events,
			Filters:    route.Filters,
			Delivery:   route.Delivery,
		})
	}
	return out
}

func normalizeNotificationRecipientSet(input notificationRecipientSetFile) notificationRecipientSetFile {
	return notificationRecipientSetFile{
		Teams: normalizeNotificationStrings(input.Teams, normalizeNotificationTeamPath),
		Users: normalizeNotificationStrings(input.Users, normalizeNotificationEmail),
	}
}

func normalizeNotificationEmail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Address))
}

func normalizeNotificationTeamPath(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "/")
	value = filepath.ToSlash(value)
	if strings.Contains(value, "..") {
		return ""
	}
	return value
}

func normalizeNotificationStrings(values []string, normalizer func(string) string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizer(value)
		if normalized == "" {
			continue
		}
		key := strings.ToLower(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeNotificationFilters(input notificationRouteFiltersFile) (notificationRouteFiltersFile, error) {
	pipelines, err := normalizeNotificationPatternFilter(input.Pipelines)
	if err != nil {
		return notificationRouteFiltersFile{}, fmt.Errorf("invalid pipeline notification filter: %w", err)
	}
	repos, err := normalizeNotificationPatternFilter(input.Repos)
	if err != nil {
		return notificationRouteFiltersFile{}, fmt.Errorf("invalid repository notification filter: %w", err)
	}
	branches, err := normalizeNotificationPatternFilter(input.Branches)
	if err != nil {
		return notificationRouteFiltersFile{}, fmt.Errorf("invalid branch notification filter: %w", err)
	}
	return notificationRouteFiltersFile{Pipelines: pipelines, Repos: repos, Branches: branches}, nil
}

func normalizeNotificationPatternFilter(input notificationPatternFilter) (notificationPatternFilter, error) {
	include := normalizeNotificationPatterns(input.Include)
	exclude := normalizeNotificationPatterns(input.Exclude)
	if len(input.Include) > 0 && len(include) == 0 {
		return notificationPatternFilter{}, fmt.Errorf("include patterns are empty")
	}
	if len(input.Exclude) > 0 && len(exclude) == 0 {
		return notificationPatternFilter{}, fmt.Errorf("exclude patterns are empty")
	}
	if len(include) == 0 {
		include = []string{"*"}
	}
	return notificationPatternFilter{Include: include, Exclude: exclude}, nil
}

func normalizeNotificationPatterns(values []string) []string {
	return normalizeNotificationStrings(values, func(value string) string {
		value = strings.TrimSpace(filepath.ToSlash(value))
		if value == "" || strings.Contains(value, "..") {
			return ""
		}
		return value
	})
}

func normalizeNotificationDelivery(input notificationDeliveryFile) (notificationDeliveryFile, error) {
	channels := normalizeNotificationStrings(input.Channels, func(value string) string {
		return strings.ToLower(strings.TrimSpace(value))
	})
	if len(channels) == 0 {
		channels = []string{"mail"}
	}
	for _, channel := range channels {
		if channel != "mail" {
			return notificationDeliveryFile{}, fmt.Errorf("unsupported notification channel %q", channel)
		}
	}
	throttle := input.Throttle
	throttle.DedupeWindow = strings.TrimSpace(throttle.DedupeWindow)
	if throttle.DedupeWindow == "" {
		throttle.DedupeWindow = "10m"
	}
	if _, err := time.ParseDuration(throttle.DedupeWindow); err != nil {
		return notificationDeliveryFile{}, fmt.Errorf("invalid dedupe_window: %w", err)
	}
	if throttle.MaxPerRun <= 0 {
		throttle.MaxPerRun = 5
	}
	return notificationDeliveryFile{Channels: channels, Throttle: throttle}, nil
}

func normalizeNotificationEventType(raw string) string {
	eventType := strings.ToLower(strings.TrimSpace(raw))
	eventType = strings.ReplaceAll(eventType, "-", "_")
	eventType = strings.ReplaceAll(eventType, " ", "_")
	return eventType
}

func notificationEventTypeSet() map[string]struct{} {
	out := map[string]struct{}{}
	for _, eventType := range notificationEventTypes {
		out[eventType] = struct{}{}
	}
	return out
}

func parseNotificationRouteDefinition(content, sourcePath string) (notificationRouteDefinition, error) {
	var file notificationRouteDefinitionFile
	if err := yaml.Unmarshal([]byte(content), &file); err != nil {
		return notificationRouteDefinition{}, fmt.Errorf("failed to parse notification route '%s': %w", sourcePath, err)
	}
	definition, err := normalizeNotificationRouteDefinition(file)
	if err != nil {
		return notificationRouteDefinition{}, fmt.Errorf("invalid notification route '%s': %w", sourcePath, err)
	}
	return definition, nil
}

func parseGitOpsNotificationRoutes(files map[string]string, basePath string, binding models.ConfigRepository, boundTeam string) (map[string]storedNotificationRoute, error) {
	routes := map[string]storedNotificationRoute{}
	basePath = filepath.ToSlash(strings.Trim(basePath, "/"))
	rootRoutePath := configsync.RepoJoinPath(basePath, "notifications.yaml")

	for rawPath, content := range files {
		normalized := filepath.ToSlash(rawPath)
		if !isYAMLFile(normalized) {
			continue
		}

		if binding.ScopeType != models.ConfigRepositoryScopeTeam || normalized != rootRoutePath {
			continue
		}
		teamPath := boundTeam
		teamPath = normalizeNotificationTeamPath(teamPath)
		if teamPath == "" {
			return nil, fmt.Errorf("notification route '%s' must target a team path", normalized)
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam && !configsync.ResourceUnderScope(teamPath, boundTeam) {
			return nil, fmt.Errorf("notification route '%s' targets team '%s' outside bound team '%s'", normalized, teamPath, boundTeam)
		}
		definition, err := parseNotificationRouteDefinition(content, normalized)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(teamPath)
		if _, exists := routes[key]; exists {
			return nil, fmt.Errorf("duplicate notification route for team '%s' detected", teamPath)
		}
		routes[key] = storedNotificationRoute{
			teamPath:   teamPath,
			definition: definition,
			sourcePath: normalized,
		}
	}
	return routes, nil
}

func parseGitOpsConfigRepositoryNotificationRoutes(files map[string]string, configRepositoryDir string, binding models.ConfigRepository, boundTeam string) (map[string]storedNotificationRoute, error) {
	routes := map[string]storedNotificationRoute{}
	configRepositoryDir = filepath.ToSlash(strings.Trim(configRepositoryDir, "/"))

	for rawPath, content := range files {
		normalized := filepath.ToSlash(rawPath)
		rel, ok := configsync.RelativePath(normalized, configRepositoryDir)
		if !ok {
			continue
		}
		teamPath, ok, err := configRepositoryTeamNotificationRoutePath(rel)
		if err != nil {
			return nil, fmt.Errorf("invalid notification route path '%s': %w", normalized, err)
		}
		if !ok {
			continue
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam {
			teamPath, err = configsync.NormalizePathForTeam(boundTeam, teamPath)
			if err != nil {
				return nil, fmt.Errorf("invalid notification route path '%s': %w", normalized, err)
			}
		}
		teamPath = normalizeNotificationTeamPath(teamPath)
		if teamPath == "" {
			return nil, fmt.Errorf("notification route '%s' must target a team path", normalized)
		}
		if binding.ScopeType == models.ConfigRepositoryScopeTeam && !configsync.ResourceUnderScope(teamPath, boundTeam) {
			return nil, fmt.Errorf("notification route '%s' targets team '%s' outside bound team '%s'", normalized, teamPath, boundTeam)
		}
		definition, err := parseNotificationRouteDefinition(content, normalized)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(teamPath)
		if _, exists := routes[key]; exists {
			return nil, fmt.Errorf("duplicate notification route for team '%s' detected", teamPath)
		}
		routes[key] = storedNotificationRoute{
			teamPath:   teamPath,
			definition: definition,
			sourcePath: normalized,
		}
	}
	return routes, nil
}

func configRepositoryTeamNotificationRoutePath(rel string) (string, bool, error) {
	path := strings.Trim(strings.ReplaceAll(filepath.ToSlash(rel), "\\", "/"), "/")
	if path == "" || !isYAMLFile(path) {
		return "", false, nil
	}
	parts := strings.Split(path, "/")
	if len(parts) < 3 || parts[0] != "teams" {
		return "", false, nil
	}
	fileName := strings.ToLower(parts[len(parts)-1])
	if fileName != "notifications.yaml" && fileName != "notifications.yml" {
		return "", false, nil
	}
	teamPath := strings.Trim(strings.Join(parts[1:len(parts)-1], "/"), "/")
	if _, err := configsync.CleanPathSegments(teamPath, false); err != nil {
		return "", true, err
	}
	return teamPath, true, nil
}

func notificationRouteResourceScope(route storedNotificationRoute) string {
	return route.teamPath
}

func (a *App) handleGetTeamNotificationRoute(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.read")
	if !ok {
		return
	}
	teamID, err := notificationRouteTeamIDForPath(r.Context(), a.db, resource.ID)
	if err != nil {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	record, err := a.loadNotificationRouteByTeamID(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusOK, notificationRouteRecord{
				TeamID:     teamID,
				TeamPath:   resource.ID,
				Definition: defaultNotificationRouteDefinition(),
				Source:     "database",
				UpdatedAt:  time.Now(),
			})
			return
		}
		http.Error(w, "failed to load notification route", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (a *App) handleUpsertTeamNotificationRoute(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}
	teamID, err := notificationRouteTeamIDForPath(r.Context(), a.db, resource.ID)
	if err != nil {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	var req notificationRouteDefinitionFile
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid notification route payload", http.StatusBadRequest)
		return
	}
	definition, err := normalizeNotificationRouteDefinition(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	record, err := a.upsertNotificationRoute(r.Context(), teamID, definition, "database", nil, "", "", false, actorIDFromRequest(r))
	if err != nil {
		http.Error(w, "failed to save notification route", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (a *App) handleDeleteTeamNotificationRoute(w http.ResponseWriter, r *http.Request) {
	resource, ok := a.requireTeamConfigRepositoryDecision(w, r, "config_repo.manage")
	if !ok {
		return
	}
	teamID, err := notificationRouteTeamIDForPath(r.Context(), a.db, resource.ID)
	if err != nil {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}
	if _, err := a.db.Exec(r.Context(), "DELETE FROM notification_routes WHERE team_id = $1", teamID); err != nil {
		http.Error(w, "failed to delete notification route", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) loadNotificationRouteByTeamID(ctx context.Context, teamID int) (notificationRouteRecord, error) {
	return scanNotificationRoute(a.db.QueryRow(ctx, `
		SELECT nr.id, nr.team_id, g.name, nr.definition::text, COALESCE(nr.source, 'database'),
		       nr.config_repo_id, COALESCE(nr.config_source_path, ''), COALESCE(nr.config_source_commit_sha, ''),
		       nr.managed_by_config_repo, COALESCE(nr.updated_by, ''), nr.updated_at
		FROM notification_routes nr
		JOIN teams g ON g.id = nr.team_id
		WHERE nr.team_id = $1
	`, teamID))
}

func (a *App) upsertNotificationRoute(ctx context.Context, teamID int, definition notificationRouteDefinition, source string, configRepoID *int64, sourcePath, commitSHA string, managed bool, actor string) (notificationRouteRecord, error) {
	definitionJSON, err := json.Marshal(definition)
	if err != nil {
		return notificationRouteRecord{}, err
	}
	record, err := scanNotificationRoute(a.db.QueryRow(ctx, `
		INSERT INTO notification_routes (
			team_id, definition, source, config_repo_id, config_source_path,
			config_source_commit_sha, managed_by_config_repo, updated_by, updated_at
		) VALUES ($1, $2::jsonb, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (team_id) DO UPDATE SET
			definition = EXCLUDED.definition,
			source = EXCLUDED.source,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = EXCLUDED.managed_by_config_repo,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
		RETURNING id, team_id, (SELECT name FROM teams WHERE id = $1), definition::text,
		          COALESCE(source, 'database'), config_repo_id, COALESCE(config_source_path, ''),
		          COALESCE(config_source_commit_sha, ''), managed_by_config_repo, COALESCE(updated_by, ''), updated_at
	`, teamID, string(definitionJSON), source, configRepoID, sourcePath, commitSHA, managed, actor))
	return record, err
}

func scanNotificationRoute(row interface{ Scan(dest ...any) error }) (notificationRouteRecord, error) {
	var record notificationRouteRecord
	var definitionRaw string
	var configRepoID sql.NullInt64
	err := row.Scan(
		&record.ID,
		&record.TeamID,
		&record.TeamPath,
		&definitionRaw,
		&record.Source,
		&configRepoID,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
		&record.ManagedByConfigRepo,
		&record.UpdatedBy,
		&record.UpdatedAt,
	)
	if err != nil {
		return notificationRouteRecord{}, err
	}
	var stored notificationRouteDefinition
	if err := json.Unmarshal([]byte(definitionRaw), &stored); err != nil {
		return notificationRouteRecord{}, err
	}
	definition, err := normalizeNotificationRouteDefinition(notificationRouteDefinitionFileFromDefinition(stored))
	if err != nil {
		return notificationRouteRecord{}, err
	}
	record.Definition = definition
	if configRepoID.Valid {
		id := configRepoID.Int64
		record.ConfigRepoID = &id
	}
	return record, nil
}

func notificationRouteTeamIDForPath(ctx context.Context, runner interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, teamPath string) (int, error) {
	teamPath = normalizeNotificationTeamPath(teamPath)
	if teamPath == "" {
		return 0, fmt.Errorf("team path is required")
	}
	var teamID int
	if err := runner.QueryRow(ctx, "SELECT id FROM teams WHERE name = $1", teamPath).Scan(&teamID); err != nil {
		return 0, err
	}
	return teamID, nil
}

func sortedNotificationRoutes(routes map[string]storedNotificationRoute) []storedNotificationRoute {
	out := make([]storedNotificationRoute, 0, len(routes))
	for _, route := range routes {
		out = append(out, route)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].teamPath) < strings.ToLower(out[j].teamPath)
	})
	return out
}

func notificationRouteResourceRef(teamPath string) aaamodel.ResourceRef {
	return aaamodel.ResourceRef{Type: grantResourceTeam, ID: normalizeNotificationTeamPath(teamPath)}
}
