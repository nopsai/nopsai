package nopsai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

const gitWebhookSourceSelect = `
	SELECT id, name, description, provider, enabled, COALESCE(team_path, ''), auth_mode, credential_ref,
	       repository_allowlist, rate_limit, created_by, created_at, updated_at, last_used_at,
	       COALESCE(source, 'database'), config_repo_id, COALESCE(config_source_path, ''),
	       COALESCE(config_source_commit_sha, ''), managed_by_config_repo
	FROM git_webhook_sources`

func scanGitWebhookSource(scanner interface{ Scan(...any) error }) (gitWebhookSourceRecord, error) {
	var source gitWebhookSourceRecord
	var allowlistJSON, rateLimitJSON []byte
	var lastUsed sql.NullTime
	var configRepoID sql.NullInt64
	if err := scanner.Scan(
		&source.ID,
		&source.Name,
		&source.Description,
		&source.Provider,
		&source.Enabled,
		&source.TeamPath,
		&source.AuthMode,
		&source.CredentialRef,
		&allowlistJSON,
		&rateLimitJSON,
		&source.CreatedBy,
		&source.CreatedAt,
		&source.UpdatedAt,
		&lastUsed,
		&source.Source,
		&configRepoID,
		&source.ConfigSourcePath,
		&source.ConfigSourceCommitSHA,
		&source.ManagedByGitOps,
	); err != nil {
		return source, err
	}
	if lastUsed.Valid {
		source.LastUsedAt = &lastUsed.Time
	}
	if configRepoID.Valid {
		value := configRepoID.Int64
		source.ConfigRepoID = &value
	}
	_ = decodeJSONWithDefault(allowlistJSON, &source.RepositoryAllowlist, []string{})
	_ = decodeJSONWithDefault(rateLimitJSON, &source.RateLimit, map[string]any{})
	return source, nil
}

func (a *App) loadGitWebhookSource(ctx context.Context, id string) (gitWebhookSourceRecord, error) {
	return scanGitWebhookSource(a.db.QueryRow(ctx, gitWebhookSourceSelect+` WHERE id = $1`, strings.TrimSpace(id)))
}

func (a *App) insertGitWebhookDelivery(ctx context.Context, record gitWebhookDeliveryRecord) (bool, error) {
	runIDs, _ := json.Marshal(record.RunIDs)
	tag, err := a.db.Exec(ctx, `
		INSERT INTO git_webhook_deliveries (
			id, source_id, delivery_id, provider, event_type, repository_full_name,
			status, run_ids, error, source_ip
		)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10)
		ON CONFLICT (source_id, delivery_id) DO NOTHING
	`, record.ID, record.SourceID, record.DeliveryID, record.Provider, record.EventType,
		record.RepositoryFullName, record.Status, string(runIDs), record.Error, record.SourceIP)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (a *App) updateGitWebhookDelivery(ctx context.Context, id, status string, runIDs []string, message string) error {
	runIDsJSON, _ := json.Marshal(runIDs)
	_, err := a.db.Exec(ctx, `
		UPDATE git_webhook_deliveries
		SET status = $2,
		    run_ids = $3::jsonb,
		    error = $4,
		    completed_at = NOW()
		WHERE id = $1::uuid
	`, id, status, string(runIDsJSON), strings.TrimSpace(message))
	return err
}

func scanGitWebhookDelivery(scanner interface{ Scan(...any) error }) (gitWebhookDeliveryRecord, error) {
	var record gitWebhookDeliveryRecord
	var runIDsJSON []byte
	var completed sql.NullTime
	err := scanner.Scan(
		&record.ID,
		&record.SourceID,
		&record.DeliveryID,
		&record.Provider,
		&record.EventType,
		&record.RepositoryFullName,
		&record.Status,
		&runIDsJSON,
		&record.Error,
		&record.SourceIP,
		&record.ReceivedAt,
		&completed,
	)
	if err != nil {
		return record, err
	}
	if completed.Valid {
		record.CompletedAt = &completed.Time
	}
	_ = decodeJSONWithDefault(bytes.TrimSpace(runIDsJSON), &record.RunIDs, []string{})
	return record, nil
}

func isNotFoundError(err error) bool {
	return errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows)
}
