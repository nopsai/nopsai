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
		version BIGINT NOT NULL DEFAULT 1,
		source TEXT NOT NULL DEFAULT 'database',
		config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL,
		config_source_path TEXT NOT NULL DEFAULT '',
		config_source_commit_sha TEXT NOT NULL DEFAULT '',
		managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS payload JSONB NOT NULL DEFAULT '{}'::jsonb`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'database'`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS config_repo_id BIGINT REFERENCES config_repositories(id) ON DELETE SET NULL`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS config_source_path TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS config_source_commit_sha TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS managed_by_config_repo BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE runtime_settings ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`,
	`CREATE INDEX IF NOT EXISTS idx_runtime_settings_config_repo ON runtime_settings(config_repo_id)`,
}

type runtimeSettingsRecord struct {
	runtimeSettingsSnapshotFile `yaml:",inline"`
	Version                     int64      `json:"version,omitempty" yaml:"-"`
	Source                      string     `json:"source,omitempty" yaml:"-"`
	ConfigRepoID                *int64     `json:"config_repo_id,omitempty" yaml:"-"`
	ConfigSourcePath            string     `json:"config_source_path,omitempty" yaml:"-"`
	ConfigSourceCommitSHA       string     `json:"config_source_commit_sha,omitempty" yaml:"-"`
	ManagedByConfigRepo         bool       `json:"managed_by_config_repo" yaml:"-"`
	UpdatedAt                   *time.Time `json:"updated_at,omitempty" yaml:"-"`
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

func runtimeSettingsPayloadFromFile(file runtimeSettingsSnapshotFile) systemConfigPayload {
	payload := systemConfigPayload{
		LogLevel:                      file.LogLevel,
		LogFormat:                     file.LogFormat,
		Environment:                   file.Environment,
		PublicURL:                     file.PublicURL,
		CORSAllowedOrigins:            file.CORSAllowedOrigins,
		MetricsRequireAuth:            file.MetricsRequireAuth,
		NotificationMailLogoURL:       file.NotificationMailLogoURL,
		NotificationMailWebsiteURL:    file.NotificationMailWebsiteURL,
		NotificationMailSupportURL:    file.NotificationMailSupportURL,
		NotificationMailFooterAddress: file.NotificationMailFooterAddress,
		RequireProductionGates:        file.RequireProductionGates,
		NopsaiAPIURL:                  file.NopsaiAPIURL,
		DispatcherAddress:             file.DispatcherAddress,
		AgentImage:                    file.AgentImage,
		DockerNetworkName:             file.DockerNetworkName,
		AutoRemovalAgentContainer:     file.AutoRemovalAgentContainer,
		DefaultPipelineTimeout:        file.DefaultPipelineTimeout,
		RuntimeOutputMaxBytes:         file.RuntimeOutputMaxBytes,
		LLMAgentTimeout:               file.LLMAgentTimeout,
		DispatcherRouting:             file.DispatcherRouting,
		EjectedRunnerIDs:              file.EjectedRunnerIDs,
		RunnerID:                      file.RunnerID,
		RunnerScopes:                  file.RunnerScopes,
		RunnerCapacity:                file.RunnerCapacity,
		Runtime:                       file.Runtime,
		Kubernetes:                    file.Kubernetes,
		Limits:                        file.Limits,
		RuntimePools:                  file.RuntimePools,
		Assistant:                     file.Assistant,
		GitBotAPIURL:                  file.GitBotAPIURL,
		GitHubAppID:                   firstPresentStringPtr(file.AppID, file.GitHubAppID),
		GitHubAppSlug:                 file.AppSlug,
		GitHubWebhookURL:              file.WebhookURL,
		GitHubInstallationID:          file.GitHubInstallationID,
		GitHubPrivateKeyRef:           firstPresentStringPtr(file.PrivateKeyCredentialRef, file.GitHubPrivateKeyRef),
		GitHubWebhookRef:              firstPresentStringPtr(file.WebhookCredentialRef, file.GitHubWebhookRef),
	}
	if gitHubInstallationSettingsPresent(file.githubSettingsGitOpsFile) {
		installations := file.Installations
		if len(installations) == 0 {
			installations = file.GitHubInstallations
		}
		payload.GitHubInstallations = &installations
	}
	return payload
}

func gitHubInstallationSettingsPresent(file githubSettingsGitOpsFile) bool {
	return file.GitHubInstallationID != nil || file.Installations != nil || file.GitHubInstallations != nil
}

func firstPresentStringPtr(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func normalizeRuntimeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applySystemConfigToConfig(cfg *config.Config, payload systemConfigPayload) (config.Config, error) {
	if cfg == nil {
		return config.Config{}, fmt.Errorf("config is required")
	}
	if payload.RunnerCapacity != nil && *payload.RunnerCapacity <= 0 {
		return config.Config{}, fmt.Errorf("runner_capacity must be a positive integer")
	}
	if payload.RuntimeOutputMaxBytes != nil && *payload.RuntimeOutputMaxBytes <= 0 {
		return config.Config{}, fmt.Errorf("runtime_output_max_bytes must be a positive integer")
	}
	if payload.Limits != nil {
		if err := systemconfig.ValidateRunnerLimits(*payload.Limits); err != nil {
			return config.Config{}, err
		}
	}
	routing := systemconfig.NormalizeDispatcherRoutingConfig(payload.DispatcherRouting)

	if payload.LogLevel != nil {
		cfg.LogLevel = strings.TrimSpace(*payload.LogLevel)
	}
	if payload.LogFormat != nil {
		cfg.LogFormat = strings.TrimSpace(*payload.LogFormat)
	}
	if payload.Environment != nil {
		cfg.Environment = strings.TrimSpace(*payload.Environment)
	}
	if payload.LicenseKey != nil {
		cfg.LicenseKey = strings.TrimSpace(*payload.LicenseKey)
	}
	if payload.PublicURL != nil {
		cfg.PublicURL = strings.TrimSpace(*payload.PublicURL)
	}
	if payload.CORSAllowedOrigins != nil {
		cfg.CORSAllowedOrigins = normalizeRuntimeStringSlice(payload.CORSAllowedOrigins)
	}
	if payload.MetricsRequireAuth != nil {
		cfg.MetricsRequireAuth = *payload.MetricsRequireAuth
	}
	if payload.NotificationMailLogoURL != nil {
		cfg.NotificationMailLogoURL = strings.TrimSpace(*payload.NotificationMailLogoURL)
	}
	if payload.NotificationMailWebsiteURL != nil {
		cfg.NotificationMailWebsiteURL = strings.TrimSpace(*payload.NotificationMailWebsiteURL)
	}
	if payload.NotificationMailSupportURL != nil {
		cfg.NotificationMailSupportURL = strings.TrimSpace(*payload.NotificationMailSupportURL)
	}
	if payload.NotificationMailFooterAddress != nil {
		cfg.NotificationMailFooterAddress = strings.TrimSpace(*payload.NotificationMailFooterAddress)
	}
	if payload.RequireProductionGates != nil {
		cfg.RequireProductionGates = *payload.RequireProductionGates
	}
	if payload.NopsaiAPIURL != nil {
		cfg.NopsaiAPIURL = strings.TrimSpace(*payload.NopsaiAPIURL)
		cfg.AgentNopsaiAPIURL = cfg.NopsaiAPIURL
		cfg.GitBotNopsaiAPIURL = cfg.NopsaiAPIURL
	}
	if payload.GitBotAPIURL != nil {
		cfg.NopsaiGitBotAPIURL = strings.TrimSpace(*payload.GitBotAPIURL)
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
	if payload.RuntimeOutputMaxBytes != nil {
		cfg.RuntimeOutputMaxBytes = *payload.RuntimeOutputMaxBytes
	}
	if payload.LLMAgentTimeout != nil {
		cfg.LLMAgentTimeout = strings.TrimSpace(*payload.LLMAgentTimeout)
	}
	if payload.DispatcherRouting != nil {
		cfg.DispatcherRouting = routing
	}
	if payload.EjectedRunnerIDs != nil {
		cfg.EjectedRunnerIDs = config.NormalizeRunnerIDs(payload.EjectedRunnerIDs)
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
	if payload.GitHubAppSlug != nil {
		cfg.GitHubAppSlug = normalizeGitHubAppSlug(*payload.GitHubAppSlug)
	}
	if payload.GitHubAppOwner != nil {
		cfg.GitHubAppOwner = strings.TrimSpace(*payload.GitHubAppOwner)
	}
	if payload.GitHubWebhookURL != nil {
		cfg.GitHubWebhookURL = strings.TrimRight(strings.TrimSpace(*payload.GitHubWebhookURL), "/")
	}
	if payload.GitHubInstallationID != nil {
		cfg.GitHubInstallID = strings.TrimSpace(*payload.GitHubInstallationID)
	}
	if payload.GitHubInstallations != nil {
		cfg.GitHubInstallations = config.NormalizeGitHubInstallations(*payload.GitHubInstallations, cfg.GitHubInstallID)
		cfg.GitHubInstallID = ""
	} else {
		cfg.GitHubInstallations = config.NormalizeGitHubInstallations(cfg.GitHubInstallations, cfg.GitHubInstallID)
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
	if payload.Assistant != nil {
		cfg.Assistant = config.NormalizeAssistantConfig(*payload.Assistant)
	}
	cfg.DispatcherRouting, _ = systemconfig.RemoveRunnersFromDispatcherRouting(cfg.DispatcherRouting, cfg.EjectedRunnerIDs)
	cfg.NormalizeServiceTopology()

	return *cfg, nil
}

func applyRuntimeSettingsRecordToConfig(cfg *config.Config, record runtimeSettingsRecord) (config.Config, error) {
	return applySystemConfigToConfig(cfg, runtimeSettingsPayloadFromFile(record.runtimeSettingsSnapshotFile))
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
	payload := buildRuntimeSettingsSnapshotFile(cfg)
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal runtime settings: %w", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO runtime_settings (
			id, payload, version, source, config_repo_id, config_source_path,
			config_source_commit_sha, managed_by_config_repo, updated_at
		) VALUES (
			TRUE, $1::jsonb, 1, $2, $3, $4, $5, $6, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			payload = EXCLUDED.payload,
			version = runtime_settings.version + 1,
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
		SELECT payload, COALESCE(version, 1), COALESCE(source, 'database'), config_repo_id,
		       COALESCE(config_source_path, ''), COALESCE(config_source_commit_sha, ''),
		       managed_by_config_repo, updated_at
		FROM runtime_settings
		WHERE id = TRUE
	`).Scan(
		&raw,
		&record.Version,
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
		if err := json.Unmarshal(raw, &record.runtimeSettingsSnapshotFile); err != nil {
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
