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

	"nopsai/pkg/logforward"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	defaultAgentImage     = "nopsai-agent:latest"
	defaultDispatcherAddr = "localhost:9090"
	defaultRunnerID       = "runner"
	dockerRuntimeName     = "docker"
)

type Runner interface {
	ServeForever()
}

type RunnerOptions struct {
	RunnerID         string
	RunnerScopes     string
	Capacity         int32
	DispatcherAddr   string
	DispatcherCreds  *serviceauth.Credentials
	TransportCreds   credentials.TransportCredentials
	Docker           *client.Client
	DockerNetwork    string
	DockerNetworkSet bool
}

type dockerRunner struct {
	id              string
	scopes          []string
	capacity        int32
	dispatcherAddr  string
	dispatcherCreds *serviceauth.Credentials
	transportCreds  credentials.TransportCredentials
	docker          *client.Client
	active          atomic.Int32
	dockerNetwork   string
	networkSet      bool
	stopMu          sync.Mutex
	stoppedRuns     map[string]struct{}
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
		id:              runnerID,
		scopes:          parseScopes(options.RunnerScopes),
		capacity:        capacity,
		dispatcherAddr:  dispatcherAddr,
		dispatcherCreds: options.DispatcherCreds,
		transportCreds:  options.TransportCreds,
		docker:          options.Docker,
		dockerNetwork:   strings.TrimSpace(options.DockerNetwork),
		networkSet:      options.DockerNetworkSet,
		stoppedRuns:     make(map[string]struct{}),
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

	sendCh <- &proto.RunnerMessage{
		Message: &proto.RunnerMessage_Register{
			Register: &proto.RunnerRegistration{
				RunnerId: r.id,
				Scopes:   r.scopes,
				Capacity: r.capacity,
				Metadata: r.registrationMetadata(),
			},
		},
	}

	hbStop := make(chan struct{})
	defer close(hbStop)
	go r.heartbeatLoop(sendCh, hbStop)

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
				go r.handleJob(context.Background(), dispatcherClient, body.Job, sendCh)
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

func (r *dockerRunner) heartbeatLoop(sendCh chan<- *proto.RunnerMessage, stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			sendCh <- &proto.RunnerMessage{
				Message: &proto.RunnerMessage_Heartbeat{
					Heartbeat: &proto.RunnerHeartbeat{
						RunnerId:   r.id,
						ActiveJobs: r.active.Load(),
					},
				},
			}
		}
	}
}

func (r *dockerRunner) handleJob(ctx context.Context, dispatcher proto.DispatcherServiceClient, job *proto.JobRequest, sendCh chan<- *proto.RunnerMessage) {
	if job == nil {
		return
	}

	r.active.Add(1)
	sendJobResult(sendCh, job.RunId, "accepted", "")

	go func() {
		defer r.active.Add(-1)
		defer r.clearRunStopRequested(job.RunId)

		agentImage := job.AgentImage
		if strings.TrimSpace(agentImage) == "" {
			agentImage = defaultAgentImage
		}

		runtimeVars := append([]string(nil), job.Env...)
		if strings.TrimSpace(r.dispatcherAddr) != "" {
			runtimeVars = upsertRuntimeVar(runtimeVars, "DISPATCHER_ADDRESS", strings.TrimSpace(r.dispatcherAddr))
		}
		if r.networkSet {
			runtimeVars = upsertRuntimeVar(runtimeVars, "DOCKER_NETWORK_NAME", strings.TrimSpace(job.DockerNetwork))
		}

		runCtx := context.Background()
		if err := ensureImageExists(runCtx, r.docker, agentImage); err != nil {
			sendJobResult(sendCh, job.RunId, "failed", err.Error())
			return
		}

		sharedVolume := job.SharedVolumeName
		if strings.TrimSpace(sharedVolume) == "" {
			sharedVolume = fmt.Sprintf("vol-%s", job.RunId)
		}

		_, err := r.docker.VolumeCreate(runCtx, client.VolumeCreateOptions{Name: sharedVolume})
		if err != nil {
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("create volume: %v", err))
			return
		}
		defer r.docker.VolumeRemove(context.Background(), sharedVolume, client.VolumeRemoveOptions{Force: true})

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
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("container create: %v", err))
			return
		}

		if _, err := r.docker.ContainerStart(runCtx, resp.ID, client.ContainerStartOptions{}); err != nil {
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("container start: %v", err))
			return
		}

		log.Info().Str("run_id", job.RunId).Str("container_id", resp.ID).Msg("started agent container")

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go r.monitorRunCancellation(runCtx, dispatcher, job.RunId, resp.ID)
		go r.streamLogs(runCtx, dispatcher, job, resp.ID)

		waitResult := r.docker.ContainerWait(context.Background(), resp.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
		select {
		case err := <-waitResult.Error:
			if err != nil {
				sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("container wait: %v", err))
				return
			}
		case status := <-waitResult.Result:
			if status.StatusCode != 0 {
				sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("exit code %d", status.StatusCode))
				return
			}
		}

		sendJobResult(sendCh, job.RunId, "completed", "")
	}()
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
			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
				r.stopContainer(containerID)
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

func (r *dockerRunner) stopContainer(containerID string) {
	if r == nil || r.docker == nil || strings.TrimSpace(containerID) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	timeout := 1
	if _, err := r.docker.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		log.Warn().Err(err).Str("container_id", containerID).Msg("failed to stop agent container after cancellation")
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

func sendJobResult(sendCh chan<- *proto.RunnerMessage, runID, status, errMsg string) {
	msg := &proto.RunnerMessage{
		Message: &proto.RunnerMessage_JobResult{
			JobResult: &proto.JobResult{
				RunId:  runID,
				Status: status,
				Error:  errMsg,
			},
		},
	}
	select {
	case sendCh <- msg:
	default:
		log.Warn().Str("run_id", runID).Msg("send buffer full while reporting job status")
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

func ensureImageExists(ctx context.Context, cli *client.Client, imageName string) error {
	imageFilters := make(client.Filters).Add("reference", imageName)
	images, err := cli.ImageList(ctx, client.ImageListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	if len(images.Items) == 0 {
		log.Info().Msgf("image %s not found locally, pulling", imageName)
		out, err := cli.ImagePull(ctx, imageName, client.ImagePullOptions{})
		if err != nil {
			return fmt.Errorf("pull image: %w", err)
		}
		defer out.Close()
		io.Copy(io.Discard, out)
	}
	return nil
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
