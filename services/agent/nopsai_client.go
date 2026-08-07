package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"nopsai/pkg/correlation"
	"nopsai/pkg/serviceauth"
	agentapp "nopsai/services/agent/internal/app"
	"nopsai/services/agent/internal/approval"
	includeflow "nopsai/services/agent/internal/include"

	"github.com/rs/zerolog"
)

func nopsaiAgentRequest(ctx context.Context, method, endpoint string, payload any, out any) error {
	ctx, _ = correlation.EnsureRequestID(ctx)
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("NOPSAI_API_URL")), "/")
	if baseURL == "" {
		return fmt.Errorf("NOPSAI_API_URL is not configured")
	}
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: os.Getenv(serviceauth.EnvSigningKey),
		Issuer:     os.Getenv(serviceauth.EnvIssuer),
		Audience:   os.Getenv(serviceauth.EnvAudience),
		Role:       serviceauth.RoleAgent,
		ServiceID:  os.Getenv(serviceauth.EnvServiceID),
	})
	if err != nil {
		return err
	}
	token, err := credentials.MintToken(ctx)
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+"/"+strings.TrimLeft(endpoint, "/"), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	correlation.SetHTTPHeaders(ctx, req.Header)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("nopsai api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type runtimeOutputReportItem struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type runtimeOutputResolveResponse struct {
	Outputs []runtimeOutputReportItem `json:"outputs"`
}

func reportTaskOutputs(pipelineName, runID, stepName, taskName string, outputs map[string]agentapp.RuntimeOutputValue) error {
	if len(outputs) == 0 {
		return nil
	}
	items := make([]runtimeOutputReportItem, 0, len(outputs))
	for _, output := range outputs {
		items = append(items, runtimeOutputReportItem{
			Name:      output.Name,
			Value:     output.Value,
			Sensitive: output.Sensitive,
			SizeBytes: output.SizeBytes,
		})
	}
	endpoint := fmt.Sprintf(
		"/v1/internal/runs/%s/steps/%s/tasks/%s/outputs",
		url.PathEscape(runID),
		url.PathEscape(stepName),
		url.PathEscape(taskName),
	)
	err := nopsaiAgentRequest(context.Background(), http.MethodPost, endpoint, map[string]any{"outputs": items}, nil)
	if err != nil {
		stepLog(runID, pipelineName, stepName, taskName).Warn().Err(err).Msg("Failed to report runtime outputs to NopsAI API")
	}
	return err
}

func fetchChildRuntimeOutputs(ctx context.Context, parentRunID, parentStepName, childRunID string, names []string) (map[string]includeflow.RuntimeOutput, error) {
	if len(names) == 0 {
		return nil, nil
	}
	endpoint := fmt.Sprintf("/v1/internal/runs/%s/task-outputs/resolve", url.PathEscape(childRunID))
	payload := map[string]any{
		"parent_run_id":    parentRunID,
		"parent_step_name": parentStepName,
		"names":            names,
	}
	var resp runtimeOutputResolveResponse
	if err := nopsaiAgentRequest(ctx, http.MethodPost, endpoint, payload, &resp); err != nil {
		return nil, err
	}
	outputs := make(map[string]includeflow.RuntimeOutput, len(resp.Outputs))
	for _, item := range resp.Outputs {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		outputs[name] = includeflow.RuntimeOutput{
			Name:      name,
			Value:     item.Value,
			Sensitive: item.Sensitive,
			SizeBytes: item.SizeBytes,
		}
	}
	return outputs, nil
}

func requestApprovalPause(ctx context.Context, runID string, req approval.PausePayload) (approval.PauseResponse, error) {
	var resp approval.PauseResponse
	err := nopsaiAgentRequest(ctx, http.MethodPost, fmt.Sprintf("/v1/internal/runs/%s/approvals/pause", runID), req, &resp)
	return resp, err
}

func fetchApprovalCheckpoint(ctx context.Context, runID, checkpointID string) (agentApprovalCheckpointResponse, error) {
	var resp agentApprovalCheckpointResponse
	err := nopsaiAgentRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/internal/runs/%s/checkpoints/%s", runID, checkpointID), nil, &resp)
	return resp, err
}

func triggerPipeline(ctx context.Context, parentRunID, parentPipelineName, parentStepName, pipelineIdentifier string, pipelineDef []byte, history string, variables map[string]string, sensitiveVariables []string) (string, error) {
	if dispatcherClient == nil {
		return "", fmt.Errorf("dispatcher client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	scope := os.Getenv("SCOPE")
	parentRunnerID := os.Getenv("RUNNER_ID")
	gitContext := make(map[string]string)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GIT_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				gitContext[parts[0]] = parts[1]
			}
		}
	}

	return agentapp.TriggerPipeline(ctx, dispatcherClient, agentapp.TriggerPipelineRequest{
		ParentRunID:        parentRunID,
		ParentRunnerID:     parentRunnerID,
		ParentPipelineName: parentPipelineName,
		ParentStepName:     parentStepName,
		PipelineIdentifier: pipelineIdentifier,
		PipelineDefinition: pipelineDef,
		History:            history,
		Scope:              scope,
		GitContext:         gitContext,
		Variables:          variables,
		SensitiveVariables: sensitiveVariables,
	})
}

func monitorPipeline(ctx context.Context, logger *zerolog.Logger, runID string) (string, error) {
	if dispatcherClient == nil {
		return "", fmt.Errorf("dispatcher client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ticker := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	defer ticker.Stop()

	// Timeout for monitoring to prevent infinite waits
	monitorCtx, cancel := context.WithTimeout(ctx, 1*time.Hour)
	defer cancel()

	childLogger := zerolog.Nop()
	if logger != nil {
		childLogger = logger.With().Str("child_run_id", runID).Logger()
	}
	childLogger.Info().Msg("Starting to monitor child pipeline")
	for {
		select {
		case <-ticker.C:
			status, err := agentapp.GetRunStatus(monitorCtx, dispatcherClient, runID)
			if err != nil {
				childLogger.Error().Err(err).Msg("Failed to poll child pipeline status via dispatcher")
				continue
			}

			childLogger.Info().Str("status", status).Msg("Polling child pipeline status")
			if status == "success" || status == "warning" || status == "failure" || status == "cancelled" || status == "timed_out" || status == "rejected" {
				return status, nil
			}
		case <-monitorCtx.Done():
			return "failure", fmt.Errorf("monitor child pipeline %s: %w", runID, monitorCtx.Err())
		}
	}
}

func watchRunCancellation(pipelineName, runID string, onCancel func()) {
	if dispatcherClient == nil || strings.TrimSpace(runID) == "" || onCancel == nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		status, err := agentapp.GetRunStatus(reqCtx, dispatcherClient, runID)
		cancel()
		if err != nil {
			agentLog(runID, pipelineName).Warn().Err(err).Msg("Failed to poll run status for cancellation")
			continue
		}

		if strings.EqualFold(strings.TrimSpace(status), "cancelled") {
			agentLog(runID, pipelineName).Warn().Msg("Run was cancelled. Cleaning up and exiting")
			onCancel()
			return
		}
	}
}

func getPipelineDef(ctx context.Context, pipelineName string) ([]byte, error) {
	if dispatcherClient == nil {
		return nil, fmt.Errorf("dispatcher client not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	repoOwner := os.Getenv("GIT_REPO_OWNER")
	repoName := os.Getenv("GIT_REPO_NAME")
	commitSHA := os.Getenv("GIT_COMMIT_SHA")

	return agentapp.FetchPipeline(ctx, dispatcherClient, agentapp.FetchPipelineRequest{
		PipelineName: pipelineName,
		RepoOwner:    repoOwner,
		RepoName:     repoName,
		CommitSHA:    commitSHA,
	})
}
