package app

import (
	"context"
	"fmt"
	"strings"

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
type PolicyRevisionChecker func(context.Context, string) (models.PolicyRevisionResponse, error)
type TaskOutputReporter func(pipelineName, runID, stepName, taskName string, outputs map[string]RuntimeOutputValue) error

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
	ExecuteAction(context.Context, string, *proto.Action, []string, string, executor.OutputLineHandler) (string, string, int)
	PrepareOutputDirectory(context.Context, string) error
	CollectOutputs(context.Context, string, []models.TaskOutput, map[string]bool, int64) (map[string]RuntimeOutputValue, error)
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
	OutputsEnabled   bool
	RuntimePool      string
}

type ContainerStepRuntimeOptions struct {
	SharedVolumeName  string
	DockerNetworkName string
	GitRepoName       string
	RegistryAuth      dockerexec.RegistryAuthResolver
}

type ContainerStepRuntime struct {
	runtime           *ExecutionRuntime
	sharedVolumeName  string
	dockerNetworkName string
	gitRepoName       string
	registryAuth      dockerexec.RegistryAuthResolver
}

func NewContainerStepRuntime(runtime *ExecutionRuntime, opts ContainerStepRuntimeOptions) ContainerStepRuntime {
	return ContainerStepRuntime{
		runtime:           runtime,
		sharedVolumeName:  opts.SharedVolumeName,
		dockerNetworkName: opts.DockerNetworkName,
		gitRepoName:       opts.GitRepoName,
		registryAuth:      opts.RegistryAuth,
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
	dockerexec.StartImagePrePullWithAuth(ctx, prePullLogger, r.runtime.Docker, queue, r.registryAuth)
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
			OutputsEnabled:   req.OutputsEnabled,
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
		OutputsEnabled:    req.OutputsEnabled,
		SharedVolumeName:  r.sharedVolumeName,
		DockerNetworkName: r.dockerNetworkName,
		ContainerName:     stepContainerName,
		RegistryAuth:      r.registryAuth,
	})
}

func (r ContainerStepRuntime) ExecuteAction(ctx context.Context, sessionID string, action *proto.Action, runtimeVars []string, workingDirectory string, onLine executor.OutputLineHandler) (string, string, int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if r.runtime != nil && r.runtime.Kubernetes != nil {
		return r.runtime.Kubernetes.ExecuteAction(ctx, sessionID, action, runtimeVars, workingDirectory, onLine)
	}
	if r.runtime == nil || r.runtime.Docker == nil {
		return "", "execution runtime is not configured", 1
	}
	return executor.ExecuteDockerAction(ctx, r.runtime.Docker, sessionID, action, runtimeVars, workingDirectory, onLine)
}

func (r ContainerStepRuntime) PrepareOutputDirectory(ctx context.Context, sessionID string) error {
	action := &proto.Action{
		Type: "EXECUTE_COMMAND",
		Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{
			Command: fmt.Sprintf(
				"mkdir -p %s && (rm -rf %s/* %s/.[!.]* %s/..?* 2>/dev/null || true) && test -w %s",
				executor.ShellQuote(models.RuntimeOutputsMountPath),
				executor.ShellQuote(models.RuntimeOutputsMountPath),
				executor.ShellQuote(models.RuntimeOutputsMountPath),
				executor.ShellQuote(models.RuntimeOutputsMountPath),
				executor.ShellQuote(models.RuntimeOutputsMountPath),
			),
		}},
	}
	_, stderr, exitCode := r.ExecuteAction(ctx, sessionID, action, nil, models.DefaultPipelineWorkingDirectory, nil)
	if exitCode != 0 {
		return fmt.Errorf("runtime output directory %s is not writable: %s", models.RuntimeOutputsMountPath, strings.TrimSpace(stderr))
	}
	return nil
}

func (r ContainerStepRuntime) CollectOutputs(ctx context.Context, sessionID string, outputs []models.TaskOutput, required map[string]bool, maxBytes int64) (map[string]RuntimeOutputValue, error) {
	return collectRuntimeOutputFiles(ctx, r, sessionID, outputs, required, maxBytes)
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
