package nopsai

import (
	"context"
	"strings"

	"nopsai/services/nopsai/internal/runnerinstall"
	systemlogkubernetes "nopsai/services/nopsai/internal/systemlogs/kubernetes"
)

func (a *App) RunnerSourceHints(ctx context.Context) ([]systemlogkubernetes.RunnerSourceHint, error) {
	if a == nil {
		return nil, nil
	}
	platformID := runnerinstall.KubernetesPlatformID(a.getConfigSnapshot())
	status, err := a.fetchDispatcherStatus(ctx)
	if err != nil {
		return nil, err
	}
	revoked := a.revokedRunnerIDSet()
	hints := make([]systemlogkubernetes.RunnerSourceHint, 0, len(status.GetRunners()))
	seen := map[string]struct{}{}
	for _, runner := range status.GetRunners() {
		runnerID := strings.TrimSpace(runner.GetRunnerId())
		if runnerID == "" {
			continue
		}
		if _, blocked := revoked[runnerID]; blocked {
			continue
		}
		metadata := runner.GetMetadata()
		runtime := strings.ToLower(strings.TrimSpace(metadata["runtime"]))
		namespace := strings.TrimSpace(metadata["kubernetes_namespace"])
		selector := strings.TrimSpace(metadata["kubernetes_label_selector"])
		if runtime != "kubernetes" && namespace == "" && selector == "" {
			continue
		}
		if namespace == "" || selector == "" {
			continue
		}
		sourceID := runnerLogSourceID(runnerID, metadata)
		if sourceID == "" {
			continue
		}
		runnerPlatformID := strings.TrimSpace(metadata["nopsai_platform_id"])
		if runnerPlatformID == "" {
			runnerPlatformID = platformID
		}
		if _, exists := seen[sourceID]; exists {
			continue
		}
		seen[sourceID] = struct{}{}
		hints = append(hints, systemlogkubernetes.RunnerSourceHint{
			RunnerID:      runnerID,
			SourceID:      sourceID,
			PlatformID:    runnerPlatformID,
			Namespace:     namespace,
			LabelSelector: selector,
			ContainerName: "runner",
		})
	}
	return hints, nil
}
