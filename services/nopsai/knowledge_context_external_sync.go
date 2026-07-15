package nopsai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nopsai/pkg/models"
)

func (a *App) fetchAndStoreExternalKnowledgePage(ctx context.Context, detail knowledgeContextDetail, connection knowledgeConnectionRecord) (ExternalPage, error) {
	if connection.Disabled {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorUnavailable, 0, "Knowledge connection is disabled.")
	}
	provider, err := a.knowledgePageProvider(connection.Provider)
	if err != nil {
		return ExternalPage{}, err
	}
	var page ExternalPage
	if strings.TrimSpace(detail.ExternalPageID) != "" {
		page, err = provider.GetPage(ctx, connection, detail.ExternalPageID)
	} else if strings.TrimSpace(detail.ExternalPageURL) != "" {
		page, err = provider.ResolvePage(ctx, connection, detail.ExternalPageURL)
	} else {
		return ExternalPage{}, newKnowledgeProviderError(knowledgeProviderErrorInvalidRequest, 400, "external_page_id or external_page_url is required")
	}
	if err != nil {
		a.markExternalKnowledgeSyncFailure(ctx, detail, connection, err)
		return ExternalPage{}, err
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
	_, err = a.db.Exec(ctx, `
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
		    last_synced_at = $8,
		    updated_at = NOW()
		WHERE kind = $9 AND team_path = $10 AND name = $11
	`, page.Text, page.Hash, connection.Provider, page.ID, page.URL, page.Title, nullableTimePtr(page.ModifiedAt), now, detail.Kind, detail.Team, detail.Name)
	if err != nil {
		return ExternalPage{}, fmt.Errorf("store external knowledge page: %w", err)
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
		    updated_at = NOW()
		WHERE kind = $3 AND team_path = $4 AND name = $5
	`, status, message, detail.Kind, detail.Team, detail.Name)
	a.updateKnowledgeConnectionHealth(ctx, connection, knowledgeProviderErrorStatus(syncErr), message, nil)
}

func externalKnowledgeSyncStatus(err error) string {
	status := knowledgeProviderErrorStatus(err)
	switch status {
	case knowledgeConnectionStatusAuthenticationRequired:
		return "authentication_required"
	case knowledgeConnectionStatusPermissionDenied:
		return "permission_denied"
	case "page_unavailable":
		return "page_unavailable"
	default:
		return "failed"
	}
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
