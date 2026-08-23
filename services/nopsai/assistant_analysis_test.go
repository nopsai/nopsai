package nopsai

import (
	"strings"
	"testing"
)

// A planner that picks a real, permitted tool we forgot to ship a schema for is
// right; the turn used to die on our omission. It now gets the schema.

// Schema selection withholds proposal and mutation tools on purpose. Repair
// exists to fix our routing misses, not to hand back a guardrail.

func TestComposeAnalysisReplyRanksFindingsAndOffersTheNextStep(t *testing.T) {
	reply := composeAssistantReply(
		assistantTurnPlan{Intent: "llm_planned", Goal: "review the team"},
		"assistant",
		[]assistantToolActivity{{
			Name:   "nopsai.analyze_team",
			Status: assistantToolStatusSuccess,
			Output: analysisTestTeamOutput(),
		}},
	)

	for _, want := range []string{
		"Platform scores 60/100",
		"## Findings",
		"1. **45% of runs failed in the last 30 days** (critical · reliability)",
		"- Failure rate: 45%",
		"Recommended: Fix the pipelines that fail most first.",
		"## Next step",
		"`nopsai.analyze_pipeline`",
		"Data source: NopsAI monitoring evidence via `nopsai.analyze_team`",
		"deterministic, not model-derived",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("analysis reply missing %q:\n%s", want, reply)
		}
	}
}

func TestComposeAnalysisReplyLeadsWithTheRunDiagnosis(t *testing.T) {
	reply := composeAssistantReply(
		assistantTurnPlan{Intent: "llm_planned", Goal: "explain the failure"},
		"assistant",
		[]assistantToolActivity{{
			Name:   "nopsai.analyze_run",
			Status: assistantToolStatusSuccess,
			Output: analyzeRunEvidence(
				analysisSubject{Type: "run", ID: "run-9", Label: "platform/deploy-api"},
				analysisWindow{Days: 30},
				analysisTestFailedRunEvidence(),
			),
		}},
	)

	for _, want := range []string{
		"Run platform/deploy-api failed. Likely domain: Application tests",
		"First failure point: test / unit",
		"- Exit code: 1",
		"`nopsai.get_pipeline_run_logs`",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("run analysis reply missing %q:\n%s", want, reply)
		}
	}
}

func TestComposeAnalysisReplyReadsEvidenceStoredAsJSON(t *testing.T) {
	// A conversation reloaded from the database carries []any rather than
	// []map[string]any, and the same reply has to come out of both.
	output := analysisTestTeamOutput()
	output["findings"] = []any{
		map[string]any{"title": "45% of runs failed in the last 30 days", "severity": "critical", "category": "reliability"},
	}
	output["next_actions"] = []any{
		map[string]any{"label": "Analyse platform/deploy-api", "tool": "nopsai.analyze_pipeline"},
	}

	reply := composeAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_team",
		Status: assistantToolStatusSuccess,
		Output: output,
	}})

	if !strings.Contains(reply, "45% of runs failed") || !strings.Contains(reply, "nopsai.analyze_pipeline") {
		t.Fatalf("reply did not read JSON-shaped evidence:\n%s", reply)
	}
}

func TestComposeAnalysisReplyExplainsAnUnresolvedTeam(t *testing.T) {
	reply := composeAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_team",
		Status: assistantToolStatusSuccess,
		Output: map[string]any{
			"ok":    false,
			"error": "no visible team matches \"platfrom\"",
			"available_teams": []map[string]any{
				{"id": "7", "path": "/platform", "label": "Platform"},
			},
		},
	}})

	if !strings.Contains(reply, "no visible team matches") {
		t.Fatalf("reply should name the resolution failure:\n%s", reply)
	}
	if !strings.Contains(reply, "Platform /platform") {
		t.Fatalf("reply should list the teams the user can analyse instead:\n%s", reply)
	}
}

func TestComposeAnalysisReplyIsSkippedWithoutASuccessfulAnalysis(t *testing.T) {
	if reply := composeAnalysisReply([]assistantToolActivity{{
		Name:   "nopsai.analyze_team",
		Status: assistantToolStatusError,
		Output: analysisTestTeamOutput(),
	}}); reply != "" {
		t.Fatalf("failed analysis must not compose a reply, got:\n%s", reply)
	}
}

func analysisTestTeamOutput() map[string]any {
	return map[string]any{
		"analysis":     "team",
		"summary":      "Platform scores 60/100 over the last 30 days: 3 findings, 1 critical or high.",
		"health_score": 60,
		"window":       map[string]any{"from": "2026-07-22T00:00:00Z", "to": "2026-08-21T00:00:00Z", "days": 30},
		"findings": []map[string]any{
			{
				"title":    "45% of runs failed in the last 30 days",
				"severity": "critical",
				"category": "reliability",
				"summary":  "A failure rate at this level means the pipeline result is not trustworthy.",
				"evidence": []map[string]any{
					{"label": "Failure rate", "value": "45%", "kind": "metric"},
					{"label": "Runs", "value": "18 failed of 40", "kind": "metric"},
				},
				"recommendations": []map[string]any{
					{"title": "Fix the pipelines that fail most first", "detail": "Rank failures by pipeline rather than by run."},
				},
			},
		},
		"next_actions": []map[string]any{
			{"label": "Analyse platform/deploy-api, the least reliable pipeline in this window", "tool": "nopsai.analyze_pipeline", "args": map[string]any{"pipeline": "platform/deploy-api"}},
		},
		"limitations": []string{"Only evidence the current user is allowed to read contributes to the findings."},
	}
}
