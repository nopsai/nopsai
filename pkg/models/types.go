package models

import (
	"time"
)

type LogLine struct {
	ID          int64          `json:"id"`
	Timestamp   time.Time      `json:"timestamp"`
	Line        string         `json:"line"`
	Source      string         `json:"source,omitempty"`
	Stream      string         `json:"stream,omitempty"`
	Level       string         `json:"level,omitempty"`
	StepName    string         `json:"step_name,omitempty"`
	TaskName    string         `json:"task_name,omitempty"`
	RunnerID    string         `json:"runner_id,omitempty"`
	RequestID   string         `json:"request_id,omitempty"`
	Traceparent string         `json:"traceparent,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type RunListItem struct {
	RunID                      string                    `json:"run_id"`
	PipelineName               string                    `json:"pipeline_name"`
	PipelinePath               string                    `json:"pipeline_path"`
	PipelineVersion            string                    `json:"pipeline_version"`
	PipelineSource             string                    `json:"pipeline_source,omitempty"`
	Status                     string                    `json:"status"`
	CreatedAt                  time.Time                 `json:"created_at,omitempty"`
	GitCommitSHA               string                    `json:"git_commit_sha"`
	GitCommitURL               string                    `json:"git_commit_url,omitempty"`
	GitCommitMessage           string                    `json:"git_commit_message,omitempty"`
	GitCommitAuthorName        string                    `json:"git_commit_author_name,omitempty"`
	GitCommitAuthorEmail       string                    `json:"git_commit_author_email,omitempty"`
	GitCommitAuthorUsername    string                    `json:"git_commit_author_username,omitempty"`
	GitRepoName                string                    `json:"git_repo_name"`
	GitRepoOwner               string                    `json:"git_repo_owner"`
	GitCloneURL                string                    `json:"git_clone_url,omitempty"`
	GitSSHURL                  string                    `json:"git_ssh_url,omitempty"`
	GitRef                     string                    `json:"git_ref"`
	GitTargetRef               string                    `json:"git_target_ref"`
	StartedAt                  time.Time                 `json:"started_at"`
	FinishedAt                 time.Time                 `json:"finished_at"`
	TimeoutAt                  time.Time                 `json:"timeout_at,omitempty"`
	Duration                   string                    `json:"duration"`
	IsComplete                 bool                      `json:"is_complete"`
	ParentRunID                *string                   `json:"parent_run_id"`
	TriggerEventID             string                    `json:"trigger_event_id,omitempty"`
	TriggerSource              string                    `json:"trigger_source,omitempty"`
	Scope                      string                    `json:"scope,omitempty"`
	TeamID                     int                       `json:"team_id,omitempty"`
	RequestedByType            string                    `json:"requested_by_type,omitempty"`
	RequestedByID              string                    `json:"requested_by_id,omitempty"`
	EffectiveSubjectType       string                    `json:"effective_subject_type,omitempty"`
	EffectiveSubjectID         string                    `json:"effective_subject_id,omitempty"`
	RuntimeVariableOverrides   map[string]any            `json:"runtime_variable_overrides,omitempty"`
	ExternalTriggerID          string                    `json:"external_trigger_id,omitempty"`
	ExternalTriggerName        string                    `json:"external_trigger_name,omitempty"`
	ExternalTriggerEventType   string                    `json:"external_trigger_event_type,omitempty"`
	ExternalTriggerCallerType  string                    `json:"external_trigger_caller_type,omitempty"`
	ExternalTriggerCallerID    string                    `json:"external_trigger_caller_id,omitempty"`
	ExternalTriggerIdempotency string                    `json:"external_trigger_idempotency_key,omitempty"`
	ScheduleID                 string                    `json:"schedule_id,omitempty"`
	ScheduleName               string                    `json:"schedule_name,omitempty"`
	SchedulePath               string                    `json:"schedule_path,omitempty"`
	GitPusherName              string                    `json:"git_pusher_name"`
	GitPusherEmail             string                    `json:"git_pusher_email,omitempty"`
	GitCheckRunID              int64                     `json:"git_check_run_id,omitempty"`
	ParentStepName             string                    `json:"parent_step_name,omitempty"`
	FailureReason              string                    `json:"failure_reason,omitempty"`
	AIUsage                    AIUsageSummary            `json:"ai_usage"`
	FinalOutputStatus          *FinalOutputStatusSummary `json:"final_output_status,omitempty"`
}

type AIUsageSummary struct {
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}

type FinalOutputStatusSummary struct {
	Status     string     `json:"status"`
	Configured int        `json:"configured"`
	Total      int        `json:"total"`
	Pending    int        `json:"pending"`
	Generating int        `json:"generating"`
	Generated  int        `json:"generated"`
	Failed     int        `json:"failed"`
	Cancelled  int        `json:"cancelled"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

type StepConfiguration struct {
	Image            string                `json:"image,omitempty"`
	Include          string                `json:"include,omitempty"`
	Sync             bool                  `json:"sync"`
	Approval         ApprovalDefinition    `json:"approval,omitempty"`
	Secrets          []string              `json:"secrets,omitempty"`
	Volumes          []string              `json:"volumes,omitempty"`
	Variables        map[string]string     `json:"variables,omitempty"`
	Outputs          []TaskOutput          `json:"outputs,omitempty"`
	IgnoreFailure    bool                  `json:"ignore_failure"`
	LlmOutputSharing *bool                 `json:"llm_output_sharing,omitempty"`
	AgentProfile     string                `json:"agent_profile,omitempty"`
	LLMProfile       string                `json:"llm_profile,omitempty"`
	MCPProfiles      []string              `json:"mcp_profiles,omitempty"`
	RuntimePool      string                `json:"runtime_pool,omitempty"`
	KnowledgeContext []KnowledgeContextRef `json:"knowledge_context,omitempty"`
	Tasks            []Task                `json:"tasks,omitempty"`
}

type StepDetail struct {
	Name          string            `json:"name"`
	Status        string            `json:"status"`
	DependsOn     []string          `json:"depends_on"`
	Tasks         []TaskDetail      `json:"tasks"`
	Duration      string            `json:"duration"`
	Configuration StepConfiguration `json:"configuration"`
	AIUsage       AIUsageSummary    `json:"ai_usage"`
}

type TaskDetail struct {
	TaskID     string              `json:"task_id"`
	StepName   string              `json:"step_name"`
	TaskName   string              `json:"task_name"`
	Status     string              `json:"status"`
	ExitCode   *int                `json:"exit_code"`
	StartedAt  time.Time           `json:"started_at"`
	FinishedAt time.Time           `json:"finished_at"`
	TaskIndex  int                 `json:"task_index"`
	Outputs    []TaskRuntimeOutput `json:"outputs,omitempty"`
	AIUsage    AIUsageSummary      `json:"ai_usage"`
}

type TaskRuntimeOutput struct {
	StepName  string `json:"step_name,omitempty"`
	TaskName  string `json:"task_name,omitempty"`
	Name      string `json:"name"`
	Sensitive bool   `json:"sensitive,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type ParentRunInfo struct {
	RunID           string `json:"run_id"`
	PipelineName    string `json:"pipeline_name"`
	PipelinePath    string `json:"pipeline_path"`
	PipelineVersion string `json:"pipeline_version"`
}

type PipelineRunFinalOutput struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Type                string                 `json:"type"`
	Status              string                 `json:"status"`
	Content             string                 `json:"content,omitempty"`
	Error               string                 `json:"error,omitempty"`
	LLMProfile          string                 `json:"llm_profile,omitempty"`
	DashboardTarget     *DashboardOutputTarget `json:"dashboard_target,omitempty"`
	GenerationAttempts  int                    `json:"generation_attempts,omitempty"`
	ContractViolations  int                    `json:"contract_violations,omitempty"`
	RenderAttempts      int                    `json:"render_attempts,omitempty"`
	RenderFailures      int                    `json:"render_failures,omitempty"`
	CreatedAt           time.Time              `json:"created_at,omitempty"`
	GenerationStartedAt *time.Time             `json:"generation_started_at,omitempty"`
	UpdatedAt           time.Time              `json:"updated_at,omitempty"`
	GenerationDuration  string                 `json:"generation_duration,omitempty"`
	GenerationSeconds   float64                `json:"generation_duration_seconds,omitempty"`
}

type RunDetail struct {
	RunInfo                RunListItem                `json:"run_info"`
	Steps                  []StepDetail               `json:"steps"`
	PipelineDefinition     Pipeline                   `json:"pipeline_definition"`
	PipelineDefinitionYAML string                     `json:"pipeline_definition_yaml"`
	KnowledgeContexts      []KnowledgeContextSnapshot `json:"knowledge_contexts,omitempty"`
	FinalOutputs           []PipelineRunFinalOutput   `json:"final_outputs,omitempty"`
	ChildRuns              []RunListItem              `json:"child_runs"`
	ParentRunInfo          *ParentRunInfo             `json:"parent_run_info,omitempty"`
}

type StepStatusUpdate struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

type AIUsageReport struct {
	StepName         string         `json:"step_name,omitempty"`
	TaskName         string         `json:"task_name,omitempty"`
	Feature          string         `json:"feature,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	LLMProfile       string         `json:"llm_profile,omitempty"`
	PromptTokens     int64          `json:"prompt_tokens,omitempty"`
	CompletionTokens int64          `json:"completion_tokens,omitempty"`
	TotalTokens      int64          `json:"total_tokens,omitempty"`
	InputCostUSD     float64        `json:"input_cost_usd,omitempty"`
	OutputCostUSD    float64        `json:"output_cost_usd,omitempty"`
	TotalCostUSD     float64        `json:"total_cost_usd,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type PolicyRevisionResponse struct {
	RunStartPolicyRevision string                     `json:"run_start_policy_revision"`
	CurrentPolicyRevision  string                     `json:"current_policy_revision"`
	BlockingContextCount   int                        `json:"blocking_context_count"`
	KnowledgeContexts      []KnowledgeContextSnapshot `json:"knowledge_contexts,omitempty"`
	CheckedAt              time.Time                  `json:"checked_at,omitempty"`
}

type SecretRequest struct {
	Value string `json:"value"`
}

type VariableRequest struct {
	Value string `json:"value"`
}

type ScopeResponse struct {
	Scope string `json:"scope"`
}

type VariableValueResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type PipelineRequest struct {
	Definition string `json:"definition"`
}

type TriggerOverrideRequest struct {
	TriggerDefinition string `json:"trigger_definition"`
}

type FinalizeRequest struct {
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type Team struct {
	ID                 int        `json:"id"`
	Name               string     `json:"name"`
	Kind               string     `json:"kind,omitempty"`
	ParentID           *int       `json:"parent_id"`
	Description        string     `json:"description,omitempty"`
	RepoURL            string     `json:"repo_url,omitempty"`
	RepositoryFullName string     `json:"repository_full_name,omitempty"`
	Children           []Team     `json:"children"`
	LastRunAt          *time.Time `json:"last_run_at,omitempty"`
	NavigationOnly     bool       `json:"navigation_only,omitempty"`
}
