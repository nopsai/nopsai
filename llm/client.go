package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultGeminiModel string = "gemini-1.5-flash"

// --- Structs for Phase 1: Execution Planning ---

type PlannedAction struct {
	OriginalUserStepName string   `json:"original_user_step_name"`
	ActionName           string   `json:"action_name"`
	ActionPrompt         string   `json:"action_prompt"`
	Dependencies         []string `json:"dependencies,omitempty"`
	IgnoreFailure        bool     `json:"ignore_failure,omitempty"`
	Description          string   `json:"description,omitempty"`
}

type ExecutionPlan struct {
	OverallPlanSummary string          `json:"overall_plan_summary"`
	PlannedActions     []PlannedAction `json:"planned_actions"`
}

// --- Struct for Phase 2: Code Generation for a specific action ---

type PromptContextForCode struct {
	PreciseActionPrompt string
}

// Client remains the same
type Client struct {
	apiKey    string
	modelName string
}

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

// Gemini API related structures
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
	ResponseMIMEType string  `json:"responseMimeType,omitempty"`
	ResponseSchema   *Schema `json:"responseSchema,omitempty"`
	MaxOutputTokens  int     `json:"maxOutputTokens,omitempty"`
	Temperature      float64 `json:"temperature,omitempty"`
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
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []GeminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason,omitempty"`
	} `json:"candidates"`
}

// GenerateExecutionPlan (Phase 1 LLM call)
func (c *Client) GenerateExecutionPlan(userPipelineDefinition string, verbose bool) (*ExecutionPlan, error) {
	if verbose {
		fmt.Printf("  LLM Client: Requesting execution plan using model %s for pipeline:\n---\n%s\n---\n", c.modelName, userPipelineDefinition)
	}

	systemInstruction := `You are an AI pipeline planning assistant for a DevOps automation tool called Nopsai.
Your task is to analyze a user's entire pipeline definition (list of steps, their prompts, dependencies, and ignore_failure flags) and create a structured execution plan.
The plan should consist of a list of "PlannedActions". Each PlannedAction represents a concrete, executable unit.
You may need to break down a single complex user step into multiple PlannedActions. For example, if a user step is "clone repo X, checkout branch Y, and then calculate version Z", this should become at least two or three distinct PlannedActions: one for cloning, one for checkout, and one for version calculation, with appropriate dependencies between them.
For each PlannedAction, you must define:
1.  'original_user_step_name': The 'name' of the user's step this action helps fulfill.
2.  'action_name': A unique, descriptive snake_case name for this specific planned action (e.g., "clone_repo_x", "checkout_branch_y", "calculate_version_z").
3.  'action_prompt': A VERY PRECISE and self-contained natural language prompt that will be given to another LLM instance in a subsequent phase to generate ONLY the shell script/command for THIS specific action. This prompt should include all necessary details for that action. If this action needs to operate within a specific directory context established by a previous action (e.g., a cloned repository), include that instruction in this action_prompt (e.g., "In directory 'repo_x', checkout branch 'dev'"). If this action depends on the output of another *planned action*, use the placeholder format '{outputs.previous_action_name}' in this 'action_prompt' to refer to the standard output of that previous action. For example: "Build a docker image using the version string '{outputs.calculate_version_z}' and name it 'myimage:{outputs.calculate_version_z}'."
4.  'dependencies': A list of 'action_name's of other PlannedActions that this action depends on. Ensure these dependencies are logical.
5.  'ignore_failure': A boolean, typically carried over from the original user step's 'ignore_failure' flag. If a user step is broken into multiple planned actions, all those planned actions should inherit the 'ignore_failure' status of the original user step.
6.  'description': A brief human-readable description of what this planned action does.

IMPORTANT: Ensure that any action requiring a specific working directory (like git commands after a clone, or docker build commands) has its 'action_prompt' clearly state that the operation should occur in that directory, or include commands like 'cd <directory_name>' as the first part of the action_prompt if appropriate for the code generation phase.

Output your response as a single JSON object matching the ExecutionPlan schema.
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
					"planned_actions": {
						Type: "ARRAY",
						Items: &Schema{
							Type: "OBJECT",
							Properties: map[string]Property{
								"original_user_step_name": {Type: "STRING", Description: "Name of the user's step this action corresponds to."},
								"action_name":             {Type: "STRING", Description: "Unique snake_case name for this planned action."},
								"action_prompt":           {Type: "STRING", Description: "Precise prompt for the code generation phase for this action. Should include 'cd <dir>' if necessary."},
								"dependencies":            {Type: "ARRAY", Items: &Schema{Type: "STRING"}, Nullable: true, Description: "List of action_names this action depends on."},
								"ignore_failure":          {Type: "BOOLEAN", Description: "If true, pipeline continues even if this action fails."},
								"description":             {Type: "STRING", Description: "Human-readable description of this planned action."},
							},
							Required: []string{"original_user_step_name", "action_name", "action_prompt", "description"},
						},
					},
				},
				Required: []string{"planned_actions"},
			},
			MaxOutputTokens: 8192,
			Temperature:     0.3,
		},
	}

	payloadBytes, err := json.Marshal(geminiPayload)
	if err != nil {
		return nil, fmt.Errorf("error marshalling plan generation payload: %w", err)
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("gemini API key is not configured")
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

// GenerateCodeForAction (Phase 2 LLM call)
func (c *Client) GenerateCodeForAction(context PromptContextForCode, verbose bool) (string, error) {
	if verbose {
		fmt.Printf("  LLM Client: Generating code using model %s for planned action prompt:\n---\n%s\n---\n", c.modelName, context.PreciseActionPrompt)
	}

	systemInstructionPrefix := `You are an AI assistant for a DevOps automation tool. Your task is to generate a shell script or command to perform the requested task.
The user will provide a very precise prompt for a specific, well-defined action, which may include context from previous actions (like outputs of other actions referenced as {outputs.action_name}).
Based on this input, provide ONLY the shell script or command to execute.
ALWAYS start any multi-command bash script with 'set -e' to ensure it exits immediately if a command fails UNLESS the prompt explicitly indicates a command might fail and its failure should be ignored or handled.
If a command like 'git fetch --tags' or 'git describe' is used to gather information and might fail (e.g., no tags exist), make that specific command fault-tolerant within the script, for example by using 'git fetch --tags || true' or by checking its exit code if 'set -e' is active for the rest of the script. The script should still proceed with its logic if such an informational command "fails" gracefully.
If the action prompt includes changing directories (e.g., "In directory 'repo_x', do Y"), ensure your script includes the 'cd repo_x || exit 1' command.
Do NOT include any explanations, markdown formatting,  or any text other than the code itself.
No Comments.
Example: If the precise action prompt is "Run the command: echo 'Hello World'", your output should be exactly: echo 'Hello World'
Example: If the precise action prompt is "Create a file named data.txt with the content 'test data'", your output should be exactly: set -e\necho 'test data' > data.txt
Example: If the precise action prompt is "In directory 'my_project', run tests", your output should be: set -e\ncd my_project || exit 1\nmake test
`
	fullPromptForGemini := systemInstructionPrefix + "\nPrecise action to perform (ensure you handle any placeholders like {outputs.action_name} by incorporating them into your script logic if the prompt implies their use):\n" + context.PreciseActionPrompt

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
