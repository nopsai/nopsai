package nopsai

import (
	"strings"
	"testing"
)

func analysisTestFailedRunEvidence() analysisEvidenceSet {
	return analysisEvidenceSet{
		Sources: []string{"/v1/runs/run-9"},
		Data: map[string]map[string]any{
			"detail": {
				"run_info": map[string]any{
					"run_id":          "run-9",
					"pipeline_name":   "deploy-api",
					"pipeline_path":   "platform",
					"status":          "failure",
					"is_complete":     true,
					"git_commit_sha":  "bbbbbbbbbbbbbbbb",
					"git_ref":         "refs/heads/main",
					"trigger_source":  "webhook",
					"pipeline_source": "database",
					"scope":           "prod",
				},
				"steps": []any{
					map[string]any{"name": "build", "status": "success", "tasks": []any{
						map[string]any{"task_name": "compile", "status": "success", "task_index": float64(0)},
					}},
					map[string]any{"name": "test", "status": "failure", "tasks": []any{
						map[string]any{"task_name": "unit", "status": "failure", "exit_code": float64(1), "task_index": float64(1)},
						map[string]any{"task_name": "lint", "status": "success", "task_index": float64(0)},
					}},
				},
				"child_runs":    []any{},
				"final_outputs": []any{},
			},
			"peers": {
				"runs": []any{
					map[string]any{
						"run_id": "run-8", "pipeline_name": "deploy-api", "pipeline_path": "platform",
						"status": "success", "is_complete": true, "created_at": "2026-08-20T10:00:00Z",
						"git_commit_sha": "aaaaaaaaaaaaaaaa", "git_ref": "refs/heads/main",
						"trigger_source": "webhook", "pipeline_source": "database", "scope": "prod",
					},
					map[string]any{"run_id": "run-9", "pipeline_name": "deploy-api", "pipeline_path": "platform", "status": "failure"},
				},
			},
			"logs": {
				"logs": []any{
					map[string]any{"line": "starting unit tests"},
					map[string]any{"line": "FAILED tests/user_spec.rb:42 assertion failed"},
				},
			},
		},
	}
}

func TestAnalyzeRunEvidenceNamesTheDomainAndFirstFailurePoint(t *testing.T) {
	result := analyzeRunEvidence(
		analysisSubject{Type: "run", ID: "run-9", Label: "platform/deploy-api"},
		analysisWindow{Days: 30},
		analysisTestFailedRunEvidence(),
	)

	diagnosis, _ := result["primary_diagnosis"].(map[string]any)
	if diagnosis == nil || diagnosis["domain"] != "Application tests" {
		t.Fatalf("primary_diagnosis = %v, want the application tests domain", diagnosis)
	}

	findings, _ := result["findings"].([]map[string]any)
	titles := analysisFindingTitles(findings)
	for _, want := range []string{
		"Likely failure domain: Application tests",
		"First failure point: test / unit",
		"1 input changed since the last successful run",
	} {
		if !analysisContains(titles, want) {
			t.Fatalf("missing finding %q; got %v", want, titles)
		}
	}

	// The comparison names what moved, which is where an operator should start.
	comparison := analysisTestFindingByTitle(findings, "1 input changed since the last successful run")
	evidence, _ := comparison["evidence"].([]map[string]any)
	if len(evidence) != 1 || evidence[0]["label"] != "Application commit" {
		t.Fatalf("comparison evidence = %v, want the changed commit", evidence)
	}
	if !strings.Contains(evidence[0]["value"].(string), "->") {
		t.Fatalf("comparison value = %v, want before -> after", evidence[0]["value"])
	}
}

func TestAnalyzeRunEvidenceOffersLogsPipelineAndComparisonAsNextSteps(t *testing.T) {
	result := analyzeRunEvidence(
		analysisSubject{Type: "run", ID: "run-9", Label: "platform/deploy-api"},
		analysisWindow{Days: 30},
		analysisTestFailedRunEvidence(),
	)

	actions, _ := result["next_actions"].([]map[string]any)
	tools := make([]string, 0, len(actions))
	for _, action := range actions {
		tools = append(tools, action["tool"].(string))
	}
	for _, want := range []string{"nopsai.get_pipeline_run_logs", "nopsai.analyze_pipeline", "nopsai.get_pipeline_run"} {
		if !analysisContains(tools, want) {
			t.Fatalf("next actions = %v, want %s", tools, want)
		}
	}
}

func TestAnalyzeRunEvidenceClassifiesCredentialFailuresWithoutEchoingSecrets(t *testing.T) {
	set := analysisTestFailedRunEvidence()
	set.Data["detail"]["run_info"].(map[string]any)["failure_reason"] = "registry push forbidden: credential registry-token is not granted for scope prod"
	set.Data["logs"] = map[string]any{"logs": []any{}}

	result := analyzeRunEvidence(analysisSubject{Type: "run", ID: "run-9"}, analysisWindow{Days: 30}, set)

	diagnosis, _ := result["primary_diagnosis"].(map[string]any)
	if diagnosis["domain"] != "Credential or authorization" {
		t.Fatalf("domain = %v, want Credential or authorization", diagnosis["domain"])
	}
	findings, _ := result["findings"].([]map[string]any)
	reason := analysisTestFindingByTitle(findings, "The run recorded a failure reason")
	if reason == nil || reason["category"] != "security" {
		t.Fatalf("failure reason finding = %v, want a security category", reason)
	}
	evidence, _ := reason["evidence"].([]map[string]any)
	if evidence[0]["kind"] != "redacted" {
		t.Fatalf("failure reason evidence kind = %v, want redacted", evidence[0]["kind"])
	}
}

func TestAnalyzeRunEvidenceReportsAHealthyRunWithoutInventingProblems(t *testing.T) {
	set := analysisTestFailedRunEvidence()
	runInfo := set.Data["detail"]["run_info"].(map[string]any)
	runInfo["status"] = "success"
	set.Data["detail"]["steps"] = []any{
		map[string]any{"name": "build", "status": "success", "tasks": []any{}},
	}
	set.Data["logs"] = map[string]any{"logs": []any{}}

	result := analyzeRunEvidence(analysisSubject{Type: "run", ID: "run-9"}, analysisWindow{Days: 30}, set)

	if _, hasDiagnosis := result["primary_diagnosis"]; hasDiagnosis {
		t.Fatal("a successful run must not carry a failure diagnosis")
	}
	findings, _ := result["findings"].([]map[string]any)
	if len(findings) != 1 || findings[0]["title"] != "No degradation signal in this run" {
		t.Fatalf("findings = %v, want only the no-signal note", analysisFindingTitles(findings))
	}
}

func TestAnalyzeRunEvidenceFlagsPendingApprovalsChildFailuresAndOutputErrors(t *testing.T) {
	set := analysisTestFailedRunEvidence()
	set.Data["approvals"] = map[string]any{"approvals": []any{
		map[string]any{"status": "pending", "step_name": "approve-prod", "approval_type": "production-deploy"},
		map[string]any{"status": "approved", "step_name": "approve-stage"},
	}}
	set.Data["detail"]["child_runs"] = []any{
		map[string]any{"pipeline_name": "migrate-db", "status": "failure", "run_id": "run-child", "is_complete": true},
	}
	set.Data["detail"]["final_outputs"] = []any{
		map[string]any{"name": "release-notes", "status": "failure", "error": "renderer contract violation"},
	}

	result := analyzeRunEvidence(analysisSubject{Type: "run", ID: "run-9"}, analysisWindow{Days: 30}, set)
	titles := analysisFindingTitles(result["findings"].([]map[string]any))

	for _, want := range []string{
		"1 approval is still pending",
		"1 child run failed",
		"1 final output failed to generate",
	} {
		if !analysisContains(titles, want) {
			t.Fatalf("missing finding %q; got %v", want, titles)
		}
	}
}

// Run evidence that could not be read must not produce a confident-looking score.
func TestAnalyzeRunEvidenceWithoutDetailRefusesToScore(t *testing.T) {
	result := analyzeRunEvidence(
		analysisSubject{Type: "run", ID: "run-9"},
		analysisWindow{Days: 30},
		analysisEvidenceSet{Limitations: []string{"detail could not be read: status 403"}},
	)

	if result["health_score"] != nil {
		t.Fatalf("health_score = %v, want nil", result["health_score"])
	}
	if result["ok"] != false {
		t.Fatalf("ok = %v, want false", result["ok"])
	}
}

func TestAnalysisRuntimeInputDigestNamesKeysWithoutValues(t *testing.T) {
	digest := analysisRuntimeInputDigest(map[string]any{
		"runtime_variable_overrides": map[string]any{"TARGET": "prod", "API_TOKEN": "sk-live-secret"},
	})

	if digest != "API_TOKEN, TARGET" {
		t.Fatalf("digest = %q, want sorted key names only", digest)
	}
	if strings.Contains(digest, "sk-live-secret") || strings.Contains(digest, "prod") {
		t.Fatalf("digest leaked an override value: %q", digest)
	}
}

func analysisTestFindingByTitle(findings []map[string]any, title string) map[string]any {
	for _, finding := range findings {
		if finding["title"] == title {
			return finding
		}
	}
	return nil
}
