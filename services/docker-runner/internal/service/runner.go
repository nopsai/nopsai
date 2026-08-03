package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/pkg/dockerimage"
	"nopsai/pkg/dockervolume"
	"nopsai/pkg/logforward"
	"nopsai/pkg/proto"
	"nopsai/pkg/registryauth"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicelog"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	defaultAgentImage           = "nopsai-agent:latest"
	defaultDispatcherAddr       = "localhost:9090"
	defaultRunnerID             = "runner"
	dockerRuntimeName           = "docker"
	dockerRunVolumeManagedLabel = "nopsai.io/managed"
	dockerRunVolumePurposeLabel = "nopsai.io/volume-purpose"
	dockerRunVolumeOwnerLabel   = "nopsai.io/run-id"
	dockerRunVolumePurpose      = "pipeline-workspace"
)

type Runner interface {
	ServeForever()
}

type RunnerOptions struct {
	RunnerID                 string
	RunnerScopes             string
	Capacity                 int32
	DispatcherAddr           string
	DispatcherCreds          *serviceauth.Credentials
	TransportCreds           credentials.TransportCredentials
	Docker                   *client.Client
	DockerNetwork            string
	DockerNetworkSet         bool
	RegistryAuth             RegistryAuthResolver
	RegistryAuthConfigBase64 string
}

type RegistryAuthResolver = dockerimage.RegistryAuthResolver

type dockerRunner struct {
	id                       string
	scopes                   []string
	capacity                 int32
	dispatcherAddr           string
	dispatcherCreds          *serviceauth.Credentials
	transportCreds           credentials.TransportCredentials
	docker                   *client.Client
	active                   atomic.Int32
	dockerNetwork            string
	networkSet               bool
	registryAuth             RegistryAuthResolver
	registryAuthConfigBase64 string
	stopMu                   sync.Mutex
	stoppedRuns              map[string]struct{}
}

func NewDockerRunner(options RunnerOptions) Runner {
	runnerID := strings.TrimSpace(options.RunnerID)
	if runnerID == "" {
		runnerID = defaultRunnerID
	}
	dispatcherAddr := strings.TrimSpace(options.DispatcherAddr)
	if dispatcherAddr == "" {
		dispatcherAddr = defaultDispatcherAddr
	}
	capacity := options.Capacity
	if capacity <= 0 {
		capacity = 1
	}

	return &dockerRunner{
		id:                       runnerID,
		scopes:                   parseScopes(options.RunnerScopes),
		capacity:                 capacity,
		dispatcherAddr:           dispatcherAddr,
		dispatcherCreds:          options.DispatcherCreds,
		transportCreds:           options.TransportCreds,
		docker:                   options.Docker,
		dockerNetwork:            strings.TrimSpace(options.DockerNetwork),
		networkSet:               options.DockerNetworkSet,
		registryAuth:             options.RegistryAuth,
		registryAuthConfigBase64: strings.TrimSpace(options.RegistryAuthConfigBase64),
		stoppedRuns:              make(map[string]struct{}),
	}
}

func (r *dockerRunner) ServeForever() {
	log.Info().
		Str("runner_id", r.id).
		Str("runtime", dockerRuntimeName).
		Str("dispatcher_addr", r.dispatcherAddr).
		Strs("scopes", r.scopes).
		Int("capacity", int(r.capacity)).
		Msg("runner starting")

	for {
		if err := r.connectAndServe(); err != nil {
			log.Error().Err(err).Msg("dispatcher stream ended, retrying")
			time.Sleep(3 * time.Second)
		}
	}
}

func (r *dockerRunner) connectAndServe() error {
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(r.transportCreds),
		grpc.WithBlock(),
		grpc.WithChainUnaryInterceptor(servicelog.GRPCUnaryClientInterceptor()),
		grpc.WithChainStreamInterceptor(servicelog.GRPCStreamClientInterceptor()),
	}
	if r.dispatcherCreds != nil {
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(r.dispatcherCreds))
	}
	dialCtx, dialCancel := context.WithTimeout(context.Background(), 20*time.Second)
	conn, err := grpc.DialContext(dialCtx, r.dispatcherAddr, dialOptions...)
	dialCancel()
	if err != nil {
		return fmt.Errorf("failed to dial dispatcher: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dispatcherClient := proto.NewDispatcherServiceClient(conn)
	stream, err := dispatcherClient.Register(ctx)
	if err != nil {
		return fmt.Errorf("failed to open register stream: %w", err)
	}

	sendCh := make(chan *proto.RunnerMessage, 64)
	sendErrCh := make(chan error, 1)
	go func() {
		for {
			select {
			case msg, ok := <-sendCh:
				if !ok {
					return
				}
				if err := stream.Send(msg); err != nil {
					sendErrCh <- err
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	if !sendRunnerMessage(ctx, sendCh, &proto.RunnerMessage{
		Message: &proto.RunnerMessage_Register{
			Register: &proto.RunnerRegistration{
				RunnerId: r.id,
				Scopes:   r.scopes,
				Capacity: r.capacity,
				Metadata: r.registrationMetadata(),
			},
		},
	}, runnerMessageBlock) {
		return ctx.Err()
	}

	go r.heartbeatLoop(ctx, sendCh)

	for {
		select {
		case err := <-sendErrCh:
			return err
		default:
		}

		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		switch body := msg.Message.(type) {
		case *proto.DispatcherMessage_Job:
			if body.Job != nil {
				go r.handleJob(ctx, dispatcherClient, body.Job, sendCh)
			}
		case *proto.DispatcherMessage_Note:
			log.Info().Str("note", body.Note).Msg("dispatcher message")
		default:
			log.Warn().Msg("received unknown message from dispatcher")
		}
	}
}

func (r *dockerRunner) registrationMetadata() map[string]string {
	metadata := map[string]string{
		"version":         "v1",
		"runtime":         dockerRuntimeName,
		"dispatcher_addr": r.dispatcherAddr,
	}
	if host, err := os.Hostname(); err == nil {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			metadata["hostname"] = trimmed
		}
	}
	if r.networkSet {
		metadata["docker_network"] = r.dockerNetwork
	}
	return metadata
}

func (r *dockerRunner) heartbeatLoop(ctx context.Context, sendCh chan<- *proto.RunnerMessage) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !sendRunnerMessage(ctx, sendCh, &proto.RunnerMessage{
				Message: &proto.RunnerMessage_Heartbeat{
					Heartbeat: &proto.RunnerHeartbeat{
						RunnerId:   r.id,
						ActiveJobs: r.active.Load(),
					},
				},
			}, runnerMessageDropIfFull) {
				log.Debug().Str("runner_id", r.id).Msg("dropped heartbeat because runner send buffer is full")
			}
		}
	}
}

func (r *dockerRunner) handleJob(ctx context.Context, dispatcher proto.DispatcherServiceClient, job *proto.JobRequest, sendCh chan<- *proto.RunnerMessage) {
	if job == nil {
		return
	}

	r.active.Add(1)
	defer r.active.Add(-1)
	defer r.clearRunStopRequested(job.RunId)
	sendJobResult(ctx, sendCh, job.RunId, "accepted", "")

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	agentImage := job.AgentImage
	if strings.TrimSpace(agentImage) == "" {
		agentImage = defaultAgentImage
	}

	runtimeVars := append([]string(nil), job.Env...)
	if strings.TrimSpace(r.dispatcherAddr) != "" {
		runtimeVars = upsertRuntimeVar(runtimeVars, "DISPATCHER_GRPC_ADDRESS", strings.TrimSpace(r.dispatcherAddr))
	}
	if r.networkSet {
		runtimeVars = upsertRuntimeVar(runtimeVars, "DOCKER_NETWORK_NAME", strings.TrimSpace(job.DockerNetwork))
	}
	if strings.TrimSpace(r.registryAuthConfigBase64) != "" {
		runtimeVars = upsertRuntimeVar(runtimeVars, registryauth.DockerConfigBase64Env, r.registryAuthConfigBase64)
	}

	if err := ensureImageExists(runCtx, r.docker, agentImage, r.registryAuth); err != nil {
		sendJobResult(ctx, sendCh, job.RunId, "failed", err.Error())
		return
	}

	sharedVolume := job.SharedVolumeName
	if strings.TrimSpace(sharedVolume) == "" {
		sharedVolume = fmt.Sprintf("vol-%s", job.RunId)
	}

	if err := ensureManagedRunVolume(runCtx, r.docker, sharedVolume, job.RunId); err != nil {
		sendJobResult(ctx, sendCh, job.RunId, "failed", fmt.Sprintf("create volume: %v", err))
		return
	}
	defer r.removeRunVolume(ctx, sharedVolume)

	hostConfig := &container.HostConfig{
		Binds: []string{
			"/var/run/docker.sock:/var/run/docker.sock",
			fmt.Sprintf("%s:/workspace", sharedVolume),
		},
		AutoRemove: job.AutoRemove,
	}

	networking := &network.NetworkingConfig{}
	if name := strings.TrimSpace(job.DockerNetwork); name != "" {
		networking.EndpointsConfig = map[string]*network.EndpointSettings{
			name: {},
		}
	}

	containerName := job.ContainerName
	if strings.TrimSpace(containerName) == "" {
		containerName = fmt.Sprintf("agent-%s", job.RunId)
	}

	resp, err := r.docker.ContainerCreate(runCtx, client.ContainerCreateOptions{
		Config: &container.Config{
			Image: agentImage,
			Env:   runtimeVars,
		},
		HostConfig:       hostConfig,
		NetworkingConfig: networking,
		Name:             containerName,
	})
	if err != nil {
		sendJobResult(ctx, sendCh, job.RunId, "failed", fmt.Sprintf("container create: %v", err))
		return
	}

	if _, err := r.docker.ContainerStart(runCtx, resp.ID, client.ContainerStartOptions{}); err != nil {
		r.removeCreatedContainer(ctx, resp.ID)
		sendJobResult(ctx, sendCh, job.RunId, "failed", fmt.Sprintf("container start: %v", err))
		return
	}

	log.Info().Str("run_id", job.RunId).Str("container_id", resp.ID).Msg("started agent container")

	go r.monitorRunCancellation(runCtx, dispatcher, job.RunId, resp.ID)
	go r.streamLogs(runCtx, dispatcher, job, resp.ID)

	waitResult := r.docker.ContainerWait(runCtx, resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			sendJobResult(ctx, sendCh, job.RunId, "failed", fmt.Sprintf("container wait: %v", err))
			return
		}
	case status := <-waitResult.Result:
		if status.StatusCode != 0 {
			sendJobResult(ctx, sendCh, job.RunId, "failed", fmt.Sprintf("exit code %d", status.StatusCode))
			return
		}
	case <-runCtx.Done():
		r.stopContainer(ctx, resp.ID)
		sendJobResult(ctx, sendCh, job.RunId, "failed", runCtx.Err().Error())
		return
	}

	sendJobResult(ctx, sendCh, job.RunId, "completed", "")
}

func (r *dockerRunner) monitorRunCancellation(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, containerID string) {
	if dispatcher == nil || strings.TrimSpace(runID) == "" || strings.TrimSpace(containerID) == "" {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			resp, err := dispatcher.GetRunStatus(reqCtx, &proto.RunStatusRequest{RunId: runID})
			cancel()
			if err != nil {
				log.Warn().Err(err).Str("run_id", runID).Msg("failed to poll run status for cancellation")
				continue
			}

			if strings.EqualFold(strings.TrimSpace(resp.GetStatus()), "cancelled") {
				if !r.markRunStopRequested(runID) {
					return
				}
				log.Warn().Str("run_id", runID).Str("container_id", containerID).Msg("run cancelled; stopping agent container")
				r.stopContainer(ctx, containerID)
				return
			}
		}
	}
}

func (r *dockerRunner) markRunStopRequested(runID string) bool {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()

	if _, exists := r.stoppedRuns[runID]; exists {
		return false
	}
	r.stoppedRuns[runID] = struct{}{}
	return true
}

func (r *dockerRunner) clearRunStopRequested(runID string) {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()

	delete(r.stoppedRuns, runID)
}

func (r *dockerRunner) stopContainer(ctx context.Context, containerID string) {
	if r == nil || r.docker == nil || strings.TrimSpace(containerID) == "" {
		return
	}

	ctx, cancel := dockerCleanupContext(ctx, 15*time.Second)
	defer cancel()

	timeout := 1
	if _, err := r.docker.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		log.Warn().Err(err).Str("container_id", containerID).Msg("failed to stop agent container after cancellation")
	}
}

func (r *dockerRunner) removeCreatedContainer(ctx context.Context, containerID string) {
	if r == nil || r.docker == nil || strings.TrimSpace(containerID) == "" {
		return
	}
	cleanupCtx, cancel := dockerCleanupContext(ctx, 30*time.Second)
	defer cancel()
	if _, err := r.docker.ContainerRemove(cleanupCtx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
		log.Warn().Err(err).Str("container_id", containerID).Msg("failed to remove agent container after start failure")
	}
}

func (r *dockerRunner) removeRunVolume(ctx context.Context, volumeName string) {
	if r == nil || r.docker == nil || strings.TrimSpace(volumeName) == "" {
		return
	}
	cleanupCtx, cancel := dockerCleanupContext(ctx, 30*time.Second)
	defer cancel()
	if _, err := r.docker.VolumeRemove(cleanupCtx, volumeName, client.VolumeRemoveOptions{Force: true}); err != nil {
		log.Warn().Err(err).Str("volume", volumeName).Msg("failed to remove managed run volume")
	}
}

func (r *dockerRunner) streamLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, job *proto.JobRequest, containerID string) {
	if dispatcher == nil {
		log.Warn().Str("run_id", job.RunId).Msg("dispatcher client not available; skipping log forwarding")
		return
	}

	logReader, err := r.docker.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Timestamps: true})
	if err != nil {
		log.Error().Err(err).Str("run_id", job.RunId).Msg("failed to attach to container logs")
		return
	}
	defer logReader.Close()

	rPipe, wPipe := io.Pipe()
	defer rPipe.Close()
	go func() {
		defer wPipe.Close()
		_, err := stdcopy.StdCopy(wPipe, wPipe, logReader)
		if err != nil {
			log.Error().Err(err).Str("run_id", job.RunId).Msg("demultiplex log stream failed")
		}
	}()

	logforward.Forward(ctx, rPipe, func(sendCtx context.Context, lines []string) {
		r.flushLogs(sendCtx, dispatcher, job.RunId, lines)
	}, logforward.Options{
		OnScannerError: func(err error) {
			log.Error().Err(err).Str("run_id", job.RunId).Msg("log scanner error")
		},
	})
}

func (r *dockerRunner) flushLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID string, lines []string) {
	if len(lines) == 0 {
		return
	}

	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := dispatcher.IngestLogs(sendCtx, &proto.LogBatch{
		RunId: runID,
		Lines: lines,
	})
	if err != nil {
		log.Error().Err(err).Str("run_id", runID).Msg("failed to send log batch to dispatcher")
	}
}

type runnerMessageOverflowPolicy int

const (
	runnerMessageBlock runnerMessageOverflowPolicy = iota
	runnerMessageDropIfFull
)

func sendJobResult(ctx context.Context, sendCh chan<- *proto.RunnerMessage, runID, status, errMsg string) {
	msg := &proto.RunnerMessage{
		Message: &proto.RunnerMessage_JobResult{
			JobResult: &proto.JobResult{
				RunId:  runID,
				Status: status,
				Error:  errMsg,
			},
		},
	}
	if !sendRunnerMessage(ctx, sendCh, msg, runnerMessageBlock) {
		log.Warn().Str("run_id", runID).Str("status", status).Msg("runner context ended before job status could be reported")
	}
}

func sendRunnerMessage(ctx context.Context, sendCh chan<- *proto.RunnerMessage, msg *proto.RunnerMessage, policy runnerMessageOverflowPolicy) bool {
	if sendCh == nil || msg == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if policy == runnerMessageDropIfFull {
		select {
		case sendCh <- msg:
			return true
		case <-ctx.Done():
			return false
		default:
			return false
		}
	}
	select {
	case sendCh <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

func parseScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var scopes []string
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			scopes = append(scopes, v)
		}
	}
	return scopes
}

func ensureImageExists(ctx context.Context, cli *client.Client, imageName string, authResolver RegistryAuthResolver) error {
	result, err := dockerimage.EnsureExists(ctx, cli, imageName, authResolver)
	if err != nil {
		return err
	}
	switch {
	case result.FoundLocal:
		log.Info().Str("image", imageName).Msg("image found locally")
	case result.Pulled && result.UsedRegistryAuth:
		log.Info().Str("image", imageName).Msg("pulled image with local Docker registry auth")
	case result.Pulled:
		log.Info().Str("image", imageName).Msg("pulled image")
	}
	return nil
}

func ensureManagedRunVolume(ctx context.Context, cli *client.Client, volumeName, runID string) error {
	volumeName = strings.TrimSpace(volumeName)
	runID = strings.TrimSpace(runID)
	if volumeName == "" || runID == "" {
		return fmt.Errorf("volume name and run id are required")
	}
	return dockervolume.EnsureManaged(ctx, cli, dockervolume.ManagedSpec{
		Name:              volumeName,
		Labels:            dockerRunVolumeLabels(runID),
		ValidateOwnership: func(labels map[string]string) bool { return dockerRunVolumeOwnedBy(labels, runID) },
		OwnerDescription:  "run",
	})
}

func dockerRunVolumeLabels(runID string) map[string]string {
	return map[string]string{
		dockerRunVolumeManagedLabel: "true",
		dockerRunVolumePurposeLabel: dockerRunVolumePurpose,
		dockerRunVolumeOwnerLabel:   strings.TrimSpace(runID),
	}
}

func dockerRunVolumeOwnedBy(labels map[string]string, runID string) bool {
	return strings.EqualFold(strings.TrimSpace(labels[dockerRunVolumeManagedLabel]), "true") &&
		strings.TrimSpace(labels[dockerRunVolumePurposeLabel]) == dockerRunVolumePurpose &&
		strings.TrimSpace(labels[dockerRunVolumeOwnerLabel]) == strings.TrimSpace(runID)
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

func dockerCleanupContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
