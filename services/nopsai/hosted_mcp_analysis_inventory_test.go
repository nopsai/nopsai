package nopsai

import (
	"testing"
)

func analysisTestInventory() []analysisInventoryItem {
	return []analysisInventoryItem{
		{Kind: "pipeline", ID: "platform/deploy-api", Label: "deploy-api", TeamPath: "platform", Source: "git", Active: true},
		{Kind: "pipeline", ID: "platform/deploy_api", Label: "deploy_api", TeamPath: "platform", Source: "database", Active: true},
		{Kind: "schedule", ID: "s1", Label: "nightly-report", Description: "Deprecated nightly report", TeamPath: "platform", Source: "git", Active: false},
		{Kind: "trigger", ID: "t1", Label: "example/api", TeamPath: "", Source: "git", Active: true},
		{Kind: "pipeline", ID: "platform/core/team/edge/deep", Label: "deep", TeamPath: "platform/core/team/edge", Source: "git", Active: true},
	}
}

func TestAnalysisInventoryFindsDuplicatesAcrossNamingStyles(t *testing.T) {
	findings := analysisInventoryFindings(analysisTestInventory(), "platform")

	duplicate := analysisDefinitionFindingByTitle(findings, "2 pipeline resources share one name")
	if duplicate == nil {
		t.Fatalf("expected the duplicate finding, got %v", analysisDefinitionTitles(findings))
	}
	if duplicate.Category != "efficiency" || duplicate.Severity != "medium" {
		t.Fatalf("duplicate finding = %s/%s, want efficiency/medium", duplicate.Category, duplicate.Severity)
	}
}

// A duplicated credential is a different problem from a duplicated pipeline:
// picking the wrong one to rotate is an outage.
func TestAnalysisInventoryTreatsSensitiveDuplicatesAsSecurity(t *testing.T) {
	findings := analysisInventoryFindings([]analysisInventoryItem{
		{Kind: "credential", ID: "c1", Label: "registry-token", TeamPath: "platform", Source: "git", Active: true},
		{Kind: "credential", ID: "c2", Label: "registry_token", TeamPath: "platform", Source: "git", Active: true},
	}, "platform")

	duplicate := analysisDefinitionFindingByTitle(findings, "2 credential resources share one name")
	if duplicate == nil || duplicate.Category != "security" || duplicate.Severity != "high" {
		t.Fatalf("credential duplicate = %v, want a high security finding", duplicate)
	}
}

func TestAnalysisInventoryFlagsInheritedMixedSourceAndInactiveResources(t *testing.T) {
	titles := analysisDefinitionTitles(analysisInventoryFindings(analysisTestInventory(), "platform"))

	for _, want := range []string{
		"1 resource is used here but owned globally",
		"Resources are split between GitOps and the database",
		"1 resource is disabled, stale, or deprecated",
		"1 resource sit four or more levels deep",
	} {
		if !analysisContains(titles, want) {
			t.Fatalf("missing finding %q; got %v", want, titles)
		}
	}
}

// The inherited-resource rule is about a boundary, so it only applies when there
// is one: an unscoped analysis must not report every global resource.
func TestAnalysisInventorySkipsInheritedFindingWithoutATeamScope(t *testing.T) {
	titles := analysisDefinitionTitles(analysisInventoryFindings(analysisTestInventory(), ""))

	for _, title := range titles {
		if title == "1 resource is used here but owned globally" {
			t.Fatalf("inherited finding should not fire without a team scope: %v", titles)
		}
	}
}

func TestAnalysisInventorySpotsNearIdenticalReusableResources(t *testing.T) {
	findings := analysisInventoryFindings([]analysisInventoryItem{
		{Kind: "pipeline", ID: "a", Label: "build and publish api image", TeamPath: "platform", Source: "git", Active: true},
		{Kind: "pipeline", ID: "b", Label: "build and publish api image v2", TeamPath: "platform", Source: "git", Active: true},
		{Kind: "pipeline", ID: "c", Label: "run database migrations", TeamPath: "platform", Source: "git", Active: true},
	}, "platform")

	similar := analysisDefinitionFindingByTitle(findings, "2 resources look like copies of each other")
	if similar == nil || similar.Severity != "opportunity" {
		t.Fatalf("expected the similar-resources opportunity, got %v", analysisDefinitionTitles(findings))
	}
	for _, item := range similar.Evidence {
		if item.Label == "run database migrations" {
			t.Fatalf("an unrelated resource was grouped as a copy: %v", similar.Evidence)
		}
	}
}

func TestAnalysisInventoryReportsAnEmptyTeamAsAnOwnershipProblem(t *testing.T) {
	findings := analysisInventoryFindings(nil, "platform")

	if len(findings) != 1 || findings[0].Title != "No resources are attributed to this team" {
		t.Fatalf("findings = %v, want the ownership finding", analysisDefinitionTitles(findings))
	}
	if findings[0].Severity != "high" {
		t.Fatalf("severity = %q, want high", findings[0].Severity)
	}
}

func TestAnalysisInventoryOnlyKeepsResourcesInTheTeamsBranch(t *testing.T) {
	cases := []struct {
		item, team string
		keep       bool
	}{
		{"platform", "platform", true},
		{"platform/core", "platform", true},
		{"platform", "platform/core", true},
		{"payments", "platform", false},
		{"", "platform", true},
		{"payments", "", true},
	}
	for _, testCase := range cases {
		if got := analysisInventoryBelongsToTeam(testCase.item, testCase.team); got != testCase.keep {
			t.Fatalf("analysisInventoryBelongsToTeam(%q, %q) = %v, want %v", testCase.item, testCase.team, got, testCase.keep)
		}
	}
}

func TestAnalysisInventoryRowNormalizesEachResourceKind(t *testing.T) {
	pipeline := analysisInventoryRow("pipelines", map[string]any{"id": "platform/deploy-api", "source": "git"}, "platform")
	if pipeline["kind"] != "pipeline" || pipeline["label"] != "deploy-api" || pipeline["team_path"] != "platform" {
		t.Fatalf("pipeline row = %v", pipeline)
	}

	schedule := analysisInventoryRow("schedules", map[string]any{
		"identifier": "platform/nightly", "path": "platform", "enabled": false, "source": "database",
	}, "platform")
	if schedule["kind"] != "schedule" || schedule["active"] != false {
		t.Fatalf("schedule row = %v", schedule)
	}

	if other := analysisInventoryRow("pipelines", map[string]any{"id": "payments/deploy"}, "platform"); other != nil {
		t.Fatalf("another team's resource must not enter this analysis: %v", other)
	}
	if unknown := analysisInventoryRow("dashboards", map[string]any{"id": "d1"}, "platform"); unknown != nil {
		t.Fatalf("unknown kinds must be skipped: %v", unknown)
	}
}

func TestAnalysisResponseRowsReadsBareArraysAndWrappedLists(t *testing.T) {
	bare := analysisResponseRows(map[string]any{"response": []any{map[string]any{"id": "a"}}})
	if len(bare) != 1 || bare[0]["id"] != "a" {
		t.Fatalf("bare array rows = %v", bare)
	}
	wrapped := analysisResponseRows(map[string]any{"response": map[string]any{"triggers": []any{map[string]any{"id": "b"}}}})
	if len(wrapped) != 1 || wrapped[0]["id"] != "b" {
		t.Fatalf("wrapped rows = %v", wrapped)
	}
	if rows := analysisResponseRows(map[string]any{"response_text": "not json"}); rows != nil {
		t.Fatalf("unreadable payload rows = %v, want nil", rows)
	}
}

// Inventory evidence alone is enough to score a team that has never run anything.
func TestAnalyzeTeamEvidenceScoresFromInventoryAlone(t *testing.T) {
	items := make([]any, 0, 2)
	for _, item := range []map[string]any{
		{"kind": "credential", "id": "c1", "label": "registry-token", "team_path": "platform", "source": "git", "active": true},
		{"kind": "credential", "id": "c2", "label": "registry_token", "team_path": "platform", "source": "git", "active": true},
	} {
		items = append(items, item)
	}

	result := analyzeTeamEvidence(
		analysisSubject{Type: "team", ID: "7", Label: "Platform", Path: "platform"},
		analysisWindow{Days: 30},
		analysisEvidenceSet{Data: map[string]map[string]any{"inventory": {"items": items}}},
	)

	if result["health_score"] == nil {
		t.Fatal("inventory evidence should produce a score")
	}
	findings, _ := result["findings"].([]map[string]any)
	if !analysisContains(analysisFindingTitles(findings), "2 credential resources share one name") {
		t.Fatalf("inventory findings did not reach the result: %v", analysisFindingTitles(findings))
	}
}
