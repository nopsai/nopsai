package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"nopsai/pkg/models"
)

func cleanModelTextResponse(raw string) string {
	cleaned := strings.TrimSpace(raw)

	if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
		if len(cleaned) >= 4 && strings.EqualFold(cleaned[:4], "json") {
			cleaned = strings.TrimSpace(cleaned[4:])
		}
		if idx := strings.LastIndex(cleaned, "```"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
	}

	cleaned = strings.TrimSpace(cleaned)
	if len(cleaned) >= 4 && strings.EqualFold(cleaned[:4], "json") {
		cleaned = strings.TrimSpace(cleaned[4:])
	}

	return strings.Trim(cleaned, "` \n\r\t")
}

func decodeActionResponse(raw string) (*models.Action, error) {
	actionJSON := cleanModelTextResponse(raw)
	action, err := decodeActionJSON(actionJSON)
	if err == nil {
		return action, nil
	}
	strictErr := err

	for _, candidate := range extractActionJSONCandidates(raw) {
		action, err := decodeActionJSON(cleanModelTextResponse(candidate))
		if err == nil {
			return action, nil
		}
	}

	return nil, fmt.Errorf(
		"failed to unmarshal action response: %w. response_sha256=%s response_bytes=%d",
		strictErr,
		promptSHA256(actionJSON),
		len([]byte(actionJSON)),
	)
}

func decodeActionJSON(actionJSON string) (*models.Action, error) {
	var actionWrapper struct {
		Action models.Action `json:"action"`
	}
	if err := json.Unmarshal([]byte(actionJSON), &actionWrapper); err != nil {
		return nil, err
	}
	if err := validateAction(actionWrapper.Action); err != nil {
		return nil, err
	}

	return &actionWrapper.Action, nil
}

func validateAction(action models.Action) error {
	switch action.Type {
	case models.ActionTypeExecuteCommand:
		if action.CommandAction == nil || strings.TrimSpace(action.CommandAction.Command) == "" {
			return fmt.Errorf("EXECUTE_COMMAND action requires command_action.command")
		}
	case models.ActionTypeReplaceFile:
		if action.FileAction == nil || strings.TrimSpace(action.FileAction.Path) == "" {
			return fmt.Errorf("REPLACE_FILE action requires file_action.path")
		}
	case models.ActionTypeReturnAnswer:
		if action.AnswerAction == nil {
			return fmt.Errorf("RETURN_ANSWER action requires answer_action")
		}
	case models.ActionTypeCallMCPTool:
		if action.MCPToolAction == nil || strings.TrimSpace(action.MCPToolAction.Tool) == "" {
			return fmt.Errorf("CALL_MCP_TOOL action requires mcp_tool_action.tool")
		}
	case models.ActionTypeCallWorkspaceTool:
		if action.WorkspaceToolAction == nil || strings.TrimSpace(action.WorkspaceToolAction.Tool) == "" {
			return fmt.Errorf("CALL_WORKSPACE_TOOL action requires workspace_tool_action.tool")
		}
	default:
		return fmt.Errorf("unsupported action type %q", action.Type)
	}
	return nil
}

func extractActionJSONCandidates(raw string) []string {
	seen := map[string]struct{}{}
	candidates := []string{}
	addCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	for _, candidate := range extractFencedBlocks(raw) {
		addCandidate(candidate)
	}
	for _, candidate := range extractBalancedJSONObjects(raw) {
		addCandidate(candidate)
	}

	return candidates
}

func extractFencedBlocks(raw string) []string {
	var candidates []string
	remaining := raw
	for {
		fenceStart := strings.Index(remaining, "```")
		if fenceStart < 0 {
			break
		}
		remaining = remaining[fenceStart+len("```"):]
		lineEnd := strings.IndexAny(remaining, "\r\n")
		if lineEnd < 0 {
			break
		}

		language := strings.ToLower(strings.TrimSpace(remaining[:lineEnd]))
		contentStart := lineEnd + 1
		if remaining[lineEnd] == '\r' && contentStart < len(remaining) && remaining[contentStart] == '\n' {
			contentStart++
		}
		end := strings.Index(remaining[contentStart:], "```")
		block := remaining[contentStart:]
		if end >= 0 {
			block = remaining[contentStart : contentStart+end]
			remaining = remaining[contentStart+end+len("```"):]
		} else {
			remaining = ""
		}

		if language == "" || strings.HasPrefix(language, "json") {
			candidates = append(candidates, block)
		}
		if end < 0 {
			break
		}
	}
	return candidates
}

func extractBalancedJSONObjects(raw string) []string {
	var candidates []string
	for start := 0; start < len(raw); start++ {
		if raw[start] != '{' {
			continue
		}
		if candidate, ok := balancedJSONObjectAt(raw, start); ok {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func balancedJSONObjectAt(raw string, start int) (string, bool) {
	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return raw[start : i+1], true
			}
		}
	}

	return "", false
}

func parseBooleanText(raw string) (bool, error) {
	responseText := strings.ToLower(strings.TrimSpace(cleanModelTextResponse(raw)))
	responseText = strings.Trim(responseText, "\"'` \n\r\t")

	switch {
	case strings.HasPrefix(responseText, "true"):
		return true, nil
	case strings.HasPrefix(responseText, "false"):
		return false, nil
	default:
		return false, fmt.Errorf("unexpected boolean response: %s", raw)
	}
}
