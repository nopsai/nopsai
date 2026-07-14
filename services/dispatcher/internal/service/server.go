package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"

	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	registeredRunners  map[string]*runnerRecord
	queue              []*proto.JobRequest
	routing            map[string][]string
	triggerAssignments map[string]string
	connSeq            uint64

	nopsai nopsaiClient
}

func newDispatcherServer(routing map[string][]string, nopsaiBase string, internalCredentials ...*serviceauth.Credentials) *dispatcherServer {
	var credentials *serviceauth.Credentials
	if len(internalCredentials) > 0 {
		credentials = internalCredentials[0]
	}
	return &dispatcherServer{
		runners:            make(map[string]*runnerConn),
		registeredRunners:  make(map[string]*runnerRecord),
		routing:            normalizeDispatcherRouting(routing),
		triggerAssignments: make(map[string]string),
		nopsai:             newNopsaiHTTPClient(nopsaiBase, credentials),
	}
}

func NewDispatcherServer(routing map[string][]string, nopsaiBase string, internalCredentials ...*serviceauth.Credentials) *dispatcherServer {
	return newDispatcherServer(routing, nopsaiBase, internalCredentials...)
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

func NewDispatcherAuth(authenticator *serviceauth.Authenticator, expectedServiceIDs map[string]string) *dispatcherAuth {
	return newDispatcherAuth(authenticator, expectedServiceIDs)
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

func (a *dispatcherAuth) UnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	return a.unaryInterceptor(ctx, req, info, handler)
}

func (a *dispatcherAuth) unaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	claims, err := a.authenticate(ctx, info.FullMethod)
	if err != nil {
		return nil, err
	}
	return handler(serviceauth.WithClaims(ctx, claims), req)
}

func (a *dispatcherAuth) StreamInterceptor(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return a.streamInterceptor(srv, stream, info, handler)
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
		resp.Message = fmt.Sprintf("run status %q is not dispatchable", runStatus)
		log.Info().Str("run_id", job.RunId).Str("status", runStatus).Msg("Rejecting job submission for non-dispatchable run")
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

	for _, r := range d.registeredRunnerRecordsLocked() {
		if info := runnerInfoFromRecord(r); info != nil {
			resp.Runners = append(resp.Runners, info)
		}
	}

	return resp, nil
}

func (d *dispatcherServer) runnerInfoLocked(r *runnerConn) *proto.RunnerInfo {
	return runnerInfoFromRecord(connectedRunnerRecord(r))
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
		record := d.registeredRunners[runnerID]
		if record == nil {
			d.mu.Unlock()
			return nil, status.Error(codes.NotFound, "runner is not registered")
		}
		record.allowDispatch = req.AllowDispatch
		info := runnerInfoFromRecord(record)
		d.mu.Unlock()
		return &proto.UpdateRunnerDispatchResponse{Runner: info}, nil
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

	record := d.recordRunnerSnapshotLocked(target, true, time.Time{})
	info := runnerInfoFromRecord(record)
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
	if err := d.nopsai.IngestLogs(ctx, batch.RunId, batch.Lines); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (d *dispatcherServer) ReportTaskStatus(ctx context.Context, req *proto.TaskStatusReport) (*emptypb.Empty, error) {
	if req == nil || strings.TrimSpace(req.RunId) == "" || strings.TrimSpace(req.StepName) == "" || strings.TrimSpace(req.TaskName) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id, step_name, and task_name are required")
	}
	if err := d.nopsai.ReportTaskStatus(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (d *dispatcherServer) FinalizeRun(ctx context.Context, req *proto.FinalizeRunRequest) (*emptypb.Empty, error) {
	if req == nil || strings.TrimSpace(req.RunId) == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	if err := d.nopsai.FinalizeRun(ctx, req.RunId, req.Status, ""); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (d *dispatcherServer) FetchPipeline(ctx context.Context, req *proto.FetchPipelineRequest) (*proto.FetchPipelineResponse, error) {
	if req == nil || strings.TrimSpace(req.PipelineName) == "" {
		return nil, status.Error(codes.InvalidArgument, "pipeline_name is required")
	}
	body, err := d.nopsai.FetchPipeline(ctx, req)
	if err != nil {
		return nil, err
	}
	return &proto.FetchPipelineResponse{PipelineDefinition: body}, nil
}

func (d *dispatcherServer) TriggerPipeline(ctx context.Context, req *proto.TriggerPipelineRequest) (*proto.TriggerPipelineResponse, error) {
	if req == nil || len(req.PipelineDefinition) == 0 {
		return nil, status.Error(codes.InvalidArgument, "pipeline_definition is required")
	}
	return d.nopsai.TriggerPipeline(ctx, req)
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
	d.recordRunnerSnapshotLocked(runner, true, time.Time{})
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
		log.Warn().Str("run_id", result.RunId).Str("runner_id", runner.id).Str("error", result.Error).Msg("job failed on runner")
		go d.reportRunnerJobFailure(result.RunId, result.Error)
	case "completed":
		log.Info().Str("run_id", result.RunId).Str("runner_id", runner.id).Msg("job completed")
	case "accepted":
		log.Info().Str("run_id", result.RunId).Str("runner_id", runner.id).Msg("runner acknowledged job")
	default:
		log.Info().Str("run_id", result.RunId).Str("runner_id", runner.id).Str("status", statusText).Msg("received job update from runner")
	}
	d.recordRunnerSnapshotLocked(runner, true, time.Time{})
}

func (d *dispatcherServer) reportRunnerJobFailure(runID, detail string) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return
	}
	reason := "Runner job failed"
	if detail = strings.TrimSpace(detail); detail != "" {
		reason += ": " + detail
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := d.nopsai.IngestLogs(ctx, runID, []string{reason}); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("failed to record runner job failure log")
	}
	if err := d.nopsai.FinalizeRun(ctx, runID, "failure", reason); err != nil {
		log.Warn().Err(err).Str("run_id", runID).Msg("failed to finalize runner job failure")
	}
}

func (d *dispatcherServer) addRunner(rc *runnerConn) {
	d.mu.Lock()
	if existing := d.registeredRunners[rc.id]; existing != nil {
		rc.allowDispatch = existing.allowDispatch
	}
	d.runners[rc.connectionID] = rc
	d.recordRunnerSnapshotLocked(rc, true, time.Time{})
	log.Info().
		Str("runner_id", rc.id).
		Str("connection_id", rc.connectionID).
		Int("scopes", len(rc.scopes)).
		Int32("capacity", rc.capacity).
		Msg("runner connected")

	d.mu.Unlock()
}

func (d *dispatcherServer) removeRunner(connectionID string) {
	var inflightJobs []*proto.JobRequest
	d.mu.Lock()
	runner, ok := d.runners[connectionID]
	if ok {
		delete(d.runners, connectionID)
		d.dropTriggerAssignmentsForRunner(runner.id)
		for _, job := range runner.inflight {
			if job == nil {
				continue
			}
			inflightJobs = append(inflightJobs, job)
			log.Warn().Str("run_id", job.RunId).Str("runner_id", runner.id).Msg("requeuing inflight job after runner disconnect")
		}
		runner.inflight = make(map[string]*proto.JobRequest)
		runner.active = 0
		d.queue = append(d.queue, inflightJobs...)
		d.markRunnerUnreachableLocked(runner, time.Now())
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
	if len(inflightJobs) > 0 {
		go d.pumpQueue()
	}
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

func (d *dispatcherServer) ReapStaleRunners(interval, ttl time.Duration, stop <-chan struct{}) {
	d.reapStaleRunners(interval, ttl, stop)
}

func (d *dispatcherServer) fetchRunStatusValue(ctx context.Context, runID string) (string, error) {
	return d.nopsai.RunStatus(ctx, runID)
}

func (d *dispatcherServer) syncRoutingFromNopsai(ctx context.Context) error {
	routing, err := d.nopsai.DispatcherRouting(ctx)
	if err != nil {
		return err
	}
	d.updateRouting(routing)
	return nil
}

func (d *dispatcherServer) syncRoutingLoop(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		return
	}

	syncOnce := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.syncRoutingFromNopsai(ctx); err != nil {
			log.Debug().Err(err).Msg("dispatcher routing sync skipped")
		}
	}

	syncOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			syncOnce()
		}
	}
}

func (d *dispatcherServer) SyncRoutingLoop(interval time.Duration, stop <-chan struct{}) {
	d.syncRoutingLoop(interval, stop)
}
