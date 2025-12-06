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
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

func splitPipelineIdentifier(identifier string) (string, string) {
	trimmed := strings.TrimSpace(identifier)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", ""
	}

	normalized := filepath.ToSlash(trimmed)
	lower := strings.ToLower(normalized)
	if strings.HasSuffix(lower, ".yaml") {
		normalized = normalized[:len(normalized)-len(".yaml")]
	} else if strings.HasSuffix(lower, ".yml") {
		normalized = normalized[:len(normalized)-len(".yml")]
	}

	parts := strings.Split(normalized, "/")
	name := parts[len(parts)-1]
	var path string
	if len(parts) > 1 {
		path = strings.Join(parts[:len(parts)-1], "/")
	}
	return path, name
}

type Matcher struct {
	Pattern  string
	IsDir    bool
	IsGlobal bool
}

func (m *Matcher) Matches(relPath string, isDir bool) bool {
	if m.IsDir {
		if !isDir && !strings.HasPrefix(relPath, m.Pattern) {
			return false
		}
		// Match if the path is the directory itself or a path within it
		return relPath == m.Pattern || strings.HasPrefix(relPath, m.Pattern+"/")
	}

	if m.IsGlobal {
		// If the pattern has no '/', it should match the basename of the path.
		base := filepath.Base(relPath)
		matched, _ := filepath.Match(m.Pattern, base)
		return matched
	}

	// It's a full path pattern
	matched, _ := filepath.Match(m.Pattern, relPath)
	return matched
}

func isIgnored(path string, matchers []Matcher, root string, isDir bool) bool {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	// On Windows, convert backslashes to forward slashes for consistent matching
	relPath = filepath.ToSlash(relPath)

	for _, matcher := range matchers {
		if matcher.Matches(relPath, isDir) {
			return true
		}
	}
	return false
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
	Name      string
	Success   bool
	Skipped   bool
	Condition string
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
func getDirectoryListing(logger *zerolog.Logger, root string, ignorePatterns []string) map[string]string {
	directoryListing := make(map[string]string)
	var matchers []Matcher

	// Create matchers from the pipeline's ignorePatterns
	for _, p := range ignorePatterns {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		isDir := strings.HasSuffix(p, "/")
		pattern := strings.TrimSuffix(p, "/")
		matchers = append(matchers, Matcher{
			Pattern:  pattern,
			IsDir:    isDir,
			IsGlobal: !strings.Contains(pattern, "/"),
		})
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error().Err(err).Str("path", path).Msg("Error accessing path")
			return nil
		}

		// Check if the path should be ignored by the patterns from the pipeline directive
		if isIgnored(path, matchers, root, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir // Skip the entire directory
			}
			return nil // Skip this file
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
		stepName := step.GetName()
		tasks := step.GetTasks()
		if len(tasks) > 0 {
			stepTaskCounts[stepName] = len(tasks)
		} else { // Legacy or include step is treated as a single task
			stepTaskCounts[stepName] = 1
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
		stepName := step.GetName()

		// 2a. First, check if the step's own dependencies are met
		stepDependenciesMet := true
		for _, depStepName := range step.GetDependsOn() {
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
		if taskStep, ok := step.AsTaskStep(); ok && len(taskStep.Tasks) > 0 {
			for j := range taskStep.Tasks {
				tasksToCheck = append(tasksToCheck, &taskStep.Tasks[j])
			}
		} else { // Legacy or include step
			tasksToCheck = append(tasksToCheck, &models.Task{
				Name:      stepName,
				Goal:      step.GetGoal(),
				Script:    step.GetScript(),
				DependsOn: []string{}, // Legacy step dependencies are handled at the step level
			})
		}

		for _, task := range tasksToCheck {
			globalKey := fmt.Sprintf("%s/%s", stepName, task.Name)
			if _, done := completedTasks[globalKey]; done {
				continue
			}

			// 2c. Check the task's internal dependencies (which are other tasks in the same step)
			taskDependenciesMet := true
			for _, depTaskName := range task.DependsOn {
				depGlobalKey := fmt.Sprintf("%s/%s", stepName, depTaskName)
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

	timeout := 1 // 1 second timeout for stop
	if err := cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
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

func triggerPipeline(parentRunID, parentPipelineName, parentStepName, pipelineIdentifier string, pipelineDef []byte, history string) (string, error) {
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

	if pipelineIdentifier != "" {
		pathPart, _ := splitPipelineIdentifier(pipelineIdentifier)
		req.Header.Set("X-Nopsai-Pipeline-Path", pathPart)
	}

	if history != "" {
		encodedHistory := base64.StdEncoding.EncodeToString([]byte(history))
		req.Header.Set("X-Nopsai-Parent-History", encodedHistory)
	}

	scope := os.Getenv("SCOPE")
	if scope != "" {
		req.Header.Set("X-Nopsai-Scope", scope)
	}

	if triggerEventID := os.Getenv("GIT_TRIGGER_EVENT_ID"); triggerEventID != "" {
		req.Header.Set("X-Nopsai-Trigger-Event-ID", triggerEventID)
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

	// Add git context as query parameters if available
	repoOwner := os.Getenv("GIT_REPO_OWNER")
	repoName := os.Getenv("GIT_REPO_NAME")
	commitSHA := os.Getenv("GIT_COMMIT_SHA")
	if repoOwner != "" && repoName != "" && commitSHA != "" {
		url = fmt.Sprintf("%s?repoOwner=%s&repoName=%s&commitSHA=%s", url, repoOwner, repoName, commitSHA)
	}

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
	logFormat := os.Getenv("LOG_FORMAT")
	if logFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}
	zerolog.SetGlobalLevel(zerolog.TraceLevel)

	runID := os.Getenv("RUN_ID")
	pipelineName := os.Getenv("PIPELINE_NAME")
	triggerEventID := os.Getenv("GIT_TRIGGER_EVENT_ID")
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

	if triggerEventID == "" {
		triggerEventID = "N/A"
	}
	agentLog(runID, pipeline.Name).Info().Str("trigger_event_id", triggerEventID).Msg("Pipeline execution starting")
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

	cleanupStepContainers := func(reason string) {
		sessionMutex.Lock()
		containers := make(map[string]string, len(sessionContainers))
		for sessionName, containerID := range sessionContainers {
			containers[sessionName] = containerID
			delete(sessionContainers, sessionName)
		}
		sessionMutex.Unlock()
		for sessionName, containerID := range containers {
			agentLog(runID, pipeline.Name).Info().Str("session", sessionName).Str("reason", reason).Msg("Cleaning up session container")
			cleanup(cli, containerID, pipeline.Name, runID)
		}
	}

	defer cleanupStepContainers("exit")

	activeTasks := make(map[string]struct{})
	activeTasksMutex := &sync.Mutex{}

	addActiveTask := func(stepName, taskName string) {
		activeTasksMutex.Lock()
		activeTasks[stepName+"/"+taskName] = struct{}{}
		activeTasksMutex.Unlock()
	}

	removeActiveTask := func(stepName, taskName string) {
		activeTasksMutex.Lock()
		delete(activeTasks, stepName+"/"+taskName)
		activeTasksMutex.Unlock()
	}

	cancelActiveTasks := func(reason string) {
		activeTasksMutex.Lock()
		keys := make([]string, 0, len(activeTasks))
		for key := range activeTasks {
			keys = append(keys, key)
		}
		activeTasks = make(map[string]struct{})
		activeTasksMutex.Unlock()
		for _, key := range keys {
			parts := strings.SplitN(key, "/", 2)
			if len(parts) != 2 {
				continue
			}
			stepName, taskName := parts[0], parts[1]
			stepLog(runID, pipeline.Name, stepName, taskName).Warn().Str("reason", reason).Msg("Marking task as cancelled")
			updateTaskStatus(pipeline.Name, runID, stepName, taskName, "cancelled", 0, 0)
		}
	}

	defer cancelActiveTasks("exit")

	setTaskRunning := func(stepName, taskName string) {
		updateTaskStatus(pipeline.Name, runID, stepName, taskName, "running", 0, 0)
		addActiveTask(stepName, taskName)
	}

	finalizeTask := func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
		updateTaskStatus(pipeline.Name, runID, stepName, taskName, status, exitCode, llmDurationMs)
		removeActiveTask(stepName, taskName)
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signalChan)

	go func() {
		sig := <-signalChan
		agentLog(runID, pipeline.Name).Warn().Str("signal", sig.String()).Msg("Received termination signal")
		cancelActiveTasks("signal")
		cleanupStepContainers("signal")
		os.Exit(0)
	}()

	var timeoutCtx context.Context
	var timeoutCancel context.CancelFunc
	var timeoutTriggered atomic.Bool
	if pipelineTimeoutStr != "" {
		timeout, err := time.ParseDuration(pipelineTimeoutStr)
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Invalid pipeline timeout duration")
		} else if timeout > 0 {
			timeoutCtx, timeoutCancel = context.WithTimeout(context.Background(), timeout)
			defer timeoutCancel()
		}
	}

	if timeoutCtx != nil {
		go func() {
			<-timeoutCtx.Done()
			if timeoutCtx.Err() == context.DeadlineExceeded {
				if timeoutTriggered.CompareAndSwap(false, true) {
					agentLog(runID, pipeline.Name).Error().Msg("Pipeline execution timed out. Cleaning up and exiting")
					cancelActiveTasks("timeout")
					cleanupStepContainers("timeout")
				}
			}
		}()
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
		if tasks := step.GetTasks(); len(tasks) > 0 {
			totalTasks += len(tasks)
		} else {
			totalTasks++
		}
	}

	for len(completedTasks) < totalTasks {
		if timeoutTriggered.Load() {
			pipelineFailed = true
			break
		}
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
			if timeoutTriggered.Load() {
				break
			}
			wg.Add(1)
			go func(runnable *RunnableTask) {
				defer wg.Done()
				if timeoutTriggered.Load() {
					if runnable.Step != nil && runnable.Task != nil {
						finalizeTask(runnable.Step.GetName(), runnable.Task.Name, "cancelled", 0, 0)
					}
					results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					return
				}

				step := runnable.Step
				task := runnable.Task
				stepName := step.GetName()
				taskLogger := stepLog(runID, pipeline.Name, stepName, task.Name)

				// --- CONDITION EVALUATION LOGIC START ---
				condition := strings.TrimSpace(step.GetCondition())
				if condition != "" {
					taskLogger.Info().Msgf("Evaluating condition for step '%s': \"%s\"", stepName, condition)

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()

					envMap := make(map[string]string)
					for _, e := range os.Environ() {
						parts := strings.SplitN(e, "=", 2)
						if len(parts) == 2 {
							envMap[parts[0]] = parts[1]
						}
					}

					req := &proto.ConditionRequest{
						Goal:        condition,
						History:     historySnapshot,
						Environment: envMap,
					}

					var resp *proto.ConditionResponse
					err = withRetry(func() error {
						ctx, cancel := context.WithTimeout(context.Background(), llmAgentTimeout)
						defer cancel()
						var e error
						resp, e = llmClient.EvaluateCondition(ctx, req)
						return e
					}, 3, 1*time.Second)

					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to evaluate condition from LLM agent. Skipping step.")
						pipelineFailed = true // Mark failure to stop pipeline
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					if !resp.Result {
						taskLogger.Info().Msg("Condition evaluated to false. Skipping all tasks in this step.")
						// Mark all tasks in this step as skipped and completed
						tasksInStep := step.GetTasks()
						if len(tasksInStep) == 0 { // For legacy/include steps
							tasksInStep = []models.Task{{Name: stepName}}
						}
						for _, t := range tasksInStep {
							finalizeTask(stepName, t.Name, "skipped", 0, 0)
							// We send a success result so the main loop can correctly count this as "handled"
							results <- TaskResult{Name: fmt.Sprintf("%s/%s", stepName, t.Name), Success: true, Skipped: true}
						}
						return
					}
					taskLogger.Info().Msg("Condition evaluated to true. Proceeding with step.")
				}
				// --- CONDITION EVALUATION LOGIC END ---

				includeTarget := strings.TrimSpace(step.GetInclude())
				if includeTarget != "" {
					setTaskRunning(stepName, stepName)

					parts := strings.SplitN(includeTarget, ":", 2)
					if len(parts) != 2 || parts[0] != "pipeline" {
						taskLogger.Error().Str("include", includeTarget).Msg("Invalid include format")
						finalizeTask(stepName, stepName, "failure", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					childPipelineName := parts[1]

					childDef, err := getPipelineDef(childPipelineName)
					if err != nil {
						if strings.Contains(err.Error(), "nopsai api returned non-200 status 404") {
							taskLogger.Warn().Str("child_pipeline", childPipelineName).Msg("Child pipeline not found, marking as not found")
							finalizeTask(stepName, stepName, "not_found", 0, 0)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false} // Treat as failure for dependency purposes
							return
						}

						taskLogger.Error().Err(err).Msg("Failed to get child pipeline definition")
						finalizeTask(stepName, stepName, "failure", 1, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					childRunID, err := triggerPipeline(runID, pipelineName, stepName, childPipelineName, childDef, historySnapshot)
					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to trigger child pipeline")
						finalizeTask(stepName, stepName, "failure", 1, 0)
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
						finalizeTask(stepName, stepName, finalStatus, 0, 0)
						if finalStatus == "failure" && step.GetSync() {
							pipelineFailed = true
						}
					}

					if step.GetSync() {
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
						finalizeTask(stepName, stepName, "success", 0, 0)
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					}
					return
				}

				var stepContainerID string
				setTaskRunning(stepName, task.Name)

				var stepEnvVars []string
				requiredEnvKeys := make(map[string]struct{})
				for _, key := range pipeline.Environment {
					requiredEnvKeys[key] = struct{}{}
				}

				for _, e := range os.Environ() {
					parts := strings.SplitN(e, "=", 2)
					if len(parts) == 2 {
						key := parts[0]
						if _, ok := requiredEnvKeys[key]; ok {
							stepEnvVars = append(stepEnvVars, e)
						}
					}

					if strings.HasPrefix(e, "GIT_") || strings.HasPrefix(e, "SCOPE=") {
						stepEnvVars = append(stepEnvVars, e)
					}
				}

				stepSecrets := step.GetSecrets()
				if len(stepSecrets) > 0 && len(secrets) > 0 {
					for _, secretName := range stepSecrets {
						if secretValue, ok := secrets[secretName]; ok {
							stepEnvVars = append(stepEnvVars, fmt.Sprintf("%s=%s", secretName, secretValue))
						} else {
							taskLogger.Warn().Str("secret", secretName).Msg("Secret was requested by step but not provided")
						}
					}
				}

				imageName := step.GetImage()
				if imageName == "" {
					imageName = pipeline.ContainerImage
				}

				sessionMutex.Lock()
				containerID, ok := sessionContainers[stepName]
				if !ok {
					taskLogger.Info().Str("image", imageName).Msg("Creating new container for step")
					if err := ensureImageExists(context.Background(), taskLogger, cli, imageName); err != nil {
						taskLogger.Error().Err(err).Msg("Failed to ensure step image exists. Shutting down")
						finalizeTask(stepName, task.Name, "failure", 1, 0)
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					binds := []string{fmt.Sprintf("%s:/workspace", sharedVolumeName)}
					if stepVolumes := step.GetVolumes(); len(stepVolumes) > 0 {
						for _, vol := range stepVolumes {
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
						finalizeTask(stepName, task.Name, "failure", 1, 0)
						sessionMutex.Unlock()
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					if err := cli.ContainerStart(context.Background(), cont.ID, container.StartOptions{}); err != nil {
						taskLogger.Error().Err(err).Msg("Failed to start step container")
						finalizeTask(stepName, task.Name, "failure", 1, 0)
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

				goalText := strings.TrimSpace(task.Goal)
				if goalText == "" {
					goalText = strings.TrimSpace(step.GetGoal())
				}

				if task.Script != "" {
					if goalText != "" {
						taskLogger.Info().Msgf("Executing direct script for goal: %s", goalText)
					} else {
						taskLogger.Info().Msg("Executing direct script")
					}
					action = &proto.Action{
						Type:    "EXECUTE_COMMAND",
						Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: task.Script}},
					}
					actionStr = task.Script
				} else {
					if goalText != "" {
						taskLogger.Info().Msgf("Resolving goal with LLM: %s", goalText)
					} else {
						taskLogger.Info().Msg("Resolving goal with LLM")
					}
					shareContent := true
					if pipeline.LlmContentSharing != nil {
						shareContent = *pipeline.LlmContentSharing
					}

					var directoryListing map[string]string
					if shareContent {
						taskLogger.Debug().Msg("Content sharing is ENABLED for this pipeline. Scanning directory")
						directoryListing = getDirectoryListing(taskLogger, "/workspace", pipeline.LlmContentIgnore)
						if len(directoryListing) == 0 {
							taskLogger.Debug().Msg("Sharing directory listing metadata with LLM agent (empty)")
						} else {
							fileNames := make([]string, 0, len(directoryListing))
							for name := range directoryListing {
								fileNames = append(fileNames, name)
							}
							sort.Strings(fileNames)
							maxLoggedFiles := 5
							if len(fileNames) < maxLoggedFiles {
								maxLoggedFiles = len(fileNames)
							}
							evt := taskLogger.Debug().Int("directory_file_count", len(directoryListing)).Strs("directory_file_sample", fileNames[:maxLoggedFiles])
							if len(fileNames) > maxLoggedFiles {
								evt = evt.Int("directory_file_remaining", len(fileNames)-maxLoggedFiles)
							}
							evt.Msg("Sharing directory listing metadata with LLM agent")
						}
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
						Goal:             goalText,
						History:          historySnapshot,
						DirectoryListing: directoryListing,
						Environment:      envMap,
					}

					err = withRetry(func() error {
						ctx, cancel := context.WithTimeout(context.Background(), llmAgentTimeout)
						defer cancel()
						var e error
						action, e = llmClient.GetAction(ctx, req)
						return e
					}, 3, 1*time.Second)
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
						actionStr = goalText
					}
				}

				debugLogger := taskLogger.With().
					Str("action_type", action.Type).
					Logger()
				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				var stdout, stderr string
				var exitCode int

				// Retry logic for potential race conditions (e.g. filesystem locks)
				for attempt := 0; attempt < 10; attempt++ {
					stdout, stderr, exitCode = executeAction(cli, stepContainerID, action, stepEnvVars)
					if exitCode == 0 {
						break
					}
					// Check for common race condition errors in stderr/stdout?
					// For now, retry all non-zero exits as robust fallback.
					time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				}

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
				historyGoal = goalText
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
					finalizeTask(stepName, task.Name, "success", exitCode, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: true}
				} else {
					if task.IgnoreFailure {
						finalizeTask(stepName, task.Name, "failure (ignored)", exitCode, llmDurationMs)
						taskLogger.Warn().Msg("Task failed, but failure is ignored")
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					} else {
						finalizeTask(stepName, task.Name, "failure", exitCode, llmDurationMs)
						taskLogger.Error().Msg("Critical task failed")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					}
				}
			}(runnable)
		}

		wg.Wait()
		close(results)

		for result := range results {
			if !result.Skipped {
				if result.Success {
					completedTasks[result.Name] = true
				} else {
					pipelineFailed = true
				}
			} else {
				completedTasks[result.Name] = true
			}
		}

		if timeoutTriggered.Load() {
			pipelineFailed = true
			break
		}

		if pipelineFailed {
			break
		}
	}

	syncWg.Wait()

	finalStatus := "success"
	if timeoutTriggered.Load() {
		finalStatus = "failure"
		agentLog(runID, pipeline.Name).Error().Msg("Pipeline timed out before completion")
	} else if pipelineFailed {
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

func withRetry(op func() error, attempts int, initialBackoff time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(initialBackoff * time.Duration(1<<uint(i-1)))
		}
		if err = op(); err == nil {
			return nil
		}
	}
	return err
}
