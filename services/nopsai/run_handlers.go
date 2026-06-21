package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/httpapi"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/internal/configsync"
	runquery "nopsai/services/nopsai/internal/runs"
	"nopsai/services/nopsai/pkg/routeauthz"
)

func (a *App) handleListRuns(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
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

	var groupID *int
	rootGroup := false
	if groupIDStr := r.URL.Query().Get("groupId"); groupIDStr != "" {
		if strings.EqualFold(groupIDStr, rootGrantID) {
			rootGroup = true
		} else {
			parsedGroupID, err := strconv.Atoi(groupIDStr)
			if err != nil {
				http.Error(w, "Invalid group ID", http.StatusBadRequest)
				return
			}
			groupID = &parsedGroupID
		}
	}

	allRuns, err := runquery.List(r.Context(), a.db, runquery.ListFilter{
		GroupID:   groupID,
		RootGroup: rootGroup,
		Branch:    r.URL.Query().Get("branch"),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to query runs from database")
		http.Error(w, "Failed to retrieve runs", http.StatusInternalServerError)
		return
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
	approvableSet, err := a.approvableRunSet(r, runResources)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}

	filteredRuns := make([]RunListItem, 0, len(allRuns))
	for _, run := range allRuns {
		runKey := resourceKey(routeauthz.RunResource(run.RunID))
		if _, ok := allowedSet[runKey]; !ok {
			if _, ok := approvableSet[runKey]; !ok {
				continue
			}
		}
		filteredRuns = append(filteredRuns, run)
	}

	if groupID != nil || rootGroup {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runquery.GroupByBranch(filteredRuns))
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(filteredRuns)
	}
}

func (a *App) handleGetRunDetails(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	runID := r.PathValue("runID")

	record, err := runquery.LoadRunRecord(r.Context(), a.db, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query run details from database")
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	run := record.Run

	authorized, err := a.canReadRunOrApprove(r, runID)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	if !authorized {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	parentRunInfo, err := runquery.LoadParentRunInfo(r.Context(), a.db, run.ParentRunID)
	if err != nil {
		parentRunID := ""
		if run.ParentRunID != nil {
			parentRunID = *run.ParentRunID
		}
		log.Error().Err(err).Str("parent_run_id", parentRunID).Msg("Failed to query parent pipeline name")
	}

	childRuns, err := runquery.LoadChildRuns(r.Context(), a.db, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query child runs for details view")
		childRuns = []RunListItem{}
	}

	tasksByStep, err := runquery.LoadTaskDetailsByStep(r.Context(), a.db, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query tasks for run")
		http.Error(w, "Failed to retrieve tasks", http.StatusInternalServerError)
		return
	}

	stepAIUsage, err := runquery.LoadAIUsageByStep(r.Context(), a.db, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query AI usage by step for run")
		http.Error(w, "Failed to retrieve run AI usage", http.StatusInternalServerError)
		return
	}
	taskAIUsage, err := runquery.LoadAIUsageByTask(r.Context(), a.db, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query AI usage by task for run")
		http.Error(w, "Failed to retrieve run AI usage", http.StatusInternalServerError)
		return
	}

	var originalPipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(record.PipelineDefinitionYAML), &originalPipeline); err != nil {
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

	knowledgeContexts, err := a.loadRunKnowledgeContextSnapshots(r.Context(), runID)
	if err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("Failed to load run knowledge context snapshots")
	}

	finalOutputs, err := runquery.LoadFinalOutputs(r.Context(), a.db, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("Failed to query final outputs for run")
		http.Error(w, "Failed to retrieve run final outputs", http.StatusInternalServerError)
		return
	}

	response := runquery.BuildDetail(runquery.DetailBuildInput{
		Run:                    run,
		PipelineDefinitionYAML: record.PipelineDefinitionYAML,
		OriginalPipeline:       originalPipeline,
		ResolvedPipeline:       *resolvedPipeline,
		ChildRuns:              childRuns,
		TasksByStep:            tasksByStep,
		StepAIUsage:            stepAIUsage,
		TaskAIUsage:            taskAIUsage,
		KnowledgeContexts:      knowledgeContexts,
		FinalOutputs:           finalOutputs,
		ParentRunInfo:          parentRunInfo,
	})

	etag := runquery.BuildRunDetailETag(run, childRuns, tasksByStep, stepAIUsage, taskAIUsage, finalOutputs)
	w.Header().Set("ETag", etag)
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *App) handleDownloadRunFinalOutput(w http.ResponseWriter, r *http.Request) {
	setNoStoreHeaders(w)
	runID := strings.TrimSpace(r.PathValue("runID"))
	outputID := strings.TrimSpace(r.PathValue("outputID"))
	if runID == "" || outputID == "" {
		http.Error(w, "Run ID and output ID are required", http.StatusBadRequest)
		return
	}
	authorized, err := a.canReadRunOrApprove(r, runID)
	if err != nil {
		http.Error(w, "Authorization unavailable", http.StatusServiceUnavailable)
		return
	}
	if !authorized {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	output, err := a.loadPipelineFinalOutputForDownload(r.Context(), runID, outputID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Final output not found", http.StatusNotFound)
			return
		}
		log.Error().Err(err).Str("run_id", runID).Str("output_id", outputID).Msg("Failed to load final output")
		http.Error(w, "Failed to load final output", http.StatusInternalServerError)
		return
	}
	if strings.TrimSpace(output.Status) != finalOutputStatusSuccess || strings.TrimSpace(output.Content) == "" {
		_, _, _, renderErr := renderPipelineFinalOutputDownload(r.Context(), output, nil)
		http.Error(w, renderErr.Error(), http.StatusConflict)
		return
	}
	var pdfConverter pipelinePDFConverter
	if normalizePipelineFinalOutputType(output.Type) == "pdf" {
		pdfConverter, err = a.pipelineFinalOutputPDFConverter()
		if err != nil {
			a.recordPipelineFinalOutputRenderResult(r.Context(), output.ID, false)
			log.Error().Err(err).Str("run_id", runID).Str("output_id", outputID).Msg("PDF renderer is unavailable")
			http.Error(w, "PDF renderer is unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	payload, contentType, filename, err := renderPipelineFinalOutputDownload(r.Context(), output, pdfConverter)
	a.recordPipelineFinalOutputRenderResult(r.Context(), output.ID, err == nil)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Str("output_id", outputID).Str("output_type", output.Type).Msg("Failed to render final output")
		http.Error(w, "Failed to render final output", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
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
		pathPart, namePart, _, parseErr := configsync.SplitPipelineIdentifier(pipelineNameFromPath)
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
		"event_type":             r.Header.Get("X-Nopsai-Git-Event-Type"),
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

	groupPathForRun := strings.Trim(strings.TrimSpace(r.Header.Get("X-Nopsai-Group-Path")), "/")
	runs := newRunService(a)
	record, err := runs.createPendingRun(r.Context(), createPendingRunRequest{
		Pipeline:           pipeline,
		PipelinePath:       pipelinePathForRun,
		PipelineDefinition: pipelineDef,
		ParentRunID:        parentRunID,
		ParentStepName:     parentStepName,
		Scope:              scope,
		PipelineSource:     pipelineSource,
		TriggerSource:      triggerSource,
		CallerType:         callerType,
		CallerID:           callerID,
		GitContext:         gitContext,
		GroupPath:          groupPathForRun,
		AuthSnapshot:       authSnapshot,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert initial run record")
		http.Error(w, "Failed to create run record", http.StatusInternalServerError)
		a.notifyGitBotOfFinalStatus("failure", "", "", "Failed to create initial run record in DB", gitContext)
		return
	}
	runID := record.RunID

	createGitHubCheck := !strings.HasPrefix(strings.ToLower(strings.TrimSpace(triggerSource)), "git_webhook_")
	if parentRunID != "" && strings.EqualFold(strings.TrimSpace(triggerSource), "child_pipeline") {
		var parentTriggerSource sql.NullString
		if err := a.db.QueryRow(r.Context(), `
			SELECT trigger_source
			FROM pipeline_runs
			WHERE run_id = $1
		`, parentRunID).Scan(&parentTriggerSource); err == nil {
			createGitHubCheck = strings.HasPrefix(strings.ToLower(strings.TrimSpace(parentTriggerSource.String)), "github")
		}
	}
	if parentRunID != "" && createGitHubCheck {
		parentPipelineName := r.Header.Get("X-Nopsai-Parent-Pipeline-Name")

		// Run git-bot notification in background and update DB with the new check_run_id
		go func(rID string) {
			checkRunID, err := a.createChildGitHubCheckRun(
				gitContext["repo_owner"],
				gitContext["repo_name"],
				gitContext["commit_sha"],
				parentPipelineName,
				pipeline.Name,
				pipelineDef,
			)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create child check run through git-bot (async)")
				return
			}

			// Update the record with the obtained check_run_id
			_, err = a.db.Exec(context.Background(), "UPDATE pipeline_runs SET git_check_run_id = $1 WHERE run_id = $2", checkRunID, rID)
			if err != nil {
				log.Error().Err(err).Str("run_id", rID).Int64("check_run_id", checkRunID).Msg("Failed to update pipeline run with check_run_id (async)")
			}
		}(runID.String())
	}

	preparedRun, prepErr := runs.preparePipelineRun(r.Context(), preparePipelineRunRequest{
		RunID:          runID,
		Pipeline:       pipeline,
		PipelinePath:   pipelinePathForRun,
		PipelineSource: pipelineSource,
		Scope:          scope,
		TriggerSource:  triggerSource,
		CallerType:     callerType,
		CallerID:       callerID,
		GitContext:     gitContext,
		AuthChecks:     authChecks,
		AuthSnapshot:   authSnapshot,
	})
	if prepErr != nil {
		http.Error(w, prepErr.Message, prepErr.StatusCode)
		return
	}
	resolvedPipeline := preparedRun.Pipeline
	resolvedPipelineDef := preparedRun.PipelineDefinition

	if createGitHubCheck {
		// Create or initialize GitHub check run without blocking the trigger path.
		a.ensureCheckRunAsync(runID, resolvedPipeline, resolvedPipelineDef, gitContext, pipelineSource, rerunCommitSHA != "")
	}

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

	runs.launchPipeline(runID, parentRunID, parentRunnerID, resolvedPipeline, resolvedPipelineDef, timeoutDuration, gitContext, parentHistory, scope, overrideVars)

	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"run_id":           runID.String(),
			"trigger_event_id": gitContext["trigger_event_id"],
		})
		return
	}
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

	runs := newRunService(a)
	record, err := runs.createPendingRun(r.Context(), createPendingRunRequest{
		Pipeline:           pipeline,
		PipelinePath:       pipelinePathDB.String,
		PipelineDefinition: []byte(pipelineDef.String),
		Scope:              scope.String,
		PipelineSource:     pipelineSourceDB.String,
		TriggerSource:      rerunTriggerSource,
		CallerType:         rerunCallerType,
		CallerID:           rerunCallerID,
		GitContext:         gitContext,
		AuthSnapshot:       authSnapshot,
		NewTriggerEventID:  true,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert initial record for rerun")
		http.Error(w, "Failed to create rerun record", http.StatusInternalServerError)
		return
	}
	runID := record.RunID

	preparedRun, prepErr := runs.preparePipelineRun(r.Context(), preparePipelineRunRequest{
		RunID:          runID,
		Pipeline:       pipeline,
		PipelinePath:   pipelinePathDB.String,
		PipelineSource: pipelineSourceDB.String,
		Scope:          scope.String,
		TriggerSource:  rerunTriggerSource,
		CallerType:     rerunCallerType,
		CallerID:       rerunCallerID,
		GitContext:     gitContext,
		AuthChecks:     authChecks,
		AuthSnapshot:   authSnapshot,
		ErrorContext:   " on rerun",
	})
	if prepErr != nil {
		http.Error(w, prepErr.Message, prepErr.StatusCode)
		return
	}

	runs.launchPipeline(runID, "", "", preparedRun.Pipeline, preparedRun.PipelineDefinition, timeoutDuration, gitContext, "", scope.String, nil)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"runId":          runID.String(),
		"triggerEventId": record.TriggerEventID,
	})
}
