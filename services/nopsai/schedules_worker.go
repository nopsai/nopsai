package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	aaamodel "nopsai/services/aaa/pkg/model"
)

func (a *App) executeSchedule(ctx context.Context, record scheduleRecord) (string, error) {
	runTeamPath := effectiveScheduleRunTeamPath(record)
	payload := runRequestPayload{
		Pipeline:  record.pipelineIdentifier(),
		Scope:     record.Scope,
		Variables: record.Variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/run", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Nopsai-Caller-Type", aaamodel.SubjectTypeServiceAccount)
	req.Header.Set("X-Nopsai-Caller-ID", scheduleServiceAccountID(record.ID))
	req.Header.Set("X-Nopsai-Trigger-Source", scheduleTriggerSource)
	req.Header.Set("X-Nopsai-Pipeline-Source", scheduleTriggerSource)
	req.Header.Set("X-Nopsai-Trigger-Event-ID", fmt.Sprintf("schedule:%s:%d", record.ID, time.Now().Unix()))
	if strings.TrimSpace(record.Scope) != "" {
		req.Header.Set("X-Nopsai-Scope", record.Scope)
	}
	if strings.TrimSpace(runTeamPath) != "" {
		req.Header.Set("X-Nopsai-Team-Path", runTeamPath)
	}
	req = req.WithContext(withAAASubject(req.Context(), aaamodel.Subject{
		Type: aaamodel.SubjectTypeServiceAccount,
		ID:   scheduleServiceAccountID(record.ID),
	}))

	recorder := httptest.NewRecorder()
	a.handleRunPipeline(recorder, req)
	if recorder.Code < 200 || recorder.Code >= 300 {
		message := strings.TrimSpace(recorder.Body.String())
		if message == "" {
			message = fmt.Sprintf("schedule execution failed with status %d", recorder.Code)
		}
		_, _ = a.db.Exec(ctx, `
			UPDATE pipeline_schedules
			SET last_run_at = NOW(),
				last_status = $2,
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID, "failure")
		return "", fmt.Errorf("%s", message)
	}

	var response struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || strings.TrimSpace(response.RunID) == "" {
		response.RunID = parseRunIDFromCreatedMessage(recorder.Body.String())
	}
	if strings.TrimSpace(response.RunID) == "" {
		return "", fmt.Errorf("schedule execution did not return a run id")
	}

	teamID, teamErr := a.resolveTeamIDForPath(ctx, runTeamPath)
	if teamErr != nil {
		log.Warn().Err(teamErr).Str("schedule_id", record.ID).Str("team_path", runTeamPath).Msg("Failed to resolve schedule run team")
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_runs
		SET schedule_id = $2,
			team_id = CASE WHEN $3::boolean THEN $4::integer ELSE team_id END
		WHERE run_id::text = $1
	`, response.RunID, record.ID, teamID.Valid, teamID); err != nil {
		return "", err
	}
	if _, err := a.db.Exec(ctx, `
		UPDATE pipeline_schedules
		SET last_run_id = $2,
			last_run_at = NOW(),
			last_status = 'pending',
			updated_at = NOW()
		WHERE id::text = $1
	`, record.ID, response.RunID); err != nil {
		return "", err
	}
	return response.RunID, nil
}

func effectiveScheduleRunTeamPath(record scheduleRecord) string {
	if teamPath := strings.Trim(strings.TrimSpace(record.RunTeamPath), "/"); teamPath != "" {
		return teamPath
	}
	return globalGrantID
}

func parseRunIDFromCreatedMessage(message string) string {
	message = strings.TrimSpace(message)
	const marker = "ID:"
	idx := strings.LastIndex(message, marker)
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(message[idx+len(marker):])
}

func (a *App) runScheduleWorker(ctx context.Context) {
	ticker := time.NewTicker(scheduleWorkerPollInterval)
	defer ticker.Stop()

	a.dispatchDueSchedules(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.dispatchDueSchedules(ctx)
		}
	}
}

func (a *App) dispatchDueSchedules(ctx context.Context) {
	if a == nil || a.db == nil {
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to begin scheduled pipeline dispatch")
		return
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, baseScheduleSelect()+`
		WHERE s.enabled = TRUE
		  AND s.next_run_at IS NOT NULL
		  AND s.next_run_at <= NOW()
		ORDER BY s.next_run_at ASC
		LIMIT $1
		FOR UPDATE OF s SKIP LOCKED
	`, scheduleWorkerBatchSize)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to claim due schedules")
		return
	}
	var due []scheduleRecord
	for rows.Next() {
		record, err := scanScheduleRecord(rows)
		if err != nil {
			rows.Close()
			log.Warn().Err(err).Msg("Failed to scan due schedule")
			return
		}
		due = append(due, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Warn().Err(err).Msg("Failed to read due schedules")
		return
	}
	rows.Close()

	now := time.Now()
	for _, record := range due {
		if normalizeScheduleKindValue(record.ScheduleKind) == scheduleKindOnce {
			if _, err := tx.Exec(ctx, `
				UPDATE pipeline_schedules
				SET enabled = FALSE,
					next_run_at = NULL,
					updated_at = NOW()
				WHERE id::text = $1
			`, record.ID); err != nil {
				log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to complete one-time schedule claim")
				return
			}
			continue
		}

		nextRunAt, err := nextScheduleRunAt(record.CronExpression, record.Timezone, now)
		if err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to calculate next schedule time")
			_, _ = tx.Exec(ctx, `
				UPDATE pipeline_schedules
				SET next_run_at = NULL,
					last_status = 'failure',
					updated_at = NOW()
				WHERE id::text = $1
			`, record.ID)
			continue
		}
		if _, err := tx.Exec(ctx, `
			UPDATE pipeline_schedules
			SET next_run_at = $2,
				updated_at = NOW()
			WHERE id::text = $1
		`, record.ID, nextRunAt); err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Failed to advance schedule")
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to commit due schedule claims")
		return
	}

	for _, record := range due {
		if _, err := a.executeSchedule(ctx, record); err != nil {
			log.Warn().Err(err).Str("schedule_id", record.ID).Msg("Scheduled pipeline execution failed")
		}
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
