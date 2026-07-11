package nopsai

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestParseResourceAccessPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantOperation string
		wantType      string
		wantID        string
		wantGrantID   string
	}{
		{
			name:          "access",
			path:          "/v1/resources/pipeline/team-1/build/access",
			wantOperation: "access",
			wantType:      "pipeline",
			wantID:        "team-1/build",
		},
		{
			name:          "create grant",
			path:          "/v1/resources/scope/team-1/dev/grants",
			wantOperation: "grants",
			wantType:      "scope",
			wantID:        "team-1/dev",
		},
		{
			name:          "delete grant",
			path:          "/v1/resources/step/platform/docker-build/grants/grant_42",
			wantOperation: "grant",
			wantType:      "step",
			wantID:        "platform/docker-build",
			wantGrantID:   "grant_42",
		},
		{
			name:          "encoded path segment",
			path:          "/v1/resources/pipeline/team%201/build/access",
			wantOperation: "access",
			wantType:      "pipeline",
			wantID:        "team 1/build",
		},
		{
			name:          "default scope",
			path:          "/v1/resources/scope/default/access",
			wantOperation: "access",
			wantType:      "scope",
			wantID:        "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResourceAccessPath(tt.path)
			if err != nil {
				t.Fatalf("parseResourceAccessPath() error = %v", err)
			}
			if got.Operation != tt.wantOperation || got.ResourceType != tt.wantType || got.ResourceID != tt.wantID || got.GrantID != tt.wantGrantID {
				t.Fatalf("parseResourceAccessPath() = %#v", got)
			}
		})
	}
}

func TestNormalizeScopeGrantResourceID(t *testing.T) {
	id, lookup, display := normalizeScopeGrantResourceID("default")
	if id != "" || lookup != "" || display != "default" {
		t.Fatalf("normalizeScopeGrantResourceID(default) = (%q, %q, %q), want empty lookup with default display", id, lookup, display)
	}
	if !isDefaultScopeGrantResource(id, display) {
		t.Fatal("default scope should be recognized as a logical scope resource")
	}

	id, lookup, display = normalizeScopeGrantResourceID("/team-1/dev/")
	if id != "team-1/dev" || lookup != "team-1/dev" || display != "team-1/dev" {
		t.Fatalf("normalizeScopeGrantResourceID(team scope) = (%q, %q, %q)", id, lookup, display)
	}
	if isDefaultScopeGrantResource(id, display) {
		t.Fatal("non-default scope was recognized as default")
	}
}

func TestResolveDefaultScopeGrantResourceDoesNotRequireRows(t *testing.T) {
	resource, err := resolveAccessGrantResource(t.Context(), &noopQueryRunner{}, grantResourceScope, "default", true)
	if err != nil {
		t.Fatalf("resolveAccessGrantResource(default scope) error = %v", err)
	}
	if resource.Type != grantResourceScope || resource.ID != "" || resource.Display != "default" {
		t.Fatalf("default scope resource = %#v", resource)
	}
}

func TestListAccessAuthTeamsQueriesAuthTeamsOnly(t *testing.T) {
	runner := &accessAuthTeamsQueryRunner{
		rows: &accessAuthTeamRows{
			rows: []accessAuthTeamResponse{
				{ID: " team-b ", Name: " Beta "},
				{ID: "", Name: "missing-id"},
				{ID: "team-a", Name: ""},
				{ID: "team-c", Name: "Alpha"},
			},
		},
	}

	got, err := listAccessAuthTeams(context.Background(), runner)
	if err != nil {
		t.Fatalf("listAccessAuthTeams() error = %v", err)
	}
	if !strings.Contains(runner.query, "FROM auth_teams") {
		t.Fatalf("query = %q, want auth_teams source", runner.query)
	}
	want := []accessAuthTeamResponse{
		{ID: "team-b", Name: "Beta"},
		{ID: "team-c", Name: "Alpha"},
	}
	if len(got) != len(want) {
		t.Fatalf("listAccessAuthTeams() = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("listAccessAuthTeams() = %#v, want %#v", got, want)
		}
	}
}

func TestValidateResourceVisibilityPolicy(t *testing.T) {
	if err := validateResourceVisibilityPolicy(grantResourcePipeline, resourceVisibilityWorkspace); err != nil {
		t.Fatalf("pipeline workspace visibility error = %v", err)
	}
	if err := validateResourceVisibilityPolicy(grantResourceStep, resourceVisibilityWorkspace); err != nil {
		t.Fatalf("step workspace visibility error = %v", err)
	}
	for _, resourceType := range []string{grantResourceLLMProfile, grantResourceAgentProfile, grantResourceMCPServer, grantResourceMCPProfile} {
		t.Run(resourceType, func(t *testing.T) {
			if err := validateResourceVisibilityPolicy(resourceType, resourceVisibilityWorkspace); err != nil {
				t.Fatalf("validateResourceVisibilityPolicy(%q, workspace) error = %v", resourceType, err)
			}
		})
	}
	for _, resourceType := range []string{grantResourceScope, grantResourceSecret, grantResourceVariable, grantResourceRunner} {
		t.Run(resourceType, func(t *testing.T) {
			if err := validateResourceVisibilityPolicy(resourceType, resourceVisibilityWorkspace); err == nil {
				t.Fatalf("validateResourceVisibilityPolicy(%q, workspace) error = nil, want error", resourceType)
			}
			if err := validateResourceVisibilityPolicy(resourceType, resourceVisibilityRestricted); err != nil {
				t.Fatalf("validateResourceVisibilityPolicy(%q, restricted) error = %v", resourceType, err)
			}
		})
	}
}

func TestNormalizeUseGrantActions(t *testing.T) {
	actions, err := normalizeUseGrantActions(grantResourcePipeline, nil)
	if err != nil {
		t.Fatalf("normalizeUseGrantActions() error = %v", err)
	}
	if len(actions) != 1 || actions[0] != "pipeline.use" {
		t.Fatalf("normalizeUseGrantActions() = %#v, want pipeline.use", actions)
	}
	actions, err = normalizeUseGrantActions(grantResourceLLMProfile, nil)
	if err != nil {
		t.Fatalf("normalizeUseGrantActions(llm_profile) error = %v", err)
	}
	if len(actions) != 1 || actions[0] != "llm_profile.use" {
		t.Fatalf("normalizeUseGrantActions(llm_profile) = %#v, want llm_profile.use", actions)
	}

	if _, err := normalizeUseGrantActions(grantResourcePipeline, []string{"pipeline.execute"}); err == nil {
		t.Fatal("normalizeUseGrantActions() accepted non-use action")
	}
	if _, err := normalizeUseGrantActions(grantResourcePipeline, []string{"scope.use"}); err == nil {
		t.Fatal("normalizeUseGrantActions() accepted mismatched use action")
	}
	if _, err := normalizeUseGrantActions(grantResourceMCPProfile, []string{"llm_profile.use"}); err == nil {
		t.Fatal("normalizeUseGrantActions() accepted mismatched AI profile use action")
	}
}

func TestInheritedAccessParentTeamsForPipeline(t *testing.T) {
	got := inheritedAccessParentTeams(accessGrantResource{Type: grantResourcePipeline, ID: "team-1/dev/deploy"})
	want := []string{"team-1", "team-1/dev"}
	if len(got) != len(want) {
		t.Fatalf("parent teams = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("parent teams = %#v, want %#v", got, want)
		}
	}
}

func TestInheritedAccessParentTeamsForGlobalAIProfile(t *testing.T) {
	if got := inheritedAccessParentTeams(accessGrantResource{Type: grantResourceLLMProfile, ID: "hosted"}); len(got) != 0 {
		t.Fatalf("parent teams = %#v, want none for global profile name", got)
	}
	got := inheritedAccessParentTeams(accessGrantResource{Type: grantResourceMCPProfile, ID: "team-1/dev/github"})
	want := []string{"team-1", "team-1/dev"}
	if len(got) != len(want) {
		t.Fatalf("parent teams = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("parent teams = %#v, want %#v", got, want)
		}
	}
}

type accessAuthTeamsQueryRunner struct {
	rows  pgx.Rows
	query string
}

func (r *accessAuthTeamsQueryRunner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unexpected exec")
}

func (r *accessAuthTeamsQueryRunner) Query(_ context.Context, query string, _ ...any) (pgx.Rows, error) {
	r.query = query
	return r.rows, nil
}

func (r *accessAuthTeamsQueryRunner) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeScanRow{err: fmt.Errorf("unexpected query row")}
}

type accessAuthTeamRows struct {
	rows  []accessAuthTeamResponse
	index int
}

func (r *accessAuthTeamRows) Close() {}

func (r *accessAuthTeamRows) Err() error {
	return nil
}

func (r *accessAuthTeamRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (r *accessAuthTeamRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (r *accessAuthTeamRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}

func (r *accessAuthTeamRows) Scan(dest ...any) error {
	if len(dest) != 2 {
		return fmt.Errorf("expected two scan destinations, got %d", len(dest))
	}
	row := r.rows[r.index-1]
	*(dest[0].(*string)) = row.ID
	*(dest[1].(*string)) = row.Name
	return nil
}

func (r *accessAuthTeamRows) Values() ([]any, error) {
	return nil, nil
}

func (r *accessAuthTeamRows) RawValues() [][]byte {
	return nil
}

func (r *accessAuthTeamRows) Conn() *pgx.Conn {
	return nil
}
