package app

import (
	"fmt"

	"nopsai/services/agent/internal/kubernetesexec"

	"github.com/moby/moby/client"
	"github.com/rs/zerolog"
)

type ExecutionRuntime struct {
	Docker     *client.Client
	Kubernetes *kubernetesexec.Runtime
}

func NewExecutionRuntime(mode, sharedVolumeName string, affinityEnabled *bool, logger *zerolog.Logger) (*ExecutionRuntime, error) {
	if mode == kubernetesexec.RuntimeName {
		runtime, err := kubernetesexec.NewFromEnv(sharedVolumeName, affinityEnabled, logger)
		if err != nil {
			return nil, fmt.Errorf("initialize Kubernetes runtime: %w", err)
		}
		return &ExecutionRuntime{Kubernetes: runtime}, nil
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	return &ExecutionRuntime{Docker: dockerClient}, nil
}

func (r *ExecutionRuntime) Close() error {
	if r == nil || r.Docker == nil {
		return nil
	}
	return r.Docker.Close()
}
