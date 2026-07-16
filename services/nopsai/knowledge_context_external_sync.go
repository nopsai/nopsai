package nopsai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nopsai/pkg/models"
)

func (a *App) fetchAndStoreExternalKnowledgePage(ctx context.Context, detail knowledgeContextDetail, connection knowledgeConnectionRecord) (ExternalPage, error) {
	if connection.Disabled {
		err := newKnowledgeProviderError(knowledgeProviderErrorDisabled, 0, "Knowledge connection is disabled.")
		a.markExternalKnowledgeSyncFailure(ctx, detail, connection, err)
		return ExternalPage{}, err
	}
	provider, err := a.knowledgePageProvider(connection.Provider)
	if err != nil {
		a.markExternalKnowledgeSyncFailure(ctx, detail, connection, err)
		return ExternalPage{}, err
	}
	var page ExternalPage
	if strings.TrimSpace(detail.ExternalPageID) != "" {
		page, err = provider.GetPage(ctx, connection, detail.ExternalPageID)
	} else if strings.TrimSpace(detail.ExternalPageURL) != "" {
		page, err = provider.ResolvePage(ctx, connection, detail.ExternalPageURL)
	} else {
		err := newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, 400, "external_page_id or external_page_url is required")
		a.markExternalKnowledgeSyncFailure(ctx, detail, connection, err)
		return ExternalPage{}, err
	}
	if err != nil {
		a.markExternalKnowledgeSyncFailure(ctx, detail, connection, err)
		return ExternalPage{}, err
	}
	if strings.TrimSpace(page.Text) == "" && len(page.Assets) > 0 {
		page.Text = preservedAssetOnlyContent(page.Assets)
	}
	if strings.TrimSpace(page.Text) == "" {
		err := newKnowledgeProviderError(knowledgeProviderErrorPageUnavailable, 404, "Provider page returned empty content.")
		a.markExternalKnowledgeSyncFailure(ctx, detail, connection, err)
		return ExternalPage{}, err
	}
	if page.Hash == "" {
		page.Hash = hashKnowledgeText(page.Text)
	}
	now := time.Now().UTC()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return ExternalPage{}, fmt.Errorf("begin external knowledge sync transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	var knowledgeID string
	err = tx.QueryRow(ctx, `
		UPDATE knowledge_contexts
		SET content = $1,
		    synced_content = $1,
		    content_hash = $2,
		    source = $3,
		    content_source = 'external_page',
		    external_provider = $3,
		    external_page_id = CASE WHEN $4 <> '' THEN $4 ELSE external_page_id END,
		    external_page_url = CASE WHEN $5 <> '' THEN $5 ELSE external_page_url END,
		    external_page_title = $6,
		    source_modified_at = $7,
			    sync_status = 'up_to_date',
			    last_sync_status = 'up_to_date',
			    sync_error = '',
			    last_sync_error = '',
			    last_sync_started_at = NULL,
			    sync_attempt_count = 0,
			    next_sync_attempt_at = CASE
			        WHEN sync_mode = 'periodic' THEN $8::timestamptz + make_interval(mins => CASE WHEN sync_interval_minutes > 0 THEN sync_interval_minutes ELSE $12 END)
			        ELSE NULL
			    END,
			    last_synced_at = $8,
			    updated_at = NOW()
			WHERE kind = $9 AND team_path = $10 AND name = $11
			RETURNING id::text
		`, page.Text, page.Hash, connection.Provider, page.ID, page.URL, page.Title, nullableTimePtr(page.ModifiedAt), now, detail.Kind, detail.Team, detail.Name, defaultKnowledgeSyncIntervalMinutes).Scan(&knowledgeID)
	if err != nil {
		return ExternalPage{}, fmt.Errorf("store external knowledge page: %w", err)
	}
	if err := a.replaceKnowledgeContextAssets(ctx, tx, knowledgeID, connection.Provider, page); err != nil {
		return ExternalPage{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExternalPage{}, fmt.Errorf("commit external knowledge sync transaction: %w", err)
	}
	a.updateKnowledgeConnectionHealth(ctx, connection, knowledgeConnectionStatusConnected, "", &now)
	return page, nil
}

func (a *App) markExternalKnowledgeSyncFailure(ctx context.Context, detail knowledgeContextDetail, connection knowledgeConnectionRecord, syncErr error) {
	status := externalKnowledgeSyncStatus(syncErr)
	message := syncErr.Error()
	_, _ = a.db.Exec(ctx, `
		UPDATE knowledge_contexts
			SET sync_status = $1,
			    last_sync_status = $1,
			    sync_error = $2,
			    last_sync_error = $2,
			    last_sync_started_at = NULL,
			    sync_attempt_count = sync_attempt_count + 1,
			    next_sync_attempt_at = CASE
			        WHEN sync_mode = 'periodic' THEN
			            CASE
			                WHEN sync_attempt_count + 1 >= $3 THEN NOW() + make_interval(mins => CASE WHEN sync_interval_minutes > 0 THEN sync_interval_minutes ELSE $4 END)
			                WHEN sync_attempt_count <= 0 THEN NOW() + INTERVAL '1 minute'
			                WHEN sync_attempt_count = 1 THEN NOW() + INTERVAL '2 minutes'
			                WHEN sync_attempt_count = 2 THEN NOW() + INTERVAL '4 minutes'
			                ELSE NOW() + INTERVAL '15 minutes'
			            END
			        ELSE NULL
			    END,
			    updated_at = NOW()
			WHERE kind = $5 AND team_path = $6 AND name = $7
		`, status, message, knowledgeSyncMaxRetryAttempts, defaultKnowledgeSyncIntervalMinutes, detail.Kind, detail.Team, detail.Name)
	a.updateKnowledgeConnectionHealth(ctx, connection, knowledgeProviderErrorStatus(syncErr), message, nil)
}

func externalKnowledgeSyncStatus(err error) string {
	status := knowledgeProviderErrorStatus(err)
	switch status {
	case knowledgeConnectionStatusAuthenticationRequired:
		return "authentication_required"
	case knowledgeConnectionStatusPermissionDenied:
		return "permission_denied"
	case knowledgeConnectionStatusDisabled:
		return "connection_disabled"
	case "page_unavailable":
		return "page_unavailable"
	}
	var providerErr knowledgeProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Kind {
		case knowledgeProviderErrorPageTooLarge:
			return "page_too_large"
		case knowledgeProviderErrorInvalidRequest:
			return "invalid_request"
		}
	}
	return "failed"
}

func nullableTimePtr(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	return value
}

func applyExternalKnowledgeFailureMode(snapshot models.KnowledgeContextSnapshot, ref models.KnowledgeContextRef, failureMode string, _ error) (models.KnowledgeContextSnapshot, bool) {
	switch strings.TrimSpace(failureMode) {
	case knowledgeFailureModeUseCached:
		if strings.TrimSpace(snapshot.Content) != "" {
			return snapshot, true
		}
	case knowledgeFailureModeSkip:
		if !ref.Required {
			return models.KnowledgeContextSnapshot{}, false
		}
	}
	return models.KnowledgeContextSnapshot{}, false
}
