package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

func getDirectoryListing(logger zerolog.Logger, root string) map[string]string {
	directoryListing := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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

func executeAction(action *proto.Action, logger zerolog.Logger) (stdout, stderr string, exitCode int, directoryListing map[string]string) {
	// The core execution logic remains the same...
	switch action.Type {
	case "EXECUTE_COMMAND":
		cmdAction := action.GetCommandAction()
		if cmdAction == nil {
			return "", "Invalid command action payload", 1, nil
		}

		logger.Debug().Msgf("Executing command: `%s`", cmdAction.Command)
		cmd := exec.Command("sh", "-c", cmdAction.Command)

		var outb, errb strings.Builder
		cmd.Stdout = &outb
		cmd.Stderr = &errb

		err := cmd.Run()
		stdout = strings.TrimSpace(outb.String())
		stderr = strings.TrimSpace(errb.String())

		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		} else {
			exitCode = 0
		}

	case "REPLACE_FILE":
		fileAction := action.GetFileAction()
		if fileAction == nil {
			return "", "Invalid file action payload", 1, nil
		}

		logger.Debug().Msgf("Replacing file: `%s`", fileAction.Path)
		err := os.WriteFile(fileAction.Path, []byte(fileAction.Content), 0644)
		if err != nil {
			return "", fmt.Sprintf("Failed to write file: %v", err), 1, nil
		}
		stdout = fmt.Sprintf("Successfully wrote to %s", fileAction.Path)
		exitCode = 0

	case "RETURN_ANSWER":
		ansAction := action.GetAnswerAction()
		if ansAction == nil {
			return "", "Invalid answer action payload", 1, nil
		}
		logger.Debug().Msgf("Received answer: %s", ansAction.Answer)
		stdout = ansAction.Answer
		exitCode = 0

	default:
		stderr = fmt.Sprintf("Unknown action type: %s", action.Type)
		exitCode = 1
	}

	directoryListing = getDirectoryListing(logger, ".")
	return
}

func main() {
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
		// Use a custom console writer to control the output format
		cw := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen, NoColor: true}
		cw.FormatMessage = func(i interface{}) string {
			return "" // We format the message manually in the event
		}
		cw.FormatFieldName = func(i interface{}) string {
			return fmt.Sprintf("%s=", i)
		}
		cw.FormatFieldValue = func(i interface{}) string {
			return fmt.Sprintf("%s", i)
		}
		log.Logger = log.Output(cw)
	} else {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}
	zerolog.SetGlobalLevel(logLevel)

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

				action := runnableStep.GetAction()
				var actionStr string
				if cmd := action.GetCommandAction(); cmd != nil {
					actionStr = cmd.Command
				} else if file := action.GetFileAction(); file != nil {
					actionStr = fmt.Sprintf("Write to %s", file.Path)
				} else if ans := action.GetAnswerAction(); ans != nil {
					actionStr = ans.Answer
				}

				debugLogger := log.With().
					Str("pipeline_name", runnableStep.GetPipelineName()).
					Str("run_id", runID).
					Str("step_name", runnableStep.GetStepName()).
					Str("step_id", runnableStep.GetStepId()).
					Str("action_type", action.Type).
					Logger()

				stdout, stderr, exitCode, directoryListing := executeAction(action, debugLogger)

				status := "Succeeded"
				output := stdout
				if exitCode != 0 {
					status = "Failed"
					output = stderr
				}

				if zerolog.GlobalLevel() <= zerolog.InfoLevel {
					log.Info().
						Str("pipeline", runnableStep.GetPipelineName()).
						Str("step", runnableStep.GetStepName()).
						Str("status", status).
						Str("action", actionStr).
						Str("output", output).
						Msg("")
				}

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
					StepId:           runnableStep.GetStepId(),
					Stdout:           stdout,
					Stderr:           stderr,
					ExitCode:         int32(exitCode),
					ActionTaken:      string(actionTakenBytes),
					DirectoryListing: directoryListing,
				}
				resultReq := &proto.StreamRequest{
					RunId: runID,
					Event: &proto.StreamRequest_Result{Result: result},
				}

				if err := stream.Send(resultReq); err != nil {
					debugLogger.Error().Err(err).Msg("Failed to send result")
				}
			}(step)
		}
	}

	wg.Wait()
	log.Info().Msg("All dispatched steps have completed.")
}
