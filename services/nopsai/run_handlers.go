package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	query := `
		SELECT
		    run_id, pipeline_name, pipeline_path, pipeline_version, status, COALESCE(git_commit_sha, ''),
		    COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''), started_at, finished_at, parent_run_id,
		    COALESCE(git_pusher_name, ''), COALESCE(git_ref, ''), COALESCE(git_target_ref, ''),
			COALESCE(pipeline_source, ''), COALESCE(trigger_event_id, ''), COALESCE(failure_reason, '')
    	FROM pipeline_runs
	`
	args := []interface{}{}
	var conditions []string

	limit := 300
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil {
			switch {
			case v <= 0:
				limit = 50
			case v > 1000:
				limit = 1000
			default:
				limit = v
			}
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if v, err := strconv.Atoi(offsetStr); err == nil && v > 0 {
			offset = v
		}
	}

	if groupIDStr := r.URL.Query().Get("groupId"); groupIDStr != "" {
		groupID, err := strconv.Atoi(groupIDStr)
		if err == nil {
			conditions = append(conditions, fmt.Sprintf("group_id = $%d", len(args)+1))
			args = append(args, groupID)
		}
	}

	if branchName := r.URL.Query().Get("branch"); branchName != "" {
		conditions = append(conditions, fmt.Sprintf("git_ref = $%d", len(args)+1))
		args = append(args, "refs/heads/"+branchName)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)

	rows, err := a.db.Query(context.Background(), query, args...)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query runs from database")
		http.Error(w, "Failed to retrieve runs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	runsByBranch := make(map[string][]RunListItem)
	var allRuns []RunListItem
	for rows.Next() {
		var run RunListItem
		var startedAt, finishedAt sql.NullTime
		var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, pipelineSource, pipelineVersion, pipelinePath, triggerEventID, failureReason sql.NullString
		err := rows.Scan(
			&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
			&repoOwner, &repoName, &startedAt, &finishedAt, &run.ParentRunID, &pusherName, &gitRef, &gitTargetRef, &pipelineSource, &triggerEventID, &failureReason,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to scan run row")
			continue
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
		run.FailureReason = failureReason.String
		if startedAt.Valid {
			run.StartedAt = startedAt.Time
			if finishedAt.Valid {
				run.FinishedAt = finishedAt.Time
				run.Duration = run.FinishedAt.Sub(run.StartedAt).Round(time.Second).String()
				run.IsComplete = true
			} else {
				run.Duration = time.Since(run.StartedAt).Round(time.Second).String()
				run.IsComplete = false
			}
		} else {
			run.IsComplete = true // If it hasn't even started, it's not "running"
		}
		allRuns = append(allRuns, run)
	}

	runResources := make([]model.ResourceRef, 0, len(allRuns))
	for _, run := range allRuns {
		runResources = append(runResources, routeauthz.RunResource(run.RunID))
	}
	allowedSet, err := a.allowedResourceSet(r, "pipeline_run.list", runResources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	filteredRuns := make([]RunListItem, 0, len(allRuns))
	for _, run := range allRuns {
		if _, ok := allowedSet[resourceKey(routeauthz.RunResource(run.RunID))]; !ok {
			continue
		}
		filteredRuns = append(filteredRuns, run)
	}

	if r.URL.Query().Get("groupId") != "" {
		for _, run := range filteredRuns {
			branch := "Others"
			if run.GitRef != "" {
				branch = strings.TrimPrefix(run.GitRef, "refs/heads/")
			}
			runsByBranch[branch] = append(runsByBranch[branch], run)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runsByBranch)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(filteredRuns)
	}
}

func (a *App) handleGetRunDetails(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	runID := r.PathValue("runID")

	var run RunListItem
	var pipelineDefinition string
	var startedAt, finishedAt sql.NullTime
	var commitSHA, repoOwner, repoName, pusherName, gitRef, gitTargetRef, failureReason, pipelineSource, pipelineVersion, pipelinePath, triggerEventID sql.NullString
	err := a.db.QueryRow(context.Background(), `
		SELECT
			run_id, pipeline_name, pipeline_path, pipeline_version, status, COALESCE(git_commit_sha, ''),
			COALESCE(git_repo_owner, ''), COALESCE(git_repo_name, ''),
			started_at, finished_at, parent_run_id,
			COALESCE(git_pusher_name, ''), pipeline_definition, COALESCE(git_ref, ''), COALESCE(git_target_ref, ''),
			failure_reason, COALESCE(pipeline_source, ''), COALESCE(trigger_event_id, '')
		FROM pipeline_runs
		WHERE run_id = $1
	`, runID).Scan(
		&run.RunID, &run.PipelineName, &pipelinePath, &pipelineVersion, &run.Status, &commitSHA,
		&repoOwner, &repoName, &startedAt, &finishedAt,
		&run.ParentRunID, &pusherName, &pipelineDefinition, &gitRef, &gitTargetRef,
		&failureReason, &pipelineSource, &triggerEventID,
	)

	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query run details from database")
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	run.PipelinePath = pipelinePath.String
	run.GitCommitSHA = commitSHA.String
	run.GitRepoOwner = repoOwner.String
	run.GitRepoName = repoName.String
	run.GitPusherName = pusherName.String
	run.GitRef = gitRef.String
	run.GitTargetRef = gitTargetRef.String
	run.FailureReason = failureReason.String
	run.PipelineSource = pipelineSource.String
	run.PipelineVersion = normalizePipelineVersion(pipelineVersion.String)
	run.TriggerEventID = triggerEventID.String
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

	var parentRunInfo *ParentRunInfo
	if run.ParentRunID != nil && *run.ParentRunID != "" {
		var parentPipelineName, parentPipelineVersion, parentPipelinePath string
		err := a.db.QueryRow(context.Background(), `
            SELECT pipeline_name, pipeline_path, pipeline_version FROM pipeline_runs WHERE run_id = $1
        `, *run.ParentRunID).Scan(&parentPipelineName, &parentPipelinePath, &parentPipelineVersion)
		if err != nil {
			log.Error().Err(err).Str("parent_run_id", *run.ParentRunID).Msg("Failed to query parent pipeline name")
		} else {
			parentRunInfo = &ParentRunInfo{
				RunID:           *run.ParentRunID,
				PipelineName:    parentPipelineName,
				PipelinePath:    parentPipelinePath,
				PipelineVersion: normalizePipelineVersion(parentPipelineVersion),
			}
		}
	}

	childRuns := make([]RunListItem, 0)
	childRows, err := a.db.Query(context.Background(), `
		SELECT run_id, pipeline_name, pipeline_path, pipeline_version, status, started_at, finished_at, parent_step_name, COALESCE(trigger_event_id, '')
		FROM pipeline_runs
		WHERE parent_run_id = $1
		ORDER BY created_at ASC
	`, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query child runs for details view")
	} else {
		defer childRows.Close()
		for childRows.Next() {
			var childRun RunListItem
			var childStartedAt, childFinishedAt sql.NullTime
			var parentStepName, childPipelineVersion, childPipelinePath, childTriggerEventID sql.NullString
			if err := childRows.Scan(&childRun.RunID, &childRun.PipelineName, &childPipelinePath, &childPipelineVersion, &childRun.Status, &childStartedAt, &childFinishedAt, &parentStepName, &childTriggerEventID); err != nil {
				log.Error().Err(err).Msg("Failed to scan child run row")
				continue
			}
			childRun.PipelinePath = childPipelinePath.String
			childRun.PipelineVersion = normalizePipelineVersion(childPipelineVersion.String)
			childRun.TriggerEventID = childTriggerEventID.String
			if childStartedAt.Valid {
				childRun.StartedAt = childStartedAt.Time
			}
			if childFinishedAt.Valid {
				childRun.FinishedAt = childFinishedAt.Time
				childRun.Duration = childRun.FinishedAt.Sub(childRun.StartedAt).Round(time.Second).String()
				childRun.IsComplete = true
			} else if childStartedAt.Valid {
				childRun.Duration = time.Since(childRun.StartedAt).Round(time.Second).String()
			}
			childRun.ParentStepName = parentStepName.String
			childRuns = append(childRuns, childRun)
		}
	}

	childRunsByStep := make(map[string][]RunListItem)
	for _, childRun := range childRuns {
		if childRun.ParentStepName == "" {
			continue
		}
		childRunsByStep[childRun.ParentStepName] = append(childRunsByStep[childRun.ParentStepName], childRun)
	}

	taskRows, err := a.db.Query(context.Background(), `
		SELECT task_id, step_name, task_name, status, exit_code, started_at, finished_at, task_index
		FROM task_runs
		WHERE run_id = $1
		ORDER BY task_index ASC
	`, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query tasks for run")
		http.Error(w, "Failed to retrieve tasks", http.StatusInternalServerError)
		return
	}
	defer taskRows.Close()

	tasksByStep := make(map[string][]TaskDetail)
	for taskRows.Next() {
		var task TaskDetail
		var startedAt, finishedAt sql.NullTime
		if err := taskRows.Scan(&task.TaskID, &task.StepName, &task.TaskName, &task.Status, &task.ExitCode, &startedAt, &finishedAt, &task.TaskIndex); err != nil {
			log.Error().Err(err).Msg("Failed to scan task row")
			continue
		}
		if startedAt.Valid {
			task.StartedAt = startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = finishedAt.Time
		}
		tasksByStep[task.StepName] = append(tasksByStep[task.StepName], task)
	}

	var originalPipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDefinition), &originalPipeline); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to parse original pipeline definition")
		http.Error(w, "Failed to parse pipeline definition", http.StatusInternalServerError)
		return
	}

	tempPipelineForResolving := originalPipeline
	resolvedPipeline, err := a.resolveStepIncludes(&tempPipelineForResolving)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to resolve includes for details view")
		resolvedPipeline = &originalPipeline
	}

	steps := make([]StepDetail, 0) // Initialize as an empty slice
	for _, pStep := range resolvedPipeline.Steps {
		stepName := pStep.GetName()
		stepTasks := tasksByStep[stepName]
		stepChildRuns := childRunsByStep[stepName]
		status := finalizeRunDetailStepStatus(deriveRunDetailStepStatus(stepTasks, stepChildRuns), stepTasks, run.Status)
		stepDuration := deriveRunDetailStepDuration(stepTasks, stepChildRuns)

		originalPStep, _ := findStepByName(originalPipeline.Steps, stepName)

		config := StepConfiguration{
			Image:            pStep.GetImage(),
			Include:          originalPStep.GetInclude(),
			Sync:             pStep.GetSync(),
			Secrets:          pStep.GetSecrets(),
			Volumes:          pStep.GetVolumes(),
			Variables:        pStep.GetVariables(),
			IgnoreFailure:    pStep.GetIgnoreFailure(),
			LlmOutputSharing: pStep.GetLlmOutputSharing(),
			LLMProfile:       pStep.GetLLMProfile(),
			MCPProfiles:      pStep.GetMCPProfiles(),
			Tasks:            pStep.GetTasks(),
		}

		steps = append(steps, StepDetail{
			Name:          stepName,
			Status:        status,
			DependsOn:     pStep.GetDependsOn(),
			Tasks:         stepTasks,
			Duration:      stepDuration.Round(time.Second).String(),
			Configuration: config,
		})
	}

	response := RunDetail{
		RunInfo:                run,
		Steps:                  steps,
		PipelineDefinition:     originalPipeline,
		PipelineDefinitionYAML: pipelineDefinition,
		ChildRuns:              childRuns,
		ParentRunInfo:          parentRunInfo,
	}

	etag := buildRunDetailETag(run, childRuns, tasksByStep)
	w.Header().Set("ETag", etag)
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func setNoStoreHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
}

func markRunRunning(ctx context.Context, runner execRunner, runID string) error {
	_, err := runner.Exec(ctx, `
		UPDATE pipeline_runs
		SET status = 'running', started_at = COALESCE(started_at, NOW())
		WHERE run_id = $1
		  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'cancelled', 'timed_out')`, runID)
	return err
}

func normalizeRunDetailStatus(status string) string {
	raw := strings.ToLower(strings.TrimSpace(status))
	switch {
	case raw == "":
		return "pending"
	case raw == "success" || raw == "running" || raw == "pending" || raw == "skipped" || raw == "cancelled":
		return raw
	case raw == "failure" || strings.Contains(raw, "not_found") || strings.Contains(raw, "timeout"):
		return "failure"
	case strings.Contains(raw, "ignored"):
		return "failure (ignored)"
	case strings.Contains(raw, "fail") || strings.Contains(raw, "error"):
		return "failure"
	default:
		return raw
	}
}

func summarizeRunDetailStatuses(statuses []string) string {
	if len(statuses) == 0 {
		return "pending"
	}
	priority := map[string]int{
		"failure":           0,
		"failure (ignored)": 1,
		"cancelled":         2,
		"running":           3,
		"pending":           4,
		"skipped":           5,
		"success":           6,
	}
	best := "pending"
	bestPriority := len(priority) + 1
	for _, status := range statuses {
		normalized := normalizeRunDetailStatus(status)
		rank, ok := priority[normalized]
		if !ok {
			normalized = "failure"
			rank = priority[normalized]
		}
		if rank < bestPriority {
			best = normalized
			bestPriority = rank
		}
	}
	return best
}

func deriveRunDetailStepStatus(tasks []TaskDetail, childRuns []RunListItem) string {
	taskStatuses := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskStatuses = append(taskStatuses, task.Status)
	}
	taskStatus := summarizeRunDetailStatuses(taskStatuses)

	if len(childRuns) == 0 {
		return taskStatus
	}

	childStatuses := make([]string, 0, len(childRuns))
	for _, childRun := range childRuns {
		childStatuses = append(childStatuses, childRun.Status)
	}
	childStatus := summarizeRunDetailStatuses(childStatuses)

	if taskStatus == "failure" || taskStatus == "failure (ignored)" || taskStatus == "cancelled" {
		return summarizeRunDetailStatuses([]string{taskStatus, childStatus})
	}
	if childStatus != "pending" {
		return childStatus
	}
	return summarizeRunDetailStatuses([]string{taskStatus, childStatus})
}

func finalizeRunDetailStepStatus(stepStatus string, tasks []TaskDetail, runStatus string) string {
	normalizedStep := normalizeRunDetailStatus(stepStatus)
	normalizedRun := normalizeRunDetailStatus(runStatus)
	if normalizedStep != "running" && normalizedStep != "pending" {
		return normalizedStep
	}

	switch normalizedRun {
	case "success":
		return "success"
	case "failure", "failure (ignored)":
		if normalizedStep == "running" || hasInFlightRunDetailTask(tasks) {
			return normalizedRun
		}
		return "skipped"
	case "cancelled":
		if normalizedStep == "running" || hasInFlightRunDetailTask(tasks) {
			return "cancelled"
		}
		return "skipped"
	default:
		return normalizedStep
	}
}

func hasInFlightRunDetailTask(tasks []TaskDetail) bool {
	for _, task := range tasks {
		if !task.StartedAt.IsZero() && task.FinishedAt.IsZero() {
			return true
		}
	}
	return false
}

func deriveRunDetailStepDuration(tasks []TaskDetail, childRuns []RunListItem) time.Duration {
	var earliestStart time.Time
	var latestFinish time.Time
	hasActiveWork := false

	for _, task := range tasks {
		if !task.StartedAt.IsZero() && (earliestStart.IsZero() || task.StartedAt.Before(earliestStart)) {
			earliestStart = task.StartedAt
		}
		if !task.FinishedAt.IsZero() && task.FinishedAt.After(latestFinish) {
			latestFinish = task.FinishedAt
		}
		if !task.StartedAt.IsZero() && task.FinishedAt.IsZero() {
			hasActiveWork = true
		}
	}

	for _, childRun := range childRuns {
		if !childRun.StartedAt.IsZero() && (earliestStart.IsZero() || childRun.StartedAt.Before(earliestStart)) {
			earliestStart = childRun.StartedAt
		}
		if !childRun.FinishedAt.IsZero() && childRun.FinishedAt.After(latestFinish) {
			latestFinish = childRun.FinishedAt
		}
		if !childRun.StartedAt.IsZero() && childRun.FinishedAt.IsZero() {
			hasActiveWork = true
		}
	}

	if earliestStart.IsZero() {
		return 0
	}
	if !latestFinish.IsZero() && !hasActiveWork {
		return latestFinish.Sub(earliestStart)
	}
	return time.Since(earliestStart)
}

func buildRunDetailETag(run RunListItem, childRuns []RunListItem, tasksByStep map[string][]TaskDetail) string {
	hasher := sha256.New()
	fmt.Fprintf(
		hasher,
		"run|%s|%s|%t|%d|%d|%s|%s|%s|",
		run.RunID,
		normalizeRunDetailStatus(run.Status),
		run.IsComplete,
		run.StartedAt.UnixNano(),
		run.FinishedAt.UnixNano(),
		strings.TrimSpace(run.FailureReason),
		strings.TrimSpace(run.PipelineSource),
		strings.TrimSpace(run.TriggerEventID),
	)

	for _, childRun := range childRuns {
		fmt.Fprintf(
			hasher,
			"child|%s|%s|%t|%d|%d|%s|",
			childRun.RunID,
			normalizeRunDetailStatus(childRun.Status),
			childRun.IsComplete,
			childRun.StartedAt.UnixNano(),
			childRun.FinishedAt.UnixNano(),
			strings.TrimSpace(childRun.ParentStepName),
		)
	}

	stepNames := make([]string, 0, len(tasksByStep))
	for stepName := range tasksByStep {
		stepNames = append(stepNames, stepName)
	}
	sort.Strings(stepNames)

	for _, stepName := range stepNames {
		fmt.Fprintf(hasher, "step|%s|", stepName)
		for _, task := range tasksByStep[stepName] {
			exitCode := ""
			if task.ExitCode != nil {
				exitCode = strconv.Itoa(*task.ExitCode)
			}
			fmt.Fprintf(
				hasher,
				"task|%s|%s|%s|%s|%d|%d|%d|",
				task.TaskID,
				task.StepName,
				task.TaskName,
				normalizeRunDetailStatus(task.Status),
				task.TaskIndex,
				task.StartedAt.UnixNano(),
				task.FinishedAt.UnixNano(),
			)
			fmt.Fprintf(hasher, "exit|%s|", exitCode)
		}
	}

	return fmt.Sprintf(`W/"%x"`, hasher.Sum(nil))
}

func allTasksDone(tasks []TaskDetail) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, t := range tasks {
		if t.Status != "success" && !strings.Contains(t.Status, "ignore") {
			return false
		}
	}
	return true
}

func (a *App) updateRunRecordWithFailure(runID uuid.UUID, reason string, gitContext map[string]string) {
	log.Error().Str("run_id", runID.String()).Msg(reason)
	_, err := a.db.Exec(context.Background(),
		"UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2",
		reason, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to update run record with failure reason")
	}
	if gitContext["check_run_id"] != "" {
		a.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
	}
}

func (a *App) resolveGroupIDForRepo(repoOwner, repoName string) (sql.NullInt32, error) {
	var groupID sql.NullInt32
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return groupID, nil
	}

	repoOwner = strings.TrimSpace(repoOwner)
	fullRepoName := repositoryFullName(repoOwner, repoName)
	matches, err := a.repositoryGroupMatches(context.Background(), repoOwner, repoName)
	if err != nil {
		return groupID, err
	}
	if len(matches) > 0 {
		groupID.Int32 = int32(matches[0].ID)
		groupID.Valid = true
		return groupID, nil
	}

	var existingID int32
	err = a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE name = $1", fullRepoName).Scan(&existingID)
	if err == pgx.ErrNoRows {
		if repoOwner != "" {
			err = a.db.QueryRow(context.Background(), "SELECT id FROM groups WHERE name = $1", repoName).Scan(&existingID)
			if err == nil {
				log.Info().Str("old_name", repoName).Str("new_name", fullRepoName).Msg("Found matching folder, renaming to claim it for the repository.")
				if _, updateErr := a.db.Exec(context.Background(), "UPDATE groups SET name = $1 WHERE id = $2", fullRepoName, existingID); updateErr != nil {
					log.Error().Err(updateErr).Msg("Failed to rename existing folder to claim it.")
					existingID = 0
				}
			} else if err == pgx.ErrNoRows {
				log.Info().Str("repo", fullRepoName).Msg("No existing folder found. Creating a new one at the root.")
				err = a.db.QueryRow(context.Background(), `INSERT INTO groups (name, parent_id) VALUES ($1, NULL) RETURNING id`, fullRepoName).Scan(&existingID)
			}
		} else {
			log.Info().Str("repo", repoName).Msg("No existing folder found. Creating a new one at the root.")
			err = a.db.QueryRow(context.Background(), `INSERT INTO groups (name, parent_id) VALUES ($1, NULL) RETURNING id`, repoName).Scan(&existingID)
		}
	}
	if err != nil && err != pgx.ErrNoRows {
		return groupID, err
	}
	if existingID != 0 {
		groupID.Int32 = existingID
		groupID.Valid = true
	}
	return groupID, nil
}

func (a *App) recordMissingPipelineRun(identifier string, pipelineVersion string, pipelineDef []byte, gitContext map[string]string, scopeValue, pipelineSource, summary string) {
	runID := uuid.New()
	pathPart, namePart, _, err := splitPipelineIdentifier(identifier)
	if err != nil {
		namePart = sanitizeInput(identifier)
		pathPart = ""
	}
	namePart = sanitizeInput(namePart)
	if namePart == "" {
		namePart = "missing-pipeline"
	}

	groupID, groupErr := a.resolveGroupIDForRepo(gitContext["repo_owner"], gitContext["repo_name"])
	if groupErr != nil {
		log.Error().Err(groupErr).Str("pipeline", identifier).Msg("Failed to resolve group for missing pipeline run")
	}

	var triggerEventIDSQL sql.NullString
	if gitContext != nil {
		id := strings.TrimSpace(gitContext["trigger_event_id"])
		if id == "" {
			id = deriveTriggerEventID(gitContext)
		}
		if id == "" {
			id = runID.String()
		}
		if id != "" {
			triggerEventIDSQL.String = id
			triggerEventIDSQL.Valid = true
			gitContext["trigger_event_id"] = id
		}
	}

	now := time.Now()
	_, err = a.db.Exec(context.Background(), `
		INSERT INTO pipeline_runs (
			run_id, pipeline_name, pipeline_path, pipeline_version, status,
			pipeline_definition, git_repo_owner, git_repo_name, git_clone_url, git_ssh_url,
			git_ref, git_target_ref, git_commit_sha, git_commit_url, git_commit_message,
			git_commit_author_name, git_commit_author_email, git_commit_author_username,
			git_pusher_name, git_pusher_email, git_check_run_id, group_id, trigger_event_id,
			scope, pipeline_source, started_at, finished_at, failure_reason
		) VALUES (
			$1, $2, $3, $4, 'failure', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
		)`,
		runID,
		namePart,
		pathPart,
		normalizePipelineVersion(pipelineVersion),
		string(pipelineDef),
		gitContext["repo_owner"],
		gitContext["repo_name"],
		gitContext["clone_url"],
		gitContext["ssh_url"],
		gitContext["ref"],
		gitContext["target_ref"],
		gitContext["commit_sha"],
		gitContext["commit_url"],
		gitContext["commit_message"],
		gitContext["commit_author_name"],
		gitContext["commit_author_email"],
		gitContext["commit_author_username"],
		gitContext["pusher_name"],
		gitContext["pusher_email"],
		gitContext["check_run_id"],
		groupID,
		triggerEventIDSQL,
		scopeValue,
		pipelineSource,
		now,
		now,
		summary,
	)
	if err != nil {
		log.Error().Err(err).Str("pipeline", identifier).Msg("Failed to record missing pipeline run")
		return
	}
}

func (a *App) recordAuthorizationDeniedPipelineRun(identifier string, pipelineVersion string, pipelineDef []byte, gitContext map[string]string, scopeValue, pipelineSource, triggerSource, callerType, callerID, summary string, authChecks []ResourceUseAuthResult) {
	if gitContext == nil {
		gitContext = map[string]string{}
	}
	runID := uuid.New()
	pathPart, namePart, _, err := splitPipelineIdentifier(identifier)
	if err != nil {
		namePart = sanitizeInput(identifier)
		pathPart = ""
	}
	namePart = sanitizeInput(namePart)
	if namePart == "" {
		namePart = "authorization-denied"
	}

	groupID, groupErr := a.resolveGroupIDForRepo(gitContext["repo_owner"], gitContext["repo_name"])
	if groupErr != nil {
		log.Error().Err(groupErr).Str("pipeline", identifier).Msg("Failed to resolve group for authorization denied pipeline run")
	}

	var triggerEventIDSQL sql.NullString
	id := strings.TrimSpace(gitContext["trigger_event_id"])
	if id == "" {
		id = deriveTriggerEventID(gitContext)
	}
	if id == "" {
		id = runID.String()
	}
	if id != "" {
		triggerEventIDSQL.String = id
		triggerEventIDSQL.Valid = true
		gitContext["trigger_event_id"] = id
	}

	var checkRunIDSQL sql.NullInt64
	if value := strings.TrimSpace(gitContext["check_run_id"]); value != "" {
		if parsed, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil {
			checkRunIDSQL.Int64 = parsed
			checkRunIDSQL.Valid = true
		}
	}

	authSnapshot, snapshotErr := buildRunAuthorizationSnapshot(triggerSource, callerType, callerID, authChecks)
	if snapshotErr != nil {
		log.Error().Err(snapshotErr).Str("pipeline", identifier).Msg("Failed to build authorization denied run snapshot")
		authSnapshot = []byte(`{}`)
	}

	now := time.Now()
	_, err = a.db.Exec(context.Background(), `
		INSERT INTO pipeline_runs (
			run_id, pipeline_name, pipeline_path, pipeline_version, status,
			pipeline_definition, git_repo_owner, git_repo_name, git_clone_url, git_ssh_url,
			git_ref, git_target_ref, git_commit_sha, git_commit_url, git_commit_message,
			git_commit_author_name, git_commit_author_email, git_commit_author_username,
			git_pusher_name, git_pusher_email, git_check_run_id, group_id, trigger_event_id,
			scope, pipeline_source, trigger_source, requested_by_type, requested_by_id,
			effective_subject_type, effective_subject_id, authorization_snapshot, started_at,
			finished_at, failure_reason
		) VALUES (
			$1, $2, $3, $4, 'failure', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28,
			$29, $30::jsonb, $31, $32, $33
		)`,
		runID,
		namePart,
		pathPart,
		normalizePipelineVersion(pipelineVersion),
		string(pipelineDef),
		gitContext["repo_owner"],
		gitContext["repo_name"],
		gitContext["clone_url"],
		gitContext["ssh_url"],
		gitContext["ref"],
		gitContext["target_ref"],
		gitContext["commit_sha"],
		gitContext["commit_url"],
		gitContext["commit_message"],
		gitContext["commit_author_name"],
		gitContext["commit_author_email"],
		gitContext["commit_author_username"],
		gitContext["pusher_name"],
		gitContext["pusher_email"],
		checkRunIDSQL,
		groupID,
		triggerEventIDSQL,
		scopeValue,
		pipelineSource,
		triggerSource,
		callerType,
		callerID,
		callerType,
		callerID,
		string(authSnapshot),
		now,
		now,
		summary,
	)
	if err != nil {
		log.Error().Err(err).Str("pipeline", identifier).Msg("Failed to record authorization denied pipeline run")
		return
	}
}

func (a *App) launchAndRunPipeline(
	runID uuid.UUID,
	parentRunID string,
	parentRunnerID string,
	pipeline models.Pipeline,
	pipelineDef []byte,
	timeoutDuration time.Duration,
	gitContext map[string]string,
	parentHistory string,
	scope string,
	overrides map[string]string,
) {
	tx, err := a.db.Begin(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction for tasks")
		a.updateRunRecordWithFailure(runID, "Failed to start database transaction for tasks", gitContext)
		return
	}
	defer tx.Rollback(context.Background())

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if step.GetInclude() != "" {
			_, err := tx.Exec(context.Background(),
				"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, stepName, stepName, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert 'include' step %s as a task", stepName)
				// Don't need to call updateRunRecordWithFailure here as the transaction will be rolled back
				return
			}
		} else if tasks := step.GetTasks(); len(tasks) > 0 {
			for i, task := range tasks {
				_, err := tx.Exec(context.Background(),
					"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
					runID, stepName, task.Name, i+1,
				)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to insert task %s for step %s", task.Name, stepName)
					return
				}
			}
		} else { // Legacy step
			_, err := tx.Exec(context.Background(),
				"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, stepName, stepName, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert step %s as a task", stepName)
				return
			}
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		log.Error().Err(err).Msg("Failed to commit tasks transaction")
		a.updateRunRecordWithFailure(runID, "Failed to commit tasks transaction", gitContext)
		return
	}

	go a.launchAgent(runID.String(), parentRunID, parentRunnerID, pipeline, pipelineDef, timeoutDuration, gitContext, parentHistory, scope, overrides)
}

type runRequestPayload struct {
	Pipeline   string            `json:"pipeline"`
	Scope      string            `json:"scope"`
	Variables  map[string]string `json:"variables"`
	Definition string            `json:"definition"`
}

func (a *App) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	var pipeline models.Pipeline
	var pipelineDef []byte
	var err error

	parentRunID := r.Header.Get("X-Nopsai-Parent-Run-ID")
	parentRunnerID := r.Header.Get("X-Nopsai-Parent-Runner-ID")
	parentHistory := r.Header.Get("X-Nopsai-Parent-History")
	scope := strings.TrimSpace(r.Header.Get("X-Nopsai-Scope"))
	parentStepName := r.Header.Get("X-Nopsai-Parent-Step-Name")
	pipelineSource := r.Header.Get("X-Nopsai-Pipeline-Source")
	pipelineNameFromPath := r.PathValue("pipelineName")
	pipelinePathForRun := strings.TrimSpace(r.Header.Get("X-Nopsai-Pipeline-Path"))
	pipelinePathForRun = filepath.ToSlash(strings.Trim(pipelinePathForRun, "/"))
	if pipelinePathForRun == "." {
		pipelinePathForRun = ""
	}

	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	var payload runRequestPayload
	if strings.Contains(contentType, "application/json") {
		if err := httpapi.DecodeOptionalJSON(r, &payload); err != nil {
			http.Error(w, "Invalid JSON payload for run request", http.StatusBadRequest)
			return
		}
	}

	if scope == "" {
		scope = strings.TrimSpace(payload.Scope)
	}

	if pipelineNameFromPath == "" {
		pipelineNameFromPath = strings.TrimSpace(payload.Pipeline)
	}

	overrideVars := make(map[string]string)
	if len(payload.Variables) > 0 {
		var invalidKeys []string
		for key, value := range payload.Variables {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			if !envKeyPattern.MatchString(trimmedKey) {
				invalidKeys = append(invalidKeys, trimmedKey)
				continue
			}
			overrideVars[trimmedKey] = value
		}
		if len(invalidKeys) > 0 {
			http.Error(w, fmt.Sprintf("Invalid variable override name(s): %s. Allowed characters: letters, numbers, underscores, dots, and hyphens.", strings.Join(invalidKeys, ", ")), http.StatusBadRequest)
			return
		}
	}

	rawDefinition := strings.TrimSpace(payload.Definition)
	usePayloadDefinition := rawDefinition != ""

	if strings.Contains(contentType, "application/json") && pipelineNameFromPath == "" && !usePayloadDefinition {
		http.Error(w, "Pipeline identifier or definition is required for JSON run requests", http.StatusBadRequest)
		return
	}

	if pipelineNameFromPath != "" {
		pathPart, namePart, _, parseErr := splitPipelineIdentifier(pipelineNameFromPath)
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}
		pipelinePathForRun = pathPart
		if !usePayloadDefinition {
			var pipelineDefStr string
			err = a.db.QueryRow(context.Background(), "SELECT definition FROM pipelines WHERE path = $1 AND name = $2", pathPart, namePart).Scan(&pipelineDefStr)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					log.Info().Str("pipeline", pipelineNameFromPath).Msg("Pipeline not found in database")
					http.Error(w, "Pipeline not found", http.StatusNotFound)
					return
				}
				log.Error().Err(err).Str("pipeline", pipelineNameFromPath).Msg("Failed to load pipeline definition from database")
				http.Error(w, "Failed to load pipeline", http.StatusInternalServerError)
				return
			}
			pipelineDef = []byte(pipelineDefStr)
		}
	}

	if usePayloadDefinition {
		pipelineDef = []byte(rawDefinition)
	} else if pipelineNameFromPath == "" {
		pipelineDef, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading request body", http.StatusInternalServerError)
			return
		}
	}

	if len(pipelineDef) == 0 {
		http.Error(w, "Pipeline definition is required to start a run", http.StatusBadRequest)
		return
	}

	if err = yaml.Unmarshal(pipelineDef, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("Pipeline YAML is malformed: %v", err), http.StatusBadRequest)
		return
	}

	if pipelinePathForRun != "" && strings.Contains(pipelinePathForRun, "..") {
		http.Error(w, "Invalid pipeline path", http.StatusBadRequest)
		return
	}

	gitContext := map[string]string{
		"repo_owner":             r.Header.Get("X-Git-Repo-Owner"),
		"repo_name":              r.Header.Get("X-Git-Repo-Name"),
		"clone_url":              r.Header.Get("X-Git-Clone-URL"),
		"ssh_url":                r.Header.Get("X-Git-SSH-URL"),
		"ref":                    r.Header.Get("X-Git-Ref"),
		"target_ref":             r.Header.Get("X-Git-Target-Ref"),
		"commit_sha":             r.Header.Get("X-Git-Commit-SHA"),
		"commit_url":             r.Header.Get("X-Git-Commit-URL"),
		"commit_message":         r.Header.Get("X-Git-Commit-Message"),
		"commit_author_name":     r.Header.Get("X-Git-Commit-Author-Name"),
		"commit_author_email":    r.Header.Get("X-Git-Commit-Author-Email"),
		"commit_author_username": r.Header.Get("X-Git-Commit-Author-Username"),
		"pusher_name":            r.Header.Get("X-Git-Pusher-Name"),
		"pusher_email":           r.Header.Get("X-Git-Pusher-Email"),
		"check_run_id":           r.Header.Get("X-Git-Check-Run-ID"),
		"trigger_event_id":       r.Header.Get("X-Nopsai-Trigger-Event-ID"),
	}

	pipeline.Name = sanitizeInput(pipeline.Name)
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if !a.requireAAADecision(w, r, "pipeline.execute", routeauthz.PipelineResource(pipelinePathForRun, pipeline.Name)) {
		return
	}

	callerType, callerID := resourceUseCallerFromRequest(a, r, gitContext)
	triggerSource := runTriggerSourceFromRequest(r, gitContext)
	authChecks, err := a.authorizeRunResourceUses(r.Context(), callerType, callerID, triggerSource, gitContext, pipelinePathForRun, pipelineSource, pipeline, scope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	authSnapshot, err := buildRunAuthorizationSnapshot(triggerSource, callerType, callerID, authChecks)
	if err != nil {
		http.Error(w, "Failed to build authorization snapshot", http.StatusInternalServerError)
		return
	}

	rerunCommitSHA := r.Header.Get("X-Git-Rerun-Commit-SHA")
	if rerunCommitSHA != "" {
		log.Info().Str("commit_sha", rerunCommitSHA).Msg("Handling as a re-run: looking for original context.")
		var originalPusherName sql.NullString

		err := a.db.QueryRow(context.Background(),
			`SELECT git_pusher_name FROM pipeline_runs 
			 WHERE git_commit_sha = $1 AND git_repo_owner = $2 AND git_repo_name = $3
			 ORDER BY created_at DESC LIMIT 1`,
			rerunCommitSHA, gitContext["repo_owner"], gitContext["repo_name"]).Scan(&originalPusherName)

		if err != nil {
			log.Warn().Err(err).Str("commit_sha", rerunCommitSHA).Msg("Could not find original run to copy context from.")
		} else {
			if gitContext["pusher_name"] == "" && originalPusherName.Valid {
				gitContext["pusher_name"] = originalPusherName.String
				log.Info().Str("pusher_name", originalPusherName.String).Msg("Copied pusher name from original run.")
			}
		}
	}

	runID := uuid.New()

	var parentRunIDSQL sql.NullString
	if parentRunID != "" {
		parentRunIDSQL.String = parentRunID
		parentRunIDSQL.Valid = true
	}
	var parentStepNameSQL sql.NullString
	if parentStepName != "" {
		parentStepNameSQL.String = parentStepName
		parentStepNameSQL.Valid = true
	}
	var triggerEventIDSQL sql.NullString
	id := strings.TrimSpace(gitContext["trigger_event_id"])
	if id == "" {
		id = deriveTriggerEventID(gitContext)
	}
	if id == "" {
		id = runID.String()
	}
	if id != "" {
		triggerEventIDSQL.String = id
		triggerEventIDSQL.Valid = true
		gitContext["trigger_event_id"] = id
	}

	groupID, err := a.resolveGroupIDForRepo(gitContext["repo_owner"], gitContext["repo_name"])
	if err != nil {
		repoOwner := strings.TrimSpace(gitContext["repo_owner"])
		repoName := strings.TrimSpace(gitContext["repo_name"])
		repoFullName := repoName
		if repoOwner != "" {
			repoFullName = fmt.Sprintf("%s/%s", repoOwner, repoName)
		}
		log.Error().Err(err).Str("repo", repoFullName).Msg("Failed to find or create group for repository")
	}

	var checkRunIDSQL sql.NullInt64
	if val := gitContext["check_run_id"]; val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 64); err == nil {
			checkRunIDSQL.Int64 = parsed
			checkRunIDSQL.Valid = true
		}
	}

	_, err = a.db.Exec(context.Background(),
		`INSERT INTO pipeline_runs (run_id, parent_run_id, pipeline_name, pipeline_path, pipeline_version, status, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id, group_id, parent_step_name, trigger_event_id, scope, pipeline_source,
			trigger_source, requested_by_type, requested_by_id, effective_subject_type, effective_subject_id, authorization_snapshot)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32::jsonb)`,
		runID, parentRunIDSQL, pipeline.Name, pipelinePathForRun, pipeline.Version, string(pipelineDef),
		gitContext["repo_owner"], gitContext["repo_name"], gitContext["clone_url"], gitContext["ssh_url"], gitContext["ref"], gitContext["target_ref"],
		gitContext["commit_sha"], gitContext["commit_url"], gitContext["commit_message"], gitContext["commit_author_name"],
		gitContext["commit_author_email"], gitContext["commit_author_username"], gitContext["pusher_name"],
		gitContext["pusher_email"], checkRunIDSQL, groupID, parentStepNameSQL, triggerEventIDSQL, scope, pipelineSource,
		triggerSource, callerType, callerID, callerType, callerID, string(authSnapshot),
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to insert initial run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		a.notifyGitBotOfFinalStatus("failure", "", "", "Failed to create initial run record in DB", gitContext)
		return
	}

	if parentRunID != "" {
		parentPipelineName := r.Header.Get("X-Nopsai-Parent-Pipeline-Name")
		gitbotURL := fmt.Sprintf("%s/v1/checks/create-child", a.cfg.NopsaiGitBotAPIURL)

		payload := map[string]string{
			"owner":               gitContext["repo_owner"],
			"repo":                gitContext["repo_name"],
			"ref":                 gitContext["commit_sha"],
			"parent_name":         parentPipelineName,
			"include_name":        pipeline.Name,
			"pipeline_definition": string(pipelineDef),
		}

		// Run git-bot notification in background and update DB with the new check_run_id
		go func(rID string) {
			body, _ := json.Marshal(payload)
			resp, err := a.postJSON(gitbotURL, body)
			if err != nil {
				log.Error().Err(err).Msg("Failed to request new check run from git-bot (async)")
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				log.Error().Int("status", resp.StatusCode).Msg("Git-bot returned non-OK status for child check run (async)")
				return
			}

			var respData map[string]int64
			if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
				log.Error().Err(err).Msg("Failed to decode git-bot response (async)")
				return
			}

			checkRunID := respData["check_run_id"]
			// Update the record with the obtained check_run_id
			_, err = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, rID)
			if err != nil {
				log.Error().Err(err).Str("run_id", rID).Int64("check_run_id", checkRunID).Msg("Failed to update pipeline run with check_run_id (async)")
			}
		}(runID.String())
	}

	resolvedPipeline, err := a.resolveStepIncludes(&pipeline)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to resolve step includes: %v", err)
		http.Error(w, errMsg, http.StatusBadRequest)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}

	resolvedAuthChecks, err := a.authorizeRunResourceUses(r.Context(), callerType, callerID, triggerSource, gitContext, pipelinePathForRun, pipelineSource, *resolvedPipeline, scope)
	if err != nil {
		errMsg := err.Error()
		http.Error(w, errMsg, http.StatusForbidden)
		authChecks = mergeResourceUseAuthResults(authChecks, resolvedAuthChecks)
		if updatedSnapshot, snapshotErr := buildRunAuthorizationSnapshot(triggerSource, callerType, callerID, authChecks); snapshotErr == nil {
			_, _ = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET authorization_snapshot = $1::jsonb WHERE run_id = $2", string(updatedSnapshot), runID)
		}
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}
	authChecks = mergeResourceUseAuthResults(authChecks, resolvedAuthChecks)
	if updatedSnapshot, snapshotErr := buildRunAuthorizationSnapshot(triggerSource, callerType, callerID, authChecks); snapshotErr == nil && string(updatedSnapshot) != string(authSnapshot) {
		authSnapshot = updatedSnapshot
		_, _ = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET authorization_snapshot = $1::jsonb WHERE run_id = $2", string(authSnapshot), runID)
	}

	if err := validatePipeline(resolvedPipeline); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed: %v", err)
		http.Error(w, errMsg, http.StatusBadRequest)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}
	if err := a.validatePipelineLLMProfiles(resolvedPipeline, scope); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed: %v", err)
		http.Error(w, errMsg, http.StatusBadRequest)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}
	if err := a.validatePipelineMCPProfiles(resolvedPipeline, scope); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed: %v", err)
		http.Error(w, errMsg, http.StatusBadRequest)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}

	resolvedPipelineDef, err := yaml.Marshal(resolvedPipeline)
	if err != nil {
		errMsg := "Failed to marshal resolved pipeline"
		http.Error(w, errMsg, http.StatusInternalServerError)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		return
	}

	// Create or initialize GitHub check run without blocking the trigger path.
	a.ensureCheckRunAsync(runID, *resolvedPipeline, resolvedPipelineDef, gitContext, pipelineSource, rerunCommitSHA != "")

	timeoutStr := resolvedPipeline.Timeout
	if timeoutStr == "" {
		timeoutStr = a.getDefaultPipelineTimeout()
	}

	var timeoutDuration time.Duration
	if timeoutStr != "" {
		duration, err := time.ParseDuration(timeoutStr)
		if err != nil {
			http.Error(w, "Invalid timeout duration format", http.StatusBadRequest)
			return
		}
		timeoutDuration = duration
	}

	if timeoutDuration > 0 {
		timeoutAt := time.Now().Add(timeoutDuration)
		_, err := a.db.Exec(context.Background(), "UPDATE pipeline_runs SET timeout_at = $1 WHERE run_id = $2", timeoutAt, runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to update run timeout")
		}
	}

	a.launchAndRunPipeline(runID, parentRunID, parentRunnerID, *resolvedPipeline, resolvedPipelineDef, timeoutDuration, gitContext, parentHistory, scope, overrideVars)

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte("Pipeline run created successfully with ID: " + runID.String()))
}

func (a *App) handleRerunPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	originalRunID := r.PathValue("runID")

	var pipelineDef, pipelineName, pipelinePathDB, pipelineVersionDB, scope, pipelineSourceDB sql.NullString
	var gitContext = make(map[string]string)
	var timeoutAt sql.NullTime

	query := `SELECT
				pipeline_definition, pipeline_name, pipeline_path, pipeline_version, timeout_at, scope, COALESCE(pipeline_source, ''),
				git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
				git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
				git_commit_author_email, git_commit_author_username, git_pusher_name,
				git_pusher_email, git_check_run_id, trigger_event_id, status
			  FROM pipeline_runs WHERE run_id = $1`

	var repoOwner, repoName, cloneURL, sshURL, ref, targetRef, commitSHA, commitURL, commitMessage,
		commitAuthorName, commitAuthorEmail, commitAuthorUsername, pusherName, pusherEmail, triggerEventID sql.NullString
	var originalStatus string
	var checkRunID sql.NullInt64

	err := a.db.QueryRow(context.Background(), query, originalRunID).Scan(
		&pipelineDef, &pipelineName, &pipelinePathDB, &pipelineVersionDB, &timeoutAt, &scope, &pipelineSourceDB,
		&repoOwner, &repoName, &cloneURL, &sshURL, &ref, &targetRef, &commitSHA, &commitURL, &commitMessage,
		&commitAuthorName, &commitAuthorEmail, &commitAuthorUsername, &pusherName, &pusherEmail, &checkRunID, &triggerEventID, &originalStatus,
	)

	if err != nil {
		log.Error().Err(err).Str("original_run_id", originalRunID).Msg("Failed to find original run to rerun")
		http.Error(w, "Original pipeline run not found", http.StatusNotFound)
		return
	}

	if !isTerminalRunStatus(originalStatus) {
		http.Error(w, "Original pipeline run is still in progress; wait until it finishes before rerunning.", http.StatusConflict)
		return
	}

	if !pipelineDef.Valid {
		http.Error(w, "Original pipeline definition is missing, cannot rerun", http.StatusInternalServerError)
		return
	}

	if repoOwner.Valid {
		gitContext["repo_owner"] = repoOwner.String
	}
	if repoName.Valid {
		gitContext["repo_name"] = repoName.String
	}
	if cloneURL.Valid {
		gitContext["clone_url"] = cloneURL.String
	}
	if sshURL.Valid {
		gitContext["ssh_url"] = sshURL.String
	}
	if ref.Valid {
		gitContext["ref"] = ref.String
	}
	if targetRef.Valid {
		gitContext["target_ref"] = targetRef.String
	}
	if commitSHA.Valid {
		gitContext["commit_sha"] = commitSHA.String
	}
	if commitURL.Valid {
		gitContext["commit_url"] = commitURL.String
	}
	if commitMessage.Valid {
		gitContext["commit_message"] = commitMessage.String
	}
	if commitAuthorName.Valid {
		gitContext["commit_author_name"] = commitAuthorName.String
	}
	if commitAuthorEmail.Valid {
		gitContext["commit_author_email"] = commitAuthorEmail.String
	}
	if commitAuthorUsername.Valid {
		gitContext["commit_author_username"] = commitAuthorUsername.String
	}
	if pusherName.Valid {
		gitContext["pusher_name"] = pusherName.String
	}
	if pusherEmail.Valid {
		gitContext["pusher_email"] = pusherEmail.String
	}
	if checkRunID.Valid {
		gitContext["check_run_id"] = strconv.FormatInt(checkRunID.Int64, 10)
	}
	if triggerEventID.Valid {
		gitContext["trigger_event_id"] = triggerEventID.String
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineDef.String), &pipeline); err != nil {
		http.Error(w, "Could not parse original pipeline definition", http.StatusInternalServerError)
		return
	}
	pipeline.Version = normalizePipelineVersion(pipeline.Version)
	if pipeline.Version == "latest" && pipelineVersionDB.Valid {
		pipeline.Version = normalizePipelineVersion(pipelineVersionDB.String)
	}
	pipeline.Name = sanitizeInput(pipeline.Name)

	rerunCallerType, rerunCallerID := resourceUseCallerFromRequest(a, r, map[string]string{})
	rerunTriggerSource := strings.TrimSpace(r.Header.Get("X-Nopsai-Trigger-Source"))
	if rerunTriggerSource == "" {
		rerunTriggerSource = "manual_rerun"
	}
	authChecks, err := a.authorizeRunResourceUses(r.Context(), rerunCallerType, rerunCallerID, rerunTriggerSource, gitContext, pipelinePathDB.String, pipelineSourceDB.String, pipeline, scope.String)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	authSnapshot, err := buildRunAuthorizationSnapshot(rerunTriggerSource, rerunCallerType, rerunCallerID, authChecks)
	if err != nil {
		http.Error(w, "Failed to build authorization snapshot", http.StatusInternalServerError)
		return
	}

	var timeoutDuration time.Duration
	if timeoutAt.Valid {
		var originalCreatedAt time.Time
		err := a.db.QueryRow(context.Background(), "SELECT created_at FROM pipeline_runs WHERE run_id = $1", originalRunID).Scan(&originalCreatedAt)
		if err == nil {
			timeoutDuration = timeoutAt.Time.Sub(originalCreatedAt)
		}
	}

	runID := uuid.New()

	groupID, groupErr := a.resolveGroupIDForRepo(gitContext["repo_owner"], gitContext["repo_name"])
	if groupErr != nil {
		repoFullName := repositoryFullName(gitContext["repo_owner"], gitContext["repo_name"])
		log.Error().Err(groupErr).Str("repo", repoFullName).Msg("Failed to find or create group for rerun repository")
	}
	var triggerEventIDSQL sql.NullString
	newTriggerID := runID.String()
	if newTriggerID == "" {
		newTriggerID = uuid.New().String()
	}
	triggerEventIDSQL.String = newTriggerID
	triggerEventIDSQL.Valid = true
	gitContext["trigger_event_id"] = newTriggerID

	_, err = a.db.Exec(context.Background(),
		`INSERT INTO pipeline_runs (run_id, pipeline_name, pipeline_path, pipeline_version, status, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id, group_id, trigger_event_id, scope, pipeline_source,
			trigger_source, requested_by_type, requested_by_id, effective_subject_type, effective_subject_id, authorization_snapshot)
			VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30::jsonb)`,
		runID, pipeline.Name, pipelinePathDB.String, pipeline.Version, pipelineDef.String,
		gitContext["repo_owner"], gitContext["repo_name"], gitContext["clone_url"], gitContext["ssh_url"], gitContext["ref"], gitContext["target_ref"],
		gitContext["commit_sha"], gitContext["commit_url"], gitContext["commit_message"], gitContext["commit_author_name"],
		gitContext["commit_author_email"], gitContext["commit_author_username"], gitContext["pusher_name"],
		gitContext["pusher_email"], gitContext["check_run_id"], groupID, triggerEventIDSQL, scope.String,
		pipelineSourceDB.String, rerunTriggerSource, rerunCallerType, rerunCallerID, rerunCallerType, rerunCallerID, string(authSnapshot),
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert initial record for rerun")
		http.Error(w, "Failed to create rerun record", http.StatusInternalServerError)
		return
	}

	resolvedPipeline, err := a.resolveStepIncludes(&pipeline)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to resolve step includes on rerun: %v", err)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	resolvedAuthChecks, err := a.authorizeRunResourceUses(r.Context(), rerunCallerType, rerunCallerID, rerunTriggerSource, gitContext, pipelinePathDB.String, pipelineSourceDB.String, *resolvedPipeline, scope.String)
	if err != nil {
		errMsg := err.Error()
		authChecks = mergeResourceUseAuthResults(authChecks, resolvedAuthChecks)
		if updatedSnapshot, snapshotErr := buildRunAuthorizationSnapshot(rerunTriggerSource, rerunCallerType, rerunCallerID, authChecks); snapshotErr == nil {
			_, _ = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET authorization_snapshot = $1::jsonb WHERE run_id = $2", string(updatedSnapshot), runID)
		}
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusForbidden)
		return
	}
	authChecks = mergeResourceUseAuthResults(authChecks, resolvedAuthChecks)
	if updatedSnapshot, snapshotErr := buildRunAuthorizationSnapshot(rerunTriggerSource, rerunCallerType, rerunCallerID, authChecks); snapshotErr == nil && string(updatedSnapshot) != string(authSnapshot) {
		authSnapshot = updatedSnapshot
		_, _ = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET authorization_snapshot = $1::jsonb WHERE run_id = $2", string(authSnapshot), runID)
	}

	if err := validatePipeline(resolvedPipeline); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed on rerun: %v", err)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	if err := a.validatePipelineLLMProfiles(resolvedPipeline, scope.String); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed on rerun: %v", err)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	if err := a.validatePipelineMCPProfiles(resolvedPipeline, scope.String); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed on rerun: %v", err)
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}

	resolvedPipelineDef, err := yaml.Marshal(resolvedPipeline)
	if err != nil {
		errMsg := "Failed to marshal resolved pipeline on rerun"
		a.updateRunRecordWithFailure(runID, errMsg, gitContext)
		http.Error(w, errMsg, http.StatusInternalServerError)
		return
	}

	a.launchAndRunPipeline(runID, "", "", *resolvedPipeline, resolvedPipelineDef, timeoutDuration, gitContext, "", scope.String, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"runId":          runID.String(),
		"triggerEventId": newTriggerID,
	})
}

var errRunAlreadyCompleted = errors.New("run has already completed")

func (a *App) cancelRunHierarchy(ctx context.Context, runUUID uuid.UUID, reason, childReason string) error {
	_, err := a.markRunCancelled(ctx, runUUID, reason)
	if err != nil && !errors.Is(err, errRunAlreadyCompleted) {
		return err
	}
	if errors.Is(err, errRunAlreadyCompleted) {
		return err
	}

	rows, queryErr := a.db.Query(ctx, "SELECT run_id FROM pipeline_runs WHERE parent_run_id = $1", runUUID)
	if queryErr != nil {
		return queryErr
	}
	defer rows.Close()

	var childRunIDs []uuid.UUID
	for rows.Next() {
		var childRunID uuid.UUID
		if scanErr := rows.Scan(&childRunID); scanErr != nil {
			return scanErr
		}
		childRunIDs = append(childRunIDs, childRunID)
	}
	if rows.Err() != nil {
		return rows.Err()
	}

	for _, childRunID := range childRunIDs {
		if childErr := a.cancelRunHierarchy(ctx, childRunID, childReason, childReason); childErr != nil && !errors.Is(childErr, errRunAlreadyCompleted) {
			return childErr
		}
	}

	return nil
}

func (a *App) markRunCancelled(ctx context.Context, runUUID uuid.UUID, reason string) (bool, error) {
	var status string
	var repoName, repoOwner, commitSHA sql.NullString
	var checkRunID sql.NullInt64

	err := a.db.QueryRow(ctx, `
		SELECT status, git_repo_name, git_repo_owner, git_commit_sha, git_check_run_id
		FROM pipeline_runs
		WHERE run_id = $1`, runUUID).Scan(&status, &repoName, &repoOwner, &commitSHA, &checkRunID)
	if err != nil {
		return false, err
	}

	statusLower := strings.ToLower(strings.TrimSpace(status))
	if isCompletedRunStatus(statusLower) {
		return false, errRunAlreadyCompleted
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	changed := statusLower != "cancelled"
	if changed {
		if _, err := tx.Exec(ctx, "UPDATE pipeline_runs SET status = 'cancelled', finished_at = COALESCE(finished_at, NOW()) WHERE run_id = $1", runUUID); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runUUID, reason); err != nil {
			log.Warn().Err(err).Str("run_id", runUUID.String()).Msg("Failed to record cancellation log line")
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE task_runs
		SET status = 'cancelled', finished_at = COALESCE(finished_at, NOW())
		WHERE run_id = $1
		  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'skipped', 'cancelled')`, runUUID); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	if changed && checkRunID.Valid {
		gitContext := map[string]string{
			"repo_owner":   repoOwner.String,
			"repo_name":    repoName.String,
			"commit_sha":   commitSHA.String,
			"check_run_id": strconv.FormatInt(checkRunID.Int64, 10),
		}
		a.notifyGitBotOfFinalStatus("cancelled", "", "", reason, gitContext)
	}

	return changed, nil
}

func (a *App) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	runIDStr := r.PathValue("runID")
	runUUID, err := uuid.Parse(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	if err := a.cancelRunHierarchy(context.Background(), runUUID, "Run cancelled by user.", "Run cancelled because parent run was cancelled."); err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "Pipeline run not found", http.StatusNotFound)
		case errors.Is(err, errRunAlreadyCompleted):
			http.Error(w, "Run has already completed", http.StatusBadRequest)
		default:
			log.Error().Err(err).Str("run_id", runUUID.String()).Msg("Failed to cancel run")
			http.Error(w, "Failed to cancel run", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
}

func (a *App) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" {
		http.Error(w, "Run ID is required", http.StatusBadRequest)
		return
	}

	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	commandTag, err := a.db.Exec(context.Background(), "DELETE FROM pipeline_runs WHERE run_id = $1", runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to delete pipeline run")
		http.Error(w, "Failed to delete pipeline run", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, "Pipeline run not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteRepoBranchRuns(w http.ResponseWriter, r *http.Request) {
	repoOwner := strings.TrimSpace(r.PathValue("repoOwner"))
	repoName := strings.TrimSpace(r.PathValue("repoName"))
	branchParam := strings.TrimSpace(r.PathValue("branch"))

	if repoOwner == "" || repoName == "" {
		http.Error(w, "Repository owner and name are required", http.StatusBadRequest)
		return
	}
	if branchParam == "" {
		http.Error(w, "Branch name is required", http.StatusBadRequest)
		return
	}

	branch := strings.Trim(branchParam, " ")
	branch = strings.Trim(branch, "/")

	var commandTag pgconn.CommandTag
	var err error
	branchLower := strings.ToLower(branch)

	ctx := context.Background()

	if branchLower == "others" {
		commandTag, err = a.db.Exec(ctx,
			"DELETE FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND (git_ref IS NULL OR git_ref = '')",
			repoOwner, repoName,
		)
	} else {
		normalized := branch
		if strings.HasPrefix(normalized, "refs/") {
			normalized = strings.TrimPrefix(normalized, "refs/heads/")
		}
		refWithPrefix := "refs/heads/" + normalized

		commandTag, err = a.db.Exec(ctx,
			"DELETE FROM pipeline_runs WHERE git_repo_owner = $1 AND git_repo_name = $2 AND (git_ref = $3 OR git_ref = $4)",
			repoOwner, repoName, refWithPrefix, normalized,
		)
	}

	if err != nil {
		log.Error().Err(err).
			Str("repo_owner", repoOwner).
			Str("repo_name", repoName).
			Str("branch", branch).Msg("Failed to delete pipeline runs for branch")
		http.Error(w, "Failed to delete runs for branch", http.StatusInternalServerError)
		return
	}

	if commandTag.RowsAffected() == 0 {
		http.Error(w, "No pipeline runs found for the specified branch", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleTaskUpdate(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	stepName := r.PathValue("stepName")
	taskName := r.PathValue("taskName")

	var update StepStatusUpdate
	if err := httpapi.DecodeJSON(r, &update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var runStatus string
	if err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&runStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to load run status for task update")
		http.Error(w, "Failed to update task status", http.StatusInternalServerError)
		return
	}

	runStatus = strings.ToLower(strings.TrimSpace(runStatus))
	if isTerminalRunStatus(runStatus) {
		if runStatus == "cancelled" && strings.EqualFold(update.Status, "cancelled") {
			query := `
				UPDATE task_runs
				SET status = 'cancelled', finished_at = COALESCE(finished_at, NOW())
				WHERE run_id = $1 AND step_name = $2 AND task_name = $3
				  AND status NOT IN ('success', 'failure', 'failure (ignored)', 'skipped', 'cancelled')`
			if _, err := a.db.Exec(context.Background(), query, runID, stepName, taskName); err != nil {
				log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to preserve cancelled task status")
				http.Error(w, "Failed to update task status", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		log.Info().
			Str("run_id", runID).
			Str("step", stepName).
			Str("task", taskName).
			Str("status", update.Status).
			Str("run_status", runStatus).
			Msg("Ignoring late task status update for terminal run")
		w.WriteHeader(http.StatusOK)
		return
	}

	if update.Status == "running" {
		tx, err := a.db.Begin(context.Background())
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to start transaction for task update")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback(context.Background())

		_, err = tx.Exec(context.Background(), "UPDATE task_runs SET status = 'running', started_at = NOW() WHERE run_id = $1 AND step_name = $2 AND task_name = $3", runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}

		err = markRunRunning(context.Background(), tx, runID)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to update run start time")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(context.Background()); err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to commit transaction for task update")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	} else {
		query := "UPDATE task_runs SET status = $1, exit_code = $2, finished_at = NOW() WHERE run_id = $3 AND step_name = $4 AND task_name = $5"
		_, err := a.db.Exec(context.Background(), query, update.Status, update.ExitCode, runID, stepName, taskName)
		if err != nil {
			log.Error().Err(err).Str("run_id", runID).Str("step", stepName).Str("task", taskName).Msg("Failed to update task finish status")
			http.Error(w, "Failed to update task status", http.StatusInternalServerError)
			return
		}
	}

	log.Info().Str("run_id", runID).Str("step", stepName).Str("task", taskName).Str("status", update.Status).Msg("Updated task status")

	go a.notifyGitBotOfTaskStatus(runID, stepName, taskName, update.Status)

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleFinalizeRun(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var req FinalizeRequest
	if err := httpapi.DecodeJSON(r, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Info().Str("run_id", runID).Str("status", req.Status).Msg("Received final status from agent")

	var currentStatus string
	if err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&currentStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to load run before finalization")
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}

	currentStatus = strings.ToLower(strings.TrimSpace(currentStatus))
	if currentStatus == "cancelled" {
		log.Info().Str("run_id", runID).Str("status", req.Status).Msg("Ignoring final status because run is already cancelled")
		w.WriteHeader(http.StatusOK)
		return
	}
	if isTerminalRunStatus(currentStatus) {
		log.Info().Str("run_id", runID).Str("status", req.Status).Str("current_status", currentStatus).Msg("Ignoring final status for completed run")
		w.WriteHeader(http.StatusOK)
		return
	}

	finalStatus := normalizeFinalizeRunStatus(req.Status)
	var failedStep, failedTask string
	if finalStatus == "failure" {
		err := a.db.QueryRow(context.Background(), "SELECT step_name, task_name FROM task_runs WHERE run_id = $1 AND status NOT IN ('success', 'pending', 'skipped', 'failure (ignored)', 'running') ORDER BY finished_at ASC, started_at ASC LIMIT 1", runID).Scan(&failedStep, &failedTask)
		if err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("Could not determine the exact failed task for final status notification.")
		}
	}

	var gitContext = make(map[string]string)
	var repoOwner, repoName, commitSHA sql.NullString
	var checkRunID sql.NullInt64
	query := `SELECT git_repo_owner, git_repo_name, git_commit_sha, git_check_run_id FROM pipeline_runs WHERE run_id = $1`
	err := a.db.QueryRow(context.Background(), query, runID).Scan(&repoOwner, &repoName, &commitSHA, &checkRunID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to retrieve git context for final notification")
	} else {
		if repoOwner.Valid {
			gitContext["repo_owner"] = repoOwner.String
		}
		if repoName.Valid {
			gitContext["repo_name"] = repoName.String
		}
		if commitSHA.Valid {
			gitContext["commit_sha"] = commitSHA.String
		}
		if checkRunID.Valid {
			gitContext["check_run_id"] = strconv.FormatInt(checkRunID.Int64, 10)
		}
	}

	_, err = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = $1, finished_at = COALESCE(finished_at, NOW()) WHERE run_id = $2 AND status != 'cancelled'", finalStatus, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to update final run status in DB from agent notification")
	}

	if gitContext["repo_owner"] != "" {
		// Run git-bot notification in background to prevent agent hang
		go a.notifyGitBotOfFinalStatus(finalStatus, failedStep, failedTask, "", gitContext)
	}

	w.WriteHeader(http.StatusOK)
}

func (a *App) handleGetRunStatus(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	var status string
	err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&status)
	if err != nil {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}

func (a *App) handleGetRunByCheckID(w http.ResponseWriter, r *http.Request) {
	checkRunID := r.PathValue("checkRunID")
	var runID string
	// Find the latest run for this check_run_id, as there could be multiple re-runs
	err := a.db.QueryRow(context.Background(),
		"SELECT run_id FROM pipeline_runs WHERE git_check_run_id = $1 ORDER BY created_at DESC LIMIT 1",
		checkRunID).Scan(&runID)
	if err != nil {
		http.Error(w, "Run not found for this check run ID", http.StatusNotFound)
		return
	}
	if !a.requireAAADecision(w, r, "pipeline_run.read", routeauthz.RunResource(runID)) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"run_id": runID})
}

func (a *App) launchAgent(runID string, parentRunID string, parentRunnerID string, pipeline models.Pipeline, pipelineDef []byte, timeout time.Duration, gitContext map[string]string, parentHistory string, scope string, overrides map[string]string) {
	ctx := context.Background()
	cfg := a.getConfigSnapshot()

	var currentStatus string
	if err := a.db.QueryRow(ctx, "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&currentStatus); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to load run before agent launch")
		return
	}
	if isTerminalRunStatus(currentStatus) {
		log.Info().Str("run_id", runID).Str("status", currentStatus).Msg("Skipping agent launch for terminal run")
		return
	}

	secrets, err := a.prepareSecretsForPipeline(runID, pipeline, gitContext, scope)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare secrets for pipeline")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", err.Error(), runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", err.Error(), gitContext)
		}
		return
	}

	finalVars, err := a.prepareVariablesForPipeline(runID, pipeline, gitContext, scope, overrides)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to prepare scope variables for pipeline")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", err.Error(), runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", err.Error(), gitContext)
		}
		return
	}

	if err := a.validatePipelineLLMProfiles(&pipeline, scope); err != nil {
		reason := err.Error()
		log.Error().Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
		}
		return
	}
	if err := a.validatePipelineMCPProfiles(&pipeline, scope); err != nil {
		reason := err.Error()
		log.Error().Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
		}
		return
	}

	runtimeProfiles, err := a.buildRuntimeLLMProfiles(cfg)
	if err != nil {
		reason := fmt.Sprintf("Failed to prepare LLM profiles: %v", err)
		log.Error().Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		return
	}
	runtimeProfilesJSON, err := json.Marshal(runtimeProfiles)
	if err != nil {
		reason := "Failed to marshal LLM profiles"
		log.Error().Err(err).Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		return
	}
	runtimeMCPRegistry, err := a.buildRuntimeMCPRegistry(&pipeline, scope)
	if err != nil {
		reason := fmt.Sprintf("Failed to prepare MCP registry: %v", err)
		log.Error().Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		return
	}
	runtimeMCPRegistryJSON, err := json.Marshal(runtimeMCPRegistry)
	if err != nil {
		reason := "Failed to marshal MCP registry"
		log.Error().Err(err).Str("run_id", runID).Msg(reason)
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", reason, runID)
		return
	}

	agentImageName := a.getAgentImage()
	if agentImageName == "" {
		agentImageName = "nopsai-agent:latest"
	}

	dispatcherAddr := strings.TrimSpace(a.cfg.DispatcherAddress)
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}

	sharedVolumeName := fmt.Sprintf("vol-%s", runID)

	repoName := gitContext["repo_name"]
	triggerEventID := strings.TrimSpace(gitContext["trigger_event_id"])
	agentContainerName := buildAgentContainerName(pipeline.Name, repoName, triggerEventID, runID)
	preferredRunnerID := strings.TrimSpace(parentRunnerID)

	secretsJSON, err := json.Marshal(secrets)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to marshal secrets")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Failed to marshal secrets", runID)
		return
	}

	envVars := []string{
		fmt.Sprintf("RUN_ID=%s", runID),
		fmt.Sprintf("PIPELINE_NAME=%s", pipeline.Name),
		fmt.Sprintf("PIPELINE_VERSION=%s", pipeline.Version),
		fmt.Sprintf("%s=%s", llmProfilesRuntimeEnv, base64.StdEncoding.EncodeToString(runtimeProfilesJSON)),
		fmt.Sprintf("%s=%s", mcpRegistryRuntimeEnv, base64.StdEncoding.EncodeToString(runtimeMCPRegistryJSON)),
		fmt.Sprintf("NOPSAI_API_URL=%s", cfg.AgentNopsaiAPIURL),
		fmt.Sprintf("LOG_LEVEL=%s", cfg.LogLevel),
		fmt.Sprintf("LOG_FORMAT=%s", cfg.LogFormat),
		fmt.Sprintf("PIPELINE_DEFINITION=%s", base64.StdEncoding.EncodeToString(pipelineDef)),
		fmt.Sprintf("SHARED_VOLUME_NAME=%s", sharedVolumeName),
		fmt.Sprintf("DOCKER_NETWORK_NAME=%s", a.getDockerNetworkName()),
		fmt.Sprintf("NOPSAI_SECRETS=%s", base64.StdEncoding.EncodeToString(secretsJSON)),
		fmt.Sprintf("DISPATCHER_ADDRESS=%s", dispatcherAddr),
		fmt.Sprintf("%s=%s", serviceauth.EnvSigningKey, cfg.EffectiveServiceJWTSigningKey()),
		fmt.Sprintf("%s=%s", serviceauth.EnvIssuer, cfg.EffectiveServiceJWTIssuer()),
		fmt.Sprintf("%s=%s", serviceauth.EnvAudience, cfg.EffectiveServiceJWTAudience()),
		fmt.Sprintf("%s=%s", serviceauth.EnvServiceID, cfg.EffectiveAgentServiceID()),
		fmt.Sprintf("%s=%s", servicetls.EnvMode, cfg.EffectiveDispatcherTLSMode()),
		fmt.Sprintf("%s=%s", servicetls.EnvSecret, cfg.EffectiveDispatcherTLSSecret()),
		fmt.Sprintf("%s=%s", servicetls.EnvServerName, cfg.EffectiveDispatcherTLSServerName()),
	}
	if timeout > 0 {
		envVars = append(envVars, fmt.Sprintf("PIPELINE_TIMEOUT=%s", timeout.String()))
	}
	if a.getLLMAgentTimeout() != "" {
		envVars = append(envVars, fmt.Sprintf("LLM_AGENT_TIMEOUT=%s", a.getLLMAgentTimeout()))
	}
	if parentHistory != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_EXECUTION_HISTORY=%s", parentHistory))
	}
	if scope != "" {
		envVars = append(envVars, fmt.Sprintf("SCOPE=%s", scope))
	}
	if preferredRunnerID != "" {
		envVars = append(envVars, fmt.Sprintf("PARENT_RUNNER_ID=%s", preferredRunnerID))
	}

	variablesJSON, err := json.Marshal(finalVars)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to marshal variables")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Failed to marshal variables", runID)
		return
	}

	envVars = append(envVars, fmt.Sprintf("NOPSAI_VARIABLES=%s", base64.StdEncoding.EncodeToString(variablesJSON)))

	for key, value := range gitContext {
		envKey := fmt.Sprintf("GIT_%s", strings.ToUpper(key))
		envVars = append(envVars, fmt.Sprintf("%s=%s", envKey, value))
	}

	initialLines := []string{}
	if triggerEventID != "" {
		initialLines = append(initialLines, fmt.Sprintf("Trigger Event ID: %s", triggerEventID))
	} else {
		initialLines = append(initialLines, "Trigger Event ID: N/A")
	}
	initialLines = append(initialLines, fmt.Sprintf("Preparing agent container %s with image %s", agentContainerName, agentImageName))

	appendLogs := func(lines ...string) {
		if len(lines) == 0 {
			return
		}
		dbBatch := &pgx.Batch{}
		for _, line := range lines {
			dbBatch.Queue("INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runID, line)
		}
		if br := a.db.SendBatch(context.Background(), dbBatch); br != nil {
			if err := br.Close(); err != nil {
				log.Error().Err(err).Str("run_id", runID).Msg("Failed to write log lines")
			}
		}
	}

	appendLogs(initialLines...)

	affinityKey := triggerEventID
	if affinityKey == "" {
		affinityKey = strings.TrimSpace(parentRunID)
	}
	if affinityKey == "" {
		affinityKey = runID
	}

	job := &proto.JobRequest{
		RunId:              runID,
		PipelineName:       pipeline.Name,
		PipelineVersion:    pipeline.Version,
		PipelineDefinition: pipelineDef,
		Env:                envVars,
		AgentImage:         agentImageName,
		SharedVolumeName:   sharedVolumeName,
		DockerNetwork:      a.getDockerNetworkName(),
		AutoRemove:         a.getAutoRemovalAgentContainer(),
		ContainerName:      agentContainerName,
		Scope:              scope,
		NopsaiApiUrl:       strings.TrimSpace(a.cfg.AgentNopsaiAPIURL),
		TriggerEventId:     triggerEventID,
		RunnerAffinityKey:  affinityKey,
		// Parent runner affinity is a locality hint. Scope routing remains the
		// eligibility boundary, so child runs can use any runner in the route.
		PreferredRunnerId: preferredRunnerID,
	}

	resp, err := a.dispatcher.SubmitJob(ctx, job)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to dispatch job to runner")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Failed to dispatch job to runner", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", "Failed to dispatch job to runner", gitContext)
		}
		appendLogs("Failed to dispatch job to runner: " + err.Error())
		return
	}

	switch resp.State {
	case proto.JobState_JOB_STATE_ASSIGNED:
		if err := markRunRunning(context.Background(), a.db, runID); err != nil {
			log.Error().Err(err).Str("run_id", runID).Msg("Failed to mark run as running")
		}
		log.Info().Str("run_id", runID).Str("runner_id", resp.RunnerId).Msg("Job dispatched to runner")
		appendLogs(fmt.Sprintf("Dispatched to runner %s", resp.RunnerId))
	case proto.JobState_JOB_STATE_QUEUED:
		log.Info().Str("run_id", runID).Msg("No runner available; job queued")
		appendLogs("No runner available; job queued by dispatcher")
	default:
		var latestStatus string
		if err := a.db.QueryRow(context.Background(), "SELECT status FROM pipeline_runs WHERE run_id = $1", runID).Scan(&latestStatus); err == nil && isTerminalRunStatus(latestStatus) {
			log.Info().Str("run_id", runID).Str("status", latestStatus).Msg("Dispatcher rejected job for terminal run")
			appendLogs(fmt.Sprintf("Dispatcher skipped agent launch because run is %s", strings.ToLower(strings.TrimSpace(latestStatus))))
			return
		}
		log.Error().Str("run_id", runID).Str("state", resp.State.String()).Msg("Dispatcher rejected job")
		a.db.Exec(context.Background(), "UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2", "Dispatcher rejected job", runID)
		if gitContext["repo_owner"] != "" {
			a.notifyGitBotOfFinalStatus("failure", "", "", "Dispatcher rejected job", gitContext)
		}
		appendLogs("Dispatcher rejected job")
	}
}

func (a *App) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	runID := strings.TrimSpace(r.PathValue("runID"))
	if runID == "" {
		http.Error(w, "Run ID is required", http.StatusBadRequest)
		return
	}
	if _, err := uuid.Parse(runID); err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	var payload struct {
		Lines []string `json:"lines"`
	}
	if err := httpapi.DecodeJSON(r, &payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(payload.Lines) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	batch := &pgx.Batch{}
	for _, line := range payload.Lines {
		batch.Queue("INSERT INTO pipeline_run_logs (run_id, line) VALUES ($1, $2)", runID, line)
	}
	br := a.db.SendBatch(context.Background(), batch)
	if err := br.Close(); err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to ingest log batch")
		http.Error(w, "Failed to persist logs", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleGetRunLogs(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("runID")
	sinceLineStr := r.URL.Query().Get("since_line")
	var lastID int64 = 0
	if sinceLineStr != "" {
		if parsed, err := strconv.ParseInt(sinceLineStr, 10, 64); err == nil {
			lastID = parsed
		}
	}

	rows, err := a.db.Query(context.Background(), "SELECT id, timestamp, line FROM pipeline_run_logs WHERE run_id = $1 AND id > $2 ORDER BY id ASC", runID, lastID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query logs for run")
		http.Error(w, "Failed to retrieve logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []LogLine
	for rows.Next() {
		var logLine LogLine
		if err := rows.Scan(&logLine.ID, &logLine.Timestamp, &logLine.Line); err != nil {
			log.Error().Err(err).Msg("Failed to scan log line")
			continue
		}
		logs = append(logs, logLine)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
