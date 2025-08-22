package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

type TaskResult struct {
	Name    string
	Success bool
}

// Helper struct to manage task execution
type RunnableTask struct {
	Step      *models.PipelineStep
	Task      *models.Task
	GlobalKey string
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

func sanitizeInput(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

// getDirectoryListing recursively walks the specified root directory.
func getDirectoryListing(logger zerolog.Logger, root string) map[string]string {
	directoryListing := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error().Err(err).Str("path", path).Msg("Error accessing path")
			return nil
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		if !info.IsDir() {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Error().Err(readErr).Str("file", path).Msg("Failed to read file")
				return nil
			}
			relPath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			contentType := http.DetectContentType(content)
			if strings.HasPrefix(contentType, "text/") {
				directoryListing[relPath] = string(content)
			} else {
				directoryListing[relPath] = fmt.Sprintf("[non-text file: %s]", contentType)
			}
		}
		return nil
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to walk directory")
	}
	return directoryListing
}

// executeAction runs the given action inside the pipeline container.
func executeAction(cli *client.Client, containerID string, action *proto.Action, env []string) (string, string, int) {
	var cmdStr string

	switch action.Type {
	case "EXECUTE_COMMAND":
		cmdStr = action.GetCommandAction().Command
	case "REPLACE_FILE":
		content := action.GetFileAction().Content
		encodedContent := base64.StdEncoding.EncodeToString([]byte(content))
		filePath := filepath.Join("/workspace", action.GetFileAction().Path)
		cmdStr = fmt.Sprintf("echo %s | base64 -d > %s", encodedContent, filePath)
	case "RETURN_ANSWER":
		ansAction := action.GetAnswerAction()
		if ansAction == nil {
			return "", "Invalid answer action payload", 1
		}
		return ansAction.Answer, "", 0
	default:
		return "", "Unknown action type", 1
	}

	execConfig := container.ExecOptions{
		Cmd:          []string{"sh", "-c", cmdStr},
		Env:          env,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          false,
	}

	execID, err := cli.ContainerExecCreate(context.Background(), containerID, execConfig)
	if err != nil {
		return "", fmt.Sprintf("failed to create exec: %v", err), 1
	}

	resp, err := cli.ContainerExecAttach(context.Background(), execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Sprintf("failed to attach to exec: %v", err), 1
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil {
		return "", fmt.Sprintf("failed to read output: %v", err), 1
	}

	inspect, err := cli.ContainerExecInspect(context.Background(), execID.ID)
	if err != nil {
		return "", fmt.Sprintf("failed to inspect exec: %v", err), 1
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), inspect.ExitCode
}

func getNextRunnableTasks(pipeline *models.Pipeline, completedTasks map[string]bool) []*RunnableTask {
	var runnableTasks []*RunnableTask
	taskToStepMap := make(map[string]string)

	// Map all tasks to their parent step
	for i := range pipeline.Steps {
		step := &pipeline.Steps[i]
		if len(step.Tasks) > 0 {
			for j := range step.Tasks {
				task := &step.Tasks[j]
				taskToStepMap[task.Name] = step.Name
			}
		} else { // Legacy step-as-task
			taskToStepMap[step.Name] = step.Name
		}
	}

	for i := range pipeline.Steps {
		step := &pipeline.Steps[i]
		tasksToCheck := []*models.Task{}
		if len(step.Tasks) > 0 {
			for j := range step.Tasks {
				tasksToCheck = append(tasksToCheck, &step.Tasks[j])
			}
		} else {
			tasksToCheck = append(tasksToCheck, &models.Task{
				Name:             step.Name,
				Goal:             step.Goal,
				Script:           step.Script,
				DependsOn:        step.DependsOn,
				IgnoreFailure:    step.IgnoreFailure,
				LlmOutputSharing: step.LlmOutputSharing,
			})
		}

		for _, task := range tasksToCheck {
			globalKey := fmt.Sprintf("%s/%s", step.Name, task.Name)
			if _, done := completedTasks[globalKey]; done {
				continue
			}

			dependenciesMet := true
			for _, depName := range task.DependsOn {
				depStepName := taskToStepMap[depName]
				depGlobalKey := fmt.Sprintf("%s/%s", depStepName, depName)
				if _, done := completedTasks[depGlobalKey]; !done {
					dependenciesMet = false
					break
				}
			}

			if dependenciesMet {
				runnableTasks = append(runnableTasks, &RunnableTask{
					Step:      step,
					Task:      task,
					GlobalKey: globalKey,
				})
			}
		}
	}

	return runnableTasks
}

func notifyFinalStatus(runID, status string) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		log.Error().Msg("NOPSAI_API_URL environment variable not set. Cannot report final status.")
		return
	}
	url := fmt.Sprintf("%s/v1/runs/%s/finalize", nopsaiURL, runID)

	payload := map[string]string{"status": status}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send final status to nopsai API")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from nopsai API for final status")
	} else {
		log.Info().Str("run_id", runID).Str("status", status).Msg("Successfully notified nopsai of final pipeline status.")
	}
}

// updateTaskStatus reports the final status of a task back to the nopsai API.
func updateTaskStatus(runID, stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		log.Error().Msg("NOPSAI_API_URL environment variable not set. Cannot report status.")
		return
	}
	url := fmt.Sprintf("%s/v1/runs/%s/steps/%s/tasks/%s", nopsaiURL, runID, stepName, taskName)

	payload := map[string]interface{}{
		"status":          status,
		"exit_code":       exitCode,
		"llm_duration_ms": llmDurationMs,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send status update to nopsai API")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from nopsai API")
	}
}

// cleanup stops and removes the pipeline container.
func cleanup(cli *client.Client, containerID string) {
	if containerID == "" {
		return
	}
	log.Info().Msgf("Cleaning up pipeline container: %s", containerID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{}); err != nil {
		log.Error().Err(err).Msg("Failed to stop pipeline container")
	}

	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			log.Error().Err(err).Msg("Error waiting for container to stop")
		}
	case <-statusCh:
	}

	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		log.Error().Err(err).Msg("Failed to remove pipeline container")
	}
}

func ensureImageExists(ctx context.Context, cli *client.Client, imageName string) error {
	imageFilters := filters.NewArgs(filters.Arg("reference", imageName))
	images, err := cli.ImageList(ctx, image.ListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("failed to list images to check for %s: %w", imageName, err)
	}

	if len(images) == 0 {
		log.Info().Msgf("Image %s not found locally, pulling...", imageName)
		out, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image %s: %w", imageName, err)
		}
		defer out.Close()
		io.Copy(io.Discard, out)
	} else {
		log.Info().Msgf("Image %s found locally.", imageName)
	}
	return nil
}

func triggerPipeline(parentRunID, parentPipelineName string, pipelineDef []byte, history string) (string, error) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	url := fmt.Sprintf("%s/v1/run", nopsaiURL)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(pipelineDef))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	req.Header.Set("X-Nopsai-Parent-Run-ID", parentRunID)
	req.Header.Set("X-Nopsai-Parent-Pipeline-Name", parentPipelineName)

	if history != "" {
		encodedHistory := base64.StdEncoding.EncodeToString([]byte(history))
		req.Header.Set("X-Nopsai-Parent-History", encodedHistory)
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment != "" {
		req.Header.Set("X-Nopsai-Environment", environment)
	}

	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GIT_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				headerKey := "X-" + strings.ReplaceAll(strings.Title(strings.ToLower(strings.ReplaceAll(parts[0], "_", " "))), " ", "-")
				req.Header.Set(headerKey, parts[1])
			}
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to trigger child pipeline: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("nopsai api returned non-201 status %d for trigger: %s", resp.StatusCode, string(body))
	}

	prefix := "Pipeline run created successfully with ID: "
	if !strings.HasPrefix(string(body), prefix) {
		return "", fmt.Errorf("unexpected response body from trigger: %s", string(body))
	}
	runID := strings.TrimSpace(strings.TrimPrefix(string(body), prefix))
	return runID, nil
}

func monitorPipeline(runID string) (string, error) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	url := fmt.Sprintf("%s/v1/runs/%s/status", nopsaiURL, runID)
	ticker := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	defer ticker.Stop()

	// Timeout for monitoring to prevent infinite waits
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	log.Info().Str("child_run_id", runID).Msg("Starting to monitor child pipeline.")
	for {
		select {
		case <-ticker.C:
			resp, err := http.Get(url)
			if err != nil {
				log.Error().Err(err).Str("child_run_id", runID).Msg("Failed to poll child pipeline status")
				continue
			}

			var statusResp map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
				resp.Body.Close()
				log.Error().Err(err).Str("child_run_id", runID).Msg("Failed to decode child pipeline status")
				continue
			}
			resp.Body.Close()

			status := statusResp["status"]
			log.Info().Str("child_run_id", runID).Str("status", status).Msg("Polling child pipeline status")
			if status == "success" || status == "failure" {
				return status, nil
			}
		case <-ctx.Done():
			return "failure", fmt.Errorf("timed out waiting for child pipeline %s to complete", runID)
		}
	}
}

func getPipelineDef(pipelineName string) ([]byte, error) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		return nil, fmt.Errorf("NOPSAI_API_URL not set")
	}
	// Note: We assume for now the agent doesn't have access to the repo to fetch files.
	// It must fetch named pipelines from the nopsai service.
	url := fmt.Sprintf("%s/v1/pipelines/%s", nopsaiURL, pipelineName)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call nopsai api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("nopsai api returned non-200 status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

func run() int {
	// --- Initialization ---
	logLevelStr := os.Getenv("LOG_LEVEL")
	if logLevelStr == "" {
		logLevelStr = "info"
	}
	logLevel, err := zerolog.ParseLevel(logLevelStr)
	if err != nil {
		logLevel = zerolog.InfoLevel
	}
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}
	zerolog.SetGlobalLevel(logLevel)

	runID := os.Getenv("RUN_ID")
	pipelineName := os.Getenv("PIPELINE_NAME")
	llmAgentAddress := os.Getenv("LLM_AGENT_ADDRESS")
	pipelineDefBase64 := os.Getenv("PIPELINE_DEFINITION")
	parentHistoryBase64 := os.Getenv("PARENT_EXECUTION_HISTORY")
	sharedVolumeName := os.Getenv("SHARED_VOLUME_NAME")
	pipelineTimeoutStr := os.Getenv("PIPELINE_TIMEOUT")
	dockerNetworkName := os.Getenv("DOCKER_NETWORK_NAME")
	llmAgentTimeoutStr := os.Getenv("LLM_AGENT_TIMEOUT")
	secretsBase64 := os.Getenv("NOPSAI_SECRETS")

	var secrets map[string]string
	if secretsBase64 != "" {
		secretsJSON, err := base64.StdEncoding.DecodeString(secretsBase64)
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode secrets payload")
		} else {
			if err := json.Unmarshal(secretsJSON, &secrets); err != nil {
				log.Error().Err(err).Msg("Failed to unmarshal secrets payload")
			}
		}
	}

	if llmAgentTimeoutStr == "" {
		llmAgentTimeoutStr = "2m"
	}
	llmAgentTimeout, err := time.ParseDuration(llmAgentTimeoutStr)
	if err != nil {
		log.Error().Err(err).Msg("Invalid LLM agent timeout duration")
		return 1
	}

	if runID == "" || llmAgentAddress == "" || pipelineDefBase64 == "" || pipelineName == "" || sharedVolumeName == "" {
		log.Error().Msg("Missing one or more required environment variables.")
		return 1
	}

	pipelineDefBytes, err := base64.StdEncoding.DecodeString(pipelineDefBase64)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decode pipeline definition")
		return 1
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(pipelineDefBytes, &pipeline); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal pipeline definition")
		return 1
	}

	log.Info().Str("run_id", runID).Str("pipeline", pipeline.Name).Msgf("Agent starting, connecting to LLM Agent at %s", llmAgentAddress)

	var conn *grpc.ClientConn
	for i := 0; i < 5; i++ {
		conn, err = grpc.NewClient(llmAgentAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
		log.Warn().Err(err).Msgf("Did not connect to LLM agent. Retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Error().Err(err).Msg("Failed to connect to LLM agent after multiple retries")
		return 1
	}
	defer conn.Close()
	llmClient := proto.NewLLMServiceClient(conn)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Docker client")
		return 1
	}
	defer cli.Close()

	sessionContainers := make(map[string]string)
	sessionMutex := &sync.Mutex{}

	defer func() {
		for sessionName, containerID := range sessionContainers {
			log.Info().Str("session", sessionName).Msg("Cleaning up session container")
			cleanup(cli, containerID)
		}
	}()

	var timeoutCancel context.CancelFunc
	if pipelineTimeoutStr != "" {
		timeout, err := time.ParseDuration(pipelineTimeoutStr)
		if err != nil {
			log.Error().Err(err).Msg("Invalid pipeline timeout duration")
		} else {
			var ctx context.Context
			ctx, timeoutCancel = context.WithTimeout(context.Background(), timeout)
			defer timeoutCancel()

			go func() {
				<-ctx.Done()
				if ctx.Err() == context.DeadlineExceeded {
					log.Error().Msg("Pipeline execution timed out. Cleaning up and exiting.")
					notifyFinalStatus(runID, "failed")
				}
			}()
		}
	}

	history := new(strings.Builder)
	if parentHistoryBase64 != "" {
		decodedHistory, err := base64.StdEncoding.DecodeString(parentHistoryBase64)
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode parent execution history")
		} else {
			history.Write(decodedHistory)
			history.WriteString("\n--- Inherited History Above ---\n\n")
		}
	}
	completedTasks := make(map[string]bool)
	pipelineFailed := false

	totalTasks := 0
	for _, step := range pipeline.Steps {
		if len(step.Tasks) > 0 {
			totalTasks += len(step.Tasks)
		} else {
			totalTasks++
		}
	}

	for len(completedTasks) < totalTasks {
		runnableTasks := getNextRunnableTasks(&pipeline, completedTasks)
		if len(runnableTasks) == 0 {
			if !pipelineFailed && len(completedTasks) == totalTasks {
				log.Info().Str("pipeline", pipeline.Name).Msg("All tasks completed successfully.")
			} else if !pipelineFailed {
				log.Error().Str("pipeline", pipeline.Name).Msg("Stall detected: No runnable tasks found, but not all tasks are complete.")
				pipelineFailed = true
			}
			break
		}

		var wg sync.WaitGroup
		results := make(chan TaskResult, len(runnableTasks))
		historyMutex := &sync.Mutex{}

		for _, runnable := range runnableTasks {
			wg.Add(1)
			go func(runnable *RunnableTask) {
				defer wg.Done()

				step := runnable.Step
				task := runnable.Task
				stepName := step.Name

				if step.Include != "" {
					updateTaskStatus(runID, stepName, stepName, "started", 0, 0)

					parts := strings.SplitN(step.Include, ":", 2)
					if len(parts) != 2 || parts[0] != "pipeline" {
						log.Error().Str("include", step.Include).Msg("Invalid include format")
						updateTaskStatus(runID, stepName, stepName, "failed", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					childPipelineName := parts[1]

					childDef, err := getPipelineDef(childPipelineName)
					if err != nil {
						log.Error().Err(err).Msg("Failed to get child pipeline definition")
						updateTaskStatus(runID, stepName, stepName, "failed", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					childRunID, err := triggerPipeline(runID, pipelineName, childDef, historySnapshot)
					if err != nil {
						log.Error().Err(err).Msg("Failed to trigger child pipeline")
						updateTaskStatus(runID, stepName, stepName, "failed", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					if step.Sync {
						finalStatus, err := monitorPipeline(childRunID)
						if err != nil {
							log.Error().Err(err).Msg("Failed to monitor child pipeline")
							updateTaskStatus(runID, stepName, stepName, "failed", 1, 0)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						if finalStatus == "failure" {
							log.Error().Str("child_run_id", childRunID).Msg("Synchronous child pipeline failed.")
							updateTaskStatus(runID, stepName, stepName, "failed", 1, 0)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
					}

					log.Info().Str("child_run_id", childRunID).Msg("Include step finished successfully.")
					updateTaskStatus(runID, stepName, stepName, "completed", 0, 0)
					results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					return
				}

				var stepContainerID string
				updateTaskStatus(runID, stepName, task.Name, "started", 0, 0)

				// --- MODIFICATION START ---
				// This slice will hold the final, resolved environment variables for the step.
				var stepEnvVars []string

				// Create a set of required environment variable keys for efficient lookup.
				requiredEnvKeys := make(map[string]struct{})
				for _, key := range pipeline.Environment {
					requiredEnvKeys[key] = struct{}{}
				}

				// Iterate over the agent's environment to find the resolved values.
				for _, e := range os.Environ() {
					parts := strings.SplitN(e, "=", 2)
					if len(parts) == 2 {
						key := parts[0]
						// Check if this variable is one of the ones required by the pipeline.
						if _, ok := requiredEnvKeys[key]; ok {
							stepEnvVars = append(stepEnvVars, e)
						}
					}

					// Always forward GIT_* context and the current ENVIRONMENT.
					if strings.HasPrefix(e, "GIT_") || strings.HasPrefix(e, "ENVIRONMENT=") {
						stepEnvVars = append(stepEnvVars, e)
					}
				}
				// --- MODIFICATION END ---

				if len(step.Secrets) > 0 && len(secrets) > 0 {
					for _, secretName := range step.Secrets {
						if secretValue, ok := secrets[secretName]; ok {
							stepEnvVars = append(stepEnvVars, fmt.Sprintf("%s=%s", secretName, secretValue))
						} else {
							log.Warn().Str("step", stepName).Str("secret", secretName).Msg("Secret was requested by step but not provided.")
						}
					}
				}

				imageName := step.Image
				if imageName == "" {
					imageName = pipeline.ContainerImage
				}

				sessionMutex.Lock()
				containerID, ok := sessionContainers[stepName]
				if !ok {
					log.Info().Str("step", stepName).Str("image", imageName).Msg("Creating new container for step")
					if err := ensureImageExists(context.Background(), cli, imageName); err != nil {
						log.Error().Err(err).Msg("Failed to ensure step image exists. Shutting down.")
						updateTaskStatus(runID, stepName, task.Name, "failed", 1, 0)
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					binds := []string{fmt.Sprintf("%s:/workspace", sharedVolumeName)}
					if len(step.Volumes) > 0 {
						for _, vol := range step.Volumes {
							parts := strings.Split(vol, ":")
							if len(parts) != 2 {
								log.Error().Str("volume", vol).Msg("Invalid volume format. Must be 'volume-name:mount-path'. Skipping.")
								continue
							}
							volumeName := parts[0]
							_, err := cli.VolumeInspect(context.Background(), volumeName)
							if err != nil {
								if client.IsErrNotFound(err) {
									log.Info().Str("volume", volumeName).Msg("Volume not found, creating it now.")
									_, createErr := cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: volumeName})
									if createErr != nil {
										log.Error().Err(createErr).Str("volume", volumeName).Msg("Failed to create volume.")
										continue
									}
								} else {
									log.Error().Err(err).Str("volume", volumeName).Msg("Failed to inspect volume.")
									continue
								}
							}
							binds = append(binds, vol)
						}
					}

					repoName := os.Getenv("GIT_REPO_NAME")
					sanitizedPipelineName := sanitizeInput(pipelineName)
					sanitizedStepName := sanitizeInput(stepName)
					shortRunID := runID[:8]

					var stepContainerName string
					if repoName != "" {
						sanitizedRepoName := sanitizeInput(repoName)
						stepContainerName = fmt.Sprintf("%s-%s-%s-%s", sanitizedRepoName, sanitizedPipelineName, sanitizedStepName, shortRunID)
					} else {
						stepContainerName = fmt.Sprintf("%s-%s-%s", sanitizedPipelineName, sanitizedStepName, shortRunID)
					}

					cont, err := cli.ContainerCreate(context.Background(), &container.Config{
						Image:      imageName,
						WorkingDir: "/workspace",
						Entrypoint: []string{"tail", "-f", "/dev/null"},
						Env:        stepEnvVars,
						Tty:        false,
					}, &container.HostConfig{
						Binds:       binds,
						NetworkMode: container.NetworkMode(dockerNetworkName),
					}, nil, nil, stepContainerName)
					if err != nil {
						log.Error().Err(err).Msg("Failed to create step container")
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					if err := cli.ContainerStart(context.Background(), cont.ID, container.StartOptions{}); err != nil {
						log.Error().Err(err).Msg("Failed to start step container")
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					sessionContainers[stepName] = cont.ID
					stepContainerID = cont.ID
				} else {
					log.Info().Str("step", stepName).Msg("Reusing existing step container")
					stepContainerID = containerID
				}
				sessionMutex.Unlock()

				var action *proto.Action
				var actionStr string
				var historyGoal string
				var llmDurationMs int64

				if task.Script != "" {
					log.Info().Str("task", task.Name).Msg("Executing direct script.")
					action = &proto.Action{
						Type:    "EXECUTE_COMMAND",
						Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: task.Script}},
					}
					actionStr = task.Script
				} else {
					log.Info().Str("task", task.Name).Msg("Resolving goal with LLM.")
					shareContent := true
					if pipeline.LlmContentSharing != nil {
						shareContent = *pipeline.LlmContentSharing
					}

					var directoryListing map[string]string
					if shareContent {
						log.Debug().Msg("Content sharing is ENABLED for this pipeline. Scanning directory.")
						directoryListing = getDirectoryListing(log.Logger, "/workspace")
					} else {
						log.Debug().Msg("Content sharing is DISABLED for this pipeline. Skipping directory scan.")
						directoryListing = make(map[string]string)
					}
					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()

					envMap := make(map[string]string)
					for _, e := range stepEnvVars {
						parts := strings.SplitN(e, "=", 2)
						if len(parts) == 2 {
							envMap[parts[0]] = parts[1]
						}
					}

					req := &proto.GetActionRequest{
						Goal:             task.Goal,
						History:          historySnapshot,
						DirectoryListing: directoryListing,
						Environment:      envMap,
					}

					ctx, cancel := context.WithTimeout(context.Background(), llmAgentTimeout)
					action, err = llmClient.GetAction(ctx, req)
					cancel()
					if err != nil {
						log.Error().Err(err).Str("task", task.Name).Msg("Failed to get action from LLM agent. Shutting down.")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					if cmd := action.GetCommandAction(); cmd != nil {
						actionStr = cmd.Command
					} else if file := action.GetFileAction(); file != nil {
						actionStr = fmt.Sprintf("Write to %s", file.Path)
					} else if ans := action.GetAnswerAction(); ans != nil {
						actionStr = task.Goal
					}
				}

				debugLogger := log.With().
					Str("pipeline_name", pipelineName).
					Str("run_id", runID).
					Str("step_name", stepName).
					Str("task_name", task.Name).
					Str("action_type", action.Type).
					Logger()
				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				stdout, stderr, exitCode := executeAction(cli, stepContainerID, action, stepEnvVars)
				status := "Succeeded"
				output := stdout
				if exitCode != 0 {
					status = "Failed"
					output = stderr + stdout
				}
				maskedOutput := maskSecrets(output, secrets)
				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					logMsg := fmt.Sprintf(`status=%s step="%s" task="%s" action="%s" output="%s"`, status, stepName, task.Name, actionStr, maskedOutput)
					log.Info().Str("pipeline", pipelineName).Msg(logMsg)
				}

				shareOutput := true
				if pipeline.LlmOutputSharing != nil {
					shareOutput = *pipeline.LlmOutputSharing
				}
				if task.LlmOutputSharing != nil {
					shareOutput = *task.LlmOutputSharing
				}
				historyGoal = task.Goal
				if historyGoal == "" {
					historyGoal = fmt.Sprintf("Execute script for task: %s", task.Name)
				}
				if !shareOutput {
					log.Debug().Msg("Output sharing is DISABLED for this task. Hiding output from history.")
					output = "[Output was hidden by pipeline configuration]"
				}

				historyMutex.Lock()
				history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", historyGoal, actionStr, exitCode, output))
				historyMutex.Unlock()

				if exitCode == 0 {
					updateTaskStatus(runID, stepName, task.Name, "completed", exitCode, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: true}
				} else {
					if task.IgnoreFailure {
						updateTaskStatus(runID, stepName, task.Name, "failed (ignored)", exitCode, llmDurationMs)
						log.Warn().Str("pipeline", pipelineName).Str("task", task.Name).Msg("Task failed, but failure is ignored.")
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					} else {
						updateTaskStatus(runID, stepName, task.Name, "failed", exitCode, llmDurationMs)
						log.Error().Str("pipeline", pipelineName).Str("task", task.Name).Msg("Critical task failed.")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					}
				}
			}(runnable)
		}

		wg.Wait()
		close(results)

		for result := range results {
			if result.Success {
				completedTasks[result.Name] = true
			} else {
				pipelineFailed = true
			}
		}

		if pipelineFailed {
			break
		}
	}

	finalStatus := "success"
	if pipelineFailed {
		finalStatus = "failure"
		log.Error().Str("pipeline", pipelineName).Msg("Pipeline finished with failed tasks.")
	} else {
		log.Info().Str("pipeline", pipelineName).Msg("Pipeline finished successfully.")
	}

	notifyFinalStatus(runID, finalStatus)
	if pipelineFailed {
		return 1
	}
	return 0
}

func maskSecrets(output string, secrets map[string]string) string {
	if len(secrets) == 0 || output == "" {
		return output
	}
	for _, secretValue := range secrets {
		if len(secretValue) < 4 {
			continue
		}
		output = strings.ReplaceAll(output, secretValue, "*****")
	}
	return output
}

func main() {
	os.Exit(run())
}
