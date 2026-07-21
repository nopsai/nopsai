package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacectx "nopsai/services/agent/internal/workspace"

	"github.com/rs/zerolog"
)

func TestBuildSharedDirectoryContextAnnotatesFileIdentity(t *testing.T) {
	listing, identities := buildSharedDirectoryContext(map[string]string{
		"./README.md": "hello",
	}, 7)

	value := listing["README.md"]
	for _, want := range []string{
		"NopsAI File Identity:",
		"path: README.md",
		"sha256: 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		"size: 5",
		"workspace_revision: 7",
		"--- Content ---\nhello",
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("annotated listing missing %q:\n%s", want, value)
		}
	}
	identity := identities["README.md"]
	if identity.Path != "README.md" || identity.SHA256 == "" || identity.Size != 5 || identity.WorkspaceRevision != 7 {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestValidateReplaceFilePreconditionRejectsChangedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	precondition := FilePrecondition{
		Path:              "README.md",
		ExpectedSHA256:    textSHA256("old"),
		WorkspaceRevision: 3,
	}
	if err := ValidateReplaceFilePrecondition(precondition, root, 3); err != nil {
		t.Fatalf("ValidateReplaceFilePrecondition() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	if err := ValidateReplaceFilePrecondition(precondition, root, 4); err == nil {
		t.Fatal("ValidateReplaceFilePrecondition() error = nil after file changed")
	}
}

func TestValidateReplaceFilePreconditionRejectsUnknownFileAfterRevisionChange(t *testing.T) {
	precondition := FilePrecondition{
		Path:              "new-file.md",
		WorkspaceRevision: 3,
	}
	if err := ValidateReplaceFilePrecondition(precondition, t.TempDir(), 3); err != nil {
		t.Fatalf("ValidateReplaceFilePrecondition() same revision error = %v", err)
	}
	if err := ValidateReplaceFilePrecondition(precondition, t.TempDir(), 4); err == nil {
		t.Fatal("ValidateReplaceFilePrecondition() error = nil for unknown file after revision change")
	}
}

func TestBuildSharedDirectoryContextFromWorkspaceIndexUsesFullFileIdentity(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("a", 30000)
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}
	logger := zerolog.Nop()
	index := workspacectx.NewIndex(root, nil, nil)
	if err := index.Refresh(&logger, 11); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	listing, identities := buildSharedDirectoryContextFromWorkspaceIndex(index)
	identity := identities["large.txt"]
	if identity.SHA256 != textSHA256(content) || identity.Size != len(content) || identity.WorkspaceRevision != 11 {
		t.Fatalf("identity = %#v, want full file hash/size", identity)
	}
	shared := listing["large.txt"]
	if !strings.Contains(shared, "sha256: "+textSHA256(content)) || !strings.Contains(shared, "...[truncated]") {
		t.Fatalf("shared context missing full identity or truncation marker:\n%s", shared)
	}
}
