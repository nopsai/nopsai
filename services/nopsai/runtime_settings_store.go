package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/services/nopsai/internal/systemconfig"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var runtimeSettingsSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS runtime_settings (
		id BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
		payload JSONB NOT NULL DEFAULT '{}'::jsonb,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_settings_config_repo ON runtime_settings(config_repo_id)`,
}

type runtimeSettingsRecord struct {
	runtimeSettingsGitOpsFile `yaml:",inline"`
	Source                    string     `json:"source,omitempty" yaml:"-"`
	ConfigRepoID              *int64     `json:"config_repo_id,omitempty" yaml:"-"`
	ConfigSourcePath          string     `json:"config_source_path,omitempty" yaml:"-"`
	ConfigSourceCommitSHA     string     `json:"config_source_commit_sha,omitempty" yaml:"-"`
	ManagedByConfigRepo       bool       `json:"managed_by_config_repo" yaml:"-"`
	UpdatedAt                 *time.Time `json:"updated_at,omitempty" yaml:"-"`
}

type runtimeSettingsQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func ensureRuntimeSettingsSchema(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin runtime settings schema transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for idx, stmt := range runtimeSettingsSchemaStatements {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply runtime settings schema statement %d: %w", idx+1, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit runtime settings schema transaction: %w", err)
	}
	return nil
}

func runtimeSettingsPayloadFromFile(file runtimeSettingsGitOpsFile) systemConfigPayload {
	return systemConfigPayload{
		AgentNopsaiAPIURL:         file.AgentNopsaiAPIURL,
		GitBotNopsaiAPIURL:        file.GitBotNopsaiAPIURL,
		NopsaiGitBotAPIURL:        file.NopsaiGitBotAPIURL,
		DispatcherAddress:         file.DispatcherAddress,
		AgentImage:                file.AgentImage,
		DockerNetworkName:         file.DockerNetworkName,
		AutoRemovalAgentContainer: file.AutoRemovalAgentContainer,
		DefaultPipelineTimeout:    file.DefaultPipelineTimeout,
		LLMAgentTimeout:           file.LLMAgentTimeout,
		DispatcherRouting:         file.DispatcherRouting,
		RunnerID:                  file.RunnerID,
		RunnerScopes:              file.RunnerScopes,
		RunnerCapacity:            file.RunnerCapacity,
		GitHubAppID:               file.GitHubAppID,
		GitHubInstallationID:      file.GitHubInstallationID,
		GitHubPrivateKeyRef:       file.GitHubPrivateKeyRef,
		GitHubWebhookRef:          file.GitHubWebhookRef,
		Runtime:                   file.Runtime,
		Kubernetes:                file.Kubernetes,
		Limits:                    file.Limits,
		RuntimePools:              file.RuntimePools,
	}
}

func applySystemConfigToConfig(cfg *config.Config, payload systemConfigPayload) (config.Config, error) {
	if cfg == nil {
		return config.Config{}, fmt.Errorf("config is required")
	}
	if payload.RunnerCapacity != nil && *payload.RunnerCapacity <= 0 {
		return config.Config{}, fmt.Errorf("runner_capacity must be a positive integer")
	}
	if payload.Limits != nil {
		if err := systemconfig.ValidateRunnerLimits(*payload.Limits); err != nil {
			return config.Config{}, err
		}
	}
	routing := systemconfig.NormalizeDispatcherRoutingConfig(payload.DispatcherRouting)

	if payload.AgentNopsaiAPIURL != nil {
		cfg.AgentNopsaiAPIURL = strings.TrimSpace(*payload.AgentNopsaiAPIURL)
	}
	if payload.GitBotNopsaiAPIURL != nil {
		cfg.GitBotNopsaiAPIURL = strings.TrimSpace(*payload.GitBotNopsaiAPIURL)
	}
	if payload.NopsaiGitBotAPIURL != nil {
		cfg.NopsaiGitBotAPIURL = strings.TrimSpace(*payload.NopsaiGitBotAPIURL)
	}
	if payload.DispatcherAddress != nil {
		cfg.DispatcherAddress = strings.TrimSpace(*payload.DispatcherAddress)
	}
	if payload.AgentImage != nil {
		cfg.AgentImage = strings.TrimSpace(*payload.AgentImage)
	}
	if payload.DockerNetworkName != nil {
		cfg.DockerNetworkName = strings.TrimSpace(*payload.DockerNetworkName)
	}
	if payload.AutoRemovalAgentContainer != nil {
		cfg.AutoRemovalAgentContainer = *payload.AutoRemovalAgentContainer
	}
	if payload.DefaultPipelineTimeout != nil {
		cfg.DefaultPipelineTimeout = strings.TrimSpace(*payload.DefaultPipelineTimeout)
	}
	if payload.LLMAgentTimeout != nil {
		cfg.LLMAgentTimeout = strings.TrimSpace(*payload.LLMAgentTimeout)
	}
	if payload.DispatcherRouting != nil {
		cfg.DispatcherRouting = routing
	}
	if payload.RunnerID != nil {
		cfg.RunnerID = strings.TrimSpace(*payload.RunnerID)
	}
	if payload.RunnerScopes != nil {
		cfg.RunnerScopes = systemconfig.NormalizeRunnerScopes(*payload.RunnerScopes)
	}
	if payload.RunnerCapacity != nil {
		cfg.RunnerCapacity = *payload.RunnerCapacity
	}
	if payload.GitHubAppID != nil {
		cfg.GitHubAppID = strings.TrimSpace(*payload.GitHubAppID)
	}
	if payload.GitHubInstallationID != nil {
		cfg.GitHubInstallID = strings.TrimSpace(*payload.GitHubInstallationID)
	}
	if payload.GitHubPrivateKeyRef != nil {
		cfg.GitHubPrivateKeyCredentialRef = strings.TrimSpace(*payload.GitHubPrivateKeyRef)
	}
	if payload.GitHubWebhookRef != nil {
		cfg.GitHubWebhookCredentialRef = strings.TrimSpace(*payload.GitHubWebhookRef)
	}
	if payload.Runtime != nil {
		cfg.Runtime = config.NormalizeRuntime(*payload.Runtime)
	}
	if payload.Kubernetes != nil {
		cfg.Kubernetes = config.NormalizeKubernetesConfig(*payload.Kubernetes)
	}
	if payload.Limits != nil {
		cfg.Limits = *payload.Limits
	}
	if payload.RuntimePools != nil {
		cfg.RuntimePools = config.NormalizeRuntimePools(payload.RuntimePools)
	}

	return *cfg, nil
}

func applyRuntimeSettingsRecordToConfig(cfg *config.Config, record runtimeSettingsRecord) (config.Config, error) {
	return applySystemConfigToConfig(cfg, runtimeSettingsPayloadFromFile(record.runtimeSettingsGitOpsFile))
}

func ApplyPersistedRuntimeSettings(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) error {
	if db == nil || cfg == nil {
		return nil
	}
	record, found, err := loadRuntimeSettingsRecord(ctx, db)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	_, err = applyRuntimeSettingsRecordToConfig(cfg, record)
	return err
}

func (a *App) persistRuntimeSettingsSnapshot(ctx context.Context, cfg config.Config, source string, configRepoID *int64, sourcePath, commitSHA string, managed bool) error {
	if a == nil || a.db == nil {
		return nil
	}
	return persistRuntimeSettingsSnapshotToDB(ctx, a.db, cfg, source, configRepoID, sourcePath, commitSHA, managed)
}

func persistRuntimeSettingsSnapshotToDB(ctx context.Context, db runtimeSettingsQuerier, cfg config.Config, source string, configRepoID *int64, sourcePath, commitSHA string, managed bool) error {
	if db == nil {
		return nil
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "database"
	}
	payload := buildRuntimeSettingsGitOpsFile(cfg)
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal runtime settings: %w", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO runtime_settings (
			id, payload, source, config_repo_id, config_source_path,
			config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES (
			TRUE, $1::jsonb, $2, $3, $4, $5, $6, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			payload = EXCLUDED.payload,
			source = EXCLUDED.source,
			config_repo_id = EXCLUDED.config_repo_id,
			config_source_path = EXCLUDED.config_source_path,
			config_source_commit_sha = EXCLUDED.config_source_commit_sha,
			managed_by_config_repo = EXCLUDED.managed_by_config_repo,
			updated_at = NOW()
	`, string(raw), source, configRepoID, strings.TrimSpace(sourcePath), strings.TrimSpace(commitSHA), managed)
	if err != nil {
		return fmt.Errorf("persist runtime settings: %w", err)
	}
	return nil
}

func loadRuntimeSettingsRecord(ctx context.Context, db runtimeSettingsQuerier) (runtimeSettingsRecord, bool, error) {
	if db == nil {
		return runtimeSettingsRecord{}, false, nil
	}
	var (
		record     runtimeSettingsRecord
		raw        []byte
		configRepo sql.NullInt64
		updatedAt  sql.NullTime
	)
	err := db.QueryRow(ctx, `
		SELECT payload, COALESCE(source, 'database'), config_repo_id,
		       COALESCE(config_source_path, ''), COALESCE(config_source_commit_sha, ''),
		       managed_by_config_repo, updated_at
		FROM runtime_settings
		WHERE id = TRUE
	`).Scan(
		&raw,
		&record.Source,
		&configRepo,
		&record.ConfigSourcePath,
		&record.ConfigSourceCommitSHA,
		&record.ManagedByConfigRepo,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return runtimeSettingsRecord{}, false, nil
	}
	if err != nil {
		return runtimeSettingsRecord{}, false, fmt.Errorf("load runtime settings: %w", err)
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &record.runtimeSettingsGitOpsFile); err != nil {
			return runtimeSettingsRecord{}, false, fmt.Errorf("parse persisted runtime settings: %w", err)
		}
	}
	if configRepo.Valid {
		id := configRepo.Int64
		record.ConfigRepoID = &id
	}
	if updatedAt.Valid {
		record.UpdatedAt = &updatedAt.Time
	}
	return record, true, nil
}
