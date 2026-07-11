package nopsai

import "testing"

func TestPipelineNotificationFiltersMatch(t *testing.T) {
	ctx := pipelineNotificationContext{
		PipelineName: "deploy",
		PipelinePath: "team-1/services/api/deploy",
		RepoOwner:    "acme",
		RepoName:     "service-api",
		GitRef:       "refs/heads/release/2026-06",
	}
	filters := notificationRouteFiltersFile{
		Pipelines: notificationPatternFilter{Include: []string{"team-1/*"}},
		Repos:     notificationPatternFilter{Include: []string{"acme/service-api"}},
		Branches:  notificationPatternFilter{Include: []string{"release/*"}, Exclude: []string{"dependabot/*"}},
	}
	if !pipelineNotificationFiltersMatch(filters, ctx) {
		t.Fatal("pipelineNotificationFiltersMatch() = false, want true")
	}
	filters.Branches.Exclude = []string{"release/*"}
	if pipelineNotificationFiltersMatch(filters, ctx) {
		t.Fatal("pipelineNotificationFiltersMatch() = true for excluded branch, want false")
	}
}

func TestNotificationDeliveryDedupeKey(t *testing.T) {
	first := notificationDeliveryDedupeKey("run-1", "failure", "mail", "Release@Example.com")
	second := notificationDeliveryDedupeKey("run-1", "failure", "mail", "release@example.com")
	other := notificationDeliveryDedupeKey("run-1", "success", "mail", "release@example.com")
	if first != second {
		t.Fatal("dedupe key should normalize recipient case")
	}
	if first == other {
		t.Fatal("dedupe key should include event type")
	}
}

func TestNotificationRouteRulesMultiRouteMatching(t *testing.T) {
	definition, err := normalizeNotificationRouteDefinition(notificationRouteDefinitionFile{
		Routes: []notificationRouteRuleFile{
			{
				Name:   "failures",
				Events: map[string]bool{"failure": true},
			},
			{
				Name:   "success audit",
				Events: map[string]bool{"success": true},
			},
		},
	})
	if err != nil {
		t.Fatalf("normalizeNotificationRouteDefinition() error = %v", err)
	}
	var failureRoutes, successRoutes int
	for _, route := range notificationRouteRules(definition) {
		if route.Enabled && route.Events["failure"] {
			failureRoutes++
		}
		if route.Enabled && route.Events["success"] {
			successRoutes++
		}
	}
	if failureRoutes != 1 || successRoutes != 1 {
		t.Fatalf("failureRoutes=%d successRoutes=%d, want one route for each event", failureRoutes, successRoutes)
	}
}

func TestNotificationTeamLineageUsesNearestPolicyOrder(t *testing.T) {
	teamID := 2
	records := map[int]teamPathRecord{
		2: {ID: 2, Name: "team-1", Path: "team-1"},
		3: {ID: 3, Name: "test-app", ParentID: &teamID, Path: "team-1/test-app"},
	}

	teamPath, lineage, err := notificationTeamLineage(records, 3)
	if err != nil {
		t.Fatalf("notificationTeamLineage() error = %v", err)
	}
	if teamPath != "team-1/test-app" {
		t.Fatalf("teamPath = %q, want team-1/test-app", teamPath)
	}
	if len(lineage) != 2 || lineage[0] != 3 || lineage[1] != 2 {
		t.Fatalf("lineage = %#v, want child before parent", lineage)
	}
}

func TestNotificationTeamLineageRejectsInvalidHierarchy(t *testing.T) {
	t.Run("missing team", func(t *testing.T) {
		if _, _, err := notificationTeamLineage(nil, 42); err == nil {
			t.Fatal("notificationTeamLineage() error = nil, want missing team error")
		}
	})

	t.Run("cycle", func(t *testing.T) {
		firstID := 1
		secondID := 2
		records := map[int]teamPathRecord{
			1: {ID: 1, ParentID: &secondID, Path: "one"},
			2: {ID: 2, ParentID: &firstID, Path: "two"},
		}
		if _, _, err := notificationTeamLineage(records, 1); err == nil {
			t.Fatal("notificationTeamLineage() error = nil, want cycle error")
		}
	})
}
