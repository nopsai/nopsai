package runs

import (
	"strings"
	"testing"
)

func TestBuildListRunsQueryFiltersGroupDescendants(t *testing.T) {
	groupID := 42

	query, args := BuildListRunsQuery(&groupID, false, "main", 50, 10)
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

func TestGroupResolutionCandidatesUsePipelinePathBeforeRepo(t *testing.T) {
	got := GroupResolutionCandidates("", "payments/backend", map[string]string{
		"repo_owner": "acme",
		"repo_name":  "payments-api",
	})

	want := []GroupResolutionCandidate{
		{Kind: GroupResolutionPath, Value: "payments/backend"},
		{Kind: GroupResolutionRepo, Value: "acme/payments-api"},
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

func normalizeSQLForTest(query string) string {
	return strings.Join(strings.Fields(query), " ")
}
