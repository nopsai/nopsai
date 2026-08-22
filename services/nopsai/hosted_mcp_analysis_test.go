package nopsai

import (
	"testing"
	"time"
)

func TestAnalyzeTeamEvidenceRanksFindingsScoresAndNextStep(t *testing.T) {
	result := analyzeTeamEvidence(
		analysisSubject{Type: "team", ID: "7", Label: "Platform", Path: "/platform"},
		analysisWindow{From: time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC), Days: 30},
		analysisEvidenceSet{
			Sources: []string{"/v1/monitoring/summary?teamId=7"},
			Data: map[string]map[string]any{
				"summary": {
					"total_runs":              float64(40),
					"failed_runs":             float64(18),
					"failure_rate":            0.45,
					"median_duration_seconds": float64(120),
					"p95_duration_seconds":    float64(2100),
				},
				"reliability": {
					"failure_reasons": []any{
						map[string]any{"label": "step exited 1", "count": float64(9)},
					},
					"repeated_failure_pipelines": []any{
						map[string]any{"pipeline_path": "platform", "pipeline_name": "deploy-api", "total_runs": float64(12), "failed_runs": float64(9), "failure_rate": 0.75},
						map[string]any{"pipeline_path": "platform", "pipeline_name": "nightly", "total_runs": float64(8), "failed_runs": float64(5), "failure_rate": 0.63},
					},
					"recent_failures": []any{
						map[string]any{"run_id": "run-123", "pipeline_name": "deploy-api", "status": "failed"},
					},
				},
				"efficiency": {
					"total_ai_spend_usd": 4.0,
					"spend_by_pipeline": []any{
						map[string]any{"label": "deploy-api", "cost_usd": 3.4},
						map[string]any{"label": "nightly", "cost_usd": 0.6},
					},
					"costly_low_success_pipelines": []any{
						map[string]any{"pipeline_name": "deploy-api", "total_runs": float64(12), "failed_runs": float64(9), "failure_rate": 0.75},
					},
				},
				"pipeline_performance": {
					"items": []any{
						map[string]any{"pipeline_path": "platform", "pipeline_name": "deploy-api", "total_runs": float64(12), "failed_runs": float64(9), "failure_rate": 0.75},
						map[string]any{"pipeline_path": "platform", "pipeline_name": "docs", "total_runs": float64(20), "failed_runs": float64(0), "failure_rate": 0},
					},
				},
			},
		},
	)

	if result["ok"] != true {
		t.Fatalf("ok = %v, want true", result["ok"])
	}
	findings, _ := result["findings"].([]map[string]any)
	if len(findings) == 0 {
		t.Fatal("expected findings")
	}
	if severity := findings[0]["severity"]; severity != "critical" {
		t.Fatalf("first finding severity = %v, want critical (most severe first)", severity)
	}

	// The score follows the published formula, so the number can be re-derived
	// from the severity counts a reader can see.
	basis, _ := result["score_basis"].(map[string]any)
	counts, _ := result["severity_counts"].(map[string]int)
	expected := analysisScoreBaseline
	for severity, count := range counts {
		expected -= count * analysisSeverityWeights[severity]
	}
	if expected < 0 {
		expected = 0
	}
	if got := result["health_score"]; got != expected {
		t.Fatalf("health_score = %v, want %d (basis %v)", got, expected, basis)
	}

	if !analysisHasFindingTitled(findings, "45% of runs failed in the last 30 days") {
		t.Fatalf("expected a failure-rate finding, got %v", analysisFindingTitles(findings))
	}
	if !analysisHasFindingCategory(findings, "cost", "high") {
		t.Fatalf("expected a high cost finding for spend on failing pipelines, got %v", analysisFindingTitles(findings))
	}

	actions, _ := result["next_actions"].([]map[string]any)
	if len(actions) == 0 {
		t.Fatal("expected next actions")
	}
	if actions[0]["tool"] != "nopsai.analyze_pipeline" {
		t.Fatalf("first next action = %v, want nopsai.analyze_pipeline", actions[0]["tool"])
	}
	args, _ := actions[0]["args"].(map[string]any)
	if args["pipeline"] != "platform/deploy-api" {
		t.Fatalf("next action pipeline = %v, want platform/deploy-api", args["pipeline"])
	}
}

// Missing evidence must never read as a healthy subject: with no run data there
// is no score at all, and the reason is stated.
func TestAnalyzeTeamEvidenceWithoutRunEvidenceRefusesToScore(t *testing.T) {
	result := analyzeTeamEvidence(
		analysisSubject{Type: "team", ID: "7", Label: "Platform"},
		analysisWindow{Days: 30},
		analysisEvidenceSet{Limitations: []string{"summary could not be read: status 403"}},
	)

	if result["health_score"] != nil {
		t.Fatalf("health_score = %v, want nil when run evidence is unavailable", result["health_score"])
	}
	if result["ok"] != false {
		t.Fatalf("ok = %v, want false", result["ok"])
	}
	if _, hasBasis := result["score_basis"]; hasBasis {
		t.Fatal("score_basis must be absent when nothing was scored")
	}
	limitations, _ := result["limitations"].([]string)
	if !analysisContains(limitations, "summary could not be read: status 403") {
		t.Fatalf("limitations = %v, want the unreadable source recorded", limitations)
	}
}

func TestAnalyzeTeamEvidenceKeepsScoreButFlagsPartialEvidence(t *testing.T) {
	result := analyzeTeamEvidence(
		analysisSubject{Type: "team", ID: "7", Label: "Platform"},
		analysisWindow{Days: 30},
		analysisEvidenceSet{
			Limitations: []string{"security could not be read: status 403"},
			Data: map[string]map[string]any{
				"summary": {"total_runs": float64(10), "failed_runs": float64(0), "failure_rate": 0.0},
			},
		},
	)

	if result["health_score"] == nil {
		t.Fatal("health_score should still be produced when run evidence is readable")
	}
	if result["ok"] != false {
		t.Fatalf("ok = %v, want false while a source is unreadable", result["ok"])
	}
}

func TestAnalyzeTeamEvidenceReportsAnIdleWindow(t *testing.T) {
	result := analyzeTeamEvidence(
		analysisSubject{Type: "team", ID: "7", Label: "Platform"},
		analysisWindow{Days: 30},
		analysisEvidenceSet{Data: map[string]map[string]any{"summary": {"total_runs": float64(0)}}},
	)

	findings, _ := result["findings"].([]map[string]any)
	if len(findings) != 1 || findings[0]["title"] != "No runs in the last 30 days" {
		t.Fatalf("findings = %v, want a single idle-window finding", analysisFindingTitles(findings))
	}
}

func TestAnalyzePipelineEvidenceFlagsDominantAndFailingSteps(t *testing.T) {
	result := analyzePipelineEvidence(
		analysisSubject{Type: "pipeline", ID: "platform/deploy-api", Label: "deploy-api", Path: "platform"},
		analysisWindow{Days: 14},
		analysisEvidenceSet{
			Data: map[string]map[string]any{
				"summary": {"total_runs": float64(30), "failed_runs": float64(4), "failure_rate": 0.13},
				"step_performance": {
					"items": []any{
						map[string]any{"step_name": "build", "total_duration_seconds": float64(3000), "average_duration_seconds": float64(100), "p95_duration_seconds": float64(180), "total_runs": float64(30), "failed_runs": float64(0)},
						map[string]any{"step_name": "test", "total_duration_seconds": float64(600), "average_duration_seconds": float64(20), "total_runs": float64(30), "failed_runs": float64(12), "failure_rate": 0.4},
					},
				},
				"reliability": {
					"recent_failures": []any{map[string]any{"run_id": "run-9", "status": "failed"}},
				},
			},
		},
	)

	findings, _ := result["findings"].([]map[string]any)
	if !analysisHasFindingTitled(findings, "Step build dominates the runtime") {
		t.Fatalf("expected the dominant-step finding, got %v", analysisFindingTitles(findings))
	}
	if !analysisHasFindingTitled(findings, "1 step fails in at least a third of runs") {
		t.Fatalf("expected the failing-step finding, got %v", analysisFindingTitles(findings))
	}

	actions, _ := result["next_actions"].([]map[string]any)
	if len(actions) == 0 || actions[0]["tool"] != "nopsai.analyze_pipeline_run_failure" {
		t.Fatalf("next actions = %v, want the most recent failure first", actions)
	}
}

func TestAnalysisWindowFromArgsUsesBoundedDays(t *testing.T) {
	if window := analysisWindowFromArgs(nil); window.Days != analysisDefaultWindowDays {
		t.Fatalf("default days = %d, want %d", window.Days, analysisDefaultWindowDays)
	}
	if window := analysisWindowFromArgs(map[string]any{"days": float64(9000)}); window.Days > analysisMaxWindowDays {
		t.Fatalf("days = %d, want it clamped to %d", window.Days, analysisMaxWindowDays)
	}
	window := analysisWindowFromArgs(map[string]any{"days": float64(7)})
	if window.Days != 7 || window.To.Sub(window.From) < 6*24*time.Hour {
		t.Fatalf("window = %+v, want a 7 day span", window)
	}
}

func TestAnalysisTeamMatchingAcceptsIDPathAndName(t *testing.T) {
	team := map[string]any{"id": "7", "path": "/platform/core", "slug": "core", "name": "core", "display_name": "Platform Core"}
	for _, needle := range []string{"7", "platform/core", "core", "platform core"} {
		if !analysisTeamMatches(team, needle) {
			t.Fatalf("analysisTeamMatches(%q) = false, want true", needle)
		}
	}
	if analysisTeamMatches(team, "billing") {
		t.Fatal("analysisTeamMatches(\"billing\") = true, want false")
	}
}

func TestHostedMCPAnalysisToolsAreRegisteredAsReadOnly(t *testing.T) {
	wanted := map[string]string{
		"nopsai.list_teams":       "team.list",
		"nopsai.get_team":         "team.read",
		"nopsai.analyze_team":     "team.read",
		"nopsai.analyze_pipeline": "pipeline.read",
	}
	found := map[string]bool{}
	for _, tool := range allHostedMCPTools() {
		action, tracked := wanted[tool.Name]
		if !tracked {
			continue
		}
		found[tool.Name] = true
		if tool.Action != action {
			t.Fatalf("%s action = %q, want %q", tool.Name, tool.Action, action)
		}
		if assistantToolRequiresActionExecution(tool) {
			t.Fatalf("%s must not be treated as a mutating tool", tool.Name)
		}
	}
	for name := range wanted {
		if !found[name] {
			t.Fatalf("%s is not registered in the hosted MCP tool catalogue", name)
		}
	}
}

func analysisFindingTitles(findings []map[string]any) []string {
	titles := make([]string, 0, len(findings))
	for _, finding := range findings {
		title, _ := finding["title"].(string)
		titles = append(titles, title)
	}
	return titles
}

func analysisHasFindingTitled(findings []map[string]any, title string) bool {
	return analysisContains(analysisFindingTitles(findings), title)
}

func analysisHasFindingCategory(findings []map[string]any, category, severity string) bool {
	for _, finding := range findings {
		if finding["category"] == category && finding["severity"] == severity {
			return true
		}
	}
	return false
}

func analysisContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
