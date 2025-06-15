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
	apiKey    string
	modelName string
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

func NewClient(apiKey string, modelName string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("gemini API key is not configured")
	}
	if modelName == "" {
		return nil, fmt.Errorf("gemini Model Name is not configured")
	}
	return &Client{
		apiKey:    apiKey,
		modelName: modelName,
	}, nil
}

func (c *Client) GenerateExecutionPlan(userPipelineDefinition string, verbose bool) (*ExecutionPlan, error) {
	fmt.Println("Analyzing user pipeline definition...")
	if verbose {
		fmt.Printf("  LLM Client: Requesting execution plan using model %s for pipeline:\n---\n%s\n---\n", c.modelName, userPipelineDefinition)
	}

	systemInstruction := `You are an AI pipeline planning assistant for a DevOps automation tool called Nopsai.
Your task is to analyze a user's entire pipeline definition (list of steps, their prompts, dependencies, and ignore_failure flags) and create a structured execution plan.
The plan should consist of a list of "PlannedSteps". Each PlannedStep represents a concrete, executable unit.
For each PlannedStep, you must define:
1.  'name': The 'name' of the user's step this action helps fulfill.
3.  'prompt': A VERY PRECISE and self-contained natural language prompt that will be given to another LLM instance in a subsequent phase to generate ONLY the shell script/command for The specific action. This prompt should include all necessary details for that action. If this action needs to operate within a specific directory context established by a previous action, include that instruction in this prompt"
4.  'dependencies': A list of 'name's of other PlannedSteps that this action depends on. Ensure these dependencies are logical.
5.  'ignore_failure': A boolean, typically carried over from the original user step's 'ignore_failure' flag. If a user step is broken into multiple planned actions, all those planned actions should inherit the 'ignore_failure' status of the original user step.
6.  'description': A brief human-readable description of what this planned action does.
Outputs of steps which are required by other steps should be stored in result.txt file as key=value. (e.g. SOME_KEY=some-value), and other steps should be aware of that. print the outputs of each step. Use native linux tools to edit file.
IMPORTANT: Ensure that any action requiring a specific working directory has its 'prompt' clearly state that the operation should occur in that directory, or include commands like 'cd <directory_name>' as the first part of the prompt if appropriate for the code generation phase.
Output your response as a single JSON.
The user's pipeline definition is:
`
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
			MaxOutputTokens: 8192,
			Temperature:     0.3,
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
		fmt.Printf("  Raw JSON Plan from Gemini: %s\n", jsonOutputFromGemini)
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
	if verbose {
		fmt.Printf("  LLM Client: Generating code using model %s for planned step prompt:\n---\n%s\n---\n", c.modelName, context.PreciseStepPrompt)
	}

	systemInstructionPrefix := `Your task is to generate a shell script or command to perform the requested task.
The user will provide a very precise prompt for a specific, well-defined action, which may include context from previous steps.
Based on this input, provide ONLY the shell script or command to execute.
ALWAYS start any multi-command bash script with 'set -e' to ensure it exits immediately if a command fails UNLESS the prompt explicitly indicates a command might fail and its failure should be ignored or handled.
make specific command fault-tolerant within the script, for example by using 'git fetch --tags || true' or by checking its exit code if 'set -e' is active for the rest of the script. The script should still proceed with its logic if such an informational command "fails" gracefully.
If the step prompt includes changing directories, ensure your script includes the command. Ensure the command is valid.
Do NOT include any explanations, commented lines, markdown formatting,  or any text other than the code itself.
Example: If the precise step prompt is "Run the command: echo 'Hello World'", your output should be exactly: echo 'Hello World'
Example: If the precise step prompt is "Create a file named data.txt with the content 'test data'", your output should be exactly: set -e\necho 'test data' > data.txt
`
	fullPromptForGemini := systemInstructionPrefix + context.PreciseStepPrompt

	geminiPayload := GeminiRequest{
		Contents: []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: fullPromptForGemini}}}},
		GenerationConfig: GenerationConfig{
			MaxOutputTokens: 8192,
			Temperature:     0.3,
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
		fmt.Printf("  Raw Code from Gemini for Action: %s\n", generatedCode)
	}
	return generatedCode, nil
}
