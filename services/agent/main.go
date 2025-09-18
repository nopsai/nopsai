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

func agentLog(runID, pipeline string) *zerolog.Logger {
	logger := log.With().
		Str("runid", runID).
		Str("pipeline", pipeline).
		Str("component", "agent").
		Logger()
	return &logger
}

func stepLog(runID, pipeline, step, task string) *zerolog.Logger {
	logger := log.With().
		Str("runid", runID).
		Str("pipeline", pipeline).
		Str("step", step)
	if task != "" {
		logger = logger.Str("task", task)
	}
	result := logger.Logger()
	return &result
}

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
func getDirectoryListing(logger *zerolog.Logger, root string) map[string]string {
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

	// 1. Build a map of completed steps by checking if all their tasks are done
	completedSteps := make(map[string]bool)
	stepTaskCounts := make(map[string]int)
	completedStepTaskCounts := make(map[string]int)

	for _, step := range pipeline.Steps {
		if len(step.Tasks) > 0 {
			stepTaskCounts[step.Name] = len(step.Tasks)
		} else { // Legacy or include step is treated as a single task
			stepTaskCounts[step.Name] = 1
		}
	}

	for taskKey := range completedTasks {
		parts := strings.Split(taskKey, "/")
		stepName := parts[0]
		completedStepTaskCounts[stepName]++
	}

	for stepName, totalTasks := range stepTaskCounts {
		if completedStepTaskCounts[stepName] == totalTasks {
			completedSteps[stepName] = true
		}
	}

	// 2. Find runnable tasks
	for i := range pipeline.Steps {
		step := &pipeline.Steps[i]

		// 2a. First, check if the step's own dependencies are met
		stepDependenciesMet := true
		for _, depStepName := range step.DependsOn {
			if !completedSteps[depStepName] {
				stepDependenciesMet = false
				break
			}
		}
		if !stepDependenciesMet {
			continue // Skip all tasks in this step if its dependencies are not met
		}

		// 2b. If step dependencies are met, check the tasks within the step
		tasksToCheck := []*models.Task{}
		if len(step.Tasks) > 0 {
			for j := range step.Tasks {
				tasksToCheck = append(tasksToCheck, &step.Tasks[j])
			}
		} else { // Legacy or include step
			tasksToCheck = append(tasksToCheck, &models.Task{
				Name:      step.Name,
				Goal:      step.Goal,
				Script:    step.Script,
				DependsOn: []string{}, // Legacy step dependencies are handled at the step level
			})
		}

		for _, task := range tasksToCheck {
			globalKey := fmt.Sprintf("%s/%s", step.Name, task.Name)
			if _, done := completedTasks[globalKey]; done {
				continue
			}

			// 2c. Check the task's internal dependencies (which are other tasks in the same step)
			taskDependenciesMet := true
			for _, depTaskName := range task.DependsOn {
				depGlobalKey := fmt.Sprintf("%s/%s", step.Name, depTaskName)
				if !completedTasks[depGlobalKey] {
					taskDependenciesMet = false
					break
				}
			}

			if taskDependenciesMet {
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

func notifyFinalStatus(pipelineName, runID, status string) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		agentLog(runID, pipelineName).Error().Msg("NOPSAI_API_URL environment variable not set. Cannot report final status")
		return
	}
	url := fmt.Sprintf("%s/v1/runs/%s/finalize", nopsaiURL, runID)

	payload := map[string]string{"status": status}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to send final status to nopsai API")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		agentLog(runID, pipelineName).Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from nopsai API for final status")
	} else {
		agentLog(runID, pipelineName).Info().Str("status", status).Msg("Successfully notified nopsai of final pipeline status")
	}
}

// updateTaskStatus reports the final status of a task back to the nopsai API.
func updateTaskStatus(pipelineName, runID, stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		stepLog(runID, pipelineName, stepName, taskName).Error().Msg("NOPSAI_API_URL environment variable not set. Cannot report status")
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
		stepLog(runID, pipelineName, stepName, taskName).Error().Err(err).Msg("Failed to send status update to nopsai API")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		stepLog(runID, pipelineName, stepName, taskName).Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from nopsai API")
	}
}

// cleanup stops and removes the pipeline container.
func cleanup(cli *client.Client, containerID, pipelineName, runID string) {
	if containerID == "" {
		return
	}
	agentLog(runID, pipelineName).Info().Str("container_id", containerID).Msg("Cleaning up pipeline container")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{}); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to stop pipeline container")
	}

	statusCh, errCh := cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			agentLog(runID, pipelineName).Error().Err(err).Msg("Error waiting for container to stop")
		}
	case <-statusCh:
	}

	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to remove pipeline container")
	}
}

func ensureImageExists(ctx context.Context, logger *zerolog.Logger, cli *client.Client, imageName string) error {
	imageFilters := filters.NewArgs(filters.Arg("reference", imageName))
	images, err := cli.ImageList(ctx, image.ListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("failed to list images to check for %s: %w", imageName, err)
	}

	if len(images) == 0 {
		logger.Info().Str("image", imageName).Msg("Image not found locally, pulling")
		out, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return fmt.Errorf("failed to pull image %s: %w", imageName, err)
		}
		defer out.Close()
		io.Copy(io.Discard, out)
	} else {
		logger.Info().Str("image", imageName).Msg("Image found locally")
	}
	return nil
}

func triggerPipeline(parentRunID, parentPipelineName, parentStepName string, pipelineDef []byte, history string) (string, error) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	url := fmt.Sprintf("%s/v1/run", nopsaiURL)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(pipelineDef))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	req.Header.Set("X-Nopsai-Parent-Run-ID", parentRunID)
	req.Header.Set("X-Nopsai-Parent-Pipeline-Name", parentPipelineName)
	req.Header.Set("X-Nopsai-Parent-Step-Name", parentStepName)

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

func monitorPipeline(logger *zerolog.Logger, runID string) (string, error) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	url := fmt.Sprintf("%s/v1/runs/%s/status", nopsaiURL, runID)
	ticker := time.NewTicker(10 * time.Second) // Poll every 10 seconds
	defer ticker.Stop()

	// Timeout for monitoring to prevent infinite waits
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()

	childLogger := logger.With().Str("child_run_id", runID).Logger()
	childLogger.Info().Msg("Starting to monitor child pipeline")
	for {
		select {
		case <-ticker.C:
			resp, err := http.Get(url)
			if err != nil {
				childLogger.Error().Err(err).Msg("Failed to poll child pipeline status")
				continue
			}

			var statusResp map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
				resp.Body.Close()
				childLogger.Error().Err(err).Msg("Failed to decode child pipeline status")
				continue
			}
			resp.Body.Close()

			status := statusResp["status"]
			childLogger.Info().Str("status", status).Msg("Polling child pipeline status")
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
			agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to decode secrets payload")
		} else {
			if err := json.Unmarshal(secretsJSON, &secrets); err != nil {
				agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to unmarshal secrets payload")
			}
		}
	}

	if llmAgentTimeoutStr == "" {
		llmAgentTimeoutStr = "2m"
	}
	llmAgentTimeout, err := time.ParseDuration(llmAgentTimeoutStr)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid LLM agent timeout duration")
		return 1
	}

	if runID == "" || llmAgentAddress == "" || pipelineDefBase64 == "" || pipelineName == "" || sharedVolumeName == "" {
		agentLog(runID, pipelineName).Error().Msg("Missing one or more required environment variables")
		return 1
	}

	pipelineDefBytes, err := base64.StdEncoding.DecodeString(pipelineDefBase64)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to decode pipeline definition")
		return 1
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(pipelineDefBytes, &pipeline); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to unmarshal pipeline definition")
		return 1
	}

	agentLog(runID, pipeline.Name).Info().Str("llm_agent", llmAgentAddress).Msg("Agent starting and connecting to LLM agent")

	var conn *grpc.ClientConn
	for i := 0; i < 5; i++ {
		conn, err = grpc.NewClient(llmAgentAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
		agentLog(runID, pipeline.Name).Warn().Err(err).Msg("Did not connect to LLM agent. Retrying in 2 seconds")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to connect to LLM agent after multiple retries")
		return 1
	}
	defer conn.Close()
	llmClient := proto.NewLLMServiceClient(conn)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to create Docker client")
		return 1
	}
	defer cli.Close()

	sessionContainers := make(map[string]string)
	sessionMutex := &sync.Mutex{}

	defer func() {
		for sessionName, containerID := range sessionContainers {
			agentLog(runID, pipeline.Name).Info().Str("session", sessionName).Msg("Cleaning up session container")
			cleanup(cli, containerID, pipeline.Name, runID)
		}
	}()

	var timeoutCancel context.CancelFunc
	if pipelineTimeoutStr != "" {
		timeout, err := time.ParseDuration(pipelineTimeoutStr)
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Invalid pipeline timeout duration")
		} else {
			var ctx context.Context
			ctx, timeoutCancel = context.WithTimeout(context.Background(), timeout)
			defer timeoutCancel()

			go func() {
				<-ctx.Done()
				if ctx.Err() == context.DeadlineExceeded {
					agentLog(runID, pipeline.Name).Error().Msg("Pipeline execution timed out. Cleaning up and exiting")
					notifyFinalStatus(pipeline.Name, runID, "failure")
				}
			}()
		}
	}

	history := new(strings.Builder)
	if parentHistoryBase64 != "" {
		decodedHistory, err := base64.StdEncoding.DecodeString(parentHistoryBase64)
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to decode parent execution history")
		} else {
			history.Write(decodedHistory)
			history.WriteString("\n--- Inherited History Above ---\n\n")
		}
	}
	completedTasks := make(map[string]bool)
	pipelineFailed := false
	var syncWg sync.WaitGroup

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
				agentLog(runID, pipeline.Name).Info().Msg("All tasks completed successfully")
			} else if !pipelineFailed {
				agentLog(runID, pipeline.Name).Error().Msg("Stall detected: No runnable tasks found, but not all tasks are complete")
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
				taskLogger := stepLog(runID, pipeline.Name, stepName, task.Name)

				if step.Include != "" {
					updateTaskStatus(pipeline.Name, runID, stepName, stepName, "running", 0, 0)

					parts := strings.SplitN(step.Include, ":", 2)
					if len(parts) != 2 || parts[0] != "pipeline" {
						taskLogger.Error().Str("include", step.Include).Msg("Invalid include format")
						updateTaskStatus(pipeline.Name, runID, stepName, stepName, "failure", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					childPipelineName := parts[1]

					childDef, err := getPipelineDef(childPipelineName)
					if err != nil {
						if strings.Contains(err.Error(), "nopsai api returned non-200 status 404") {
							taskLogger.Warn().Str("child_pipeline", childPipelineName).Msg("Child pipeline not found, marking as not found")
							updateTaskStatus(pipeline.Name, runID, stepName, stepName, "not_found", 0, 0)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false} // Treat as failure for dependency purposes
							return
						}

						taskLogger.Error().Err(err).Msg("Failed to get child pipeline definition")
						updateTaskStatus(pipeline.Name, runID, stepName, stepName, "failure", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					childRunID, err := triggerPipeline(runID, pipelineName, step.Name, childDef, historySnapshot)
					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to trigger child pipeline")
						updateTaskStatus(pipeline.Name, runID, stepName, stepName, "failure", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					monitorFunc := func() {
						finalStatus, err := monitorPipeline(taskLogger, childRunID)
						if err != nil {
							taskLogger.Error().Err(err).Str("child_run_id", childRunID).Msg("Error monitoring child pipeline")
							finalStatus = "failure"
						}
						taskLogger.Info().Str("child_run_id", childRunID).Str("status", finalStatus).Msg("Child pipeline finished")
						updateTaskStatus(pipeline.Name, runID, stepName, stepName, finalStatus, 0, 0)
						if finalStatus == "failure" && step.Sync {
							pipelineFailed = true
						}
					}

					if step.Sync {
						syncWg.Add(1)
						go func() {
							defer syncWg.Done()
							monitorFunc()
						}()
						// Mark the task as "completed" for the main loop, but its final status will be updated later.
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					} else {
						go monitorFunc()
						// For non-sync, we mark it successful immediately to unblock dependencies.
						updateTaskStatus(pipeline.Name, runID, stepName, stepName, "success", 0, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					}
					return
				}

				var stepContainerID string
				updateTaskStatus(pipeline.Name, runID, stepName, task.Name, "running", 0, 0)

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
							taskLogger.Warn().Str("secret", secretName).Msg("Secret was requested by step but not provided")
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
					taskLogger.Info().Str("image", imageName).Msg("Creating new container for step")
					if err := ensureImageExists(context.Background(), taskLogger, cli, imageName); err != nil {
						taskLogger.Error().Err(err).Msg("Failed to ensure step image exists. Shutting down")
						updateTaskStatus(pipeline.Name, runID, stepName, task.Name, "failure", 1, 0)
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					binds := []string{fmt.Sprintf("%s:/workspace", sharedVolumeName)}
					if len(step.Volumes) > 0 {
						for _, vol := range step.Volumes {
							parts := strings.Split(vol, ":")
							if len(parts) != 2 {
								taskLogger.Error().Str("volume", vol).Msg("Invalid volume format. Must be 'volume-name:mount-path'. Skipping")
								continue
							}
							volumeName := parts[0]
							_, err := cli.VolumeInspect(context.Background(), volumeName)
							if err != nil {
								if client.IsErrNotFound(err) {
									taskLogger.Info().Str("volume", volumeName).Msg("Volume not found, creating it now")
									_, createErr := cli.VolumeCreate(context.Background(), volume.CreateOptions{Name: volumeName})
									if createErr != nil {
										taskLogger.Error().Err(createErr).Str("volume", volumeName).Msg("Failed to create volume")
										continue
									}
								} else {
									taskLogger.Error().Err(err).Str("volume", volumeName).Msg("Failed to inspect volume")
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
						taskLogger.Error().Err(err).Msg("Failed to create step container")
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					if err := cli.ContainerStart(context.Background(), cont.ID, container.StartOptions{}); err != nil {
						taskLogger.Error().Err(err).Msg("Failed to start step container")
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					sessionContainers[stepName] = cont.ID
					stepContainerID = cont.ID
				} else {
					taskLogger.Info().Msg("Reusing existing step container")
					stepContainerID = containerID
				}
				sessionMutex.Unlock()

				var action *proto.Action
				var actionStr string
				var historyGoal string
				var llmDurationMs int64

				if task.Script != "" {
					taskLogger.Info().Msg("Executing direct script")
					action = &proto.Action{
						Type:    "EXECUTE_COMMAND",
						Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: task.Script}},
					}
					actionStr = task.Script
				} else {
					taskLogger.Info().Msg("Resolving goal with LLM")
					shareContent := true
					if pipeline.LlmContentSharing != nil {
						shareContent = *pipeline.LlmContentSharing
					}

					var directoryListing map[string]string
					if shareContent {
						taskLogger.Debug().Msg("Content sharing is ENABLED for this pipeline. Scanning directory")
						directoryListing = getDirectoryListing(taskLogger, "/workspace")
					} else {
						taskLogger.Debug().Msg("Content sharing is DISABLED for this pipeline. Skipping directory scan")
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
						taskLogger.Error().Err(err).Msg("Failed to get action from LLM agent. Shutting down")
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

				debugLogger := taskLogger.With().
					Str("action_type", action.Type).
					Logger()
				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				stdout, stderr, exitCode := executeAction(cli, stepContainerID, action, stepEnvVars)
				status := "success"
				output := stdout
				if exitCode != 0 {
					status = "failure"
					output = stderr + stdout
				}
				maskedOutput := maskSecrets(output, secrets)
				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					logMsg := fmt.Sprintf(`status=%s action="%s" output="%s"`, status, actionStr, maskedOutput)
					taskLogger.Info().Msg(logMsg)
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
					taskLogger.Debug().Msg("Output sharing is DISABLED for this task. Hiding output from history")
					output = "[Output was hidden by pipeline configuration]"
				}

				historyMutex.Lock()
				history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", historyGoal, actionStr, exitCode, output))
				historyMutex.Unlock()

				if exitCode == 0 {
					updateTaskStatus(pipeline.Name, runID, stepName, task.Name, "success", exitCode, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: true}
				} else {
					if task.IgnoreFailure {
						updateTaskStatus(pipeline.Name, runID, stepName, task.Name, "failure (ignored)", exitCode, llmDurationMs)
						taskLogger.Warn().Msg("Task failed, but failure is ignored")
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					} else {
						updateTaskStatus(pipeline.Name, runID, stepName, task.Name, "failure", exitCode, llmDurationMs)
						taskLogger.Error().Msg("Critical task failed")
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

	syncWg.Wait()

	finalStatus := "success"
	if pipelineFailed {
		finalStatus = "failure"
		agentLog(runID, pipeline.Name).Error().Msg("Pipeline finished with failed tasks")
	} else {
		agentLog(runID, pipeline.Name).Info().Msg("Pipeline finished successfully")
	}

	notifyFinalStatus(pipeline.Name, runID, finalStatus)
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
