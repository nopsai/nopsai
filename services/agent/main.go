package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func executeAction(action *proto.Action) (stdout, stderr string, exitCode int) {
	switch action.Type {
	case "EXECUTE_COMMAND":
		cmdAction := action.GetCommandAction()
		if cmdAction == nil {
			return "", "Invalid command action payload", 1
		}

		log.Printf("Executing command: `%s`", cmdAction.Command)
		cmd := exec.Command("sh", "-c", cmdAction.Command)

		var outb, errb strings.Builder
		cmd.Stdout = &outb
		cmd.Stderr = &errb

		err := cmd.Run()
		stdout = strings.TrimSpace(outb.String())
		stderr = strings.TrimSpace(errb.String())

		if stdout != "" {
			log.Printf("Command STDOUT:\n---\n%s\n---", stdout)
		}
		if stderr != "" {
			log.Printf("Command STDERR:\n---\n%s\n---", stderr)
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

		log.Printf("Replacing file: `%s`", fileAction.Path)
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
		log.Printf("Received answer: %s", ansAction.Answer)
		return ansAction.Answer, "", 0

	default:
		return "", fmt.Sprintf("Unknown action type: %s", action.Type), 1
	}
}

func main() {
	runID := os.Getenv("RUN_ID")
	if runID == "" {
		log.Fatal("RUN_ID environment variable must be set.")
	}
	llmAgentAddress := os.Getenv("LLM_AGENT_ADDRESS")
	if llmAgentAddress == "" {
		llmAgentAddress = "localhost:50051"
	}

	log.Printf("Agent starting for Run ID: %s, connecting to %s", runID, llmAgentAddress)

	var conn *grpc.ClientConn
	var err error
	for i := 0; i < 5; i++ {
		conn, err = grpc.NewClient(llmAgentAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err == nil {
			break
		}
		log.Printf("Did not connect: %v. Retrying in 2 seconds...", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect after multiple retries: %v", err)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.ExecutionStream(ctx)
	if err != nil {
		log.Fatalf("Could not open stream: %v", err)
	}

	log.Println("Subscribing to execution stream...")
	subReq := &proto.StreamRequest{
		RunId: runID,
		Event: &proto.StreamRequest_Subscribe{
			Subscribe: &proto.SubscribeRequest{},
		},
	}
	if err := stream.Send(subReq); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	var wg sync.WaitGroup

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			log.Println("Server closed the stream. All steps are likely dispatched.")
			break
		}
		if err != nil {
			log.Fatalf("Failed to receive message: %v", err)
		}

		if step := resp.GetStep(); step != nil {
			wg.Add(1)
			go func(runnableStep *proto.RunnableStep) {
				defer wg.Done()
				stepID := runnableStep.GetStepId()
				action := runnableStep.GetAction()
				log.Printf("Starting execution for step %s", stepID)

				stdout, stderr, exitCode := executeAction(action)

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
				if err := stream.Send(resultReq); err != nil {
					log.Printf("Step %s: failed to send result: %v", stepID, err)
				}
				log.Printf("Finished execution for step %s", stepID)
			}(step)
		}
	}

	wg.Wait()
	log.Println("All dispatched steps have completed.")
}
