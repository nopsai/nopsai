package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"nopsai/services/nopsai/internal/credentials"
)

type CredentialStore interface {
	CreateCredential(ctx context.Context, credential credentials.Credential) (credentials.Credential, error)
	UpsertCredentialMetadata(ctx context.Context, credential credentials.Credential) (credentials.Credential, error)
	GetCredentialByID(ctx context.Context, id uuid.UUID) (credentials.Credential, error)
	GetCredentialByReference(ctx context.Context, ref credentials.Reference) (credentials.Credential, error)
	ListCredentials(ctx context.Context) ([]credentials.Credential, error)
	ListCredentialVersions(ctx context.Context, credentialID uuid.UUID) ([]credentials.Version, error)
	ReserveCredentialVersion(ctx context.Context, credentialID uuid.UUID) (int, error)
	CreateCredentialVersion(ctx context.Context, credentialID uuid.UUID, version int, envelope credentials.Envelope, actor string, activate bool) (credentials.Version, error)
	ActivateCredentialVersion(ctx context.Context, credentialID uuid.UUID, version int, actor string) error
	DisableCredential(ctx context.Context, credentialID uuid.UUID, actor string) error
	EnableCredential(ctx context.Context, credentialID uuid.UUID, actor string) error
	DeleteCredentialVersion(ctx context.Context, credentialID uuid.UUID, version int) error
	DeleteCredential(ctx context.Context, credentialID uuid.UUID) error
	ResolveActiveCredential(ctx context.Context, ref credentials.Reference) (credentials.ResolvedRecord, error)
	RecordCredentialAccess(ctx context.Context, record credentials.AccessRecord) error
}

func (s *PGStore) CreateCredential(ctx context.Context, credential credentials.Credential) (credentials.Credential, error) {
	return scanCredential(s.db.QueryRow(ctx, `
		INSERT INTO credentials (
			id, namespace, name, kind, description, status, active_version, expires_at,
			managed_by_config_repo, config_repo_id, config_source_path, config_source_commit_sha,
			created_by, updated_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, 0, $7, $8, $9, $10, $11, $12, $12)
		RETURNING id, namespace, name, kind, description, status, active_version, expires_at,
		          last_rotated_at, managed_by_config_repo, config_repo_id, config_source_path,
		          config_source_commit_sha, created_by, updated_by, created_at, updated_at
	`,
		credential.ID,
		credential.Reference.Namespace,
		credential.Reference.Name,
		strings.ToLower(strings.TrimSpace(credential.Kind)),
		strings.TrimSpace(credential.Description),
		credentials.StatusPending,
		credential.ExpiresAt,
		credential.ManagedByConfigRepo,
		credential.ConfigRepoID,
		strings.TrimSpace(credential.ConfigSourcePath),
		strings.TrimSpace(credential.ConfigSourceCommitSHA),
		strings.TrimSpace(credential.CreatedBy),
	))
}

func (s *PGStore) UpsertCredentialMetadata(ctx context.Context, credential credentials.Credential) (credentials.Credential, error) {
	return scanCredential(s.db.QueryRow(ctx, `
		INSERT INTO credentials (
			id, namespace, name, kind, description, status, active_version, expires_at,
			managed_by_config_repo, config_repo_id, config_source_path, config_source_commit_sha,
			created_by, updated_by
		)
		VALUES ($1, $2, $3, $4, $5, 'pending', 0, $6, $7, $8, $9, $10, $11, $11)
		ON CONFLICT (namespace, name) DO UPDATE SET
			kind = EXCLUDED.kind,
			description = EXCLUDED.description,
			expires_at = EXCLUDED.expires_at,
			managed_by_config_repo = EXCLUDED.managed_by_config_repo,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
		RETURNING id, namespace, name, kind, description, status, active_version, expires_at,
		          last_rotated_at, managed_by_config_repo, config_repo_id, config_source_path,
		          config_source_commit_sha, created_by, updated_by, created_at, updated_at
	`,
		credential.ID,
		credential.Reference.Namespace,
		credential.Reference.Name,
		strings.ToLower(strings.TrimSpace(credential.Kind)),
		strings.TrimSpace(credential.Description),
		credential.ExpiresAt,
		credential.ManagedByConfigRepo,
		credential.ConfigRepoID,
		strings.TrimSpace(credential.ConfigSourcePath),
		strings.TrimSpace(credential.ConfigSourceCommitSHA),
		strings.TrimSpace(credential.UpdatedBy),
	))
}

func (s *PGStore) GetCredentialByID(ctx context.Context, id uuid.UUID) (credentials.Credential, error) {
	credential, err := scanCredential(s.db.QueryRow(ctx, credentialSelectSQL+` WHERE id = $1`, id))
	return mapCredentialStoreError(credential, err)
}

func (s *PGStore) GetCredentialByReference(ctx context.Context, ref credentials.Reference) (credentials.Credential, error) {
	credential, err := scanCredential(s.db.QueryRow(ctx, credentialSelectSQL+` WHERE namespace = $1 AND name = $2`, ref.Namespace, ref.Name))
	return mapCredentialStoreError(credential, err)
}

func (s *PGStore) ListCredentials(ctx context.Context) ([]credentials.Credential, error) {
	rows, err := s.db.Query(ctx, credentialSelectSQL+` ORDER BY namespace ASC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []credentials.Credential{}
	for rows.Next() {
		credential, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, credential)
	}
	return result, rows.Err()
}

func (s *PGStore) ListCredentialVersions(ctx context.Context, credentialID uuid.UUID) ([]credentials.Version, error) {
	rows, err := s.db.Query(ctx, `
		SELECT credential_id, version, ciphertext, wrapped_data_key, encryption_key_id,
		       encryption_format_version, created_by, created_at, activated_at, revoked_at
		FROM credential_versions
		WHERE credential_id = $1
		ORDER BY version DESC
	`, credentialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []credentials.Version{}
	for rows.Next() {
		version, err := scanCredentialVersion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, version)
	}
	return result, rows.Err()
}

func (s *PGStore) ReserveCredentialVersion(ctx context.Context, credentialID uuid.UUID) (int, error) {
	var version int
	err := s.db.QueryRow(ctx, `
		UPDATE credentials
		SET next_version = next_version + 1
		WHERE id = $1
		RETURNING next_version - 1
	`, credentialID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, credentials.ErrNotFound
	}
	return version, err
}

func (s *PGStore) CreateCredentialVersion(
	ctx context.Context,
	credentialID uuid.UUID,
	version int,
	envelope credentials.Envelope,
	actor string,
	activate bool,
) (credentials.Version, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return credentials.Version{}, err
	}
	defer tx.Rollback(ctx)

	if version <= 0 {
		return credentials.Version{}, errors.New("credential version must be positive")
	}

	var activatedAt any
	if activate {
		activatedAt = time.Now().UTC()
	}
	created, err := scanCredentialVersion(tx.QueryRow(ctx, `
		INSERT INTO credential_versions (
			credential_id, version, ciphertext, wrapped_data_key, encryption_key_id,
			encryption_format_version, created_by, activated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING credential_id, version, ciphertext, wrapped_data_key, encryption_key_id,
		          encryption_format_version, created_by, created_at, activated_at, revoked_at
	`, credentialID, version, envelope.Ciphertext, envelope.WrappedDataKey, envelope.EncryptionKeyID,
		envelope.EncryptionFormatVersion, strings.TrimSpace(actor), activatedAt))
	if err != nil {
		return credentials.Version{}, err
	}
	if activate {
		tag, err := tx.Exec(ctx, `
			UPDATE credentials
			SET active_version = $2, status = 'active', last_rotated_at = NOW(),
			    updated_by = $3, updated_at = NOW()
			WHERE id = $1
		`, credentialID, version, strings.TrimSpace(actor))
		if err != nil {
			return credentials.Version{}, err
		}
		if tag.RowsAffected() == 0 {
			return credentials.Version{}, credentials.ErrNotFound
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return credentials.Version{}, err
	}
	return created, nil
}

func (s *PGStore) ActivateCredentialVersion(ctx context.Context, credentialID uuid.UUID, version int, actor string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id
		FROM credentials
		WHERE id = $1
		FOR UPDATE
	`, credentialID).Scan(&currentID); errors.Is(err, pgx.ErrNoRows) {
		return credentials.ErrNotFound
	} else if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE credential_versions
		SET activated_at = COALESCE(activated_at, NOW()), revoked_at = NULL
		WHERE credential_id = $1 AND version = $2
	`, credentialID, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return credentials.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE credentials
		SET active_version = $2, status = 'active', last_rotated_at = NOW(),
		    updated_by = $3, updated_at = NOW()
		WHERE id = $1
	`, credentialID, version, strings.TrimSpace(actor)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PGStore) DisableCredential(ctx context.Context, credentialID uuid.UUID, actor string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE credentials
		SET status = 'disabled', updated_by = $2, updated_at = NOW()
		WHERE id = $1
	`, credentialID, strings.TrimSpace(actor))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return credentials.ErrNotFound
	}
	return nil
}

func (s *PGStore) EnableCredential(ctx context.Context, credentialID uuid.UUID, actor string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE credentials
		SET status = CASE WHEN active_version > 0 THEN 'active' ELSE 'pending' END,
		    updated_by = $2,
		    updated_at = NOW()
		WHERE id = $1
	`, credentialID, strings.TrimSpace(actor))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return credentials.ErrNotFound
	}
	return nil
}

func (s *PGStore) DeleteCredentialVersion(ctx context.Context, credentialID uuid.UUID, version int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var activeVersion, versionCount int
	if err := tx.QueryRow(ctx, `
		SELECT active_version, (
			SELECT COUNT(*)
			FROM credential_versions
			WHERE credential_id = credentials.id
		)
		FROM credentials
		WHERE id = $1
		FOR UPDATE
	`, credentialID).Scan(&activeVersion, &versionCount); errors.Is(err, pgx.ErrNoRows) {
		return credentials.ErrNotFound
	} else if err != nil {
		return err
	}

	var versionExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM credential_versions
			WHERE credential_id = $1 AND version = $2
		)
	`, credentialID, version).Scan(&versionExists); err != nil {
		return err
	}
	if !versionExists {
		return credentials.ErrNotFound
	}
	if version == activeVersion {
		return credentials.ErrActiveVersion
	}
	if versionCount < 2 {
		return credentials.ErrLastVersion
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM credential_versions
		WHERE credential_id = $1 AND version = $2
	`, credentialID, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PGStore) DeleteCredential(ctx context.Context, credentialID uuid.UUID) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM credentials WHERE id = $1`, credentialID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return credentials.ErrNotFound
	}
	return nil
}

func (s *PGStore) ResolveActiveCredential(ctx context.Context, ref credentials.Reference) (credentials.ResolvedRecord, error) {
	var record credentials.ResolvedRecord
	var expiresAt, lastRotatedAt, activatedAt, revokedAt sql.NullTime
	var configRepoID sql.NullInt64
	err := s.db.QueryRow(ctx, `
		SELECT c.id, c.namespace, c.name, c.kind, c.description, c.status, c.active_version,
		       c.expires_at, c.last_rotated_at, c.managed_by_config_repo, c.config_repo_id,
		       c.config_source_path, c.config_source_commit_sha, c.created_by, c.updated_by,
		       c.created_at, c.updated_at,
		       v.credential_id, v.version, v.ciphertext, v.wrapped_data_key, v.encryption_key_id,
		       v.encryption_format_version, v.created_by, v.created_at, v.activated_at, v.revoked_at
		FROM credentials c
		JOIN credential_versions v ON v.credential_id = c.id AND v.version = c.active_version
		WHERE c.namespace = $1 AND c.name = $2
	`, ref.Namespace, ref.Name).Scan(
		&record.Credential.ID,
		&record.Credential.Reference.Namespace,
		&record.Credential.Reference.Name,
		&record.Credential.Kind,
		&record.Credential.Description,
		&record.Credential.Status,
		&record.Credential.ActiveVersion,
		&expiresAt,
		&lastRotatedAt,
		&record.Credential.ManagedByConfigRepo,
		&configRepoID,
		&record.Credential.ConfigSourcePath,
		&record.Credential.ConfigSourceCommitSHA,
		&record.Credential.CreatedBy,
		&record.Credential.UpdatedBy,
		&record.Credential.CreatedAt,
		&record.Credential.UpdatedAt,
		&record.Version.CredentialID,
		&record.Version.Version,
		&record.Version.Envelope.Ciphertext,
		&record.Version.Envelope.WrappedDataKey,
		&record.Version.Envelope.EncryptionKeyID,
		&record.Version.Envelope.EncryptionFormatVersion,
		&record.Version.CreatedBy,
		&record.Version.CreatedAt,
		&activatedAt,
		&revokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return credentials.ResolvedRecord{}, credentials.ErrNotFound
	}
	if err != nil {
		return credentials.ResolvedRecord{}, err
	}
	applyCredentialNulls(&record.Credential, expiresAt, lastRotatedAt, configRepoID)
	if activatedAt.Valid {
		record.Version.ActivatedAt = &activatedAt.Time
	}
	if revokedAt.Valid {
		record.Version.RevokedAt = &revokedAt.Time
	}
	return record, nil
}

func (s *PGStore) RecordCredentialAccess(ctx context.Context, record credentials.AccessRecord) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO credential_access_logs (
			credential_id, version, consumer_service, purpose, subject_type, subject_id,
			correlation_id, success, error_code, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, COALESCE($10, NOW()))
	`, record.CredentialID, record.Version, record.ConsumerService, record.Purpose,
		record.SubjectType, record.SubjectID, record.CorrelationID, record.Success,
		record.ErrorCode, nullableTime(record.CreatedAt))
	return err
}

const credentialSelectSQL = `
	SELECT id, namespace, name, kind, description, status, active_version, expires_at,
	       last_rotated_at, managed_by_config_repo, config_repo_id, config_source_path,
	       config_source_commit_sha, created_by, updated_by, created_at, updated_at
	FROM credentials`

func scanCredential(scanner interface{ Scan(dest ...any) error }) (credentials.Credential, error) {
	var credential credentials.Credential
	var expiresAt, lastRotatedAt sql.NullTime
	var configRepoID sql.NullInt64
	err := scanner.Scan(
		&credential.ID,
		&credential.Reference.Namespace,
		&credential.Reference.Name,
		&credential.Kind,
		&credential.Description,
		&credential.Status,
		&credential.ActiveVersion,
		&expiresAt,
		&lastRotatedAt,
		&credential.ManagedByConfigRepo,
		&configRepoID,
		&credential.ConfigSourcePath,
		&credential.ConfigSourceCommitSHA,
		&credential.CreatedBy,
		&credential.UpdatedBy,
		&credential.CreatedAt,
		&credential.UpdatedAt,
	)
	if err != nil {
		return credentials.Credential{}, err
	}
	applyCredentialNulls(&credential, expiresAt, lastRotatedAt, configRepoID)
	return credential, nil
}

func scanCredentialVersion(scanner interface{ Scan(dest ...any) error }) (credentials.Version, error) {
	var version credentials.Version
	var activatedAt, revokedAt sql.NullTime
	err := scanner.Scan(
		&version.CredentialID,
		&version.Version,
		&version.Envelope.Ciphertext,
		&version.Envelope.WrappedDataKey,
		&version.Envelope.EncryptionKeyID,
		&version.Envelope.EncryptionFormatVersion,
		&version.CreatedBy,
		&version.CreatedAt,
		&activatedAt,
		&revokedAt,
	)
	if err != nil {
		return credentials.Version{}, err
	}
	if activatedAt.Valid {
		version.ActivatedAt = &activatedAt.Time
	}
	if revokedAt.Valid {
		version.RevokedAt = &revokedAt.Time
	}
	return version, nil
}

func applyCredentialNulls(credential *credentials.Credential, expiresAt, lastRotatedAt sql.NullTime, configRepoID sql.NullInt64) {
	if expiresAt.Valid {
		credential.ExpiresAt = &expiresAt.Time
	}
	if lastRotatedAt.Valid {
		credential.LastRotatedAt = &lastRotatedAt.Time
	}
	if configRepoID.Valid {
		value := configRepoID.Int64
		credential.ConfigRepoID = &value
	}
}

func mapCredentialStoreError(credential credentials.Credential, err error) (credentials.Credential, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return credentials.Credential{}, credentials.ErrNotFound
	}
	return credential, err
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func credentialStoreError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s credential: %w", operation, err)
}
