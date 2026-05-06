package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nopsai/config"
	"nopsai/pkg/proto"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type runner struct {
	id             string
	scopes         []string
	capacity       int32
	dispatcherAddr string
	docker         *client.Client
	active         atomic.Int32
	dockerNetwork  string
	networkSet     bool
	stopMu         sync.Mutex
	stoppedRuns    map[string]struct{}
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

	dispatcherAddr := cfg.DispatcherAddress
	if dispatcherAddr == "" {
		dispatcherAddr = "localhost:9090"
	}

	runnerID := cfg.RunnerID
	if runnerID == "" {
		if host, err := os.Hostname(); err == nil {
			runnerID = host
		} else {
			runnerID = "runner"
		}
	}

	scopes := parseScopes(cfg.RunnerScopes)
	capacity := int32(cfg.RunnerCapacity)
	if capacity <= 0 {
		capacity = 1
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create docker client")
	}
	defer dockerClient.Close()

	networkValue := strings.TrimSpace(cfg.DockerNetworkName)
	networkSet := false
	if envVal, ok := os.LookupEnv("DOCKER_NETWORK_NAME"); ok {
		networkValue = envVal
		networkSet = true
	} else if networkValue != "" {
		networkSet = true
	}

	r := &runner{
		id:             runnerID,
		scopes:         scopes,
		capacity:       capacity,
		dispatcherAddr: dispatcherAddr,
		docker:         dockerClient,
		dockerNetwork:  networkValue,
		networkSet:     networkSet,
		stoppedRuns:    make(map[string]struct{}),
	}

	for {
		if err := r.connectAndServe(); err != nil {
			log.Error().Err(err).Msg("dispatcher stream ended, retrying")
			time.Sleep(3 * time.Second)
		}
	}
}

func (r *runner) connectAndServe() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := grpc.DialContext(ctx, r.dispatcherAddr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return fmt.Errorf("failed to dial dispatcher: %w", err)
	}
	defer conn.Close()

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

	reg := &proto.RunnerRegistration{
		RunnerId: r.id,
		Scopes:   r.scopes,
		Capacity: r.capacity,
	}
	metadata := map[string]string{"version": "v1"}
	if host, err := os.Hostname(); err == nil {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			metadata["hostname"] = trimmed
		}
	}
	if r.networkSet {
		metadata["docker_network"] = r.dockerNetwork
	}
	if strings.TrimSpace(r.dispatcherAddr) != "" {
		metadata["dispatcher_addr"] = r.dispatcherAddr
	}
	reg.Metadata = metadata
	sendCh <- &proto.RunnerMessage{Message: &proto.RunnerMessage_Register{Register: reg}}

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

func (r *runner) heartbeatLoop(sendCh chan<- *proto.RunnerMessage, stop <-chan struct{}) {
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

func (r *runner) handleJob(ctx context.Context, dispatcher proto.DispatcherServiceClient, job *proto.JobRequest, sendCh chan<- *proto.RunnerMessage) {
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
			agentImage = "nopsai-agent:latest"
		}

		envVars := append([]string(nil), job.Env...)
		if strings.TrimSpace(r.dispatcherAddr) != "" {
			envVars = upsertEnv(envVars, "DISPATCHER_ADDRESS", strings.TrimSpace(r.dispatcherAddr))
		}
		if r.networkSet {
			envVars = upsertEnv(envVars, "DOCKER_NETWORK_NAME", strings.TrimSpace(job.DockerNetwork))
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

		_, err := r.docker.VolumeCreate(runCtx, volume.CreateOptions{Name: sharedVolume})
		if err != nil {
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("create volume: %v", err))
			return
		}
		defer r.docker.VolumeRemove(context.Background(), sharedVolume, true)

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

		resp, err := r.docker.ContainerCreate(runCtx, &container.Config{
			Image: agentImage,
			Env:   envVars,
		}, hostConfig, networking, nil, containerName)
		if err != nil {
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("container create: %v", err))
			return
		}

		if err := r.docker.ContainerStart(runCtx, resp.ID, container.StartOptions{}); err != nil {
			sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("container start: %v", err))
			return
		}

		log.Info().Str("run_id", job.RunId).Str("container_id", resp.ID).Msg("started agent container")

		runCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go r.monitorRunCancellation(runCtx, dispatcher, job.RunId, resp.ID)
		go r.streamLogs(runCtx, dispatcher, job, resp.ID)

		statusCh, errCh := r.docker.ContainerWait(context.Background(), resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("container wait: %v", err))
				return
			}
		case status := <-statusCh:
			if status.StatusCode != 0 {
				sendJobResult(sendCh, job.RunId, "failed", fmt.Sprintf("exit code %d", status.StatusCode))
				return
			}
		}

		sendJobResult(sendCh, job.RunId, "completed", "")
	}()
}

func (r *runner) monitorRunCancellation(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID, containerID string) {
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

func (r *runner) markRunStopRequested(runID string) bool {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()

	if _, exists := r.stoppedRuns[runID]; exists {
		return false
	}
	r.stoppedRuns[runID] = struct{}{}
	return true
}

func (r *runner) clearRunStopRequested(runID string) {
	r.stopMu.Lock()
	defer r.stopMu.Unlock()

	delete(r.stoppedRuns, runID)
}

func (r *runner) stopContainer(containerID string) {
	if r == nil || r.docker == nil || strings.TrimSpace(containerID) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	timeout := 1
	if err := r.docker.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		log.Warn().Err(err).Str("container_id", containerID).Msg("failed to stop agent container after cancellation")
	}
}

func (r *runner) streamLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, job *proto.JobRequest, containerID string) {
	if dispatcher == nil {
		log.Warn().Str("run_id", job.RunId).Msg("dispatcher client not available; skipping log forwarding")
		return
	}

	logReader, err := r.docker.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Timestamps: true})
	if err != nil {
		log.Error().Err(err).Str("run_id", job.RunId).Msg("failed to attach to container logs")
		return
	}
	defer logReader.Close()

	rPipe, wPipe := io.Pipe()
	go func() {
		defer wPipe.Close()
		_, err := stdcopy.StdCopy(wPipe, wPipe, logReader)
		if err != nil {
			log.Error().Err(err).Str("run_id", job.RunId).Msg("demultiplex log stream failed")
		}
	}()

	logChan := make(chan string, 100)
	go func() {
		defer close(logChan)
		scanner := bufio.NewScanner(rPipe)
		for scanner.Scan() {
			logChan <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			log.Error().Err(err).Str("run_id", job.RunId).Msg("log scanner error")
		}
	}()

	const batchSize = 50
	const batchTimeout = 500 * time.Millisecond

	var batchLines []string
	ticker := time.NewTicker(batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case line, ok := <-logChan:
			if !ok {
				r.flushLogs(ctx, dispatcher, job.RunId, batchLines)
				return
			}
			batchLines = append(batchLines, line)
			if len(batchLines) >= batchSize {
				r.flushLogs(ctx, dispatcher, job.RunId, batchLines)
				batchLines = nil
			}
		case <-ticker.C:
			if len(batchLines) > 0 {
				r.flushLogs(ctx, dispatcher, job.RunId, batchLines)
				batchLines = nil
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *runner) flushLogs(ctx context.Context, dispatcher proto.DispatcherServiceClient, runID string, lines []string) {
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
	imageFilters := filters.NewArgs(filters.Arg("reference", imageName))
	images, err := cli.ImageList(ctx, image.ListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("list images: %w", err)
	}

	if len(images) == 0 {
		log.Info().Msgf("image %s not found locally, pulling", imageName)
		out, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("pull image: %w", err)
		}
		defer out.Close()
		io.Copy(io.Discard, out)
	}
	return nil
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
