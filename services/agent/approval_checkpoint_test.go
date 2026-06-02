package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceArchiveRestoreRoundTrip(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "dist"), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "dist", "app.txt"), []byte("release artifact"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	archive, err := archiveWorkspace(source, defaultApprovalCheckpointMaxBytes)
	if err != nil {
		t.Fatalf("archiveWorkspace() error = %v", err)
	}

	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	if err := restoreWorkspaceArchive(target, archive); err != nil {
		t.Fatalf("restoreWorkspaceArchive() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(target, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("stale workspace file still exists, err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(target, "dist", "app.txt"))
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "release artifact" {
		t.Fatalf("restored file = %q", string(got))
	}
}

func TestWorkspaceArchiveHonorsSizeLimit(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "large.txt"), []byte("large checkpoint payload"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	if _, err := archiveWorkspace(source, 1); err == nil {
		t.Fatal("expected size limit error")
	}
}
