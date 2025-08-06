package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type llmAgentServer struct {
	proto.UnimplementedAgentServiceServer
	db  *pgxpool.Pool
	cfg *config.Config
	mu  sync.Mutex
}

func (s *llmAgentServer) callRealGemini(prompt string) (*models.Action, error) {
	log.Debug().Msg("Calling real Gemini API...")

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
	var runID string
	var envJSON sql.NullString
	err := s.db.QueryRow(ctx, "SELECT run_id, environment FROM runs WHERE run_id = (SELECT run_id FROM steps WHERE step_id = $1)", stepID).Scan(&runID, &envJSON)
	if err != nil {
		return "", fmt.Errorf("failed to get run info for step %s: %w", stepID, err)
	}

	var envBuilder strings.Builder
	envBuilder.WriteString("**Environment Variables:**\n")
	if envJSON.Valid && envJSON.String != "" {
		var env map[string]string
		if err := json.Unmarshal([]byte(envJSON.String), &env); err == nil {
			for key, value := range env {
				envBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
			}
		}
	}

	var directoryListingJSON sql.NullString
	directoryListingQuery := `
		SELECT s.directory_listing
		FROM steps s
		JOIN step_dependencies sd ON s.step_id = sd.depends_on_step_id
		WHERE sd.step_id = $1 AND s.status = 'completed'
		ORDER BY s.finished_at DESC NULLS LAST
		LIMIT 1`
	err = s.db.QueryRow(ctx, directoryListingQuery, stepID).Scan(&directoryListingJSON)
	if err != nil && err != pgx.ErrNoRows {
		return "", fmt.Errorf("failed to query directory listing for step %s: %w", stepID, err)
	}

	var directoryListingBuilder strings.Builder
	directoryListingBuilder.WriteString("**Working Directory Contents:**\n")
	if directoryListingJSON.Valid && directoryListingJSON.String != "" {
		var directoryListing map[string]string
		if err := json.Unmarshal([]byte(directoryListingJSON.String), &directoryListing); err == nil {
			if len(directoryListing) == 0 {
				directoryListingBuilder.WriteString("Directory is empty.\n")
			} else {
				for name, content := range directoryListing {
					directoryListingBuilder.WriteString(fmt.Sprintf("--- File: %s ---\n%s\n", name, content))
				}
			}
		} else {
			directoryListingBuilder.WriteString("Could not parse directory listing.\n")
		}
	} else {
		directoryListingBuilder.WriteString("No files in directory yet.\n")
	}

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
%s
---
%s
---
**Current Goal:**
"%s"
---
Now, choose the single best action from your toolkit and provide the response in the required JSON format.`

	fullPrompt := fmt.Sprintf(promptTemplate, envBuilder.String(), directoryListingBuilder.String(), historyBuilder.String(), currentGoal)
	log.Debug().Msgf("Full prompt:\n%s", fullPrompt)
	return fullPrompt, nil
}
func (s *llmAgentServer) ExecutionStream(stream proto.AgentService_ExecutionStreamServer) error {
	req, err := stream.Recv()
	if err != nil {
		log.Error().Err(err).Msg("Failed to receive initial message from agent")
		return err
	}
	subReq := req.GetSubscribe()
	if subReq == nil {
		return fmt.Errorf("expected first message to be a SubscribeRequest")
	}
	runID := req.GetRunId()
	log.Info().Str("run_id", runID).Msg("Agent subscribed to execution stream")

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Goroutine to handle receiving results from the agent
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

	// Main loop: periodically check for and send runnable steps
	for {
		select {
		case <-stream.Context().Done():
			log.Info().Str("run_id", runID).Msg("Agent disconnected")
			return stream.Context().Err()
		case <-ticker.C:
			steps, err := s.getAndPrepareRunnableSteps(stream.Context(), runID)
			if err != nil {
				log.Error().Err(err).Str("run_id", runID).Msg("Error getting runnable steps")
				continue
			}
			for _, step := range steps {
				l := log.With().
					Str("pipeline_name", step.GetPipelineName()).
					Str("run_id", runID).
					Str("step_name", step.GetStepName()).
					Str("step_id", step.GetStepId()).
					Logger()
				l.Info().Msg("Sending step to agent")
				resp := &proto.StreamResponse{
					Event: &proto.StreamResponse_Step{Step: step},
				}
				if err := stream.Send(resp); err != nil {
					l.Error().Err(err).Msg("Error sending step to agent")
					return err
				}
			}
		}
	}
}

func (s *llmAgentServer) handleStepResult(result *proto.StepResult) {
	var stepName, pipelineName, runID string
	err := s.db.QueryRow(context.Background(), `
		SELECT s.name, r.pipeline_name, s.run_id
		FROM steps s JOIN runs r ON s.run_id = r.run_id
		WHERE s.step_id = $1`, result.StepId).Scan(&stepName, &pipelineName, &runID)
	if err != nil {
		log.Error().Err(err).Str("step_id", result.StepId).Msg("Failed to query step context for logging")
		return
	}

	l := log.With().
		Str("pipeline_name", pipelineName).
		Str("run_id", runID).
		Str("step_name", stepName).
		Str("step_id", result.StepId).
		Logger()

	l.Info().Msg("Received action result")

	directoryListingBytes, err := json.Marshal(result.DirectoryListing)
	if err != nil {
		l.Error().Err(err).Msg("Failed to marshal directory listing")
		directoryListingBytes = []byte("{}")
	}

	_, err = s.db.Exec(context.Background(),
		"UPDATE steps SET status = $1, execution_log = $2, exit_code = $3, action_taken = $4, finished_at = NOW(), directory_listing = $5 WHERE step_id = $6",
		"completed", result.Stdout+"\n"+result.Stderr, result.ExitCode, result.ActionTaken, string(directoryListingBytes), result.StepId,
	)
	if err != nil {
		l.Error().Err(err).Msg("Failed to update step")
	}
	l.Info().Msg("Successfully updated step in database")
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
			var stepName, pipelineName string
			var runUUID uuid.UUID
			err := s.db.QueryRow(ctx, `
                SELECT s.name, r.pipeline_name, s.run_id
                FROM steps s JOIN runs r ON s.run_id = r.run_id
                WHERE s.step_id = $1`, id).Scan(&stepName, &pipelineName, &runUUID)
			if err != nil {
				log.Error().Err(err).Str("step_id", id.String()).Msg("Failed to query step context")
				continue
			}

			l := log.With().
				Str("pipeline_name", pipelineName).
				Str("run_id", runUUID.String()).
				Str("step_name", stepName).
				Str("step_id", id.String()).
				Logger()

			_, err = s.db.Exec(ctx, "UPDATE steps SET status = 'running', started_at = NOW() WHERE step_id = $1 AND status = 'pending'", id)
			if err != nil {
				l.Error().Err(err).Msg("Failed to mark step as running")
				continue
			}

			prompt, err := s.buildPrompt(ctx, id.String())
			if err != nil {
				l.Error().Err(err).Msg("Error building prompt")
				continue
			}

			actionModel, err := s.callRealGemini(prompt)
			if err != nil {
				l.Error().Err(err).Msg("Error calling Gemini")
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
				StepId:       id.String(),
				Action:       protoAction,
				PipelineName: pipelineName,
				StepName:     stepName,
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
		log.Fatal().Err(err).Msgf("Failed to load config from %s", configPath)
	}

	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Warn().Msgf("Invalid log level '%s', defaulting to 'info'", cfg.LogLevel)
		logLevel = zerolog.InfoLevel
	}
	if cfg.LogFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.Kitchen})
	}
	zerolog.SetGlobalLevel(logLevel)

	var dbpool *pgxpool.Pool
	for i := 0; i < 5; i++ {
		dbpool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err == nil {
			if err = dbpool.Ping(context.Background()); err == nil {
				log.Info().Msg("Successfully connected to the database.")
				break
			}
		}
		log.Warn().Err(err).Msgf("Unable to connect to database. Retrying in 3 seconds...")
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database after multiple retries")
	}
	defer dbpool.Close()

	lis, err := net.Listen("tcp", cfg.LlmAgentListenAddress)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to listen on %s", cfg.LlmAgentListenAddress)
	}

	s := grpc.NewServer()
	proto.RegisterAgentServiceServer(s, &llmAgentServer{db: dbpool, cfg: cfg})

	log.Info().Msgf("LLM Agent server listening at %s", cfg.LlmAgentListenAddress)
	if err := s.Serve(lis); err != nil {
		log.Fatal().Err(err).Msg("Failed to serve gRPC")
	}
}
