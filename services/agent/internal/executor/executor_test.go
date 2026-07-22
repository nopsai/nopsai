package executor

import (
	"strings"
	"testing"

	"nopsai/pkg/proto"
)

func TestResolveActionFilePathUsesWorkingDirectory(t *testing.T) {
	got, err := ResolveActionFilePath("/tmp/test", "./pipeline_output.txt")
	if err != nil {
		t.Fatalf("ResolveActionFilePath() error = %v", err)
	}
	if got != "/tmp/test/pipeline_output.txt" {
		t.Fatalf("ResolveActionFilePath() = %q, want %q", got, "/tmp/test/pipeline_output.txt")
	}
}

func TestResolveActionFilePathTreatsAbsoluteActionPathAsRelative(t *testing.T) {
	got, err := ResolveActionFilePath("/tmp/test", "/nested/output.txt")
	if err != nil {
		t.Fatalf("ResolveActionFilePath() error = %v", err)
	}
	if got != "/tmp/test/nested/output.txt" {
		t.Fatalf("ResolveActionFilePath() = %q, want %q", got, "/tmp/test/nested/output.txt")
	}
}

func TestResolveActionFilePathRejectsEscape(t *testing.T) {
	if _, err := ResolveActionFilePath("/tmp/test", "../secret.txt"); err == nil {
		t.Fatal("ResolveActionFilePath() error = nil, want error")
	}
}

func TestPrepareActionBuildsReplaceFileCommand(t *testing.T) {
	prepared, err := PrepareAction(&proto.Action{
		Type: "REPLACE_FILE",
		Payload: &proto.Action_FileAction{
			FileAction: &proto.FileAction{
				Path:    "./notes/output.txt",
				Content: "hello 'quoted' world",
			},
		},
	}, "/workspace")
	if err != nil {
		t.Fatalf("PrepareAction() error = %v", err)
	}
	if prepared.ReturnOnly {
		t.Fatal("PrepareAction() ReturnOnly = true, want false")
	}
	if !strings.Contains(prepared.Command, "base64 -d > '/workspace/notes/output.txt'") {
		t.Fatalf("PrepareAction() command = %q, want target file redirection", prepared.Command)
	}
}

func TestPrepareActionReturnsAnswerWithoutCommand(t *testing.T) {
	prepared, err := PrepareAction(&proto.Action{
		Type: "RETURN_ANSWER",
		Payload: &proto.Action_AnswerAction{
			AnswerAction: &proto.AnswerAction{Answer: "done"},
		},
	}, "/workspace")
	if err != nil {
		t.Fatalf("PrepareAction() error = %v", err)
	}
	if !prepared.ReturnOnly {
		t.Fatal("PrepareAction() ReturnOnly = false, want true")
	}
	if prepared.Stdout != "done" {
		t.Fatalf("PrepareAction() Stdout = %q, want done", prepared.Stdout)
	}
	if prepared.Command != "" {
		t.Fatalf("PrepareAction() Command = %q, want empty", prepared.Command)
	}
}

func TestOutputCaptureStreamsCompleteLinesAndKeepsFullOutput(t *testing.T) {
	var lines []string
	capture := NewOutputCapture(OutputStreamStdout, func(stream OutputStream, line string) {
		if stream != OutputStreamStdout {
			t.Fatalf("stream = %q, want stdout", stream)
		}
		lines = append(lines, line)
	})

	if _, err := capture.Write([]byte("first\nsec")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := capture.Write([]byte("ond\r\nthird")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	capture.Flush()

	if got := strings.Join(lines, "|"); got != "first|second|third" {
		t.Fatalf("streamed lines = %q, want first|second|third", got)
	}
	if capture.String() != "first\nsecond\r\nthird" {
		t.Fatalf("full output = %q, want original output", capture.String())
	}
}
