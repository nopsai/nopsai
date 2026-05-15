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
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"
	nopsaiAuth "nopsai/services/nopsai/pkg/auth"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	gproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

type runnerConn struct {
	connectionID  string
	id            string
	scopes        map[string]struct{}
	capacity      int32
	active        int32
	lastHeartbeat time.Time
	inflight      map[string]*proto.JobRequest
	sendCh        chan *proto.DispatcherMessage
	metadata      map[string]string
	allowDispatch bool
	cancel        context.CancelFunc
}

type dispatcherServer struct {
	proto.UnimplementedDispatcherServiceServer

	mu                 sync.Mutex
	runners            map[string]*runnerConn
	queue              []*proto.JobRequest
	routing            map[string][]string
	triggerAssignments map[string]string
	connSeq            uint64

	nopsaiBase          string
	httpClient          *http.Client
	internalTokenSigner *nopsaiAuth.LocalJWTService
}

func newDispatcherServer(routing map[string][]string, nopsaiBase string, internalTokenSigner ...*nopsaiAuth.LocalJWTService) *dispatcherServer {
	clean := make(map[string][]string, len(routing))
	for scope, ids := range routing {
		scopeKey := strings.TrimSpace(scope)
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		clean[scopeKey] = ids
	}
	var signer *nopsaiAuth.LocalJWTService
	if len(internalTokenSigner) > 0 {
		signer = internalTokenSigner[0]
	}
	return &dispatcherServer{
		runners:             make(map[string]*runnerConn),
		routing:             clean,
		triggerAssignments:  make(map[string]string),
		nopsaiBase:          strings.TrimRight(strings.TrimSpace(nopsaiBase), "/"),
		httpClient:          &http.Client{Timeout: 15 * time.Second},
		internalTokenSigner: signer,
	}
}

type dispatcherAuth struct {
	authenticator      *serviceauth.Authenticator
	allowedRoles       map[string]map[string]struct{}
	expectedServiceIDs map[string]string
}

func newDispatcherAuth(authenticator *serviceauth.Authenticator, expectedServiceIDs map[string]string) *dispatcherAuth {
	cleanExpectedIDs := make(map[string]string, len(expectedServiceIDs))
	for role, serviceID := range expectedServiceIDs {
		role = strings.ToLower(strings.TrimSpace(role))
		serviceID = strings.TrimSpace(serviceID)
		if role == "" || serviceID == "" {
			continue
		}
		cleanExpectedIDs[role] = serviceID
	}
	return &dispatcherAuth{
		authenticator:      authenticator,
		expectedServiceIDs: cleanExpectedIDs,
		allowedRoles: map[string]map[string]struct{}{
			"SubmitJob":            roleSet(serviceauth.RoleNopsai),
			"GetStatus":            roleSet(serviceauth.RoleNopsai),
			"UpdateRunnerDispatch": roleSet(serviceauth.RoleNopsai),
			"Register":             roleSet(serviceauth.RoleRunner),
			"IngestLogs":           roleSet(serviceauth.RoleRunner, serviceauth.RoleAgent),
			"ReportTaskStatus":     roleSet(serviceauth.RoleAgent),
			"FinalizeRun":          roleSet(serviceauth.RoleAgent),
			"FetchPipeline":        roleSet(serviceauth.RoleAgent),
			"TriggerPipeline":      roleSet(serviceauth.RoleAgent),
			"GetRunStatus":         roleSet(serviceauth.RoleRunner, serviceauth.RoleAgent),
		},
	}
}

func roleSet(roles ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		out[role] = struct{}{}
	}
	return out
}

func (a *dispatcherAuth) unaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	claims, err := a.authenticate(ctx, info.FullMethod)
	if err != nil {
		return nil, err
	}
	return handler(serviceauth.WithClaims(ctx, claims), req)
}

func (a *dispatcherAuth) streamInterceptor(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	claims, err := a.authenticate(stream.Context(), info.FullMethod)
	if err != nil {
		return err
	}
	return handler(srv, &authenticatedServerStream{
		ServerStream: stream,
		ctx:          serviceauth.WithClaims(stream.Context(), claims),
	})
}

func (a *dispatcherAuth) authenticate(ctx context.Context, fullMethod string) (*serviceauth.Claims, error) {
	if a == nil || a.authenticator == nil {
		return nil, status.Error(codes.Internal, "dispatcher auth is not configured")
	}
	method := grpcMethodName(fullMethod)
	roles := a.allowedRoles[method]
	if len(roles) == 0 {
		return nil, status.Error(codes.PermissionDenied, "dispatcher method is not available to service clients")
	}

	claims, err := a.authenticator.AuthenticateContext(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid service token")
	}
	if _, ok := roles[claims.ServiceRole()]; !ok {
		return nil, status.Error(codes.PermissionDenied, "service role is not allowed to call dispatcher method")
	}
	if expectedID := a.expectedServiceIDs[claims.ServiceRole()]; expectedID != "" && claims.ServiceID() != expectedID {
		return nil, status.Error(codes.PermissionDenied, "service identity is not allowed to call dispatcher method")
	}
	return claims, nil
}

func grpcMethodName(fullMethod string) string {
	fullMethod = strings.TrimSpace(fullMethod)
	if fullMethod == "" {
		return ""
	}
	idx := strings.LastIndex(fullMethod, "/")
	if idx == -1 || idx == len(fullMethod)-1 {
		return fullMethod
	}
	return fullMethod[idx+1:]
}

type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}

func (d *dispatcherServer) authorizeInternalRequest(ctx context.Context, req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is nil")
	}
	if d.internalTokenSigner == nil {
		return fmt.Errorf("internal token signer is not configured")
	}

	token, _, err := d.internalTokenSigner.MintAccessToken(ctx, nopsaiAuth.Claims{
		Sub:      "dispatcher",
		Provider: "internal-service",
	})
	if err != nil {
		return fmt.Errorf("mint dispatcher token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

func (d *dispatcherServer) nextConnectionID(base string) string {
	seq := atomic.AddUint64(&d.connSeq, 1)
	name := strings.TrimSpace(base)
	if name == "" {
		name = "runner"
	}
	return fmt.Sprintf("%s#%d", name, seq)
}

func (d *dispatcherServer) SubmitJob(ctx context.Context, job *proto.JobRequest) (*proto.SubmitJobResponse, error) {
	if job == nil || strings.TrimSpace(job.RunId) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}

	job.Scope = strings.TrimSpace(job.Scope)
	resp := &proto.SubmitJobResponse{RunId: job.RunId}

	runStatus, err := d.fetchRunStatusValue(ctx, job.RunId)
	if err != nil {
		log.Warn().Err(err).Str("run_id", job.RunId).Msg("Failed to verify run status before dispatch")
	} else if !runStatusAllowsDispatch(runStatus) {
		resp.State = proto.JobState_JOB_STATE_REJECTED
		resp.Message = fmt.Sprintf("run status %q is terminal", runStatus)
		log.Info().Str("run_id", job.RunId).Str("status", runStatus).Msg("Rejecting job submission for terminal run")
		return resp, nil
	}

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

	connectionID := d.nextConnectionID(reg.RunnerId)
	metadata := make(map[string]string, len(reg.Metadata)+2)
	for k, v := range reg.Metadata {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		metadata[key] = v
	}
	metadata["connection_id"] = connectionID
	metadata["connected_at"] = time.Now().UTC().Format(time.RFC3339)

	rc := &runnerConn{
		connectionID:  connectionID,
		id:            reg.RunnerId,
		scopes:        toSet(reg.Scopes),
		capacity:      capacity,
		active:        0,
		lastHeartbeat: time.Now(),
		inflight:      make(map[string]*proto.JobRequest),
		sendCh:        make(chan *proto.DispatcherMessage, 32),
		metadata:      metadata,
		allowDispatch: true,
	}

	d.addRunner(rc)
	defer d.removeRunner(rc.connectionID)

	ctx, cancel := context.WithCancel(stream.Context())
	rc.cancel = cancel
	defer cancel()
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
			d.handleHeartbeat(rc.connectionID, body.Heartbeat)
		case *proto.RunnerMessage_JobResult:
			d.handleJobResult(rc.connectionID, body.JobResult)
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
		if info := d.runnerInfoLocked(r); info != nil {
			resp.Runners = append(resp.Runners, info)
		}
	}

	return resp, nil
}

func (d *dispatcherServer) runnerInfoLocked(r *runnerConn) *proto.RunnerInfo {
	if r == nil {
		return nil
	}

	runSummaries := make([]map[string]string, 0, len(r.inflight))
	for runID, job := range r.inflight {
		entry := map[string]string{"run_id": runID}
		if job != nil {
			if name := strings.TrimSpace(job.PipelineName); name != "" {
				entry["pipeline"] = name
			}
			if trig := strings.TrimSpace(job.TriggerEventId); trig != "" {
				entry["trigger_event_id"] = trig
			}
		}
		runSummaries = append(runSummaries, entry)
	}
	sort.Slice(runSummaries, func(i, j int) bool {
		return runSummaries[i]["run_id"] < runSummaries[j]["run_id"]
	})

	var runSummariesJSON string
	if len(runSummaries) > 0 {
		if data, err := json.Marshal(runSummaries); err == nil {
			runSummariesJSON = string(data)
		}
	}

	meta := mergeMetadata(r.metadata, r.connectionID)
	if runSummariesJSON != "" {
		meta = cloneMetadata(meta)
		meta["active_runs"] = runSummariesJSON
	}

	return &proto.RunnerInfo{
		RunnerId:          r.id,
		Scopes:            keys(r.scopes),
		Capacity:          r.capacity,
		ActiveJobs:        r.active,
		InflightJobs:      int32(len(r.inflight)),
		LastHeartbeatUnix: r.lastHeartbeat.Unix(),
		Metadata:          meta,
		AllowDispatch:     r.allowDispatch,
	}
}

func (d *dispatcherServer) UpdateRunnerDispatch(ctx context.Context, req *proto.UpdateRunnerDispatchRequest) (*proto.UpdateRunnerDispatchResponse, error) {
	if req == nil || strings.TrimSpace(req.RunnerId) == "" {
		return nil, status.Error(codes.InvalidArgument, "runner_id is required")
	}

	runnerID := strings.TrimSpace(req.RunnerId)
	connectionID := strings.TrimSpace(req.ConnectionId)

	d.mu.Lock()
	var target *runnerConn
	if connectionID != "" {
		if r, ok := d.runners[connectionID]; ok && r.id == runnerID {
			target = r
		}
	}
	if target == nil {
		for _, r := range d.runners {
			if r.id == runnerID {
				target = r
				break
			}
		}
	}
	if target == nil {
		d.mu.Unlock()
		return nil, status.Error(codes.NotFound, "runner is not connected")
	}

	target.allowDispatch = req.AllowDispatch
	if req.AllowDispatch {
		// Avoid stale active counts blocking dispatch after a pause.
		if inflight := int32(len(target.inflight)); inflight < target.active {
			target.active = inflight
		}
	}
	if !req.AllowDispatch {
		d.dropTriggerAssignmentsForRunner(target.id)
	}

	info := d.runnerInfoLocked(target)
	d.mu.Unlock()

	log.Info().
		Str("runner_id", target.id).
		Str("connection_id", target.connectionID).
		Bool("allow_dispatch", target.allowDispatch).
		Msg("runner dispatch flag updated")

	go d.pumpQueue()

	return &proto.UpdateRunnerDispatchResponse{Runner: info}, nil
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
	if err := d.authorizeInternalRequest(ctx, req); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize log ingest request: %v", err)
	}

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
	if err := d.authorizeInternalRequest(ctx, httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize task status request: %v", err)
	}

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
	if err := d.authorizeInternalRequest(ctx, httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize finalize request: %v", err)
	}

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
	if err := d.authorizeInternalRequest(ctx, httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize pipeline request: %v", err)
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
	if err := d.authorizeInternalRequest(ctx, httpReq); err != nil {
		return nil, status.Errorf(codes.Internal, "authorize trigger request: %v", err)
	}

	if v := strings.TrimSpace(req.ParentRunId); v != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Run-ID", v)
	}
	if v := strings.TrimSpace(req.ParentRunnerId); v != "" {
		httpReq.Header.Set("X-Nopsai-Parent-Runner-ID", v)
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
	statusText, err := d.fetchRunStatusValue(ctx, req.RunId)
	if err != nil {
		return nil, err
	}

	return &proto.RunStatusResponse{Status: statusText}, nil
}

func (d *dispatcherServer) handleHeartbeat(connectionID string, hb *proto.RunnerHeartbeat) {
	if hb == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	runner, ok := d.runners[connectionID]
	if !ok {
		return
	}
	runner.lastHeartbeat = time.Now()
	runner.active = hb.ActiveJobs
	go d.pumpQueue()
}

func (d *dispatcherServer) handleJobResult(connectionID string, result *proto.JobResult) {
	if result == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	runner, ok := d.runners[connectionID]
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
			log.Info().Str("run_id", job.RunId).Str("runner_id", runner.id).Msg("job requeued by runner")
			d.queue = append(d.queue, job)
			go d.pumpQueue()
		}
	case "failed":
		log.Warn().Str("run_id", result.RunId).Str("runner_id", runner.id).Str("error", result.Error).Msg("job failed to start on runner")
	case "completed":
		log.Info().Str("run_id", result.RunId).Str("runner_id", runner.id).Msg("job completed")
	case "accepted":
		log.Info().Str("run_id", result.RunId).Str("runner_id", runner.id).Msg("runner acknowledged job")
	default:
		log.Info().Str("run_id", result.RunId).Str("runner_id", runner.id).Str("status", statusText).Msg("received job update from runner")
	}
}

func (d *dispatcherServer) addRunner(rc *runnerConn) {
	d.mu.Lock()
	d.runners[rc.connectionID] = rc
	log.Info().
		Str("runner_id", rc.id).
		Str("connection_id", rc.connectionID).
		Int("scopes", len(rc.scopes)).
		Int32("capacity", rc.capacity).
		Msg("runner connected")

	d.mu.Unlock()
}

func (d *dispatcherServer) removeRunner(connectionID string) {
	d.mu.Lock()
	runner, ok := d.runners[connectionID]
	if ok {
		delete(d.runners, connectionID)
		d.dropTriggerAssignmentsForRunner(runner.id)
	}
	d.mu.Unlock()

	if !ok {
		return
	}

	if runner.cancel != nil {
		runner.cancel()
	}

	close(runner.sendCh)
	log.Warn().
		Str("runner_id", runner.id).
		Str("connection_id", runner.connectionID).
		Msg("runner disconnected")
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

func affinityKeyForJob(job *proto.JobRequest) string {
	if job == nil {
		return ""
	}
	if key := strings.TrimSpace(job.RunnerAffinityKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(job.TriggerEventId); key != "" {
		return key
	}
	return strings.TrimSpace(job.RunId)
}

func (d *dispatcherServer) dispatch(job *proto.JobRequest) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	runner := d.pickRunnerForJobLocked(job)
	if runner == nil {
		return ""
	}

	jobForRunner := d.prepareJobForRunner(job, runner)
	affinityKey := affinityKeyForJob(job)
	preferredRunnerID := strings.TrimSpace(job.PreferredRunnerId)

	select {
	case runner.sendCh <- &proto.DispatcherMessage{Message: &proto.DispatcherMessage_Job{Job: jobForRunner}}:
		runner.inflight[job.RunId] = jobForRunner
		runner.active++
		log.Info().
			Str("run_id", job.RunId).
			Str("runner_id", runner.id).
			Str("pipeline", job.PipelineName).
			Str("scope", job.Scope).
			Str("trigger_event_id", job.TriggerEventId).
			Str("runner_affinity_key", affinityKey).
			Str("preferred_runner_id", preferredRunnerID).
			Msg("job dispatched to runner")
		if affinityKey != "" {
			d.triggerAssignments[affinityKey] = runner.id
		}
		d.recordAssignment(job.RunId, job.PipelineName, runner.id, job.Scope, job.TriggerEventId)
		return runner.id
	default:
		log.Warn().Str("runner_id", runner.id).Msg("runner send channel is full; queueing job")
		return ""
	}
}

func (d *dispatcherServer) pickRunnerForJobLocked(job *proto.JobRequest) *runnerConn {
	if job == nil {
		return nil
	}

	allowed := d.allowedRunnerIDs(job.Scope)
	preferredRunnerID := strings.TrimSpace(job.PreferredRunnerId)
	affinityKey := affinityKeyForJob(job)

	if preferredRunnerID != "" {
		if r := d.runnerByIDLocked(preferredRunnerID); r != nil {
			if d.runnerEligibleForJob(r, job.Scope, allowed) {
				return r
			}
			if d.runnerMatchesScope(r, job.Scope, allowed) && runnerLoad(r) >= r.capacity {
				log.Info().
					Str("run_id", job.RunId).
					Str("preferred_runner_id", preferredRunnerID).
					Msg("preferred runner at capacity; falling back to affinity/selection")
			} else {
				log.Info().
					Str("run_id", job.RunId).
					Str("preferred_runner_id", preferredRunnerID).
					Msg("preferred runner not eligible; falling back to affinity/selection")
			}
		} else {
			log.Info().
				Str("run_id", job.RunId).
				Str("preferred_runner_id", preferredRunnerID).
				Msg("preferred runner not connected; falling back to affinity/selection")
		}
	}

	if affinityKey != "" {
		if runnerID, ok := d.triggerAssignments[affinityKey]; ok {
			if r := d.runnerByIDLocked(runnerID); r != nil {
				if d.runnerEligibleForJob(r, job.Scope, allowed) {
					return r
				}
				if d.runnerMatchesScope(r, job.Scope, allowed) && runnerLoad(r) >= r.capacity {
					log.Info().
						Str("run_id", job.RunId).
						Str("runner_affinity_key", affinityKey).
						Str("runner_id", runnerID).
						Msg("affinity runner at capacity; falling back to selection")
				} else {
					delete(d.triggerAssignments, affinityKey)
				}
			} else {
				delete(d.triggerAssignments, affinityKey)
			}
		}
	}

	var candidates []*runnerConn
	var busyChoice *runnerConn
	for _, r := range d.runners {
		load := runnerLoad(r)
		if !d.runnerMatchesScope(r, job.Scope, allowed) {
			continue
		}
		if r.allowDispatch && load < r.capacity {
			candidates = append(candidates, r)
		} else if r.allowDispatch {
			if busyChoice == nil || load < runnerLoad(busyChoice) {
				busyChoice = r
			}
		}
	}

	if len(candidates) == 0 {
		if affinityKey != "" && busyChoice != nil {
			d.triggerAssignments[affinityKey] = busyChoice.id
		}
		return nil
	}

	selected := candidates[0]
	for _, r := range candidates[1:] {
		if runnerLoad(r) < runnerLoad(selected) {
			selected = r
		}
	}
	if affinityKey != "" {
		d.triggerAssignments[affinityKey] = selected.id
	}
	return selected
}

func (d *dispatcherServer) runnerMatchesScope(r *runnerConn, scope string, allowed []string) bool {
	if r == nil {
		return false
	}
	if len(allowed) > 0 && !contains(allowed, r.id) {
		return false
	}
	if scope != "" && !r.hasScope(scope) {
		return false
	}
	return r.allowDispatch
}

func (d *dispatcherServer) runnerEligibleForJob(r *runnerConn, scope string, allowed []string) bool {
	if !d.runnerMatchesScope(r, scope, allowed) {
		return false
	}
	return runnerLoad(r) < r.capacity
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

// runnerByIDLocked returns the runner connection matching the given runner ID.
// Caller must hold d.mu.
func (d *dispatcherServer) runnerByIDLocked(runnerID string) *runnerConn {
	if runnerID == "" {
		return nil
	}
	for _, r := range d.runners {
		if r.id == runnerID {
			return r
		}
	}
	return nil
}

func (d *dispatcherServer) dropTriggerAssignmentsForRunner(runnerID string) {
	if runnerID == "" {
		return
	}
	for triggerID, assigned := range d.triggerAssignments {
		if assigned == runnerID {
			delete(d.triggerAssignments, triggerID)
		}
	}
}

func (d *dispatcherServer) pumpQueue() {
	d.mu.Lock()
	if len(d.queue) == 0 {
		d.mu.Unlock()
		return
	}
	queuedJobs := append([]*proto.JobRequest(nil), d.queue...)
	d.queue = nil
	d.mu.Unlock()

	var assignments []struct {
		runID        string
		pipelineName string
		runnerID     string
		scope        string
		triggerID    string
	}
	for _, job := range queuedJobs {
		dispatchCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		runStatus, err := d.fetchRunStatusValue(dispatchCtx, job.RunId)
		cancel()
		if err != nil {
			log.Warn().Err(err).Str("run_id", job.RunId).Msg("Failed to verify queued run status before dispatch")
		} else if !runStatusAllowsDispatch(runStatus) {
			log.Info().Str("run_id", job.RunId).Str("status", runStatus).Msg("Dropping queued job for terminal run")
			continue
		}

		d.mu.Lock()
		runner := d.pickRunnerForJobLocked(job)
		if runner == nil {
			d.queue = append(d.queue, job)
			d.mu.Unlock()
			continue
		}
		jobForRunner := d.prepareJobForRunner(job, runner)
		if jobForRunner == nil {
			log.Error().Str("run_id", job.RunId).Str("runner_id", runner.id).Msg("failed to prepare queued job for runner")
			d.queue = append(d.queue, job)
			d.mu.Unlock()
			continue
		}
		affinityKey := affinityKeyForJob(job)
		preferredRunnerID := strings.TrimSpace(job.PreferredRunnerId)
		select {
		case runner.sendCh <- &proto.DispatcherMessage{Message: &proto.DispatcherMessage_Job{Job: jobForRunner}}:
			runner.inflight[job.RunId] = jobForRunner
			runner.active++
			log.Info().
				Str("run_id", job.RunId).
				Str("runner_id", runner.id).
				Str("pipeline", job.PipelineName).
				Str("scope", job.Scope).
				Str("trigger_event_id", job.TriggerEventId).
				Str("runner_affinity_key", affinityKey).
				Str("preferred_runner_id", preferredRunnerID).
				Msg("job dispatched from queue")
			if affinityKey != "" {
				d.triggerAssignments[affinityKey] = runner.id
			}
			assignments = append(assignments, struct {
				runID        string
				pipelineName string
				runnerID     string
				scope        string
				triggerID    string
			}{
				runID:        job.RunId,
				pipelineName: job.PipelineName,
				runnerID:     runner.id,
				scope:        job.Scope,
				triggerID:    job.TriggerEventId,
			})
			d.mu.Unlock()
		default:
			log.Warn().Str("runner_id", runner.id).Msg("runner send channel is full during queue dispatch")
			d.queue = append(d.queue, job)
			d.mu.Unlock()
		}
	}

	// Send assignment log entries without holding the dispatcher lock.
	for _, a := range assignments {
		d.recordAssignment(a.runID, a.pipelineName, a.runnerID, a.scope, a.triggerID)
	}
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

func mergeMetadata(meta map[string]string, connectionID string) map[string]string {
	if len(meta) == 0 && connectionID == "" {
		return nil
	}
	out := cloneMetadata(meta)
	if connectionID != "" {
		out["connection_id"] = connectionID
	}
	return out
}

func runnerLoad(r *runnerConn) int32 {
	if r == nil {
		return 1 << 30
	}
	load := r.active
	if inflight := int32(len(r.inflight)); inflight > load {
		load = inflight
	}
	return load
}

func cloneMetadata(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
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
			var stale []struct {
				connectionID string
				runnerID     string
			}

			d.mu.Lock()
			for connID, runner := range d.runners {
				if now.Sub(runner.lastHeartbeat) > ttl {
					stale = append(stale, struct {
						connectionID string
						runnerID     string
					}{connectionID: connID, runnerID: runner.id})
				}
			}
			d.mu.Unlock()

			for _, entry := range stale {
				log.Warn().
					Str("runner_id", entry.runnerID).
					Str("connection_id", entry.connectionID).
					Msg("runner considered stale; removing")
				d.removeRunner(entry.connectionID)
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

func runStatusAllowsDispatch(statusText string) bool {
	switch strings.ToLower(strings.TrimSpace(statusText)) {
	case "", "pending", "running":
		return true
	case "success", "failure", "cancelled", "timed_out":
		return false
	default:
		return true
	}
}

func (d *dispatcherServer) fetchRunStatusValue(ctx context.Context, runID string) (string, error) {
	if err := d.requireNopsaiBase(); err != nil {
		return "", err
	}

	target := fmt.Sprintf("%s/v1/runs/%s/status", d.nopsaiBase, strings.TrimSpace(runID))
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", status.Errorf(codes.Internal, "build run status request: %v", err)
	}
	if err := d.authorizeInternalRequest(ctx, httpReq); err != nil {
		return "", status.Errorf(codes.Internal, "authorize run status request: %v", err)
	}

	resp, err := d.httpClient.Do(httpReq)
	if err != nil {
		return "", status.Errorf(codes.Unavailable, "fetch run status: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", status.Errorf(codes.FailedPrecondition, "nopsai run status returned %d: %s", resp.StatusCode, string(body))
	}

	var statusResp map[string]string
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return "", status.Errorf(codes.Internal, "decode run status response: %v", err)
	}

	return statusResp["status"], nil
}

func (d *dispatcherServer) prepareJobForRunner(job *proto.JobRequest, runner *runnerConn) *proto.JobRequest {
	if job == nil {
		return nil
	}
	copyJob, ok := gproto.Clone(job).(*proto.JobRequest)
	if !ok || copyJob == nil {
		return nil
	}
	runtimeVars := append([]string(nil), copyJob.Env...)
	if runner != nil {
		if override, ok := runner.metadata["docker_network"]; ok {
			copyJob.DockerNetwork = strings.TrimSpace(override)
			runtimeVars = upsertRuntimeVar(runtimeVars, "DOCKER_NETWORK_NAME", copyJob.DockerNetwork)
		}
		if addr, ok := runner.metadata["dispatcher_addr"]; ok {
			runtimeVars = upsertRuntimeVar(runtimeVars, "DISPATCHER_ADDRESS", strings.TrimSpace(addr))
		}
		runtimeVars = upsertRuntimeVar(runtimeVars, "RUNNER_ID", runner.id)
	}
	copyJob.Env = runtimeVars
	return copyJob
}

func upsertRuntimeVar(runtimeVars []string, key, val string) []string {
	prefix := key + "="
	for i, e := range runtimeVars {
		if strings.HasPrefix(e, prefix) {
			runtimeVars[i] = prefix + val
			return runtimeVars
		}
	}
	return append(runtimeVars, prefix+val)
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
	lowerKey := strings.ToLower(key)

	// Preserve trigger event ID with the header name expected by nopsai
	if lowerKey == "git_trigger_event_id" || lowerKey == "trigger_event_id" {
		return "X-Nopsai-Trigger-Event-ID"
	}

	parts := strings.Split(lowerKey, "_")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return "X-" + strings.Join(parts, "-")
}

func (d *dispatcherServer) recordAssignment(runID, pipelineName, runnerID, scope, triggerID string) {
	runID = strings.TrimSpace(runID)
	runnerID = strings.TrimSpace(runnerID)
	if runID == "" || runnerID == "" {
		return
	}

	scope = strings.TrimSpace(scope)
	pipelineName = strings.TrimSpace(pipelineName)
	triggerID = strings.TrimSpace(triggerID)

	msg := fmt.Sprintf("Assigned run %s to runner %s", runID, runnerID)
	if pipelineName != "" {
		msg = fmt.Sprintf("Assigned pipeline %s (run %s) to runner %s", pipelineName, runID, runnerID)
	}
	if scope != "" {
		msg = fmt.Sprintf("%s (scope %s)", msg, scope)
	}
	if triggerID != "" {
		msg = fmt.Sprintf("%s [trigger %s]", msg, triggerID)
	}

	go func(runID, line string) {
		if _, err := d.IngestLogs(context.Background(), &proto.LogBatch{
			RunId: runID,
			Lines: []string{line},
		}); err != nil {
			log.Warn().Err(err).Str("run_id", runID).Msg("failed to record runner assignment")
		}
	}(runID, msg)
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

	internalTokenSigner := nopsaiAuth.NewLocalJWTService(
		[]byte(strings.TrimSpace(cfg.JWTSigningKey)),
		strings.TrimSpace(cfg.JWTIssuer),
		strings.TrimSpace(cfg.JWTAudience),
		5*time.Minute,
	)
	dispatcher := newDispatcherServer(cfg.DispatcherRouting, nopsaiBase, internalTokenSigner)
	serviceAuthenticator, err := serviceauth.NewAuthenticator(serviceauth.Config{
		SigningKey: cfg.EffectiveServiceJWTSigningKey(),
		Issuer:     cfg.EffectiveServiceJWTIssuer(),
		Audience:   cfg.EffectiveServiceJWTAudience(),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher service authentication")
	}
	dispatcherAuth := newDispatcherAuth(serviceAuthenticator, map[string]string{
		serviceauth.RoleNopsai: cfg.EffectiveNopsaiServiceID(),
		serviceauth.RoleRunner: cfg.EffectiveRunnerServiceID(),
		serviceauth.RoleAgent:  cfg.EffectiveAgentServiceID(),
	})

	dispatcherTransportCreds, err := servicetls.ServerCredentials(servicetls.Config{
		Mode:        cfg.EffectiveDispatcherTLSMode(),
		Secret:      cfg.EffectiveDispatcherTLSSecret(),
		ServerName:  cfg.EffectiveDispatcherTLSServerName(),
		ServerNames: dispatcherTLSServerNames(cfg, listenAddr),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure dispatcher transport security")
	}

	serverOptions := []grpc.ServerOption{
		grpc.UnaryInterceptor(dispatcherAuth.unaryInterceptor),
		grpc.StreamInterceptor(dispatcherAuth.streamInterceptor),
	}
	if dispatcherTransportCreds != nil {
		serverOptions = append(serverOptions, grpc.Creds(dispatcherTransportCreds))
	}
	grpcServer := grpc.NewServer(serverOptions...)
	proto.RegisterDispatcherServiceServer(grpcServer, dispatcher)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal().Err(err).Str("addr", listenAddr).Msg("failed to listen")
	}

	stop := make(chan struct{})
	go dispatcher.reapStaleRunners(10*time.Second, 30*time.Second, stop)

	log.Info().
		Str("addr", listenAddr).
		Str("tls_mode", servicetls.NormalizeMode(cfg.EffectiveDispatcherTLSMode())).
		Msg("dispatcher listening")
	if err := grpcServer.Serve(lis); err != nil {
		close(stop)
		log.Fatal().Err(err).Msg("dispatcher server failed")
	}
}

func dispatcherTLSServerNames(cfg *config.Config, listenAddr string) []string {
	names := []string{"dispatcher", "localhost", listenAddr}
	if cfg != nil {
		names = append(names,
			cfg.EffectiveDispatcherTLSServerName(),
			cfg.DispatcherAddress,
			cfg.DispatcherListenAddress,
		)
	}
	if hostname, err := os.Hostname(); err == nil {
		names = append(names, hostname)
	}
	return names
}
