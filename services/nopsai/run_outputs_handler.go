package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

type taskOutputsRequest struct {
	Outputs []taskOutputInput `json:"outputs"`
}

type taskOutputInput struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

func (a *App) handleRecordTaskOutputs(w http.ResponseWriter, r *http.Request) {
	if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
		return
	}
	runID := strings.TrimSpace(r.PathValue("runID"))
	stepName := strings.TrimSpace(r.PathValue("stepName"))
	taskName := strings.TrimSpace(r.PathValue("taskName"))
	if runID == "" || stepName == "" || taskName == "" {
		http.Error(w, "run, step and task are required", http.StatusBadRequest)
		return
	}

	var req taskOutputsRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid task outputs payload", http.StatusBadRequest)
		return
	}
	if len(req.Outputs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	tx, err := a.db.Begin(r.Context())
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to begin task output transaction")
		http.Error(w, "failed to record task outputs", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	auditOutputs := make([]map[string]any, 0, len(req.Outputs))
	for _, output := range req.Outputs {
		name := strings.TrimSpace(output.Name)
		if !models.IsValidTaskOutputName(name) {
			http.Error(w, "invalid output name", http.StatusBadRequest)
			return
		}
		value := output.Value
		if output.Sensitive {
			value, err = a.encrypt(output.Value)
			if err != nil {
				log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Str("output", name).Msg("Failed to encrypt sensitive runtime output")
				http.Error(w, "failed to protect sensitive output", http.StatusInternalServerError)
				return
			}
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO pipeline_run_task_outputs (run_id, step_name, task_name, name, value, sensitive, size_bytes, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			ON CONFLICT (run_id, step_name, task_name, name)
			DO UPDATE SET value = EXCLUDED.value,
			              sensitive = EXCLUDED.sensitive,
			              size_bytes = EXCLUDED.size_bytes,
			              updated_at = NOW()
		`, runID, stepName, taskName, name, value, output.Sensitive, runtimeOutputSizeBytes(output.SizeBytes)); err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Str("output", name).Msg("Failed to store runtime output")
			http.Error(w, "failed to record task outputs", http.StatusInternalServerError)
			return
		}
		auditOutputs = append(auditOutputs, map[string]any{
			"name":       name,
			"sensitive":  output.Sensitive,
			"size_bytes": runtimeOutputSizeBytes(output.SizeBytes),
		})
	}

	metadata, _ := json.Marshal(map[string]any{"outputs": auditOutputs})
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO pipeline_run_logs (run_id, line, source, level, step_name, task_name, metadata)
		VALUES ($1, $2, 'agent', 'info', $3, $4, $5::jsonb)
	`, runID, "Collected runtime outputs", stepName, taskName, string(metadata)); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to write runtime output audit log")
	}

	if err := tx.Commit(r.Context()); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to commit task output transaction")
		http.Error(w, "failed to record task outputs", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func runtimeOutputSizeBytes(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

type runtimeOutputQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadRuntimeOutputMetadata(ctx context.Context, db runtimeOutputQueryer, runID string) (map[string][]models.TaskRuntimeOutput, error) {
	rows, err := db.Query(ctx, `
		SELECT step_name, task_name, name, sensitive, size_bytes
		FROM pipeline_run_task_outputs
		WHERE run_id = $1
		ORDER BY step_name ASC, task_name ASC, name ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outputs := map[string][]models.TaskRuntimeOutput{}
	for rows.Next() {
		var output models.TaskRuntimeOutput
		if err := rows.Scan(&output.StepName, &output.TaskName, &output.Name, &output.Sensitive, &output.SizeBytes); err != nil {
			return nil, err
		}
		key := output.StepName + "\x00" + output.TaskName
		outputs[key] = append(outputs[key], output)
	}
	return outputs, rows.Err()
}

func attachRuntimeOutputMetadataToTasks(tasksByStep map[string][]models.TaskDetail, outputs map[string][]models.TaskRuntimeOutput) {
	if len(outputs) == 0 {
		return
	}
	for stepName, tasks := range tasksByStep {
		for taskIdx := range tasks {
			key := tasks[taskIdx].StepName + "\x00" + tasks[taskIdx].TaskName
			if values := outputs[key]; len(values) > 0 {
				tasks[taskIdx].Outputs = values
			}
		}
		tasksByStep[stepName] = tasks
	}
}
