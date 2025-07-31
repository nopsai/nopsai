package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"nopsai/pkg/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// executeAction takes an action from the server and performs it.
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
		stdout = outb.String()
		stderr = errb.String()

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
		log.Fatal("RUN_ID environment variable not set.")
	}
	llmAgentAddress := os.Getenv("LLM_AGENT_ADDRESS")
	if llmAgentAddress == "" {
		llmAgentAddress = "localhost:50051" // Default for local testing
	}

	log.Printf("Agent starting for Run ID: %s, connecting to %s", runID, llmAgentAddress)

	// Retry connecting to the gRPC server to handle startup delays
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	defer cancel()

	stream, err := client.ExecutionStream(ctx)
	if err != nil {
		log.Fatalf("Could not open stream: %v", err)
	}

	go func() {
		for {
			action, err := stream.Recv()
			if err == io.EOF {
				log.Println("Stream closed by server. Exiting.")
				os.Exit(0)
				return
			}
			if err != nil {
				log.Fatalf("Failed to receive action: %v", err)
			}

			log.Println("-------------------------------------------")
			log.Printf("Received action type '%s' from server.", action.Type)
			stdout, stderr, exitCode := executeAction(action)

			result := &proto.ReportActionResult{
				Stdout:   stdout,
				Stderr:   stderr,
				ExitCode: int32(exitCode),
			}
			req := &proto.ExecutionRequest{
				RunId: runID,
				Event: &proto.ExecutionRequest_ReportActionResult{ReportActionResult: result},
			}

			log.Printf("Reporting result (Exit Code: %d)...", exitCode)
			if err := stream.Send(req); err != nil {
				log.Fatalf("Failed to send result: %v", err)
			}
			log.Println("-------------------------------------------")

			log.Println("Requesting next action...")
			nextActionReq := &proto.ExecutionRequest{
				RunId: runID,
				Event: &proto.ExecutionRequest_GetNextAction{},
			}
			if err := stream.Send(nextActionReq); err != nil {
				log.Fatalf("Failed to request next action: %v", err)
			}
		}
	}()

	log.Println("Requesting first action.")
	initialRequest := &proto.ExecutionRequest{
		RunId: runID,
		Event: &proto.ExecutionRequest_GetNextAction{},
	}
	if err := stream.Send(initialRequest); err != nil {
		log.Fatalf("Failed to send initial request: %v", err)
	}

	<-stream.Context().Done()
	log.Printf("Execution finished: %v", stream.Context().Err())
}
