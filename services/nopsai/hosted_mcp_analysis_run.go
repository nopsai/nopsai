package nopsai

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Run analysis answers the question operators actually ask — "why did this fail,
// and is it my code or the platform" — so the first finding is a domain
// diagnosis rather than a log excerpt. The classification is ported from the
// browser engine (services/ui/src/features/analysis/model.ts) so chat and the
// Analysis modal name the same domain for the same run.

const (
	analysisRunDomainUnknown = "Unknown"
	analysisRunMaxPeers      = 200
)

var analysisRunDomainRules = []struct {
	Domain     string
	Confidence float64
	Pattern    *regexp.Regexp
	Kind       string
}{
	{"Pipeline definition", 0.84, regexp.MustCompile(`(?i)yaml|schema|definition|depends_on|dependency|variable|missing step|invalid output`), "inference"},
	{"Credential or authorization", 0.82, regexp.MustCompile(`(?i)credential|permission|forbidden|unauthorized|auth|secret|token`), "redacted"},
	{"Runner infrastructure", 0.78, regexp.MustCompile(`(?i)runner|kubernetes|pod|node|image pull|pullbackoff|network|storage|volume|heartbeat|capacity|queued`), "inference"},
	{"Timeout or capacity", 0.78, regexp.MustCompile(`(?i)timeout|deadline|timed out|quota`), "inference"},
	{"Approval or policy", 0.78, regexp.MustCompile(`(?i)approval|policy|rejected|denied`), "inference"},
	{"AI provider/model", 0.72, regexp.MustCompile(`(?i)openai|anthropic|gemini|llm|rate limit|tokens`), "inference"},
	{"Application tests", 0.82, regexp.MustCompile(`(?i)\btest\b|\bspec\b|junit|assert|coverage`), "inference"},
	{"Application code", 0.74, regexp.MustCompile(`(?i)build|compile|package|lint|static|exception|crash`), "inference"},
	{"Trigger or input", 0.68, regexp.MustCompile(`(?i)input|trigger|payload|branch|\bref\b`), "inference"},
}

type analysisRunDiagnosis struct {
	Domain     string
	Confidence float64
	Evidence   []analysisEvidenceItem
}

type analysisRunFailurePoint struct {
	Step     string
	Task     string
	ExitCode string
}

func analyzeRunEvidence(subject analysisSubject, window analysisWindow, set analysisEvidenceSet) map[string]any {
	detail := set.section("detail")
	runInfo := analysisSubsection(detail, "run_info")
	steps := analysisRows(detail, "steps")
	failed := analysisRunFailed(runInfo)
	firstFailure := analysisFirstFailedExecution(steps)
	diagnosis := analysisClassifyRun(runInfo, firstFailure, analysisFirstErrorLogLine(set))

	findings := []analysisFinding{}
	if failed {
		severity := "high"
		if diagnosis.Domain == analysisRunDomainUnknown {
			severity = "medium"
		}
		findings = append(findings, analysisFinding{
			Category:   "bug",
			Severity:   severity,
			Title:      "Likely failure domain: " + diagnosis.Domain,
			Summary:    analysisRunDomainSummary(diagnosis.Domain),
			Evidence:   diagnosis.Evidence,
			Confidence: diagnosis.Confidence,
			Recommendations: []analysisRecommendation{{
				Title:  analysisRunDomainAction(diagnosis.Domain),
				Detail: analysisRunDomainActionDetail(diagnosis.Domain, firstFailure),
			}},
		})
	}

	if reason := analysisString(runInfo, "failure_reason"); reason != "" {
		category := "reliability"
		if analysisRunDomainRules[1].Pattern.MatchString(reason) {
			category = "security"
		}
		findings = append(findings, analysisFinding{
			Category:   category,
			Severity:   "high",
			Title:      "The run recorded a failure reason",
			Summary:    "A top-level failure reason usually explains an orchestration failure that happened before or around the steps.",
			Evidence:   []analysisEvidenceItem{{Label: "Failure reason", Value: analysisTruncateScript(reason), Kind: "redacted"}},
			Confidence: 0.86,
			Recommendations: []analysisRecommendation{{
				Title:  "Resolve the orchestration error before rerunning",
				Detail: "Check the definition, credentials, runner capacity, and trigger inputs; a rerun repeats the same failure until one of those changes.",
			}},
		})
	}

	if firstFailure != nil {
		severity := "medium"
		summary := fmt.Sprintf("Step %s failed before any specific task was recorded as failed.", firstFailure.Step)
		evidence := []analysisEvidenceItem{{Label: "Step", Value: firstFailure.Step, Kind: "fact"}}
		if firstFailure.Task != "" {
			severity = "high"
			summary = fmt.Sprintf("Task %s failed inside step %s.", firstFailure.Task, firstFailure.Step)
			evidence = append(evidence,
				analysisEvidenceItem{Label: "Task", Value: firstFailure.Task, Kind: "fact"},
				analysisEvidenceItem{Label: "Exit code", Value: firstFailure.ExitCode, Kind: "fact"},
			)
		}
		findings = append(findings, analysisFinding{
			Category:   "bug",
			Severity:   severity,
			Title:      "First failure point: " + analysisRunFailurePointLabel(firstFailure),
			Summary:    summary,
			Evidence:   evidence,
			Confidence: 0.88,
			Recommendations: []analysisRecommendation{{
				Title:  "Read this point before any later error",
				Detail: "Downstream steps usually fail because this one did; their errors describe the consequence, not the cause.",
			}},
		})
	}

	if changed := analysisRunComparison(runInfo, analysisLastSuccessfulPeer(runInfo, set)); failed && len(changed) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "bug",
			Severity:   "medium",
			Title:      fmt.Sprintf("%d input%s changed since the last successful run", len(changed), plural(len(changed))),
			Summary:    "Something that was different last time it worked is the cheapest place to start, and it is usually not the pipeline definition.",
			Evidence:   changed,
			Confidence: 0.74,
			Recommendations: []analysisRecommendation{{
				Title:  "Start from the first changed input",
				Detail: "Compare the commit, ref, trigger, scope, and runtime inputs before editing the pipeline.",
			}},
		})
	}

	findings = append(findings, analysisRunApprovalFindings(set)...)
	findings = append(findings, analysisRunOutputFindings(detail)...)
	findings = append(findings, analysisRunChildFindings(detail)...)

	if len(findings) == 0 && set.has("detail") {
		findings = append(findings, analysisFinding{
			Category:   "monitoring",
			Severity:   "low",
			Title:      "No degradation signal in this run",
			Summary:    "The visible run metadata shows no failure, pending approval, output error, or child failure.",
			Evidence:   []analysisEvidenceItem{{Label: "Run status", Value: analysisString(runInfo, "status"), Kind: "fact"}},
			Confidence: 0.6,
			Recommendations: []analysisRecommendation{{
				Title:  "Keep comparison runs available",
				Detail: "Retaining recent successful runs is what lets the next failure be explained by what changed.",
			}},
		})
	}

	result := analysisResult("run", subject, window, findings, analysisRunNextActions(subject, runInfo, set), set)
	if failed {
		result["primary_diagnosis"] = map[string]any{
			"domain":     diagnosis.Domain,
			"confidence": diagnosis.Confidence,
		}
	}
	// A run is one event, so the window sentence the other subjects use reads
	// wrong here; what a reader wants first is the verdict.
	if summary := analysisRunHeadline(subject, runInfo, diagnosis, findings, result); summary != "" {
		result["summary"] = summary
	}
	return result
}

func analysisRunHeadline(
	subject analysisSubject,
	runInfo map[string]any,
	diagnosis analysisRunDiagnosis,
	findings []analysisFinding,
	result map[string]any,
) string {
	if len(runInfo) == 0 {
		return ""
	}
	label := firstNonEmptyString(subject.Label, subject.ID)
	blocking := 0
	for _, finding := range findings {
		if finding.Severity == "critical" || finding.Severity == "high" {
			blocking++
		}
	}
	if !analysisRunFailed(runInfo) {
		return fmt.Sprintf("Run %s finished %s and scores %v/100 with %d finding%s.",
			label, firstNonEmptyString(analysisString(runInfo, "status"), "complete"), result["health_score"], len(findings), plural(len(findings)))
	}
	return fmt.Sprintf("Run %s failed. Likely domain: %s (%.0f%% confidence), %d finding%s, %d critical or high.",
		label, diagnosis.Domain, diagnosis.Confidence*100, len(findings), plural(len(findings)), blocking)
}

func analysisClassifyRun(runInfo map[string]any, failure *analysisRunFailurePoint, logLine string) analysisRunDiagnosis {
	if !analysisRunFailed(runInfo) {
		return analysisRunDiagnosis{
			Domain:     analysisRunDomainUnknown,
			Confidence: 0.54,
			Evidence:   []analysisEvidenceItem{{Label: "Run status", Value: analysisString(runInfo, "status"), Kind: "fact"}},
		}
	}
	parts := []string{analysisString(runInfo, "failure_reason"), logLine}
	if failure != nil {
		parts = append(parts, failure.Step, failure.Task)
	}
	text := strings.TrimSpace(strings.Join(parts, " "))

	for _, rule := range analysisRunDomainRules {
		if text == "" || !rule.Pattern.MatchString(text) {
			continue
		}
		evidence := []analysisEvidenceItem{{Label: "Signal", Value: analysisTruncateScript(text), Kind: rule.Kind}}
		if failure != nil {
			evidence = append(evidence, analysisEvidenceItem{Label: "Failure point", Value: analysisRunFailurePointLabel(failure), Kind: "fact"})
		}
		return analysisRunDiagnosis{Domain: rule.Domain, Confidence: rule.Confidence, Evidence: evidence}
	}

	confidence := 0.45
	if failure != nil {
		confidence = 0.58
	}
	return analysisRunDiagnosis{
		Domain:     analysisRunDomainUnknown,
		Confidence: confidence,
		Evidence: []analysisEvidenceItem{
			{Label: "Run status", Value: analysisString(runInfo, "status"), Kind: "fact"},
			{Label: "Evidence still needed", Value: "Step logs, runner health, and the last successful run", Kind: "inference"},
		},
	}
}

func analysisRunDomainSummary(domain string) string {
	switch domain {
	case "Pipeline definition":
		return "The failure points at how the pipeline is defined rather than at the code it runs."
	case "Credential or authorization":
		return "The run was refused rather than broken: something it needed access to said no."
	case "Runner infrastructure":
		return "The failure is in where the work ran, not in what it ran."
	case "Timeout or capacity":
		return "The work did not finish inside its boundary, so nothing downstream can be trusted to have completed."
	case "Approval or policy":
		return "A human or policy gate stopped this run; the pipeline itself may be healthy."
	case "AI provider/model":
		return "A model call failed, so any step depending on generated output produced nothing usable."
	case "Application tests":
		return "The pipeline worked and the code under test did not, which is the pipeline doing its job."
	case "Application code":
		return "The failure is in the code being built or run, not in the pipeline that runs it."
	case "Trigger or input":
		return "The run started with inputs it could not use, so the failure begins before the first step."
	default:
		return "The visible evidence does not identify a domain yet."
	}
}

func analysisRunDomainAction(domain string) string {
	switch domain {
	case "Application code", "Application tests":
		return "Inspect the application change"
	case "Pipeline definition":
		return "Inspect the pipeline definition"
	case "Credential or authorization":
		return "Review the credential and its grant scope"
	case "Runner infrastructure":
		return "Inspect runner and dispatcher health"
	case "Timeout or capacity":
		return "Review the timeout and the capacity it competes for"
	case "Approval or policy":
		return "Check the approval owner and policy"
	case "AI provider/model":
		return "Check the model profile and provider limits"
	case "Trigger or input":
		return "Check the trigger payload and inputs"
	default:
		return "Collect the missing evidence"
	}
}

func analysisRunDomainActionDetail(domain string, failure *analysisRunFailurePoint) string {
	point := "the failing step"
	if failure != nil {
		point = analysisRunFailurePointLabel(failure)
	}
	switch domain {
	case "Application code", "Application tests":
		return fmt.Sprintf("Read the output of %s and compare the commit with the last successful run before touching the pipeline.", point)
	case "Pipeline definition":
		return "Analyse the pipeline definition; dependency names, step modes, and variable references are the usual causes."
	case "Credential or authorization":
		return "Confirm the credential exists, is active, and is granted to this scope; values stay redacted throughout."
	case "Runner infrastructure":
		return "Check dispatcher status and the runner pool this step targets before rerunning."
	case "Timeout or capacity":
		return "Compare the step duration with its timeout and check whether the queue was saturated at the time."
	case "Approval or policy":
		return "Confirm the approver team is staffed and notified, and that the policy matches intent."
	case "AI provider/model":
		return "Check the profile's provider quota and error, and whether a fallback profile is configured."
	case "Trigger or input":
		return "Compare the trigger payload, ref, and runtime variables with a run that worked."
	default:
		return fmt.Sprintf("Read the logs around %s, then compare with the last successful run.", point)
	}
}

func analysisRunApprovalFindings(set analysisEvidenceSet) []analysisFinding {
	pending := []analysisEvidenceItem{}
	for _, approval := range analysisRows(set.section("approvals"), "approvals") {
		if !strings.EqualFold(analysisString(approval, "status"), "pending") {
			continue
		}
		if len(pending) >= analysisMaxEvidenceRows {
			break
		}
		label := firstNonEmptyString(analysisString(approval, "step_name"), "Approval")
		pending = append(pending, analysisEvidenceItem{
			Label: label,
			Value: firstNonEmptyString(analysisString(approval, "approval_type"), "pending"),
			Kind:  "fact",
		})
	}
	if len(pending) == 0 {
		return nil
	}
	return []analysisFinding{{
		Category:   "reliability",
		Severity:   "medium",
		Title:      fmt.Sprintf("%d approval%s %s still pending", len(pending), plural(len(pending)), isAre(len(pending))),
		Summary:    "The run is waiting on a person, so its elapsed time is not execution time.",
		Evidence:   pending,
		Confidence: 0.82,
		Recommendations: []analysisRecommendation{{
			Title:  "Check who owns the approval",
			Detail: "Confirm the assigned team is notified and that the timeout and self-approval policy match intent.",
		}},
	}}
}

func analysisRunOutputFindings(detail map[string]any) []analysisFinding {
	failures := []analysisEvidenceItem{}
	for _, output := range analysisRows(detail, "final_outputs") {
		if !analysisStatusIsFailure(analysisString(output, "status")) {
			continue
		}
		if len(failures) >= analysisMaxEvidenceRows {
			break
		}
		value := firstNonEmptyString(analysisString(output, "error"), analysisString(output, "status"))
		failures = append(failures, analysisEvidenceItem{
			Label: firstNonEmptyString(analysisString(output, "name"), "output"),
			Value: analysisTruncateScript(value),
			Kind:  "redacted",
		})
	}
	if len(failures) == 0 {
		return nil
	}
	return []analysisFinding{{
		Category:   "monitoring",
		Severity:   "medium",
		Title:      fmt.Sprintf("%d final output%s failed to generate", len(failures), plural(len(failures))),
		Summary:    "The work ran but its report did not, so the result exists and nobody can read it.",
		Evidence:   failures,
		Confidence: 0.8,
		Recommendations: []analysisRecommendation{{
			Title:  "Repair the output contract separately from the run",
			Detail: "Output generation failures are a rendering or prompt problem, not a pipeline execution problem.",
		}},
	}}
}

func analysisRunChildFindings(detail map[string]any) []analysisFinding {
	failures := []analysisEvidenceItem{}
	for _, child := range analysisRows(detail, "child_runs") {
		if !analysisRunFailed(child) {
			continue
		}
		if len(failures) >= analysisMaxEvidenceRows {
			break
		}
		failures = append(failures, analysisEvidenceItem{
			Label: firstNonEmptyString(analysisString(child, "pipeline_name"), "child run"),
			Value: fmt.Sprintf("%s / %s", analysisString(child, "status"), analysisString(child, "run_id")),
			Kind:  "fact",
		})
	}
	if len(failures) == 0 {
		return nil
	}
	return []analysisFinding{{
		Category:   "reliability",
		Severity:   "high",
		Title:      fmt.Sprintf("%d child run%s failed", len(failures), plural(len(failures))),
		Summary:    "This run may only be reporting a failure that happened somewhere else.",
		Evidence:   failures,
		Confidence: 0.84,
		Recommendations: []analysisRecommendation{{
			Title:  "Analyse the failed child run first",
			Detail: "Fixing the parent cannot fix a failure that belongs to the child.",
		}},
	}}
}

func analysisRunNextActions(subject analysisSubject, runInfo map[string]any, set analysisEvidenceSet) []analysisNextAction {
	actions := []analysisNextAction{{
		Label: "Read the run logs around the first failure",
		Tool:  "nopsai.get_pipeline_run_logs",
		Args:  map[string]any{"run_id": subject.ID, "limit": 120},
	}}
	if pipeline := analysisRunPipelineID(runInfo); pipeline != "" {
		actions = append(actions, analysisNextAction{
			Label: "Analyse the pipeline this run belongs to",
			Tool:  "nopsai.analyze_pipeline",
			Args:  map[string]any{"pipeline": pipeline},
		})
	}
	if peer := analysisLastSuccessfulPeer(runInfo, set); peer != nil {
		actions = append(actions, analysisNextAction{
			Label: "Compare with the last successful run",
			Tool:  "nopsai.get_pipeline_run",
			Args:  map[string]any{"run_id": analysisString(peer, "run_id")},
		})
	}
	return actions
}

func analysisFirstFailedExecution(steps []map[string]any) *analysisRunFailurePoint {
	for _, step := range steps {
		tasks := analysisRows(step, "tasks")
		sort.SliceStable(tasks, func(i, j int) bool {
			return analysisFloat(tasks[i], "task_index") < analysisFloat(tasks[j], "task_index")
		})
		for _, task := range tasks {
			if !analysisStatusIsFailure(analysisString(task, "status")) {
				continue
			}
			return &analysisRunFailurePoint{
				Step:     analysisString(step, "name"),
				Task:     analysisString(task, "task_name"),
				ExitCode: analysisExitCode(task),
			}
		}
		if analysisStatusIsFailure(analysisString(step, "status")) {
			return &analysisRunFailurePoint{Step: analysisString(step, "name")}
		}
	}
	return nil
}

func analysisRunFailurePointLabel(failure *analysisRunFailurePoint) string {
	if failure == nil {
		return "the failing step"
	}
	if failure.Task == "" {
		return failure.Step
	}
	return failure.Step + " / " + failure.Task
}

func analysisExitCode(task map[string]any) string {
	if task["exit_code"] == nil {
		return "not recorded"
	}
	return fmt.Sprintf("%d", analysisInt(task, "exit_code"))
}

// analysisFirstErrorLogLine takes one line, not an excerpt: the classifier needs a
// signal, and shipping a log tail back through the assistant is how a redaction
// boundary gets crossed by accident.
func analysisFirstErrorLogLine(set analysisEvidenceSet) string {
	for _, entry := range analysisRows(set.section("logs"), "logs") {
		line := analysisString(entry, "line")
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal") {
			return analysisTruncateScript(line)
		}
	}
	return ""
}

func analysisLastSuccessfulPeer(runInfo map[string]any, set analysisEvidenceSet) map[string]any {
	pipeline := analysisRunPipelineID(runInfo)
	if pipeline == "" {
		return nil
	}
	runID := analysisString(runInfo, "run_id")
	var best map[string]any
	for _, peer := range analysisRows(set.section("peers"), "runs") {
		if analysisString(peer, "run_id") == runID || analysisRunPipelineID(peer) != pipeline {
			continue
		}
		if !analysisStatusIsSuccess(analysisString(peer, "status")) {
			continue
		}
		if best == nil || analysisString(peer, "created_at") > analysisString(best, "created_at") {
			best = peer
		}
	}
	return best
}

func analysisRunComparison(runInfo map[string]any, peer map[string]any) []analysisEvidenceItem {
	if len(runInfo) == 0 || len(peer) == 0 {
		return nil
	}
	fields := []struct {
		Label string
		Key   string
	}{
		{"Application commit", "git_commit_sha"},
		{"Pipeline revision", "pipeline_version"},
		{"Pipeline source", "pipeline_source"},
		{"Git ref", "git_ref"},
		{"Trigger source", "trigger_source"},
		{"Scope", "scope"},
	}
	changed := []analysisEvidenceItem{}
	for _, field := range fields {
		before := analysisString(peer, field.Key)
		after := analysisString(runInfo, field.Key)
		if before == after {
			continue
		}
		changed = append(changed, analysisEvidenceItem{
			Label: field.Label,
			Value: fmt.Sprintf("%s -> %s", analysisDisplayValue(before), analysisDisplayValue(after)),
			Kind:  "fact",
		})
	}
	if before, after := analysisRuntimeInputDigest(peer), analysisRuntimeInputDigest(runInfo); before != after {
		changed = append(changed, analysisEvidenceItem{
			Label: "Runtime inputs",
			Value: fmt.Sprintf("%s -> %s", analysisDisplayValue(before), analysisDisplayValue(after)),
			Kind:  "fact",
		})
	}
	return changed
}

func analysisRuntimeInputDigest(runInfo map[string]any) string {
	overrides, _ := runInfo["runtime_variable_overrides"].(map[string]any)
	if len(overrides) == 0 {
		return ""
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Names only: a runtime override value can carry anything the trigger sent.
	return strings.Join(keys, ", ")
}

func analysisDisplayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if len(value) > 12 {
		return value[:12]
	}
	return value
}

func analysisRunPipelineID(runInfo map[string]any) string {
	name := analysisString(runInfo, "pipeline_name")
	if name == "" {
		return ""
	}
	if path := strings.Trim(analysisString(runInfo, "pipeline_path"), "/"); path != "" {
		return path + "/" + name
	}
	return name
}

func analysisRunFailed(runInfo map[string]any) bool {
	if len(runInfo) == 0 {
		return false
	}
	return analysisStatusIsFailure(analysisString(runInfo, "status"))
}

func analysisStatusIsFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failure", "failed", "error", "cancelled", "canceled", "timeout", "timed_out":
		return true
	default:
		return false
	}
}

func analysisStatusIsSuccess(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "completed", "passed":
		return true
	default:
		return false
	}
}

func analysisSubsection(section map[string]any, key string) map[string]any {
	if len(section) == 0 {
		return nil
	}
	value, _ := section[key].(map[string]any)
	return value
}
