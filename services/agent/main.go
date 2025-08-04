package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// executeAction now accepts a logger to prefix messages.
func executeAction(action *proto.Action, logger zerolog.Logger) (stdout, stderr string, exitCode int) {
	switch action.Type {
	case "EXECUTE_COMMAND":
		cmdAction := action.GetCommandAction()
		if cmdAction == nil {
			return "", "Invalid command action payload", 1
		}

		logger.Info().Msgf("Executing command: `%s`", cmdAction.Command)
		cmd := exec.Command("sh", "-c", cmdAction.Command)

		var outb, errb strings.Builder
		cmd.Stdout = &outb
		cmd.Stderr = &errb

		err := cmd.Run()
		stdout = strings.TrimSpace(outb.String())
		stderr = strings.TrimSpace(errb.String())

		if stdout != "" {
			logger.Debug().Msgf("Command STDOUT:\n---\n%s\n---", stdout)
		}
		if stderr != "" {
			logger.Debug().Msgf("Command STDERR:\n---\n%s\n---", stderr)
		}

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 0
		}
		return stdout, stderr, exitCode

	case "REPLACE_FILE":
		fileAction := action.GetFileAction()
		if fileAction == nil {
			return "", "Invalid file action payload", 1
		}

		logger.Info().Msgf("Replacing file: `%s`", fileAction.Path)
		err := os.WriteFile(fileAction.Path, []byte(fileAction.Content), 0644)
		if err != nil {
			return "", fmt.Sprintf("Failed to write file: %v", err), 1
		}
		return fmt.Sprintf("Successfully wrote to %s", fileAction.Path), "", 0

	case "RETURN_ANSWER":
		ansAction := action.GetAnswerAction()
		if ansAction == nil {
			return "", "Invalid answer action payload", 1
		}
		logger.Info().Msgf("Received answer: %s", ansAction.Answer)
		return ansAction.Answer, "", 0

	default:
		return "", fmt.Sprintf("Unknown action type: %s", action.Type), 1
	}
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	// Use ConsoleWriter for more human-readable logs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	zerolog.SetGlobalLevel(zerolog.DebugLevel) // Agent logs are verbose by default

	runID := os.Getenv("RUN_ID")
	if runID == "" {
		log.Fatal().Msg("RUN_ID environment variable not set.")
	}
	llmAgentAddress := os.Getenv("LLM_AGENT_ADDRESS")
	if llmAgentAddress == "" {
		llmAgentAddress = "localhost:50051"
	}

	log.Info().Str("run_id", runID).Msgf("Agent starting, connecting to %s", llmAgentAddress)

	var conn *grpc.ClientConn
	var err error
	for i := 0; i < 5; i++ {
		conn, err = grpc.NewClient(llmAgentAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
		log.Warn().Err(err).Msgf("Did not connect. Retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect after multiple retries")
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.ExecutionStream(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Could not open stream")
	}

	log.Info().Msg("Subscribing to execution stream...")
	subReq := &proto.StreamRequest{
		RunId: runID,
		Event: &proto.StreamRequest_Subscribe{
			Subscribe: &proto.SubscribeRequest{},
		},
	}
	if err := stream.Send(subReq); err != nil {
		log.Fatal().Err(err).Msg("Failed to subscribe")
	}

	var wg sync.WaitGroup

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			log.Info().Msg("Server closed the stream. All steps are likely dispatched.")
			break
		}
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to receive message")
		}

		if step := resp.GetStep(); step != nil {
			wg.Add(1)
			go func(runnableStep *proto.RunnableStep) {
				defer wg.Done()
				stepID := runnableStep.GetStepId()
				action := runnableStep.GetAction()

				// Create a sub-logger with context for this specific step
				stepLogger := log.With().Str("step_id", stepID[:8]).Logger()
				stepLogger.Info().Msgf("Starting execution. Action Type: %s", action.Type)

				stdout, stderr, exitCode := executeAction(action, stepLogger)

				modelsAction := &models.Action{Type: action.Type}
				switch action.Type {
				case models.ActionTypeExecuteCommand:
					modelsAction.CommandAction = &models.CommandAction{Command: action.GetCommandAction().GetCommand()}
				case models.ActionTypeReplaceFile:
					modelsAction.FileAction = &models.FileAction{Path: action.GetFileAction().GetPath(), Content: action.GetFileAction().GetContent()}
				case models.ActionTypeReturnAnswer:
					modelsAction.AnswerAction = &models.AnswerAction{Answer: action.GetAnswerAction().GetAnswer()}
				}
				actionTakenBytes, _ := json.Marshal(modelsAction)

				result := &proto.StepResult{
					StepId:      stepID,
					Stdout:      stdout,
					Stderr:      stderr,
					ExitCode:    int32(exitCode),
					ActionTaken: string(actionTakenBytes),
				}
				resultReq := &proto.StreamRequest{
					RunId: runID,
					Event: &proto.StreamRequest_Result{Result: result},
				}

				stepLogger.Info().Int("exit_code", exitCode).Msg("Reporting result...")
				if err := stream.Send(resultReq); err != nil {
					stepLogger.Error().Err(err).Msg("Failed to send result")
				}
				stepLogger.Info().Msg("Finished execution.")
			}(step)
		}
	}

	wg.Wait()
	log.Info().Msg("All dispatched steps have completed.")
}
