package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nopsai/pkg/correlation"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type nopsaiClient interface {
	IngestLogs(context.Context, string, []string, LogIngestMetadata) error
	ReportTaskStatus(context.Context, *proto.TaskStatusReport) error
	FinalizeRun(context.Context, string, string, string) error
	FetchPipeline(context.Context, *proto.FetchPipelineRequest) ([]byte, error)
	TriggerPipeline(context.Context, *proto.TriggerPipelineRequest) (*proto.TriggerPipelineResponse, error)
	RunStatus(context.Context, string) (string, error)
	DispatcherControlConfig(context.Context) (dispatcherControlConfig, error)
}

type LogIngestMetadata struct {
	Source      string         `json:"source,omitempty"`
	ServiceID   string         `json:"service_id,omitempty"`
	ServiceRole string         `json:"service_role,omitempty"`
	RequestID   string         `json:"request_id,omitempty"`
	Traceparent string         `json:"traceparent,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type nopsaiHTTPClient struct {
	baseURL     string
	httpClient  *http.Client
	credentials *serviceauth.Credentials
}

func newNopsaiHTTPClient(baseURL string, credentials *serviceauth.Credentials) *nopsaiHTTPClient {
	return &nopsaiHTTPClient{
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		credentials: credentials,
	}
}

func (c *nopsaiHTTPClient) setHTTPClient(client *http.Client) {
	if client != nil {
		c.httpClient = client
	}
}

func (c *nopsaiHTTPClient) IngestLogs(ctx context.Context, runID string, lines []string, metadata LogIngestMetadata) error {
	ctx, requestID := correlation.EnsureRequestID(ctx)
	metadata = normalizeLogIngestMetadata(ctx, metadata)
	if metadata.RequestID == "" {
		metadata.RequestID = requestID
	}
	body, _ := json.Marshal(struct {
		Lines       []string       `json:"lines"`
		Source      string         `json:"source,omitempty"`
		ServiceID   string         `json:"service_id,omitempty"`
		ServiceRole string         `json:"service_role,omitempty"`
		RequestID   string         `json:"request_id,omitempty"`
		Traceparent string         `json:"traceparent,omitempty"`
		Metadata    map[string]any `json:"metadata,omitempty"`
	}{
		Lines:       lines,
		Source:      metadata.Source,
		ServiceID:   metadata.ServiceID,
		ServiceRole: metadata.ServiceRole,
		RequestID:   metadata.RequestID,
		Traceparent: metadata.Traceparent,
		Metadata:    metadata.Metadata,
	})
	return c.postJSON(ctx, fmt.Sprintf("/v1/runs/%s/logs/ingest", strings.TrimSpace(runID)), body, http.StatusOK, http.StatusNoContent)
}

func (c *nopsaiHTTPClient) ReportTaskStatus(ctx context.Context, report *proto.TaskStatusReport) error {
	if report == nil {
		return status.Error(codes.InvalidArgument, "task status report is required")
	}
	payload := map[string]interface{}{
		"status":          report.Status,
		"exit_code":       report.ExitCode,
		"llm_duration_ms": report.LlmDurationMs,
	}
	body, _ := json.Marshal(payload)
	path := fmt.Sprintf(
		"/v1/runs/%s/steps/%s/tasks/%s",
		strings.TrimSpace(report.RunId),
		url.PathEscape(report.StepName),
		url.PathEscape(report.TaskName),
	)
	return c.postJSON(ctx, path, body, http.StatusOK)
}

func (c *nopsaiHTTPClient) FinalizeRun(ctx context.Context, runID, statusText, failureReason string) error {
	payload := map[string]string{"status": statusText}
	if strings.TrimSpace(failureReason) != "" {
		payload["failure_reason"] = strings.TrimSpace(failureReason)
	}
	body, _ := json.Marshal(payload)
	return c.postJSON(ctx, fmt.Sprintf("/v1/runs/%s/finalize", strings.TrimSpace(runID)), body, http.StatusOK)
}

func (c *nopsaiHTTPClient) FetchPipeline(ctx context.Context, req *proto.FetchPipelineRequest) ([]byte, error) {
	if err := c.requireBaseURL(); err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s/v1/pipelines/%s", c.baseURL, strings.TrimLeft(req.PipelineName, "/"))
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "parse pipeline url: %v", err)
	}
	query := parsed.Query()
	if strings.TrimSpace(req.RepoOwner) != "" {
		query.Set("repoOwner", req.RepoOwner)
	}
	if strings.TrimSpace(req.RepoName) != "" {
		query.Set("repoName", req.RepoName)
	}
	if strings.TrimSpace(req.CommitSha) != "" {
		query.Set("commitSHA", req.CommitSha)
	}
	parsed.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build pipeline request: %v", err)
	}
	if err := c.authorize(ctx, httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize pipeline request: %v", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "fetch pipeline: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.FailedPrecondition, "nopsai pipeline returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *nopsaiHTTPClient) TriggerPipeline(ctx context.Context, req *proto.TriggerPipelineRequest) (*proto.TriggerPipelineResponse, error) {
	if err := c.requireBaseURL(); err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s/v1/run", c.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(req.PipelineDefinition))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build trigger request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-yaml")
	if err := c.authorize(ctx, httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize trigger request: %v", err)
	}

	if value := strings.TrimSpace(req.ParentRunId); value != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Run-ID", value)
	}
	if value := strings.TrimSpace(req.ParentRunnerId); value != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Runner-ID", value)
	}
	if value := strings.TrimSpace(req.ParentPipelineName); value != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Pipeline-Name", value)
	}
	if value := strings.TrimSpace(req.ParentStepName); value != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Step-Name", value)
	}
	if path := pipelinePath(strings.TrimSpace(req.PipelineIdentifier)); path != "" {
		httpReq.Header.Set("X-Nopsai-Pipeline-Path", path)
	}
	if history := strings.TrimSpace(req.History); history != "" {
		httpReq.Header.Set("X-Nopsai-Parent-History", base64.StdEncoding.EncodeToString([]byte(history)))
	}
	if scope := strings.TrimSpace(req.Scope); scope != "" {
		httpReq.Header.Set("X-Nopsai-Scope", scope)
	}
	for key, value := range req.GitContext {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		httpReq.Header.Set(gitHeaderKey(key), value)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "trigger pipeline: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return &proto.TriggerPipelineResponse{
			Status: resp.Status,
			Error:  fmt.Sprintf("nopsai trigger returned %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	const prefix = "Pipeline run created successfully with ID: "
	runID := strings.TrimSpace(strings.TrimPrefix(string(body), prefix))
	if runID == "" {
		return &proto.TriggerPipelineResponse{
			Status: resp.Status,
			Error:  fmt.Sprintf("unexpected response body: %s", string(body)),
		}, nil
	}
	return &proto.TriggerPipelineResponse{RunId: runID, Status: "created"}, nil
}

func (c *nopsaiHTTPClient) RunStatus(ctx context.Context, runID string) (string, error) {
	if err := c.requireBaseURL(); err != nil {
		return "", err
	}

	target := fmt.Sprintf("%s/v1/runs/%s/status", c.baseURL, strings.TrimSpace(runID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", status.Errorf(codes.Internal, "build run status request: %v", err)
	}
	if err := c.authorize(ctx, httpReq); err != nil {
		return "", status.Errorf(codes.Internal, "authorize run status request: %v", err)
	}

	body, err := c.doRead(httpReq, http.StatusOK, "fetch run status")
	if err != nil {
		return "", err
	}
	var statusResp map[string]string
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return "", status.Errorf(codes.Internal, "decode run status response: %v", err)
	}
	return statusResp["status"], nil
}

type dispatcherControlConfig struct {
	DispatcherRouting map[string][]string
	EjectedRunnerIDs  []string
}

func (c *nopsaiHTTPClient) DispatcherControlConfig(ctx context.Context) (dispatcherControlConfig, error) {
	if err := c.requireBaseURL(); err != nil {
		return dispatcherControlConfig{}, err
	}

	target := c.baseURL + "/v1/internal/dispatcher/routing"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return dispatcherControlConfig{}, status.Errorf(codes.Internal, "build dispatcher routing request: %v", err)
	}
	if err := c.authorize(ctx, httpReq); err != nil {
		return dispatcherControlConfig{}, status.Errorf(codes.Internal, "authorize dispatcher routing request: %v", err)
	}

	body, err := c.doRead(httpReq, http.StatusOK, "fetch dispatcher routing")
	if err != nil {
		return dispatcherControlConfig{}, err
	}
	var payload dispatcherRoutingResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return dispatcherControlConfig{}, status.Errorf(codes.Internal, "decode dispatcher routing response: %v", err)
	}
	return dispatcherControlConfig{
		DispatcherRouting: payload.DispatcherRouting,
		EjectedRunnerIDs:  payload.EjectedRunnerIDs,
	}, nil
}

func (c *nopsaiHTTPClient) postJSON(ctx context.Context, path string, body []byte, expectedStatuses ...int) error {
	if err := c.requireBaseURL(); err != nil {
		return err
	}
	ctx, _ = correlation.EnsureRequestID(ctx)
	target := c.baseURL + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return status.Errorf(codes.Internal, "build nopsai request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := c.authorize(ctx, httpReq); err != nil {
		return status.Errorf(codes.Internal, "authorize nopsai request: %v", err)
	}
	_, err = c.doRead(httpReq, expectedStatuses[0], "send nopsai request", expectedStatuses[1:]...)
	return err
}

func (c *nopsaiHTTPClient) doRead(req *http.Request, expectedStatus int, operation string, extraExpectedStatuses ...int) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "%s: %v", operation, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == expectedStatus {
		return body, nil
	}
	for _, code := range extraExpectedStatuses {
		if resp.StatusCode == code {
			return body, nil
		}
	}
	return nil, status.Errorf(codes.FailedPrecondition, "nopsai %s returned %d: %s", req.URL.Path, resp.StatusCode, string(body))
}

func (c *nopsaiHTTPClient) requireBaseURL() error {
	if c == nil || c.baseURL == "" {
		return status.Error(codes.FailedPrecondition, "nopsai api url is not configured on dispatcher")
	}
	return nil
}

func (c *nopsaiHTTPClient) authorize(ctx context.Context, req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if c.credentials == nil {
		return fmt.Errorf("internal service credentials are not configured")
	}
	token, err := c.credentials.MintToken(ctx)
	if err != nil {
		return fmt.Errorf("mint dispatcher token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	correlation.SetHTTPHeaders(ctx, req.Header)
	return nil
}

func normalizeLogIngestMetadata(ctx context.Context, metadata LogIngestMetadata) LogIngestMetadata {
	metadata.Source = strings.TrimSpace(metadata.Source)
	metadata.ServiceID = strings.TrimSpace(metadata.ServiceID)
	metadata.ServiceRole = strings.TrimSpace(metadata.ServiceRole)
	metadata.RequestID = strings.TrimSpace(metadata.RequestID)
	metadata.Traceparent = strings.TrimSpace(metadata.Traceparent)
	if metadata.RequestID == "" {
		metadata.RequestID = correlation.RequestIDFromContext(ctx)
	}
	if metadata.Traceparent == "" {
		metadata.Traceparent = correlation.TraceparentFromContext(ctx)
	}
	if claims, ok := serviceauth.ClaimsFromContext(ctx); ok {
		if metadata.ServiceID == "" {
			metadata.ServiceID = claims.ServiceID()
		}
		if metadata.ServiceRole == "" {
			metadata.ServiceRole = claims.ServiceRole()
		}
		if metadata.Source == "" {
			metadata.Source = claims.ServiceRole()
		}
	}
	if metadata.Source == "" {
		metadata.Source = "dispatcher"
	}
	if metadata.Metadata == nil {
		metadata.Metadata = map[string]any{}
	}
	if metadata.ServiceID != "" && metadata.Metadata["service_id"] == nil {
		metadata.Metadata["service_id"] = metadata.ServiceID
	}
	if metadata.ServiceRole != "" && metadata.Metadata["service_role"] == nil {
		metadata.Metadata["service_role"] = metadata.ServiceRole
	}
	return metadata
}

type dispatcherRoutingResponse struct {
	DispatcherRouting map[string][]string `json:"dispatcher_routing"`
	EjectedRunnerIDs  []string            `json:"ejected_runner_ids"`
}
