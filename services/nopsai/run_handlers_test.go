package nopsai

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	runquery "nopsai/services/nopsai/internal/runs"
)

func TestHandleListRunsRejectsInvalidGroupID(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/v1/runs?groupId=not-a-number", nil)
	rec := httptest.NewRecorder()

	app.handleListRuns(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestBuildListRunsQueryFiltersGroupDescendants(t *testing.T) {
	groupID := 42

	query, args := runquery.BuildListRunsQuery(&groupID, false, "main", 50, 10)
	normalized := normalizeSQLForTest(query)

	for _, want := range []string{
		"WITH RECURSIVE selected_groups AS",
		"SELECT id FROM groups WHERE id = $1",
		"JOIN selected_groups sg ON g.parent_id = sg.id",
		"pr.group_id IN (SELECT id FROM selected_groups)",
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
	if args[0] != groupID {
		t.Fatalf("group arg = %#v, want %d", args[0], groupID)
	}
	if args[1] != "refs/heads/main" {
		t.Fatalf("branch arg = %#v, want refs/heads/main", args[1])
	}
}

func TestBuildListRunsQueryWithoutGroupKeepsFlatList(t *testing.T) {
	query, args := runquery.BuildListRunsQuery(nil, false, "", 300, 0)
	normalized := normalizeSQLForTest(query)

	if strings.Contains(normalized, "WITH RECURSIVE selected_groups") {
		t.Fatalf("query unexpectedly uses recursive groups: %q", normalized)
	}
	if strings.Contains(normalized, "pr.group_id IN") {
		t.Fatalf("query unexpectedly filters groups: %q", normalized)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
	if !strings.Contains(normalized, "ORDER BY pr.created_at DESC LIMIT 300 OFFSET 0") {
		t.Fatalf("query missing default pagination: %q", normalized)
	}
}

func TestBuildListRunsQueryRootFiltersUngroupedRuns(t *testing.T) {
	query, args := runquery.BuildListRunsQuery(nil, true, "", 300, 0)
	normalized := normalizeSQLForTest(query)

	for _, want := range []string{
		"pr.group_id IS NULL",
		"ORDER BY pr.created_at DESC LIMIT 300 OFFSET 0",
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("query missing %q in %q", want, normalized)
		}
	}
	if strings.Contains(normalized, "WITH RECURSIVE selected_groups") {
		t.Fatalf("query unexpectedly uses recursive groups: %q", normalized)
	}
	if strings.Contains(normalized, "FROM groups WHERE parent_id IS NULL") {
		t.Fatalf("query unexpectedly matches a named root group: %q", normalized)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestRunGroupResolutionCandidatesPreferExplicitGroupPath(t *testing.T) {
	got := runquery.GroupResolutionCandidates(" platform/prod ", "payments/backend", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []runquery.GroupResolutionCandidate{{
		Kind:     runquery.GroupResolutionPath,
		Value:    "platform/prod",
		Required: true,
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestRunGroupResolutionCandidatesPreferRepoBeforePipelinePath(t *testing.T) {
	got := runquery.GroupResolutionCandidates("", "payments/backend", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []runquery.GroupResolutionCandidate{
		{Kind: runquery.GroupResolutionRepo, Value: "acme/payments-api"},
		{Kind: runquery.GroupResolutionPath, Value: "payments/backend"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestRunGroupResolutionCandidatesUsePipelinePathWithoutRepo(t *testing.T) {
	got := runquery.GroupResolutionCandidates("", "payments/backend", nil)

	want := []runquery.GroupResolutionCandidate{{Kind: runquery.GroupResolutionPath, Value: "payments/backend"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func TestRunGroupResolutionCandidatesFallBackToRepoOnly(t *testing.T) {
	got := runquery.GroupResolutionCandidates("", "", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []runquery.GroupResolutionCandidate{{Kind: runquery.GroupResolutionRepo, Value: "acme/payments-api"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %#v, want %#v", got, want)
	}
}

func normalizeSQLForTest(query string) string {
	return strings.Join(strings.Fields(query), " ")
}
