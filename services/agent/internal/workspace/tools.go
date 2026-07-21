package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

type Tools struct {
	index       *Index
	mu          sync.Mutex
	calls       int
	log         string
	completeLog string
	identities  map[string]FileEntry
}

func NewTools(index *Index) *Tools {
	return &Tools{index: index, identities: map[string]FileEntry{}}
}

func (t *Tools) Enabled() bool {
	return t != nil && t.index != nil
}

func (t *Tools) SuccessfulToolCalls() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.calls
}

func (t *Tools) ToolTranscript() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.log
}

func (t *Tools) CompleteToolTranscript() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.completeLog
}

func (t *Tools) IdentityFor(path string) (FileEntry, bool) {
	if t == nil {
		return FileEntry{}, false
	}
	normalized := normalizeWorkspacePath(path)
	if normalized == "" {
		return FileEntry{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.identities[normalized]
	return entry, ok
}

func (t *Tools) ToolPrompt() string {
	if !t.Enabled() {
		return "**Workspace Tools:**\nNo workspace tools are available for this goal."
	}
	return strings.TrimSpace(`**Workspace Tools:**
You may call these NopsAI-managed workspace tools before choosing a final action. To call a tool, respond with JSON like {"action":{"type":"CALL_WORKSPACE_TOOL","workspace_tool_action":{"tool":"read_file","arguments":{"path":"README.md"}}}}. After a workspace tool result is returned in the history, either call another workspace tool, call an approved MCP tool, or choose EXECUTE_COMMAND, REPLACE_FILE, or RETURN_ANSWER.
- list_files: List current workspace file identities. arguments: {"limit": number}
- list_files: List current workspace file identities. arguments: {"limit": number, "cursor": string}. Continue with next_cursor when present.
- search_code: Search current text files by substring. arguments: {"query": string, "max_results": number, "cursor": string}. Continue with next_cursor when present.
- read_file: Read one current text file byte range by relative path. arguments: {"path": string, "offset": number, "max_bytes": number}. Continue with next_offset until eof is true.
Tool results include path, sha256, size, workspace_revision, and pagination/range cursors. Use those identities when reasoning about file changes; stale REPLACE_FILE actions are rejected.`) + "\n"
}

func (t *Tools) CallTool(_ context.Context, toolName string, arguments json.RawMessage) (json.RawMessage, error) {
	if !t.Enabled() {
		return nil, fmt.Errorf("workspace tools are not available")
	}
	toolName = strings.TrimSpace(toolName)
	var result any
	switch toolName {
	case "list_files":
		var args struct {
			Limit  int    `json:"limit"`
			Cursor string `json:"cursor"`
		}
		_ = json.Unmarshal(arguments, &args)
		page := t.index.ListFilesPage(args.Cursor, args.Limit)
		result = page
		for _, entry := range page.Files {
			t.recordIdentity(entry)
		}
	case "search_code":
		var args struct {
			Query      string `json:"query"`
			MaxResults int    `json:"max_results"`
			Cursor     string `json:"cursor"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid search_code arguments: %w", err)
		}
		page, err := t.index.SearchCodePage(args.Query, args.Cursor, args.MaxResults)
		if err != nil {
			return nil, err
		}
		result = page
		for _, searchResult := range page.Results {
			t.recordIdentity(FileEntry{
				Path:              searchResult.Path,
				SHA256:            searchResult.SHA256,
				Size:              searchResult.Size,
				WorkspaceRevision: searchResult.WorkspaceRevision,
				Text:              true,
			})
		}
	case "read_file":
		var args struct {
			Path     string `json:"path"`
			Offset   int64  `json:"offset"`
			MaxBytes int    `json:"max_bytes"`
		}
		if err := json.Unmarshal(arguments, &args); err != nil {
			return nil, fmt.Errorf("invalid read_file arguments: %w", err)
		}
		file, err := t.index.ReadFileRange(args.Path, args.Offset, args.MaxBytes)
		if err != nil {
			return nil, err
		}
		t.recordIdentity(FileEntry{
			Path:              file.Path,
			SHA256:            file.SHA256,
			Size:              file.Size,
			WorkspaceRevision: file.WorkspaceRevision,
			Text:              true,
		})
		result = file
	default:
		return nil, fmt.Errorf("workspace tool %q is not available", toolName)
	}
	payload := t.index.ToolResultJSON(result)
	t.recordSuccessfulToolCall(toolName, arguments, payload)
	return payload, nil
}

func (t *Tools) recordSuccessfulToolCall(toolName string, arguments json.RawMessage, result json.RawMessage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls++
	t.completeLog += fmt.Sprintf(
		"\nWorkspace tool result: tool=%s arguments=%s result=%s\n",
		toolName,
		jsonString(arguments, 0),
		jsonString(result, 0),
	)
	t.log += fmt.Sprintf(
		"\nWorkspace tool result: tool=%s arguments=%s result=%s\n",
		toolName,
		jsonString(arguments, 2048),
		jsonString(result, 24000),
	)
}

func (t *Tools) recordIdentity(entry FileEntry) {
	if t == nil {
		return
	}
	path := normalizeWorkspacePath(entry.Path)
	if path == "" || entry.SHA256 == "" {
		return
	}
	entry.Path = path
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.identities == nil {
		t.identities = map[string]FileEntry{}
	}
	t.identities[path] = entry
}

func jsonString(raw json.RawMessage, max int) string {
	trimmed := strings.TrimSpace(string(raw))
	if max > 0 && len(trimmed) > max {
		return trimmed[:max] + "...[truncated]"
	}
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}
