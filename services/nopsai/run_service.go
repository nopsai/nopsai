package nopsai

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

type runService struct {
	app *App
}

type preparePipelineRunRequest struct {
	RunID          uuid.UUID
	Pipeline       models.Pipeline
	PipelinePath   string
	PipelineSource string
	Scope          string
	TriggerSource  string
	CallerType     string
	CallerID       string
	GitContext     map[string]string
	AuthChecks     []ResourceUseAuthResult
	AuthSnapshot   []byte
	ErrorContext   string
}

type preparedPipelineRun struct {
	Pipeline           models.Pipeline
	PipelineDefinition []byte
	AuthChecks         []ResourceUseAuthResult
	AuthSnapshot       []byte
}

type createPendingRunRequest struct {
	RunID              uuid.UUID
	ParentRunID        string
	ParentStepName     string
	Pipeline           models.Pipeline
	PipelinePath       string
	PipelineDefinition []byte
	Scope              string
	PipelineSource     string
	TriggerSource      string
	CallerType         string
	CallerID           string
	GitContext         map[string]string
	GroupPath          string
	AuthSnapshot       []byte
	NewTriggerEventID  bool
}

type createPendingRunResult struct {
	RunID          uuid.UUID
	TriggerEventID string
}

type runPreparationError struct {
	StatusCode int
	Message    string
}

func newRunService(app *App) *runService {
	return &runService{app: app}
}

func (a *App) updateRunRecordWithFailure(runID uuid.UUID, reason string, gitContext map[string]string) {
	newRunService(a).failRun(context.Background(), runID, reason, gitContext)
}

func (s *runService) createPendingRun(ctx context.Context, req createPendingRunRequest) (createPendingRunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if req.GitContext == nil {
		req.GitContext = map[string]string{}
	}
	if req.RunID == uuid.Nil {
		req.RunID = uuid.New()
	}

	triggerEventID := strings.TrimSpace(req.GitContext["trigger_event_id"])
	if req.NewTriggerEventID {
		triggerEventID = req.RunID.String()
	} else if triggerEventID == "" {
		triggerEventID = deriveTriggerEventID(req.GitContext)
	}
	if triggerEventID == "" {
		triggerEventID = req.RunID.String()
	}
	req.GitContext["trigger_event_id"] = triggerEventID

	groupID, err := s.app.resolveGroupIDForRun(ctx, strings.Trim(strings.TrimSpace(req.GroupPath), "/"), req.PipelinePath, req.GitContext)
	if err != nil {
		repoFullName := repositoryFullName(req.GitContext["repo_owner"], req.GitContext["repo_name"])
		log.Error().Err(err).Str("repo", repoFullName).Str("group_path", req.GroupPath).Msg("Failed to resolve group for run")
	}

	_, err = s.app.db.Exec(ctx,
		`INSERT INTO pipeline_runs (run_id, parent_run_id, pipeline_name, pipeline_path, pipeline_version, status, pipeline_definition,
			git_repo_owner, git_repo_name, git_clone_url, git_ssh_url, git_ref, git_target_ref,
			git_commit_sha, git_commit_url, git_commit_message, git_commit_author_name,
			git_commit_author_email, git_commit_author_username, git_pusher_name,
			git_pusher_email, git_check_run_id, group_id, parent_step_name, trigger_event_id, scope, pipeline_source,
			trigger_source, requested_by_type, requested_by_id, effective_subject_type, effective_subject_id, authorization_snapshot)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32::jsonb)`,
		req.RunID,
		nullString(req.ParentRunID),
		req.Pipeline.Name,
		req.PipelinePath,
		req.Pipeline.Version,
		string(req.PipelineDefinition),
		req.GitContext["repo_owner"],
		req.GitContext["repo_name"],
		req.GitContext["clone_url"],
		req.GitContext["ssh_url"],
		req.GitContext["ref"],
		req.GitContext["target_ref"],
		req.GitContext["commit_sha"],
		req.GitContext["commit_url"],
		req.GitContext["commit_message"],
		req.GitContext["commit_author_name"],
		req.GitContext["commit_author_email"],
		req.GitContext["commit_author_username"],
		req.GitContext["pusher_name"],
		req.GitContext["pusher_email"],
		nullableGitCheckRunID(req.GitContext),
		groupID,
		nullString(req.ParentStepName),
		nullString(triggerEventID),
		req.Scope,
		req.PipelineSource,
		req.TriggerSource,
		req.CallerType,
		req.CallerID,
		req.CallerType,
		req.CallerID,
		string(req.AuthSnapshot),
	)
	if err != nil {
		return createPendingRunResult{}, err
	}

	return createPendingRunResult{
		RunID:          req.RunID,
		TriggerEventID: triggerEventID,
	}, nil
}

func (s *runService) preparePipelineRun(ctx context.Context, req preparePipelineRunRequest) (*preparedPipelineRun, *runPreparationError) {
	resolvedPipeline, err := s.app.resolveStepIncludes(&req.Pipeline)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to resolve step includes%s: %v", req.ErrorContext, err)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusBadRequest, errMsg)
	}

	authChecks := req.AuthChecks
	authSnapshot := req.AuthSnapshot
	resolvedAuthChecks, err := s.app.authorizeRunResourceUses(
		ctx,
		req.CallerType,
		req.CallerID,
		req.TriggerSource,
		req.GitContext,
		req.PipelinePath,
		req.PipelineSource,
		*resolvedPipeline,
		req.Scope,
	)
	if err != nil {
		errMsg := err.Error()
		authChecks = mergeResourceUseAuthResults(authChecks, resolvedAuthChecks)
		s.refreshAuthorizationSnapshot(ctx, req.RunID, req.TriggerSource, req.CallerType, req.CallerID, authChecks, authSnapshot, true)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusForbidden, errMsg)
	}
	authChecks = mergeResourceUseAuthResults(authChecks, resolvedAuthChecks)
	authSnapshot = s.refreshAuthorizationSnapshot(ctx, req.RunID, req.TriggerSource, req.CallerType, req.CallerID, authChecks, authSnapshot, false)

	if err := validatePipeline(resolvedPipeline); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed%s: %v", req.ErrorContext, err)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusBadRequest, errMsg)
	}
	teamID, err := s.app.teamIDForRunProfileOwner(ctx, req.RunID.String())
	if err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed%s: %v", req.ErrorContext, err)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusInternalServerError, errMsg)
	}
	if err := s.app.validatePipelineLLMProfilesForTeam(ctx, resolvedPipeline, req.Scope, teamID); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed%s: %v", req.ErrorContext, err)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusBadRequest, errMsg)
	}
	if err := s.app.validatePipelineAgentProfilesForTeam(ctx, resolvedPipeline, teamID); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed%s: %v", req.ErrorContext, err)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusBadRequest, errMsg)
	}
	if err := s.app.validatePipelineMCPProfilesForTeam(ctx, resolvedPipeline, req.Scope, teamID); err != nil {
		errMsg := fmt.Sprintf("Pipeline validation failed%s: %v", req.ErrorContext, err)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusBadRequest, errMsg)
	}

	_, knowledgeAuthChecks, err := s.app.resolveKnowledgeContextsForRun(ctx, req.RunID, req.CallerType, req.CallerID, req.TriggerSource, req.GitContext, *resolvedPipeline)
	if err != nil {
		errMsg := fmt.Sprintf("Knowledge context resolution failed%s: %v", req.ErrorContext, err)
		authChecks = mergeResourceUseAuthResults(authChecks, knowledgeAuthChecks)
		s.refreshAuthorizationSnapshot(ctx, req.RunID, req.TriggerSource, req.CallerType, req.CallerID, authChecks, authSnapshot, true)
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusForbidden, errMsg)
	}
	authChecks = mergeResourceUseAuthResults(authChecks, knowledgeAuthChecks)
	authSnapshot = s.refreshAuthorizationSnapshot(ctx, req.RunID, req.TriggerSource, req.CallerType, req.CallerID, authChecks, authSnapshot, false)

	resolvedPipelineDef, err := yaml.Marshal(resolvedPipeline)
	if err != nil {
		errMsg := "Failed to marshal resolved pipeline" + req.ErrorContext
		s.failRun(ctx, req.RunID, errMsg, req.GitContext)
		return nil, runPrepError(http.StatusInternalServerError, errMsg)
	}

	return &preparedPipelineRun{
		Pipeline:           *resolvedPipeline,
		PipelineDefinition: resolvedPipelineDef,
		AuthChecks:         authChecks,
		AuthSnapshot:       authSnapshot,
	}, nil
}

func (s *runService) launchPipeline(
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
	ctx := context.Background()
	tx, err := s.app.db.Begin(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start database transaction for tasks")
		s.failRun(ctx, runID, "Failed to start database transaction for tasks", gitContext)
		return
	}
	defer tx.Rollback(ctx)

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if step.GetInclude() != "" {
			_, err := tx.Exec(ctx,
				"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, stepName, stepName, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert 'include' step %s as a task", stepName)
				return
			}
		} else if tasks := step.GetTasks(); len(tasks) > 0 {
			for i, task := range tasks {
				_, err := tx.Exec(ctx,
					"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
					runID, stepName, task.Name, i+1,
				)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to insert task %s for step %s", task.Name, stepName)
					return
				}
			}
		} else {
			_, err := tx.Exec(ctx,
				"INSERT INTO task_runs (task_id, run_id, step_name, task_name, status, task_index) VALUES (gen_random_uuid(), $1, $2, $3, 'pending', $4)",
				runID, stepName, stepName, 1,
			)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to insert step %s as a task", stepName)
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit tasks transaction")
		s.failRun(ctx, runID, "Failed to commit tasks transaction", gitContext)
		return
	}

	go s.app.agentRunLauncher().LaunchAgent(context.Background(), AgentRunLaunchRequest{
		RunID:              runID.String(),
		ParentRunID:        parentRunID,
		ParentRunnerID:     parentRunnerID,
		Pipeline:           pipeline,
		PipelineDefinition: pipelineDef,
		Timeout:            timeoutDuration,
		GitContext:         gitContext,
		ParentHistory:      parentHistory,
		Scope:              scope,
		Overrides:          overrides,
	})
}

func (s *runService) failRun(ctx context.Context, runID uuid.UUID, reason string, gitContext map[string]string) {
	log.Error().Str("run_id", runID.String()).Msg(reason)
	_, err := s.app.db.Exec(ctx,
		"UPDATE pipeline_runs SET status = 'failure', finished_at = NOW(), failure_reason = $1 WHERE run_id = $2",
		reason, runID)
	if err != nil {
		log.Error().Err(err).Str("run_id", runID.String()).Msg("Failed to update run record with failure reason")
	}
	if gitContext["check_run_id"] != "" {
		s.app.notifyGitBotOfFinalStatus("failure", "", "", reason, gitContext)
	}
}

func (s *runService) refreshAuthorizationSnapshot(ctx context.Context, runID uuid.UUID, triggerSource, callerType, callerID string, authChecks []ResourceUseAuthResult, current []byte, force bool) []byte {
	updatedSnapshot, snapshotErr := buildRunAuthorizationSnapshot(triggerSource, callerType, callerID, authChecks)
	if snapshotErr != nil {
		return current
	}
	if force || string(updatedSnapshot) != string(current) {
		_, _ = s.app.db.Exec(ctx, "UPDATE pipeline_runs SET authorization_snapshot = $1::jsonb WHERE run_id = $2", string(updatedSnapshot), runID)
		return updatedSnapshot
	}
	return current
}

func runPrepError(statusCode int, message string) *runPreparationError {
	return &runPreparationError{
		StatusCode: statusCode,
		Message:    message,
	}
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
