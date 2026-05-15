package main

import "testing"

func TestResolveActionFilePathUsesWorkingDirectory(t *testing.T) {
	got, err := resolveActionFilePath("/tmp/test", "./pipeline_output.txt")
	if err != nil {
		t.Fatalf("resolveActionFilePath() error = %v", err)
	}
	if got != "/tmp/test/pipeline_output.txt" {
		t.Fatalf("resolveActionFilePath() = %q, want %q", got, "/tmp/test/pipeline_output.txt")
	}
}

func TestResolveActionFilePathTreatsAbsoluteActionPathAsRelative(t *testing.T) {
	got, err := resolveActionFilePath("/tmp/test", "/nested/output.txt")
	if err != nil {
		t.Fatalf("resolveActionFilePath() error = %v", err)
	}
	if got != "/tmp/test/nested/output.txt" {
		t.Fatalf("resolveActionFilePath() = %q, want %q", got, "/tmp/test/nested/output.txt")
	}
}

func TestResolveActionFilePathRejectsEscape(t *testing.T) {
	if _, err := resolveActionFilePath("/tmp/test", "../secret.txt"); err == nil {
		t.Fatal("resolveActionFilePath() error = nil, want error")
	}
}
