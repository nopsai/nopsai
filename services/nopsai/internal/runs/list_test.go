package runs

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/pkg/models"
)

func TestBuildListRunsQueryFiltersTeamDescendants(t *testing.T) {
	teamID := 42

	query, args := BuildListRunsQuery(&teamID, false, "main", 50, 10)
	normalized := normalizeSQLForTest(query)

	for _, want := range []string{
		"WITH RECURSIVE selected_teams AS",
		"SELECT id FROM teams WHERE id = $1",
		"JOIN selected_teams sg ON g.parent_id = sg.id",
		"pr.team_id IN (SELECT id FROM selected_teams)",
		"pr.git_ref = $2",
		"COALESCE(pr.pipeline_definition, '')",
		"LEFT JOIN pipeline_run_usage_summary prus ON prus.run_id = pr.run_id",
		"LEFT JOIN LATERAL ( SELECT COUNT(*)::int AS total",
		"FROM pipeline_run_outputs WHERE run_id = pr.run_id",
		"COALESCE(prus.ai_total_tokens, 0)::bigint",
		"COALESCE(outputs.generating, 0)::int",
		"ORDER BY pr.created_at DESC LIMIT 50 OFFSET 10",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("query missing %q in %q", want, normalized)
		}
	}
	if len(args) != 2 {
		t.Fatalf("args = %#v, want 2 args", args)
	}
	if args[0] != teamID {
		t.Fatalf("team arg = %#v, want %d", args[0], teamID)
	}
	if args[1] != "refs/heads/main" {
		t.Fatalf("branch arg = %#v, want refs/heads/main", args[1])
	}
}

func TestFinalOutputAggregateStatus(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		runStatus  string
		total      int
		pending    int
		generating int
		generated  int
		failed     int
		cancelled  int
		want       string
	}{
		{name: "not configured", want: ""},
		{name: "active configured run waits for output generation", configured: 1, runStatus: "running", want: "waiting"},
		{name: "terminal configured run did not create outputs", configured: 1, runStatus: "success", want: "not_generated"},
		{name: "pending output", configured: 1, runStatus: "success", total: 1, pending: 1, want: "pending"},
		{name: "generating output", configured: 1, runStatus: "success", total: 1, generating: 1, want: "generating"},
		{name: "generated output", configured: 1, runStatus: "success", total: 1, generated: 1, want: "success"},
		{name: "failed output", configured: 1, runStatus: "success", total: 1, failed: 1, want: "failure"},
		{name: "partial failure", configured: 2, runStatus: "success", total: 2, generated: 1, failed: 1, want: "partial_failure"},
		{name: "cancelled output", configured: 1, runStatus: "cancelled", total: 1, cancelled: 1, want: "cancelled"},
		{name: "partial cancellation", configured: 2, runStatus: "cancelled", total: 2, generated: 1, cancelled: 1, want: "partial_cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FinalOutputAggregateStatus(tt.configured, tt.runStatus, tt.total, tt.pending, tt.generating, tt.generated, tt.failed, tt.cancelled)
			if got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeFinalOutputStatus(t *testing.T) {
	updatedAt := time.Date(2026, 7, 21, 8, 0, 0, 0, time.UTC)
	summary := SummarizeFinalOutputStatus(
		2,
		"success",
		2,
		0,
		1,
		1,
		0,
		0,
		sql.NullTime{Time: updatedAt, Valid: true},
	)

	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.Status != "generating" || summary.Configured != 2 || summary.Generated != 1 || summary.Generating != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.UpdatedAt == nil || !summary.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated at = %#v", summary.UpdatedAt)
	}
	if SummarizeFinalOutputStatus(0, "success", 0, 0, 0, 0, 0, 0, sql.NullTime{}) != nil {
		t.Fatal("runs without configured or stored outputs should not report final output status")
	}
}

func TestCountConfiguredFinalOutputs(t *testing.T) {
	count := CountConfiguredFinalOutputs(`
name: report
output:
  items:
    - name: Summary
      type: markdown
      prompt: Summarize the run.
    - name: Dashboard
      type: dashboard
      prompt: Build dashboard blocks.
`)
	if count != 2 {
		t.Fatalf("configured output count = %d, want 2", count)
	}
	if CountConfiguredFinalOutputs("output: [") != 0 {
		t.Fatal("malformed pipeline YAML should not report configured outputs")
	}
}

func TestApplyDirectChildRunStatusesAggregatesListStatuses(t *testing.T) {
	start := time.Unix(1_700_000_000, 0).UTC()
	finished := start.Add(30 * time.Second)
	queryer := &childStatusQueryer{
		rows: &childStatusRows{rows: []childStatusRow{
			{
				parentRunID: "parent-running",
				childRunID:  "child-running",
				status:      "running",
				startedAt:   sql.NullTime{Time: start, Valid: true},
			},
			{
				parentRunID: "parent-failed",
				childRunID:  "child-failed",
				status:      "failure",
				startedAt:   sql.NullTime{Time: start, Valid: true},
				finishedAt:  sql.NullTime{Time: finished, Valid: true},
			},
			{
				parentRunID: "parent-warning",
				childRunID:  "child-warning",
				status:      "warning",
				startedAt:   sql.NullTime{Time: start, Valid: true},
				finishedAt:  sql.NullTime{Time: finished, Valid: true},
			},
		}},
	}
	runs := []models.RunListItem{
		{
			RunID:      "parent-running",
			Status:     "success",
			StartedAt:  start,
			FinishedAt: start.Add(10 * time.Second),
			IsComplete: true,
			FinalOutputStatus: &models.FinalOutputStatusSummary{
				Status:     "not_generated",
				Configured: 1,
			},
		},
		{
			RunID:      "parent-failed",
			Status:     "success",
			StartedAt:  start,
			FinishedAt: start.Add(10 * time.Second),
			IsComplete: true,
		},
		{
			RunID:      "parent-warning",
			Status:     "success",
			StartedAt:  start,
			FinishedAt: start.Add(10 * time.Second),
			IsComplete: true,
		},
	}

	got, err := ApplyDirectChildRunStatuses(context.Background(), queryer, runs)
	if err != nil {
		t.Fatalf("ApplyDirectChildRunStatuses() error = %v", err)
	}
	if got[0].Status != "running" || got[0].IsComplete {
		t.Fatalf("first run = %#v, want running and incomplete", got[0])
	}
	if got[0].FinalOutputStatus == nil || got[0].FinalOutputStatus.Status != "waiting" {
		t.Fatalf("first run output status = %#v, want waiting after child status aggregation", got[0].FinalOutputStatus)
	}
	if got[1].Status != "failure" || !got[1].IsComplete || got[1].FinishedAt != finished {
		t.Fatalf("second run = %#v, want failed and complete at child finish", got[1])
	}
	if got[2].Status != "warning" || !got[2].IsComplete || got[2].FinishedAt != finished {
		t.Fatalf("third run = %#v, want warning and complete at child finish", got[2])
	}
	if !strings.Contains(queryer.query, "parent_run_id::text = ANY($1::text[])") {
		t.Fatalf("query = %q, want direct child status lookup", queryer.query)
	}
	argRunIDs, ok := queryer.args[0].([]string)
	if !ok {
		t.Fatalf("first query arg = %#v, want []string", queryer.args[0])
	}
	if strings.Join(argRunIDs, ",") != "parent-running,parent-failed,parent-warning" {
		t.Fatalf("parent IDs = %#v, want input run IDs", argRunIDs)
	}
}

func TestTeamResolutionCandidatesPreferRepoBeforePipelinePath(t *testing.T) {
	got := TeamResolutionCandidates("", "payments/backend", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []TeamResolutionCandidate{
		{Kind: TeamResolutionRepo, Value: "acme/payments-api"},
		{Kind: TeamResolutionPath, Value: "payments/backend"},
	}
	if len(got) != len(want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("candidate[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestTeamResolutionCandidatesUsePipelinePathWithoutRepo(t *testing.T) {
	got := TeamResolutionCandidates("", "payments/backend", nil)

	want := []TeamResolutionCandidate{{Kind: TeamResolutionPath, Value: "payments/backend"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func normalizeSQLForTest(query string) string {
	return strings.Join(strings.Fields(query), " ")
}

type childStatusQueryer struct {
	rows  pgx.Rows
	query string
	args  []any
}

func (q *childStatusQueryer) Query(_ context.Context, query string, args ...any) (pgx.Rows, error) {
	q.query = normalizeSQLForTest(query)
	q.args = args
	return q.rows, nil
}

func (q *childStatusQueryer) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("QueryRow is not used by ApplyDirectChildRunStatuses")
}

type childStatusRow struct {
	parentRunID string
	childRunID  string
	status      string
	startedAt   sql.NullTime
	finishedAt  sql.NullTime
}

type childStatusRows struct {
	rows  []childStatusRow
	index int
}

func (r *childStatusRows) Close() {}

func (r *childStatusRows) Err() error {
	return nil
}

func (r *childStatusRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *childStatusRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *childStatusRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}

func (r *childStatusRows) Scan(dest ...any) error {
	row := r.rows[r.index-1]
	*(dest[0].(*string)) = row.parentRunID
	*(dest[1].(*string)) = row.childRunID
	*(dest[2].(*string)) = row.status
	*(dest[3].(*sql.NullTime)) = row.startedAt
	*(dest[4].(*sql.NullTime)) = row.finishedAt
	return nil
}

func (r *childStatusRows) Values() ([]any, error) {
	return nil, nil
}

func (r *childStatusRows) RawValues() [][]byte {
	return nil
}

func (r *childStatusRows) Conn() *pgx.Conn {
	return nil
}
