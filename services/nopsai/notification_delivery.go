package main

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
	RunID         string
	PipelineName  string
	PipelinePath  string
	Status        string
	GroupPath     string
	RepoOwner     string
	RepoName      string
	GitRef        string
	FailureReason string
	TriggerSource string
	RouteID       int64
	Definition    notificationRouteDefinition
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

	subject := pipelineNotificationSubject(notificationCtx, eventType)
	body := pipelineNotificationBody(notificationCtx, eventType)
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
		recipients, err := a.resolveNotificationRecipients(ctx, route.Recipients, notificationCtx.GroupPath)
		if err != nil {
			return err
		}
		for _, recipient := range recipients {
			if err := a.deliverPipelineNotificationEmail(ctx, notificationCtx, route, eventType, recipient, mailSettings.notificationMailSettingsFile, subject, body); err != nil {
				log.Warn().Err(err).Str("run_id", runID).Str("event_type", eventType).Str("route", route.Name).Str("recipient", recipient).Msg("Failed to send pipeline notification mail")
			}
		}
	}
	return nil
}

func (a *App) loadPipelineNotificationContext(ctx context.Context, runID string) (pipelineNotificationContext, error) {
	var out pipelineNotificationContext
	var definitionRaw string
	err := a.db.QueryRow(ctx, `
		SELECT pr.run_id::text,
		       COALESCE(pr.pipeline_name, ''),
		       COALESCE(NULLIF(pr.pipeline_path, ''), COALESCE(pr.pipeline_name, '')),
		       COALESCE(pr.status, ''),
		       COALESCE(g.name, ''),
		       COALESCE(pr.git_repo_owner, ''),
		       COALESCE(pr.git_repo_name, ''),
		       COALESCE(pr.git_ref, ''),
		       COALESCE(pr.failure_reason, ''),
		       COALESCE(pr.trigger_source, ''),
		       nr.id,
		       nr.definition::text
		FROM pipeline_runs pr
		JOIN notification_routes nr ON nr.group_id = pr.group_id
		LEFT JOIN groups g ON g.id = pr.group_id
		WHERE pr.run_id = $1::uuid
	`, runID).Scan(
		&out.RunID,
		&out.PipelineName,
		&out.PipelinePath,
		&out.Status,
		&out.GroupPath,
		&out.RepoOwner,
		&out.RepoName,
		&out.GitRef,
		&out.FailureReason,
		&out.TriggerSource,
		&out.RouteID,
		&definitionRaw,
	)
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

func (a *App) resolveNotificationRecipients(ctx context.Context, recipients notificationRecipientsFile, groupPath string) ([]string, error) {
	include := map[string]struct{}{}
	exclude := map[string]struct{}{}
	for _, email := range recipients.Include.Users {
		if normalized := normalizeNotificationEmail(email); normalized != "" {
			include[normalized] = struct{}{}
		}
	}
	for _, team := range recipients.Include.Teams {
		if strings.EqualFold(strings.TrimSpace(team), "same_group") {
			if err := a.addNotificationGroupRecipients(ctx, include, groupPath); err != nil {
				return nil, err
			}
		}
	}
	for _, group := range recipients.Include.Groups {
		if err := a.addNotificationGroupRecipients(ctx, include, group); err != nil {
			return nil, err
		}
	}
	for _, email := range recipients.Exclude.Users {
		if normalized := normalizeNotificationEmail(email); normalized != "" {
			exclude[normalized] = struct{}{}
		}
	}
	for _, group := range recipients.Exclude.Groups {
		groupRecipients := map[string]struct{}{}
		if err := a.addNotificationGroupRecipients(ctx, groupRecipients, group); err != nil {
			return nil, err
		}
		for email := range groupRecipients {
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

func (a *App) addNotificationGroupRecipients(ctx context.Context, recipients map[string]struct{}, groupPath string) error {
	groupPath = normalizeNotificationGroupPath(groupPath)
	if groupPath == "" {
		return nil
	}
	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT LOWER(COALESCE(u.email, ''))
		FROM access_grants ag
		JOIN users u ON ag.subject_type = 'user'
		 AND (ag.subject_id = u.id::text OR ag.subject_id = u.sub OR ag.subject_id = u.email)
		WHERE ag.resource_type = 'folder'
		  AND ag.resource_id = $1
		  AND u.status = 'active'
		  AND COALESCE(u.email, '') <> ''
	`, groupPath)
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

func (a *App) deliverPipelineNotificationEmail(ctx context.Context, notificationCtx pipelineNotificationContext, route notificationRouteRule, eventType, recipient string, settings notificationMailSettingsFile, subject, body string) error {
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
	sendErr := sendNotificationMail(ctx, settings, []string{recipient}, subject, body)
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
	pipeline := firstNonEmptyString(notificationCtx.PipelinePath, notificationCtx.PipelineName, "pipeline")
	return fmt.Sprintf("NopsAI pipeline %s: %s", notificationEventLabel(eventType), pipeline)
}

func pipelineNotificationBody(notificationCtx pipelineNotificationContext, eventType string) string {
	lines := []string{
		fmt.Sprintf("Event: %s", notificationEventLabel(eventType)),
		fmt.Sprintf("Run ID: %s", notificationCtx.RunID),
		fmt.Sprintf("Pipeline: %s", firstNonEmptyString(notificationCtx.PipelinePath, notificationCtx.PipelineName, "-")),
		fmt.Sprintf("Status: %s", notificationCtx.Status),
		fmt.Sprintf("Group: %s", firstNonEmptyString(notificationCtx.GroupPath, "-")),
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
