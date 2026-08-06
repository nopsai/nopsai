package nopsai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/serviceauth"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

var (
	errChildOutputLineage   = errors.New("child run does not belong to the requested parent step")
	errChildOutputAmbiguous = errors.New("child runtime output name is ambiguous")
	errChildOutputMissing   = errors.New("child runtime output was not produced")
)

type taskOutputsRequest struct {
	Outputs []taskOutputInput `json:"outputs"`
}

type taskOutputResolveRequest struct {
	ParentRunID    string   `json:"parent_run_id"`
	ParentStepName string   `json:"parent_step_name"`
	Names          []string `json:"names"`
}

type taskOutputResolveResponse struct {
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

func (a *App) handleResolveChildTaskOutputs(w http.ResponseWriter, r *http.Request) {
	if !requireInternalServiceRole(w, r, serviceauth.RoleAgent) {
		return
	}
	childRunID := strings.TrimSpace(r.PathValue("runID"))
	if childRunID == "" {
		http.Error(w, "child run is required", http.StatusBadRequest)
		return
	}

	var req taskOutputResolveRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "invalid task output resolve payload", http.StatusBadRequest)
		return
	}
	parentRunID := strings.TrimSpace(req.ParentRunID)
	parentStepName := strings.TrimSpace(req.ParentStepName)
	if parentRunID == "" || parentStepName == "" {
		http.Error(w, "parent run and parent step are required", http.StatusBadRequest)
		return
	}
	names, ok := normalizeTaskOutputResolveNames(req.Names)
	if !ok {
		http.Error(w, "invalid output name", http.StatusBadRequest)
		return
	}
	if len(names) == 0 {
		writeJSON(w, http.StatusOK, taskOutputResolveResponse{Outputs: []taskOutputInput{}})
		return
	}

	resolution, err := resolveChildTaskOutputs(r.Context(), a.db, a.decrypt, childRunID, parentRunID, parentStepName, names)
	if err != nil {
		switch {
		case errors.Is(err, errChildOutputLineage):
			http.Error(w, errChildOutputLineage.Error(), http.StatusForbidden)
		case errors.Is(err, errChildOutputAmbiguous):
			http.Error(w, errChildOutputAmbiguous.Error(), http.StatusConflict)
		case errors.Is(err, errChildOutputMissing):
			http.Error(w, errChildOutputMissing.Error(), http.StatusNotFound)
		default:
			log.Error().Err(err).Str("run_id", childRunID).Str("parent_run_id", parentRunID).Msg("Failed to resolve child runtime outputs")
			http.Error(w, "failed to resolve task outputs", http.StatusInternalServerError)
		}
		return
	}

	metadata, _ := json.Marshal(map[string]any{
		"child_run_id": childRunID,
		"outputs":      resolution.auditOutputs,
	})
	if _, err := a.db.Exec(r.Context(), `
		INSERT INTO pipeline_run_logs (run_id, line, source, level, step_name, task_name, metadata)
		VALUES ($1, $2, 'agent', 'info', $3, $3, $4::jsonb)
	`, parentRunID, "Resolved child runtime outputs", parentStepName, string(metadata)); err != nil {
		log.Warn().Err(err).Str("run_id", parentRunID).Str("child_run_id", childRunID).Msg("Failed to write child runtime output audit log")
	}

	writeJSON(w, http.StatusOK, resolution.response)
}

type childTaskOutputResolverDB interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type childTaskOutputResolution struct {
	response     taskOutputResolveResponse
	auditOutputs []map[string]any
}

func resolveChildTaskOutputs(ctx context.Context, db childTaskOutputResolverDB, decrypt func(string) (string, error), childRunID, parentRunID, parentStepName string, names []string) (childTaskOutputResolution, error) {
	var childMatches bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pipeline_runs
			WHERE run_id::text = $1
			  AND parent_run_id::text = $2
			  AND parent_step_name = $3
		)
	`, childRunID, parentRunID, parentStepName).Scan(&childMatches); err != nil {
		return childTaskOutputResolution{}, fmt.Errorf("validate child output lineage: %w", err)
	}
	if !childMatches {
		return childTaskOutputResolution{}, errChildOutputLineage
	}

	rows, err := db.Query(ctx, `
		SELECT step_name, task_name, name, value, sensitive, size_bytes
		FROM pipeline_run_task_outputs
		WHERE run_id::text = $1
		  AND name = ANY($2::text[])
		ORDER BY step_name ASC, task_name ASC, name ASC
	`, childRunID, names)
	if err != nil {
		return childTaskOutputResolution{}, fmt.Errorf("query child runtime outputs: %w", err)
	}
	defer rows.Close()

	outputsByName := map[string]taskOutputInput{}
	for rows.Next() {
		var stepName, taskName, name, value string
		var sensitive bool
		var sizeBytes int64
		if err := rows.Scan(&stepName, &taskName, &name, &value, &sensitive, &sizeBytes); err != nil {
			return childTaskOutputResolution{}, fmt.Errorf("scan child runtime output: %w", err)
		}
		if _, exists := outputsByName[name]; exists {
			return childTaskOutputResolution{}, errChildOutputAmbiguous
		}
		if sensitive && value != "" {
			if decrypt == nil {
				return childTaskOutputResolution{}, fmt.Errorf("decrypt child runtime output %q: decryptor is not configured", name)
			}
			decrypted, err := decrypt(value)
			if err != nil {
				return childTaskOutputResolution{}, fmt.Errorf("decrypt child runtime output %q: %w", name, err)
			}
			value = decrypted
		}
		outputsByName[name] = taskOutputInput{
			Name:      name,
			Value:     value,
			Sensitive: sensitive,
			SizeBytes: runtimeOutputSizeBytes(sizeBytes),
		}
	}
	if err := rows.Err(); err != nil {
		return childTaskOutputResolution{}, fmt.Errorf("iterate child runtime outputs: %w", err)
	}

	resolution := childTaskOutputResolution{
		response:     taskOutputResolveResponse{Outputs: make([]taskOutputInput, 0, len(names))},
		auditOutputs: make([]map[string]any, 0, len(names)),
	}
	for _, name := range names {
		output, exists := outputsByName[name]
		if !exists {
			return childTaskOutputResolution{}, errChildOutputMissing
		}
		resolution.response.Outputs = append(resolution.response.Outputs, output)
		resolution.auditOutputs = append(resolution.auditOutputs, map[string]any{
			"name":       output.Name,
			"sensitive":  output.Sensitive,
			"size_bytes": output.SizeBytes,
		})
	}
	return resolution, nil
}

func normalizeTaskOutputResolveNames(values []string) ([]string, bool) {
	if len(values) == 0 {
		return nil, true
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" || !models.IsValidTaskOutputName(name) {
			return nil, false
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names, true
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
