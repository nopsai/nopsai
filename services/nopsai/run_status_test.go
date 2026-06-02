package main

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestNormalizeFinalizeRunStatus(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "success preserved", raw: "success", want: "success"},
		{name: "cancelled preserved", raw: "cancelled", want: "cancelled"},
		{name: "failure normalized", raw: "failure", want: "failure"},
		{name: "unknown treated as failure", raw: "timed_out", want: "failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeFinalizeRunStatus(tt.raw); got != tt.want {
				t.Fatalf("normalizeFinalizeRunStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestIsCompletedRunStatus(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{status: "success", want: true},
		{status: "failure", want: true},
		{status: "timed_out", want: true},
		{status: "rejected", want: true},
		{status: "cancelled", want: false},
		{status: "running", want: false},
		{status: "waiting_approval", want: false},
	}

	for _, tt := range tests {
		if got := isCompletedRunStatus(tt.status); got != tt.want {
			t.Fatalf("isCompletedRunStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestBuildLaunchAgentContainerNameAddsUniqueSuffix(t *testing.T) {
	baseName := buildAgentContainerName("deploy-prod", "payments-api", "trigger-123456789", "run-abcdef123456")
	first := buildLaunchAgentContainerName("deploy-prod", "payments-api", "trigger-123456789", "run-abcdef123456")
	second := buildLaunchAgentContainerName("deploy-prod", "payments-api", "trigger-123456789", "run-abcdef123456")

	if !strings.HasPrefix(first, baseName+"-") {
		t.Fatalf("first launch name = %q, want prefix %q", first, baseName+"-")
	}
	if first == second {
		t.Fatalf("expected unique launch names, got %q twice", first)
	}
	if len(first) > dockerContainerNameMax || len(second) > dockerContainerNameMax {
		t.Fatalf("launch name exceeded docker max length: %d, %d", len(first), len(second))
	}
}

func TestBuildLaunchAgentContainerNameCapsLongNames(t *testing.T) {
	name := buildLaunchAgentContainerName(strings.Repeat("pipeline", 80), strings.Repeat("repo", 80), "trigger-123456789", "run-abcdef123456")

	if len(name) != dockerContainerNameMax {
		t.Fatalf("launch name length = %d, want %d", len(name), dockerContainerNameMax)
	}
	if lastDash := strings.LastIndex(name, "-"); lastDash != dockerContainerNameMax-9 {
		t.Fatalf("launch suffix starts at %d, want %d in %q", lastDash, dockerContainerNameMax-9, name)
	}
}

func TestNullableGitCheckRunID(t *testing.T) {
	tests := []struct {
		name      string
		context   map[string]string
		wantValid bool
		wantValue int64
	}{
		{name: "nil context", context: nil},
		{name: "blank value", context: map[string]string{"check_run_id": ""}},
		{name: "whitespace value", context: map[string]string{"check_run_id": "   "}},
		{name: "invalid value", context: map[string]string{"check_run_id": "not-a-number"}},
		{name: "valid value", context: map[string]string{"check_run_id": "12345"}, wantValid: true, wantValue: 12345},
		{name: "valid with spaces", context: map[string]string{"check_run_id": " 678 "}, wantValid: true, wantValue: 678},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nullableGitCheckRunID(tt.context)
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v, want %v", got.Valid, tt.wantValid)
			}
			if got.Int64 != tt.wantValue {
				t.Fatalf("Int64 = %d, want %d", got.Int64, tt.wantValue)
			}
		})
	}
}

func TestMarkRunRunningPromotesNonTerminalRun(t *testing.T) {
	runner := &recordingRunExecRunner{}

	if err := markRunRunning(context.Background(), runner, "run-1"); err != nil {
		t.Fatalf("markRunRunning() error = %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 Exec call, got %d", len(runner.calls))
	}
	statement := strings.Join(strings.Fields(runner.calls[0].sql), " ")
	for _, want := range []string{
		"SET status = 'running'",
		"started_at = COALESCE(started_at, NOW())",
		"status NOT IN ('success', 'failure', 'failure (ignored)', 'cancelled', 'timed_out', 'waiting_approval', 'rejected')",
	} {
		if !strings.Contains(statement, want) {
			t.Fatalf("markRunRunning() SQL missing %q in %q", want, statement)
		}
	}
	if got := runner.calls[0].args[0]; got != "run-1" {
		t.Fatalf("markRunRunning() run id arg = %v, want run-1", got)
	}
}

type recordingRunExecRunner struct {
	calls []recordingRunExecCall
}

type recordingRunExecCall struct {
	sql  string
	args []any
}

func (r *recordingRunExecRunner) Exec(_ context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	r.calls = append(r.calls, recordingRunExecCall{sql: sql, args: arguments})
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
