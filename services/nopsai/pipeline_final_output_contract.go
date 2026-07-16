package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"nopsai/pkg/llmclient"
)

const (
	pipelineFinalOutputMaxAttempts = 2
	finalOutputOpenTag             = "<final_output>"
	finalOutputCloseTag            = "</final_output>"
)

const pipelineFinalOutputSystemInstruction = `You produce final deliverables for enterprise pipeline runs.
Return exactly one <final_output> element.
Do not describe the request, your reasoning, the run context, or formatting choices.
Do not write any text before or after the element.
The element content must follow the requested output format and must not expose secrets, credentials, tokens, or raw environment values.`

type pipelineFinalOutputCompleter interface {
	CompleteWithSystem(ctx context.Context, systemInstruction, prompt string) (llmclient.Completion, error)
}

type pipelineFinalOutputGenerationAttempt struct {
	Completion    llmclient.Completion
	ContractValid bool
}

type pipelineFinalOutputGenerationResult struct {
	Content            string
	Attempts           []pipelineFinalOutputGenerationAttempt
	ContractViolations int
}

type pipelineFinalOutputContractError struct {
	Reason  string
	Message string
}

func (e *pipelineFinalOutputContractError) Error() string {
	if e == nil {
		return "LLM final output contract violation"
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("LLM final output contract violation: %s", e.Reason)
	}
	return fmt.Sprintf("LLM final output contract violation (%s): %s", e.Reason, e.Message)
}

var (
	htmlScriptBlockPattern = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	htmlFrameBlockPattern  = regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`)
	htmlObjectBlockPattern = regexp.MustCompile(`(?is)<object\b[^>]*>.*?</object>`)
	htmlEmbedTagPattern    = regexp.MustCompile(`(?is)</?(script|iframe|object|embed|link|meta)\b[^>]*>`)
	htmlEventAttrPattern   = regexp.MustCompile(`(?is)\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	htmlStyleAttrPattern   = regexp.MustCompile(`(?is)\s+style\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	htmlJavascriptPattern  = regexp.MustCompile(`(?is)javascript\s*:`)
)

func generateValidatedPipelineFinalOutput(
	ctx context.Context,
	client pipelineFinalOutputCompleter,
	outputType string,
	prompt string,
) (pipelineFinalOutputGenerationResult, error) {
	var result pipelineFinalOutputGenerationResult
	var contractErr error
	for attempt := 1; attempt <= pipelineFinalOutputMaxAttempts; attempt++ {
		attemptPrompt := prompt
		if contractErr != nil {
			attemptPrompt = buildPipelineFinalOutputRetryPrompt(prompt, contractErr)
		}
		completion, err := client.CompleteWithSystem(ctx, pipelineFinalOutputSystemInstruction, attemptPrompt)
		if err != nil {
			result.Attempts = append(result.Attempts, pipelineFinalOutputGenerationAttempt{})
			return result, err
		}
		content, err := normalizePipelineFinalOutputContent(outputType, completion.Text)
		result.Attempts = append(result.Attempts, pipelineFinalOutputGenerationAttempt{
			Completion:    completion,
			ContractValid: err == nil,
		})
		if err == nil {
			result.Content = content
			return result, nil
		}
		result.ContractViolations++
		contractErr = err
	}
	return result, fmt.Errorf(
		"LLM final output failed validation after %d attempts: %w",
		pipelineFinalOutputMaxAttempts,
		contractErr,
	)
}

func buildPipelineFinalOutputRetryPrompt(prompt string, contractErr error) string {
	return strings.TrimSpace(prompt) + "\n\n" +
		"Correction required: the previous response was rejected. " +
		strings.TrimSpace(contractErr.Error()) + "\n" +
		"Return a corrected response that follows the system output contract exactly."
}

func normalizePipelineFinalOutputContent(outputType, raw string) (string, error) {
	content, err := extractPipelineFinalOutputElement(raw)
	if err != nil {
		return "", err
	}
	switch normalizePipelineFinalOutputType(outputType) {
	case "json":
		content = stripMarkdownFence(content)
		if !json.Valid([]byte(content)) {
			return "", newPipelineFinalOutputContractError("invalid_json", "element content is not valid JSON")
		}
	case "pdf", "html":
		content = stripMarkdownFence(content)
		spec, parseErr := parseDocumentSpec(content)
		if parseErr != nil {
			return "", newPipelineFinalOutputContractError("invalid_document_spec", parseErr.Error())
		}
		content, parseErr = marshalFinalOutputSpec(spec)
		if parseErr != nil {
			return "", newPipelineFinalOutputContractError("invalid_document_spec", parseErr.Error())
		}
	case "excel":
		content = stripMarkdownFence(content)
		spec, parseErr := parseSpreadsheetSpec(content)
		if parseErr != nil {
			return "", newPipelineFinalOutputContractError("invalid_spreadsheet_spec", parseErr.Error())
		}
		content, parseErr = marshalFinalOutputSpec(spec)
		if parseErr != nil {
			return "", newPipelineFinalOutputContractError("invalid_spreadsheet_spec", parseErr.Error())
		}
	case "dashboard":
		content = stripMarkdownFence(content)
		spec, parseErr := parseDashboardSpec(content)
		if parseErr != nil {
			return "", newPipelineFinalOutputContractError("invalid_dashboard_spec", parseErr.Error())
		}
		content, parseErr = marshalFinalOutputSpec(spec)
		if parseErr != nil {
			return "", newPipelineFinalOutputContractError("invalid_dashboard_spec", parseErr.Error())
		}
	}
	return content, nil
}

func extractPipelineFinalOutputElement(raw string) (string, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "\ufeff")
	openCount := strings.Count(raw, finalOutputOpenTag)
	closeCount := strings.Count(raw, finalOutputCloseTag)
	if openCount == 0 || closeCount == 0 {
		return "", newPipelineFinalOutputContractError(
			"missing_element",
			"response must contain one <final_output>...</final_output> element",
		)
	}
	if openCount != 1 || closeCount != 1 {
		return "", newPipelineFinalOutputContractError(
			"multiple_elements",
			"response must contain exactly one <final_output> element",
		)
	}
	start := strings.Index(raw, finalOutputOpenTag) + len(finalOutputOpenTag)
	end := strings.Index(raw, finalOutputCloseTag)
	if end < start {
		return "", newPipelineFinalOutputContractError(
			"malformed_element",
			"closing </final_output> tag appears before the opening tag",
		)
	}
	content := strings.TrimSpace(raw[start:end])
	if content == "" {
		return "", newPipelineFinalOutputContractError("empty_content", "element content is empty")
	}
	return content, nil
}

func newPipelineFinalOutputContractError(reason, message string) error {
	return &pipelineFinalOutputContractError{Reason: reason, Message: message}
}

func sanitizePipelineFinalOutputHTML(content string) string {
	content = htmlScriptBlockPattern.ReplaceAllString(content, "")
	content = htmlFrameBlockPattern.ReplaceAllString(content, "")
	content = htmlObjectBlockPattern.ReplaceAllString(content, "")
	content = htmlEmbedTagPattern.ReplaceAllString(content, "")
	content = htmlEventAttrPattern.ReplaceAllString(content, "")
	content = htmlStyleAttrPattern.ReplaceAllString(content, "")
	content = htmlJavascriptPattern.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

func stripMarkdownFence(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) >= 2 && strings.HasPrefix(strings.TrimSpace(lines[0]), "```") && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	}
	return content
}
