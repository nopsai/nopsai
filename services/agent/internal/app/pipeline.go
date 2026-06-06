package app

import (
	"context"
	"fmt"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/approval"
	"nopsai/services/agent/internal/dockerexec"
	"nopsai/services/agent/internal/executor"
	includeflow "nopsai/services/agent/internal/include"
	"nopsai/services/agent/internal/kubernetesexec"
	"nopsai/services/agent/internal/scheduler"

	"github.com/rs/zerolog"
)

type AgentLogger func(runID, pipeline string) *zerolog.Logger
type StepLogger func(runID, pipeline, step, task string) *zerolog.Logger
type TaskStatusReporter func(pipelineName, runID, stepName, taskName, status string, exitCode int, llmDurationMs int64)
type FinalStatusNotifier func(pipelineName, runID, status string)
type RunCancellationWatcher func(pipelineName, runID string, onCancel func())
type EnvironmentProvider func() []string
type ExitFunc func(code int)

type KnowledgePromptBuilder func(*models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) string
type BlockingKnowledgeKindResolver func(*models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) []string
type KnowledgeViolationDetector func(*proto.Action, *models.Pipeline, *models.PipelineStep, *models.Task, []models.KnowledgeContextSnapshot) (string, []string, bool)

type ApprovalPauser interface {
	Pause(context.Context, approval.Request) (approval.PauseResponse, error)
}

type IncludeRunner interface {
	Run(context.Context, includeflow.Request) includeflow.Result
}

type StepRuntime interface {
	Name() string
	PrePullImages(context.Context, zerolog.Logger, *models.Pipeline, int)
	CreateSession(context.Context, *zerolog.Logger, StepRuntimeSessionRequest) (string, error)
	ExecuteAction(context.Context, string, *proto.Action, []string, string) (string, string, int)
	CleanupSession(context.Context, *zerolog.Logger, string)
}

type StepRuntimeSessionRequest struct {
	RunID            string
	PipelineName     string
	StepName         string
	GitRepoName      string
	Image            string
	WorkingDirectory string
	Env              []string
	Volumes          []string
	RuntimePool      string
}

type ContainerStepRuntimeOptions struct {
	SharedVolumeName  string
	DockerNetworkName string
	GitRepoName       string
}

type ContainerStepRuntime struct {
	runtime           *ExecutionRuntime
	sharedVolumeName  string
	dockerNetworkName string
	gitRepoName       string
}

func NewContainerStepRuntime(runtime *ExecutionRuntime, opts ContainerStepRuntimeOptions) ContainerStepRuntime {
	return ContainerStepRuntime{
		runtime:           runtime,
		sharedVolumeName:  opts.SharedVolumeName,
		dockerNetworkName: opts.DockerNetworkName,
		gitRepoName:       opts.GitRepoName,
	}
}

func (r ContainerStepRuntime) Name() string {
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		return kubernetesexec.RuntimeName
	}
	return "docker"
}

func (r ContainerStepRuntime) PrePullImages(ctx context.Context, logger zerolog.Logger, pipeline *models.Pipeline, totalTasks int) {
	if r.runtime == nil || r.runtime.Docker == nil {
		return
	}
	queue := scheduler.ImagePullQueue(pipeline, totalTasks)
	if len(queue) == 0 {
		logger.Debug().Msg("No images to pre-pull for pipeline")
		return
	}
	prePullLogger := logger.With().Str("component", "image-prepull").Logger()
	dockerexec.StartImagePrePull(ctx, prePullLogger, r.runtime.Docker, queue)
}

func (r ContainerStepRuntime) CreateSession(ctx context.Context, logger *zerolog.Logger, req StepRuntimeSessionRequest) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime == nil {
		return "", fmt.Errorf("execution runtime is not configured")
	}
	if r.runtime.Kubernetes != nil {
		if logger != nil {
			logger.Info().Str("image", req.Image).Msg("Creating new Kubernetes pod for step")
		}
		return r.runtime.Kubernetes.CreateStepPod(ctx, kubernetesexec.StepPodRequest{
			RunID:            req.RunID,
			PipelineName:     req.PipelineName,
			StepName:         req.StepName,
			Image:            req.Image,
			WorkingDirectory: req.WorkingDirectory,
			Env:              req.Env,
			Volumes:          req.Volumes,
			RuntimePool:      req.RuntimePool,
		})
	}
	if r.runtime.Docker == nil {
		return "", fmt.Errorf("Docker client is not configured")
	}
	if logger != nil {
		logger.Info().Str("image", req.Image).Msg("Creating new container for step")
	}
	stepContainerName := dockerexec.BuildStepContainerName(firstNonEmpty(req.GitRepoName, r.gitRepoName), req.PipelineName, req.StepName, req.RunID)
	return dockerexec.CreateStepContainer(ctx, logger, r.runtime.Docker, dockerexec.StepContainerRequest{
		Image:             req.Image,
		WorkingDirectory:  req.WorkingDirectory,
		Env:               req.Env,
		Volumes:           req.Volumes,
		SharedVolumeName:  r.sharedVolumeName,
		DockerNetworkName: r.dockerNetworkName,
		ContainerName:     stepContainerName,
	})
}

func (r ContainerStepRuntime) ExecuteAction(ctx context.Context, sessionID string, action *proto.Action, runtimeVars []string, workingDirectory string) (string, string, int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		return r.runtime.Kubernetes.ExecuteAction(ctx, sessionID, action, runtimeVars, workingDirectory)
	}
	if r.runtime == nil || r.runtime.Docker == nil {
		return "", "execution runtime is not configured", 1
	}
	return executor.ExecuteDockerAction(ctx, r.runtime.Docker, sessionID, action, runtimeVars, workingDirectory)
}

func (r ContainerStepRuntime) CleanupSession(ctx context.Context, logger *zerolog.Logger, sessionID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		r.runtime.Kubernetes.CleanupPod(ctx, sessionID)
		return
	}
	if r.runtime == nil || r.runtime.Docker == nil {
		return
	}
	dockerexec.Cleanup(ctx, logger, r.runtime.Docker, sessionID)
}
