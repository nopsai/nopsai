package nopsai

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	runquery "nopsai/services/nopsai/internal/runs"
)

func TestHandleListRunsRejectsInvalidTeamID(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/runs?teamId=not-a-number", nil)
	rec := httptest.NewRecorder()

	app.handleListRuns(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBuildListRunsQueryFiltersTeamDescendants(t *testing.T) {
	teamID := 42

	query, args := runquery.BuildListRunsQuery(&teamID, false, "main", 50, 10)
	normalized := normalizeSQLForTest(query)

	for _, want := range []string{
		"WITH RECURSIVE selected_teams AS",
		"SELECT id FROM teams WHERE id = $1",
		"JOIN selected_teams sg ON g.parent_id = sg.id",
		"pr.team_id IN (SELECT id FROM selected_teams)",
		"pr.git_ref = $2",
		"LEFT JOIN pipeline_run_usage_summary prus ON prus.run_id = pr.run_id",
		"COALESCE(prus.ai_total_tokens, 0)::bigint",
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

func TestBuildListRunsQueryWithoutTeamKeepsFlatList(t *testing.T) {
	query, args := runquery.BuildListRunsQuery(nil, false, "", 300, 0)
	normalized := normalizeSQLForTest(query)

	if strings.Contains(normalized, "WITH RECURSIVE selected_teams") {
		t.Fatalf("query unexpectedly uses recursive teams: %q", normalized)
	}
	if strings.Contains(normalized, "pr.team_id IN") {
		t.Fatalf("query unexpectedly filters teams: %q", normalized)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
	if !strings.Contains(normalized, "ORDER BY pr.created_at DESC LIMIT 300 OFFSET 0") {
		t.Fatalf("query missing default pagination: %q", normalized)
	}
}

func TestBuildListRunsQueryRootFiltersUnteamedRuns(t *testing.T) {
	query, args := runquery.BuildListRunsQuery(nil, true, "", 300, 0)
	normalized := normalizeSQLForTest(query)

	for _, want := range []string{
		"pr.team_id IS NULL",
		"ORDER BY pr.created_at DESC LIMIT 300 OFFSET 0",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("query missing %q in %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "WITH RECURSIVE selected_teams") {
		t.Fatalf("query unexpectedly uses recursive teams: %q", normalized)
	}
	if strings.Contains(normalized, "FROM teams WHERE parent_id IS NULL") {
		t.Fatalf("query unexpectedly matches a named root team: %q", normalized)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRunTeamResolutionCandidatesPreferExplicitTeamPath(t *testing.T) {
	got := runquery.TeamResolutionCandidates(" platform/prod ", "payments/backend", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []runquery.TeamResolutionCandidate{{
		Kind:     runquery.TeamResolutionPath,
		Value:    "platform/prod",
		Required: true,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestRunTeamResolutionCandidatesPreferRepoBeforePipelinePath(t *testing.T) {
	got := runquery.TeamResolutionCandidates("", "payments/backend", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []runquery.TeamResolutionCandidate{
		{Kind: runquery.TeamResolutionRepo, Value: "acme/payments-api"},
		{Kind: runquery.TeamResolutionPath, Value: "payments/backend"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestRunTeamResolutionCandidatesUsePipelinePathWithoutRepo(t *testing.T) {
	got := runquery.TeamResolutionCandidates("", "payments/backend", nil)

	want := []runquery.TeamResolutionCandidate{{Kind: runquery.TeamResolutionPath, Value: "payments/backend"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestRunTeamResolutionCandidatesFallBackToRepoOnly(t *testing.T) {
	got := runquery.TeamResolutionCandidates("", "", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []runquery.TeamResolutionCandidate{{Kind: runquery.TeamResolutionRepo, Value: "acme/payments-api"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func normalizeSQLForTest(query string) string {
	return strings.Join(strings.Fields(query), " ")
}
