package dockerexec

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nopsai/pkg/dockerimage"
	"nopsai/pkg/dockervolume"
	"nopsai/pkg/models"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog"
)

type StepContainerRequest struct {
	Image             string
	WorkingDirectory  string
	Env               []string
	Volumes           []string
	OutputsEnabled    bool
	SharedVolumeName  string
	DockerNetworkName string
	ContainerName     string
	RegistryAuth      RegistryAuthResolver
}

type RegistryAuthResolver = dockerimage.RegistryAuthResolver

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

const (
	dockerStepVolumeManagedLabel = "nopsai.io/managed"
	dockerStepVolumePurposeLabel = "nopsai.io/volume-purpose"
	dockerStepVolumeOwnerLabel   = "nopsai.io/shared-volume"
	dockerStepVolumePurpose      = "pipeline-step"
	dockerStepTmpfsOptions       = "rw,noexec,nosuid,nodev,size=64m"
	dockerStepOutputsTmpfs       = "rw,nosuid,nodev,size=64m"
)

var defaultDockerStepPidsLimit int64 = 512

func BuildStepContainerName(repoName, pipelineName, stepName, runID string) string {
	sanitizedPipelineName := sanitizeInput(pipelineName)
	sanitizedStepName := sanitizeInput(stepName)
	shortRunID := runID
	if len(shortRunID) > 8 {
		shortRunID = shortRunID[:8]
	}

	if strings.TrimSpace(repoName) != "" {
		sanitizedRepoName := sanitizeInput(repoName)
		return fmt.Sprintf("%s-%s-%s-%s", sanitizedRepoName, sanitizedPipelineName, sanitizedStepName, shortRunID)
	}
	return fmt.Sprintf("%s-%s-%s", sanitizedPipelineName, sanitizedStepName, shortRunID)
}

func CreateStepContainer(ctx context.Context, logger *zerolog.Logger, cli *client.Client, req StepContainerRequest) (string, error) {
	if strings.TrimSpace(req.SharedVolumeName) == "" {
		return "", fmt.Errorf("shared volume name is required")
	}
	if err := EnsureImageExistsWithAuth(ctx, logger, cli, req.Image, req.RegistryAuth); err != nil {
		return "", err
	}

	binds := []string{fmt.Sprintf("%s:%s", req.SharedVolumeName, req.WorkingDirectory)}
	tmpfs := dockerStepTmpfs(req.OutputsEnabled)
	for _, vol := range req.Volumes {
		volumeName, mountPath, err := parseDockerStepVolumeSpec(vol)
		if err != nil {
			return "", err
		}
		if err := ensureManagedDockerStepVolume(ctx, logger, cli, volumeName, req.SharedVolumeName); err != nil {
			return "", err
		}
		binds = append(binds, fmt.Sprintf("%s:%s", volumeName, mountPath))
	}

	cont, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      req.Image,
			WorkingDir: req.WorkingDirectory,
			Entrypoint: []string{"tail", "-f", "/dev/null"},
			Env:        req.Env,
			Tty:        false,
		},
		HostConfig: dockerStepHostConfig(binds, tmpfs, req.DockerNetworkName),
		Name:       req.ContainerName,
	})
	if err != nil {
		return "", err
	}
	if _, err := cli.ContainerStart(ctx, cont.ID, client.ContainerStartOptions{}); err != nil {
		cleanupCtx, cancel := dockerCleanupContext(ctx, 30*time.Second)
		defer cancel()
		if _, removeErr := cli.ContainerRemove(cleanupCtx, cont.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil && logger != nil {
			logger.Warn().Err(removeErr).Str("container_id", cont.ID).Msg("Failed to roll back step container after start failure")
		}
		return "", err
	}
	return cont.ID, nil
}

func parseDockerStepVolumeSpec(spec string) (string, string, error) {
	volumeName, mountPath, ok := strings.Cut(spec, ":")
	volumeName = strings.TrimSpace(volumeName)
	mountPath = strings.TrimSpace(mountPath)
	if !ok || volumeName == "" || mountPath == "" || !strings.HasPrefix(mountPath, "/") ||
		strings.ContainsAny(volumeName, `/\`) {
		return "", "", fmt.Errorf("invalid volume format %q; must be 'volume-name:/mount-path'", spec)
	}
	return volumeName, mountPath, nil
}

func ensureManagedDockerStepVolume(ctx context.Context, logger *zerolog.Logger, cli *client.Client, volumeName, owner string) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return fmt.Errorf("step volume owner is required")
	}
	if logger != nil {
		logger.Debug().Str("volume", volumeName).Msg("Ensuring NopsAI-managed step volume")
	}
	return dockervolume.EnsureManaged(ctx, cli, dockervolume.ManagedSpec{
		Name:              volumeName,
		Labels:            dockerStepVolumeLabels(owner),
		ValidateOwnership: func(labels map[string]string) bool { return dockerStepVolumeOwnedBy(labels, owner) },
		OwnerDescription:  "run",
	})
}

func dockerStepTmpfs(outputsEnabled bool) map[string]string {
	tmpfs := map[string]string{
		"/tmp":     dockerStepTmpfsOptions,
		"/var/tmp": dockerStepTmpfsOptions,
	}
	if !outputsEnabled {
		return tmpfs
	}
	tmpfs[models.RuntimeOutputsMountPath] = dockerStepOutputsTmpfs
	return tmpfs
}

func dockerStepHostConfig(binds []string, tmpfs map[string]string, networkName string) *container.HostConfig {
	initProcess := true
	return &container.HostConfig{
		Binds:          binds,
		Tmpfs:          tmpfs,
		NetworkMode:    dockerStepNetworkMode(networkName),
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{"no-new-privileges:true"},
		ReadonlyRootfs: true,
		Resources: container.Resources{
			PidsLimit: &defaultDockerStepPidsLimit,
		},
		Init: &initProcess,
	}
}

func dockerStepNetworkMode(networkName string) container.NetworkMode {
	networkName = strings.TrimSpace(networkName)
	switch strings.ToLower(networkName) {
	case "", "none", "nopsai-net":
		return container.NetworkMode("none")
	default:
		return container.NetworkMode(networkName)
	}
}

func dockerStepVolumeLabels(owner string) map[string]string {
	return map[string]string{
		dockerStepVolumeManagedLabel: "true",
		dockerStepVolumePurposeLabel: dockerStepVolumePurpose,
		dockerStepVolumeOwnerLabel:   strings.TrimSpace(owner),
	}
}

func dockerStepVolumeOwnedBy(labels map[string]string, owner string) bool {
	return strings.EqualFold(strings.TrimSpace(labels[dockerStepVolumeManagedLabel]), "true") &&
		strings.TrimSpace(labels[dockerStepVolumePurposeLabel]) == dockerStepVolumePurpose &&
		strings.TrimSpace(labels[dockerStepVolumeOwnerLabel]) == strings.TrimSpace(owner)
}

func Cleanup(ctx context.Context, logger *zerolog.Logger, cli *client.Client, containerID string) {
	if containerID == "" {
		return
	}
	ctx, cancel := dockerCleanupContext(ctx, 30*time.Second)
	defer cancel()

	if logger == nil {
		nullLogger := zerolog.Nop()
		logger = &nullLogger
	}

	logger.Info().Str("container_id", containerID).Msg("Cleaning up pipeline container")
	timeout := 1
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		logger.Error().Err(err).Msg("Failed to stop pipeline container")
	}

	waitResult := cli.ContainerWait(ctx, containerID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			logger.Error().Err(err).Msg("Error waiting for container to stop")
		}
	case <-waitResult.Result:
	}

	if _, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
		logger.Error().Err(err).Msg("Failed to remove pipeline container")
	}
}

// EnsureImageExists keeps the pre-registry-auth image pull API available for
// older callers.
//
// Deprecated: use EnsureImageExistsWithAuth. Remove after NopsAI v3.0.0 when
// all supported runners and agents pass explicit registry-auth resolvers.
func EnsureImageExists(ctx context.Context, logger *zerolog.Logger, cli *client.Client, imageName string) error {
	return EnsureImageExistsWithAuth(ctx, logger, cli, imageName, nil)
}

func EnsureImageExistsWithAuth(ctx context.Context, logger *zerolog.Logger, cli *client.Client, imageName string, authResolver RegistryAuthResolver) error {
	result, err := dockerimage.EnsureExists(ctx, cli, imageName, authResolver)
	if err != nil {
		return fmt.Errorf("failed to ensure image %s exists: %w", imageName, err)
	}
	if logger == nil {
		return nil
	}
	switch {
	case result.FoundLocal:
		logger.Info().Str("image", imageName).Msg("Image found locally")
	case result.Pulled && result.UsedRegistryAuth:
		logger.Info().Str("image", imageName).Msg("Pulled image with local Docker registry auth")
	case result.Pulled:
		logger.Info().Str("image", imageName).Msg("Pulled image")
	}
	return nil
}

// StartImagePrePull keeps the pre-registry-auth pre-pull API available for
// older callers.
//
// Deprecated: use StartImagePrePullWithAuth. Remove after NopsAI v3.0.0 when
// all supported runners and agents pass explicit registry-auth resolvers.
func StartImagePrePull(ctx context.Context, logger zerolog.Logger, cli *client.Client, queue []string) {
	logger.Warn().Str("removal_version", "v3.0.0").Msg("Deprecated Docker image pre-pull wrapper used without explicit registry auth")
	StartImagePrePullWithAuth(ctx, logger, cli, queue, nil)
}

func StartImagePrePullWithAuth(ctx context.Context, logger zerolog.Logger, cli *client.Client, queue []string, authResolver RegistryAuthResolver) {
	if cli == nil || len(queue) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	logger.Info().Int("count", len(queue)).Msg("Starting asynchronous image pre-pull")
	go func() {
		for i, imageName := range queue {
			select {
			case <-ctx.Done():
				logger.Warn().Msg("Stopping image pre-pull due to cancellation")
				return
			default:
			}

			imageLogger := logger.With().
				Str("image", imageName).
				Int("position", i+1).
				Int("total", len(queue)).
				Logger()

			if err := EnsureImageExistsWithAuth(ctx, &imageLogger, cli, imageName, authResolver); err != nil {
				imageLogger.Warn().Err(err).Msg("Failed to pre-pull image; will pull on demand during execution")
			}
		}
	}()
}

func sanitizeInput(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

func dockerCleanupContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
