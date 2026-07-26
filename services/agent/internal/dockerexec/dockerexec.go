package dockerexec

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"nopsai/pkg/models"

	cerrdefs "github.com/containerd/errdefs"
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

type RegistryAuthResolver interface {
	Resolve(context.Context, string) (string, error)
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

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
	if err := EnsureImageExistsWithAuth(ctx, logger, cli, req.Image, req.RegistryAuth); err != nil {
		return "", err
	}

	binds := []string{fmt.Sprintf("%s:%s", req.SharedVolumeName, req.WorkingDirectory)}
	tmpfs := map[string]string(nil)
	if req.OutputsEnabled {
		tmpfs = map[string]string{models.RuntimeOutputsMountPath: "rw"}
	}
	for _, vol := range req.Volumes {
		parts := strings.Split(vol, ":")
		if len(parts) != 2 {
			logger.Error().Str("volume", vol).Msg("Invalid volume format. Must be 'volume-name:mount-path'. Skipping")
			continue
		}
		volumeName := parts[0]
		_, err := cli.VolumeInspect(ctx, volumeName, client.VolumeInspectOptions{})
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				logger.Info().Str("volume", volumeName).Msg("Volume not found, creating it now")
				_, createErr := cli.VolumeCreate(ctx, client.VolumeCreateOptions{Name: volumeName})
				if createErr != nil {
					logger.Error().Err(createErr).Str("volume", volumeName).Msg("Failed to create volume")
					continue
				}
			} else {
				logger.Error().Err(err).Str("volume", volumeName).Msg("Failed to inspect volume")
				continue
			}
		}
		binds = append(binds, vol)
	}

	cont, err := cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image:      req.Image,
			WorkingDir: req.WorkingDirectory,
			Entrypoint: []string{"tail", "-f", "/dev/null"},
			Env:        req.Env,
			Tty:        false,
		},
		HostConfig: &container.HostConfig{
			Binds:       binds,
			Tmpfs:       tmpfs,
			NetworkMode: container.NetworkMode(req.DockerNetworkName),
		},
		Name: req.ContainerName,
	})
	if err != nil {
		return "", err
	}
	if _, err := cli.ContainerStart(ctx, cont.ID, client.ContainerStartOptions{}); err != nil {
		return "", err
	}
	return cont.ID, nil
}

func Cleanup(ctx context.Context, logger *zerolog.Logger, cli *client.Client, containerID string) {
	if containerID == "" {
		return
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
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

func EnsureImageExists(ctx context.Context, logger *zerolog.Logger, cli *client.Client, imageName string) error {
	return EnsureImageExistsWithAuth(ctx, logger, cli, imageName, nil)
}

func EnsureImageExistsWithAuth(ctx context.Context, logger *zerolog.Logger, cli *client.Client, imageName string, authResolver RegistryAuthResolver) error {
	imageFilters := make(client.Filters).Add("reference", imageName)
	images, err := cli.ImageList(ctx, client.ImageListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("failed to list images to check for %s: %w", imageName, err)
	}

	if len(images.Items) == 0 {
		logger.Info().Str("image", imageName).Msg("Image not found locally, pulling")
		options := client.ImagePullOptions{}
		if authResolver != nil {
			registryAuth, err := authResolver.Resolve(ctx, imageName)
			if err != nil {
				logger.Warn().Err(err).Str("image", imageName).Msg("Failed to resolve local registry auth; pulling without registry auth")
			} else if strings.TrimSpace(registryAuth) != "" {
				options.RegistryAuth = registryAuth
				logger.Info().Str("image", imageName).Msg("Using local Docker registry auth for image pull")
			}
		}
		out, err := cli.ImagePull(ctx, imageName, options)
		if err != nil {
			return fmt.Errorf("failed to pull image %s: %w", imageName, err)
		}
		defer out.Close()
		io.Copy(io.Discard, out)
	} else {
		logger.Info().Str("image", imageName).Msg("Image found locally")
	}
	return nil
}

func StartImagePrePull(ctx context.Context, logger zerolog.Logger, cli *client.Client, queue []string) {
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
