package main

import (
	"context"

	"nopsai/pkg/proto"
	agentapp "nopsai/services/agent/internal/app"
)

var dispatcherClient proto.DispatcherServiceClient

func notifyFinalStatus(pipelineName, runID, status string) {
	if dispatcherClient == nil {
		agentLog(runID, pipelineName).Error().Msg("Dispatcher client not initialized. Cannot report final status")
		return
	}

	if err := agentapp.FinalizeRun(context.Background(), dispatcherClient, runID, status); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to send final status to dispatcher")
		return
	}

	agentLog(runID, pipelineName).Info().Str("status", status).Msg("Successfully notified dispatcher of final pipeline status")
}

// updateTaskStatus reports the final status of a task back through the dispatcher.
func updateTaskStatus(pipelineName, runID, stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	if dispatcherClient == nil {
		stepLog(runID, pipelineName, stepName, taskName).Error().Msg("Dispatcher client not initialized. Cannot report status")
		return
	}

	if err := agentapp.ReportTaskStatus(context.Background(), dispatcherClient, agentapp.TaskStatusReport{
		RunID:         runID,
		StepName:      stepName,
		TaskName:      taskName,
		Status:        status,
		ExitCode:      exitCode,
		LLMDurationMs: llmDurationMs,
	}); err != nil {
		stepLog(runID, pipelineName, stepName, taskName).Error().Err(err).Msg("Failed to send status update to dispatcher")
	}
}
