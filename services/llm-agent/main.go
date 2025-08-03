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
	"sync"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

type llmAgentServer struct {
	proto.UnimplementedAgentServiceServer
	db  *pgxpool.Pool
	cfg *config.Config
	mu  sync.Mutex
}

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

func (s *llmAgentServer) buildPrompt(ctx context.Context, stepID string) (string, error) {
	historyQuery := `
		SELECT s.goal, s.action_taken, s.execution_log, s.exit_code
		FROM steps s
		JOIN step_dependencies sd ON s.step_id = sd.depends_on_step_id
		WHERE sd.step_id = $1 AND s.status = 'completed'`

	rows, err := s.db.Query(ctx, historyQuery, stepID)
	if err != nil {
		return "", fmt.Errorf("failed to query history for step %s: %w", stepID, err)
	}
	defer rows.Close()

	var historyBuilder strings.Builder
	historyBuilder.WriteString("**Execution History (Previous Steps):**\n")
	historyCount := 0
	for rows.Next() {
		var goal string
		var actionTaken, executionLog sql.NullString
		var exitCode sql.NullInt32

		if err := rows.Scan(&goal, &actionTaken, &executionLog, &exitCode); err != nil {
			return "", fmt.Errorf("failed to scan history row: %w", err)
		}
		historyBuilder.WriteString(fmt.Sprintf("- Goal: %s\n  Action: %s\n  Result (Exit Code %d): %s\n", goal, actionTaken.String, exitCode.Int32, executionLog.String))
		historyCount++
	}
	if historyCount == 0 {
		historyBuilder.WriteString("No previous steps.\n")
	}

	var currentGoal string
	err = s.db.QueryRow(ctx, "SELECT goal FROM steps WHERE step_id = $1", stepID).Scan(&currentGoal)
	if err != nil {
		return "", fmt.Errorf("failed to get goal for current step %s: %w", stepID, err)
	}

	promptTemplate := `You are an expert CI/CD automation bot. Your task is to achieve a user's goal by choosing the correct action from a toolkit.
You must only respond with a single JSON object. Inside this object, there should be a single key "action" which contains the action to perform.

Here are the available actions:
1. **EXECUTE_COMMAND**: {"action": {"type": "EXECUTE_COMMAND", "command_action": {"command": "your-bash-command-here"}}}
2. **REPLACE_FILE**: {"action": {"type": "REPLACE_FILE", "file_action": {"path": "./path/to/file.txt", "content": "The full new content of the file."}}}
3. **RETURN_ANSWER**: {"action": {"type": "RETURN_ANSWER", "answer_action": {"answer": "The answer to the user's question."}}}
---
%s
---
**Current Goal:**
"%s"
---
Now, choose the single best action from your toolkit and provide the response in the required JSON format.`

	fullPrompt := fmt.Sprintf(promptTemplate, historyBuilder.String(), currentGoal)
	return fullPrompt, nil
}

func (s *llmAgentServer) ExecutionStream(stream proto.AgentService_ExecutionStreamServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	subReq := req.GetSubscribe()
	if subReq == nil {
		return fmt.Errorf("expected first message to be a SubscribeRequest")
	}
	runID := req.GetRunId()
	log.Printf("Agent for RunID %s subscribed.", runID)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			req, err := stream.Recv()
			if err != nil {
				return
			}
			if result := req.GetResult(); result != nil {
				s.handleStepResult(result)
			}
		}
	}()

	for {
		select {
		case <-stream.Context().Done():
			log.Printf("Agent for RunID %s disconnected.", runID)
			return stream.Context().Err()
		case <-ticker.C:
			steps, err := s.getAndPrepareRunnableSteps(stream.Context(), runID)
			if err != nil {
				log.Printf("Error getting runnable steps for %s: %v", runID, err)
				continue
			}
			for _, step := range steps {
				log.Printf("Sending step %s to agent for run %s", step.GetStepId(), runID)
				resp := &proto.StreamResponse{
					Event: &proto.StreamResponse_Step{Step: step},
				}
				if err := stream.Send(resp); err != nil {
					log.Printf("Error sending step to agent for run %s: %v", runID, err)
					return err
				}
			}
		}
	}
}

func (s *llmAgentServer) handleStepResult(result *proto.StepResult) {
	log.Printf("Received action result for step %s", result.StepId)
	_, err := s.db.Exec(context.Background(),
		"UPDATE steps SET status = $1, execution_log = $2, exit_code = $3, action_taken = $4, finished_at = NOW() WHERE step_id = $5",
		"completed", result.Stdout+"\n"+result.Stderr, result.ExitCode, result.ActionTaken, result.StepId,
	)
	if err != nil {
		log.Printf("Failed to update step %s: %v", result.StepId, err)
	}
	log.Printf("Successfully updated step %s in database.", result.StepId)
}

func (s *llmAgentServer) getAndPrepareRunnableSteps(ctx context.Context, runID string) ([]*proto.RunnableStep, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		SELECT s.step_id FROM steps s
		WHERE s.run_id = $1 AND s.status = 'pending'
		AND NOT EXISTS (
			SELECT 1 FROM step_dependencies sd
			JOIN steps dep_s ON sd.depends_on_step_id = dep_s.step_id
			WHERE sd.step_id = s.step_id AND dep_s.status IN ('pending', 'running')
		)`

	rows, err := s.db.Query(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stepIDs []uuid.UUID
	for rows.Next() {
		var stepID uuid.UUID
		if err := rows.Scan(&stepID); err != nil {
			return nil, err
		}
		stepIDs = append(stepIDs, stepID)
	}

	var runnableSteps []*proto.RunnableStep
	if len(stepIDs) > 0 {
		for _, id := range stepIDs {
			_, err := s.db.Exec(ctx, "UPDATE steps SET status = 'running', started_at = NOW() WHERE step_id = $1 AND status = 'pending'", id)
			if err != nil {
				log.Printf("Failed to mark step %s as running: %v", id, err)
				continue
			}

			prompt, err := s.buildPrompt(ctx, id.String())
			if err != nil {
				log.Printf("Error building prompt for step %s: %v", id, err)
				continue
			}

			actionModel, err := s.callRealGemini(prompt)
			if err != nil {
				log.Printf("Error calling Gemini for step %s: %v", id, err)
				continue
			}

			protoAction := &proto.Action{Type: string(actionModel.Type)}
			switch actionModel.Type {
			case models.ActionTypeExecuteCommand:
				protoAction.Payload = &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: actionModel.CommandAction.Command}}
			case models.ActionTypeReplaceFile:
				protoAction.Payload = &proto.Action_FileAction{FileAction: &proto.FileAction{Path: actionModel.FileAction.Path, Content: actionModel.FileAction.Content}}
			case models.ActionTypeReturnAnswer:
				protoAction.Payload = &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{Answer: actionModel.AnswerAction.Answer}}
			}

			runnableSteps = append(runnableSteps, &proto.RunnableStep{
				StepId: id.String(),
				Action: protoAction,
			})
		}
	}

	return runnableSteps, nil
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
