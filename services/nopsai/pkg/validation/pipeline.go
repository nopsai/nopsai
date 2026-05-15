package validation

import (
	"fmt"
	"regexp"
	"strings"

	"nopsai/pkg/models"
)

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
