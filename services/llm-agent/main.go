package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

type llmAgentServer struct {
	proto.UnimplementedAgentServiceServer
	db  *pgxpool.Pool
	cfg *config.Config
}

// Gemini API specific structures
type GeminiRequest struct {
	Contents []Content `json:"contents"`
}
type Content struct {
	Parts []Part `json:"parts"`
}
type Part struct {
	Text string `json:"text"`
}
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []Part `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// callRealGemini makes a real API call to the Gemini API.
func (s *llmAgentServer) callRealGemini(prompt string) (*models.Action, error) {
	log.Println("Calling real Gemini API...")

	geminiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", s.cfg.GeminiModel, s.cfg.GeminiAPIKey)

	reqPayload := models.GeminiRequest{
		Contents: []models.Content{
			{Parts: []models.Part{{Text: prompt}}},
		},
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gemini request: %w", err)
	}

	resp, err := http.Post(geminiURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to call gemini api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	var geminiResp models.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gemini response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("invalid or empty response from gemini: %s", string(body))
	}

	actionJSON := geminiResp.Candidates[0].Content.Parts[0].Text
	actionJSON = strings.Trim(actionJSON, " \n\r\t`")
	actionJSON = strings.TrimPrefix(actionJSON, "json")

	var actionWrapper struct {
		Action models.Action `json:"action"`
	}
	if err := json.Unmarshal([]byte(actionJSON), &actionWrapper); err != nil {
		return nil, fmt.Errorf("failed to unmarshal action from gemini response: %w. Response text: %s", err, actionJSON)
	}

	return &actionWrapper.Action, nil
}

func (s *llmAgentServer) buildPrompt(ctx context.Context, runID string) (prompt string, stepID string, err error) {
	rows, err := s.db.Query(ctx, "SELECT goal, action_taken, execution_log, exit_code FROM steps WHERE run_id = $1 AND status = 'completed' ORDER BY step_index ASC", runID)
	if err != nil {
		return "", "", fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var historyBuilder strings.Builder
	historyBuilder.WriteString("**Execution History (Previous Steps):**\n")
	historyCount := 0
	for rows.Next() {
		// Use sql.NullString to gracefully handle NULL values from the database.
		var goal string
		var actionTaken, executionLog sql.NullString
		var exitCode sql.NullInt32

		if err := rows.Scan(&goal, &actionTaken, &executionLog, &exitCode); err != nil {
			return "", "", fmt.Errorf("failed to scan history row: %w", err)
		}
		historyBuilder.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", goal, actionTaken.String, exitCode.Int32, executionLog.String))
		historyCount++
	}
	if historyCount == 0 {
		historyBuilder.WriteString("No previous steps.\n")
	}

	var currentGoal, currentStepID string
	err = s.db.QueryRow(ctx, "SELECT step_id, goal FROM steps WHERE run_id = $1 AND status = 'pending' ORDER BY step_index ASC LIMIT 1", runID).Scan(&currentStepID, &currentGoal)
	if err != nil {
		// If no pending rows are found, it's not an error, it just means the pipeline is done.
		if err.Error() == "no rows in result set" {
			return "", "", nil // Return empty prompt and nil error
		}
		return "", "", fmt.Errorf("failed to get next pending step: %w", err)
	}

	promptTemplate := `You are an expert CI/CD automation bot. Your task is to achieve a user's goal by choosing the correct action from a toolkit.
You must only respond with a single JSON object. Inside this object, there should be a single key "action" which contains the action to perform.

Here are the available actions:
1. **EXECUTE_COMMAND**: {"action": {"type": "EXECUTE_COMMAND", "command_action": {"command": "your-bash-command-here"}}}
2. **REPLACE_FILE**: {"action": {"type": "REPLACE_FILE", "file_action": {"path": "./path/to/file.txt", "content": "The full new content of the file."}}}
3. **RETURN_ANSWER**: {"action": {"type": "RETURN_ANSWER", "answer_action": {"answer": "The answer to the user's question."}}}
---
**Previous Steps:**
%s
---
**Current Goal:**
"%s"
---
Now, choose the single best action from your toolkit and provide the response in the required JSON format.`

	fullPrompt := fmt.Sprintf(promptTemplate, historyBuilder.String(), currentGoal)
	return fullPrompt, currentStepID, nil
}

// ExecutionStream is the bidirectional streaming RPC handler.
func (s *llmAgentServer) ExecutionStream(stream proto.AgentService_ExecutionStreamServer) error {
	log.Println("Agent connected to ExecutionStream.")
	var currentStepID string
	var lastActionSentJSON string // Variable to hold the JSON of the last action sent

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			log.Println("Agent closed the stream.")
			return nil
		}
		if err != nil {
			log.Printf("Error receiving from agent: %v", err)
			return err
		}

		runID := req.GetRunId()

		switch event := req.Event.(type) {
		case *proto.ExecutionRequest_GetNextAction:
			log.Printf("Received request for next action from Run ID: %s", runID)

			prompt, stepID, err := s.buildPrompt(stream.Context(), runID)
			if err != nil {
				log.Printf("Error building prompt for run %s: %v", runID, err)
				continue
			}
			if stepID == "" {
				log.Printf("No more pending steps for run %s. Closing stream.", runID)
				return nil // End of pipeline
			}
			currentStepID = stepID

			actionModel, err := s.callRealGemini(prompt)
			if err != nil {
				log.Printf("Error calling Gemini: %v", err)
				continue
			}

			// Store the JSON of the action we are about to send.
			actionBytes, err := json.Marshal(actionModel)
			if err != nil {
				log.Printf("Error marshalling action model: %v", err)
				continue
			}
			lastActionSentJSON = string(actionBytes)

			protoAction := &proto.Action{Type: string(actionModel.Type)}
			switch actionModel.Type {
			case models.ActionTypeExecuteCommand:
				if actionModel.CommandAction == nil {
					log.Printf("Error: Gemini returned EXECUTE_COMMAND without a command_action payload.")
					continue
				}
				protoAction.Payload = &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: actionModel.CommandAction.Command}}
			case models.ActionTypeReplaceFile:
				if actionModel.FileAction == nil {
					log.Printf("Error: Gemini returned REPLACE_FILE without a file_action payload.")
					continue
				}
				protoAction.Payload = &proto.Action_FileAction{FileAction: &proto.FileAction{Path: actionModel.FileAction.Path, Content: actionModel.FileAction.Content}}
			case models.ActionTypeReturnAnswer:
				if actionModel.AnswerAction == nil {
					log.Printf("Error: Gemini returned RETURN_ANSWER without an answer_action payload.")
					continue
				}
				protoAction.Payload = &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{Answer: actionModel.AnswerAction.Answer}}
			}

			log.Printf("Sending action type '%s' to agent.", protoAction.Type)
			if err := stream.Send(protoAction); err != nil {
				log.Printf("Error sending action to agent: %v", err)
				return err
			}

		case *proto.ExecutionRequest_ReportActionResult:
			result := event.ReportActionResult
			log.Printf("Received action result for step %s", currentStepID)

			// Update the step in the database, now including the action_taken.
			_, err := s.db.Exec(stream.Context(),
				"UPDATE steps SET status = $1, execution_log = $2, exit_code = $3, action_taken = $4, finished_at = NOW() WHERE step_id = $5",
				"completed", result.Stdout+"\n"+result.Stderr, result.ExitCode, lastActionSentJSON, currentStepID,
			)
			if err != nil {
				log.Printf("Failed to update step %s: %v", currentStepID, err)
			}
			log.Printf("Successfully updated step %s in database.", currentStepID)
		}
	}
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config from %s: %v", configPath, err)
	}

	var dbpool *pgxpool.Pool
	for i := 0; i < 5; i++ {
		dbpool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			if err = dbpool.Ping(context.Background()); err == nil {
				log.Println("Successfully connected to the database.")
				break
			}
		}
		log.Printf("Unable to connect to database: %v. Retrying in 3 seconds...", err)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to connect to database after multiple retries: %v", err)
	}
	defer dbpool.Close()

	lis, err := net.Listen("tcp", cfg.LlmAgentListenAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	proto.RegisterAgentServiceServer(s, &llmAgentServer{db: dbpool, cfg: cfg})

	log.Printf("LLM Agent server listening at %s", cfg.LlmAgentListenAddress)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
