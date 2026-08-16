package nopsai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"nopsai/pkg/correlation"
	"nopsai/pkg/httpapi"
	aaamodel "nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/credentials"
	"nopsai/services/nopsai/internal/gitwebhook"
	"nopsai/services/nopsai/pkg/routeauthz"
)

const maxGitWebhookPayloadBytes = 5 << 20

func (a *App) handleListGitWebhookSources(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), gitWebhookSourceSelect+` ORDER BY updated_at DESC, id ASC`)
	if err != nil {
		http.Error(w, "failed to list git webhook sources", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sources := []gitWebhookSourceRecord{}
	resources := []aaamodel.ResourceRef{}
	for rows.Next() {
		source, err := scanGitWebhookSource(rows)
		if err != nil {
			http.Error(w, "failed to read git webhook sources", http.StatusInternalServerError)
			return
		}
		source, err = a.enrichGitWebhookSource(r.Context(), source)
		if err != nil {
			http.Error(w, "failed to read git webhook source connections", http.StatusInternalServerError)
			return
		}
		sources = append(sources, source)
		resources = append(resources, routeauthz.GitWebhookSourceResource(source.ID))
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read git webhook sources", http.StatusInternalServerError)
		return
	}
	allowed, err := a.allowedResourceSet(r, "git_webhook_source.read", resources)
	if err != nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	filtered := make([]gitWebhookSourceRecord, 0, len(sources))
	for _, source := range sources {
		if _, ok := allowed[resourceKey(routeauthz.GitWebhookSourceResource(source.ID))]; ok {
			filtered = append(filtered, source)
		}
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, filtered)
}

func (a *App) handleCreateGitWebhookSource(w http.ResponseWriter, r *http.Request) {
	var input gitWebhookSourceInput
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}
	source, err := normalizeGitWebhookSourceInputWithOptions(input, "", gitWebhookSourceNormalizeOptions{
		AllowGeneratedCredential: true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.requireTeamOwnedCreateDecision(w, r, "git_webhook_source.create", routeauthz.GitWebhookSourceResource(source.ID), source.TeamPath) {
		return
	}
	if err := a.ensureGitWebhookCredentialAllowed(r, subject, source); err != nil {
		writeGitWebhookCredentialPreparationError(w, err)
		return
	}
	source, generatedCredential, createdCredentialID, err := a.prepareGitWebhookSourceCredential(
		r.Context(),
		source,
		credentialActor(r),
	)
	if err != nil {
		writeGitWebhookCredentialPreparationError(w, err)
		return
	}
	allowlistJSON, _ := json.Marshal(source.RepositoryAllowlist)
	rateLimitJSON, _ := json.Marshal(source.RateLimit)
	createdBy := formatSubjectLabel(subject.Type, firstNonEmptyString(subject.ID, subject.Sub, subject.Email))
	_, err = a.db.Exec(r.Context(), `
		INSERT INTO git_webhook_sources (
			id, name, description, provider, enabled, team_path, visibility, auth_mode, credential_ref,
			repository_allowlist, rate_limit, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11::jsonb, $12)
	`, source.ID, source.Name, source.Description, source.Provider, source.Enabled, source.TeamPath,
		source.Visibility, source.AuthMode, source.CredentialRef, string(allowlistJSON), string(rateLimitJSON), createdBy)
	if err != nil {
		if createdCredentialID != nil {
			_ = a.credentials.Delete(r.Context(), *createdCredentialID, credentialActor(r))
		}
		if isUniqueViolation(err) {
			http.Error(w, "git webhook source already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to create git webhook source", http.StatusInternalServerError)
		return
	}
	created, err := a.loadGitWebhookSource(r.Context(), source.ID)
	if err != nil {
		http.Error(w, "failed to load git webhook source", http.StatusInternalServerError)
		return
	}
	created, _ = a.enrichGitWebhookSource(r.Context(), created)
	_ = httpapi.WriteJSON(w, http.StatusCreated, gitWebhookSourceCreateResponse{
		gitWebhookSourceRecord: created,
		GeneratedCredential:    generatedCredential,
	})
}

func (a *App) handleGetGitWebhookSource(w http.ResponseWriter, r *http.Request) {
	source, err := a.loadGitWebhookSource(r.Context(), r.PathValue("sourceID"))
	if err != nil {
		if isNotFoundError(err) {
			http.Error(w, "git webhook source not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load git webhook source", http.StatusInternalServerError)
		return
	}
	source, err = a.enrichGitWebhookSource(r.Context(), source)
	if err != nil {
		http.Error(w, "failed to load git webhook source connections", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, source)
}

func (a *App) handleUpdateGitWebhookSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("sourceID"))
	_, err := a.loadGitWebhookSource(r.Context(), id)
	if err != nil {
		if isNotFoundError(err) {
			http.Error(w, "git webhook source not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load git webhook source", http.StatusInternalServerError)
		return
	}
	var input gitWebhookSourceInput
	if err := httpapi.DecodeJSON(r, &input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	subject, ok := a.currentAAASubject(r)
	if !ok {
		http.Error(w, "missing authorization subject", http.StatusUnauthorized)
		return
	}
	source, err := normalizeGitWebhookSourceInput(input, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.ensureGitWebhookCredentialAllowed(r, subject, source); err != nil {
		writeGitWebhookCredentialPreparationError(w, err)
		return
	}
	if err := a.ensureCredentialReference(
		r.Context(),
		source.CredentialRef,
		gitWebhookSecretCredentialKind,
		"Webhook secret for Git source "+source.ID,
		credentialActor(r),
	); err != nil {
		http.Error(w, "failed to prepare webhook credential reference", http.StatusBadRequest)
		return
	}
	allowlistJSON, _ := json.Marshal(source.RepositoryAllowlist)
	rateLimitJSON, _ := json.Marshal(source.RateLimit)
	tag, err := a.db.Exec(r.Context(), `
		UPDATE git_webhook_sources
		SET name = $2,
		    description = $3,
		    provider = $4,
		    enabled = $5,
		    team_path = $6,
		    visibility = $7,
		    auth_mode = $8,
		    credential_ref = $9,
		    repository_allowlist = $10::jsonb,
		    rate_limit = $11::jsonb,
		    source = 'database',
		    config_repo_id = NULL,
		    config_source_path = '',
		    config_source_commit_sha = '',
		    managed_by_config_repo = FALSE,
		    updated_at = NOW()
	WHERE id = $1
	`, id, source.Name, source.Description, source.Provider, source.Enabled, source.TeamPath,
		source.Visibility, source.AuthMode, source.CredentialRef, string(allowlistJSON), string(rateLimitJSON))
	if err != nil {
		http.Error(w, "failed to update git webhook source", http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.Error(w, "git webhook source not found", http.StatusNotFound)
		return
	}
	updated, err := a.loadGitWebhookSource(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to load git webhook source", http.StatusInternalServerError)
		return
	}
	updated, _ = a.enrichGitWebhookSource(r.Context(), updated)
	_ = httpapi.WriteJSON(w, http.StatusOK, updated)
}

func (a *App) handleDeleteGitWebhookSource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("sourceID"))
	_, err := a.loadGitWebhookSource(r.Context(), id)
	if err != nil {
		if isNotFoundError(err) {
			http.Error(w, "git webhook source not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load git webhook source", http.StatusInternalServerError)
		return
	}
	if _, err := a.db.Exec(r.Context(), `DELETE FROM git_webhook_sources WHERE id = $1`, id); err != nil {
		http.Error(w, "failed to delete git webhook source", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListGitWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		SELECT id::text, source_id, delivery_id, provider, event_type, repository_full_name,
		       status, run_ids, error, source_ip, received_at, completed_at
		FROM git_webhook_deliveries
		WHERE source_id = $1
		ORDER BY received_at DESC
		LIMIT 100
	`, strings.TrimSpace(r.PathValue("sourceID")))
	if err != nil {
		http.Error(w, "failed to list git webhook deliveries", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	deliveries := []gitWebhookDeliveryRecord{}
	for rows.Next() {
		record, err := scanGitWebhookDelivery(rows)
		if err != nil {
			http.Error(w, "failed to read git webhook deliveries", http.StatusInternalServerError)
			return
		}
		deliveries = append(deliveries, record)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "failed to read git webhook deliveries", http.StatusInternalServerError)
		return
	}
	_ = httpapi.WriteJSON(w, http.StatusOK, deliveries)
}

func (a *App) handleGitWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	source, err := a.loadGitWebhookSource(r.Context(), r.PathValue("sourceID"))
	if err != nil {
		if isNotFoundError(err) {
			http.Error(w, "git webhook source not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to load git webhook source", http.StatusInternalServerError)
		return
	}
	if !source.Enabled {
		http.Error(w, "git webhook source is disabled", http.StatusForbidden)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxGitWebhookPayloadBytes+1))
	if err != nil {
		http.Error(w, "failed to read webhook payload", http.StatusBadRequest)
		return
	}
	if len(body) > maxGitWebhookPayloadBytes {
		http.Error(w, "webhook payload exceeds 5 MiB", http.StatusRequestEntityTooLarge)
		return
	}
	secret := ""
	if source.AuthMode != gitwebhook.AuthModeNone {
		secret, err = a.resolveCredentialText(r.Context(), source.CredentialRef, credentials.Purpose{
			ConsumerService: "nopsai",
			Operation:       "verify_git_webhook",
			SubjectType:     "git_webhook_source",
			SubjectID:       source.ID,
			CorrelationID:   requestIDFromContext(r.Context()),
		})
		if err != nil {
			log.Warn().Err(err).Str("source_id", source.ID).Msg("Failed to resolve git webhook credential")
			http.Error(w, "webhook credential is unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	if err := gitwebhook.Verify(source.Provider, source.AuthMode, secret, r.Header, body, time.Now()); err != nil {
		// This endpoint is unauthenticated: the specific reason distinguishes
		// auth mode, credential state and signature shape for an attacker who
		// only needs to know which knob to turn. Keep the detail in the log.
		log.Warn().Err(err).Str("source_id", source.ID).Msg("Git webhook verification failed")
		http.Error(w, "webhook verification failed", http.StatusUnauthorized)
		return
	}
	event, err := gitwebhook.Normalize(source.Provider, r.Header, body)
	if err != nil {
		if strings.HasPrefix(err.Error(), "ignored ") {
			_ = httpapi.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "ignored", "message": err.Error()})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !gitWebhookRepositoryAllowed(event.RepositoryFull, source.RepositoryAllowlist) {
		http.Error(w, "repository is not allowed for this git webhook source", http.StatusForbidden)
		return
	}

	deliveryUUID := uuid.New()
	delivery := gitWebhookDeliveryRecord{
		ID:                 deliveryUUID.String(),
		SourceID:           source.ID,
		DeliveryID:         event.DeliveryID,
		Provider:           source.Provider,
		EventType:          event.EventType,
		RepositoryFullName: event.RepositoryFull,
		Status:             gitWebhookDeliveryPending,
		SourceIP:           externalTriggerSourceIP(r),
	}
	inserted, err := a.insertGitWebhookDelivery(r.Context(), delivery)
	if err != nil {
		http.Error(w, "failed to record webhook delivery", http.StatusInternalServerError)
		return
	}
	if !inserted {
		_ = httpapi.WriteJSON(w, http.StatusOK, gitWebhookDeliveryResponse{
			DeliveryID:         event.DeliveryID,
			Status:             "duplicate",
			Provider:           source.Provider,
			EventType:          event.EventType,
			RepositoryFullName: event.RepositoryFull,
		})
		return
	}
	if exceeded, message, err := a.gitWebhookRateLimitExceeded(r.Context(), source.ID, source.RateLimit); err != nil {
		_ = a.updateGitWebhookDelivery(r.Context(), delivery.ID, gitWebhookDeliveryFailed, nil, "failed to validate rate limit")
		http.Error(w, "failed to validate webhook rate limit", http.StatusInternalServerError)
		return
	} else if exceeded {
		_ = a.updateGitWebhookDelivery(r.Context(), delivery.ID, gitWebhookDeliveryFailed, nil, message)
		http.Error(w, message, http.StatusTooManyRequests)
		return
	}

	result := a.dispatchGitWebhookEvent(r, source, event)
	message := strings.Join(result.Errors, "; ")
	if err := a.updateGitWebhookDelivery(r.Context(), delivery.ID, result.Status, result.RunIDs, message); err != nil {
		log.Warn().Err(err).Str("delivery_id", delivery.ID).Msg("Failed to finalize git webhook delivery audit")
	}
	_, _ = a.db.Exec(r.Context(), `UPDATE git_webhook_sources SET last_used_at = NOW() WHERE id = $1`, source.ID)
	response := gitWebhookDeliveryResponse{
		DeliveryID:         event.DeliveryID,
		Status:             result.Status,
		Provider:           source.Provider,
		EventType:          event.EventType,
		RepositoryFullName: event.RepositoryFull,
		MatchedPipelines:   result.MatchedPipelines,
		RunIDs:             result.RunIDs,
		Errors:             result.Errors,
	}
	statusCode := http.StatusAccepted
	switch result.Status {
	case gitWebhookDeliveryNoMatch:
		statusCode = http.StatusOK
	case gitWebhookDeliveryFailed:
		statusCode = result.HTTPStatus
		if statusCode < 400 {
			statusCode = http.StatusUnprocessableEntity
		}
	}
	_ = httpapi.WriteJSON(w, statusCode, response)
}

func (a *App) gitWebhookRateLimitExceeded(ctx context.Context, sourceID string, rateLimit map[string]any) (bool, string, error) {
	perMinute := intFromMap(rateLimit, "per_minute", "requests_per_minute", "deliveries_per_minute")
	if perMinute <= 0 {
		return false, "", nil
	}
	var count int
	err := a.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM git_webhook_deliveries
		WHERE source_id = $1
		  AND received_at > NOW() - INTERVAL '1 minute'
	`, sourceID).Scan(&count)
	if err != nil {
		return false, "", err
	}
	if count > perMinute {
		return true, "git webhook source rate limit exceeded", nil
	}
	return false, "", nil
}

func requestIDFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return correlation.RequestIDFromContext(ctx)
}
