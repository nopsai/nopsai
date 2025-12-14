package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type runnerConn struct {
	id            string
	scopes        map[string]struct{}
	capacity      int32
	active        int32
	lastHeartbeat time.Time
	inflight      map[string]*proto.JobRequest
	sendCh        chan *proto.DispatcherMessage
	metadata      map[string]string
	allowDispatch bool
}

type dispatcherServer struct {
	proto.UnimplementedDispatcherServiceServer

	mu      sync.Mutex
	runners map[string]*runnerConn
	queue   []*proto.JobRequest
	routing map[string][]string

	nopsaiBase string
	httpClient *http.Client
}

func newDispatcherServer(routing map[string][]string, nopsaiBase string) *dispatcherServer {
	clean := make(map[string][]string, len(routing))
	for scope, ids := range routing {
		scopeKey := strings.TrimSpace(scope)
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		clean[scopeKey] = ids
	}
	return &dispatcherServer{
		runners:    make(map[string]*runnerConn),
		routing:    clean,
		nopsaiBase: strings.TrimRight(strings.TrimSpace(nopsaiBase), "/"),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (d *dispatcherServer) SubmitJob(ctx context.Context, job *proto.JobRequest) (*proto.SubmitJobResponse, error) {
	if job == nil || strings.TrimSpace(job.RunId) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}

	job.Scope = strings.TrimSpace(job.Scope)
	resp := &proto.SubmitJobResponse{RunId: job.RunId}

	runnerID := d.dispatch(job)
	if runnerID != "" {
		resp.State = proto.JobState_JOB_STATE_ASSIGNED
		resp.RunnerId = runnerID
		resp.Message = "dispatched"
		return resp, nil
	}

	d.enqueue(job)
	resp.State = proto.JobState_JOB_STATE_QUEUED
	resp.Message = "queued"
	return resp, nil
}

func (d *dispatcherServer) Register(stream proto.DispatcherService_RegisterServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	reg := first.GetRegister()
	if reg == nil || strings.TrimSpace(reg.RunnerId) == "" {
		return status.Error(codes.InvalidArgument, "first message must be runner registration")
	}

	capacity := reg.Capacity
	if capacity <= 0 {
		capacity = 1
	}
	rc := &runnerConn{
		id:            reg.RunnerId,
		scopes:        toSet(reg.Scopes),
		capacity:      capacity,
		active:        0,
		lastHeartbeat: time.Now(),
		inflight:      make(map[string]*proto.JobRequest),
		sendCh:        make(chan *proto.DispatcherMessage, 32),
		metadata:      reg.Metadata,
		allowDispatch: true,
	}

	d.addRunner(rc)
	defer d.removeRunner(rc.id)

	ctx, cancel := context.WithCancel(context.Background())
	sendErrCh := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-rc.sendCh:
				if !ok {
					return
				}
				if err := stream.Send(msg); err != nil {
					sendErrCh <- err
					return
				}
			}
		}
	}()

	rc.send(&proto.DispatcherMessage{Message: &proto.DispatcherMessage_Note{Note: "registered"}})
	d.pumpQueue()

	for {
		select {
		case err := <-sendErrCh:
			cancel()
			return err
		default:
		}
		msg, err := stream.Recv()
		if err != nil {
			cancel()
			return err
		}
		switch body := msg.Message.(type) {
		case *proto.RunnerMessage_Heartbeat:
			d.handleHeartbeat(rc.id, body.Heartbeat)
		case *proto.RunnerMessage_JobResult:
			d.handleJobResult(rc.id, body.JobResult)
		case *proto.RunnerMessage_Register:
			// Ignore duplicate registration attempts on the same stream.
			log.Warn().Str("runner_id", rc.id).Msg("received duplicate registration message on stream")
		default:
			log.Warn().Str("runner_id", rc.id).Msg("received unknown runner message")
		}
	}
}

func (d *dispatcherServer) GetStatus(ctx context.Context, _ *emptypb.Empty) (*proto.DispatcherStatus, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	resp := &proto.DispatcherStatus{
		QueuedJobs: int32(len(d.queue)),
	}

	for _, r := range d.runners {
		info := &proto.RunnerInfo{
			RunnerId:          r.id,
			Scopes:            keys(r.scopes),
			Capacity:          r.capacity,
			ActiveJobs:        r.active,
			InflightJobs:      int32(len(r.inflight)),
			LastHeartbeatUnix: r.lastHeartbeat.Unix(),
			Metadata:          r.metadata,
			AllowDispatch:     r.allowDispatch,
		}
		resp.Runners = append(resp.Runners, info)
	}

	return resp, nil
}

func (d *dispatcherServer) IngestLogs(ctx context.Context, batch *proto.LogBatch) (*emptypb.Empty, error) {
	if batch == nil || strings.TrimSpace(batch.RunId) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	if err := d.requireNopsaiBase(); err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string][]string{"lines": batch.Lines})
	target := fmt.Sprintf("%s/v1/runs/%s/logs/ingest", d.nopsaiBase, strings.TrimSpace(batch.RunId))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build log ingest request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "send log ingest: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, status.Errorf(codes.FailedPrecondition, "nopsai log ingest returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return &emptypb.Empty{}, nil
}

func (d *dispatcherServer) ReportTaskStatus(ctx context.Context, req *proto.TaskStatusReport) (*emptypb.Empty, error) {
	if req == nil || strings.TrimSpace(req.RunId) == "" || strings.TrimSpace(req.StepName) == "" || strings.TrimSpace(req.TaskName) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id, step_name, and task_name are required")
	}
	if err := d.requireNopsaiBase(); err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"status":          req.Status,
		"exit_code":       req.ExitCode,
		"llm_duration_ms": req.LlmDurationMs,
	}
	body, _ := json.Marshal(payload)

	target := fmt.Sprintf("%s/v1/runs/%s/steps/%s/tasks/%s", d.nopsaiBase, strings.TrimSpace(req.RunId), url.PathEscape(req.StepName), url.PathEscape(req.TaskName))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build task status request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "send task status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, status.Errorf(codes.FailedPrecondition, "nopsai task status returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return &emptypb.Empty{}, nil
}

func (d *dispatcherServer) FinalizeRun(ctx context.Context, req *proto.FinalizeRunRequest) (*emptypb.Empty, error) {
	if req == nil || strings.TrimSpace(req.RunId) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	if err := d.requireNopsaiBase(); err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string]string{"status": req.Status})
	target := fmt.Sprintf("%s/v1/runs/%s/finalize", d.nopsaiBase, strings.TrimSpace(req.RunId))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build finalize request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "send finalize: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, status.Errorf(codes.FailedPrecondition, "nopsai finalize returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return &emptypb.Empty{}, nil
}

func (d *dispatcherServer) FetchPipeline(ctx context.Context, req *proto.FetchPipelineRequest) (*proto.FetchPipelineResponse, error) {
	if req == nil || strings.TrimSpace(req.PipelineName) == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline_name is required")
	}
	if err := d.requireNopsaiBase(); err != nil {
		return nil, err
	}

	base := d.nopsaiBase
	target := fmt.Sprintf("%s/v1/pipelines/%s", base, strings.TrimLeft(req.PipelineName, "/"))
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "parse pipeline url: %v", err)
	}
	q := parsed.Query()
	if strings.TrimSpace(req.RepoOwner) != "" {
		q.Set("repoOwner", req.RepoOwner)
	}
	if strings.TrimSpace(req.RepoName) != "" {
		q.Set("repoName", req.RepoName)
	}
	if strings.TrimSpace(req.CommitSha) != "" {
		q.Set("commitSHA", req.CommitSha)
	}
	parsed.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build pipeline request: %v", err)
	}

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "fetch pipeline: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.FailedPrecondition, "nopsai pipeline returned %d: %s", resp.StatusCode, string(body))
	}

	return &proto.FetchPipelineResponse{PipelineDefinition: body}, nil
}

func (d *dispatcherServer) TriggerPipeline(ctx context.Context, req *proto.TriggerPipelineRequest) (*proto.TriggerPipelineResponse, error) {
	if req == nil || len(req.PipelineDefinition) == 0 {
		return nil, status.Error(codes.InvalidArgument, "pipeline_definition is required")
	}
	if err := d.requireNopsaiBase(); err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s/v1/run", d.nopsaiBase)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(req.PipelineDefinition))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build trigger request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-yaml")

	if v := strings.TrimSpace(req.ParentRunId); v != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Run-ID", v)
	}
	if v := strings.TrimSpace(req.ParentPipelineName); v != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Pipeline-Name", v)
	}
	if v := strings.TrimSpace(req.ParentStepName); v != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Step-Name", v)
	}

	if path := pipelinePath(strings.TrimSpace(req.PipelineIdentifier)); path != "" {
		httpReq.Header.Set("X-Nopsai-Pipeline-Path", path)
	}

	if h := strings.TrimSpace(req.History); h != "" {
		httpReq.Header.Set("X-Nopsai-Parent-History", base64.StdEncoding.EncodeToString([]byte(h)))
	}

	if scope := strings.TrimSpace(req.Scope); scope != "" {
		httpReq.Header.Set("X-Nopsai-Scope", scope)
	}

	for key, value := range req.GitContext {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		headerKey := gitHeaderKey(key)
		httpReq.Header.Set(headerKey, value)
	}

	resp, err := d.httpClient.Do(httpReq)
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

	return &proto.TriggerPipelineResponse{
		RunId:  runID,
		Status: "created",
	}, nil
}

func (d *dispatcherServer) GetRunStatus(ctx context.Context, req *proto.RunStatusRequest) (*proto.RunStatusResponse, error) {
	if req == nil || strings.TrimSpace(req.RunId) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	if err := d.requireNopsaiBase(); err != nil {
		return nil, err
	}

	target := fmt.Sprintf("%s/v1/runs/%s/status", d.nopsaiBase, strings.TrimSpace(req.RunId))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "build run status request: %v", err)
	}

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "fetch run status: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, status.Errorf(codes.FailedPrecondition, "nopsai run status returned %d: %s", resp.StatusCode, string(body))
	}

	var statusResp map[string]string
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, status.Errorf(codes.Internal, "decode run status response: %v", err)
	}

	return &proto.RunStatusResponse{Status: statusResp["status"]}, nil
}

func (d *dispatcherServer) handleHeartbeat(runnerID string, hb *proto.RunnerHeartbeat) {
	if hb == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	runner, ok := d.runners[runnerID]
	if !ok {
		return
	}
	runner.lastHeartbeat = time.Now()
	runner.active = hb.ActiveJobs
	runner.allowDispatch = true
	go d.pumpQueue()
}

func (d *dispatcherServer) handleJobResult(runnerID string, result *proto.JobResult) {
	if result == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	runner, ok := d.runners[runnerID]
	if !ok {
		return
	}

	statusText := strings.ToLower(strings.TrimSpace(result.Status))
	job, tracked := runner.inflight[result.RunId]
	if tracked && statusText != "accepted" {
		delete(runner.inflight, result.RunId)
		if runner.active > 0 && (statusText == "failed" || statusText == "completed" || statusText == "requeued") {
			runner.active--
		}
	}

	switch statusText {
	case "requeued":
		if job != nil {
			log.Info().Str("run_id", job.RunId).Str("runner_id", runnerID).Msg("job requeued by runner")
			d.queue = append(d.queue, job)
			go d.pumpQueue()
		}
	case "failed":
		log.Warn().Str("run_id", result.RunId).Str("runner_id", runnerID).Str("error", result.Error).Msg("job failed to start on runner")
	case "completed":
		log.Info().Str("run_id", result.RunId).Str("runner_id", runnerID).Msg("job completed")
	case "accepted":
		log.Info().Str("run_id", result.RunId).Str("runner_id", runnerID).Msg("runner acknowledged job")
	default:
		log.Info().Str("run_id", result.RunId).Str("runner_id", runnerID).Str("status", statusText).Msg("received job update from runner")
	}
}

func (d *dispatcherServer) addRunner(rc *runnerConn) {
	var toRequeue *runnerConn

	d.mu.Lock()

	if existing, ok := d.runners[rc.id]; ok {
		log.Warn().Str("runner_id", rc.id).Msg("replacing existing runner connection")
		delete(d.runners, rc.id)
		close(existing.sendCh)
		toRequeue = existing
	}

	d.runners[rc.id] = rc
	log.Info().
		Str("runner_id", rc.id).
		Int("scopes", len(rc.scopes)).
		Int32("capacity", rc.capacity).
		Msg("runner connected")

	d.mu.Unlock()

	if toRequeue != nil {
		go d.requeueInflight(toRequeue)
	}
}

func (d *dispatcherServer) removeRunner(runnerID string) {
	d.mu.Lock()
	runner, ok := d.runners[runnerID]
	if ok {
		delete(d.runners, runnerID)
	}
	d.mu.Unlock()

	if !ok {
		return
	}

	close(runner.sendCh)
	log.Warn().Str("runner_id", runnerID).Msg("runner disconnected")
	d.requeueInflight(runner)
}

func (d *dispatcherServer) requeueInflight(runner *runnerConn) {
	if runner == nil {
		return
	}
	if len(runner.inflight) == 0 {
		return
	}

	d.mu.Lock()
	for _, job := range runner.inflight {
		log.Warn().Str("run_id", job.RunId).Str("runner_id", runner.id).Msg("requeuing inflight job after runner disconnect")
		d.queue = append(d.queue, job)
	}
	runner.inflight = make(map[string]*proto.JobRequest)
	d.mu.Unlock()

	go d.pumpQueue()
}

func (d *dispatcherServer) enqueue(job *proto.JobRequest) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queue = append(d.queue, job)
	log.Info().Str("run_id", job.RunId).Str("scope", job.Scope).Msg("job queued")
}

func (d *dispatcherServer) dispatch(job *proto.JobRequest) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	runner := d.pickRunnerLocked(job.Scope)
	if runner == nil {
		return ""
	}

	jobForRunner := d.prepareJobForRunner(job, runner)

	select {
	case runner.sendCh <- &proto.DispatcherMessage{Message: &proto.DispatcherMessage_Job{Job: jobForRunner}}:
		runner.inflight[job.RunId] = jobForRunner
		runner.active++
		log.Info().Str("run_id", job.RunId).Str("runner_id", runner.id).Str("scope", job.Scope).Msg("job dispatched to runner")
		return runner.id
	default:
		log.Warn().Str("runner_id", runner.id).Msg("runner send channel is full; queueing job")
		return ""
	}
}

func (d *dispatcherServer) pickRunnerLocked(scope string) *runnerConn {
	var candidates []*runnerConn
	allowed := d.allowedRunnerIDs(scope)

	for _, r := range d.runners {
		if len(allowed) > 0 && !contains(allowed, r.id) {
			continue
		}
		if scope != "" && !r.hasScope(scope) {
			continue
		}
		if r.active >= r.capacity {
			continue
		}
		if !r.allowDispatch {
			continue
		}
		candidates = append(candidates, r)
	}

	if len(candidates) == 0 {
		return nil
	}

	selected := candidates[0]
	for _, r := range candidates[1:] {
		if r.active < selected.active {
			selected = r
		}
	}
	return selected
}

func (d *dispatcherServer) allowedRunnerIDs(scope string) []string {
	if len(d.routing) == 0 {
		return nil
	}
	var ids []string
	if runners, ok := d.routing[scope]; ok {
		ids = append(ids, runners...)
	}
	if runners, ok := d.routing["*"]; ok {
		ids = append(ids, runners...)
	}
	return ids
}

func (d *dispatcherServer) pumpQueue() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.queue) == 0 {
		return
	}

	var remaining []*proto.JobRequest
	for _, job := range d.queue {
		runner := d.pickRunnerLocked(job.Scope)
		if runner == nil {
			remaining = append(remaining, job)
			continue
		}
		jobForRunner := d.prepareJobForRunner(job, runner)
		select {
		case runner.sendCh <- &proto.DispatcherMessage{Message: &proto.DispatcherMessage_Job{Job: jobForRunner}}:
			runner.inflight[job.RunId] = jobForRunner
			runner.active++
			log.Info().Str("run_id", job.RunId).Str("runner_id", runner.id).Msg("job dispatched from queue")
		default:
			log.Warn().Str("runner_id", runner.id).Msg("runner send channel is full during queue dispatch")
			remaining = append(remaining, job)
		}
	}
	d.queue = remaining
}

func (r *runnerConn) hasScope(scope string) bool {
	if scope == "" {
		return true
	}
	if len(r.scopes) == 0 {
		return true
	}
	_, ok := r.scopes[strings.ToLower(scope)]
	return ok
}

func toSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, v := range values {
		trimmed := strings.ToLower(strings.TrimSpace(v))
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}
	return result
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

func keys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (r *runnerConn) send(msg *proto.DispatcherMessage) {
	select {
	case r.sendCh <- msg:
	default:
		log.Warn().Str("runner_id", r.id).Msg("runner send buffer full, dropping message")
	}
}

func (d *dispatcherServer) reapStaleRunners(interval, ttl time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			var stale []string

			d.mu.Lock()
			for id, runner := range d.runners {
				if now.Sub(runner.lastHeartbeat) > ttl {
					stale = append(stale, id)
				}
			}
			d.mu.Unlock()

			for _, id := range stale {
				log.Warn().Str("runner_id", id).Msg("runner considered stale; removing")
				d.removeRunner(id)
			}
		}
	}
}

func (d *dispatcherServer) requireNopsaiBase() error {
	if d.nopsaiBase == "" {
		return status.Error(codes.FailedPrecondition, "nopsai api url is not configured on dispatcher")
	}
	return nil
}

func (d *dispatcherServer) prepareJobForRunner(job *proto.JobRequest, runner *runnerConn) *proto.JobRequest {
	if job == nil {
		return nil
	}
	copyJob := *job
	envCopy := append([]string(nil), job.Env...)
	if runner != nil {
		if override, ok := runner.metadata["docker_network"]; ok {
			copyJob.DockerNetwork = strings.TrimSpace(override)
			envCopy = upsertEnv(envCopy, "DOCKER_NETWORK_NAME", copyJob.DockerNetwork)
		}
		if addr, ok := runner.metadata["dispatcher_addr"]; ok {
			envCopy = upsertEnv(envCopy, "DISPATCHER_ADDRESS", strings.TrimSpace(addr))
		}
	}
	copyJob.Env = envCopy
	return &copyJob
}

func upsertEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

func pipelinePath(identifier string) string {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".yaml") {
		trimmed = trimmed[:len(trimmed)-len(".yaml")]
	} else if strings.HasSuffix(lower, ".yml") {
		trimmed = trimmed[:len(trimmed)-len(".yml")]
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) <= 1 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], "/")
}

func gitHeaderKey(envKey string) string {
	key := strings.TrimSpace(envKey)
	key = strings.TrimPrefix(key, "X-")
	parts := strings.Split(strings.ToLower(key), "_")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return "X-" + strings.Join(parts, "-")
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)

	listenAddr := cfg.DispatcherListenAddress
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = ":9090"
	}

	nopsaiBase := strings.TrimSpace(cfg.AgentNopsaiAPIURL)
	if nopsaiBase == "" {
		log.Fatal().Msg("Agent Nopsai API URL (agent_nopsai_api_url) must be configured for dispatcher")
	}

	dispatcher := newDispatcherServer(cfg.DispatcherRouting, nopsaiBase)

	grpcServer := grpc.NewServer()
	proto.RegisterDispatcherServiceServer(grpcServer, dispatcher)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", listenAddr).Msg("failed to listen")
	}

	stop := make(chan struct{})
	go dispatcher.reapStaleRunners(10*time.Second, 30*time.Second, stop)

	log.Info().Str("addr", listenAddr).Msg("dispatcher listening")
	if err := grpcServer.Serve(lis); err != nil {
		close(stop)
		log.Fatal().Err(err).Msg("dispatcher server failed")
	}
}
