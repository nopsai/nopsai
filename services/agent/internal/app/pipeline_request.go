package app

import (
	"time"

	"nopsai/pkg/models"
	"nopsai/services/agent/internal/resolver"
)

type ApprovalResumeSnapshot struct {
	ExecutionHistory string
	CompletedTasks   []string
}

type PipelineRunRequest struct {
	RunID                  string
	PipelineName           string
	PipelineDefinitionYAML []byte
	ParentHistoryBase64    string
	PipelineTimeout        string
	SharedVolumeName       string
	WorkspaceDir           string
	WorkingDirectory       string
	RunnerID               string
	Pipeline               models.Pipeline
	Variables              map[string]string
	Secrets                map[string]string
	ResumeCheckpoint       *ApprovalResumeSnapshot
	KnowledgeSnapshots     []models.KnowledgeContextSnapshot
	PipelineLLMEnabled     bool
	LLMTimeout             time.Duration
	RuntimeOutputMaxBytes  int64

	StepRuntime             StepRuntime
	ConditionClientResolver resolver.ConditionClientResolver
	ActionSessionResolver   resolver.ActionSessionResolver
	ApprovalPauser          ApprovalPauser
	IncludeRunner           IncludeRunner
	DirectoryLister         resolver.DirectoryLister
	StopRetry               func(error) bool

	Logger                 AgentLogger
	StepLogger             StepLogger
	UpdateTaskStatus       TaskStatusReporter
	ReportTaskOutputs      TaskOutputReporter
	NotifyFinalStatus      FinalStatusNotifier
	WatchRunCancellation   RunCancellationWatcher
	Env                    EnvLookup
	Environment            EnvironmentProvider
	Exit                   ExitFunc
	KnowledgePrompt        KnowledgePromptBuilder
	BlockingKnowledgeKinds BlockingKnowledgeKindResolver
	KnowledgeViolation     KnowledgeViolationDetector
}

type PipelineRunResult struct {
	ExitCode    int
	FinalStatus string
	Paused      bool
}

type taskResult struct {
	name    string
	success bool
	skipped bool
	paused  bool
}
