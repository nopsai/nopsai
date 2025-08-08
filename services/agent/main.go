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
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

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
	_, err = io.Copy(&stdout, resp.Reader)
	if err != nil {
		return "", fmt.Sprintf("failed to read output: %v", err), 1
	}

	inspect, err := cli.ContainerExecInspect(context.Background(), execID.ID)
	if err != nil {
		return stdout.String(), fmt.Sprintf("failed to inspect exec: %v", err), 1
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), inspect.ExitCode
}

// getNextRunnableStep finds the next step to execute.
func getNextRunnableStep(pipeline *models.Pipeline, completedSteps map[string]bool) *models.PipelineStep {
	for _, step := range pipeline.Steps {
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
			return &step
		}
	}
	return nil
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

// cleanup stops and removes the pipeline container.
func cleanup(cli *client.Client, containerID string) {
	log.Info().Msgf("Cleaning up pipeline container: %s", containerID)
	// Use a new context for cleanup in case the main context is cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cli.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true}); err != nil {
		log.Error().Err(err).Msg("Failed to remove pipeline container")
	}
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

	// --- gRPC and Docker Client Setup ---
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

	// --- Resource Provisioning ---
	log.Info().Msgf("Pulling pipeline image: %s", pipeline.ContainerImage)
	out, err := cli.ImagePull(context.Background(), pipeline.ContainerImage, image.PullOptions{})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to pull pipeline container image")
	}
	io.ReadAll(out)
	out.Close()

	pipelineContainerName := fmt.Sprintf("pipeline-%s", runID)
	cont, err := cli.ContainerCreate(context.Background(), &container.Config{
		Image:      pipeline.ContainerImage,
		WorkingDir: "/workspace",
		Tty:        true,
	}, &container.HostConfig{
		Binds: []string{fmt.Sprintf("%s:/workspace", sharedVolumeName)},
	}, nil, nil, pipelineContainerName)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create pipeline container")
	}

	if err := cli.ContainerStart(context.Background(), cont.ID, container.StartOptions{}); err != nil {
		log.Fatal().Err(err).Msg("Failed to start pipeline container")
	}
	log.Info().Msgf("Successfully started pipeline container: %s", pipelineContainerName)

	// --- Main Execution Loop ---
	history := new(strings.Builder)
	completedSteps := make(map[string]bool)

	for {
		nextStep := getNextRunnableStep(&pipeline, completedSteps)
		if nextStep == nil {
			log.Info().Str("pipeline", pipeline.Name).Msg("All steps completed successfully.")
			cleanup(cli, cont.ID)
			os.Exit(0)
		}

		directoryListing := getDirectoryListing(log.Logger, "/workspace")
		req := &proto.GetActionRequest{
			Goal:             nextStep.Goal,
			History:          history.String(),
			DirectoryListing: directoryListing,
			Environment:      pipeline.Environment,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		action, err := llmClient.GetAction(ctx, req)
		cancel()
		if err != nil {
			log.Error().Err(err).Str("step", nextStep.Name).Msg("Failed to get action from LLM agent. Shutting down.")
			cleanup(cli, cont.ID)
			os.Exit(1)
		}

		var actionStr string
		if cmd := action.GetCommandAction(); cmd != nil {
			actionStr = cmd.Command
		} else if file := action.GetFileAction(); file != nil {
			actionStr = fmt.Sprintf("Write to %s", file.Path)
		} else if ans := action.GetAnswerAction(); ans != nil {
			actionStr = ans.Answer
		}

		debugLogger := log.With().
			Str("pipeline_name", pipelineName).
			Str("run_id", runID).
			Str("step_name", nextStep.Name).
			Str("action_type", action.Type).
			Logger()

		debugLogger.Debug().Msgf("Executing action: %s", actionStr)

		stdout, stderr, exitCode := executeAction(cli, cont.ID, action)

		status := "Succeeded"
		output := stdout
		if exitCode != 0 {
			status = "Failed"
			output = stderr
		}

		if zerolog.GlobalLevel() <= zerolog.InfoLevel {
			logMsg := fmt.Sprintf(`status=%s step="%s" action="%s" output="%s"`, status, nextStep.Name, actionStr, output)
			log.Info().Str("pipeline", pipelineName).Msg(logMsg)
		}

		history.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", nextStep.Goal, actionStr, exitCode, output))

		if exitCode == 0 {
			completedSteps[nextStep.Name] = true
			updateStepStatus(runID, nextStep.Name, "completed", exitCode)
		} else {
			updateStepStatus(runID, nextStep.Name, "failed", exitCode)
			if !nextStep.IgnoreFailure {
				log.Error().Str("pipeline", pipelineName).Str("step", nextStep.Name).Msg("Critical step failed. Shutting down.")
				cleanup(cli, cont.ID)
				os.Exit(1)
			} else {
				log.Warn().Str("pipeline", pipelineName).Str("step", nextStep.Name).Msg("Step failed, but failure is ignored.")
				completedSteps[nextStep.Name] = true
			}
		}
	}
}
