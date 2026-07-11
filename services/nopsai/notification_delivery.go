package nopsai

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type pipelineNotificationContext struct {
	RunID                  string
	PipelineName           string
	PipelinePath           string
	PipelineDefinitionYAML string
	Status                 string
	TeamID                 int
	TeamPath               string
	RepoOwner              string
	RepoName               string
	RepoURL                string
	GitRef                 string
	GitCommitSHA           string
	GitCommitURL           string
	FailureReason          string
	TriggerSource          string
	StartedAt              time.Time
	FinishedAt             time.Time
	Duration               string
	Steps                  []pipelineNotificationStep
	LogExcerpt             []pipelineNotificationLogEntry
	FailureStep            string
	FailureTask            string
	RouteID                int64
	Definition             notificationRouteDefinition
}

func (a *App) dispatchPipelineRunNotification(runID, eventType string) {
	runID = strings.TrimSpace(runID)
	eventType = normalizeNotificationEventType(eventType)
	if runID == "" || eventType == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.deliverPipelineRunNotification(ctx, runID, eventType); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("event_type", eventType).Msg("Pipeline notification delivery skipped or failed")
	}
}

func (a *App) deliverPipelineRunNotification(ctx context.Context, runID, eventType string) error {
	notificationCtx, err := a.loadPipelineNotificationContext(ctx, runID)
	if err != nil {
		if err == pgx.ErrNoRows || err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if !notificationCtx.Definition.Enabled {
		return nil
	}
	mailSettings, err := a.loadNotificationMailSettings(ctx)
	if err != nil {
		return err
	}
	if !mailSettings.Enabled {
		return nil
	}

	if err := a.enrichPipelineNotificationContext(ctx, &notificationCtx, eventType); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("Pipeline notification enrichment was incomplete")
	}
	mailMessage, err := a.renderPipelineNotificationMail(notificationCtx, eventType)
	if err != nil {
		return err
	}
	for _, route := range notificationRouteRules(notificationCtx.Definition) {
		if !route.Enabled || !route.Events[eventType] {
			continue
		}
		if !pipelineNotificationFiltersMatch(route.Filters, notificationCtx) {
			continue
		}
		if !notificationChannelEnabled(route.Delivery.Channels, "mail") {
			continue
		}
		recipients, err := a.resolveNotificationRecipients(ctx, route.Recipients, notificationCtx.TeamPath)
		if err != nil {
			return err
		}
		for _, recipient := range recipients {
			if err := a.deliverPipelineNotificationEmail(ctx, notificationCtx, route, eventType, recipient, mailSettings.notificationMailSettingsFile, mailMessage); err != nil {
				log.Warn().Err(err).Str("run_id", runID).Str("event_type", eventType).Str("route", route.Name).Str("recipient", recipient).Msg("Failed to send pipeline notification mail")
			}
		}
	}
	return nil
}

func (a *App) loadPipelineNotificationContext(ctx context.Context, runID string) (pipelineNotificationContext, error) {
	var out pipelineNotificationContext
	var startedAt, finishedAt sql.NullTime
	err := a.db.QueryRow(ctx, `
		SELECT pr.run_id::text,
		       COALESCE(pr.pipeline_name, ''),
		       COALESCE(NULLIF(pr.pipeline_path, ''), COALESCE(pr.pipeline_name, '')),
		       COALESCE(pr.status, ''),
		       pr.team_id,
		       COALESCE(pr.git_repo_owner, ''),
		       COALESCE(pr.git_repo_name, ''),
		       COALESCE(g.repo_url, ''),
		       COALESCE(pr.git_ref, ''),
		       COALESCE(pr.git_commit_sha, ''),
		       COALESCE(pr.git_commit_url, ''),
		       COALESCE(pr.failure_reason, ''),
		       COALESCE(pr.trigger_source, ''),
		       pr.started_at,
		       pr.finished_at,
		       COALESCE(pr.pipeline_definition, '')
		FROM pipeline_runs pr
		LEFT JOIN teams g ON g.id = pr.team_id
		WHERE pr.run_id = $1::uuid
		  AND pr.team_id IS NOT NULL
	`, runID).Scan(
		&out.RunID,
		&out.PipelineName,
		&out.PipelinePath,
		&out.Status,
		&out.TeamID,
		&out.RepoOwner,
		&out.RepoName,
		&out.RepoURL,
		&out.GitRef,
		&out.GitCommitSHA,
		&out.GitCommitURL,
		&out.FailureReason,
		&out.TriggerSource,
		&startedAt,
		&finishedAt,
		&out.PipelineDefinitionYAML,
	)
	if err != nil {
		return pipelineNotificationContext{}, err
	}
	if startedAt.Valid {
		out.StartedAt = startedAt.Time
	}
	if finishedAt.Valid {
		out.FinishedAt = finishedAt.Time
	}
	if !out.StartedAt.IsZero() {
		finished := out.FinishedAt
		if finished.IsZero() {
			finished = time.Now()
		}
		out.Duration = finished.Sub(out.StartedAt).Round(time.Second).String()
	}

	teamRecords, err := loadTeamPathRecords(ctx, a.db)
	if err != nil {
		return pipelineNotificationContext{}, err
	}
	var teamLineage []int
	out.TeamPath, teamLineage, err = notificationTeamLineage(teamRecords, out.TeamID)
	if err != nil {
		return pipelineNotificationContext{}, err
	}

	var definitionRaw string
	err = a.db.QueryRow(ctx, `
		SELECT id, definition::text
		FROM notification_routes
		WHERE team_id = ANY($1::int[])
		ORDER BY array_position($1::int[], team_id)
		LIMIT 1
	`, teamLineage).Scan(&out.RouteID, &definitionRaw)
	if err != nil {
		return pipelineNotificationContext{}, err
	}
	var stored notificationRouteDefinition
	if err := json.Unmarshal([]byte(definitionRaw), &stored); err != nil {
		return pipelineNotificationContext{}, err
	}
	definition, err := normalizeNotificationRouteDefinition(notificationRouteDefinitionFileFromDefinition(stored))
	if err != nil {
		return pipelineNotificationContext{}, err
	}
	out.Definition = definition
	return out, nil
}

func notificationTeamLineage(records map[int]teamPathRecord, teamID int) (string, []int, error) {
	record, ok := records[teamID]
	if !ok {
		return "", nil, fmt.Errorf("notification team %d not found", teamID)
	}

	lineage := make([]int, 0, 4)
	visited := make(map[int]struct{}, 4)
	current := record
	for {
		if _, exists := visited[current.ID]; exists {
			return "", nil, fmt.Errorf("notification team hierarchy contains a cycle at team %d", current.ID)
		}
		visited[current.ID] = struct{}{}
		lineage = append(lineage, current.ID)
		if current.ParentID == nil {
			break
		}
		parent, exists := records[*current.ParentID]
		if !exists {
			return "", nil, fmt.Errorf("notification parent team %d not found", *current.ParentID)
		}
		current = parent
	}
	return record.Path, lineage, nil
}

func (a *App) resolveNotificationRecipients(ctx context.Context, recipients notificationRecipientsFile, teamPath string) ([]string, error) {
	include := map[string]struct{}{}
	exclude := map[string]struct{}{}
	for _, email := range recipients.Include.Users {
		if normalized := normalizeNotificationEmail(email); normalized != "" {
			include[normalized] = struct{}{}
		}
	}
	for _, team := range recipients.Include.Teams {
		if strings.EqualFold(strings.TrimSpace(team), "same_team") {
			if err := a.addNotificationTeamRecipients(ctx, include, teamPath); err != nil {
				return nil, err
			}
		}
	}
	for _, team := range recipients.Include.Teams {
		if err := a.addNotificationTeamRecipients(ctx, include, team); err != nil {
			return nil, err
		}
	}
	for _, email := range recipients.Exclude.Users {
		if normalized := normalizeNotificationEmail(email); normalized != "" {
			exclude[normalized] = struct{}{}
		}
	}
	for _, team := range recipients.Exclude.Teams {
		teamRecipients := map[string]struct{}{}
		if err := a.addNotificationTeamRecipients(ctx, teamRecipients, team); err != nil {
			return nil, err
		}
		for email := range teamRecipients {
			exclude[email] = struct{}{}
		}
	}
	for email := range exclude {
		delete(include, email)
	}
	out := make([]string, 0, len(include))
	for email := range include {
		out = append(out, email)
	}
	sort.Strings(out)
	return out, nil
}

func (a *App) addNotificationTeamRecipients(ctx context.Context, recipients map[string]struct{}, teamPath string) error {
	teamPath = normalizeNotificationTeamPath(teamPath)
	if teamPath == "" {
		return nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT LOWER(COALESCE(u.email, ''))
		FROM access_grants ag
		JOIN users u ON ag.subject_type = 'user'
		 AND (ag.subject_id = u.id::text OR ag.subject_id = u.sub OR ag.subject_id = u.email)
		WHERE ag.resource_type = 'team'
		  AND ag.resource_id = $1
		  AND u.status = 'active'
		  AND COALESCE(u.email, '') <> ''
	`, teamPath)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return err
		}
		if normalized := normalizeNotificationEmail(email); normalized != "" {
			recipients[normalized] = struct{}{}
		}
	}
	return rows.Err()
}

func (a *App) deliverPipelineNotificationEmail(ctx context.Context, notificationCtx pipelineNotificationContext, route notificationRouteRule, eventType, recipient string, settings notificationMailSettingsFile, mailMessage notificationMailMessage) error {
	if notificationDeliveryMaxReached(ctx, a, notificationCtx.RunID, "mail", recipient, route.Delivery.Throttle.MaxPerRun) {
		return nil
	}
	dedupeKey := notificationDeliveryDedupeKey(notificationCtx.RunID, eventType, "mail", recipient)
	var deliveryID uuid.UUID
	err := a.db.QueryRow(ctx, `
		INSERT INTO notification_deliveries (run_id, event_type, channel, recipient, status, dedupe_key)
		VALUES ($1::uuid, $2, 'mail', $3, 'pending', $4)
		ON CONFLICT (dedupe_key) DO NOTHING
		RETURNING id
	`, notificationCtx.RunID, eventType, recipient, dedupeKey).Scan(&deliveryID)
	if err != nil {
		if err == pgx.ErrNoRows || err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	sendErr := a.sendNotificationMailMessage(ctx, settings, []string{recipient}, mailMessage)
	status := "sent"
	errorMessage := ""
	if sendErr != nil {
		status = "failed"
		errorMessage = truncateNotificationDeliveryError(sendErr.Error())
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = $2, error = $3, sent_at = CASE WHEN $2 = 'sent' THEN NOW() ELSE sent_at END
		WHERE id = $1
	`, deliveryID, status, errorMessage); err != nil {
		return err
	}
	return sendErr
}

func notificationDeliveryMaxReached(ctx context.Context, a *App, runID, channel, recipient string, maxPerRun int) bool {
	if maxPerRun <= 0 {
		maxPerRun = 5
	}
	var count int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM notification_deliveries
		WHERE run_id = $1::uuid AND channel = $2 AND recipient = $3
	`, runID, channel, recipient).Scan(&count)
	return err == nil && count >= maxPerRun
}

func notificationDeliveryDedupeKey(runID, eventType, channel, recipient string) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{runID, eventType, channel, strings.ToLower(recipient)}, "|")))
	return hex.EncodeToString(hash[:])
}

func notificationChannelEnabled(channels []string, channel string) bool {
	for _, candidate := range channels {
		if strings.EqualFold(strings.TrimSpace(candidate), channel) {
			return true
		}
	}
	return false
}

func pipelineNotificationFiltersMatch(filters notificationRouteFiltersFile, notificationCtx pipelineNotificationContext) bool {
	repo := strings.Trim(strings.Join([]string{notificationCtx.RepoOwner, notificationCtx.RepoName}, "/"), "/")
	branch := notificationBranchLabel(notificationCtx.GitRef)
	return notificationPatternAllowsAny(filters.Pipelines, notificationCtx.PipelinePath, notificationCtx.PipelineName) &&
		notificationPatternAllowsAny(filters.Repos, repo) &&
		notificationPatternAllowsAny(filters.Branches, branch, notificationCtx.GitRef)
}

func notificationPatternAllowsAny(filter notificationPatternFilter, values ...string) bool {
	cleanValues := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleanValues = append(cleanValues, trimmed)
		}
	}
	if len(cleanValues) == 0 {
		cleanValues = append(cleanValues, "")
	}
	include := filter.Include
	if len(include) == 0 {
		include = []string{"*"}
	}
	included := false
	for _, pattern := range include {
		for _, value := range cleanValues {
			if notificationGlobMatch(pattern, value) {
				included = true
				break
			}
		}
		if included {
			break
		}
	}
	if !included {
		return false
	}
	for _, pattern := range filter.Exclude {
		for _, value := range cleanValues {
			if notificationGlobMatch(pattern, value) {
				return false
			}
		}
	}
	return true
}

func notificationGlobMatch(pattern, value string) bool {
	pattern = strings.TrimSpace(pattern)
	value = strings.TrimSpace(value)
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if matched, err := path.Match(pattern, value); err == nil && matched {
		return true
	}
	if strings.Contains(pattern, "*") {
		expr := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), "\\*", ".*") + "$"
		if matched, err := regexp.MatchString("(?i)"+expr, value); err == nil && matched {
			return true
		}
	}
	return strings.EqualFold(pattern, value)
}

func notificationBranchLabel(ref string) string {
	ref = strings.TrimSpace(ref)
	for _, prefix := range []string{"refs/heads/", "refs/tags/"} {
		if strings.HasPrefix(ref, prefix) {
			return strings.TrimPrefix(ref, prefix)
		}
	}
	return ref
}

func pipelineNotificationSubject(notificationCtx pipelineNotificationContext, eventType string) string {
	statusLabel, _, _, _ := pipelineNotificationPresentation(notificationCtx.Status, eventType)
	subject := fmt.Sprintf("[%s] %s", statusLabel, pipelineNotificationDisplayName(notificationCtx))
	if location := pipelineNotificationFailureLocationLabel(notificationCtx.FailureStep, notificationCtx.FailureTask); location != "" {
		subject += " - " + location
	}
	progress := summarizePipelineNotificationProgress(notificationCtx.Steps)
	if progress.Total > 0 {
		subject += fmt.Sprintf(" (%d/%d steps passed)", progress.Passed, progress.Total)
	}
	return subject
}

func pipelineNotificationBody(notificationCtx pipelineNotificationContext, eventType string) string {
	lines := []string{
		fmt.Sprintf("Event: %s", notificationEventLabel(eventType)),
		fmt.Sprintf("Run ID: %s", notificationCtx.RunID),
		fmt.Sprintf("Pipeline: %s", pipelineNotificationDisplayName(notificationCtx)),
		fmt.Sprintf("Status: %s", notificationCtx.Status),
		fmt.Sprintf("Team: %s", firstNonEmptyString(notificationCtx.TeamPath, "-")),
	}
	repo := strings.Trim(strings.Join([]string{notificationCtx.RepoOwner, notificationCtx.RepoName}, "/"), "/")
	if repo != "" {
		lines = append(lines, fmt.Sprintf("Repository: %s", repo))
	}
	if branch := notificationBranchLabel(notificationCtx.GitRef); branch != "" {
		lines = append(lines, fmt.Sprintf("Branch: %s", branch))
	}
	if notificationCtx.TriggerSource != "" {
		lines = append(lines, fmt.Sprintf("Trigger: %s", notificationCtx.TriggerSource))
	}
	if notificationCtx.FailureReason != "" {
		lines = append(lines, "", "Failure reason:", notificationCtx.FailureReason)
	}
	return strings.Join(lines, "\n")
}

func notificationEventLabel(eventType string) string {
	switch normalizeNotificationEventType(eventType) {
	case "waiting_approval":
		return "waiting approval"
	case "approval_requested":
		return "approval requested"
	case "approval_approved":
		return "approval approved"
	case "approval_rejected":
		return "approval rejected"
	default:
		return strings.ReplaceAll(normalizeNotificationEventType(eventType), "_", " ")
	}
}

func truncateNotificationDeliveryError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1000 {
		return value
	}
	return value[:1000]
}
