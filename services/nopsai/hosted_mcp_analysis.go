package nopsai

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// The analysis contract is deliberately identical to the one the UI analysis
// engine publishes (services/ui/src/features/analysis/model.ts): the same
// baseline, the same severity weights, the same clamp. A health score that means
// one thing in the Analysis modal and another in chat is worse than no score.
const (
	analysisScoreBaseline     = 100
	analysisDefaultWindowDays = 30
	analysisMaxWindowDays     = 365
	analysisMaxEvidenceRows   = 3
)

var analysisSeverityWeights = map[string]int{
	"critical":    25,
	"high":        15,
	"medium":      8,
	"low":         3,
	"opportunity": 1,
}

var analysisSeverityOrder = []string{"critical", "high", "medium", "low", "opportunity"}

type analysisSubject struct {
	Type  string
	ID    string
	Label string
	Path  string
}

type analysisWindow struct {
	From time.Time
	To   time.Time
	Days int
}

type analysisEvidenceItem struct {
	Label string
	Value string
	// Kind separates measured facts from derived judgement so a reader can tell
	// which numbers came from NopsAI and which came from a rule.
	Kind string
}

type analysisRecommendation struct {
	Title  string
	Detail string
}

type analysisFinding struct {
	Category        string
	Severity        string
	Title           string
	Summary         string
	Evidence        []analysisEvidenceItem
	Recommendations []analysisRecommendation
	Confidence      float64
}

type analysisNextAction struct {
	Label string
	Tool  string
	Args  map[string]any
}

// analysisEvidenceSet is the permission-filtered material a rule set reads.
// Sources that could not be read stay recorded: an unreadable source is a
// limitation of the analysis, never a category that quietly scores well.
type analysisEvidenceSet struct {
	Data        map[string]map[string]any
	Sources     []string
	Limitations []string
}

func (set analysisEvidenceSet) section(key string) map[string]any {
	if set.Data == nil {
		return nil
	}
	return set.Data[key]
}

func (set analysisEvidenceSet) has(key string) bool {
	return len(set.section(key)) > 0
}

func analyzeTeamEvidence(subject analysisSubject, window analysisWindow, set analysisEvidenceSet) map[string]any {
	findings := []analysisFinding{}
	findings = append(findings, analysisRunHealthFindings(set, window)...)
	findings = append(findings, analysisReliabilityFindings(set)...)
	findings = append(findings, analysisEfficiencyFindings(set)...)
	findings = append(findings, analysisSecurityFindings(set)...)
	findings = append(findings, analysisTeamPipelineFindings(set)...)
	if set.has("inventory") {
		findings = append(findings, analysisInventoryFindings(analysisInventoryFromEvidence(set), subject.Path)...)
	}
	return analysisResult("team", subject, window, findings, analysisTeamNextActions(subject, set), set)
}

func analyzePipelineEvidence(subject analysisSubject, window analysisWindow, set analysisEvidenceSet) map[string]any {
	findings := []analysisFinding{}
	findings = append(findings, analysisRunHealthFindings(set, window)...)
	findings = append(findings, analysisReliabilityFindings(set)...)
	findings = append(findings, analysisEfficiencyFindings(set)...)
	findings = append(findings, analysisStepFindings(set)...)
	findings = append(findings, analysisPipelineDefinitionFindings(analysisString(set.section("definition"), "yaml"))...)
	return analysisResult("pipeline", subject, window, findings, analysisPipelineNextActions(subject, set), set)
}

func analysisResult(
	kind string,
	subject analysisSubject,
	window analysisWindow,
	findings []analysisFinding,
	nextActions []analysisNextAction,
	set analysisEvidenceSet,
) map[string]any {
	sortAnalysisFindings(findings)
	// Without evidence there is nothing to score. Reporting 100 because no problem
	// was observed would turn "we could not look" into "all is well". A pipeline
	// definition is evidence in its own right, so it scores even with no runs.
	scored := set.has("summary") || set.has("definition") || set.has("detail") || set.has("inventory")
	limitations := append([]string{}, set.Limitations...)
	if !scored {
		limitations = append(limitations, "No readable evidence was available for this subject, so no health score was calculated.")
	}
	limitations = append(limitations,
		"This is a deterministic reviewer score over the selected window, not an SLO or an uptime metric.",
		"Only evidence the current user is allowed to read contributes to the findings.",
	)

	result := map[string]any{
		"analysis": kind,
		"subject": map[string]any{
			"type":  subject.Type,
			"id":    subject.ID,
			"label": subject.Label,
			"path":  subject.Path,
		},
		"window": map[string]any{
			"from": window.From.UTC().Format(time.RFC3339),
			"to":   window.To.UTC().Format(time.RFC3339),
			"days": window.Days,
		},
		"finding_count":   len(findings),
		"severity_counts": analysisSeverityCounts(findings),
		"findings":        analysisFindingMaps(kind, subject, findings),
		"scores":          analysisCategoryScores(findings),
		"next_actions":    analysisNextActionMaps(nextActions),
		"limitations":     limitations,
		"data_sources":    set.Sources,
		"applied":         false,
		"ok":              scored && len(set.Limitations) == 0,
	}
	if scored {
		basis := analysisScoreBasis(findings)
		result["health_score"] = basis["score"]
		result["score_basis"] = basis
	} else {
		result["health_score"] = nil
	}
	result["summary"] = analysisHeadline(kind, subject, window, findings, scored, result["health_score"])
	return result
}

func analysisHeadline(
	kind string,
	subject analysisSubject,
	window analysisWindow,
	findings []analysisFinding,
	scored bool,
	score any,
) string {
	label := strings.TrimSpace(subject.Label)
	if label == "" {
		label = strings.TrimSpace(subject.ID)
	}
	if !scored {
		return fmt.Sprintf("No run evidence was readable for %s %s over the last %d days, so no score was produced.", kind, label, window.Days)
	}
	blocking := 0
	for _, finding := range findings {
		if finding.Severity == "critical" || finding.Severity == "high" {
			blocking++
		}
	}
	if len(findings) == 0 {
		return fmt.Sprintf("%s %s scores %v/100 over the last %d days with no findings.", analysisTitleCase(kind), label, score, window.Days)
	}
	return fmt.Sprintf(
		"%s %s scores %v/100 over the last %d days: %d finding%s, %d critical or high.",
		analysisTitleCase(kind), label, score, window.Days, len(findings), plural(len(findings)), blocking,
	)
}

func analysisRunHealthFindings(set analysisEvidenceSet, window analysisWindow) []analysisFinding {
	summary := set.section("summary")
	if len(summary) == 0 {
		return nil
	}
	totalRuns := analysisInt(summary, "total_runs")
	if totalRuns == 0 {
		return []analysisFinding{{
			Category:   "reliability",
			Severity:   "low",
			Title:      fmt.Sprintf("No runs in the last %d days", window.Days),
			Summary:    "Nothing executed in this window, so reliability, efficiency, and cost could not be evaluated from run history.",
			Evidence:   []analysisEvidenceItem{{Label: "Total runs", Value: "0", Kind: "metric"}},
			Confidence: 0.95,
			Recommendations: []analysisRecommendation{{
				Title:  "Widen the window or check triggers",
				Detail: "Re-run this analysis with a longer window, or check that schedules and triggers for this subject are enabled.",
			}},
		}}
	}

	findings := []analysisFinding{}
	failed := analysisInt(summary, "failed_runs")
	failureRate := analysisFloat(summary, "failure_rate")
	if failureRate <= 0 && totalRuns > 0 {
		failureRate = float64(failed) / float64(totalRuns)
	}
	if severity := analysisFailureRateSeverity(failureRate); severity != "" {
		evidence := []analysisEvidenceItem{
			{Label: "Failure rate", Value: analysisPercent(failureRate), Kind: "metric"},
			{Label: "Runs", Value: fmt.Sprintf("%d failed of %d", failed, totalRuns), Kind: "metric"},
		}
		if reason := analysisTopRowLabel(set.section("reliability"), "failure_reasons"); reason != "" {
			evidence = append(evidence, analysisEvidenceItem{Label: "Most common failure reason", Value: reason, Kind: "fact"})
		}
		findings = append(findings, analysisFinding{
			Category: "reliability",
			Severity: severity,
			Title:    fmt.Sprintf("%s of runs failed in the last %d days", analysisPercent(failureRate), window.Days),
			Summary:  "A failure rate at this level means the pipeline result is not trustworthy without a human checking every run.",
			Evidence: evidence,
			Recommendations: []analysisRecommendation{{
				Title:  "Fix the pipelines that fail most first",
				Detail: "Rank failures by pipeline rather than by run, then read the logs of the most recent failure of the worst one.",
			}},
			Confidence: 0.9,
		})
	}

	median := analysisFloat(summary, "median_duration_seconds")
	p95 := analysisFloat(summary, "p95_duration_seconds")
	if median > 0 && p95 >= 3*median && p95 >= 300 {
		findings = append(findings, analysisFinding{
			Category: "efficiency",
			Severity: "medium",
			Title:    "Run duration has a long tail",
			Summary:  "The slowest runs take several times longer than a typical run, which usually points at queueing, retries, or one dominating step.",
			Evidence: []analysisEvidenceItem{
				{Label: "Median duration", Value: analysisDuration(median), Kind: "metric"},
				{Label: "P95 duration", Value: analysisDuration(p95), Kind: "metric"},
				{Label: "Ratio", Value: fmt.Sprintf("p95 is %.1fx the median", p95/median), Kind: "inference"},
			},
			Recommendations: []analysisRecommendation{{
				Title:  "Compare step timings on the slowest runs",
				Detail: "Read step-level performance for this window and check whether one step, queue time, or a retry loop explains the tail.",
			}},
			Confidence: 0.75,
		})
	}
	if p95 >= 1800 {
		findings = append(findings, analysisFinding{
			Category: "efficiency",
			Severity: "medium",
			Title:    "Slow runs take over 30 minutes",
			Summary:  "Long feedback loops delay every change that depends on this result.",
			Evidence: []analysisEvidenceItem{
				{Label: "P95 duration", Value: analysisDuration(p95), Kind: "metric"},
				{Label: "Average duration", Value: analysisDuration(analysisFloat(summary, "average_duration_seconds")), Kind: "metric"},
			},
			Recommendations: []analysisRecommendation{{
				Title:  "Parallelise independent steps",
				Detail: "Steps without a depends_on relationship can run together; caching dependency installs usually removes the largest single block.",
			}},
			Confidence: 0.7,
		})
	}

	if queued := analysisInt(summary, "queued_jobs"); queued >= 10 {
		findings = append(findings, analysisFinding{
			Category: "efficiency",
			Severity: "medium",
			Title:    fmt.Sprintf("%d jobs are waiting for a runner", queued),
			Summary:  "Queued work means wall-clock time is spent waiting for capacity rather than doing anything.",
			Evidence: []analysisEvidenceItem{
				{Label: "Queued jobs", Value: fmt.Sprintf("%d", queued), Kind: "metric"},
				{Label: "Active runners", Value: fmt.Sprintf("%d", analysisInt(summary, "active_runners")), Kind: "metric"},
			},
			Recommendations: []analysisRecommendation{{
				Title:  "Add runner capacity or reduce concurrency demand",
				Detail: "Check dispatcher status for the pools in use before adding runners; a stuck pool looks the same as a busy one from the queue.",
			}},
			Confidence: 0.8,
		})
	}

	if failures := analysisInt(summary, "notification_failures"); failures > 0 {
		findings = append(findings, analysisFinding{
			Category: "monitoring",
			Severity: "medium",
			Title:    "Notifications are failing to deliver",
			Summary:  "Failed notifications mean nobody is being told when a run fails, so real failures can sit unnoticed.",
			Evidence: []analysisEvidenceItem{
				{Label: "Failed notifications", Value: fmt.Sprintf("%d", failures), Kind: "metric"},
			},
			Recommendations: []analysisRecommendation{{
				Title:  "Check the notification route and mail settings",
				Detail: "Verify the team notification route target and send a test message before relying on alerts again.",
			}},
			Confidence: 0.85,
		})
	}
	return findings
}

func analysisReliabilityFindings(set analysisEvidenceSet) []analysisFinding {
	reliability := set.section("reliability")
	if len(reliability) == 0 {
		return nil
	}
	findings := []analysisFinding{}

	if rows := analysisRows(reliability, "repeated_failure_pipelines"); len(rows) > 0 {
		severity := "medium"
		if len(rows) >= 3 {
			severity = "high"
		}
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   severity,
			Title:      fmt.Sprintf("%d pipeline%s fail%s repeatedly", len(rows), plural(len(rows)), verbS(len(rows))),
			Summary:    "Repeated failure of the same pipeline is a standing defect, not bad luck; each run costs time and produces no result.",
			Evidence:   analysisPerformanceEvidence(rows),
			Confidence: 0.9,
			Recommendations: []analysisRecommendation{{
				Title:  "Analyse the worst pipeline first",
				Detail: "Run a pipeline analysis on the top entry, then read the logs of its most recent failure.",
			}},
		})
	}
	if rows := analysisRows(reliability, "flaky_pipelines"); len(rows) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "medium",
			Title:      fmt.Sprintf("%d pipeline%s alternate%s between pass and fail", len(rows), plural(len(rows)), verbS(len(rows))),
			Summary:    "Flaky results train people to re-run instead of investigating, which hides real regressions.",
			Evidence:   analysisPerformanceEvidence(rows),
			Confidence: 0.8,
			Recommendations: []analysisRecommendation{{
				Title:  "Separate flaky steps from real failures",
				Detail: "Compare failing and passing runs of the same commit; a step that differs across identical inputs is the flake.",
			}},
		})
	}
	if rows := analysisRows(reliability, "stuck_runs"); len(rows) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "high",
			Title:      fmt.Sprintf("%d run%s %s stuck", len(rows), plural(len(rows)), isAre(len(rows))),
			Summary:    "A stuck run holds capacity and never reports a result, so it blocks both the queue and whoever is waiting on it.",
			Evidence:   analysisRunEvidence(rows),
			Confidence: 0.85,
			Recommendations: []analysisRecommendation{{
				Title:  "Cancel and re-dispatch the stuck runs",
				Detail: "Check dispatcher and runner health first; a run stuck without a runner is a capacity problem, not a pipeline problem.",
			}},
		})
	}
	if rows := analysisRows(reliability, "approvals_waiting_too_long"); len(rows) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "organization",
			Severity:   "medium",
			Title:      fmt.Sprintf("%d approval%s %s been waiting too long", len(rows), plural(len(rows)), hasHave(len(rows))),
			Summary:    "Delivery is blocked on people, not on the platform. Long approval waits usually mean the approver group is wrong or unaware.",
			Evidence:   analysisNamedCountEvidence(rows),
			Confidence: 0.85,
			Recommendations: []analysisRecommendation{{
				Title:  "Check the approver group and notification route",
				Detail: "Confirm the approval step targets a team that is staffed and notified, and that self-approval policy matches intent.",
			}},
		})
	}
	return findings
}

func analysisEfficiencyFindings(set analysisEvidenceSet) []analysisFinding {
	efficiency := set.section("efficiency")
	if len(efficiency) == 0 {
		return nil
	}
	findings := []analysisFinding{}

	if rows := analysisRows(efficiency, "costly_low_success_pipelines"); len(rows) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "cost",
			Severity:   "high",
			Title:      fmt.Sprintf("%d expensive pipeline%s mostly fail%s", len(rows), plural(len(rows)), verbS(len(rows))),
			Summary:    "Spend on a pipeline that rarely succeeds buys nothing. This is usually the cheapest saving available.",
			Evidence:   analysisPerformanceEvidence(rows),
			Confidence: 0.85,
			Recommendations: []analysisRecommendation{{
				Title:  "Fix or disable before optimising anything else",
				Detail: "Either repair the failure or stop scheduling the pipeline until it is repaired; a failing schedule keeps spending.",
			}},
		})
	}
	if rows := analysisRows(efficiency, "frequent_reruns"); len(rows) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "efficiency",
			Severity:   "medium",
			Title:      fmt.Sprintf("%d pipeline%s %s re-run frequently", len(rows), plural(len(rows)), isAre(len(rows))),
			Summary:    "Frequent re-runs are a human working around an unreliable result, and they double the cost of every affected change.",
			Evidence:   analysisPerformanceEvidence(rows),
			Confidence: 0.75,
			Recommendations: []analysisRecommendation{{
				Title:  "Treat re-run rate as a defect signal",
				Detail: "Investigate why the first attempt is not trusted; retries hide the failure rather than removing it.",
			}},
		})
	}

	spend := analysisFloat(efficiency, "total_ai_spend_usd")
	byPipeline := analysisRows(efficiency, "spend_by_pipeline")
	if spend > 0 && len(byPipeline) > 1 {
		top := analysisFloat(byPipeline[0], "cost_usd")
		if share := top / spend; share >= 0.6 {
			findings = append(findings, analysisFinding{
				Category: "cost",
				Severity: "opportunity",
				Title:    "AI spend is concentrated in one pipeline",
				Summary:  "One pipeline accounts for most of the AI spend in this window, so it is the only place where a model or prompt change moves the bill.",
				Evidence: []analysisEvidenceItem{
					{Label: "Total AI spend", Value: analysisMoney(spend), Kind: "metric"},
					{Label: analysisRowLabel(byPipeline[0]), Value: fmt.Sprintf("%s (%s of spend)", analysisMoney(top), analysisPercent(share)), Kind: "metric"},
				},
				Confidence: 0.8,
				Recommendations: []analysisRecommendation{{
					Title:  "Review the model profile on that pipeline",
					Detail: "Check whether the steps there need the largest model, and whether repeated context can be cached or trimmed.",
				}},
			})
		}
	}

	for idx, recommendation := range analysisStrings(efficiency, "recommendations") {
		if idx >= analysisMaxEvidenceRows {
			break
		}
		findings = append(findings, analysisFinding{
			Category:   "efficiency",
			Severity:   "opportunity",
			Title:      recommendation,
			Summary:    "Raised by NopsAI monitoring for this window.",
			Evidence:   []analysisEvidenceItem{{Label: "Source", Value: "Monitoring efficiency recommendations", Kind: "fact"}},
			Confidence: 0.6,
		})
	}
	return findings
}

func analysisSecurityFindings(set analysisEvidenceSet) []analysisFinding {
	security := set.section("security")
	if len(security) == 0 {
		return nil
	}
	rows := analysisRows(security, "high_risk_failed_pipelines")
	if len(rows) == 0 {
		return nil
	}
	return []analysisFinding{{
		Category:   "security",
		Severity:   "high",
		Title:      fmt.Sprintf("%d high-risk pipeline%s failed in this window", len(rows), plural(len(rows))),
		Summary:    "Failures in pipelines that carry elevated access are worth reading before ordinary failures: a partial run can leave credentials or state half-applied.",
		Evidence:   analysisPerformanceEvidence(rows),
		Confidence: 0.8,
		Recommendations: []analysisRecommendation{{
			Title:  "Read these failures before the rest",
			Detail: "Confirm no partial change was applied, then check whether the failing step needed that level of access at all.",
		}},
	}}
}

func analysisTeamPipelineFindings(set analysisEvidenceSet) []analysisFinding {
	rows := analysisRows(set.section("pipeline_performance"), "items")
	worst := analysisWorstPipelineRow(rows)
	if worst == nil {
		return nil
	}
	failureRate := analysisFloat(worst, "failure_rate")
	if failureRate < 0.5 {
		return nil
	}
	return []analysisFinding{{
		Category: "reliability",
		Severity: "high",
		Title:    fmt.Sprintf("%s fails more often than it succeeds", analysisRowLabel(worst)),
		Summary:  "A pipeline below a coin flip is not a delivery path; everything downstream of it is effectively manual.",
		Evidence: analysisPerformanceEvidence([]map[string]any{worst}),
		Recommendations: []analysisRecommendation{{
			Title:  "Analyse this pipeline on its own",
			Detail: "A pipeline-level analysis separates a broken step from a broken environment.",
		}},
		Confidence: 0.9,
	}}
}

func analysisStepFindings(set analysisEvidenceSet) []analysisFinding {
	rows := analysisRows(set.section("step_performance"), "items")
	if len(rows) == 0 {
		return nil
	}
	findings := []analysisFinding{}

	total := 0.0
	for _, row := range rows {
		total += analysisFloat(row, "total_duration_seconds")
	}
	if total > 0 {
		slowest := rows[0]
		for _, row := range rows {
			if analysisFloat(row, "total_duration_seconds") > analysisFloat(slowest, "total_duration_seconds") {
				slowest = row
			}
		}
		if share := analysisFloat(slowest, "total_duration_seconds") / total; share >= 0.5 {
			findings = append(findings, analysisFinding{
				Category: "efficiency",
				Severity: "medium",
				Title:    fmt.Sprintf("Step %s dominates the runtime", analysisRowLabel(slowest)),
				Summary:  "One step accounts for most of the pipeline's execution time, so it is the only step whose optimisation changes the total.",
				Evidence: []analysisEvidenceItem{
					{Label: "Share of runtime", Value: analysisPercent(share), Kind: "inference"},
					{Label: "Average duration", Value: analysisDuration(analysisFloat(slowest, "average_duration_seconds")), Kind: "metric"},
					{Label: "P95 duration", Value: analysisDuration(analysisFloat(slowest, "p95_duration_seconds")), Kind: "metric"},
				},
				Confidence: 0.85,
				Recommendations: []analysisRecommendation{{
					Title:  "Split, cache, or parallelise this step",
					Detail: "Check whether the step depends on every prior step; if not, it can start earlier or run alongside them.",
				}},
			})
		}
	}

	failing := []map[string]any{}
	for _, row := range rows {
		if analysisFloat(row, "failure_rate") >= 0.3 && analysisInt(row, "total_runs") >= 3 {
			failing = append(failing, row)
		}
	}
	if len(failing) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "high",
			Title:      fmt.Sprintf("%d step%s fail%s in at least a third of runs", len(failing), plural(len(failing)), verbS(len(failing))),
			Summary:    "A step failing this often is the pipeline's actual failure mode, whatever the run-level reason says.",
			Evidence:   analysisPerformanceEvidence(failing),
			Confidence: 0.85,
			Recommendations: []analysisRecommendation{{
				Title:  "Read the logs of the failing step, not the run",
				Detail: "Run-level failure reasons usually restate the step exit code; the step's own output carries the cause.",
			}},
		})
	}
	return findings
}

func analysisTeamNextActions(subject analysisSubject, set analysisEvidenceSet) []analysisNextAction {
	actions := []analysisNextAction{}
	if worst := analysisWorstPipelineRow(analysisRows(set.section("pipeline_performance"), "items")); worst != nil {
		name := analysisPipelineIdentity(worst)
		actions = append(actions, analysisNextAction{
			Label: fmt.Sprintf("Analyse %s, the least reliable pipeline in this window", name),
			Tool:  "nopsai.analyze_pipeline",
			Args:  map[string]any{"pipeline": name, "days": analysisDefaultWindowDays},
		})
	}
	if run := analysisFirstRunID(set.section("reliability"), "recent_failures"); run != "" {
		actions = append(actions, analysisNextAction{
			Label: "Read the most recent failure in this window",
			Tool:  "nopsai.analyze_pipeline_run_failure",
			Args:  map[string]any{"run_id": run},
		})
	}
	if subject.Path != "" {
		actions = append(actions, analysisNextAction{
			Label: "Review AI spend for this team",
			Tool:  "nopsai.get_monitoring_ai_usage",
			Args:  map[string]any{"team_path": subject.Path},
		})
	}
	return actions
}

func analysisPipelineNextActions(subject analysisSubject, set analysisEvidenceSet) []analysisNextAction {
	actions := []analysisNextAction{}
	if run := analysisFirstRunID(set.section("reliability"), "recent_failures"); run != "" {
		actions = append(actions, analysisNextAction{
			Label: "Read the most recent failure of this pipeline",
			Tool:  "nopsai.analyze_pipeline_run_failure",
			Args:  map[string]any{"run_id": run},
		})
	}
	actions = append(actions,
		analysisNextAction{
			Label: "Read the pipeline definition",
			Tool:  "nopsai.get_pipeline",
			Args:  map[string]any{"pipeline": subject.ID},
		},
		analysisNextAction{
			Label: "Compare step timings for this pipeline",
			Tool:  "nopsai.get_monitoring_step_performance",
			Args:  map[string]any{"pipeline": subject.ID},
		},
	)
	return actions
}

func analysisFailureRateSeverity(rate float64) string {
	switch {
	case rate >= 0.4:
		return "critical"
	case rate >= 0.2:
		return "high"
	case rate >= 0.1:
		return "medium"
	default:
		return ""
	}
}

func analysisScoreBasis(findings []analysisFinding) map[string]any {
	counts := analysisSeverityCounts(findings)
	deduction := 0
	for severity, weight := range analysisSeverityWeights {
		deduction += counts[severity] * weight
	}
	weights := map[string]any{}
	for severity, weight := range analysisSeverityWeights {
		weights[severity] = weight
	}
	return map[string]any{
		"baseline":         analysisScoreBaseline,
		"formula":          "Starts at 100; subtracts critical x 25, high x 15, medium x 8, low x 3, opportunity x 1; clamps between 0 and 100.",
		"severity_weights": weights,
		"severity_counts":  counts,
		"finding_count":    len(findings),
		"total_deduction":  deduction,
		"score":            clampAnalysisScore(analysisScoreBaseline - deduction),
	}
}

func analysisCategoryScores(findings []analysisFinding) []map[string]any {
	byCategory := map[string][]analysisFinding{}
	for _, finding := range findings {
		byCategory[finding.Category] = append(byCategory[finding.Category], finding)
	}
	categories := make([]string, 0, len(byCategory))
	for category := range byCategory {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	scores := make([]map[string]any, 0, len(categories))
	for _, category := range categories {
		group := byCategory[category]
		deduction := 0
		for _, finding := range group {
			deduction += analysisSeverityWeights[finding.Severity]
		}
		scores = append(scores, map[string]any{
			"category":      category,
			"score":         clampAnalysisScore(analysisScoreBaseline - deduction),
			"finding_count": len(group),
			"deduction":     deduction,
			"basis":         fmt.Sprintf("%s starts at 100 and subtracts %d point%s from %d finding%s.", analysisTitleCase(category), deduction, plural(deduction), len(group), plural(len(group))),
		})
	}
	return scores
}

func analysisSeverityCounts(findings []analysisFinding) map[string]int {
	counts := map[string]int{}
	for _, severity := range analysisSeverityOrder {
		counts[severity] = 0
	}
	for _, finding := range findings {
		counts[finding.Severity]++
	}
	return counts
}

func analysisFindingMaps(kind string, subject analysisSubject, findings []analysisFinding) []map[string]any {
	items := make([]map[string]any, 0, len(findings))
	for index, finding := range findings {
		evidence := make([]map[string]any, 0, len(finding.Evidence))
		for _, item := range finding.Evidence {
			evidence = append(evidence, map[string]any{"label": item.Label, "value": item.Value, "kind": item.Kind})
		}
		recommendations := make([]map[string]any, 0, len(finding.Recommendations))
		for _, item := range finding.Recommendations {
			recommendations = append(recommendations, map[string]any{"title": item.Title, "detail": item.Detail})
		}
		items = append(items, map[string]any{
			"id":              analysisFindingID(kind, subject, finding, index),
			"category":        finding.Category,
			"severity":        finding.Severity,
			"title":           finding.Title,
			"summary":         finding.Summary,
			"evidence":        evidence,
			"recommendations": recommendations,
			"confidence":      finding.Confidence,
		})
	}
	return items
}

func analysisNextActionMaps(actions []analysisNextAction) []map[string]any {
	items := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		args := action.Args
		if args == nil {
			args = map[string]any{}
		}
		items = append(items, map[string]any{"label": action.Label, "tool": action.Tool, "args": args})
	}
	return items
}

func analysisFindingID(kind string, subject analysisSubject, finding analysisFinding, index int) string {
	parts := []string{kind, subject.ID, finding.Category, finding.Severity, fmt.Sprintf("%d", index+1)}
	return analysisSlug(strings.Join(parts, "-"))
}

func sortAnalysisFindings(findings []analysisFinding) {
	rank := map[string]int{}
	for index, severity := range analysisSeverityOrder {
		rank[severity] = index
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if rank[findings[i].Severity] != rank[findings[j].Severity] {
			return rank[findings[i].Severity] < rank[findings[j].Severity]
		}
		return findings[i].Title < findings[j].Title
	})
}

func analysisPerformanceEvidence(rows []map[string]any) []analysisEvidenceItem {
	items := make([]analysisEvidenceItem, 0, analysisMaxEvidenceRows)
	for index, row := range rows {
		if index >= analysisMaxEvidenceRows {
			break
		}
		total := analysisInt(row, "total_runs")
		failed := analysisInt(row, "failed_runs")
		value := fmt.Sprintf("%d failed of %d runs", failed, total)
		if rate := analysisFloat(row, "failure_rate"); rate > 0 {
			value += fmt.Sprintf(" (%s)", analysisPercent(rate))
		}
		if total == 0 && failed == 0 {
			value = analysisDuration(analysisFloat(row, "average_duration_seconds"))
		}
		items = append(items, analysisEvidenceItem{Label: analysisRowLabel(row), Value: value, Kind: "metric"})
	}
	return items
}

func analysisRunEvidence(rows []map[string]any) []analysisEvidenceItem {
	items := make([]analysisEvidenceItem, 0, analysisMaxEvidenceRows)
	for index, row := range rows {
		if index >= analysisMaxEvidenceRows {
			break
		}
		value := analysisString(row, "status")
		if duration := analysisFloat(row, "duration_seconds"); duration > 0 {
			value = strings.TrimSpace(value + " for " + analysisDuration(duration))
		}
		label := analysisRowLabel(row)
		if runID := analysisString(row, "run_id"); runID != "" {
			label = strings.TrimSpace(label + " " + runID)
		}
		items = append(items, analysisEvidenceItem{Label: label, Value: value, Kind: "fact"})
	}
	return items
}

func analysisNamedCountEvidence(rows []map[string]any) []analysisEvidenceItem {
	items := make([]analysisEvidenceItem, 0, analysisMaxEvidenceRows)
	for index, row := range rows {
		if index >= analysisMaxEvidenceRows {
			break
		}
		value := fmt.Sprintf("%d", analysisInt(row, "count"))
		if seconds := analysisFloat(row, "seconds"); seconds > 0 {
			value = analysisDuration(seconds)
		}
		items = append(items, analysisEvidenceItem{Label: analysisRowLabel(row), Value: value, Kind: "metric"})
	}
	return items
}

// analysisPipelineIdentity rebuilds the pipeline id other tools accept, so a
// next action can be executed as given instead of being re-resolved by hand.
func analysisPipelineIdentity(row map[string]any) string {
	name := analysisString(row, "pipeline_name")
	if name == "" {
		return analysisRowLabel(row)
	}
	if path := strings.Trim(analysisString(row, "pipeline_path"), "/"); path != "" {
		return path + "/" + name
	}
	return name
}

func analysisWorstPipelineRow(rows []map[string]any) map[string]any {
	var worst map[string]any
	for _, row := range rows {
		if analysisInt(row, "total_runs") < 3 {
			continue
		}
		if worst == nil || analysisFloat(row, "failure_rate") > analysisFloat(worst, "failure_rate") {
			worst = row
		}
	}
	if worst == nil || analysisFloat(worst, "failure_rate") <= 0 {
		return nil
	}
	return worst
}

func analysisFirstRunID(section map[string]any, key string) string {
	for _, row := range analysisRows(section, key) {
		if runID := analysisString(row, "run_id"); runID != "" {
			return runID
		}
	}
	return ""
}

func analysisTopRowLabel(section map[string]any, key string) string {
	rows := analysisRows(section, key)
	if len(rows) == 0 {
		return ""
	}
	label := analysisRowLabel(rows[0])
	if count := analysisInt(rows[0], "count"); count > 0 {
		return fmt.Sprintf("%s (%d)", label, count)
	}
	return label
}

func analysisRowLabel(row map[string]any) string {
	for _, key := range []string{"label", "step_name", "task_name", "pipeline_name", "pipeline_path", "name", "key"} {
		if value := analysisString(row, key); value != "" {
			return value
		}
	}
	return "unnamed"
}

func analysisRows(section map[string]any, key string) []map[string]any {
	if len(section) == 0 {
		return nil
	}
	raw, ok := section[key].([]any)
	if !ok {
		return nil
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if row, ok := item.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func analysisStrings(section map[string]any, key string) []string {
	if len(section) == 0 {
		return nil
	}
	raw, ok := section[key].([]any)
	if !ok {
		return nil
	}
	values := make([]string, 0, len(raw))
	for _, item := range raw {
		if value, ok := item.(string); ok && strings.TrimSpace(value) != "" {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return values
}

func analysisFloat(section map[string]any, key string) float64 {
	if len(section) == 0 {
		return 0
	}
	switch value := section[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func analysisInt(section map[string]any, key string) int {
	return int(math.Round(analysisFloat(section, key)))
}

func analysisString(section map[string]any, key string) string {
	if len(section) == 0 {
		return ""
	}
	value, _ := section[key].(string)
	return strings.TrimSpace(value)
}

func analysisPercent(value float64) string {
	return fmt.Sprintf("%.0f%%", value*100)
}

func analysisMoney(value float64) string {
	if value > 0 && value < 0.01 {
		return fmt.Sprintf("$%.4f", value)
	}
	return fmt.Sprintf("$%.2f", value)
}

func analysisDuration(seconds float64) string {
	if seconds <= 0 {
		return "unknown"
	}
	if seconds < 60 {
		return fmt.Sprintf("%.0fs", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%.0fm %02ds", math.Floor(minutes), int(seconds)%60)
	}
	hours := minutes / 60
	return fmt.Sprintf("%.0fh %02dm", math.Floor(hours), int(minutes)%60)
}

func clampAnalysisScore(value int) int {
	if value < 0 {
		return 0
	}
	if value > analysisScoreBaseline {
		return analysisScoreBaseline
	}
	return value
}

func analysisSlug(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	builder := strings.Builder{}
	previousDash := false
	for _, char := range lower {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			previousDash = false
		default:
			if !previousDash && builder.Len() > 0 {
				builder.WriteRune('-')
				previousDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

// analysisTitleCase upper-cases the first rune only; strings.Title is deprecated
// and would also capitalise words inside a category label.
func analysisTitleCase(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func plural(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// Finding titles are read by people, so the verb has to agree with the count.
func verbS(count int) string {
	if count == 1 {
		return "s"
	}
	return ""
}

func isAre(count int) string {
	if count == 1 {
		return "is"
	}
	return "are"
}

func hasHave(count int) string {
	if count == 1 {
		return "has"
	}
	return "have"
}
