package nopsai

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

// Definition rules are ported from the browser analysis engine
// (services/ui/src/features/analysis/model.ts) so a pipeline reviewed in chat and
// a pipeline reviewed in the Analysis modal raise the same findings. They read
// the parsed pipeline where structure matters and the raw text where a literal
// matters, which is the only way to see a secret that should not be there.

var (
	analysisSecretLinePattern     = regexp.MustCompile(`(?i)^\s*[\w.-]*(token|secret|password|api[_-]?key|private[_-]?key)[\w.-]*\s*:\s*(.+)$`)
	analysisSecretReferenceValue  = regexp.MustCompile(`(?i)^(credential|secret)://`)
	analysisTemplatedValue        = regexp.MustCompile(`^\$\{[^}]+\}$`)
	analysisPrivilegedPattern     = regexp.MustCompile(`(?i)\bprivileged\s*:\s*true\b`)
	analysisProductionPattern     = regexp.MustCompile(`(?i)\b(prod|production)\b`)
	analysisOutputSignalPattern   = regexp.MustCompile(`(?i)artifact|junit|report|summary|metrics|correlation`)
	analysisPipeToShellPattern    = regexp.MustCompile(`(?is)(curl|wget)\b.+\|\s*(sh|bash)`)
	analysisEvalPattern           = regexp.MustCompile(`(?i)\b(eval|source)\b`)
	analysisShellToolPattern      = regexp.MustCompile(`(?i)\b(kubectl|helm|docker|ssh|scp|bash|sh)\b`)
	analysisUntrustedInputPattern = regexp.MustCompile(`(?i)\$\{?[A-Z0-9_]*(target|environment|branch|ref|command|script)[A-Z0-9_]*\}?`)
	analysisCheckStepPattern      = regexp.MustCompile(`(?i)\b(test|lint|scan|build)\b`)
	analysisOrderedStepPattern    = regexp.MustCompile(`(?i)\b(deploy|release|publish|promote|rollback|approve|approval)\b`)
)

func analysisPipelineDefinitionFindings(raw string) []analysisFinding {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		return []analysisFinding{{
			Category:   "maintainability",
			Severity:   "critical",
			Title:      "Pipeline YAML does not parse",
			Summary:    "The stored definition could not be parsed, so nothing downstream of it can be trusted to run.",
			Evidence:   []analysisEvidenceItem{{Label: "Parser error", Value: err.Error(), Kind: "fact"}},
			Confidence: 0.96,
			Recommendations: []analysisRecommendation{{
				Title:  "Fix the YAML before the next run",
				Detail: "Resolve the syntax error and validate the definition before saving or executing it.",
			}},
		}}
	}

	findings := []analysisFinding{}
	steps := analysisPipelineSteps(pipeline)
	mentionsProduction := analysisProductionPattern.MatchString(raw)

	if strings.TrimSpace(pipeline.Description) == "" {
		findings = append(findings, analysisFinding{
			Category:   "maintainability",
			Severity:   "medium",
			Title:      "Pipeline has no operator description",
			Summary:    "Nobody on call can tell what this pipeline does, what it affects, or when it should be used.",
			Evidence:   []analysisEvidenceItem{{Label: "Description", Value: "Missing", Kind: "fact"}},
			Confidence: 0.88,
			Recommendations: []analysisRecommendation{{
				Title:  "Document owner, trigger, and rollback",
				Detail: "State which application it affects, how it starts, what it produces, and how to roll back.",
			}},
		})
	}

	if len(steps) == 0 {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "critical",
			Title:      "Pipeline has no executable steps",
			Summary:    "The definition parses but declares no work, so every run is a no-op.",
			Evidence:   []analysisEvidenceItem{{Label: "Step count", Value: "0", Kind: "metric"}},
			Confidence: 0.94,
			Recommendations: []analysisRecommendation{{
				Title:  "Declare at least one step",
				Detail: "Add a script, goal, tasks, reusable include, or approval step.",
			}},
		})
	}

	if secretLines := analysisSecretLiterals(raw); len(secretLines) > 0 {
		findings = append(findings, analysisFinding{
			Category: "security",
			Severity: "critical",
			Title:    fmt.Sprintf("%d line%s embed%s a secret-like literal", len(secretLines), plural(len(secretLines)), verbS(len(secretLines))),
			Summary:  "A credential written into the definition is readable by everyone who can read the repository, and rotating it means editing YAML.",
			// Values are never echoed: the finding proves the line, not the secret.
			Evidence:   secretLines,
			Confidence: 0.92,
			Recommendations: []analysisRecommendation{{
				Title:  "Move the value behind a credential or scoped secret",
				Detail: "Store it in the credential broker or a scoped secret and reference it; the definition should carry the reference only.",
			}},
		})
	}

	if images := analysisUnpinnedImages(pipeline, steps); len(images) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "security",
			Severity:   "high",
			Title:      fmt.Sprintf("%d container image%s %s not pinned", len(images), plural(len(images)), isAre(len(images))),
			Summary:    "A mutable tag means two runs of the same commit can execute different code, which breaks both reproducibility and supply-chain review.",
			Evidence:   images,
			Confidence: 0.86,
			Recommendations: []analysisRecommendation{{
				Title:  "Pin images by digest",
				Detail: "Use an immutable digest on production paths and make base-image upgrades an explicit reviewed change.",
			}},
		})
	}

	if analysisPrivilegedPattern.MatchString(raw) {
		findings = append(findings, analysisFinding{
			Category:   "security",
			Severity:   "high",
			Title:      "Privileged container execution is requested",
			Summary:    "Privileged execution hands the workload the runner host, so it should be exceptional and isolated.",
			Evidence:   []analysisEvidenceItem{{Label: "Privilege flag", Value: "privileged: true", Kind: "fact"}},
			Confidence: 0.9,
			Recommendations: []analysisRecommendation{{
				Title:  "Remove it or isolate the pool",
				Detail: "If it is genuinely required, run it on a hardened pool behind an approval gate.",
			}},
		})
	}

	if scripts := analysisRiskyScripts(steps); len(scripts) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "security",
			Severity:   "high",
			Title:      fmt.Sprintf("%d script%s combine%s shell execution with external input", len(scripts), plural(len(scripts)), verbS(len(scripts))),
			Summary:    "Piping a remote installer into a shell, or interpolating a caller-controlled value into a command, turns an input into code.",
			Evidence:   scripts,
			Confidence: 0.76,
			Recommendations: []analysisRecommendation{{
				Title:  "Pass structured arguments instead",
				Detail: "Validate enumerated inputs and pass them as arguments rather than interpolating them into a shell string.",
			}},
		})
	}

	if strings.TrimSpace(pipeline.Timeout) == "" {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "medium",
			Title:      "Pipeline has no timeout",
			Summary:    "Without a timeout a stuck run holds a runner indefinitely and never reports a result.",
			Evidence:   []analysisEvidenceItem{{Label: "Timeout", Value: "Missing", Kind: "fact"}},
			Confidence: 0.88,
			Recommendations: []analysisRecommendation{{
				Title:  "Set a pipeline timeout",
				Detail: "Add a pipeline-level timeout, and tighter step timeouts around external calls.",
			}},
		})
	}

	approvals := analysisApprovalSteps(steps)
	if mentionsProduction && len(approvals) == 0 {
		findings = append(findings, analysisFinding{
			Category: "security",
			Severity: "high",
			Title:    "Production path has no approval gate",
			Summary:  "The definition targets production and nothing pauses for a human before it does.",
			Evidence: []analysisEvidenceItem{
				{Label: "Production signal", Value: "prod/production appears in the definition", Kind: "inference"},
				{Label: "Approval steps", Value: "0", Kind: "metric"},
			},
			Confidence: 0.8,
			Recommendations: []analysisRecommendation{{
				Title:  "Add an approval step before the production action",
				Detail: "Assign an accountable approver team and disable self-approval on that gate.",
			}},
		})
	}
	if selfApproval := analysisSelfApprovalSteps(approvals); mentionsProduction && len(selfApproval) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "security",
			Severity:   "medium",
			Title:      "A production approval allows self-approval",
			Summary:    "An approval the requester can grant themselves records a decision without adding one, so the gate documents rather than controls.",
			Evidence:   selfApproval,
			Confidence: 0.82,
			Recommendations: []analysisRecommendation{{
				Title:  "Set allow_self_approval: false",
				Detail: "Keep the approver team accountable and separate from whoever triggers the run.",
			}},
		})
	}

	findings = append(findings, analysisDependencyFindings(steps)...)

	if analysisPipelineHasNoOutputSignal(pipeline, raw) {
		findings = append(findings, analysisFinding{
			Category:   "monitoring",
			Severity:   "medium",
			Title:      "Pipeline produces no operator-facing output",
			Summary:    "No declared output, report, or metric means the only way to learn what a run did is to read its logs.",
			Evidence:   []analysisEvidenceItem{{Label: "Output metadata", Value: "No output, report, or metrics indicators found", Kind: "inference"}},
			Confidence: 0.7,
			Recommendations: []analysisRecommendation{{
				Title:  "Publish a summary output",
				Detail: "Emit the deployed identifier, test summary, or failure classification as a final output.",
			}},
		})
	}

	if finding := analysisSequentialOpportunity(steps); finding != nil {
		findings = append(findings, *finding)
	}
	return findings
}

type analysisStepView struct {
	Name          string
	DependsOn     []string
	Image         string
	Script        string
	IsApproval    bool
	SelfApproval  bool
	ApprovalTeams []string
}

func analysisPipelineSteps(pipeline models.Pipeline) []analysisStepView {
	views := make([]analysisStepView, 0, len(pipeline.Steps))
	for _, wrapper := range pipeline.Steps {
		step := wrapper.Step
		if step == nil {
			continue
		}
		view := analysisStepView{
			Name:      strings.TrimSpace(step.GetName()),
			DependsOn: step.GetDependsOn(),
			Image:     strings.TrimSpace(step.GetImage()),
		}
		if script, ok := step.AsScriptStep(); ok {
			view.Script = script.Script
		}
		if task, ok := step.AsTaskStep(); ok {
			scripts := make([]string, 0, len(task.Tasks))
			for _, item := range task.Tasks {
				if text := strings.TrimSpace(item.Script); text != "" {
					scripts = append(scripts, text)
				}
			}
			view.Script = strings.Join(scripts, "\n")
		}
		if approval, ok := step.AsApprovalStep(); ok {
			view.IsApproval = true
			view.SelfApproval = approval.Approval.AllowSelfApproval
			view.ApprovalTeams = approval.Approval.Teams
		}
		views = append(views, view)
	}
	return views
}

func analysisSecretLiterals(raw string) []analysisEvidenceItem {
	items := []analysisEvidenceItem{}
	for index, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		match := analysisSecretLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		value := strings.Trim(strings.TrimSpace(match[2]), `'"`)
		if value == "" || value == "true" || value == "false" || value == "<redacted>" {
			continue
		}
		if analysisSecretReferenceValue.MatchString(value) ||
			analysisTemplatedValue.MatchString(value) ||
			strings.HasPrefix(value, "ENC[") ||
			strings.Trim(value, "*") == "" {
			continue
		}
		if len(items) >= analysisMaxEvidenceRows {
			break
		}
		items = append(items, analysisEvidenceItem{
			Label: fmt.Sprintf("Line %d", index+1),
			Value: analysisRedactLine(line),
			Kind:  "redacted",
		})
	}
	return items
}

func analysisRedactLine(line string) string {
	if cut := strings.Index(line, ":"); cut >= 0 {
		return strings.TrimSpace(line[:cut+1]) + " [redacted]"
	}
	return "[redacted]"
}

func analysisUnpinnedImages(pipeline models.Pipeline, steps []analysisStepView) []analysisEvidenceItem {
	items := []analysisEvidenceItem{}
	if image := strings.TrimSpace(pipeline.ContainerImage); analysisImageIsUnpinned(image) {
		items = append(items, analysisEvidenceItem{Label: "Pipeline image", Value: image, Kind: "fact"})
	}
	for _, step := range steps {
		if len(items) >= analysisMaxEvidenceRows {
			break
		}
		if analysisImageIsUnpinned(step.Image) {
			items = append(items, analysisEvidenceItem{Label: "Step " + step.Name, Value: step.Image, Kind: "fact"})
		}
	}
	return items
}

func analysisImageIsUnpinned(image string) bool {
	image = strings.TrimSpace(image)
	if image == "" || strings.Contains(image, "@sha256:") {
		return false
	}
	segments := strings.Split(image, "/")
	last := segments[len(segments)-1]
	if !strings.Contains(last, ":") {
		return true
	}
	return strings.HasSuffix(strings.ToLower(last), ":latest")
}

func analysisRiskyScripts(steps []analysisStepView) []analysisEvidenceItem {
	items := []analysisEvidenceItem{}
	for _, step := range steps {
		script := strings.TrimSpace(step.Script)
		if script == "" || !analysisScriptIsRisky(script) {
			continue
		}
		if len(items) >= analysisMaxEvidenceRows {
			break
		}
		items = append(items, analysisEvidenceItem{
			Label: "Step " + step.Name,
			Value: analysisTruncateScript(script),
			Kind:  "redacted",
		})
	}
	return items
}

func analysisScriptIsRisky(script string) bool {
	if analysisPipeToShellPattern.MatchString(script) {
		return true
	}
	if analysisEvalPattern.MatchString(script) && strings.ContainsAny(script, "$`") {
		return true
	}
	return analysisShellToolPattern.MatchString(script) && analysisUntrustedInputPattern.MatchString(script)
}

func analysisTruncateScript(script string) string {
	collapsed := strings.Join(strings.Fields(script), " ")
	if len(collapsed) <= 120 {
		return collapsed
	}
	return collapsed[:117] + "..."
}

func analysisApprovalSteps(steps []analysisStepView) []analysisStepView {
	approvals := []analysisStepView{}
	for _, step := range steps {
		if step.IsApproval {
			approvals = append(approvals, step)
		}
	}
	return approvals
}

func analysisSelfApprovalSteps(approvals []analysisStepView) []analysisEvidenceItem {
	items := []analysisEvidenceItem{}
	for _, step := range approvals {
		if !step.SelfApproval {
			continue
		}
		teams := strings.Join(step.ApprovalTeams, ", ")
		if teams == "" {
			teams = "no approver team declared"
		}
		items = append(items, analysisEvidenceItem{
			Label: "Step " + step.Name,
			Value: "allow_self_approval: true (" + teams + ")",
			Kind:  "fact",
		})
	}
	return items
}

func analysisDependencyFindings(steps []analysisStepView) []analysisFinding {
	if len(steps) == 0 {
		return nil
	}
	known := map[string]bool{}
	for _, step := range steps {
		known[step.Name] = true
	}
	findings := []analysisFinding{}

	missing := []analysisEvidenceItem{}
	for _, step := range steps {
		for _, dependency := range step.DependsOn {
			dependency = strings.TrimSpace(dependency)
			if dependency == "" || known[dependency] {
				continue
			}
			if len(missing) >= analysisMaxEvidenceRows {
				break
			}
			missing = append(missing, analysisEvidenceItem{
				Label: step.Name,
				Value: "depends_on " + dependency,
				Kind:  "fact",
			})
		}
	}
	if len(missing) > 0 {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "high",
			Title:      "Step dependencies point at steps that do not exist",
			Summary:    "An unresolvable dependency name means the intended order is not the order that runs.",
			Evidence:   missing,
			Confidence: 0.92,
			Recommendations: []analysisRecommendation{{
				Title:  "Correct the dependency names",
				Detail: "Fix the spelling or add the missing step before the next run.",
			}},
		})
	}

	if analysisStepsHaveCycle(steps) {
		findings = append(findings, analysisFinding{
			Category:   "reliability",
			Severity:   "critical",
			Title:      "Step dependencies contain a cycle",
			Summary:    "Execution order cannot be resolved, so the pipeline cannot run as written.",
			Evidence:   []analysisEvidenceItem{{Label: "Dependency graph", Value: "Cycle detected in depends_on", Kind: "fact"}},
			Confidence: 0.94,
			Recommendations: []analysisRecommendation{{
				Title:  "Break the cycle",
				Detail: "Remove one edge, or split shared setup into its own step that both sides depend on.",
			}},
		})
	}
	return findings
}

func analysisStepsHaveCycle(steps []analysisStepView) bool {
	dependencies := map[string][]string{}
	for _, step := range steps {
		dependencies[step.Name] = step.DependsOn
	}
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := map[string]int{}
	var visit func(name string) bool
	visit = func(name string) bool {
		switch state[name] {
		case visiting:
			return true
		case done:
			return false
		}
		state[name] = visiting
		for _, dependency := range dependencies[name] {
			dependency = strings.TrimSpace(dependency)
			if _, known := dependencies[dependency]; !known {
				continue
			}
			if visit(dependency) {
				return true
			}
		}
		state[name] = done
		return false
	}
	for _, step := range steps {
		if visit(step.Name) {
			return true
		}
	}
	return false
}

func analysisPipelineHasNoOutputSignal(pipeline models.Pipeline, raw string) bool {
	if len(pipeline.Output.Items) > 0 {
		return false
	}
	return !analysisOutputSignalPattern.MatchString(raw)
}

func analysisSequentialOpportunity(steps []analysisStepView) *analysisFinding {
	if len(steps) < 3 {
		return nil
	}
	chained := 0
	checks := 0
	for index, step := range steps {
		name := strings.ToLower(step.Name)
		// Approval and release ordering is deliberate; only independent checks are
		// candidates for running together.
		if step.IsApproval || analysisOrderedStepPattern.MatchString(name) {
			return nil
		}
		if analysisCheckStepPattern.MatchString(name) {
			checks++
		}
		if index == 0 {
			continue
		}
		for _, dependency := range step.DependsOn {
			if strings.TrimSpace(dependency) == steps[index-1].Name {
				chained++
				break
			}
		}
	}
	if chained < len(steps)-1 || checks < 2 {
		return nil
	}
	return &analysisFinding{
		Category: "efficiency",
		Severity: "opportunity",
		Title:    "Independent checks run one after another",
		Summary:  "Build, lint, scan, and test style steps are chained in a straight line, so the pipeline takes their sum rather than their maximum.",
		Evidence: []analysisEvidenceItem{
			{Label: "Sequential edges", Value: fmt.Sprintf("%d of %d", chained, len(steps)-1), Kind: "metric"},
		},
		Confidence: 0.64,
		Recommendations: []analysisRecommendation{{
			Title:  "Let independent checks run together",
			Detail: "Drop depends_on between checks that do not consume each other's output, and keep deploy and publish ordered.",
		}},
	}
}
