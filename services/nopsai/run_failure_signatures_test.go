package nopsai

import (
	"strings"
	"testing"
)

// The release pipeline's helm login is the run that exposed this: the analysis
// had the decisive line in hand and still answered "the run failed, go read the
// logs". These tests pin the conclusion to the evidence.
func TestAnalyzeRunFailureLogsNamesAnInteractivePrompt(t *testing.T) {
	cause, action, evidence := analyzeRunFailureLogs([]runFailureLogLine{
		{"line": "release_phase=publish-helm-chart", "level": "info", "step_name": "publish-helm-chart"},
		{
			"line":      "Error: inappropriate ioctl for device",
			"level":     "error",
			"stream":    "stderr",
			"step_name": "publish-helm-chart",
			"task_name": "publish-helm-chart",
		},
		{"line": "release_phase=publish-helm-chart failed line=21 command=helm registry login ghcr.io exit_code=1", "level": "info"},
	})

	if cause == "" {
		t.Fatal("expected a root cause for a terminal-read failure")
	}
	if !strings.Contains(cause, "no terminal") {
		t.Fatalf("cause should explain the missing terminal, got %q", cause)
	}
	if !strings.Contains(action, "--password-stdin") {
		t.Fatalf("action should name the non-interactive fix, got %q", action)
	}
	if evidence["step_name"] != "publish-helm-chart" {
		t.Fatalf("evidence should carry the failing step, got %v", evidence["step_name"])
	}
	if !strings.Contains(evidence["line"].(string), "inappropriate ioctl") {
		t.Fatalf("evidence should be the decisive line, got %v", evidence["line"])
	}
}

func TestAnalyzeRunFailureLogsReadsCredentialRejection(t *testing.T) {
	cause, action, evidence := analyzeRunFailureLogs([]runFailureLogLine{
		{"line": "denied: permission_denied: write_package", "level": "error"},
	})

	if !strings.Contains(cause, "credentials") {
		t.Fatalf("cause should name the credential rejection, got %q", cause)
	}
	if action == "" || len(evidence) == 0 {
		t.Fatal("a recognised signature must carry an action and its evidence")
	}
}

func TestAnalyzeRunFailureLogsFallsBackToTheFirstErrorLine(t *testing.T) {
	cause, action, evidence := analyzeRunFailureLogs([]runFailureLogLine{
		{"line": "starting step", "level": "info"},
		{"line": "something unusual went wrong in the widget", "level": "error", "step_name": "build"},
		{"line": "a later error nobody should be pointed at", "level": "error"},
	})

	if cause == "" || action == "" {
		t.Fatal("an unrecognised failure still has to name its evidence")
	}
	if evidence["line"] != "something unusual went wrong in the widget" {
		t.Fatalf("fallback must point at the first error line, got %v", evidence["line"])
	}
}

// An info-level run with nothing wrong in it must not invent a cause; the caller
// falls back to the recorded failure reason instead.
func TestAnalyzeRunFailureLogsStaysSilentWithoutEvidence(t *testing.T) {
	cause, action, evidence := analyzeRunFailureLogs([]runFailureLogLine{
		{"line": "step completed", "level": "info"},
		{"line": "", "level": "info"},
	})

	if cause != "" || action != "" || len(evidence) != 0 {
		t.Fatalf("expected no conclusion, got cause=%q action=%q evidence=%v", cause, action, evidence)
	}
}

func TestAnalysisEvidenceSectionKeepsArrayPayloads(t *testing.T) {
	section, limitation := analysisEvidenceSection("logs", map[string]any{
		"response": []any{
			map[string]any{"line": "Error: inappropriate ioctl for device"},
		},
	})

	if limitation != "" {
		t.Fatalf("an array payload is readable evidence, got limitation %q", limitation)
	}
	rows := analysisRows(section, "logs")
	if len(rows) != 1 {
		t.Fatalf("expected the array to be readable as rows, got %d", len(rows))
	}
}

func TestAnalysisEvidenceSectionReportsUnreadablePayloads(t *testing.T) {
	if _, limitation := analysisEvidenceSection("logs", map[string]any{"response_text": "not json"}); limitation == "" {
		t.Fatal("a payload with no response must stay a limitation, not a silent zero")
	}
}
