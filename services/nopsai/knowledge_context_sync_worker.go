package nopsai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	knowledgeSyncWorkerPollInterval = time.Minute
	knowledgeSyncWorkerBatchSize    = 8
	knowledgeSyncWorkerTimeout      = 45 * time.Second
	knowledgeSyncStuckAfter         = 10 * time.Minute
	knowledgeSyncMaxRetryAttempts   = 3
	knowledgeProviderRateLimitDelay = 350 * time.Millisecond
)

type knowledgePeriodicSyncJob struct {
	UUID       string
	Kind       string
	Team       string
	Name       string
	Provider   string
	Connection string
}

func (j knowledgePeriodicSyncJob) ID() string {
	return buildKnowledgeContextIdentifier(j.Kind, j.Team, j.Name)
}

type knowledgeSyncMetricKey struct {
	Provider  string
	Mode      string
	Operation string
	Result    string
}

type knowledgeSyncMetrics struct {
	mu              sync.Mutex
	attempts        map[knowledgeSyncMetricKey]float64
	durationSeconds map[knowledgeSyncMetricKey]float64
	beforeRunBlocks map[string]float64
}

func (m *knowledgeSyncMetrics) recordAttempt(provider, mode, result string, duration time.Duration) {
	if m == nil {
		return
	}
	key := knowledgeSyncMetricKey{
		Provider: normalizeMetricLabel(firstNonEmptyString(provider, "unknown")),
		Mode:     normalizeMetricLabel(firstNonEmptyString(mode, "unknown")),
		Result:   normalizeMetricLabel(firstNonEmptyString(result, "unknown")),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attempts == nil {
		m.attempts = map[knowledgeSyncMetricKey]float64{}
	}
	if m.durationSeconds == nil {
		m.durationSeconds = map[knowledgeSyncMetricKey]float64{}
	}
	m.attempts[key]++
	m.durationSeconds[key] += duration.Seconds()
}

func (m *knowledgeSyncMetrics) recordProviderRequest(provider, operation, result string, duration time.Duration) {
	if m == nil {
		return
	}
	key := knowledgeSyncMetricKey{
		Provider:  normalizeMetricLabel(firstNonEmptyString(provider, "unknown")),
		Operation: normalizeMetricLabel(firstNonEmptyString(operation, "unknown")),
		Result:    normalizeMetricLabel(firstNonEmptyString(result, "unknown")),
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.durationSeconds == nil {
		m.durationSeconds = map[knowledgeSyncMetricKey]float64{}
	}
	m.durationSeconds[key] += duration.Seconds()
}

func (m *knowledgeSyncMetrics) recordBeforeRunBlock(provider string) {
	if m == nil {
		return
	}
	provider = normalizeMetricLabel(firstNonEmptyString(provider, "unknown"))
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.beforeRunBlocks == nil {
		m.beforeRunBlocks = map[string]float64{}
	}
	m.beforeRunBlocks[provider]++
}

func (m *knowledgeSyncMetrics) snapshot() (attempts map[knowledgeSyncMetricKey]float64, durations map[knowledgeSyncMetricKey]float64, beforeRunBlocks map[string]float64) {
	if m == nil {
		return nil, nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	attempts = copyKnowledgeSyncMetricMap(m.attempts)
	durations = copyKnowledgeSyncMetricMap(m.durationSeconds)
	beforeRunBlocks = make(map[string]float64, len(m.beforeRunBlocks))
	for key, value := range m.beforeRunBlocks {
		beforeRunBlocks[key] = value
	}
	return attempts, durations, beforeRunBlocks
}

func copyKnowledgeSyncMetricMap(in map[knowledgeSyncMetricKey]float64) map[knowledgeSyncMetricKey]float64 {
	out := make(map[knowledgeSyncMetricKey]float64, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func (a *App) runKnowledgeContextPeriodicSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(knowledgeSyncWorkerPollInterval)
	defer ticker.Stop()

	a.dispatchDuePeriodicKnowledgeSyncs(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dispatchDuePeriodicKnowledgeSyncs(ctx)
		}
	}
}

func (a *App) dispatchDuePeriodicKnowledgeSyncs(ctx context.Context) {
	if a == nil || a.db == nil {
		return
	}
	jobs, err := a.claimDuePeriodicKnowledgeSyncs(ctx, knowledgeSyncWorkerBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to claim periodic Knowledge Context sync jobs")
		return
	}
	lastProviderAttempt := map[string]time.Time{}
	for _, job := range jobs {
		if err := ctx.Err(); err != nil {
			return
		}
		provider := strings.TrimSpace(job.Provider)
		if lastAttempt, ok := lastProviderAttempt[provider]; ok {
			if wait := knowledgeProviderRateLimitDelay - time.Since(lastAttempt); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
		lastProviderAttempt[provider] = time.Now()
		if err := a.runPeriodicKnowledgeSyncJob(ctx, job); err != nil {
			log.Warn().
				Err(err).
				Str("knowledge_context", job.ID()).
				Str("knowledge_connection", job.Connection).
				Msg("Periodic Knowledge Context sync failed")
		}
	}
}

func (a *App) claimDuePeriodicKnowledgeSyncs(ctx context.Context, limit int) ([]knowledgePeriodicSyncJob, error) {
	if limit <= 0 {
		limit = knowledgeSyncWorkerBatchSize
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		WITH due AS (
			SELECT k.id
			FROM knowledge_contexts k
			JOIN knowledge_context_connections c ON c.id = k.connection_id
			WHERE k.sync_mode = 'periodic'
			  AND (k.content_source = 'external_page' OR k.external_page_id <> '' OR k.external_page_url <> '')
			  AND (k.next_sync_attempt_at IS NULL OR k.next_sync_attempt_at <= NOW())
			  AND (
			  	k.sync_status <> 'syncing'
			  	OR k.last_sync_started_at IS NULL
			  	OR k.last_sync_started_at < NOW() - ($2::text)::interval
			  )
			  AND (
			  	k.last_synced_at IS NULL
			  	OR k.last_synced_at + make_interval(mins => CASE WHEN k.sync_interval_minutes > 0 THEN k.sync_interval_minutes ELSE $3 END) <= NOW()
			  	OR k.sync_status IN ('failed', 'page_unavailable', 'page_too_large', 'invalid_request', 'authentication_required', 'permission_denied', 'provider_unavailable', 'connection_disabled')
			  	OR k.sync_status = 'syncing'
			  )
			ORDER BY COALESCE(k.next_sync_attempt_at, k.last_synced_at, k.updated_at), k.kind, k.team_path, k.name
			LIMIT $1
			FOR UPDATE OF k SKIP LOCKED
		)
		UPDATE knowledge_contexts k
		SET sync_status = 'syncing',
		    sync_error = '',
		    last_sync_started_at = NOW(),
		    updated_at = NOW()
		FROM due, knowledge_context_connections c
		WHERE k.id = due.id
		  AND c.id = k.connection_id
		RETURNING k.id::text, k.kind, k.team_path, k.name, c.provider, c.team_path || '/' || c.name
	`, limit, fmt.Sprintf("%d seconds", int(knowledgeSyncStuckAfter.Seconds())), defaultKnowledgeSyncIntervalMinutes)
	if err != nil {
		return nil, err
	}
	var jobs []knowledgePeriodicSyncJob
	for rows.Next() {
		var job knowledgePeriodicSyncJob
		if err := rows.Scan(&job.UUID, &job.Kind, &job.Team, &job.Name, &job.Provider, &job.Connection); err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (a *App) runPeriodicKnowledgeSyncJob(ctx context.Context, job knowledgePeriodicSyncJob) error {
	jobCtx, cancel := context.WithTimeout(ctx, knowledgeSyncWorkerTimeout)
	defer cancel()

	detail, err := a.loadKnowledgeContextDetail(jobCtx, job.Kind, job.Team, job.Name)
	if err != nil {
		a.knowledgeSyncMetrics.recordAttempt(job.Provider, knowledgeSyncModePeriodic, "failure", 0)
		return err
	}
	connection, err := a.loadKnowledgeConnectionByIdentifier(jobCtx, detail.ConnectionID)
	if err != nil {
		a.markExternalKnowledgeSyncFailure(jobCtx, detail, knowledgeConnectionRecord{
			knowledgeConnectionListItem: knowledgeConnectionListItem{
				ID:       job.Connection,
				UUID:     strings.TrimSpace(detail.ConnectionID),
				Provider: job.Provider,
			},
		}, fmt.Errorf("knowledge connection unavailable: %w", err))
		a.knowledgeSyncMetrics.recordAttempt(job.Provider, knowledgeSyncModePeriodic, "failure", 0)
		return err
	}
	_, err = a.syncExternalKnowledgePage(jobCtx, detail, connection, knowledgeSyncModePeriodic)
	return err
}

func (a *App) syncExternalKnowledgePage(ctx context.Context, detail knowledgeContextDetail, connection knowledgeConnectionRecord, mode string) (ExternalPage, error) {
	start := time.Now()
	page, err := a.fetchAndStoreExternalKnowledgePage(ctx, detail, connection)
	duration := time.Since(start)
	result := "success"
	if err != nil {
		result = externalKnowledgeSyncStatus(err)
	}
	a.knowledgeSyncMetrics.recordAttempt(connection.Provider, mode, result, duration)
	a.knowledgeSyncMetrics.recordProviderRequest(connection.Provider, "get_page", result, duration)
	return page, err
}

func (a *App) markKnowledgeContextSyncing(ctx context.Context, detail knowledgeContextDetail) (bool, error) {
	tag, err := a.db.Exec(ctx, `
		UPDATE knowledge_contexts
		SET sync_status = 'syncing',
		    sync_error = '',
		    last_sync_started_at = NOW(),
		    updated_at = NOW()
		WHERE kind = $1
		  AND team_path = $2
		  AND name = $3
		  AND (
		  	sync_status <> 'syncing'
		  	OR last_sync_started_at IS NULL
		  	OR last_sync_started_at < NOW() - ($4::text)::interval
		  )
	`, detail.Kind, detail.Team, detail.Name, fmt.Sprintf("%d seconds", int(knowledgeSyncStuckAfter.Seconds())))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
