package dockervolume

import (
	"context"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type ManagedSpec struct {
	Name              string
	Labels            map[string]string
	ValidateOwnership func(map[string]string) bool
	OwnerDescription  string
}

func EnsureManaged(ctx context.Context, cli *client.Client, spec ManagedSpec) error {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return fmt.Errorf("volume name is required")
	}
	if cli == nil {
		return fmt.Errorf("docker client is required")
	}
	if spec.ValidateOwnership == nil {
		return fmt.Errorf("volume ownership validator is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	inspected, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err == nil {
		if spec.ValidateOwnership(inspected.Volume.Labels) {
			return nil
		}
		return fmt.Errorf("docker volume %q is not owned by this NopsAI %s", name, ownerDescription(spec.OwnerDescription))
	}
	if !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("inspect docker volume %q: %w", name, err)
	}

	_, err = cli.VolumeCreate(ctx, client.VolumeCreateOptions{
		Name:   name,
		Labels: cloneLabels(spec.Labels),
	})
	if err != nil {
		return fmt.Errorf("create docker volume %q: %w", name, err)
	}
	inspected, err = cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return fmt.Errorf("inspect docker volume %q after create: %w", name, err)
	}
	if !spec.ValidateOwnership(inspected.Volume.Labels) {
		return fmt.Errorf("docker volume %q is not owned by this NopsAI %s", name, ownerDescription(spec.OwnerDescription))
	}
	return nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	clone := make(map[string]string, len(labels))
	for key, value := range labels {
		clone[key] = value
	}
	return clone
}

func ownerDescription(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return "owner"
	}
	return description
}
