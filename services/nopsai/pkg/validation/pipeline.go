package validation

import (
	"fmt"
	"regexp"
	"strings"

	appconfig "nopsai/config"
	"nopsai/pkg/models"
)

type LLMProfileDefinition struct {
	AllowedScopes []string
}

type LLMProfileValidationOptions struct {
	DefaultProfile string
	Profiles       map[string]LLMProfileDefinition
	Scope          string
}

type MCPProfileDefinition struct {
	Enabled       bool
	AllowedScopes []string
}

type MCPProfileValidationOptions struct {
	Profiles map[string]MCPProfileDefinition
	Scope    string
}

func ValidatePipeline(pipeline *models.Pipeline) error {
	if pipeline.Name == "" {
		return fmt.Errorf("'name' is a required field")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(pipeline.Name) {
		return fmt.Errorf("pipeline name can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if pipeline.Version == "" {
		pipeline.Version = "latest"
	} else if !regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`).MatchString(pipeline.Version) {
		return fmt.Errorf("pipeline version can only contain alphanumeric characters, underscores, dots, and hyphens")
	}
	if pipeline.ContainerImage == "" && len(pipeline.Steps) > 0 && pipeline.Steps[0].GetImage() == "" {
		return fmt.Errorf("'container_image' is a required field if steps don't have their own image")
	}
	if _, err := models.NormalizePipelineWorkingDirectory(pipeline.WorkingDirectory); err != nil {
		return err
	}
	if len(pipeline.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	allStepNames := make(map[string]bool)
	stepToTaskNames := make(map[string]map[string]bool)

	// First pass: Collect all step and task names
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		if stepName == "" {
			return fmt.Errorf("a step is missing its required 'name' field")
		}
		if allStepNames[stepName] {
			return fmt.Errorf("duplicate step name '%s' found. Step names must be unique", stepName)
		}
		allStepNames[stepName] = true
		stepToTaskNames[stepName] = make(map[string]bool)

		isIncludeStep := step.GetInclude() != ""
		tasks := step.GetTasks()
		isTaskStep := len(tasks) > 0
		isLegacyStep := strings.TrimSpace(step.GetGoal()) != "" || strings.TrimSpace(step.GetScript()) != ""

		if isIncludeStep {
			if isTaskStep || isLegacyStep {
				return fmt.Errorf("step '%s' is an 'include' step and cannot also contain 'tasks', 'goal', or 'script'", stepName)
			}
		} else if isTaskStep {
			if isLegacyStep {
				return fmt.Errorf("step '%s' has tasks and should not also contain 'goal' or 'script'", stepName)
			}
			for _, task := range tasks {
				if task.Name == "" {
					return fmt.Errorf("a task in step '%s' is missing its required 'name' field", stepName)
				}
				if stepToTaskNames[stepName][task.Name] {
					return fmt.Errorf("duplicate task name '%s' found within step '%s'. Task names must be unique within a step", task.Name, stepName)
				}
				stepToTaskNames[stepName][task.Name] = true

				hasGoal := strings.TrimSpace(task.Goal) != ""
				hasScript := strings.TrimSpace(task.Script) != ""
				if hasGoal && hasScript {
					return fmt.Errorf("task '%s' in step '%s' cannot define both 'goal' and 'script'", task.Name, stepName)
				}
				if !hasGoal && !hasScript {
					return fmt.Errorf("task '%s' in step '%s' must define either 'goal' or 'script'", task.Name, stepName)
				}
			}
		} else if !isLegacyStep {
			return fmt.Errorf("step '%s' must contain 'include', 'tasks', 'goal', or 'script'", stepName)
		}
	}

	// Second pass: Validate dependencies based on the corrected rules
	for _, step := range pipeline.Steps {
		stepName := step.GetName()
		// Rule: A step can only depend on other steps.
		for _, depName := range step.GetDependsOn() {
			if !allStepNames[depName] {
				return fmt.Errorf("step '%s' has an undefined dependency: '%s'", stepName, depName)
			}
		}

		// Rule: If a step has tasks, those tasks can only depend on other tasks within the SAME step.
		tasks := step.GetTasks()
		if len(tasks) > 0 {
			for _, task := range tasks {
				for _, depName := range task.DependsOn {
					if !stepToTaskNames[stepName][depName] {
						return fmt.Errorf("task '%s' in step '%s' has an invalid dependency: '%s'. Tasks can only depend on other tasks within the same step", task.Name, stepName, depName)
					}
				}
			}
		}
	}

	return nil
}

func ValidatePipelineMCPProfiles(pipeline *models.Pipeline, opts MCPProfileValidationOptions) error {
	if pipeline == nil {
		return fmt.Errorf("pipeline is required")
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
	profiles := opts.Profiles
	if len(profiles) == 0 {
		return fmt.Errorf("no LLM profiles are configured")
	}

	defaultProfile := strings.TrimSpace(opts.DefaultProfile)
	if defaultProfile == "" {
		defaultProfile = appconfig.DefaultLLMProfileName
	}
	if _, ok := profiles[defaultProfile]; !ok {
		return fmt.Errorf("default LLM profile %q is not configured", defaultProfile)
	}

	validateProfile := func(profileName string, location string) error {
		profileName = strings.TrimSpace(profileName)
		if profileName == "" {
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
