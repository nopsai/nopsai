// llm/client.go
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	// "time"
)

const defaultGeminiModel string = "gemini-1.5-flash"

// PromptContext provides context to the LLM when analyzing a step.
type PromptContext struct {
	CurrentStepPrompt string // This will contain the step's prompt and context from dependencies
}

// Client represents a client for interacting with the LLM.
type Client struct {
	apiKey    string
	modelName string
}

// NewClient creates a new LLM client.
func NewClient(apiKey string, modelName string) *Client {
	effectiveModelName := modelName
	if effectiveModelName == "" {
		effectiveModelName = defaultGeminiModel
	}
	return &Client{
		apiKey:    apiKey,
		modelName: effectiveModelName,
	}
}

// Gemini API related structures (simplified for text-only request/response)
type GeminiRequest struct {
	Contents         []GeminiContent  `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []GeminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
	} `json:"candidates"`
}

// GenerateCodeForStep sends a prompt to the LLM (Gemini) and expects only the executable code string back.
func (c *Client) GenerateCodeForStep(context PromptContext, verbose bool) (string, error) {
	if verbose {
		fmt.Printf("  LLM Client: Generating code using model %s for prompt:\n---\n%s\n---\n", c.modelName, context.CurrentStepPrompt)
	}

	systemInstructionPrefix := `You are an AI assistant for a DevOps automation tool. Your task is to generate a shell script or command to perform the requested task.
The user will provide a prompt that includes the task for the current step and may include outputs from previous dependent steps for context.
Based on this entire input, provide ONLY the shell script or command to execute.
Do NOT include any explanations, markdown formatting or any text other than the code itself.
If the task is to "echo 'Hello World'", your output should be exactly: echo 'Hello World'
If the task is "create a file named test.txt with content 'hello'", your output should be exactly: echo 'hello' > test.txt
`
	fullPromptForGemini := systemInstructionPrefix + "\nUser's task and context:\n" + context.CurrentStepPrompt

	geminiPayload := GeminiRequest{
		Contents: []GeminiContent{
			{Role: "user", Parts: []GeminiPart{{Text: fullPromptForGemini}}},
		},
		GenerationConfig: GenerationConfig{
			MaxOutputTokens: 1024,
			Temperature:     0.2,
		},
	}

	payloadBytes, err := json.Marshal(geminiPayload)
	if err != nil {
		return "", fmt.Errorf("error marshalling Gemini payload: %w", err)
	}

	if c.apiKey == "" {
		return "", fmt.Errorf("ini API key is")
	}
	geminiAPIURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.modelName, c.apiKey)

	req, err := http.NewRequest("POST", geminiAPIURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("error creating Gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to Gemini API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading Gemini response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("quest failed with status")
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("error unmarshalling Gemini response into GeminiResponse struct: %w. Body: %s", err, string(body))
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content found in Gemini response. Body: %s", string(body))
	}

	generatedCode := geminiResp.Candidates[0].Content.Parts[0].Text
	generatedCode = strings.TrimSpace(generatedCode)
	if strings.HasPrefix(generatedCode, "```bash") {
		generatedCode = strings.TrimPrefix(generatedCode, "```bash")
		generatedCode = strings.TrimSuffix(generatedCode, "```")
		generatedCode = strings.TrimSpace(generatedCode)
	} else if strings.HasPrefix(generatedCode, "```") {
		generatedCode = strings.TrimPrefix(generatedCode, "```")
		generatedCode = strings.TrimSuffix(generatedCode, "```")
		generatedCode = strings.TrimSpace(generatedCode)
	}

	if verbose {
		fmt.Printf("  Raw Code from Gemini: %s\n", generatedCode)
	}
	return generatedCode, nil
}
