package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestIndexListsSearchesAndReadsWorkspaceFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "hello workspace\n")
	writeFile(t, root, "src/main.go", "package main\nfunc main() {}\n")
	writeFile(t, root, "secret.pem", "private")
	logger := zerolog.Nop()

	index := NewIndex(root, []string{"README.md", "src/"}, []string{"secret.pem"})
	if err := index.Refresh(&logger, 3); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	files := index.ListFiles(10)
	if len(files) != 2 {
		t.Fatalf("files = %#v, want two visible files", files)
	}
	if files[0].Path != "README.md" || files[0].SHA256 == "" || files[0].WorkspaceRevision != 3 {
		t.Fatalf("first file = %#v", files[0])
	}
	results, err := index.SearchCode("func main", 10)
	if err != nil {
		t.Fatalf("SearchCode() error = %v", err)
	}
	if len(results) != 1 || results[0].Path != "src/main.go" || results[0].Line != 2 {
		t.Fatalf("search results = %#v", results)
	}
	read, err := index.ReadFile("README.md", 100)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if read.Content != "hello workspace\n" || read.SHA256 == "" || read.WorkspaceRevision != 3 {
		t.Fatalf("read file = %#v", read)
	}
	if _, err := index.ReadFile("secret.pem", 100); err == nil {
		t.Fatal("ReadFile() error = nil for ignored file")
	}
}

func TestWorkspaceToolsReturnBoundedJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "hello workspace\n")
	logger := zerolog.Nop()
	index := NewIndex(root, nil, nil)
	if err := index.Refresh(&logger, 5); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	tools := NewTools(index)
	result, err := tools.CallTool(t.Context(), "read_file", json.RawMessage(`{"path":"README.md"}`))
	if err != nil {
		t.Fatalf("CallTool(read_file) error = %v", err)
	}
	if !strings.Contains(string(result), `"workspace_revision":5`) || !strings.Contains(string(result), `"content":"hello workspace\n"`) {
		t.Fatalf("read_file result = %s", result)
	}
	if tools.SuccessfulToolCalls() != 1 || !strings.Contains(tools.ToolTranscript(), "Workspace tool result") {
		t.Fatalf("tool transcript/calls = %d %q", tools.SuccessfulToolCalls(), tools.ToolTranscript())
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
