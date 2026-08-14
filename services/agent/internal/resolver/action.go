package resolver

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	workspacectx "nopsai/services/agent/internal/workspace"

	"github.com/rs/zerolog"
)

type ActionSession interface {
	ProfileName() string
	AgentProfileName() string
	MCPEnabled() bool
	MCPProfiles() []string
	MCPToolCount() int
	RequiresMCPToolCall() bool
	SuccessfulMCPToolCalls() int
	ReviewPolicy(context.Context, PolicyReviewRequest) (*models.PolicyReview, error)
	GetAction(context.Context, *proto.GetActionRequest, *workspacectx.Tools) (*proto.Action, error)
}

type ActionSessionResolver func(*models.Pipeline, *models.PipelineStep, *models.Task) (ActionSession, error)
type DirectoryLister func(*zerolog.Logger, string, []string, []string) map[string]string

type PolicyReviewRequest struct {
	Phase            string
	Goal             string
	History          string
	Variables        map[string]string
	KnowledgeContext string
	ProposedAction   *proto.Action
}

type ActionSessionResolutionStage string

const (
	ActionSessionResolutionLLMProfile   ActionSessionResolutionStage = "model"
	ActionSessionResolutionAgentProfile ActionSessionResolutionStage = "agent_role"
	ActionSessionResolutionMCPProfile   ActionSessionResolutionStage = "mcp_profile"
)

type ActionSessionResolutionError struct {
	Stage ActionSessionResolutionStage
	Err   error
}

func (e ActionSessionResolutionError) Error() string {
	if e.Err == nil {
		return "action session resolution failed"
	}
	return e.Err.Error()
}

func (e ActionSessionResolutionError) Unwrap() error {
	return e.Err
}

func NewActionSessionResolutionError(stage ActionSessionResolutionStage, err error) error {
	return ActionSessionResolutionError{Stage: stage, Err: err}
}

type ActionRequest struct {
	Logger                 *zerolog.Logger
	Pipeline               *models.Pipeline
	Step                   *models.PipelineStep
	Task                   *models.Task
	Context                ExecutionContext
	History                string
	ParentContext          context.Context
	WorkspaceDir           string
	WorkspaceRevision      uint64
	WorkspaceIndex         *workspacectx.Index
	IsRunStopping          func() bool
	Secrets                map[string]string
	KnowledgePrompt        string
	BlockingKnowledgeKinds []string
	LLMTimeout             time.Duration
	LLMEnabled             bool
	SessionResolver        ActionSessionResolver
	DirectoryLister        DirectoryLister
	StopRetry              func(error) bool
}

type ActionResult struct {
	Action           *proto.Action
	ActionSummary    string
	Goal             string
	FilePrecondition FilePrecondition
	LLMDurationMs    int64
	LLMDurationSet   bool
	Failed           bool
	// FailClosed marks policy/guardrail enforcement failures that must stop the run even when ignore_failure is set.
	FailClosed       bool
	FinalizeStatus   string
	FinalizeExitCode int
}

type actionResolver interface {
	Resolve(context.Context, ActionRequest) ActionResult
}

type TaskActionResolver struct {
	script actionResolver
	llm    actionResolver
}

func NewTaskActionResolver() TaskActionResolver {
	return TaskActionResolver{
		script: scriptActionResolver{},
		llm:    llmBackedActionResolver{},
	}
}

func (r TaskActionResolver) Resolve(ctx context.Context, req ActionRequest) ActionResult {
	if req.Task != nil && strings.TrimSpace(req.Task.Script) != "" {
		return r.script.Resolve(ctx, req)
	}
	return r.llm.Resolve(ctx, req)
}

type scriptActionResolver struct{}

func (scriptActionResolver) Resolve(ctx context.Context, req ActionRequest) ActionResult {
	goal := taskGoal(req.Step, req.Task)
	if req.Logger != nil {
		if goal != "" {
			req.Logger.Info().Msgf("Executing direct script for goal: %s", goal)
		} else {
			req.Logger.Info().Msg("Executing direct script")
		}
	}
	script := ""
	if req.Task != nil {
		script = req.Task.Script
	}
	if len(req.BlockingKnowledgeKinds) > 0 {
		return validateDirectScriptAction(ctx, req, goal, script)
	}
	return ActionResult{
		Action: &proto.Action{
			Type:    "EXECUTE_COMMAND",
			Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: script}},
		},
		ActionSummary: script,
		Goal:          goal,
	}
}

func validateDirectScriptAction(ctx context.Context, req ActionRequest, goal, script string) ActionResult {
	if !req.LLMEnabled || req.SessionResolver == nil {
		interpretation := models.InterpretPolicyReview(models.EffectiveGovernanceLevel(req.Pipeline, req.Step, req.Task), nil, false)
		if interpretation.Allowed {
			if req.Logger != nil {
				req.Logger.Warn().
					Str("governance_level", models.EffectiveGovernanceLevel(req.Pipeline, req.Step, req.Task)).
					Strs("knowledge_context_kinds", req.BlockingKnowledgeKinds).
					Msg("Cannot validate direct script against blocking knowledge context because LLM is disabled; governance allows continuing with warning")
			}
			return ActionResult{
				Action:        commandAction(script),
				ActionSummary: script,
				Goal:          goal,
			}
		}
		if req.Logger != nil {
			req.Logger.Error().
				Strs("knowledge_context_kinds", req.BlockingKnowledgeKinds).
				Msg("Cannot validate direct script against blocking knowledge context because LLM is disabled")
		}
		return ActionResult{
			Goal:             goal,
			Failed:           true,
			FailClosed:       true,
			FinalizeStatus:   "failure",
			FinalizeExitCode: 1,
		}
	}

	session, sessionErr := req.SessionResolver(req.Pipeline, req.Step, req.Task)
	if sessionErr != nil {
		logActionSessionResolutionError(req.Logger, sessionErr)
		return ActionResult{Goal: goal, Failed: true, FailClosed: true, FinalizeStatus: "failure", FinalizeExitCode: 1}
	}
	if session == nil {
		if req.Logger != nil {
			req.Logger.Error().Msg("Failed to resolve action session for direct script validation")
		}
		return ActionResult{Goal: goal, Failed: true, FailClosed: true, FinalizeStatus: "failure", FinalizeExitCode: 1}
	}
	if req.Logger != nil {
		req.Logger.Info().
			Str("model", session.ProfileName()).
			Strs("knowledge_context_kinds", req.BlockingKnowledgeKinds).
			Msg("Validating direct script against blocking knowledge context")
	}

	parentCtx := req.ParentContext
	if parentCtx == nil {
		parentCtx = ctx
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	llmTimeout := req.LLMTimeout
	if llmTimeout <= 0 {
		llmTimeout = 2 * time.Minute
	}

	var llmDurationMs int64
	proposedAction := commandAction(script)
	if duration, failure, failed := runActionPolicyReview(parentCtx, req, session, PolicyReviewPhaseBefore, directScriptReviewGoal(goal, script), proposedAction); failed {
		failure.LLMDurationMs = duration
		failure.LLMDurationSet = true
		return failure
	} else {
		llmDurationMs += duration
	}

	actionReq := req.Context.BuildActionRequest(
		directScriptValidationGoal(goal, script),
		req.History,
		map[string]string{},
		req.KnowledgePrompt,
		req.Secrets,
	)

	var validationAction *proto.Action
	actionStart := time.Now()
	err := withRetry(func() error {
		attemptCtx, cancel := context.WithTimeout(parentCtx, llmTimeout)
		defer cancel()
		var callErr error
		validationAction, callErr = session.GetAction(attemptCtx, actionReq, nil)
		return callErr
	}, 3, time.Second, req.StopRetry)
	llmDurationMs += time.Since(actionStart).Milliseconds()
	if err != nil {
		interpretation := models.InterpretPolicyReview(models.EffectiveGovernanceLevel(req.Pipeline, req.Step, req.Task), nil, false)
		if interpretation.Allowed {
			if req.Logger != nil {
				req.Logger.Warn().Err(err).Msg("Direct script guardrail validation was unavailable; governance allows continuing with warning")
			}
		} else {
			if req.Logger != nil {
				req.Logger.Error().Err(err).Msg("Direct script guardrail validation failed")
			}
			return ActionResult{
				Goal:             goal,
				LLMDurationMs:    llmDurationMs,
				LLMDurationSet:   true,
				Failed:           true,
				FailClosed:       interpretation.FailClosed,
				FinalizeStatus:   "failure",
				FinalizeExitCode: 1,
			}
		}
	} else if answer := validationAction.GetAnswerAction(); answer != nil {
		reason := strings.TrimSpace(answer.Answer)
		if reason == "" {
			reason = "Direct script blocked by guardrail or policy."
		}
		review := &models.PolicyReview{Decision: models.PolicyDecisionBlock, Reason: reason, Refs: req.BlockingKnowledgeKinds}
		interpretation := models.InterpretPolicyReview(models.EffectiveGovernanceLevel(req.Pipeline, req.Step, req.Task), review, false)
		if interpretation.Allowed {
			if req.Logger != nil {
				req.Logger.Warn().
					Strs("knowledge_context_kinds", req.BlockingKnowledgeKinds).
					Msgf("Knowledge context warned on direct script: %s", req.Context.MaskText(reason, req.Secrets))
			}
		} else {
			if req.Logger != nil {
				req.Logger.Error().
					Strs("knowledge_context_kinds", req.BlockingKnowledgeKinds).
					Msgf("Knowledge context blocked direct script: %s", req.Context.MaskText(reason, req.Secrets))
			}
			return ActionResult{
				Goal:             goal,
				LLMDurationMs:    llmDurationMs,
				LLMDurationSet:   true,
				Failed:           true,
				FailClosed:       interpretation.FailClosed,
				FinalizeStatus:   "failure",
				FinalizeExitCode: 1,
			}
		}
	} else {
		allowedCommand := ""
		if cmd := validationAction.GetCommandAction(); cmd != nil {
			allowedCommand = cmd.Command
		}
		if strings.TrimSpace(allowedCommand) != strings.TrimSpace(script) {
			interpretation := models.InterpretPolicyReview(models.EffectiveGovernanceLevel(req.Pipeline, req.Step, req.Task), nil, false)
			if interpretation.Allowed {
				if req.Logger != nil {
					req.Logger.Warn().
						Str("validation_action", actionSummary(validationAction)).
						Msg("Direct script guardrail validation did not return the exact script command; governance allows continuing with warning")
				}
			} else {
				if req.Logger != nil {
					req.Logger.Error().
						Str("validation_action", actionSummary(validationAction)).
						Msg("Direct script guardrail validation did not return the exact script command")
				}
				return ActionResult{
					Goal:             goal,
					LLMDurationMs:    llmDurationMs,
					LLMDurationSet:   true,
					Failed:           true,
					FailClosed:       interpretation.FailClosed,
					FinalizeStatus:   "failure",
					FinalizeExitCode: 1,
				}
			}
		}
	}

	if duration, failure, failed := runActionPolicyReview(parentCtx, req, session, PolicyReviewPhaseAfter, goal, proposedAction); failed {
		failure.LLMDurationMs = llmDurationMs + duration
		failure.LLMDurationSet = true
		return failure
	} else {
		llmDurationMs += duration
	}

	return ActionResult{
		Action:         proposedAction,
		ActionSummary:  script,
		Goal:           goal,
		LLMDurationMs:  llmDurationMs,
		LLMDurationSet: true,
	}
}

const (
	PolicyReviewPhaseBefore = "before"
	PolicyReviewPhaseAfter  = "after"
)

func runActionPolicyReview(ctx context.Context, req ActionRequest, session ActionSession, phase, goal string, proposedAction *proto.Action) (int64, ActionResult, bool) {
	if len(req.BlockingKnowledgeKinds) == 0 || session == nil {
		return 0, ActionResult{}, false
	}
	start := time.Now()
	var review *models.PolicyReview
	err := withRetry(func() error {
		attemptCtx, cancel := context.WithTimeout(ctx, effectiveLLMTimeout(req.LLMTimeout))
		defer cancel()
		var callErr error
		review, callErr = session.ReviewPolicy(attemptCtx, PolicyReviewRequest{
			Phase:            phase,
			Goal:             goal,
			History:          req.History,
			Variables:        req.Context.PromptVariables(),
			KnowledgeContext: req.KnowledgePrompt,
			ProposedAction:   proposedAction,
		})
		return callErr
	}, 3, time.Second, req.StopRetry)
	durationMs := time.Since(start).Milliseconds()

	level := models.EffectiveGovernanceLevel(req.Pipeline, req.Step, req.Task)
	interpretation := models.InterpretPolicyReview(level, review, false)
	if err != nil {
		interpretation = models.InterpretPolicyReview(level, nil, false)
		if req.Logger != nil {
			req.Logger.Warn().
				Err(err).
				Str("governance_level", level).
				Str("policy_review_phase", strings.TrimSpace(phase)).
				Str("policy_decision", interpretation.Decision).
				Msg("AI policy review was unavailable")
		}
	} else {
		logActionPolicyReview(req.Logger, level, phase, review, interpretation)
	}

	if interpretation.Allowed {
		return durationMs, ActionResult{}, false
	}
	reason := strings.TrimSpace(interpretation.Reason)
	if err != nil {
		reason = fmt.Sprintf("AI policy review was unavailable: %v", err)
	} else if reason == "" {
		reason = "policy review did not allow the action"
	}
	if req.Logger != nil {
		req.Logger.Error().
			Str("governance_level", level).
			Str("policy_review_phase", strings.TrimSpace(phase)).
			Str("policy_decision", interpretation.Decision).
			Msgf("Governance blocked action: %s", req.Context.MaskText(reason, req.Secrets))
	}
	return durationMs, ActionResult{
		Goal:             taskGoal(req.Step, req.Task),
		Failed:           true,
		FailClosed:       interpretation.FailClosed,
		FinalizeStatus:   "failure",
		FinalizeExitCode: 1,
	}, true
}

func logActionPolicyReview(logger *zerolog.Logger, level, phase string, review *models.PolicyReview, interpretation models.GovernanceInterpretation) {
	if logger == nil {
		return
	}
	evt := logger.Info().
		Str("governance_level", level).
		Str("policy_review_phase", strings.TrimSpace(phase)).
		Str("policy_decision", interpretation.Decision)
	if review != nil {
		if confidence := strings.TrimSpace(review.Confidence); confidence != "" {
			evt = evt.Str("policy_confidence", confidence)
		}
		if len(review.Refs) > 0 {
			evt = evt.Strs("policy_refs", review.Refs)
		}
	}
	switch {
	case !interpretation.Allowed:
		evt.Msg("AI policy review blocked action")
	case interpretation.Warning:
		evt.Msg("AI policy review allowed action with warning")
	default:
		evt.Msg("AI policy review allowed action")
	}
}

func effectiveLLMTimeout(value time.Duration) time.Duration {
	if value <= 0 {
		return 2 * time.Minute
	}
	return value
}

func commandAction(command string) *proto.Action {
	return &proto.Action{
		Type:    "EXECUTE_COMMAND",
		Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: command}},
	}
}

func directScriptReviewGoal(goal, script string) string {
	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal == "" {
		trimmedGoal = "Execute a direct script task."
	}
	return fmt.Sprintf("%s\n\nDirect script proposed for execution:\n%s", trimmedGoal, script)
}

func directScriptValidationGoal(goal, script string) string {
	trimmedGoal := strings.TrimSpace(goal)
	if trimmedGoal == "" {
		trimmedGoal = "Execute a direct script task."
	}
	return fmt.Sprintf(
		"Validate this direct script before execution. The script is the exact proposed EXECUTE_COMMAND for the current task goal: %s\n\nInspect the script against all guardrails and policies in the knowledge context. If it complies, return EXECUTE_COMMAND with exactly this command text and no changes. If it conflicts with any guardrail or policy, return RETURN_ANSWER with a short explanation naming the conflicting guardrail or policy.\n\nScript:\n%s",
		trimmedGoal,
		script,
	)
}

type llmBackedActionResolver struct{}

func (llmBackedActionResolver) Resolve(ctx context.Context, req ActionRequest) ActionResult {
	goal := taskGoal(req.Step, req.Task)
	if !req.LLMEnabled || req.SessionResolver == nil {
		if req.Logger != nil {
			req.Logger.Error().Msg("Cannot resolve goal because LLM is disabled for this pipeline")
		}
		return ActionResult{
			Goal:             goal,
			Failed:           true,
			FinalizeStatus:   "failure",
			FinalizeExitCode: 1,
		}
	}

	if req.Logger != nil {
		if goal != "" {
			req.Logger.Info().Msgf("Resolving goal with LLM: %s", goal)
		} else {
			req.Logger.Info().Msg("Resolving goal with LLM")
		}
	}

	workspaceRevision := effectiveWorkspaceRevision(req.WorkspaceRevision)
	var sharedDirectoryListing map[string]string
	var sharedFileIdentities map[string]SharedFileIdentity
	if models.PipelineLLMContentSharing(req.Pipeline) && req.WorkspaceIndex != nil {
		sharedDirectoryListing, sharedFileIdentities = buildSharedDirectoryContextFromWorkspaceIndex(req.WorkspaceIndex)
		logDirectoryListingMetadata(req.Logger, sharedDirectoryListing)
	} else {
		directoryListing := collectActionDirectoryListing(req)
		sharedDirectoryListing, sharedFileIdentities = buildSharedDirectoryContext(directoryListing, workspaceRevision)
	}
	actionReq := req.Context.BuildActionRequest(goal, req.History, sharedDirectoryListing, req.KnowledgePrompt, req.Secrets)
	workspaceTools := workspacectx.NewTools(req.WorkspaceIndex)

	session, sessionErr := req.SessionResolver(req.Pipeline, req.Step, req.Task)
	if sessionErr != nil {
		logActionSessionResolutionError(req.Logger, sessionErr)
		return ActionResult{Goal: goal, Failed: true}
	}
	if session == nil {
		if req.Logger != nil {
			req.Logger.Error().Msg("Failed to resolve action session for goal")
		}
		return ActionResult{Goal: goal, Failed: true}
	}
	if req.Logger != nil {
		req.Logger.Info().Str("model", session.ProfileName()).Msg("Using LLM profile for goal")
		if agentProfile := strings.TrimSpace(session.AgentProfileName()); agentProfile != "" {
			req.Logger.Info().Str("agent_role", agentProfile).Msg("Using agent profile for goal")
		}
	}
	logMCPSession(req.Logger, session)

	actionParentCtx := req.ParentContext
	if actionParentCtx == nil {
		actionParentCtx = ctx
	}
	if actionParentCtx == nil {
		actionParentCtx = context.Background()
	}
	llmTimeout := req.LLMTimeout
	if llmTimeout <= 0 {
		llmTimeout = 2 * time.Minute
	}

	var llmDurationMs int64
	if duration, failure, failed := runActionPolicyReview(actionParentCtx, req, session, PolicyReviewPhaseBefore, goal, nil); failed {
		failure.LLMDurationMs = duration
		failure.LLMDurationSet = true
		return failure
	} else {
		llmDurationMs += duration
	}

	var action *proto.Action
	actionStart := time.Now()
	err := withRetry(func() error {
		attemptCtx, cancel := context.WithTimeout(actionParentCtx, llmTimeout)
		defer cancel()
		var callErr error
		action, callErr = session.GetAction(attemptCtx, actionReq, workspaceTools)
		if callErr != nil {
			return callErr
		}
		if session.RequiresMCPToolCall() && session.SuccessfulMCPToolCalls() == 0 {
			action = nil
			return fmt.Errorf("MCP tool call is required before executing a final action")
		}
		return nil
	}, 3, time.Second, req.StopRetry)
	llmDurationMs += time.Since(actionStart).Milliseconds()

	if err != nil {
		if req.runStopping() {
			if req.Logger != nil {
				req.Logger.Warn().Err(err).Msg("Goal resolution stopped because the run is cancelling")
			}
			return ActionResult{Goal: goal, LLMDurationMs: llmDurationMs, LLMDurationSet: true, Failed: true}
		}
		if req.StopRetry != nil && req.StopRetry(err) {
			failureReason := req.Context.MaskText(err.Error(), req.Secrets)
			if req.Logger != nil {
				req.Logger.Error().Err(err).Msgf("Goal resolution failed: %s", failureReason)
				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					req.Logger.Info().Msgf(`status=failure action="Resolve goal" output="%s"`, failureReason)
				}
			}
			return ActionResult{
				Goal:             goal,
				LLMDurationMs:    llmDurationMs,
				LLMDurationSet:   true,
				Failed:           true,
				FinalizeStatus:   "failure",
				FinalizeExitCode: 1,
			}
		}

		if req.Logger != nil {
			req.Logger.Warn().Err(err).Msg("GetAction failed after retries; attempting one final retry")
		}
		retryStart := time.Now()
		attemptCtx, cancel := context.WithTimeout(actionParentCtx, llmTimeout)
		action, err = session.GetAction(attemptCtx, actionReq, workspaceTools)
		cancel()
		if err == nil && session.RequiresMCPToolCall() && session.SuccessfulMCPToolCalls() == 0 {
			action = nil
			err = fmt.Errorf("MCP tool call is required before executing a final action")
		}
		llmDurationMs += time.Since(retryStart).Milliseconds()
		if err != nil {
			if req.Logger != nil {
				req.Logger.Error().Err(err).Msg("Failed to get action from LLM. Shutting down")
			}
			return ActionResult{Goal: goal, LLMDurationMs: llmDurationMs, LLMDurationSet: true, Failed: true}
		}
	}

	if req.runStopping() {
		if req.Logger != nil {
			req.Logger.Warn().Msg("Goal resolution finished after the run was cancelled; skipping action execution")
		}
		return ActionResult{Goal: goal, LLMDurationMs: llmDurationMs, LLMDurationSet: true, Failed: true}
	}
	if session.RequiresMCPToolCall() && req.Logger != nil {
		req.Logger.Info().
			Int("mcp_successful_tool_calls", session.SuccessfulMCPToolCalls()).
			Msgf("MCP tool calls completed before final action (count=%d)", session.SuccessfulMCPToolCalls())
	}

	if duration, failure, failed := runActionPolicyReview(actionParentCtx, req, session, PolicyReviewPhaseAfter, goal, action); failed {
		failure.LLMDurationMs = llmDurationMs + duration
		failure.LLMDurationSet = true
		return failure
	} else {
		llmDurationMs += duration
	}

	filePrecondition := replaceFilePrecondition(action, sharedFileIdentities, workspaceRevision)
	if filePrecondition.ExpectedSHA256 == "" && workspaceTools != nil {
		if identity, ok := workspaceTools.IdentityFor(filePrecondition.Path); ok {
			filePrecondition.ExpectedSHA256 = identity.SHA256
			filePrecondition.Size = int(identity.Size)
			filePrecondition.WorkspaceRevision = identity.WorkspaceRevision
		}
	}

	return ActionResult{
		Action:           action,
		ActionSummary:    actionSummary(action),
		Goal:             goal,
		FilePrecondition: filePrecondition,
		LLMDurationMs:    llmDurationMs,
		LLMDurationSet:   true,
	}
}

func effectiveWorkspaceRevision(revision uint64) uint64 {
	if revision == 0 {
		return 1
	}
	return revision
}

func collectActionDirectoryListing(req ActionRequest) map[string]string {
	if !models.PipelineLLMContentSharing(req.Pipeline) {
		if req.Logger != nil {
			req.Logger.Debug().Msg("Content sharing is DISABLED for this pipeline. Skipping directory scan")
		}
		return map[string]string{}
	}

	if req.Logger != nil {
		req.Logger.Debug().Msg("Content sharing is ENABLED for this pipeline. Scanning directory")
	}
	lister := req.DirectoryLister
	if lister == nil {
		return map[string]string{}
	}
	logger := req.Logger
	if logger == nil {
		nopLogger := zerolog.Nop()
		logger = &nopLogger
	}
	var include, ignore []string
	if req.Pipeline != nil {
		include = req.Pipeline.LlmContentInclude
		ignore = req.Pipeline.LlmContentIgnore
	}
	directoryListing := lister(logger, strings.TrimSpace(req.WorkspaceDir), include, ignore)
	logDirectoryListingMetadata(req.Logger, directoryListing)
	return directoryListing
}

func logDirectoryListingMetadata(logger *zerolog.Logger, directoryListing map[string]string) {
	if logger == nil {
		return
	}
	if len(directoryListing) == 0 {
		logger.Debug().Msg("Sharing directory listing metadata with LLM (empty)")
		return
	}
	fileNames := make([]string, 0, len(directoryListing))
	for name := range directoryListing {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)
	maxLoggedFiles := 5
	if len(fileNames) < maxLoggedFiles {
		maxLoggedFiles = len(fileNames)
	}
	evt := logger.Debug().
		Int("directory_file_count", len(directoryListing)).
		Strs("directory_file_sample", fileNames[:maxLoggedFiles])
	if len(fileNames) > maxLoggedFiles {
		evt = evt.Int("directory_file_remaining", len(fileNames)-maxLoggedFiles)
	}
	evt.Msg("Sharing directory listing metadata with LLM")
}

func logMCPSession(logger *zerolog.Logger, session ActionSession) {
	if logger == nil || session == nil || !session.MCPEnabled() {
		return
	}
	mcpProfiles := session.MCPProfiles()
	logger.Info().
		Strs("mcp_profiles", mcpProfiles).
		Int("mcp_tool_count", session.MCPToolCount()).
		Bool("mcp_requires_tool_call", session.RequiresMCPToolCall()).
		Msgf("Using MCP profiles for goal (profiles=%s tools=%d require_tool_call=%t)", strings.Join(mcpProfiles, ","), session.MCPToolCount(), session.RequiresMCPToolCall())
}

func taskGoal(step *models.PipelineStep, task *models.Task) string {
	if task != nil && strings.TrimSpace(task.Goal) != "" {
		return strings.TrimSpace(task.Goal)
	}
	if step == nil {
		return ""
	}
	return strings.TrimSpace(step.GetGoal())
}

func actionSummary(action *proto.Action) string {
	if action == nil {
		return ""
	}
	if cmd := action.GetCommandAction(); cmd != nil {
		return cmd.Command
	}
	if file := action.GetFileAction(); file != nil {
		return fmt.Sprintf("Write to %s", file.Path)
	}
	if ans := action.GetAnswerAction(); ans != nil {
		return "Return answer"
	}
	return ""
}

func logActionSessionResolutionError(logger *zerolog.Logger, err error) {
	if logger == nil {
		return
	}
	var resolutionErr ActionSessionResolutionError
	if !errors.As(err, &resolutionErr) {
		logger.Error().Err(err).Msg("Failed to resolve action session for goal")
		return
	}
	switch resolutionErr.Stage {
	case ActionSessionResolutionLLMProfile:
		logger.Error().Err(err).Msg("Failed to resolve LLM profile for goal")
	case ActionSessionResolutionAgentProfile:
		logger.Error().Err(err).Msg("Failed to resolve agent profile for goal")
	case ActionSessionResolutionMCPProfile:
		logger.Error().Err(err).Msg("Failed to resolve MCP profiles for goal")
	default:
		logger.Error().Err(err).Msg("Failed to resolve action session for goal")
	}
}

func (req ActionRequest) runStopping() bool {
	if req.IsRunStopping == nil {
		return false
	}
	return req.IsRunStopping()
}
