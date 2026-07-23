package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nopsai/services/nopsai/internal/credentials"
)

type RunnerRegistryCredential struct {
	RunnerID              string
	CredentialRef         credentials.Reference
	RegistryHosts         []string
	Source                string
	ManagedByConfigRepo   bool
	ConfigRepoID          *int64
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
	CreatedBy             string
	UpdatedBy             string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type RunnerRegistryCredentialInput struct {
	CredentialRef         credentials.Reference
	RegistryHosts         []string
	Source                string
	ManagedByConfigRepo   bool
	ConfigRepoID          *int64
	ConfigSourcePath      string
	ConfigSourceCommitSHA string
	Actor                 string
}

func (s *PGStore) ReplaceRunnerRegistryCredentials(
	ctx context.Context,
	runnerID string,
	assignments []RunnerRegistryCredentialInput,
) error {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" {
		return errors.New("runner_id is required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM runner_registry_credentials WHERE runner_id = $1`, runnerID); err != nil {
		return err
	}
	for _, assignment := range assignments {
		if assignment.CredentialRef.String() == "" {
			return credentials.ErrInvalidReference
		}
		source := strings.TrimSpace(assignment.Source)
		if source == "" {
			source = "database"
		}
		actor := strings.TrimSpace(assignment.Actor)
		if _, err := tx.Exec(ctx, `
			INSERT INTO runner_registry_credentials (
				runner_id, credential_ref, registry_hosts, source, managed_by_config_repo,
				config_repo_id, config_source_path, config_source_commit_sha, created_by, updated_by
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		`,
			runnerID,
			assignment.CredentialRef.String(),
			registryHostsJSON(assignment.RegistryHosts),
			source,
			assignment.ManagedByConfigRepo,
			assignment.ConfigRepoID,
			strings.TrimSpace(assignment.ConfigSourcePath),
			strings.TrimSpace(assignment.ConfigSourceCommitSHA),
			actor,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *PGStore) ReplaceManagedRunnerRegistryCredentials(
	ctx context.Context,
	configRepoID int64,
	sourcePath string,
	assignments map[string][]RunnerRegistryCredentialInput,
) error {
	sourcePath = strings.TrimSpace(sourcePath)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		DELETE FROM runner_registry_credentials
		WHERE managed_by_config_repo = TRUE
		  AND config_repo_id = $1
		  AND config_source_path = $2
	`, configRepoID, sourcePath); err != nil {
		return err
	}
	for runnerID, runnerAssignments := range assignments {
		runnerID = strings.TrimSpace(runnerID)
		if runnerID == "" {
			return errors.New("runner_id is required")
		}
		for _, assignment := range runnerAssignments {
			if assignment.CredentialRef.String() == "" {
				return credentials.ErrInvalidReference
			}
			source := strings.TrimSpace(assignment.Source)
			if source == "" {
				source = "git"
			}
			actor := strings.TrimSpace(assignment.Actor)
			if _, err := tx.Exec(ctx, `
				INSERT INTO runner_registry_credentials (
					runner_id, credential_ref, registry_hosts, source, managed_by_config_repo,
					config_repo_id, config_source_path, config_source_commit_sha, created_by, updated_by
				)
				VALUES ($1, $2, $3, $4, TRUE, $5, $6, $7, $8, $8)
			`,
				runnerID,
				assignment.CredentialRef.String(),
				registryHostsJSON(assignment.RegistryHosts),
				source,
				configRepoID,
				sourcePath,
				strings.TrimSpace(assignment.ConfigSourceCommitSHA),
				actor,
			); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func (s *PGStore) ListRunnerRegistryCredentials(ctx context.Context, runnerID string) ([]RunnerRegistryCredential, error) {
	runnerID = strings.TrimSpace(runnerID)
	if runnerID == "" {
		return nil, errors.New("runner_id is required")
	}
	rows, err := s.db.Query(ctx, runnerRegistryCredentialSelectSQL+` WHERE runner_id = $1 ORDER BY credential_ref ASC`, runnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RunnerRegistryCredential{}
	for rows.Next() {
		record, err := scanRunnerRegistryCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (s *PGStore) GetRunnerRegistryCredential(
	ctx context.Context,
	runnerID string,
	ref credentials.Reference,
) (RunnerRegistryCredential, error) {
	record, err := scanRunnerRegistryCredential(s.db.QueryRow(
		ctx,
		runnerRegistryCredentialSelectSQL+` WHERE runner_id = $1 AND credential_ref = $2`,
		strings.TrimSpace(runnerID),
		ref.String(),
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return RunnerRegistryCredential{}, credentials.ErrNotFound
	}
	return record, err
}

const runnerRegistryCredentialSelectSQL = `
	SELECT runner_id, credential_ref, registry_hosts, source, managed_by_config_repo,
	       config_repo_id, config_source_path, config_source_commit_sha,
	       created_by, updated_by, created_at, updated_at
	FROM runner_registry_credentials`

func scanRunnerRegistryCredential(scanner interface{ Scan(dest ...any) error }) (RunnerRegistryCredential, error) {
	var record RunnerRegistryCredential
	var ref string
	var hostsRaw []byte
	var configRepoID sql.NullInt64
	err := scanner.Scan(
		&record.RunnerID,
		&ref,
		&hostsRaw,
		&record.Source,
		&record.ManagedByConfigRepo,
		&configRepoID,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
		&record.CreatedBy,
		&record.UpdatedBy,
		&record.CreatedAt,
		&record.UpdatedAt,
	)
	if err != nil {
		return RunnerRegistryCredential{}, err
	}
	parsedRef, err := credentials.ParseReference(ref)
	if err != nil {
		return RunnerRegistryCredential{}, err
	}
	record.CredentialRef = parsedRef
	record.RegistryHosts = parseRegistryHosts(hostsRaw)
	if configRepoID.Valid {
		value := configRepoID.Int64
		record.ConfigRepoID = &value
	}
	return record, nil
}

func registryHostsJSON(hosts []string) []byte {
	normalized := make([]string, 0, len(hosts))
	seen := map[string]struct{}{}
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		normalized = append(normalized, host)
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return []byte(`[]`)
	}
	return data
}

func parseRegistryHosts(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	var hosts []string
	if err := json.Unmarshal(data, &hosts); err != nil {
		return nil
	}
	return hosts
}
