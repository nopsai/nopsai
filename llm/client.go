package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type PlannedStep struct {
	Name          string   `json:"name"`
	Prompt        string   `json:"prompt"`
	Dependencies  []string `json:"dependencies,omitempty"`
	IgnoreFailure bool     `json:"ignore_failure,omitempty"`
	Description   string   `json:"description,omitempty"`
}

type ExecutionPlan struct {
	OverallPlanSummary string        `json:"overall_plan_summary"`
	PlannedSteps       []PlannedStep `json:"planned_steps"`
}

type PromptContextForCode struct {
	PreciseStepPrompt string
}

type Schema struct {
	Type        string              `json:"type"`
	Properties  map[string]Property `json:"properties,omitempty"`
	Items       *Schema             `json:"items,omitempty"`
	Required    []string            `json:"required,omitempty"`
	Enum        []string            `json:"enum,omitempty"`
	Description string              `json:"description,omitempty"`
}

type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Items       *Schema  `json:"items,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Nullable    bool     `json:"nullable,omitempty"`
}

type Client struct {
	apiKey          string
	modelName       string
	maxOutputTokens int
	temperature     float64
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GenerationConfig struct {
	ResponseMIMEType string  `json:"responseMimeType,omitempty"`
	ResponseSchema   *Schema `json:"responseSchema,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
}

type GeminiRequest struct {
	Contents         []GeminiContent  `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig,omitempty"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []GeminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason,omitempty"`
	} `json:"candidates"`
}

const systemInstruction = `You are a DevOps automation tool called Nopsai. Your primary function is to convert a user's high-level pipeline definition into a deterministic, structured, and machine-readable execution plan.

Your Task:
Analyze the user's entire pipeline definition, including the 'outputs' keys for each step, and create a JSON object containing a list of "PlannedSteps".

State Management:
- All step outputs are persisted as environment variables in a shared file located at '$WORKSPACE/nopsai_outputs.env'.
- The executor will automatically 'source' this file before each step, making all previously defined variables available.
- The format for variables in the file is: 'export VAR-NAME="value"'.

Rules for Generating the Plan:
1.  name: This MUST EXACTLY match the name from the user's corresponding step.
2.  dependencies: List the name of any steps that MUST complete before this one.
3.  prompt: Create a VERY PRECISE, UNAMBIGUOUS prompt for another LLM to generate a shell script.
    - **For steps that produce outputs:** The prompt must instruct the LLM to generate a script that calculates the value for each output key defined in the user's pipeline. For each output, the script MUST append a line to the file at '$WORKSPACE/nopsai-outputs.env'. The line MUST be in the format 'echo 'export VAR-NAME="value"' >> $WORKSPACE/nopsai-outputs.env'.
    - The 'VAR-NAME' MUST follow this convention: '{STEP-NAME-SNAKE-CASE-UPPER}_{OUTPUT-KEY-UPPER}'. (e.g., for step 'clone repository' and output 'repository-name', the variable is 'CLONE-REPOSITORY_REPOSITORY-NAME').
    - **For steps that consume outputs:** The prompt must instruct the LLM to generate a script that directly uses the environment variables (e.g., 'echo $CLONE-REPOSITORY_REPOSITORY-NAME'). The script should assume these variables are already present in the environment.

Output ONLY the final JSON object. Do not include any other commentary.
The user's pipeline definition is:
`
const systemInstructionPrefix = `You are an elite-level shell script generator. Your sole purpose is to convert a precise prompt into a single, production-quality, non-interactive shell script.

Mandatory Rules:
1.Your output MUST be ONLY the raw shell script. Do NOT include explanations, comments, markdown fences (like  bash), or any text other than the code itself.
2.ALWAYS start multi-command scripts with  set -eo pipefail. This ensures the script exits immediately on any error or pipe failure. The only exception is if a command's failure is explicitly meant to be ignored.
3.If a specific command might fail but shouldn't stop the script, make it fault-tolerant within the script (e.g. command || true).
4.Do not use commands that prompt for input.
5.Where possible, write scripts that are idempotent. For example, use  mkdir -p instead of  mkdir.

Example: Simple Command
Precise Prompt: "Run the command: echo 'Hello World'"
Your Output:
 echo 'Hello World'

Now, generate the script for the following precise step prompt:
`

func NewClient(apiKey string, modelName string, maxTokens int, temperature float64) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini API key is not configured")
	}
	if modelName == "" {
		return nil, fmt.Errorf("gemini Model Name is not configured")
	}
	if maxTokens <= 0 {
		return nil, fmt.Errorf("maxOutputTokens must be positive, got %d", maxTokens)
	}
	return &Client{
		apiKey:          apiKey,
		modelName:       modelName,
		maxOutputTokens: maxTokens,
		temperature:     temperature,
	}, nil
}

func (c *Client) GenerateExecutionPlan(userPipelineDefinition string, verbose bool) (*ExecutionPlan, error) {
	fmt.Println("Analyzing user pipeline definition...")
	if verbose {
		fmt.Printf("  LLM Client: Requesting execution plan using model %s for pipeline:\n---\n%s\n---\n", c.modelName, userPipelineDefinition)
	}
	fullPromptForGemini := systemInstruction + userPipelineDefinition

	geminiPayload := GeminiRequest{
		Contents: []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: fullPromptForGemini}}}},
		GenerationConfig: GenerationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema: &Schema{
				Type: "OBJECT",
				Properties: map[string]Property{
					"overall_plan_summary": {Type: "STRING", Description: "A brief summary of the entire execution plan."},
					"planned_steps": {
						Type: "ARRAY",
						Items: &Schema{
							Type: "OBJECT",
							Properties: map[string]Property{
								"name":           {Type: "STRING", Description: "Name of the user's step this action corresponds to."},
								"prompt":         {Type: "STRING", Description: "Precise prompt for the code generation phase for this step."},
								"dependencies":   {Type: "ARRAY", Items: &Schema{Type: "STRING"}, Nullable: true, Description: "List of dependencies."},
								"ignore_failure": {Type: "BOOLEAN", Description: "If true, pipeline continues even if this step fails."},
								"description":    {Type: "STRING", Description: "Human-readable description of this planned step."},
							},
							Required: []string{"name", "prompt", "description"},
						},
					},
				},
				Required: []string{"planned_steps"},
			},
			MaxOutputTokens: c.maxOutputTokens,
			Temperature:     c.temperature,
		},
	}

	payloadBytes, err := json.Marshal(geminiPayload)
	if err != nil {
		return nil, fmt.Errorf("error marshalling plan generation payload: %w", err)
	}

	geminiAPIURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.modelName, c.apiKey)

	req, err := http.NewRequest("POST", geminiAPIURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("error creating plan generation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making plan generation request to Gemini API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading plan generation response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini plan generation API request failed status %s: %s", resp.Status, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("error unmarshalling plan generation GeminiResponse: %w. Body: %s", err, string(body))
	}

	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates found in plan generation Gemini response. Body: %s", string(body))
	}

	candidate := geminiResp.Candidates[0]
	if candidate.FinishReason == "MAX_TOKENS" {
		return nil, fmt.Errorf("llm plan generation response was truncated due to MAX_TOKENS limit. The plan was too long for the configured output limit")
	}
	if candidate.FinishReason != "STOP" && candidate.FinishReason != "" {
		return nil, fmt.Errorf("llm plan generation stopped for an unexpected reason: %s. Body: %s", candidate.FinishReason, string(body))
	}
	if len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content parts in plan generation Gemini response candidate. FinishReason: %s. Body: %s", candidate.FinishReason, string(body))
	}

	jsonOutputFromGemini := geminiResp.Candidates[0].Content.Parts[0].Text
	if verbose {
		fmt.Printf("  Raw JSON Plan: %s\n", jsonOutputFromGemini)
	}

	var executionPlan ExecutionPlan
	cleanedJsonOutput := strings.TrimSpace(jsonOutputFromGemini)
	if strings.HasPrefix(cleanedJsonOutput, "```json") {
		cleanedJsonOutput = strings.TrimPrefix(cleanedJsonOutput, "```json")
		cleanedJsonOutput = strings.TrimSuffix(cleanedJsonOutput, "```")
		cleanedJsonOutput = strings.TrimSpace(cleanedJsonOutput)
	}
	if err := json.Unmarshal([]byte(cleanedJsonOutput), &executionPlan); err != nil {
		return nil, fmt.Errorf("error unmarshalling ExecutionPlan from Gemini: %w. JSON: %s", err, cleanedJsonOutput)
	}
	return &executionPlan, nil
}

func (c *Client) GenerateCodeForStep(context PromptContextForCode, verbose bool) (string, error) {

	fullPromptForGemini := systemInstructionPrefix + context.PreciseStepPrompt

	geminiPayload := GeminiRequest{
		Contents: []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: fullPromptForGemini}}}},
		GenerationConfig: GenerationConfig{
			MaxOutputTokens: c.maxOutputTokens,
			Temperature:     c.temperature,
		},
	}
	payloadBytes, err := json.Marshal(geminiPayload)
	if err != nil {
		return "", fmt.Errorf("error marshalling code generation payload: %w", err)
	}
	if c.apiKey == "" {
		return "", fmt.Errorf("emini API key is not configured")
	}

	geminiAPIURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.modelName, c.apiKey)

	req, err := http.NewRequest("POST", geminiAPIURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("error creating code generation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making code generation request to Gemini API: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading code generation response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("emini code generation API request failed status %s: %s", resp.Status, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", fmt.Errorf("error unmarshalling code generation GeminiResponse: %w. Body: %s", err, string(body))
	}
	if len(geminiResp.Candidates) == 0 {
		return "", fmt.Errorf("no candidates in code generation Gemini response. Body: %s", string(body))
	}
	candidate := geminiResp.Candidates[0]
	if candidate.FinishReason == "MAX_TOKENS" {
		return "", fmt.Errorf("LLM code generation response truncated (MAX_TOKENS)")
	}
	if candidate.FinishReason != "STOP" && candidate.FinishReason != "" {
		return "", fmt.Errorf("LLM code generation stopped unexpectedly: %s", candidate.FinishReason)
	}
	if len(candidate.Content.Parts) == 0 {
		return "", fmt.Errorf("no content parts in code generation Gemini response. FinishReason: %s", candidate.FinishReason)
	}

	generatedCode := candidate.Content.Parts[0].Text
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
		fmt.Printf("raw code: %s\n", generatedCode)
	}
	return generatedCode, nil
}
