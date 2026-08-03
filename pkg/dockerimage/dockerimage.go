package dockerimage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/moby/moby/client"
)

type RegistryAuthResolver interface {
	Resolve(context.Context, string) (string, error)
}

type EnsureResult struct {
	FoundLocal       bool
	Pulled           bool
	UsedRegistryAuth bool
}

func EnsureExists(ctx context.Context, cli *client.Client, imageName string, authResolver RegistryAuthResolver) (EnsureResult, error) {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" {
		return EnsureResult{}, fmt.Errorf("image name is required")
	}
	if cli == nil {
		return EnsureResult{}, fmt.Errorf("docker client is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	imageFilters := make(client.Filters).Add("reference", imageName)
	images, err := cli.ImageList(ctx, client.ImageListOptions{Filters: imageFilters})
	if err != nil {
		return EnsureResult{}, fmt.Errorf("list images for %s: %w", imageName, err)
	}
	if len(images.Items) > 0 {
		return EnsureResult{FoundLocal: true}, nil
	}

	options, usingRegistryAuth, err := PullOptions(ctx, imageName, authResolver)
	if err != nil {
		return EnsureResult{}, err
	}
	out, err := cli.ImagePull(ctx, imageName, options)
	if err != nil {
		return EnsureResult{}, fmt.Errorf("pull image %s: %w", imageName, err)
	}
	if err := drainAndClosePullResponse(out); err != nil {
		return EnsureResult{}, fmt.Errorf("pull image %s: %w", imageName, err)
	}
	return EnsureResult{Pulled: true, UsedRegistryAuth: usingRegistryAuth}, nil
}

func PullOptions(ctx context.Context, imageName string, authResolver RegistryAuthResolver) (client.ImagePullOptions, bool, error) {
	options := client.ImagePullOptions{}
	if authResolver == nil {
		return options, false, nil
	}
	registryAuth, err := authResolver.Resolve(ctx, imageName)
	if err != nil {
		return options, false, fmt.Errorf("resolve registry auth for image %s: %w", imageName, err)
	}
	registryAuth = strings.TrimSpace(registryAuth)
	if registryAuth == "" {
		return options, false, nil
	}
	options.RegistryAuth = registryAuth
	return options, true, nil
}

func drainAndClosePullResponse(out io.ReadCloser) error {
	if out == nil {
		return nil
	}
	_, copyErr := io.Copy(io.Discard, out)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("read pull response: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close pull response: %w", closeErr)
	}
	return nil
}
