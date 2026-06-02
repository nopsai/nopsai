package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"nopsai/pkg/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	db *pgxpool.Pool
}

func NewPGStore(db *pgxpool.Pool) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) GetRunListItem(ctx context.Context, runID string) (*models.RunListItem, error) {
	var run models.RunListItem
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, pipelineSource, pipelineVersion, pipelinePath, triggerEventID, triggerSource, scheduleID, scheduleName, schedulePath sql.NullString

	query := `
        SELECT
		    pr.run_id, pr.pipeline_name, pr.pipeline_path, pr.pipeline_version, pr.status, COALESCE(pr.git_commit_sha, ''),
            COALESCE(pr.git_repo_owner, ''), COALESCE(pr.git_repo_name, ''), pr.started_at, pr.finished_at,
			pr.parent_run_id, COALESCE(pr.git_pusher_name, ''), COALESCE(pr.git_ref, ''), COALESCE(pr.git_target_ref, ''),
			COALESCE(pr.pipeline_source, ''), COALESCE(pr.trigger_event_id, ''),
			COALESCE(pr.trigger_source, ''), COALESCE(pr.schedule_id::text, ''), COALESCE(ps.name, ''), COALESCE(ps.path, '')
        FROM pipeline_runs pr
		LEFT JOIN pipeline_schedules ps ON ps.id = pr.schedule_id
        WHERE pr.run_id = $1
    `
	err := s.db.QueryRow(ctx, query, runID).Scan(
		&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &gitRef, &gitTargetRef, &pipelineSource, &triggerEventID,
		&triggerSource, &scheduleID, &scheduleName, &schedulePath,
	)

	if err != nil {
		return nil, err
	}

	run.PipelinePath = pipelinePath.String
	run.GitCommitSHA = commitSHA.String
	run.PipelineVersion = normalizePipelineVersion(pipelineVersion.String)
	run.GitRepoOwner = repoOwner.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	run.GitRef = gitRef.String
	run.GitTargetRef = gitTargetRef.String
	run.PipelineSource = pipelineSource.String
	run.TriggerEventID = triggerEventID.String
	run.TriggerSource = triggerSource.String
	run.ScheduleID = scheduleID.String
	run.ScheduleName = scheduleName.String
	run.SchedulePath = schedulePath.String

	if startedAt.Valid {
		run.StartedAt = startedAt.Time
		if finishedAt.Valid {
			run.FinishedAt = finishedAt.Time
			run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
			run.IsComplete = true
		} else {
			run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
			run.IsComplete = isTerminalRunStatus(run.Status)
		}
	} else {
		run.Duration = "0s"
		run.IsComplete = isTerminalRunStatus(run.Status)
	}

	return &run, nil
}

var (
	ErrConfigRepositoryNotFound = errors.New("config repository not found")
	ErrConfigRepositoryConflict = errors.New("config repository conflict")
)

func (s *PGStore) CreateOrUpdateConfigRepository(ctx context.Context, input models.ConfigRepositoryInput) (models.ConfigRepository, error) {
	scopeType := strings.TrimSpace(input.ScopeType)
	scopeID := strings.Trim(strings.TrimSpace(input.ScopeID), "/")
	repoURL := strings.TrimSpace(input.RepoURL)
	branch := strings.TrimSpace(input.Branch)
	if branch == "" {
		branch = "main"
	}
	basePath := normalizeConfigRepositoryBasePath(input.BasePath)
	writeBranch := strings.TrimSpace(input.WriteBranch)
	actor := strings.TrimSpace(input.Actor)

	const query = `
		INSERT INTO config_repositories (
			scope_type, scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		ON CONFLICT (scope_type, scope_id) DO UPDATE SET
			repo_url = EXCLUDED.repo_url,
			branch = EXCLUDED.branch,
			base_path = EXCLUDED.base_path,
			enabled = EXCLUDED.enabled,
			write_enabled = EXCLUDED.write_enabled,
			write_branch = EXCLUDED.write_branch,
			config_repo_id = NULL,
			config_source_path = '',
			config_source_commit_sha = '',
			managed_by_config_repo = FALSE,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
		RETURNING
			id, scope_type, scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
			last_sync_status, last_sync_message, last_sync_started_at, last_sync_completed_at,
			last_sync_commit_sha, created_by, updated_by, created_at, updated_at
	`
	row := s.db.QueryRow(ctx, query, scopeType, scopeID, repoURL, branch, basePath, input.Enabled, input.WriteEnabled, writeBranch, actor)
	repo, err := scanConfigRepository(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return models.ConfigRepository{}, ErrConfigRepositoryConflict
		}
		return models.ConfigRepository{}, err
	}
	return repo, nil
}

func (s *PGStore) GetConfigRepositoryByScope(ctx context.Context, scopeType, scopeID string) (models.ConfigRepository, error) {
	const query = `
		SELECT
			id, scope_type, scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
			last_sync_status, last_sync_message, last_sync_started_at, last_sync_completed_at,
			last_sync_commit_sha, created_by, updated_by, created_at, updated_at
		FROM config_repositories
		WHERE scope_type = $1 AND scope_id = $2
	`
	repo, err := scanConfigRepository(s.db.QueryRow(ctx, query, strings.TrimSpace(scopeType), strings.Trim(strings.TrimSpace(scopeID), "/")))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ConfigRepository{}, ErrConfigRepositoryNotFound
		}
		return models.ConfigRepository{}, err
	}
	return repo, nil
}

func (s *PGStore) DeleteConfigRepositoryByScope(ctx context.Context, scopeType, scopeID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.Trim(strings.TrimSpace(scopeID), "/")

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var repoID int64
	if err := tx.QueryRow(ctx, `
		SELECT id FROM config_repositories WHERE scope_type = $1 AND scope_id = $2
	`, scopeType, scopeID).Scan(&repoID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrConfigRepositoryNotFound
		}
		return err
	}

	for _, tableName := range configManagedResourceTables {
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE %s
			SET config_repo_id = NULL,
				config_source_path = '',
				config_source_commit_sha = '',
				managed_by_config_repo = FALSE
			WHERE config_repo_id = $1
		`, tableName), repoID); err != nil {
			return err
		}
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM config_repositories WHERE scope_type = $1 AND scope_id = $2
	`, scopeType, scopeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConfigRepositoryNotFound
	}

	return tx.Commit(ctx)
}

func (s *PGStore) ListConfigRepositories(ctx context.Context, filter models.ConfigRepositoryFilter) ([]models.ConfigRepository, error) {
	conditions := []string{"1 = 1"}
	args := []any{}
	if strings.TrimSpace(filter.ScopeType) != "" {
		args = append(args, strings.TrimSpace(filter.ScopeType))
		conditions = append(conditions, fmt.Sprintf("scope_type = $%d", len(args)))
	}
	if strings.TrimSpace(filter.ScopeID) != "" {
		args = append(args, strings.Trim(strings.TrimSpace(filter.ScopeID), "/"))
		conditions = append(conditions, fmt.Sprintf("scope_id = $%d", len(args)))
	}
	if filter.Enabled != nil {
		args = append(args, *filter.Enabled)
		conditions = append(conditions, fmt.Sprintf("enabled = $%d", len(args)))
	}

	query := `
		SELECT
			id, scope_type, scope_id, repo_url, branch, base_path, enabled, write_enabled, write_branch,
			config_repo_id, config_source_path, config_source_commit_sha, managed_by_config_repo,
			last_sync_status, last_sync_message, last_sync_started_at, last_sync_completed_at,
			last_sync_commit_sha, created_by, updated_by, created_at, updated_at
		FROM config_repositories
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY scope_type ASC, scope_id ASC
	`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []models.ConfigRepository
	for rows.Next() {
		repo, err := scanConfigRepository(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return repos, nil
}

func (s *PGStore) UpdateConfigRepositorySyncStatus(ctx context.Context, id int64, status, message, commitSHA string, startedAt, completedAt *time.Time) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE config_repositories
		SET last_sync_status = $2,
			last_sync_message = $3,
			last_sync_commit_sha = $4,
			last_sync_started_at = COALESCE($5, last_sync_started_at),
			last_sync_completed_at = $6,
			updated_at = NOW()
		WHERE id = $1
	`, id, strings.TrimSpace(status), strings.TrimSpace(message), strings.TrimSpace(commitSHA), startedAt, completedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConfigRepositoryNotFound
	}
	return nil
}

var configManagedResourceTables = []string{"config_repositories", "pipelines", "steps", "pipeline_schedules", "triggers", "external_triggers", "variables", "secrets", "knowledge_contexts"}

type configRepositoryScanner interface {
	Scan(dest ...any) error
}

func scanConfigRepository(row configRepositoryScanner) (models.ConfigRepository, error) {
	var repo models.ConfigRepository
	var startedAt, completedAt sql.NullTime
	var configRepoID sql.NullInt64
	err := row.Scan(
		&repo.ID,
		&repo.ScopeType,
		&repo.ScopeID,
		&repo.RepoURL,
		&repo.Branch,
		&repo.BasePath,
		&repo.Enabled,
		&repo.WriteEnabled,
		&repo.WriteBranch,
		&configRepoID,
		&repo.ConfigSourcePath,
		&repo.ConfigSourceCommitSHA,
		&repo.ManagedByConfigRepo,
		&repo.LastSyncStatus,
		&repo.LastSyncMessage,
		&startedAt,
		&completedAt,
		&repo.LastSyncCommitSHA,
		&repo.CreatedBy,
		&repo.UpdatedBy,
		&repo.CreatedAt,
		&repo.UpdatedAt,
	)
	if err != nil {
		return models.ConfigRepository{}, err
	}
	if configRepoID.Valid {
		id := configRepoID.Int64
		repo.ConfigRepoID = &id
	}
	if startedAt.Valid {
		repo.LastSyncStartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		repo.LastSyncCompletedAt = &completedAt.Time
	}
	return repo, nil
}

func normalizeConfigRepositoryBasePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.Trim(value, "/")
	if value == "." {
		return ""
	}
	return value
}

// Helpers needed for GetRunListItem
// In a full refactor, these might be in a utils package or shared method
func normalizePipelineVersion(version string) string {
	if version == "" {
		return "latest"
	}
	return version
}

func isTerminalRunStatus(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "success" || status == "failure" || status == "cancelled" || status == "timed_out" || status == "rejected"
}
