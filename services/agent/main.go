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
	"strings"
	"sync"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

type StepResult struct {
	Name    string
	Success bool
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
func executeAction(cli *client.Client, containerID string, action *proto.Action) (string, string, int) {
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

func getNextRunnableSteps(pipeline *models.Pipeline, completedSteps map[string]bool) []*models.PipelineStep {
	var runnableSteps []*models.PipelineStep
	for i := range pipeline.Steps {
		step := &pipeline.Steps[i]
		if _, done := completedSteps[step.Name]; done {
			continue
		}
		dependenciesMet := true
		for _, depName := range step.DependsOn {
			if _, done := completedSteps[depName]; !done {
				dependenciesMet = false
				break
			}
		}
		if dependenciesMet {
			runnableSteps = append(runnableSteps, step)
		}
	}
	return runnableSteps
}

// updateStepStatus reports the final status of a step back to the nopsai API.
func updateStepStatus(runID, stepName, status string, exitCode int) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		log.Error().Msg("NOPSAI_API_URL environment variable not set. Cannot report status.")
		return
	}
	url := fmt.Sprintf("%s/v1/runs/%s/steps/%s", nopsaiURL, runID, stepName)

	payload := map[string]interface{}{
		"status":    status,
		"exit_code": exitCode,
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

// updateRunStatus reports the final status of the entire run.
func updateRunStatus(runID, status string) {
	nopsaiURL := os.Getenv("NOPSAI_API_URL")
	if nopsaiURL == "" {
		log.Error().Msg("NOPSAI_API_URL environment variable not set. Cannot report final run status.")
		return
	}
	url := fmt.Sprintf("%s/v1/runs/%s/status", nopsaiURL, runID)

	payload := map[string]string{"status": status}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send final run status update to nopsai API")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Error().Int("status_code", resp.StatusCode).Msg("Received non-OK status from nopsai API for final run status")
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

func main() {
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
	sharedVolumeName := os.Getenv("SHARED_VOLUME_NAME")
	pipelineTimeoutStr := os.Getenv("PIPELINE_TIMEOUT")
	dockerNetworkName := os.Getenv("DOCKER_NETWORK_NAME")
	llmAgentTimeoutStr := os.Getenv("LLM_AGENT_TIMEOUT")
	if llmAgentTimeoutStr == "" {
		llmAgentTimeoutStr = "2m" // Default to 2 minutes
	}
	llmAgentTimeout, err := time.ParseDuration(llmAgentTimeoutStr)
	if err != nil {
		log.Fatal().Err(err).Msg("Invalid LLM agent timeout duration")
	}

	if runID == "" || llmAgentAddress == "" || pipelineDefBase64 == "" || pipelineName == "" || sharedVolumeName == "" {
		log.Fatal().Msg("Missing one or more required environment variables.")
	}

	pipelineDefBytes, err := base64.StdEncoding.DecodeString(pipelineDefBase64)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to decode pipeline definition")
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(pipelineDefBytes, &pipeline); err != nil {
		log.Fatal().Err(err).Msg("Failed to unmarshal pipeline definition")
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
		log.Fatal().Err(err).Msg("Failed to connect to LLM agent after multiple retries")
	}
	defer conn.Close()
	llmClient := proto.NewLLMServiceClient(conn)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Docker client")
	}
	defer cli.Close()

	if pipelineTimeoutStr != "" {
		timeout, err := time.ParseDuration(pipelineTimeoutStr)
		if err != nil {
			log.Error().Err(err).Msg("Invalid pipeline timeout duration")
		} else {
			log.Info().Msgf("Pipeline timeout is set to: %s", timeout)
			time.AfterFunc(timeout, func() {
				log.Error().Msg("Pipeline execution timed out. Cleaning up and exiting.")
				updateRunStatus(runID, "failed")
				os.Exit(1)
			})
		}
	}

	// Create and start the main pipeline container
	mainImageName := pipeline.ContainerImage
	if err := ensureImageExists(context.Background(), cli, mainImageName); err != nil {
		log.Fatal().Err(err).Msg("Failed to ensure main pipeline image exists")
	}

	pipelineEnvVars := []string{}
	for key, value := range pipeline.Environment {
		pipelineEnvVars = append(pipelineEnvVars, fmt.Sprintf("%s=%s", key, value))
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GIT_") {
			pipelineEnvVars = append(pipelineEnvVars, e)
		}
	}

	mainContainerName := fmt.Sprintf("pipeline-%s-main", runID)
	mainCont, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image:      mainImageName,
		WorkingDir: "/workspace",
		Entrypoint: []string{"tail", "-f", "/dev/null"},
		Env:        pipelineEnvVars,
		Tty:        false,
	}, &container.HostConfig{
		Binds:       []string{fmt.Sprintf("%s:/workspace", sharedVolumeName)},
		NetworkMode: container.NetworkMode(dockerNetworkName),
	}, nil, nil, mainContainerName)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create main pipeline container")
	}

	if err := cli.ContainerStart(context.Background(), mainCont.ID, container.StartOptions{}); err != nil {
		log.Fatal().Err(err).Msg("Failed to start main pipeline container")
	}
	defer cleanup(cli, mainCont.ID)

	history := new(strings.Builder)
	completedSteps := make(map[string]bool)

	for len(completedSteps) < len(pipeline.Steps) {
		runnableSteps := getNextRunnableSteps(&pipeline, completedSteps)
		if len(runnableSteps) == 0 {
			log.Info().Str("pipeline", pipeline.Name).Msg("All steps completed successfully.")
			break
		}

		var wg sync.WaitGroup
		results := make(chan StepResult, len(runnableSteps))
		historyMutex := &sync.Mutex{}

		for _, step := range runnableSteps {
			wg.Add(1)
			go func(step *models.PipelineStep) {
				defer wg.Done()

				var stepContainerID string
				var tempContainer bool

				imageName := step.Image
				if imageName == "" || imageName == mainImageName {
					log.Info().Str("step", step.Name).Msg("Executing in main pipeline container.")
					stepContainerID = mainCont.ID
					tempContainer = false
				} else {
					log.Info().Str("step", step.Name).Str("image", imageName).Msg("Preparing to run step in dedicated container")
					if err := ensureImageExists(context.Background(), cli, imageName); err != nil {
						log.Error().Err(err).Msg("Failed to ensure step image exists. Shutting down.")
						updateStepStatus(runID, step.Name, "failed", 1)
						results <- StepResult{Name: step.Name, Success: false}
						return
					}

					stepContainerName := fmt.Sprintf("pipeline-%s-step-%s", runID, step.Name)
					cont, err := cli.ContainerCreate(context.Background(), &container.Config{
						Image:      imageName,
						WorkingDir: "/workspace",
						Entrypoint: []string{"tail", "-f", "/dev/null"},
						Env:        pipelineEnvVars,
						Tty:        false,
					}, &container.HostConfig{
						Binds:       []string{fmt.Sprintf("%s:/workspace", sharedVolumeName)},
						NetworkMode: container.NetworkMode(dockerNetworkName),
					}, nil, nil, stepContainerName)
					if err != nil {
						log.Fatal().Err(err).Msg("Failed to create pipeline step container")
					}

					if err := cli.ContainerStart(context.Background(), cont.ID, container.StartOptions{}); err != nil {
						log.Fatal().Err(err).Msg("Failed to start pipeline step container")
					}
					stepContainerID = cont.ID
					tempContainer = true
				}
				if tempContainer {
					defer cleanup(cli, stepContainerID)
				}

				var action *proto.Action
				var actionStr string
				var historyGoal string

				if step.Script != "" {
					log.Info().Str("step", step.Name).Msg("Executing direct script.")
					action = &proto.Action{
						Type:    "EXECUTE_COMMAND",
						Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: step.Script}},
					}
					actionStr = step.Script
				} else {
					log.Info().Str("step", step.Name).Msg("Resolving goal with LLM.")
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
					req := &proto.GetActionRequest{
						Goal:             step.Goal,
						History:          historySnapshot,
						DirectoryListing: directoryListing,
						Environment:      pipeline.Environment,
					}

					ctx, cancel := context.WithTimeout(context.Background(), llmAgentTimeout)
					action, err = llmClient.GetAction(ctx, req)
					cancel()
					if err != nil {
						log.Error().Err(err).Str("step", step.Name).Msg("Failed to get action from LLM agent. Shutting down.")
						results <- StepResult{Name: step.Name, Success: false}
						return
					}

					if cmd := action.GetCommandAction(); cmd != nil {
						actionStr = cmd.Command
					} else if file := action.GetFileAction(); file != nil {
						actionStr = fmt.Sprintf("Write to %s", file.Path)
					} else if ans := action.GetAnswerAction(); ans != nil {
						actionStr = ans.Answer
					}
				}

				debugLogger := log.With().
					Str("pipeline_name", pipelineName).
					Str("run_id", runID).
					Str("step_name", step.Name).
					Str("action_type", action.Type).
					Logger()

				debugLogger.Debug().Msgf("Executing action: %s", actionStr)

				stdout, stderr, exitCode := executeAction(cli, stepContainerID, action)

				status := "Succeeded"
				output := stdout
				if exitCode != 0 {
					status = "Failed"
					output = stderr + stdout
				}

				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					logMsg := fmt.Sprintf(`status=%s step="%s" action="%s" output="%s"`, status, step.Name, actionStr, output)
					log.Info().Str("pipeline", pipelineName).Msg(logMsg)
				}

				shareOutput := true
				if pipeline.LlmOutputSharing != nil {
					shareOutput = *pipeline.LlmOutputSharing
				}
				if step.LlmOutputSharing != nil {
					shareOutput = *step.LlmOutputSharing
				}

				historyGoal = step.Goal
				if historyGoal == "" {
					historyGoal = fmt.Sprintf("Execute script for step: %s", step.Name)
				}

				if !shareOutput {
					log.Debug().Msg("Output sharing is DISABLED for this step. Hiding output from history.")
					output = "[Output was hidden by pipeline configuration]"
				}

				historyMutex.Lock()
				history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", historyGoal, actionStr, exitCode, output))
				historyMutex.Unlock()

				if exitCode == 0 {
					updateStepStatus(runID, step.Name, "completed", exitCode)
					results <- StepResult{Name: step.Name, Success: true}
				} else {
					updateStepStatus(runID, step.Name, "failed", exitCode)
					if step.IgnoreFailure {
						log.Warn().Str("pipeline", pipelineName).Str("step", step.Name).Msg("Step failed, but failure is ignored.")
						results <- StepResult{Name: step.Name, Success: true}
					} else {
						log.Error().Str("pipeline", pipelineName).Str("step", step.Name).Msg("Critical step failed.")
						results <- StepResult{Name: step.Name, Success: false}
					}
				}
			}(step)
		}

		wg.Wait()
		close(results)

		anyFailed := false
		for result := range results {
			if result.Success {
				completedSteps[result.Name] = true
			} else {
				anyFailed = true
			}
		}

		if anyFailed {
			log.Error().Str("pipeline", pipelineName).Msg("One or more critical steps failed. Shutting down.")
			os.Exit(1)
		}
	}
}
