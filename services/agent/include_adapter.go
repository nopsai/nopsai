package agent

import (
	"strings"

	includeflow "nopsai/services/agent/internal/include"
)

func newAgentChildPipelineIncludeRunner() includeflow.Runner {
	return includeflow.NewRunner(includeflow.Config{
		FetchDefinition: getPipelineDef,
		TriggerPipeline: triggerPipeline,
		MonitorPipeline: monitorPipeline,
		IsNotFound: func(err error) bool {
			return err != nil && strings.Contains(err.Error(), "nopsai api returned non-200 status 404")
		},
	})
}
