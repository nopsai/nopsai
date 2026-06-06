package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"

	"google.golang.org/grpc"
)

type DispatcherClientConfig struct {
	Address       string
	ServiceID     string
	SigningKey    string
	Issuer        string
	Audience      string
	TLSMode       string
	TLSSecret     string
	TLSServerName string
}

type TaskStatusReport struct {
	RunID         string
	StepName      string
	TaskName      string
	Status        string
	ExitCode      int
	LLMDurationMs int64
}

type TriggerPipelineRequest struct {
	ParentRunID        string
	ParentRunnerID     string
	ParentPipelineName string
	ParentStepName     string
	PipelineIdentifier string
	PipelineDefinition []byte
	History            string
	Scope              string
	GitContext         map[string]string
}

type FetchPipelineRequest struct {
	PipelineName string
	RepoOwner    string
	RepoName     string
	CommitSHA    string
}

func LoadDispatcherClientConfig(lookup EnvLookup) (DispatcherClientConfig, error) {
	if lookup == nil {
		lookup = os.Getenv
	}

	cfg := DispatcherClientConfig{
		Address:       strings.TrimSpace(lookup("DISPATCHER_ADDRESS")),
		ServiceID:     strings.TrimSpace(lookup(serviceauth.EnvServiceID)),
		SigningKey:    lookup(serviceauth.EnvSigningKey),
		Issuer:        lookup(serviceauth.EnvIssuer),
		Audience:      lookup(serviceauth.EnvAudience),
		TLSMode:       lookup(servicetls.EnvMode),
		TLSSecret:     strings.TrimSpace(lookup(servicetls.EnvSecret)),
		TLSServerName: lookup(servicetls.EnvServerName),
	}
	if cfg.Address == "" {
		return cfg, fmt.Errorf("DISPATCHER_ADDRESS is not configured")
	}
	if cfg.ServiceID == "" {
		cfg.ServiceID = strings.TrimSpace(lookup("AGENT_SERVICE_ID"))
	}
	if cfg.TLSSecret == "" {
		cfg.TLSSecret = cfg.SigningKey
	}
	return cfg, nil
}

func NewDispatcherClientFromEnv(lookup EnvLookup) (*grpc.ClientConn, proto.DispatcherServiceClient, error) {
	cfg, err := LoadDispatcherClientConfig(lookup)
	if err != nil {
		return nil, nil, err
	}

	dispatcherCreds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: cfg.SigningKey,
		Issuer:     cfg.Issuer,
		Audience:   cfg.Audience,
		Role:       serviceauth.RoleAgent,
		ServiceID:  cfg.ServiceID,
	})
	if err != nil {
		return nil, nil, err
	}
	transportCreds, err := servicetls.ClientCredentials(servicetls.Config{
		Mode:       cfg.TLSMode,
		Secret:     cfg.TLSSecret,
		Role:       serviceauth.RoleAgent,
		ServiceID:  cfg.ServiceID,
		ServerName: cfg.TLSServerName,
	})
	if err != nil {
		return nil, nil, err
	}

	conn, err := grpc.Dial(
		cfg.Address,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithPerRPCCredentials(dispatcherCreds),
	)
	if err != nil {
		return nil, nil, err
	}
	return conn, proto.NewDispatcherServiceClient(conn), nil
}

func FinalizeRun(ctx context.Context, client proto.DispatcherServiceClient, runID, status string) error {
	if client == nil {
		return fmt.Errorf("dispatcher client is not configured")
	}
	reqCtx, cancel := context.WithTimeout(defaultContext(ctx), 10*time.Second)
	defer cancel()
	_, err := client.FinalizeRun(reqCtx, &proto.FinalizeRunRequest{
		RunId:  runID,
		Status: status,
	})
	return err
}

func ReportTaskStatus(ctx context.Context, client proto.DispatcherServiceClient, report TaskStatusReport) error {
	if client == nil {
		return fmt.Errorf("dispatcher client is not configured")
	}
	reqCtx, cancel := context.WithTimeout(defaultContext(ctx), 10*time.Second)
	defer cancel()
	_, err := client.ReportTaskStatus(reqCtx, &proto.TaskStatusReport{
		RunId:         report.RunID,
		StepName:      report.StepName,
		TaskName:      report.TaskName,
		Status:        report.Status,
		ExitCode:      int32(report.ExitCode),
		LlmDurationMs: report.LLMDurationMs,
	})
	return err
}

func TriggerPipeline(ctx context.Context, client proto.DispatcherServiceClient, req TriggerPipelineRequest) (string, error) {
	if client == nil {
		return "", fmt.Errorf("dispatcher client is not configured")
	}
	reqCtx, cancel := context.WithTimeout(defaultContext(ctx), 20*time.Second)
	defer cancel()
	resp, err := client.TriggerPipeline(reqCtx, &proto.TriggerPipelineRequest{
		ParentRunId:        req.ParentRunID,
		ParentRunnerId:     req.ParentRunnerID,
		ParentPipelineName: req.ParentPipelineName,
		ParentStepName:     req.ParentStepName,
		PipelineIdentifier: req.PipelineIdentifier,
		PipelineDefinition: req.PipelineDefinition,
		History:            req.History,
		Scope:              req.Scope,
		GitContext:         req.GitContext,
	})
	if err != nil {
		return "", fmt.Errorf("dispatcher trigger pipeline: %w", err)
	}
	if resp.GetError() != "" {
		return "", fmt.Errorf("dispatcher trigger pipeline: %s", resp.GetError())
	}
	if strings.TrimSpace(resp.GetRunId()) == "" {
		return "", fmt.Errorf("dispatcher returned empty run id for child pipeline")
	}
	return resp.GetRunId(), nil
}

func FetchPipeline(ctx context.Context, client proto.DispatcherServiceClient, req FetchPipelineRequest) ([]byte, error) {
	if client == nil {
		return nil, fmt.Errorf("dispatcher client is not configured")
	}
	reqCtx, cancel := context.WithTimeout(defaultContext(ctx), 15*time.Second)
	defer cancel()
	resp, err := client.FetchPipeline(reqCtx, &proto.FetchPipelineRequest{
		PipelineName: req.PipelineName,
		RepoOwner:    req.RepoOwner,
		RepoName:     req.RepoName,
		CommitSha:    req.CommitSHA,
	})
	if err != nil {
		return nil, fmt.Errorf("dispatcher fetch pipeline: %w", err)
	}
	if len(resp.GetPipelineDefinition()) == 0 {
		return nil, fmt.Errorf("dispatcher returned empty pipeline definition")
	}
	return resp.GetPipelineDefinition(), nil
}

func GetRunStatus(ctx context.Context, client proto.DispatcherServiceClient, runID string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("dispatcher client is not configured")
	}
	reqCtx, cancel := context.WithTimeout(defaultContext(ctx), 10*time.Second)
	defer cancel()
	resp, err := client.GetRunStatus(reqCtx, &proto.RunStatusRequest{RunId: runID})
	if err != nil {
		return "", err
	}
	return resp.GetStatus(), nil
}

func defaultContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}
