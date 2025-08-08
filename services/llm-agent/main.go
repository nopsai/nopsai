package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/pkg/proto"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
)

type llmAgentServer struct {
	proto.UnimplementedLLMServiceServer
	cfg *config.Config
}

// GetAction is the single RPC method that receives the entire context from an agent.
func (s *llmAgentServer) GetAction(ctx context.Context, req *proto.GetActionRequest) (*proto.Action, error) {
	prompt := s.buildPrompt(req)

	actionModel, err := s.callRealGemini(prompt)
	if err != nil {
		log.Error().Err(err).Msg("Error calling Gemini")
		return nil, err
	}

	// Translate the model's response into the protobuf Action message.
	protoAction := &proto.Action{Type: string(actionModel.Type)}
	switch actionModel.Type {
	case models.ActionTypeExecuteCommand:
		protoAction.Payload = &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: actionModel.CommandAction.Command}}
	case models.ActionTypeReplaceFile:
		protoAction.Payload = &proto.Action_FileAction{FileAction: &proto.FileAction{Path: actionModel.FileAction.Path, Content: actionModel.FileAction.Content}}
	case models.ActionTypeReturnAnswer:
		protoAction.Payload = &proto.Action_AnswerAction{AnswerAction: &proto.AnswerAction{Answer: actionModel.AnswerAction.Answer}}
	}

	return protoAction, nil
}

// buildPrompt assembles the full context into a single string for the LLM.
func (s *llmAgentServer) buildPrompt(req *proto.GetActionRequest) string {
	var envBuilder strings.Builder
	envBuilder.WriteString("**Environment Variables:**\n")
	for key, value := range req.GetEnvironment() {
		envBuilder.WriteString(fmt.Sprintf("- %s: %s\n", key, value))
	}

	var directoryListingBuilder strings.Builder
	directoryListingBuilder.WriteString("**Working Directory Contents:**\n")
	if len(req.GetDirectoryListing()) == 0 {
		directoryListingBuilder.WriteString("Directory is empty.\n")
	} else {
		for name, content := range req.GetDirectoryListing() {
			directoryListingBuilder.WriteString(fmt.Sprintf("--- File: %s ---\n%s\n", name, content))
		}
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
**Execution History (Previous Steps):**
%s
---
**Current Goal:**
"%s"
---
Now, choose the single best action from your toolkit and provide the response in the required JSON format.`

	// Use "No history yet." if the history is empty for clarity.
	history := req.GetHistory()
	if history == "" {
		history = "No history yet."
	}

	fullPrompt := fmt.Sprintf(promptTemplate, envBuilder.String(), directoryListingBuilder.String(), history, req.GetGoal())
	log.Debug().Msgf("Full prompt:\n%s", fullPrompt)
	return fullPrompt
}

// callRealGemini handles the actual HTTP request to the Google Gemini API.
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

	lis, err := net.Listen("tcp", cfg.LlmAgentListenAddress)
	if err != nil {
		log.Fatal().Err(err).Msgf("Failed to listen on %s", cfg.LlmAgentListenAddress)
	}

	s := grpc.NewServer()
	proto.RegisterLLMServiceServer(s, &llmAgentServer{cfg: cfg})

	log.Info().Msgf("LLM Agent server listening at %s", cfg.LlmAgentListenAddress)
	if err := s.Serve(lis); err != nil {
		log.Fatal().Err(err).Msg("Failed to serve gRPC")
	}
}
