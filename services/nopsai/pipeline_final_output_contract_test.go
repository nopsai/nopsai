package nopsai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nopsai/pkg/llmclient"
	"nopsai/pkg/models"
)

type scriptedPipelineFinalOutputCompleter struct {
	completions []llmclient.Completion
	errors      []error
	systems     []string
	prompts     []string
}

func (c *scriptedPipelineFinalOutputCompleter) CompleteWithSystem(
	_ context.Context,
	systemInstruction string,
	prompt string,
) (llmclient.Completion, error) {
	index := len(c.prompts)
	c.systems = append(c.systems, systemInstruction)
	c.prompts = append(c.prompts, prompt)
	if index < len(c.errors) && c.errors[index] != nil {
		return llmclient.Completion{}, c.errors[index]
	}
	if index >= len(c.completions) {
		return llmclient.Completion{}, errors.New("unexpected completion call")
	}
	return c.completions[index], nil
}

func TestExtractPipelineFinalOutputElementDropsPreambleAndTrailingText(t *testing.T) {
	content, err := extractPipelineFinalOutputElement(
		"The user wants me to create a report.\n<final_output>\n# Report\n\nEverything passed.\n</final_output>\nDone.",
	)
	if err != nil {
		t.Fatalf("extractPipelineFinalOutputElement() error = %v", err)
	}
	if content != "# Report\n\nEverything passed." {
		t.Fatalf("content = %q", content)
	}
}

func TestExtractPipelineFinalOutputElementRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		reason string
	}{
		{name: "missing element", raw: "The user wants me to create a report.", reason: "missing_element"},
		{name: "multiple elements", raw: "<final_output>one</final_output><final_output>two</final_output>", reason: "multiple_elements"},
		{name: "closing first", raw: "</final_output><final_output>report", reason: "malformed_element"},
		{name: "empty element", raw: "<final_output>  </final_output>", reason: "empty_content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractPipelineFinalOutputElement(tt.raw)
			if err == nil || !strings.Contains(err.Error(), tt.reason) {
				t.Fatalf("error = %v, want reason %q", err, tt.reason)
			}
		})
	}
}

func TestNormalizePipelineFinalOutputContentRejectsInvalidDocumentSpec(t *testing.T) {
	_, err := normalizePipelineFinalOutputContent(
		"html",
		"<final_output><script>secret()</script><iframe src=\"bad\"></iframe></final_output>",
	)
	if err == nil || !strings.Contains(err.Error(), "invalid_document_spec") {
		t.Fatalf("error = %v, want invalid_document_spec", err)
	}
}

func TestGenerateValidatedPipelineFinalOutputRetriesMissingElement(t *testing.T) {
	client := &scriptedPipelineFinalOutputCompleter{completions: []llmclient.Completion{
		{Text: "The user wants me to create a report.\n\n# Report"},
		{Text: "<final_output>\n# Report\n\nEverything passed.\n</final_output>"},
	}}
	result, err := generateValidatedPipelineFinalOutput(t.Context(), client, "markdown", "Build the report.")
	if err != nil {
		t.Fatalf("generateValidatedPipelineFinalOutput() error = %v", err)
	}
	if result.Content != "# Report\n\nEverything passed." ||
		len(result.Attempts) != 2 ||
		result.ContractViolations != 1 ||
		result.Attempts[0].ContractValid ||
		!result.Attempts[1].ContractValid {
		t.Fatalf("result = %#v", result)
	}
	if len(client.systems) != 2 ||
		client.systems[0] != pipelineFinalOutputSystemInstruction ||
		client.systems[1] != pipelineFinalOutputSystemInstruction {
		t.Fatalf("systems = %#v", client.systems)
	}
	if !strings.Contains(client.prompts[1], "missing_element") ||
		!strings.Contains(client.prompts[1], "Correction required") {
		t.Fatalf("retry prompt = %q", client.prompts[1])
	}
}

func TestGenerateValidatedPipelineFinalOutputRetriesInvalidJSON(t *testing.T) {
	client := &scriptedPipelineFinalOutputCompleter{completions: []llmclient.Completion{
		{Text: "<final_output>{not-json</final_output>"},
		{Text: `<final_output>{"ok":true}</final_output>`},
	}}
	result, err := generateValidatedPipelineFinalOutput(t.Context(), client, "json", "Build JSON.")
	if err != nil {
		t.Fatalf("generateValidatedPipelineFinalOutput() error = %v", err)
	}
	if result.Content != `{"ok":true}` || result.ContractViolations != 1 {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(client.prompts[1], "invalid_json") {
		t.Fatalf("retry prompt = %q", client.prompts[1])
	}
}

func TestGenerateValidatedPipelineFinalOutputRetriesSeriesModeWithoutChart(t *testing.T) {
	client := &scriptedPipelineFinalOutputCompleter{completions: []llmclient.Completion{
		{Text: `<final_output>{"version":"1","title":"Build timeline","blocks":[{"type":"list","title":"Timeline","items":["nopsai-dashboard built first"]}]}</final_output>`},
		{Text: `<final_output>{"version":"1","title":"Build timeline","blocks":[{"type":"series","title":"Build duration timeline","chart":{"type":"line","series":[{"key":"duration","label":"Duration","points":[{"label":"nopsai-dashboard","value":24}]}]}}]}</final_output>`},
	}}
	validator := pipelineFinalOutputRecordContentValidator(pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{Type: "dashboard", Name: "Build timeline"},
		Dashboard:              models.DashboardOutputTarget{Mode: "series", Preset: "timeline"},
	})
	result, err := generateValidatedPipelineFinalOutput(t.Context(), client, "dashboard", "Build timeline.", validator)
	if err != nil {
		t.Fatalf("generateValidatedPipelineFinalOutput() error = %v", err)
	}
	if result.ContractViolations != 1 || len(result.Attempts) != 2 || result.Attempts[0].ContractValid || !result.Attempts[1].ContractValid {
		t.Fatalf("result = %#v", result)
	}
	if !strings.Contains(client.prompts[1], "series publication requires at least one chart or series block") ||
		!strings.Contains(client.prompts[1], "include at least one chart or series block") {
		t.Fatalf("retry prompt = %q", client.prompts[1])
	}
}

func TestBuildPipelineFinalOutputRetryPromptAddsDashboardSchemaHint(t *testing.T) {
	prompt := buildPipelineFinalOutputRetryPrompt(
		"Build dashboard.",
		newPipelineFinalOutputContractError(
			"invalid_dashboard_spec",
			"invalid DashboardSpec: json: cannot unmarshal string into Go struct field DashboardBlock.blocks.items of type models.DashboardBlockItem",
		),
	)
	for _, want := range []string{
		"invalid_dashboard_spec",
		"use one flat top-level blocks array",
		"do not put nested blocks or widgets inside a block",
		"If the dashboard publish mode is series",
		"Use the validation error to correct incompatible field shapes",
		"Keep the dashboard focused on the user's requested intent",
		"evidence present in the run context",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("retry prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `{"text":"Disk usage high"}`) {
		t.Fatalf("retry prompt should not include static dashboard item examples:\n%s", prompt)
	}
}

func TestGenerateValidatedPipelineFinalOutputFailsAfterOneRetry(t *testing.T) {
	client := &scriptedPipelineFinalOutputCompleter{completions: []llmclient.Completion{
		{Text: "analysis first"},
		{Text: "analysis again"},
	}}
	result, err := generateValidatedPipelineFinalOutput(t.Context(), client, "pdf", "Build PDF source.")
	if err == nil || !strings.Contains(err.Error(), "after 2 attempts") {
		t.Fatalf("error = %v", err)
	}
	if len(result.Attempts) != 2 || result.ContractViolations != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestGenerateValidatedPipelineFinalOutputDoesNotRetryProviderFailure(t *testing.T) {
	client := &scriptedPipelineFinalOutputCompleter{
		completions: []llmclient.Completion{{Text: "<final_output>unused</final_output>"}},
		errors:      []error{errors.New("provider unavailable")},
	}
	result, err := generateValidatedPipelineFinalOutput(t.Context(), client, "markdown", "Build report.")
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("error = %v", err)
	}
	if len(client.prompts) != 1 || len(result.Attempts) != 1 || result.ContractViolations != 0 {
		t.Fatalf("calls = %d result = %#v", len(client.prompts), result)
	}
}
