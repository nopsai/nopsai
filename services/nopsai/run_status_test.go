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
		{status: "cancelled", want: false},
		{status: "running", want: false},
	}

	for _, tt := range tests {
		if got := isCompletedRunStatus(tt.status); got != tt.want {
			t.Fatalf("isCompletedRunStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
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
		"status NOT IN ('success', 'failure', 'failure (ignored)', 'cancelled', 'timed_out')",
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
