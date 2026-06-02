package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/pkg/serviceauth"
	"nopsai/pkg/servicetls"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
)

const agentWorkspaceDir = models.DefaultPipelineWorkingDirectory

const (
	resumeCheckpointIDEnv                   = "RESUME_CHECKPOINT_ID"
	approvalCheckpointMaxBytesEnv           = "NOPSAI_APPROVAL_CHECKPOINT_MAX_BYTES"
	defaultApprovalCheckpointMaxBytes int64 = 50 * 1024 * 1024
)

type agentApprovalPauseRequest struct {
	StepName               string            `json:"step_name"`
	TaskName               string            `json:"task_name"`
	ExecutionHistory       string            `json:"execution_history"`
	CompletedTasks         []string          `json:"completed_tasks"`
	PipelineDefinitionYAML string            `json:"pipeline_definition_yaml"`
	Variables              map[string]string `json:"variables,omitempty"`
	WorkspaceArchiveBase64 string            `json:"workspace_archive_base64"`
	SharedVolumeName       string            `json:"shared_volume_name,omitempty"`
	RunnerID               string            `json:"runner_id,omitempty"`
}

type agentApprovalPauseResponse struct {
	ApprovalID   string `json:"approval_id"`
	CheckpointID string `json:"checkpoint_id"`
	Status       string `json:"status"`
}

type agentApprovalCheckpointResponse struct {
	CheckpointID           string            `json:"checkpoint_id"`
	RunID                  string            `json:"run_id"`
	StepName               string            `json:"step_name"`
	ExecutionHistory       string            `json:"execution_history"`
	CompletedTasks         []string          `json:"completed_tasks"`
	PipelineDefinitionYAML string            `json:"pipeline_definition_yaml"`
	Variables              map[string]string `json:"variables,omitempty"`
	WorkspaceArchiveBase64 string            `json:"workspace_archive_base64,omitempty"`
	WorkspaceArchiveFormat string            `json:"workspace_archive_format,omitempty"`
}

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

func (m Matcher) Matches(relPath string, isDir bool) bool {
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

func buildPathMatchers(patterns []string) []Matcher {
	var matchers []Matcher
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "#") {
			continue
		}
		p = filepath.ToSlash(p)
		p = strings.TrimPrefix(p, "./")
		isDir := strings.HasSuffix(p, "/")
		pattern := strings.TrimSuffix(p, "/")
		if pattern == "" {
			continue
		}
		matchers = append(matchers, Matcher{
			Pattern:  pattern,
			IsDir:    isDir,
			IsGlobal: !strings.Contains(pattern, "/"),
		})
	}
	return matchers
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

func isIncluded(path string, matchers []Matcher, root string, isDir bool) bool {
	if len(matchers) == 0 {
		return true
	}
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)

	for _, matcher := range matchers {
		if matcher.Matches(relPath, isDir) {
			return true
		}
	}
	if isDir {
		return false
	}

	dir := filepath.ToSlash(filepath.Dir(relPath))
	for dir != "." && dir != "/" && dir != "" {
		for _, matcher := range matchers {
			if matcher.Matches(dir, true) {
				return true
			}
		}
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			break
		}
		dir = parent
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
	Paused    bool
	Condition string
}

// Helper struct to manage task execution
type RunnableTask struct {
	Step      *models.PipelineStep
	Task      *models.Task
	GlobalKey string
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

var dispatcherClient proto.DispatcherServiceClient

func sanitizeInput(name string) string {
	sanitized := strings.ReplaceAll(name, " ", "-")
	return nonAlphanumericRegex.ReplaceAllString(sanitized, "")
}

// getDirectoryListing recursively walks the specified root directory.
func getDirectoryListing(logger *zerolog.Logger, root string, includePatterns, ignorePatterns []string) map[string]string {
	directoryListing := make(map[string]string)
	includeMatchers := buildPathMatchers(includePatterns)
	ignoreMatchers := buildPathMatchers(ignorePatterns)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error().Err(err).Str("path", path).Msg("Error accessing path")
			return nil
		}

		// Check if the path should be ignored by the patterns from the pipeline directive
		if isIgnored(path, ignoreMatchers, root, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir // Skip the entire directory
			}
			return nil // Skip this file
		}

		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if !info.IsDir() {
			if !isIncluded(path, includeMatchers, root, false) {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				logger.Error().Err(readErr).Str("file", path).Msg("Failed to read file")
				return nil
			}
			relPath, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			relPath = filepath.ToSlash(relPath)
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

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func resolveActionFilePath(workingDirectory, actionPath string) (string, error) {
	resolvedWorkingDirectory, err := models.NormalizePipelineWorkingDirectory(workingDirectory)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(actionPath)
	if trimmed == "" {
		return "", fmt.Errorf("file action path cannot be empty")
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	cleaned := strings.TrimPrefix(path.Clean(normalized), "/")
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("file action path cannot be empty")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("file action path cannot escape working directory")
	}

	return path.Join(resolvedWorkingDirectory, cleaned), nil
}

// executeAction runs the given action inside the pipeline container.
func executeAction(cli *client.Client, containerID string, action *proto.Action, runtimeVars []string, workingDirectory string) (string, string, int) {
	resolvedWorkingDirectory, err := models.NormalizePipelineWorkingDirectory(workingDirectory)
	if err != nil {
		return "", err.Error(), 1
	}

	var cmdStr string

	switch action.Type {
	case "EXECUTE_COMMAND":
		cmdStr = action.GetCommandAction().Command
	case "REPLACE_FILE":
		content := action.GetFileAction().Content
		encodedContent := base64.StdEncoding.EncodeToString([]byte(content))
		filePath, err := resolveActionFilePath(resolvedWorkingDirectory, action.GetFileAction().Path)
		if err != nil {
			return "", err.Error(), 1
		}
		cmdStr = fmt.Sprintf("printf %%s %s | base64 -d > %s", shellQuote(encodedContent), shellQuote(filePath))
	case "RETURN_ANSWER":
		ansAction := action.GetAnswerAction()
		if ansAction == nil {
			return "", "Invalid answer action payload", 1
		}
		return ansAction.Answer, "", 0
	default:
		return "", "Unknown action type", 1
	}

	execConfig := client.ExecCreateOptions{
		Cmd:          []string{"sh", "-c", cmdStr},
		Env:          runtimeVars,
		WorkingDir:   resolvedWorkingDirectory,
		AttachStdout: true,
		AttachStderr: true,
		TTY:          false,
	}

	execID, err := cli.ExecCreate(context.Background(), containerID, execConfig)
	if err != nil {
		return "", fmt.Sprintf("failed to create exec: %v", err), 1
	}

	resp, err := cli.ExecAttach(context.Background(), execID.ID, client.ExecAttachOptions{})
	if err != nil {
		return "", fmt.Sprintf("failed to attach to exec: %v", err), 1
	}
	defer resp.Close()

	var stdout, stderr bytes.Buffer
	_, err = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)
	if err != nil {
		return "", fmt.Sprintf("failed to read output: %v", err), 1
	}

	inspect, err := cli.ExecInspect(context.Background(), execID.ID, client.ExecInspectOptions{})
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

func countPipelineTasks(pipeline *models.Pipeline) int {
	totalTasks := 0
	if pipeline == nil {
		return totalTasks
	}

	for _, step := range pipeline.Steps {
		if tasks := step.GetTasks(); len(tasks) > 0 {
			totalTasks += len(tasks)
		} else {
			totalTasks++
		}
	}
	return totalTasks
}

func buildImagePullQueue(pipeline *models.Pipeline, totalTasks int) []string {
	queue := make([]string, 0)
	if pipeline == nil || totalTasks == 0 {
		return queue
	}

	seen := make(map[string]bool)
	simulatedCompleted := make(map[string]bool)

	for len(simulatedCompleted) < totalTasks {
		runnable := getNextRunnableTasks(pipeline, simulatedCompleted)
		if len(runnable) == 0 {
			break
		}

		for _, r := range runnable {
			if _, ok := r.Step.AsApprovalStep(); ok {
				simulatedCompleted[r.GlobalKey] = true
				continue
			}
			image := r.Step.GetImage()
			if image == "" {
				image = pipeline.ContainerImage
			}

			if image != "" && !seen[image] {
				queue = append(queue, image)
				seen[image] = true
			}
			simulatedCompleted[r.GlobalKey] = true
		}
	}

	return queue
}

func notifyFinalStatus(pipelineName, runID, status string) {
	if dispatcherClient == nil {
		agentLog(runID, pipelineName).Error().Msg("Dispatcher client not initialized. Cannot report final status")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := dispatcherClient.FinalizeRun(ctx, &proto.FinalizeRunRequest{
		RunId:  runID,
		Status: status,
	})
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to send final status to dispatcher")
		return
	}

	agentLog(runID, pipelineName).Info().Str("status", status).Msg("Successfully notified dispatcher of final pipeline status")
}

// updateTaskStatus reports the final status of a task back through the dispatcher.
func updateTaskStatus(pipelineName, runID, stepName, taskName, status string, exitCode int, llmDurationMs int64) {
	if dispatcherClient == nil {
		stepLog(runID, pipelineName, stepName, taskName).Error().Msg("Dispatcher client not initialized. Cannot report status")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := dispatcherClient.ReportTaskStatus(ctx, &proto.TaskStatusReport{
		RunId:         runID,
		StepName:      stepName,
		TaskName:      taskName,
		Status:        status,
		ExitCode:      int32(exitCode),
		LlmDurationMs: llmDurationMs,
	})
	if err != nil {
		stepLog(runID, pipelineName, stepName, taskName).Error().Err(err).Msg("Failed to send status update to dispatcher")
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
	if _, err := cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to stop pipeline container")
	}

	waitResult := cli.ContainerWait(ctx, containerID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case err := <-waitResult.Error:
		if err != nil {
			agentLog(runID, pipelineName).Error().Err(err).Msg("Error waiting for container to stop")
		}
	case <-waitResult.Result:
	}

	if _, err := cli.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{Force: true}); err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to remove pipeline container")
	}
}

func ensureImageExists(ctx context.Context, logger *zerolog.Logger, cli *client.Client, imageName string) error {
	imageFilters := make(client.Filters).Add("reference", imageName)
	images, err := cli.ImageList(ctx, client.ImageListOptions{Filters: imageFilters})
	if err != nil {
		return fmt.Errorf("failed to list images to check for %s: %w", imageName, err)
	}

	if len(images.Items) == 0 {
		logger.Info().Str("image", imageName).Msg("Image not found locally, pulling")
		out, err := cli.ImagePull(ctx, imageName, client.ImagePullOptions{})
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

func startImagePrePull(ctx context.Context, cli *client.Client, pipeline *models.Pipeline, runID string, totalTasks int) {
	if cli == nil || pipeline == nil {
		return
	}

	queue := buildImagePullQueue(pipeline, totalTasks)
	if len(queue) == 0 {
		agentLog(runID, pipeline.Name).Debug().Msg("No images to pre-pull for pipeline")
		return
	}

	if ctx == nil {
		ctx = context.Background()
	}

	prePullLogger := agentLog(runID, pipeline.Name).With().Str("component", "image-prepull").Logger()
	prePullLogger.Info().Int("count", len(queue)).Msg("Starting asynchronous image pre-pull")

	go func() {
		for i, imageName := range queue {
			select {
			case <-ctx.Done():
				prePullLogger.Warn().Msg("Stopping image pre-pull due to cancellation")
				return
			default:
			}

			imageLogger := prePullLogger.With().
				Str("image", imageName).
				Int("position", i+1).
				Int("total", len(queue)).
				Logger()

			if err := ensureImageExists(ctx, &imageLogger, cli, imageName); err != nil {
				imageLogger.Warn().Err(err).Msg("Failed to pre-pull image; will pull on demand during execution")
			}
		}
	}()
}

func approvalCheckpointMaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv(approvalCheckpointMaxBytesEnv))
	if raw == "" {
		return defaultApprovalCheckpointMaxBytes
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return defaultApprovalCheckpointMaxBytes
	}
	return value
}

type limitedBuffer struct {
	buf bytes.Buffer
	max int64
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	if w == nil {
		return 0, fmt.Errorf("checkpoint writer is not configured")
	}
	if w.max > 0 && int64(w.buf.Len()+len(p)) > w.max {
		return 0, fmt.Errorf("workspace checkpoint exceeds %d bytes", w.max)
	}
	return w.buf.Write(p)
}

func (w *limitedBuffer) Bytes() []byte {
	if w == nil {
		return nil
	}
	return w.buf.Bytes()
}

func archiveWorkspace(root string, maxBytes int64) ([]byte, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil, fmt.Errorf("workspace root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}

	limited := &limitedBuffer{max: maxBytes}
	gzipWriter := gzip.NewWriter(limited)
	tarWriter := tar.NewWriter(gzipWriter)
	walkErr := filepath.Walk(root, func(entryPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if entryPath == root {
			return nil
		}
		mode := info.Mode()
		if !mode.IsRegular() && !info.IsDir() && mode&os.ModeSymlink == 0 {
			return nil
		}

		rel, err := filepath.Rel(root, entryPath)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		linkName := ""
		if mode&os.ModeSymlink != 0 {
			linkName, err = os.Readlink(entryPath)
			if err != nil {
				return err
			}
		}
		header, err := tar.FileInfoHeader(info, linkName)
		if err != nil {
			return err
		}
		header.Name = rel
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !mode.IsRegular() {
			return nil
		}
		file, err := os.Open(entryPath)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tarWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeErr := tarWriter.Close()
	gzipErr := gzipWriter.Close()
	if walkErr != nil {
		return nil, fmt.Errorf("archive workspace: %w", walkErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("finalize workspace archive: %w", closeErr)
	}
	if gzipErr != nil {
		return nil, fmt.Errorf("compress workspace archive: %w", gzipErr)
	}
	return limited.Bytes(), nil
}

func restoreWorkspaceArchive(root string, archive []byte) error {
	if len(archive) == 0 {
		return nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return fmt.Errorf("workspace root is required")
	}
	if err := clearWorkspaceContents(root); err != nil {
		return err
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open workspace archive: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read workspace archive: %w", err)
		}

		relPath, err := safeArchiveRelativePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(root, relPath)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tarReader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := ensureSafeSymlinkTarget(relPath, header.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func clearWorkspaceContents(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read workspace root: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return fmt.Errorf("clear workspace entry %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func safeArchiveRelativePath(name string) (string, error) {
	rel := filepath.Clean(filepath.FromSlash(strings.TrimSpace(name)))
	if rel == "." || rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("workspace archive contains unsafe path %q", name)
	}
	return rel, nil
}

func ensureSafeSymlinkTarget(relPath, linkName string) error {
	linkName = strings.TrimSpace(linkName)
	if linkName == "" || filepath.IsAbs(linkName) {
		return fmt.Errorf("workspace archive contains unsafe symlink target")
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(relPath), filepath.FromSlash(linkName)))
	if target == ".." || strings.HasPrefix(target, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("workspace archive contains symlink escaping workspace")
	}
	return nil
}

func completedTaskKeysSnapshot(completedTasks map[string]bool) []string {
	keys := make([]string, 0, len(completedTasks))
	for key, done := range completedTasks {
		if done {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func firstApprovalRunnable(runnableTasks []*RunnableTask) *RunnableTask {
	for _, runnable := range runnableTasks {
		if runnable == nil || runnable.Step == nil {
			continue
		}
		if _, ok := runnable.Step.AsApprovalStep(); ok {
			return runnable
		}
	}
	return nil
}

func nopsaiAgentRequest(ctx context.Context, method, endpoint string, payload any, out any) error {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("NOPSAI_API_URL")), "/")
	if baseURL == "" {
		return fmt.Errorf("NOPSAI_API_URL is not configured")
	}
	credentials, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: os.Getenv(serviceauth.EnvSigningKey),
		Issuer:     os.Getenv(serviceauth.EnvIssuer),
		Audience:   os.Getenv(serviceauth.EnvAudience),
		Role:       serviceauth.RoleAgent,
		ServiceID:  os.Getenv(serviceauth.EnvServiceID),
	})
	if err != nil {
		return err
	}
	token, err := credentials.MintToken(ctx)
	if err != nil {
		return err
	}

	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+"/"+strings.TrimLeft(endpoint, "/"), body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("nopsai api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func requestApprovalPause(ctx context.Context, runID string, req agentApprovalPauseRequest) (agentApprovalPauseResponse, error) {
	var resp agentApprovalPauseResponse
	err := nopsaiAgentRequest(ctx, http.MethodPost, fmt.Sprintf("/v1/internal/runs/%s/approvals/pause", runID), req, &resp)
	return resp, err
}

func fetchApprovalCheckpoint(ctx context.Context, runID, checkpointID string) (agentApprovalCheckpointResponse, error) {
	var resp agentApprovalCheckpointResponse
	err := nopsaiAgentRequest(ctx, http.MethodGet, fmt.Sprintf("/v1/internal/runs/%s/checkpoints/%s", runID, checkpointID), nil, &resp)
	return resp, err
}

func triggerPipeline(parentRunID, parentPipelineName, parentStepName, pipelineIdentifier string, pipelineDef []byte, history string) (string, error) {
	if dispatcherClient == nil {
		return "", fmt.Errorf("dispatcher client not initialized")
	}

	scope := os.Getenv("SCOPE")
	parentRunnerID := os.Getenv("RUNNER_ID")
	gitContext := make(map[string]string)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GIT_") {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				gitContext[parts[0]] = parts[1]
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := dispatcherClient.TriggerPipeline(ctx, &proto.TriggerPipelineRequest{
		ParentRunId:        parentRunID,
		ParentRunnerId:     parentRunnerID,
		ParentPipelineName: parentPipelineName,
		ParentStepName:     parentStepName,
		PipelineIdentifier: pipelineIdentifier,
		PipelineDefinition: pipelineDef,
		History:            history,
		Scope:              scope,
		GitContext:         gitContext,
	})
	if err != nil {
		return "", fmt.Errorf("dispatcher trigger pipeline: %w", err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("dispatcher trigger pipeline: %s", resp.Error)
	}
	if strings.TrimSpace(resp.RunId) == "" {
		return "", fmt.Errorf("dispatcher returned empty run id for child pipeline")
	}
	return resp.RunId, nil
}

func monitorPipeline(logger *zerolog.Logger, runID string) (string, error) {
	if dispatcherClient == nil {
		return "", fmt.Errorf("dispatcher client not initialized")
	}

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
			reqCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			resp, err := dispatcherClient.GetRunStatus(reqCtx, &proto.RunStatusRequest{RunId: runID})
			cancel()
			if err != nil {
				childLogger.Error().Err(err).Msg("Failed to poll child pipeline status via dispatcher")
				continue
			}

			status := resp.GetStatus()
			childLogger.Info().Str("status", status).Msg("Polling child pipeline status")
			if status == "success" || status == "failure" || status == "cancelled" || status == "timed_out" || status == "rejected" {
				return status, nil
			}
		case <-ctx.Done():
			return "failure", fmt.Errorf("timed out waiting for child pipeline %s to complete", runID)
		}
	}
}

func watchRunCancellation(pipelineName, runID string, onCancel func()) {
	if dispatcherClient == nil || strings.TrimSpace(runID) == "" || onCancel == nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := dispatcherClient.GetRunStatus(reqCtx, &proto.RunStatusRequest{RunId: runID})
		cancel()
		if err != nil {
			agentLog(runID, pipelineName).Warn().Err(err).Msg("Failed to poll run status for cancellation")
			continue
		}

		if strings.EqualFold(strings.TrimSpace(resp.GetStatus()), "cancelled") {
			agentLog(runID, pipelineName).Warn().Msg("Run was cancelled. Cleaning up and exiting")
			onCancel()
			return
		}
	}
}

func getPipelineDef(pipelineName string) ([]byte, error) {
	if dispatcherClient == nil {
		return nil, fmt.Errorf("dispatcher client not initialized")
	}

	repoOwner := os.Getenv("GIT_REPO_OWNER")
	repoName := os.Getenv("GIT_REPO_NAME")
	commitSHA := os.Getenv("GIT_COMMIT_SHA")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := dispatcherClient.FetchPipeline(ctx, &proto.FetchPipelineRequest{
		PipelineName: pipelineName,
		RepoOwner:    repoOwner,
		RepoName:     repoName,
		CommitSha:    commitSHA,
	})
	if err != nil {
		return nil, fmt.Errorf("dispatcher fetch pipeline: %w", err)
	}
	if len(resp.GetPipelineDefinition()) == 0 {
		return nil, fmt.Errorf("dispatcher returned empty pipeline definition")
	}
	return resp.PipelineDefinition, nil
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
	pipelineDefBase64 := os.Getenv("PIPELINE_DEFINITION")
	parentHistoryBase64 := os.Getenv("PARENT_EXECUTION_HISTORY")
	sharedVolumeName := os.Getenv("SHARED_VOLUME_NAME")
	pipelineTimeoutStr := os.Getenv("PIPELINE_TIMEOUT")
	dockerNetworkName := os.Getenv("DOCKER_NETWORK_NAME")
	llmTimeoutStr := os.Getenv("LLM_AGENT_TIMEOUT")
	secretsBase64 := os.Getenv("NOPSAI_SECRETS")
	variablesBase64 := os.Getenv("NOPSAI_VARIABLES")
	resumeCheckpointID := os.Getenv(resumeCheckpointIDEnv)
	runScope := os.Getenv("SCOPE")

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

	var variables map[string]string
	if variablesBase64 != "" {
		variablesJSON, err := base64.StdEncoding.DecodeString(variablesBase64)
		if err != nil {
			agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to decode variables payload")
		} else {
			if err := json.Unmarshal(variablesJSON, &variables); err != nil {
				agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to unmarshal variables payload")
			}
		}
	}
	if variables == nil {
		variables = map[string]string{}
	}

	if llmTimeoutStr == "" {
		llmTimeoutStr = "2m"
	}
	llmTimeout, err := time.ParseDuration(llmTimeoutStr)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid LLM timeout duration")
		return 1
	}

	if runID == "" || pipelineDefBase64 == "" || pipelineName == "" || sharedVolumeName == "" {
		agentLog(runID, pipelineName).Error().Msg("Missing one or more required runtime variables")
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

	var resumeCheckpoint *agentApprovalCheckpointResponse
	if strings.TrimSpace(resumeCheckpointID) != "" {
		checkpoint, err := fetchApprovalCheckpoint(context.Background(), runID, strings.TrimSpace(resumeCheckpointID))
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Str("checkpoint_id", resumeCheckpointID).Msg("Failed to fetch approval checkpoint")
			return 1
		}
		if checkpoint.WorkspaceArchiveBase64 != "" {
			archiveBytes, err := base64.StdEncoding.DecodeString(checkpoint.WorkspaceArchiveBase64)
			if err != nil {
				agentLog(runID, pipeline.Name).Error().Err(err).Str("checkpoint_id", resumeCheckpointID).Msg("Failed to decode approval workspace archive")
				return 1
			}
			if err := restoreWorkspaceArchive(agentWorkspaceDir, archiveBytes); err != nil {
				agentLog(runID, pipeline.Name).Error().Err(err).Str("checkpoint_id", resumeCheckpointID).Msg("Failed to restore approval workspace checkpoint")
				return 1
			}
		}
		if checkpoint.Variables != nil {
			variables = checkpoint.Variables
		}
		resumeCheckpoint = &checkpoint
		agentLog(runID, pipeline.Name).Info().
			Str("checkpoint_id", checkpoint.CheckpointID).
			Str("step", checkpoint.StepName).
			Int("completed_tasks", len(checkpoint.CompletedTasks)).
			Msg("Restored approval checkpoint")
	}
	pipelineLLMEnabled := models.PipelineLLMEnabled(&pipeline)

	var llmRegistry *LLMProfileRegistry
	if pipelineLLMEnabled {
		llmRegistry, err = NewLLMProfileRegistryFromEnv(runScope)
		if err != nil {
			agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid LLM profile configuration")
			return 1
		}
	}
	mcpRegistry, err := NewMCPProfileRegistryFromEnv(runScope)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid MCP registry configuration")
		return 1
	}

	dispatcherAddr := os.Getenv("DISPATCHER_ADDRESS")
	if dispatcherAddr == "" {
		agentLog(runID, pipelineName).Error().Msg("DISPATCHER_ADDRESS OS variable not set. Cannot contact dispatcher")
		return 1
	}
	dispatcherServiceID := os.Getenv(serviceauth.EnvServiceID)
	if strings.TrimSpace(dispatcherServiceID) == "" {
		dispatcherServiceID = os.Getenv("AGENT_SERVICE_ID")
	}
	dispatcherCreds, err := serviceauth.NewCredentials(serviceauth.Config{
		SigningKey: os.Getenv(serviceauth.EnvSigningKey),
		Issuer:     os.Getenv(serviceauth.EnvIssuer),
		Audience:   os.Getenv(serviceauth.EnvAudience),
		Role:       serviceauth.RoleAgent,
		ServiceID:  dispatcherServiceID,
	})
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to configure dispatcher client authentication")
		return 1
	}
	dispatcherTLSSecret := strings.TrimSpace(os.Getenv(servicetls.EnvSecret))
	if dispatcherTLSSecret == "" {
		dispatcherTLSSecret = os.Getenv(serviceauth.EnvSigningKey)
	}
	transportCreds, err := servicetls.ClientCredentials(servicetls.Config{
		Mode:       os.Getenv(servicetls.EnvMode),
		Secret:     dispatcherTLSSecret,
		Role:       serviceauth.RoleAgent,
		ServiceID:  dispatcherServiceID,
		ServerName: os.Getenv(servicetls.EnvServerName),
	})
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to configure dispatcher transport security")
		return 1
	}
	conn, err := grpc.Dial(
		dispatcherAddr,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithPerRPCCredentials(dispatcherCreds),
	)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Str("dispatcher_addr", dispatcherAddr).Msg("Failed to connect to dispatcher")
		return 1
	}
	defer conn.Close()
	dispatcherClient = proto.NewDispatcherServiceClient(conn)

	knowledgeSnapshots, err := loadRuntimeKnowledgeContexts()
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Failed to load knowledge context snapshots")
		return 1
	}
	workingDirectory, err := models.NormalizePipelineWorkingDirectory(pipeline.WorkingDirectory)
	if err != nil {
		agentLog(runID, pipelineName).Error().Err(err).Msg("Invalid pipeline working_directory")
		return 1
	}

	if triggerEventID == "" {
		triggerEventID = "N/A"
	}
	agentLog(runID, pipeline.Name).Info().Str("trigger_event_id", triggerEventID).Str("working_directory", workingDirectory).Msg("Pipeline execution starting")
	if pipelineLLMEnabled {
		defaultLLMProfile, _ := llmRegistry.DefaultProfile()
		startupLog := agentLog(runID, pipeline.Name).Info().
			Str("llm_profile", llmRegistry.DefaultProfileName()).
			Str("llm_provider", defaultLLMProfile.Provider)
		switch defaultLLMProfile.Provider {
		case appconfig.LLMProviderGemini:
			startupLog.Str("llm_model", defaultLLMProfile.Model).Msg("Agent starting with embedded LLM profile registry")
		case appconfig.LLMProviderLMStudio:
			logEvent := startupLog.Str("lmstudio_base_url", defaultLLMProfile.BaseURL)
			if strings.TrimSpace(defaultLLMProfile.Model) != "" {
				logEvent = logEvent.Str("llm_model", defaultLLMProfile.Model)
			} else {
				logEvent = logEvent.Str("llm_model", "auto-discover")
			}
			if defaultLLMProfile.Reasoning != "" {
				logEvent = logEvent.Str("lmstudio_reasoning", defaultLLMProfile.Reasoning)
			}
			if defaultLLMProfile.Thinking != nil {
				logEvent = logEvent.Bool("lmstudio_thinking", *defaultLLMProfile.Thinking)
			}
			logEvent.Msg("Agent starting with embedded LLM profile registry")
		default:
			startupLog.Msg("Agent starting with embedded LLM profile registry")
		}
	} else {
		agentLog(runID, pipeline.Name).Info().Msg("LLM is disabled for this pipeline; LLM profile registry will not be loaded")
	}

	runtimeMode := appconfig.NormalizeRuntime(os.Getenv("NOPSAI_RUNTIME"))
	var cli *client.Client
	var k8sRuntime *kubernetesAgentRuntime
	if runtimeMode == kubernetesRuntimeName {
		k8sRuntime, err = newKubernetesAgentRuntimeFromEnv(runID, pipeline.Name, sharedVolumeName, pipeline.AffinityEnabled)
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to initialize Kubernetes runtime")
			return 1
		}
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to create Docker client")
			return 1
		}
		defer cli.Close()
	}

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
			if k8sRuntime != nil {
				k8sRuntime.cleanupPod(context.Background(), containerID)
			} else {
				cleanup(cli, containerID, pipeline.Name, runID)
			}
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

	go watchRunCancellation(pipeline.Name, runID, func() {
		cancelActiveTasks("run_cancelled")
		cleanupStepContainers("run_cancelled")
		os.Exit(0)
	})

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
	isRunStopping := func() bool {
		return timeoutTriggered.Load() || (timeoutCtx != nil && timeoutCtx.Err() != nil)
	}

	totalTasks := countPipelineTasks(&pipeline)

	prePullCtx := context.Background()
	if timeoutCtx != nil {
		prePullCtx = timeoutCtx
	}
	if cli != nil {
		startImagePrePull(prePullCtx, cli, &pipeline, runID, totalTasks)
	}

	history := new(strings.Builder)
	if resumeCheckpoint != nil {
		history.WriteString(resumeCheckpoint.ExecutionHistory)
		if resumeCheckpoint.ExecutionHistory != "" && !strings.HasSuffix(resumeCheckpoint.ExecutionHistory, "\n") {
			history.WriteString("\n")
		}
	} else if parentHistoryBase64 != "" {
		decodedHistory, err := base64.StdEncoding.DecodeString(parentHistoryBase64)
		if err != nil {
			agentLog(runID, pipeline.Name).Error().Err(err).Msg("Failed to decode parent execution history")
		} else {
			history.Write(decodedHistory)
			history.WriteString("\n--- Inherited History Above ---\n\n")
		}
	}
	completedTasks := make(map[string]bool)
	if resumeCheckpoint != nil {
		for _, key := range resumeCheckpoint.CompletedTasks {
			key = strings.TrimSpace(key)
			if key != "" {
				completedTasks[key] = true
			}
		}
	}
	pipelineFailed := false
	pipelinePaused := false
	var syncWg sync.WaitGroup

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
		if approvalRunnable := firstApprovalRunnable(runnableTasks); approvalRunnable != nil {
			runnableTasks = []*RunnableTask{approvalRunnable}
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
				var llmDurationMs int64
				inheritedEnv := os.Environ()
				stepContext, missingSecrets := buildStepExecutionContext(&pipeline, step, inheritedEnv, variables, secrets)
				for _, secretName := range missingSecrets {
					taskLogger.Warn().Str("secret", secretName).Msg("Secret was requested by step but not provided")
				}

				// --- CONDITION EVALUATION LOGIC START ---
				condition := strings.TrimSpace(step.GetCondition())
				if condition != "" {
					if !pipelineLLMEnabled || llmRegistry == nil {
						taskLogger.Error().Msg("Cannot evaluate condition because LLM is disabled for this pipeline")
						pipelineFailed = true
						finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					taskLogger.Info().Msgf("Evaluating condition for step '%s': \"%s\"", stepName, condition)

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()

					knowledgePrompt := buildEffectiveKnowledgeContextPrompt(&pipeline, step, nil, knowledgeSnapshots)
					req := stepContext.buildConditionRequest(condition, historySnapshot, knowledgePrompt, secrets)
					conditionClient, conditionProfile, profileErr := llmRegistry.ClientFor(&pipeline, step, nil)
					if profileErr != nil {
						taskLogger.Error().Err(profileErr).Msg("Failed to resolve LLM profile for condition")
						pipelineFailed = true
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					taskLogger.Info().Str("llm_profile", conditionProfile).Msg("Using LLM profile for condition")

					var resp *proto.ConditionResponse
					conditionStart := time.Now()
					err = withRetry(func() error {
						ctx, cancel := context.WithTimeout(context.Background(), llmTimeout)
						defer cancel()
						var e error
						resp, e = conditionClient.EvaluateCondition(ctx, req)
						return e
					}, 3, 1*time.Second)
					llmDurationMs = time.Since(conditionStart).Milliseconds()

					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to evaluate condition from LLM. Skipping step.")
						pipelineFailed = true // Mark failure to stop pipeline
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					if !resp.Result {
						blockingKinds := effectiveBlockingKnowledgeContextKinds(&pipeline, step, nil, knowledgeSnapshots)
						if len(blockingKinds) > 0 {
							taskLogger.Error().
								Strs("knowledge_context_kinds", blockingKinds).
								Msg("Condition evaluated to false under blocking knowledge context. Failing task.")
							finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						taskLogger.Info().Msg("Condition evaluated to false. Skipping task.")
						finalizeTask(stepName, task.Name, "skipped", 0, llmDurationMs)
						// We send a success result so the main loop can correctly count this as "handled".
						results <- TaskResult{Name: runnable.GlobalKey, Success: true, Skipped: true}
						return
					}
					taskLogger.Info().Msg("Condition evaluated to true. Proceeding with step.")
				}
				// --- CONDITION EVALUATION LOGIC END ---

				if approvalStep, ok := step.AsApprovalStep(); ok {
					setTaskRunning(stepName, task.Name)

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					completedSnapshot := completedTaskKeysSnapshot(completedTasks)

					workspaceArchive, err := archiveWorkspace(agentWorkspaceDir, approvalCheckpointMaxBytesFromEnv())
					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to archive workspace for approval checkpoint")
						finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					pauseResp, err := requestApprovalPause(context.Background(), runID, agentApprovalPauseRequest{
						StepName:               stepName,
						TaskName:               task.Name,
						ExecutionHistory:       historySnapshot,
						CompletedTasks:         completedSnapshot,
						PipelineDefinitionYAML: string(pipelineDefBytes),
						Variables:              variables,
						WorkspaceArchiveBase64: base64.StdEncoding.EncodeToString(workspaceArchive),
						SharedVolumeName:       sharedVolumeName,
						RunnerID:               os.Getenv("RUNNER_ID"),
					})
					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to request approval pause")
						finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					removeActiveTask(stepName, task.Name)
					taskLogger.Info().
						Str("approval_id", pauseResp.ApprovalID).
						Str("checkpoint_id", pauseResp.CheckpointID).
						Str("approval_type", approvalStep.Approval.Type).
						Strs("groups", approvalStep.Approval.Groups).
						Msg("Pipeline paused for approval")
					results <- TaskResult{Name: runnable.GlobalKey, Success: true, Paused: true}
					return
				}

				includeTarget := strings.TrimSpace(step.GetInclude())
				if includeTarget != "" {
					setTaskRunning(stepName, stepName)

					parts := strings.SplitN(includeTarget, ":", 2)
					if len(parts) != 2 || parts[0] != "pipeline" {
						taskLogger.Error().Str("include", includeTarget).Msg("Invalid include format")
						finalizeTask(stepName, stepName, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					childPipelineName := parts[1]

					childDef, err := getPipelineDef(childPipelineName)
					if err != nil {
						if strings.Contains(err.Error(), "nopsai api returned non-200 status 404") {
							taskLogger.Warn().Str("child_pipeline", childPipelineName).Msg("Child pipeline not found, marking as not found")
							finalizeTask(stepName, stepName, "not_found", 0, llmDurationMs)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false} // Treat as failure for dependency purposes
							return
						}

						taskLogger.Error().Err(err).Msg("Failed to get child pipeline definition")
						finalizeTask(stepName, stepName, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}

					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					childRunID, err := triggerPipeline(runID, pipelineName, stepName, childPipelineName, childDef, historySnapshot)
					if err != nil {
						taskLogger.Error().Err(err).Msg("Failed to trigger child pipeline")
						finalizeTask(stepName, stepName, "failure", 1, llmDurationMs)
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
						finalizeTask(stepName, stepName, finalStatus, 0, llmDurationMs)
						if finalStatus != "success" && step.GetSync() {
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
						finalizeTask(stepName, stepName, "success", 0, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: true}
					}
					return
				}

				var stepContainerID string
				setTaskRunning(stepName, task.Name)

				stepRuntimeVars := stepContext.containerVariables()
				taskContext := stepContext.withTask(task)
				taskRuntimeVars := taskContext.containerVariables()

				imageName := step.GetImage()
				if imageName == "" {
					imageName = pipeline.ContainerImage
				}

				sessionMutex.Lock()
				containerID, ok := sessionContainers[stepName]
				if !ok {
					if k8sRuntime != nil {
						taskLogger.Info().Str("image", imageName).Msg("Creating new Kubernetes pod for step")
						podName, err := k8sRuntime.createStepPod(context.Background(), kubernetesStepPodRequest{
							RunID:            runID,
							PipelineName:     pipelineName,
							StepName:         stepName,
							Image:            imageName,
							WorkingDirectory: workingDirectory,
							Env:              stepRuntimeVars,
							Volumes:          step.GetVolumes(),
							RuntimePool:      firstNonEmpty(step.GetRuntimePool(), pipeline.RuntimePool),
						})
						if err != nil {
							taskLogger.Error().Err(err).Msg("Failed to create step pod")
							finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
							sessionMutex.Unlock()
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						sessionContainers[stepName] = podName
						stepContainerID = podName
					} else {
						taskLogger.Info().Str("image", imageName).Msg("Creating new container for step")
						if err := ensureImageExists(context.Background(), taskLogger, cli, imageName); err != nil {
							taskLogger.Error().Err(err).Msg("Failed to ensure step image exists. Shutting down")
							finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
							sessionMutex.Unlock()
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}

						binds := []string{fmt.Sprintf("%s:%s", sharedVolumeName, workingDirectory)}
						if stepVolumes := step.GetVolumes(); len(stepVolumes) > 0 {
							for _, vol := range stepVolumes {
								parts := strings.Split(vol, ":")
								if len(parts) != 2 {
									taskLogger.Error().Str("volume", vol).Msg("Invalid volume format. Must be 'volume-name:mount-path'. Skipping")
									continue
								}
								volumeName := parts[0]
								_, err := cli.VolumeInspect(context.Background(), volumeName, client.VolumeInspectOptions{})
								if err != nil {
									if cerrdefs.IsNotFound(err) {
										taskLogger.Info().Str("volume", volumeName).Msg("Volume not found, creating it now")
										_, createErr := cli.VolumeCreate(context.Background(), client.VolumeCreateOptions{Name: volumeName})
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

						cont, err := cli.ContainerCreate(context.Background(), client.ContainerCreateOptions{
							Config: &container.Config{
								Image:      imageName,
								WorkingDir: workingDirectory,
								Entrypoint: []string{"tail", "-f", "/dev/null"},
								Env:        stepRuntimeVars,
								Tty:        false,
							},
							HostConfig: &container.HostConfig{
								Binds:       binds,
								NetworkMode: container.NetworkMode(dockerNetworkName),
							},
							Name: stepContainerName,
						})
						if err != nil {
							taskLogger.Error().Err(err).Msg("Failed to create step container")
							finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
							sessionMutex.Unlock()
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						if _, err := cli.ContainerStart(context.Background(), cont.ID, client.ContainerStartOptions{}); err != nil {
							taskLogger.Error().Err(err).Msg("Failed to start step container")
							finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
							sessionMutex.Unlock()
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						sessionContainers[stepName] = cont.ID
						stepContainerID = cont.ID
					}
				} else {
					if k8sRuntime != nil {
						taskLogger.Info().Msg("Reusing existing step pod")
					} else {
						taskLogger.Info().Msg("Reusing existing step container")
					}
					stepContainerID = containerID
				}
				sessionMutex.Unlock()

				var action *proto.Action
				var actionStr string
				var historyGoal string

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
					if !pipelineLLMEnabled || llmRegistry == nil {
						taskLogger.Error().Msg("Cannot resolve goal because LLM is disabled for this pipeline")
						finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
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
						directoryListing = getDirectoryListing(taskLogger, agentWorkspaceDir, pipeline.LlmContentInclude, pipeline.LlmContentIgnore)
						if len(directoryListing) == 0 {
							taskLogger.Debug().Msg("Sharing directory listing metadata with LLM (empty)")
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
							evt.Msg("Sharing directory listing metadata with LLM")
						}
					} else {
						taskLogger.Debug().Msg("Content sharing is DISABLED for this pipeline. Skipping directory scan")
						directoryListing = make(map[string]string)
					}
					historyMutex.Lock()
					historySnapshot := history.String()
					historyMutex.Unlock()
					knowledgePrompt := buildEffectiveKnowledgeContextPrompt(&pipeline, step, task, knowledgeSnapshots)
					req := taskContext.buildActionRequest(goalText, historySnapshot, directoryListing, knowledgePrompt, secrets)
					actionClient, actionProfile, profileErr := llmRegistry.ClientFor(&pipeline, step, task)
					if profileErr != nil {
						taskLogger.Error().Err(profileErr).Msg("Failed to resolve LLM profile for goal")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					taskLogger.Info().Str("llm_profile", actionProfile).Msg("Using LLM profile for goal")
					mcpRuntime, mcpErr := mcpRegistry.ResolveFor(&pipeline, step, task)
					if mcpErr != nil {
						taskLogger.Error().Err(mcpErr).Msg("Failed to resolve MCP profiles for goal")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					if mcpRuntime.Enabled() {
						mcpProfiles := mcpRuntime.Profiles()
						taskLogger.Info().
							Strs("mcp_profiles", mcpProfiles).
							Int("mcp_tool_count", len(mcpRuntime.tools)).
							Bool("mcp_requires_tool_call", mcpRuntime.RequiresToolCall()).
							Msgf("Using MCP profiles for goal (profiles=%s tools=%d require_tool_call=%t)", strings.Join(mcpProfiles, ","), len(mcpRuntime.tools), mcpRuntime.RequiresToolCall())
					}

					actionParentCtx := context.Background()
					if timeoutCtx != nil {
						actionParentCtx = timeoutCtx
					}
					actionStart := time.Now()
					err = withRetry(func() error {
						ctx, cancel := context.WithTimeout(actionParentCtx, llmTimeout)
						defer cancel()
						var e error
						action, e = actionClient.GetActionWithMCP(ctx, req, mcpRuntime)
						if e != nil {
							return e
						}
						if mcpRuntime.RequiresToolCall() && mcpRuntime.SuccessfulToolCalls() == 0 {
							action = nil
							return fmt.Errorf("MCP tool call is required before executing a final action")
						}
						return nil
					}, 3, 1*time.Second)
					llmDurationMs = time.Since(actionStart).Milliseconds()
					if err != nil {
						if isRunStopping() {
							taskLogger.Warn().Err(err).Msg("Goal resolution stopped because the run is cancelling")
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						if isNonRetryableGoalResolutionError(err) {
							failureReason := taskContext.maskText(err.Error(), secrets)
							taskLogger.Error().Err(err).Msgf("Goal resolution failed: %s", failureReason)
							if zerolog.GlobalLevel() <= zerolog.InfoLevel {
								taskLogger.Info().Msgf(`status=failure action="Resolve goal" output="%s"`, failureReason)
							}
							finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
						// One more best-effort retry to increase durability.
						taskLogger.Warn().Err(err).Msg("GetAction failed after retries; attempting one final retry")
						ctx, cancel := context.WithTimeout(actionParentCtx, llmTimeout)
						action, err = actionClient.GetActionWithMCP(ctx, req, mcpRuntime)
						cancel()
						if err == nil && mcpRuntime.RequiresToolCall() && mcpRuntime.SuccessfulToolCalls() == 0 {
							action = nil
							err = fmt.Errorf("MCP tool call is required before executing a final action")
						}
						llmDurationMs = time.Since(actionStart).Milliseconds()
						if err != nil {
							taskLogger.Error().Err(err).Msg("Failed to get action from LLM. Shutting down")
							results <- TaskResult{Name: runnable.GlobalKey, Success: false}
							return
						}
					}
					if isRunStopping() {
						taskLogger.Warn().Msg("Goal resolution finished after the run was cancelled; skipping action execution")
						results <- TaskResult{Name: runnable.GlobalKey, Success: false}
						return
					}
					if mcpRuntime.RequiresToolCall() {
						taskLogger.Info().
							Int("mcp_successful_tool_calls", mcpRuntime.SuccessfulToolCalls()).
							Msgf("MCP tool calls completed before final action (count=%d)", mcpRuntime.SuccessfulToolCalls())
					}

					if cmd := action.GetCommandAction(); cmd != nil {
						actionStr = cmd.Command
					} else if file := action.GetFileAction(); file != nil {
						actionStr = fmt.Sprintf("Write to %s", file.Path)
					} else if ans := action.GetAnswerAction(); ans != nil {
						actionStr = "Return answer"
					}
				}

				if failureReason, blockingKinds, ok := knowledgeContextViolationFailureReason(action, &pipeline, step, task, knowledgeSnapshots); ok {
					maskedReason := taskContext.maskText(failureReason, secrets)
					taskLogger.Error().
						Strs("knowledge_context_kinds", blockingKinds).
						Msgf("Knowledge context blocked task: %s", maskedReason)
					if zerolog.GlobalLevel() <= zerolog.InfoLevel {
						taskLogger.Info().Msgf(`status=failure action="Return answer" output="%s"`, maskedReason)
					}
					historyGoal = goalText
					if historyGoal == "" {
						historyGoal = fmt.Sprintf("Execute script for task: %s", task.Name)
					}
					historyMutex.Lock()
					history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: Return answer\n  Result (Exit Code 1): %s\n", historyGoal, maskedReason))
					historyMutex.Unlock()
					finalizeTask(stepName, task.Name, "failure", 1, llmDurationMs)
					results <- TaskResult{Name: runnable.GlobalKey, Success: false}
					return
				}

				debugLogger := taskLogger.With().
					Str("action_type", action.Type).
					Logger()
				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				var stdout, stderr string
				var exitCode int

				// Retry logic for potential race conditions (e.g. filesystem locks)
				for attempt := 0; attempt < 10; attempt++ {
					if k8sRuntime != nil {
						stdout, stderr, exitCode = k8sRuntime.executeAction(context.Background(), stepContainerID, action, taskRuntimeVars, workingDirectory)
					} else {
						stdout, stderr, exitCode = executeAction(cli, stepContainerID, action, taskRuntimeVars, workingDirectory)
					}
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
				maskedOutput := taskContext.maskText(output, secrets)
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
				} else {
					output = maskedOutput
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
			if result.Paused {
				pipelinePaused = true
				continue
			}
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

		if pipelinePaused {
			break
		}

		if pipelineFailed {
			break
		}
	}

	syncWg.Wait()

	if pipelinePaused {
		agentLog(runID, pipeline.Name).Info().Msg("Pipeline paused for approval")
		return 0
	}

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

func main() {
	os.Exit(run())
}

func withRetry(op func() error, attempts int, initialBackoff time.Duration) error {
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(initialBackoff * time.Duration(1<<uint(i-1)))
		}
		err = op()
		if err == nil {
			return nil
		}
		if isNonRetryableGoalResolutionError(err) {
			return err
		}
	}
	return err
}
