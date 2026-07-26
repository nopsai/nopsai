package validation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
)

type AgentProfileDefinition struct {
	Enabled bool
}

type AgentProfileValidationOptions struct {
	DefaultProfile        string
	RequireDefaultProfile bool
	Profiles              map[string]AgentProfileDefinition
}

type LLMProfileDefinition struct {
	AllowedScopes []string
}

type LLMProfileValidationOptions struct {
	DefaultProfile        string
	RequireDefaultProfile bool
	Profiles              map[string]LLMProfileDefinition
	Scope                 string
}

type MCPProfileDefinition struct {
	Enabled       bool
	AllowedScopes []string
}

type MCPProfileValidationOptions struct {
	Profiles map[string]MCPProfileDefinition
	Scope    string
}

var supportedKnowledgeContextKinds = map[string]struct{}{
	"architecture": {},
	"guardrail":    {},
	"policy":       {},
	"adr":          {},
	"guideline":    {},
	"runbook":      {},
	"reference":    {},
	"example":      {},
}

var supportedPipelineOutputTypes = map[string]struct{}{
	"markdown":  {},
	"pdf":       {},
	"excel":     {},
	"json":      {},
	"html":      {},
	"dashboard": {},
}

var supportedPipelineOutputWhen = map[string]struct{}{
	"always":  {},
	"success": {},
	"failure": {},
}

var supportedDashboardOutputModes = map[string]struct{}{
	"":         {},
	"replace":  {},
	"append":   {},
	"snapshot": {},
	"series":   {},
}

var supportedDashboardOutputPresets = map[string]struct{}{
	"":           {},
	"auto":       {},
	"report":     {},
	"table":      {},
	"status":     {},
	"timeline":   {},
	"comparison": {},
	"metrics":    {},
	"mixed":      {},
}

func ValidatePipeline(pipeline *models.Pipeline) error {
	if pipeline.Name == "" {
		return fmt.Errorf("'name' is a required field")
	}
	if err := validatePipelineLLMSettings(pipeline); err != nil {
		return err
	}
	if err := validatePipelineOutput(pipeline.Output); err != nil {
		return err
	}
	if err := validatePipelineVariableDeclarations(pipeline.Variables); err != nil {
		return err
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(pipeline.Name) {
		return fmt.Errorf("pipeline name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if pipeline.Version == "" {
		pipeline.Version = "latest"
	} else if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(pipeline.Version) {
		return fmt.Errorf("pipeline version can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if pipeline.ContainerImage == "" {
		for _, step := range pipeline.Steps {
			if _, ok := step.AsApprovalStep(); ok {
				continue
			}
			if step.GetImage() == "" {
				return fmt.Errorf("'container_image' is a required field if executable steps don't have their own image")
			}
		}
	}
	if _, err := models.NormalizePipelineWorkingDirectory(pipeline.WorkingDirectory); err != nil {
		return err
	}
	if len(pipeline.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	if err := validateKnowledgeContextRefs(pipeline.KnowledgeContext, "pipeline"); err != nil {
		return err
	}
	if err := validatePolicyMergeMode(pipeline.PolicyMergeMode, "pipeline"); err != nil {
		return err
	}

	allStepNames := make(map[string]bool)
	stepToTaskNames := make(map[string]map[string]bool)
	taskOutputDeclarations := make(map[string]map[string]models.TaskOutput)

	// First pass: Collect all step and task names
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			return fmt.Errorf("a step is missing its required 'name' field")
		}
		if allStepNames[stepName] {
			return fmt.Errorf("duplicate step name '%s' found. Step names must be unique", stepName)
		}
		if err := validateKnowledgeContextRefs(step.GetKnowledgeContext(), fmt.Sprintf("step '%s'", stepName)); err != nil {
			return err
		}
		if err := validatePolicyMergeMode(step.GetPolicyMergeMode(), fmt.Sprintf("step '%s'", stepName)); err != nil {
			return err
		}
		if err := validateReservedRuntimeOutputMounts(step.GetVolumes(), fmt.Sprintf("step '%s'", stepName)); err != nil {
			return err
		}
		if err := validateRuntimeVariableMap(step.GetVariables(), fmt.Sprintf("step '%s'", stepName)); err != nil {
			return err
		}
		if err := validateScopedRuntimeRefs(step.GetSecrets(), fmt.Sprintf("secrets in step '%s'", stepName)); err != nil {
			return err
		}
		allStepNames[stepName] = true
		stepToTaskNames[stepName] = make(map[string]bool)

		isIncludeStep := step.GetInclude() != ""
		tasks := step.GetTasks()
		isTaskStep := len(tasks) > 0
		isLegacyStep := strings.TrimSpace(step.GetGoal()) != "" || strings.TrimSpace(step.GetScript()) != ""
		approvalStep, isApprovalStep := step.AsApprovalStep()

		if isIncludeStep {
			if isTaskStep || isLegacyStep || isApprovalStep {
				return fmt.Errorf("step '%s' is an 'include' step and cannot also contain 'tasks', 'goal', 'script', or 'approval'", stepName)
			}
			if len(step.GetOutputs()) > 0 {
				return fmt.Errorf("include step '%s' cannot declare task outputs", stepName)
			}
		} else if isApprovalStep {
			if isTaskStep || isLegacyStep {
				return fmt.Errorf("step '%s' is an 'approval' step and cannot also contain 'tasks', 'goal', or 'script'", stepName)
			}
			if len(step.GetOutputs()) > 0 {
				return fmt.Errorf("approval step '%s' cannot declare task outputs", stepName)
			}
			if err := validateApprovalDefinition(approvalStep.Approval, stepName); err != nil {
				return err
			}
		} else if isTaskStep {
			if isLegacyStep {
				return fmt.Errorf("step '%s' has tasks and should not also contain 'goal' or 'script'", stepName)
			}
			if len(step.GetOutputs()) > 0 {
				return fmt.Errorf("step '%s' has tasks, so outputs must be declared on individual tasks", stepName)
			}
			for _, task := range tasks {
				if task.Name == "" {
					return fmt.Errorf("a task in step '%s' is missing its required 'name' field", stepName)
				}
				if stepToTaskNames[stepName][task.Name] {
					return fmt.Errorf("duplicate task name '%s' found within step '%s'. Task names must be unique within a step", task.Name, stepName)
				}
				stepToTaskNames[stepName][task.Name] = true
				if err := validateRuntimeVariableMap(task.Variables, fmt.Sprintf("task '%s' in step '%s'", task.Name, stepName)); err != nil {
					return err
				}

				hasGoal := strings.TrimSpace(task.Goal) != ""
				hasScript := strings.TrimSpace(task.Script) != ""
				if hasGoal && hasScript {
					return fmt.Errorf("task '%s' in step '%s' cannot define both 'goal' and 'script'", task.Name, stepName)
				}
				if !hasGoal && !hasScript {
					return fmt.Errorf("task '%s' in step '%s' must define either 'goal' or 'script'", task.Name, stepName)
				}
				if err := validateKnowledgeContextRefs(task.KnowledgeContext, fmt.Sprintf("task '%s' in step '%s'", task.Name, stepName)); err != nil {
					return err
				}
				if err := validatePolicyMergeMode(task.PolicyMergeMode, fmt.Sprintf("task '%s' in step '%s'", task.Name, stepName)); err != nil {
					return err
				}
				if err := validateTaskOutputs(task.Outputs, fmt.Sprintf("task '%s' in step '%s'", task.Name, stepName)); err != nil {
					return err
				}
				recordTaskOutputDeclarations(taskOutputDeclarations, stepName, task.Name, task.Outputs)
			}
		} else if !isLegacyStep {
			return fmt.Errorf("step '%s' must contain 'include', 'tasks', 'goal', 'script', or 'approval'", stepName)
		} else if err := validateTaskOutputs(step.GetOutputs(), fmt.Sprintf("step '%s'", stepName)); err != nil {
			return err
		} else {
			stepToTaskNames[stepName][stepName] = true
			recordTaskOutputDeclarations(taskOutputDeclarations, stepName, stepName, step.GetOutputs())
		}
	}

	// Second pass: Validate dependencies based on the corrected rules
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		// A step can depend on a whole step or on a qualified producer task.
		for _, depName := range step.GetDependsOn() {
			if strings.Contains(depName, models.RuntimeOutputReferencePrefix) {
				return fmt.Errorf("step '%s' dependency %q cannot use a runtime output reference", stepName, depName)
			}
			if _, _, ok := resolvePipelineTaskDependency(depName, stepName, allStepNames, stepToTaskNames); ok {
				continue
			}
			if !allStepNames[depName] {
				return fmt.Errorf("step '%s' has an undefined dependency: '%s'", stepName, depName)
			}
		}

		if err := validateRuntimeOutputRefsInVariables(step.GetVariables(), outputRefConsumer{
			stepName:      stepName,
			taskName:      stepName,
			stepDependsOn: step.GetDependsOn(),
		}, stepToTaskNames, taskOutputDeclarations); err != nil {
			return err
		}

		// Rule: If a step has tasks, those tasks can depend on tasks in the same step
		// or on a fully-qualified task dependency in an earlier step.
		tasks := step.GetTasks()
		if len(tasks) > 0 {
			for _, task := range tasks {
				for _, depName := range task.DependsOn {
					if strings.Contains(depName, models.RuntimeOutputReferencePrefix) {
						return fmt.Errorf("task '%s' in step '%s' dependency %q cannot use a runtime output reference", task.Name, stepName, depName)
					}
					_, depTask, ok := resolvePipelineTaskDependency(depName, stepName, allStepNames, stepToTaskNames)
					if !ok || depTask == "" {
						return fmt.Errorf("task '%s' in step '%s' has an invalid dependency: '%s'. Tasks can depend on another task in the same step or use a qualified step.task dependency", task.Name, stepName, depName)
					}
				}
				if err := validateRuntimeOutputRefsInVariables(task.Variables, outputRefConsumer{
					stepName:      stepName,
					taskName:      task.Name,
					taskDependsOn: task.DependsOn,
					stepDependsOn: step.GetDependsOn(),
				}, stepToTaskNames, taskOutputDeclarations); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

type outputRefConsumer struct {
	stepName      string
	taskName      string
	stepDependsOn []string
	taskDependsOn []string
}

func ValidateReusableStep(step *models.PipelineStep) error {
	if step == nil || step.Step == nil {
		return fmt.Errorf("reusable step is required")
	}
	if strings.TrimSpace(step.GetName()) == "" {
		return fmt.Errorf("reusable step is missing the required 'name' field")
	}
	pipeline := &models.Pipeline{
		Name:           "reusable-step-validation",
		ContainerImage: "nopsai/reusable-step-validation",
		Steps:          []models.PipelineStep{*step},
	}
	if err := ValidatePipeline(pipeline); err != nil {
		return fmt.Errorf("invalid reusable step %q: %w", strings.TrimSpace(step.GetName()), err)
	}
	return nil
}

func validatePipelineVariableDeclarations(variables []string) error {
	seenRuntimeNames := map[string]string{}
	for idx, raw := range variables {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return fmt.Errorf("variables[%d] is empty", idx)
		}
		ref, err := models.ParseScopedRuntimeRef(trimmed, "")
		if err != nil {
			return fmt.Errorf("variables[%d] %q is invalid: %w", idx, raw, err)
		}
		if err := validateScopedRuntimeRefParts(ref, fmt.Sprintf("variables[%d]", idx)); err != nil {
			return err
		}
		nameKey := strings.TrimSpace(ref.Name)
		if previous, ok := seenRuntimeNames[nameKey]; ok {
			return fmt.Errorf("variables[%d] %q declares runtime variable %q more than once; previous declaration was %q", idx, raw, nameKey, previous)
		}
		seenRuntimeNames[nameKey] = trimmed
	}
	return nil
}

func validateRuntimeVariableMap(variables map[string]string, location string) error {
	for rawName := range variables {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return fmt.Errorf("variables in %s contain an empty key", location)
		}
		if !models.IsValidRuntimeReferenceName(name) {
			return fmt.Errorf("variable %q in %s is invalid; variable names must match ^[A-Za-z0-9_.-]+$", rawName, location)
		}
	}
	return nil
}

func validateScopedRuntimeRefs(values []string, location string) error {
	seenRuntimeNames := map[string]string{}
	for idx, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return fmt.Errorf("%s[%d] is empty", location, idx)
		}
		ref, err := models.ParseScopedRuntimeRef(trimmed, "")
		if err != nil {
			return fmt.Errorf("%s[%d] %q is invalid: %w", location, idx, raw, err)
		}
		if err := validateScopedRuntimeRefParts(ref, fmt.Sprintf("%s[%d]", location, idx)); err != nil {
			return err
		}
		nameKey := strings.TrimSpace(ref.Name)
		if previous, ok := seenRuntimeNames[nameKey]; ok && previous != ref.LookupKey() {
			return fmt.Errorf("%s[%d] %q resolves runtime name %q from multiple scopes", location, idx, raw, nameKey)
		}
		seenRuntimeNames[nameKey] = ref.LookupKey()
	}
	return nil
}

func validateScopedRuntimeRefParts(ref models.ScopedRuntimeRef, location string) error {
	if !models.IsValidRuntimeReferenceName(ref.Name) {
		return fmt.Errorf("%s name %q is invalid; names must match ^[A-Za-z0-9_.-]+$", location, ref.Name)
	}
	if ref.ExplicitScope {
		scope := strings.Trim(strings.TrimSpace(ref.Scope), "/")
		if scope != "" {
			if err := validateRelativeKnowledgePath(scope); err != nil {
				return fmt.Errorf("%s scope %q is invalid: %w", location, ref.Scope, err)
			}
		}
	}
	return nil
}

func validateTaskOutputs(outputs []models.TaskOutput, location string) error {
	if len(outputs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for idx, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			return fmt.Errorf("outputs[%d] in %s is missing its required name", idx, location)
		}
		if !models.IsValidTaskOutputName(name) {
			return fmt.Errorf("output %q in %s is invalid; output names must match ^[A-Za-z_][A-Za-z0-9_]*$", output.Name, location)
		}
		if seen[name] {
			return fmt.Errorf("output %q in %s is declared more than once", name, location)
		}
		seen[name] = true
	}
	return nil
}

func validateReservedRuntimeOutputMounts(volumes []string, location string) error {
	for _, raw := range volumes {
		parts := strings.Split(raw, ":")
		if len(parts) != 2 {
			continue
		}
		mountPath := strings.TrimRight(strings.TrimSpace(parts[1]), "/")
		if mountPath == models.RuntimeOutputsMountPath {
			return fmt.Errorf("%s volume %q cannot mount reserved runtime output path %s", location, raw, models.RuntimeOutputsMountPath)
		}
	}
	return nil
}

func recordTaskOutputDeclarations(declarations map[string]map[string]models.TaskOutput, stepName, taskName string, outputs []models.TaskOutput) {
	if len(outputs) == 0 {
		return
	}
	taskKey := models.RuntimeOutputRefKey(stepName, taskName, "")
	taskKey = strings.TrimSuffix(taskKey, "/")
	if declarations[taskKey] == nil {
		declarations[taskKey] = map[string]models.TaskOutput{}
	}
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name != "" {
			declarations[taskKey][name] = output
		}
	}
}

func validateRuntimeOutputRefsInVariables(variables map[string]string, consumer outputRefConsumer, stepToTaskNames map[string]map[string]bool, declarations map[string]map[string]models.TaskOutput) error {
	if len(variables) == 0 {
		return nil
	}
	for name, value := range variables {
		ref, found, err := models.ParseRuntimeOutputRef(value)
		if err != nil {
			return fmt.Errorf("variable %q in step '%s' references an invalid runtime output: %w", name, consumer.stepName, err)
		}
		if !found {
			if strings.Contains(value, models.RuntimeOutputReferencePrefix) {
				return fmt.Errorf("variable %q in step '%s' uses a runtime output in an unsupported expression; use the full value $steps.<step>.<task>.outputs.<name>", name, consumer.stepName)
			}
			continue
		}
		if !stepToTaskNames[ref.StepName][ref.TaskName] {
			return fmt.Errorf("variable %q references missing output producer task %s.%s", name, ref.StepName, ref.TaskName)
		}
		producerKey := strings.TrimSuffix(models.RuntimeOutputRefKey(ref.StepName, ref.TaskName, ""), "/")
		if _, ok := declarations[producerKey][ref.OutputName]; !ok {
			return fmt.Errorf("variable %q references undeclared output %s.%s.outputs.%s", name, ref.StepName, ref.TaskName, ref.OutputName)
		}
		if !consumerDependsOnOutputProducer(consumer, ref) {
			return fmt.Errorf("variable %q consumes output %s.%s.outputs.%s without a valid dependency", name, ref.StepName, ref.TaskName, ref.OutputName)
		}
	}
	return nil
}

func consumerDependsOnOutputProducer(consumer outputRefConsumer, ref models.RuntimeOutputRef) bool {
	if consumer.stepName == ref.StepName && consumer.taskName == ref.TaskName {
		return false
	}
	for _, dep := range consumer.stepDependsOn {
		dep = strings.TrimSpace(dep)
		if dep == ref.StepName || dep == ref.StepName+"."+ref.TaskName {
			return true
		}
	}
	for _, dep := range consumer.taskDependsOn {
		dep = strings.TrimSpace(dep)
		if consumer.stepName == ref.StepName && dep == ref.TaskName {
			return true
		}
		if dep == ref.StepName+"."+ref.TaskName {
			return true
		}
	}
	return false
}

func resolvePipelineTaskDependency(depName, currentStep string, allStepNames map[string]bool, stepToTaskNames map[string]map[string]bool) (string, string, bool) {
	depName = strings.TrimSpace(depName)
	currentStep = strings.TrimSpace(currentStep)
	if depName == "" {
		return "", "", false
	}
	if stepToTaskNames[currentStep][depName] {
		return currentStep, depName, true
	}
	bestStep := ""
	bestTask := ""
	for stepName, taskNames := range stepToTaskNames {
		prefix := stepName + "."
		if !strings.HasPrefix(depName, prefix) {
			continue
		}
		taskName := strings.TrimSpace(strings.TrimPrefix(depName, prefix))
		if taskName == "" || !taskNames[taskName] {
			continue
		}
		if len(stepName) > len(bestStep) {
			bestStep = stepName
			bestTask = taskName
		}
	}
	if bestStep != "" {
		return bestStep, bestTask, true
	}
	if allStepNames[depName] {
		return depName, "", true
	}
	return "", "", false
}

func validatePolicyMergeMode(value string, location string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if !models.SupportedPolicyMergeMode(value) {
		return fmt.Errorf("policy_merge_mode in %s must be one of %q, %q, or %q",
			location,
			models.PolicyMergeModeRestrictive,
			models.PolicyMergeModeOverride,
			models.PolicyMergeModeFailOnConflict,
		)
	}
	return nil
}

func validateApprovalDefinition(approval models.ApprovalDefinition, stepName string) error {
	approvalType := strings.TrimSpace(approval.Type)
	if approvalType == "" {
		return fmt.Errorf("approval step '%s' must define approval.type", stepName)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(approvalType) {
		return fmt.Errorf("approval step '%s' approval.type can only contain alphanumeric characters, underscores, dots, and hyphens", stepName)
	}
	teams := trimmedStrings(approval.Teams)
	if len(teams) == 0 {
		return fmt.Errorf("approval step '%s' must assign at least one approval team", stepName)
	}
	seen := make(map[string]bool, len(teams))
	for _, team := range teams {
		normalized := strings.Trim(strings.ReplaceAll(team, "\\", "/"), "/")
		if normalized == "" {
			return fmt.Errorf("approval step '%s' contains an empty approval team", stepName)
		}
		if filepath.IsAbs(team) || strings.HasPrefix(strings.TrimSpace(team), "~") {
			return fmt.Errorf("approval step '%s' approval team %q must be a relative team path", stepName, team)
		}
		for _, segment := range strings.Split(normalized, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return fmt.Errorf("approval step '%s' approval team %q contains invalid path segments", stepName, team)
			}
		}
		key := strings.ToLower(normalized)
		if seen[key] {
			return fmt.Errorf("approval step '%s' repeats approval team %q", stepName, team)
		}
		seen[key] = true
	}
	return nil
}

func validatePipelineLLMSettings(pipeline *models.Pipeline) error {
	if pipeline == nil {
		return nil
	}
	if models.PipelineLLMEnabled(pipeline) {
		return nil
	}
	if len(pipeline.Output.Items) > 0 {
		return fmt.Errorf("pipeline has LLM disabled but defines final outputs")
	}
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			stepName = "unknown"
		}
		if strings.TrimSpace(step.GetCondition()) != "" {
			return fmt.Errorf("pipeline has LLM disabled but step %q defines condition", stepName)
		}
		if strings.TrimSpace(step.GetGoal()) != "" {
			return fmt.Errorf("pipeline has LLM disabled but step %q defines goal", stepName)
		}
		for _, task := range step.GetTasks() {
			taskName := task.Name
			if taskName == "" {
				taskName = "unknown"
			}
			if strings.TrimSpace(task.Goal) != "" {
				return fmt.Errorf("pipeline has LLM disabled but task %q in step %q defines goal", taskName, stepName)
			}
		}
	}
	return nil
}

func validatePipelineOutput(output models.PipelineOutput) error {
	outputProfile := strings.TrimSpace(output.LLMProfile)
	if len(output.Items) == 0 {
		if outputProfile != "" {
			return fmt.Errorf("output.llm_profile requires at least one output item")
		}
		return nil
	}
	seenNames := map[string]bool{}
	for idx, item := range output.Items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return fmt.Errorf("output.items[%d] name is required", idx)
		}
		nameKey := strings.ToLower(name)
		if seenNames[nameKey] {
			return fmt.Errorf("output item %q is defined more than once", name)
		}
		seenNames[nameKey] = true

		outputType := strings.ToLower(strings.TrimSpace(item.Type))
		if outputType == "" {
			return fmt.Errorf("output item %q type is required", name)
		}
		if _, ok := supportedPipelineOutputTypes[outputType]; !ok {
			return fmt.Errorf("output item %q has unsupported type %q", name, item.Type)
		}
		when := strings.ToLower(strings.TrimSpace(item.When))
		if when != "" {
			if _, ok := supportedPipelineOutputWhen[when]; !ok {
				return fmt.Errorf("output item %q has unsupported when %q", name, item.When)
			}
		}
		if strings.TrimSpace(item.Prompt) == "" {
			return fmt.Errorf("output item %q prompt is required", name)
		}
		if outputType == "dashboard" {
			if err := validateDashboardOutputTarget(name, item.Dashboard); err != nil {
				return err
			}
		} else if strings.TrimSpace(item.Dashboard.Ref) != "" ||
			strings.TrimSpace(item.Dashboard.Section) != "" ||
			strings.TrimSpace(item.Dashboard.EntryKey) != "" ||
			strings.TrimSpace(item.Dashboard.Mode) != "" ||
			strings.TrimSpace(item.Dashboard.Preset) != "" ||
			strings.TrimSpace(item.Dashboard.TTL) != "" {
			return fmt.Errorf("output item %q dashboard configuration requires type \"dashboard\"", name)
		}
	}
	return nil
}

func validateDashboardOutputTarget(itemName string, target models.DashboardOutputTarget) error {
	ref := strings.Trim(strings.TrimSpace(target.Ref), "/")
	if ref == "" {
		return fmt.Errorf("output item %q dashboard.ref is required", itemName)
	}
	if err := validateRelativeKnowledgePath(ref); err != nil {
		return fmt.Errorf("output item %q dashboard.ref is invalid: %w", itemName, err)
	}
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return fmt.Errorf("output item %q dashboard.ref must use team/dashboard format", itemName)
	}
	section := strings.TrimSpace(target.Section)
	if section == "" {
		return fmt.Errorf("output item %q dashboard.section is required", itemName)
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(section) {
		return fmt.Errorf("output item %q dashboard.section can only contain alphanumeric characters, underscores, dots, and hyphens", itemName)
	}
	if entryKey := strings.TrimSpace(target.EntryKey); entryKey != "" && !regexp.MustCompile(`^[a-zA-Z0-9_.:/-]+$`).MatchString(entryKey) {
		return fmt.Errorf("output item %q dashboard.entry_key can only contain alphanumeric characters, underscores, dots, colons, slashes, and hyphens", itemName)
	}
	mode := strings.ToLower(strings.TrimSpace(target.Mode))
	if _, ok := supportedDashboardOutputModes[mode]; !ok {
		return fmt.Errorf("output item %q dashboard.mode %q is not supported", itemName, target.Mode)
	}
	preset := strings.ToLower(strings.TrimSpace(target.Preset))
	if _, ok := supportedDashboardOutputPresets[preset]; !ok {
		return fmt.Errorf("output item %q dashboard.preset %q is not supported", itemName, target.Preset)
	}
	return nil
}

func validateKnowledgeContextRefs(refs []models.KnowledgeContextRef, location string) error {
	for idx, ref := range refs {
		if err := validateKnowledgeContextRef(ref); err != nil {
			return fmt.Errorf("knowledge_context[%d] in %s is invalid: %w", idx, location, err)
		}
	}
	return nil
}

func validateKnowledgeContextRef(ref models.KnowledgeContextRef) error {
	kind := strings.ToLower(strings.TrimSpace(ref.Kind))
	if kind == "" {
		return fmt.Errorf("kind is required")
	}
	if _, ok := supportedKnowledgeContextKinds[kind]; !ok {
		return fmt.Errorf("kind %q is not supported", ref.Kind)
	}
	hasRef := strings.TrimSpace(ref.Ref) != ""
	hasPath := strings.TrimSpace(ref.Path) != ""
	switch {
	case hasRef && hasPath:
		return fmt.Errorf("only one of ref or path may be set")
	case !hasRef && !hasPath:
		return fmt.Errorf("ref or path is required")
	}
	if hasRef {
		if err := validateRelativeKnowledgePath(ref.Ref); err != nil {
			return fmt.Errorf("ref: %w", err)
		}
		parts := strings.Split(strings.Trim(ref.Ref, "/"), "/")
		if len(parts) < 2 {
			return fmt.Errorf("ref must use team/document format")
		}
	}
	if hasPath {
		if err := validateRelativeKnowledgePath(ref.Path); err != nil {
			return fmt.Errorf("path: %w", err)
		}
	}
	return nil
}

func validateRelativeKnowledgePath(value string) error {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if normalized == "" {
		return fmt.Errorf("must not be empty")
	}
	if filepath.IsAbs(normalized) || strings.HasPrefix(normalized, "~") {
		return fmt.Errorf("must be relative")
	}
	normalized = strings.Trim(normalized, "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("contains invalid path segments")
		}
	}
	return nil
}

func ValidatePipelineMCPProfiles(pipeline *models.Pipeline, opts MCPProfileValidationOptions) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is required")
	}
	if !models.PipelineLLMEnabled(pipeline) {
		return nil
	}

	validateRefs := func(profileNames []string, location string) error {
		for _, rawProfileName := range profileNames {
			profileName := strings.TrimSpace(rawProfileName)
			if profileName == "" {
				return fmt.Errorf("empty MCP profile referenced by %s", location)
			}
			profile, ok := opts.Profiles[profileName]
			if !ok {
				return fmt.Errorf("MCP profile %q referenced by %s is not configured", profileName, location)
			}
			if !profile.Enabled {
				return fmt.Errorf("MCP profile %q referenced by %s is disabled", profileName, location)
			}
			if !profileAllowedInScope(profile.AllowedScopes, opts.Scope) {
				return fmt.Errorf("MCP profile %q is not allowed in scope %q", profileName, strings.TrimSpace(opts.Scope))
			}
		}
		return nil
	}

	if err := validateRefs(pipeline.MCPProfiles, "pipeline"); err != nil {
		return err
	}

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			stepName = "unknown"
		}
		stepProfiles := step.GetMCPProfiles()
		hasStepProfiles := len(trimmedStrings(stepProfiles)) > 0

		if _, ok := step.AsIncludeStep(); ok && hasStepProfiles {
			return fmt.Errorf("include step %q cannot define mcp_profiles", stepName)
		}
		if _, ok := step.AsScriptStep(); ok && hasStepProfiles {
			return fmt.Errorf("script step %q cannot define mcp_profiles", stepName)
		}
		if err := validateRefs(stepProfiles, fmt.Sprintf("step %q", stepName)); err != nil {
			return err
		}

		for _, task := range step.GetTasks() {
			taskName := task.Name
			if taskName == "" {
				taskName = "unknown"
			}
			taskProfiles := task.MCPProfiles
			hasTaskProfiles := len(trimmedStrings(taskProfiles)) > 0
			if strings.TrimSpace(task.Script) != "" && hasTaskProfiles {
				return fmt.Errorf("script task %q in step %q cannot define mcp_profiles", taskName, stepName)
			}
			if err := validateRefs(taskProfiles, fmt.Sprintf("task %q in step %q", taskName, stepName)); err != nil {
				return err
			}
		}
	}

	return nil
}

func ValidatePipelineLLMProfiles(pipeline *models.Pipeline, opts LLMProfileValidationOptions) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is required")
	}
	if !models.PipelineLLMEnabled(pipeline) {
		return nil
	}
	profiles := opts.Profiles
	if len(profiles) == 0 {
		return fmt.Errorf("no LLM profiles are configured")
	}

	defaultProfile := strings.TrimSpace(opts.DefaultProfile)
	if defaultProfile == "" && !opts.RequireDefaultProfile {
		defaultProfile = appconfig.DefaultLLMProfileName
	}
	if defaultProfile != "" {
		if _, ok := profiles[defaultProfile]; !ok {
			return fmt.Errorf("default LLM profile %q is not configured", defaultProfile)
		}
	} else if opts.RequireDefaultProfile {
		defaultProfile = ""
	} else {
		return fmt.Errorf("default LLM profile %q is not configured", defaultProfile)
	}

	validateProfile := func(profileName string, location string) error {
		profileName = strings.TrimSpace(profileName)
		if profileName == "" {
			if defaultProfile == "" {
				return fmt.Errorf("no default LLM profile is configured for %s", location)
			}
			profileName = defaultProfile
		}
		profile, ok := profiles[profileName]
		if !ok {
			return fmt.Errorf("LLM profile %q referenced by %s is not configured", profileName, location)
		}
		if !profileAllowedInScope(profile.AllowedScopes, opts.Scope) {
			return fmt.Errorf("LLM profile %q is not allowed in scope %q", profileName, strings.TrimSpace(opts.Scope))
		}
		return nil
	}

	pipelineProfile := strings.TrimSpace(pipeline.LLMProfile)
	if err := validateProfile(pipelineProfile, "pipeline"); err != nil {
		return err
	}
	if pipelineProfile == "" {
		pipelineProfile = defaultProfile
	}

	if strings.TrimSpace(pipeline.Output.LLMProfile) != "" {
		if err := validateProfile(pipeline.Output.LLMProfile, "pipeline output"); err != nil {
			return err
		}
	}
	outputProfile := firstNonEmpty(pipeline.Output.LLMProfile, pipelineProfile)
	for _, item := range pipeline.Output.Items {
		itemName := strings.TrimSpace(item.Name)
		if itemName == "" {
			itemName = "unknown"
		}
		if err := validateProfile(firstNonEmpty(item.LLMProfile, outputProfile), fmt.Sprintf("output item %q", itemName)); err != nil {
			return err
		}
	}

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			stepName = "unknown"
		}
		stepProfile := strings.TrimSpace(step.GetLLMProfile())
		if err := validateProfile(firstNonEmpty(stepProfile, pipelineProfile), fmt.Sprintf("step %q", stepName)); err != nil {
			return err
		}
		if stepProfile == "" {
			stepProfile = pipelineProfile
		}

		for _, task := range step.GetTasks() {
			taskName := task.Name
			if taskName == "" {
				taskName = "unknown"
			}
			taskProfile := strings.TrimSpace(task.LLMProfile)
			if err := validateProfile(firstNonEmpty(taskProfile, stepProfile), fmt.Sprintf("task %q in step %q", taskName, stepName)); err != nil {
				return err
			}
		}
	}

	return nil
}

func ValidatePipelineAgentProfiles(pipeline *models.Pipeline, opts AgentProfileValidationOptions) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is required")
	}
	profiles := opts.Profiles
	if len(profiles) == 0 {
		return fmt.Errorf("no agent profiles are configured")
	}

	defaultProfile := strings.TrimSpace(opts.DefaultProfile)
	if defaultProfile == "" && !opts.RequireDefaultProfile {
		defaultProfile = models.DefaultAgentProfileID
	}
	if defaultProfile != "" {
		defaultDefinition, ok := profiles[defaultProfile]
		if !ok {
			return fmt.Errorf("default agent profile %q is not configured", defaultProfile)
		}
		if !defaultDefinition.Enabled {
			return fmt.Errorf("default agent profile %q is disabled", defaultProfile)
		}
	}

	validateProfile := func(profileName string, location string) error {
		profileName = strings.TrimSpace(profileName)
		if profileName == "" {
			if defaultProfile == "" {
				return fmt.Errorf("no default agent profile is configured for %s", location)
			}
			profileName = defaultProfile
		}
		profile, ok := profiles[profileName]
		if !ok {
			return fmt.Errorf("agent profile %q referenced by %s is not configured", profileName, location)
		}
		if !profile.Enabled {
			return fmt.Errorf("agent profile %q referenced by %s is disabled", profileName, location)
		}
		return nil
	}

	pipelineProfile := strings.TrimSpace(pipeline.AgentProfile)
	if err := validateProfile(pipelineProfile, "pipeline"); err != nil {
		return err
	}
	if pipelineProfile == "" {
		pipelineProfile = defaultProfile
	}

	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			stepName = "unknown"
		}
		stepProfile := strings.TrimSpace(step.GetAgentProfile())
		if err := validateProfile(firstNonEmpty(stepProfile, pipelineProfile), fmt.Sprintf("step %q", stepName)); err != nil {
			return err
		}
	}

	return nil
}

func ResolvePipelineMCPProfiles(pipelineProfiles, stepProfiles, taskProfiles []string) []string {
	seen := map[string]bool{}
	resolved := make([]string, 0, len(pipelineProfiles)+len(stepProfiles)+len(taskProfiles))
	for _, list := range [][]string{pipelineProfiles, stepProfiles, taskProfiles} {
		for _, raw := range list {
			name := strings.TrimSpace(raw)
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			resolved = append(resolved, name)
		}
	}
	return resolved
}

func profileAllowedInScope(allowedScopes []string, scope string) bool {
	if len(allowedScopes) == 0 {
		return true
	}
	scope = strings.Trim(strings.TrimSpace(scope), "/")
	for _, allowed := range allowedScopes {
		if strings.EqualFold(strings.Trim(strings.TrimSpace(allowed), "/"), scope) {
			return true
		}
	}
	return false
}

func trimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
