package validation

import (
	"fmt"
	"strings"
	"testing"

	"nopsai/pkg/models"

	"gopkg.in/yaml.v3"
)

func TestValidatePipeline_Valid(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks: []models.Task{
						{Name: "task1", Script: "echo 'hello'"},
					},
				},
			},
		},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Errorf("expected valid pipeline, got error: %v", err)
	}
}

func TestValidatePipelineAllowsScriptOnlyWhenLLMDisabled(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		LLMEnabled:     &disabled,
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks: []models.Task{
						{Name: "task1", Script: "echo 'hello'"},
					},
				},
			},
		},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("expected LLM-disabled script pipeline to validate, got %v", err)
	}
}

func TestValidatePipelineRejectsInvalidPolicyMergeMode(t *testing.T) {
	p := &models.Pipeline{
		Name:            "invalid-policy-mode",
		ContainerImage:  "ubuntu:latest",
		PolicyMergeMode: "loose",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
				},
			},
		},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "policy_merge_mode") {
		t.Fatalf("ValidatePipeline() error = %v, want policy_merge_mode error", err)
	}
}

func TestValidatePipelineRejectsTooManySteps(t *testing.T) {
	p := &models.Pipeline{
		Name:           "too-many-steps",
		ContainerImage: "alpine:latest",
		Steps:          make([]models.PipelineStep, maxPipelineSteps+1),
	}
	for i := range p.Steps {
		p.Steps[i] = models.PipelineStep{Step: &models.ScriptStep{
			BaseStep: models.BaseStep{Name: fmt.Sprintf("step-%03d", i)},
			Script:   "echo ok",
		}}
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "maximum is") {
		t.Fatalf("ValidatePipeline() error = %v, want step limit error", err)
	}
}

func TestValidatePipelineRejectsTooManyTasksInStep(t *testing.T) {
	tasks := make([]models.Task, maxPipelineTasksPerStep+1)
	for i := range tasks {
		tasks[i] = models.Task{Name: fmt.Sprintf("task-%03d", i), Script: "echo ok"}
	}
	p := &models.Pipeline{
		Name:           "too-many-tasks",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{{Step: &models.TaskStep{
			BaseStep: models.BaseStep{Name: "build"},
			Tasks:    tasks,
		}}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "has 257 tasks") {
		t.Fatalf("ValidatePipeline() error = %v, want task limit error", err)
	}
}

func TestValidatePipelineRejectsTooManyDependenciesOnNode(t *testing.T) {
	deps := make([]string, maxPipelineDependenciesPerNode+1)
	for i := range deps {
		deps[i] = fmt.Sprintf("producer-%03d", i)
	}
	p := &models.Pipeline{
		Name:           "too-many-dependencies",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{{Step: &models.ScriptStep{
			BaseStep: models.BaseStep{Name: "build", DependsOn: deps},
			Script:   "echo ok",
		}}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "dependencies; maximum") {
		t.Fatalf("ValidatePipeline() error = %v, want dependency limit error", err)
	}
}

func TestValidatePipelineRejectsTooManyVolumesOnStep(t *testing.T) {
	volumes := make([]string, maxPipelineVolumesPerStep+1)
	for i := range volumes {
		volumes[i] = fmt.Sprintf("cache-%03d:/cache-%03d", i, i)
	}
	p := &models.Pipeline{
		Name:           "too-many-volumes",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{{Step: &models.ScriptStep{
			BaseStep: models.BaseStep{Name: "build", Volumes: volumes},
			Script:   "echo ok",
		}}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "volumes; maximum") {
		t.Fatalf("ValidatePipeline() error = %v, want volume limit error", err)
	}
}

func TestValidatePipelineAllowsTaskVariablesFromYAML(t *testing.T) {
	const raw = `
name: main-pipeline
description: A sample pipeline.
container_image: "hoseindocker/pipeline-image:latest"
variables:
  - DOCKER_HOST
  - API_VERSION
  - default:FROM_OTHER
  - data-team/dev:DEVVAR
steps:
  - name: preparation
    tasks:
      - name: list-envs-containers
        variables:
          API_VERSION: "inside task overwrite"
        script: |
          env
`
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		t.Fatalf("yaml.Unmarshal() error = %v", err)
	}
	if err := ValidatePipeline(&pipeline); err != nil {
		t.Fatalf("ValidatePipeline() error = %v", err)
	}
	tasks := pipeline.Steps[0].GetTasks()
	if got := tasks[0].Variables["API_VERSION"]; got != "inside task overwrite" {
		t.Fatalf("task variable API_VERSION = %q, want inside task overwrite", got)
	}
}

func TestValidatePipelineRejectsInvalidVariableDeclarations(t *testing.T) {
	p := &models.Pipeline{
		Name:           "invalid-variable-ref",
		ContainerImage: "alpine:latest",
		Variables:      []string{"team-1/dev:API_VERSION", "bad/name"},
		Steps: []models.PipelineStep{{
			Step: &models.ScriptStep{
				BaseStep: models.BaseStep{Name: "run"},
				Script:   "echo ok",
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "variables[1]") {
		t.Fatalf("ValidatePipeline() error = %v, want invalid pipeline variable declaration", err)
	}
}

func TestValidatePipelineRejectsDuplicateRuntimeVariableDeclarations(t *testing.T) {
	p := &models.Pipeline{
		Name:           "duplicate-variable-ref",
		ContainerImage: "alpine:latest",
		Variables:      []string{"default:API_VERSION", "team-1/prod:API_VERSION"},
		Steps: []models.PipelineStep{{
			Step: &models.ScriptStep{
				BaseStep: models.BaseStep{Name: "run"},
				Script:   "echo ok",
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "declares runtime variable") {
		t.Fatalf("ValidatePipeline() error = %v, want duplicate runtime variable declaration", err)
	}
}

func TestValidatePipelineRuntimeOutputsAndQualifiedDependencies(t *testing.T) {
	p := &models.Pipeline{
		Name:           "runtime-output-pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "prepare"},
				Tasks: []models.Task{{
					Name:    "generate-tag",
					Script:  "printf %s v1 > /nopsai/outputs/image_tag",
					Outputs: []models.TaskOutput{{Name: "image_tag"}, {Name: "IMAGE_TAG"}},
				}},
			}},
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "build"},
				Tasks: []models.Task{{
					Name:      "image",
					DependsOn: []string{"prepare.generate-tag"},
					Variables: map[string]string{
						"image_tag": "$steps.prepare.generate-tag.outputs.image_tag",
					},
					Script: "echo build",
				}},
			}},
		},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("ValidatePipeline() error = %v", err)
	}
}

func TestValidatePipelineAllowsLegacyStepRuntimeOutputs(t *testing.T) {
	p := &models.Pipeline{
		Name:           "legacy-output-pipeline",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:    "prepare",
					Outputs: []models.TaskOutput{{Name: "release_manifest"}},
				},
				Script: "printf manifest > /nopsai/outputs/release_manifest",
			}},
			{Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:      "consume",
					DependsOn: []string{"prepare"},
					Variables: map[string]string{
						"RELEASE_MANIFEST": "$steps.prepare.prepare.outputs.release_manifest",
					},
				},
				Script: "echo consume",
			}},
		},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("ValidatePipeline() error = %v", err)
	}
}

func TestValidatePipelineRejectsInvalidStepVariableAndSecretNames(t *testing.T) {
	p := &models.Pipeline{
		Name:           "invalid-step-vars",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{{
			Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:      "run",
					Secrets:   []string{"team-1/prod:DEPLOY_TOKEN", "bad/name"},
					Variables: map[string]string{"BAD/NAME": "value"},
				},
				Script: "echo ok",
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "BAD/NAME") {
		t.Fatalf("ValidatePipeline() error = %v, want invalid step variable name", err)
	}

	p.Steps[0].SetVariables(map[string]string{"GOOD_NAME": "value"})
	err = ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "bad/name") {
		t.Fatalf("ValidatePipeline() error = %v, want invalid scoped secret name", err)
	}
}

func TestValidateReusableStepUsesPipelineYAMLRules(t *testing.T) {
	valid := models.PipelineStep{Step: &models.ScriptStep{
		BaseStep: models.BaseStep{
			Name:      "checkout",
			Variables: map[string]string{"BRANCH": "main"},
			Outputs:   []models.TaskOutput{{Name: "commit_sha"}},
		},
		Script: "git rev-parse HEAD > /nopsai/outputs/commit_sha",
	}}
	if err := ValidateReusableStep(&valid); err != nil {
		t.Fatalf("ValidateReusableStep() error = %v", err)
	}

	invalid := models.PipelineStep{Step: &models.ScriptStep{
		BaseStep: models.BaseStep{
			Name:      "checkout",
			Variables: map[string]string{"BAD/NAME": "main"},
		},
		Script: "echo ok",
	}}
	err := ValidateReusableStep(&invalid)
	if err == nil || !strings.Contains(err.Error(), "BAD/NAME") {
		t.Fatalf("ValidateReusableStep() error = %v, want invalid variable name", err)
	}
}

func TestValidatePipelineRejectsReservedRuntimeOutputVolume(t *testing.T) {
	p := &models.Pipeline{
		Name:           "reserved-output-volume",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{{
			Step: &models.ScriptStep{
				BaseStep: models.BaseStep{
					Name:    "build",
					Volumes: []string{"tmp:/nopsai/outputs"},
				},
				Script: "echo ok",
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "reserved runtime output path") {
		t.Fatalf("ValidatePipeline() error = %v, want reserved mount error", err)
	}
}

func TestValidatePipelineRejectsDuplicateTaskOutputNamesExactly(t *testing.T) {
	p := &models.Pipeline{
		Name:           "duplicate-output",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "prepare"},
				Tasks: []models.Task{{
					Name:    "generate",
					Script:  "echo ok",
					Outputs: []models.TaskOutput{{Name: "image_tag"}, {Name: "IMAGE_TAG"}, {Name: "image_tag"}},
				}},
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), `output "image_tag"`) {
		t.Fatalf("ValidatePipeline() error = %v, want duplicate output error", err)
	}
}

func TestValidatePipelineRejectsRuntimeOutputWithoutDependency(t *testing.T) {
	p := &models.Pipeline{
		Name:           "missing-output-dependency",
		ContainerImage: "alpine:latest",
		Steps: []models.PipelineStep{
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "prepare"},
				Tasks: []models.Task{{
					Name:    "generate",
					Script:  "echo ok",
					Outputs: []models.TaskOutput{{Name: "image_tag"}},
				}},
			}},
			{Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "build"},
				Tasks: []models.Task{{
					Name: "image",
					Variables: map[string]string{
						"image_tag": "$steps.prepare.generate.outputs.image_tag",
					},
					Script: "echo build",
				}},
			}},
		},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "without a valid dependency") {
		t.Fatalf("ValidatePipeline() error = %v, want missing dependency error", err)
	}
}

func TestValidatePipelineRejectsDependencyCycles(t *testing.T) {
	tests := []struct {
		name     string
		pipeline models.Pipeline
	}{
		{
			name: "step cycle",
			pipeline: models.Pipeline{
				Name:           "step-cycle",
				ContainerImage: "alpine:latest",
				Steps: []models.PipelineStep{
					{Step: &models.ScriptStep{
						BaseStep: models.BaseStep{Name: "build", DependsOn: []string{"deploy"}},
						Script:   "echo build",
					}},
					{Step: &models.ScriptStep{
						BaseStep: models.BaseStep{Name: "deploy", DependsOn: []string{"build"}},
						Script:   "echo deploy",
					}},
				},
			},
		},
		{
			name: "task cycle",
			pipeline: models.Pipeline{
				Name:           "task-cycle",
				ContainerImage: "alpine:latest",
				Steps: []models.PipelineStep{{
					Step: &models.TaskStep{
						BaseStep: models.BaseStep{Name: "test"},
						Tasks: []models.Task{
							{Name: "unit", DependsOn: []string{"integration"}, Script: "echo unit"},
							{Name: "integration", DependsOn: []string{"unit"}, Script: "echo integration"},
						},
					},
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePipeline(&tt.pipeline)
			if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
				t.Fatalf("ValidatePipeline() error = %v, want dependency cycle error", err)
			}
		})
	}
}

func TestValidatePipelineLLMEnabledHelperUsesPipelineFlag(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		LLMEnabled:     &disabled,
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Script:   "echo 'hello'",
				},
			},
		},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("expected llm_enabled=false to validate for script-only pipeline, got %v", err)
	}
	if models.PipelineLLMEnabled(p) {
		t.Fatal("expected llm_enabled=false to disable LLM")
	}
}

func TestValidatePipelineRejectsGoalWhenLLMDisabled(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		LLMEnabled:     &disabled,
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks: []models.Task{
						{Name: "task1", Goal: "Summarize"},
					},
				},
			},
		},
	}

	err := ValidatePipeline(p)
	if err == nil {
		t.Fatal("expected error for LLM-disabled goal task")
	}
	if !strings.Contains(err.Error(), `pipeline has LLM disabled but task "task1" in step "step1" defines goal`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineFinalOutputs(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		Output: models.PipelineOutput{
			LLMProfile: "report-writer",
			Items: []models.PipelineOutputItem{{
				Name:   "Executive summary",
				Type:   "markdown",
				Prompt: "Summarize the run for management.",
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "step1"},
				Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
			},
		}},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("expected final outputs to validate, got %v", err)
	}
}

func TestValidatePipelineDashboardFinalOutput(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		Output: models.PipelineOutput{
			Items: []models.PipelineOutputItem{{
				Name:   "Dashboard summary",
				Type:   "dashboard",
				Prompt: "Summarize the run.",
				Dashboard: models.DashboardOutputTarget{
					Ref:      "platform/ops",
					Section:  "overview",
					EntryKey: "daily",
					Mode:     "replace",
					Preset:   "status",
					TTL:      "24h",
				},
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "step1"},
				Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
			},
		}},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("expected dashboard final output to validate, got %v", err)
	}
	p.Output.Items[0].Dashboard.Mode = "snapshot"
	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("expected snapshot dashboard output to validate, got %v", err)
	}
}

func TestValidatePipelineDashboardFinalOutputRequiresTarget(t *testing.T) {
	p := &models.Pipeline{
		Name:           "invalid-pipeline",
		ContainerImage: "ubuntu:latest",
		Output: models.PipelineOutput{
			Items: []models.PipelineOutputItem{{
				Name:   "Dashboard summary",
				Type:   "dashboard",
				Prompt: "Summarize the run.",
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "step1"},
				Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "dashboard.ref is required") {
		t.Fatalf("ValidatePipeline() error = %v, want dashboard.ref error", err)
	}
}

func TestValidatePipelineRejectsDashboardConfigOnOtherTypes(t *testing.T) {
	p := &models.Pipeline{
		Name:           "invalid-pipeline",
		ContainerImage: "ubuntu:latest",
		Output: models.PipelineOutput{
			Items: []models.PipelineOutputItem{{
				Name:   "Summary",
				Type:   "markdown",
				Prompt: "Summarize.",
				Dashboard: models.DashboardOutputTarget{
					Ref:     "platform/ops",
					Section: "overview",
				},
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "step1"},
				Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), `dashboard configuration requires type "dashboard"`) {
		t.Fatalf("ValidatePipeline() error = %v, want dashboard config error", err)
	}
}

func TestValidatePipelineFinalOutputsRejectUnsupportedType(t *testing.T) {
	p := &models.Pipeline{
		Name:           "invalid-pipeline",
		ContainerImage: "ubuntu:latest",
		Output: models.PipelineOutput{
			Items: []models.PipelineOutputItem{{
				Name:   "Archive",
				Type:   "zip",
				Prompt: "Package everything.",
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "step1"},
				Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), `unsupported type "zip"`) {
		t.Fatalf("ValidatePipeline() error = %v, want unsupported type", err)
	}
}

func TestValidatePipelineFinalOutputsRejectUnsupportedWhen(t *testing.T) {
	p := &models.Pipeline{
		Name:           "test-pipeline",
		ContainerImage: "alpine:3.20",
		Output: models.PipelineOutput{Items: []models.PipelineOutputItem{{
			Name:   "Summary",
			Type:   "markdown",
			When:   "manual",
			Prompt: "Summarize the run.",
		}}},
		Steps: []models.PipelineStep{
			{Step: &models.ScriptStep{BaseStep: models.BaseStep{Name: "build"}, Script: "echo ok"}},
		},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "unsupported when") {
		t.Fatalf("ValidatePipeline() error = %v, want unsupported when", err)
	}
}

func TestValidatePipelineRejectsFinalOutputsWhenLLMDisabled(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "invalid-pipeline",
		ContainerImage: "ubuntu:latest",
		LLMEnabled:     &disabled,
		Output: models.PipelineOutput{
			Items: []models.PipelineOutputItem{{
				Name:   "Summary",
				Type:   "markdown",
				Prompt: "Summarize.",
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "step1"},
				Tasks:    []models.Task{{Name: "task1", Script: "echo ok"}},
			},
		}},
	}

	err := ValidatePipeline(p)
	if err == nil || !strings.Contains(err.Error(), "defines final outputs") {
		t.Fatalf("ValidatePipeline() error = %v, want LLM-disabled output error", err)
	}
}

func TestValidatePipelineRejectsConditionWhenLLMDisabled(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "valid-pipeline",
		ContainerImage: "ubuntu:latest",
		LLMEnabled:     &disabled,
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{Name: "step1", Condition: "run tests?"},
					Script:   "echo 'hello'",
				},
			},
		},
	}

	err := ValidatePipeline(p)
	if err == nil {
		t.Fatal("expected error for LLM-disabled condition")
	}
	if !strings.Contains(err.Error(), `pipeline has LLM disabled but step "step1" defines condition`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipeline_InvalidName(t *testing.T) {
	p := &models.Pipeline{
		Name: "Invalid Name With Spaces",
	}
	err := ValidatePipeline(p)
	if err == nil {
		t.Error("expected error for invalid name")
	}
	if !strings.Contains(err.Error(), "alphanumeric") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidatePipeline_MissingImage(t *testing.T) {
	p := &models.Pipeline{
		Name: "valid-name",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks: []models.Task{
						{Name: "task1", Script: "echo 'hello'"},
					},
				},
			},
		},
	}
	// ContainerImage is empty and steps don't have images
	err := ValidatePipeline(p)
	if err == nil {
		t.Error("expected error for missing container image")
	}
}

func TestValidatePipelineAllowsApprovalStepWithoutImage(t *testing.T) {
	p := &models.Pipeline{
		Name: "approval-pipeline",
		Steps: []models.PipelineStep{
			{
				Step: &models.ApprovalStep{
					BaseStep: models.BaseStep{Name: "prod-gate"},
					Approval: models.ApprovalDefinition{
						Type:  "production-deploy",
						Teams: []string{"platform/prod"},
					},
				},
			},
		},
	}

	if err := ValidatePipeline(p); err != nil {
		t.Fatalf("expected approval-only pipeline to validate without image, got %v", err)
	}
}

func TestValidatePipelineRejectsInvalidApprovalDefinition(t *testing.T) {
	tests := []struct {
		name     string
		approval models.ApprovalDefinition
		want     string
	}{
		{name: "missing type", approval: models.ApprovalDefinition{Teams: []string{"platform/prod"}}, want: "must define approval.type"},
		{name: "invalid type", approval: models.ApprovalDefinition{Type: "prod deploy", Teams: []string{"platform/prod"}}, want: "approval.type can only contain"},
		{name: "missing teams", approval: models.ApprovalDefinition{Type: "prod-deploy"}, want: "must assign at least one approval team"},
		{name: "absolute team", approval: models.ApprovalDefinition{Type: "prod-deploy", Teams: []string{"/platform/prod"}}, want: "must be a relative team path"},
		{name: "escaping team", approval: models.ApprovalDefinition{Type: "prod-deploy", Teams: []string{"../prod"}}, want: "contains invalid path segments"},
		{name: "duplicate team", approval: models.ApprovalDefinition{Type: "prod-deploy", Teams: []string{"platform/prod", "platform/prod"}}, want: "repeats approval team"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &models.Pipeline{
				Name: "approval-pipeline",
				Steps: []models.PipelineStep{
					{
						Step: &models.ApprovalStep{
							BaseStep: models.BaseStep{Name: "prod-gate"},
							Approval: tt.approval,
						},
					},
				},
			}
			err := ValidatePipeline(p)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidatePipeline_DuplicateStep(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks:    []models.Task{{Name: "t1", Script: "ls"}},
				},
			},
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"}, // Duplicate
					Tasks:    []models.Task{{Name: "t2", Script: "ls"}},
				},
			},
		},
	}

	err := ValidatePipeline(p)
	if err == nil {
		t.Error("expected error for duplicate step name")
	}
}

func TestValidatePipeline_InvalidWorkingDirectory(t *testing.T) {
	p := &models.Pipeline{
		Name:             "valid-name",
		ContainerImage:   "ubuntu",
		WorkingDirectory: "/",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "step1"},
					Tasks:    []models.Task{{Name: "t1", Script: "ls"}},
				},
			},
		},
	}

	err := ValidatePipeline(p)
	if err == nil {
		t.Fatal("expected error for invalid working_directory")
	}
	if !strings.Contains(err.Error(), "working_directory") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestValidatePipelineLLMProfilesInheritanceAndScope(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		LLMProfile:     "reasoning",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "quick", LLMProfile: "fast"},
					Tasks: []models.Task{
						{Name: "summarize", Goal: "Summarize"},
					},
				},
			},
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "deep"},
					Tasks: []models.Task{
						{Name: "review", Goal: "Review"},
					},
				},
			},
		},
	}

	opts := LLMProfileValidationOptions{
		DefaultProfile: "standard",
		Scope:          "prod",
		Profiles: map[string]LLMProfileDefinition{
			"standard":  {},
			"fast":      {AllowedScopes: []string{"dev", "test", "prod"}},
			"reasoning": {AllowedScopes: []string{"dev", "internal"}},
		},
	}

	err := ValidatePipelineLLMProfiles(p, opts)
	if err == nil {
		t.Fatal("expected scope validation error")
	}
	if !strings.Contains(err.Error(), `LLM profile "reasoning" is not allowed in scope "prod"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineLLMProfilesTaskOverrideAllowed(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		LLMProfile:     "standard",
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "deep"},
					Tasks: []models.Task{
						{Name: "review", Goal: "Review", LLMProfile: "reasoning"},
					},
				},
			},
		},
	}

	err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{
		DefaultProfile: "standard",
		Scope:          "dev",
		Profiles: map[string]LLMProfileDefinition{
			"standard":  {AllowedScopes: []string{"dev", "test", "prod"}},
			"reasoning": {AllowedScopes: []string{"dev", "internal"}},
		},
	})
	if err != nil {
		t.Fatalf("expected task profile to be allowed in dev, got %v", err)
	}
}

func TestValidatePipelineLLMProfilesFinalOutputOverride(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		LLMProfile:     "standard",
		Output: models.PipelineOutput{
			LLMProfile: "report-writer",
			Items: []models.PipelineOutputItem{{
				Name:   "Executive summary",
				Type:   "markdown",
				Prompt: "Summarize.",
			}},
		},
		Steps: []models.PipelineStep{{
			Step: &models.TaskStep{
				BaseStep: models.BaseStep{Name: "deep"},
				Tasks:    []models.Task{{Name: "review", Goal: "Review"}},
			},
		}},
	}

	err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{
		DefaultProfile: "standard",
		Scope:          "prod",
		Profiles: map[string]LLMProfileDefinition{
			"standard":      {AllowedScopes: []string{"prod"}},
			"report-writer": {AllowedScopes: []string{"dev"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `LLM profile "report-writer" is not allowed in scope "prod"`) {
		t.Fatalf("ValidatePipelineLLMProfiles() error = %v, want output profile scope error", err)
	}
}

func TestValidatePipelineLLMProfilesUnknown(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.GoalStep{
					BaseStep: models.BaseStep{Name: "review", LLMProfile: "missing"},
					Goal:     "Review",
				},
			},
		},
	}

	err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{
		DefaultProfile: "standard",
		Profiles: map[string]LLMProfileDefinition{
			"standard": {},
		},
	})
	if err == nil {
		t.Fatal("expected unknown profile error")
	}
	if !strings.Contains(err.Error(), `LLM profile "missing" referenced by step "review" is not configured`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineLLMProfilesSkippedForScriptOnlyPipelineWithoutProfiles(t *testing.T) {
	p := &models.Pipeline{
		Name:           "script-only",
		ContainerImage: "ubuntu",
		LLMProfile:     "standard",
		Steps: []models.PipelineStep{{
			Step: &models.ScriptStep{
				BaseStep: models.BaseStep{Name: "build"},
				Script:   "go test ./...",
			},
		}},
	}

	if err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{}); err != nil {
		t.Fatalf("ValidatePipelineLLMProfiles() error = %v, want nil", err)
	}
}

func TestValidatePipelineLLMProfilesRequiresProfilesForGoalPipeline(t *testing.T) {
	p := &models.Pipeline{
		Name:           "goal",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{{
			Step: &models.GoalStep{
				BaseStep: models.BaseStep{Name: "review"},
				Goal:     "Review the change.",
			},
		}},
	}

	err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{})
	if err == nil || !strings.Contains(err.Error(), "no LLM profiles are configured") {
		t.Fatalf("ValidatePipelineLLMProfiles() error = %v, want missing profiles error", err)
	}
}

func TestValidatePipelineLLMProfilesSkippedWhenLLMDisabled(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		LLMEnabled:     &disabled,
		LLMProfile:     "missing",
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{Name: "build", LLMProfile: "missing-too"},
					Script:   "go test ./...",
				},
			},
		},
	}

	err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{})
	if err != nil {
		t.Fatalf("expected LLM validation to be skipped, got %v", err)
	}
}

func TestValidatePipelineLLMProfilesRequiresTeamDefaultOnlyWhenNeeded(t *testing.T) {
	p := &models.Pipeline{
		Name:           "team-default",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{{
			Step: &models.GoalStep{
				BaseStep: models.BaseStep{Name: "review"},
				Goal:     "Review",
			},
		}},
	}

	err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{
		RequireDefaultProfile: true,
		Profiles: map[string]LLMProfileDefinition{
			"team-reasoning": {},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "no default LLM profile is configured") {
		t.Fatalf("ValidatePipelineLLMProfiles() error = %v, want missing team default error", err)
	}

	p.LLMProfile = "team-reasoning"
	if err := ValidatePipelineLLMProfiles(p, LLMProfileValidationOptions{
		RequireDefaultProfile: true,
		Profiles: map[string]LLMProfileDefinition{
			"team-reasoning": {},
		},
	}); err != nil {
		t.Fatalf("ValidatePipelineLLMProfiles() with explicit profile error = %v", err)
	}
}

func TestValidatePipelineAgentProfilesInheritance(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		AgentProfile:   "sre",
		Steps: []models.PipelineStep{
			{
				Step: &models.GoalStep{
					BaseStep: models.BaseStep{Name: "pipeline-profile"},
					Goal:     "Review reliability",
				},
			},
			{
				Step: &models.GoalStep{
					BaseStep: models.BaseStep{Name: "step-profile", AgentProfile: "security-engineer"},
					Goal:     "Review security",
				},
			},
		},
	}

	err := ValidatePipelineAgentProfiles(p, AgentProfileValidationOptions{
		DefaultProfile: models.DefaultAgentProfileID,
		Profiles: map[string]AgentProfileDefinition{
			models.DefaultAgentProfileID: {Enabled: true},
			"sre":                        {Enabled: true},
			"security-engineer":          {Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("expected agent profiles to validate, got %v", err)
	}
}

func TestValidatePipelineAgentProfilesRejectsUnknownAndDisabled(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.GoalStep{
					BaseStep: models.BaseStep{Name: "review", AgentProfile: "disabled"},
					Goal:     "Review",
				},
			},
		},
	}

	err := ValidatePipelineAgentProfiles(p, AgentProfileValidationOptions{
		DefaultProfile: models.DefaultAgentProfileID,
		Profiles: map[string]AgentProfileDefinition{
			models.DefaultAgentProfileID: {Enabled: true},
			"disabled":                   {Enabled: false},
		},
	})
	if err == nil {
		t.Fatal("expected disabled profile validation error")
	}
	if !strings.Contains(err.Error(), `agent profile "disabled" referenced by step "review" is disabled`) {
		t.Fatalf("unexpected error: %v", err)
	}

	p.Steps[0].SetAgentProfile("missing")
	err = ValidatePipelineAgentProfiles(p, AgentProfileValidationOptions{
		DefaultProfile: models.DefaultAgentProfileID,
		Profiles: map[string]AgentProfileDefinition{
			models.DefaultAgentProfileID: {Enabled: true},
		},
	})
	if err == nil {
		t.Fatal("expected unknown profile validation error")
	}
	if !strings.Contains(err.Error(), `agent profile "missing" referenced by step "review" is not configured`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineAgentProfilesRequiresEnabledDefault(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{Name: "build"},
					Script:   "go test ./...",
				},
			},
		},
	}

	err := ValidatePipelineAgentProfiles(p, AgentProfileValidationOptions{
		DefaultProfile: models.DefaultAgentProfileID,
		Profiles: map[string]AgentProfileDefinition{
			models.DefaultAgentProfileID: {Enabled: false},
		},
	})
	if err == nil {
		t.Fatal("expected disabled default profile validation error")
	}
	if !strings.Contains(err.Error(), `default agent profile "devops-engineer" is disabled`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineAgentProfilesRequiresTeamDefaultOnlyWhenNeeded(t *testing.T) {
	p := &models.Pipeline{
		Name:           "team-agent-default",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{{
			Step: &models.GoalStep{
				BaseStep: models.BaseStep{Name: "review"},
				Goal:     "Review",
			},
		}},
	}

	err := ValidatePipelineAgentProfiles(p, AgentProfileValidationOptions{
		RequireDefaultProfile: true,
		Profiles: map[string]AgentProfileDefinition{
			"team-agent": {Enabled: true},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "no default agent profile is configured") {
		t.Fatalf("ValidatePipelineAgentProfiles() error = %v, want missing team default error", err)
	}

	p.AgentProfile = "team-agent"
	if err := ValidatePipelineAgentProfiles(p, AgentProfileValidationOptions{
		RequireDefaultProfile: true,
		Profiles: map[string]AgentProfileDefinition{
			"team-agent": {Enabled: true},
		},
	}); err != nil {
		t.Fatalf("ValidatePipelineAgentProfiles() with explicit profile error = %v", err)
	}
}

func TestValidatePipelineMCPProfilesAdditiveGoalUsage(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		MCPProfiles:    []string{"github-readonly"},
		Steps: []models.PipelineStep{
			{
				Step: &models.TaskStep{
					BaseStep: models.BaseStep{Name: "review", MCPProfiles: []string{"jira-readonly"}},
					Tasks: []models.Task{
						{Name: "inspect", Goal: "Review", MCPProfiles: []string{"slack-readonly"}},
						{Name: "test", Script: "go test ./..."},
					},
				},
			},
		},
	}

	err := ValidatePipelineMCPProfiles(p, MCPProfileValidationOptions{
		Scope: "prod",
		Profiles: map[string]MCPProfileDefinition{
			"github-readonly": {Enabled: true, AllowedScopes: []string{"prod"}},
			"jira-readonly":   {Enabled: true},
			"slack-readonly":  {Enabled: true},
		},
	})
	if err != nil {
		t.Fatalf("expected MCP profiles to validate, got %v", err)
	}

	resolved := ResolvePipelineMCPProfiles(p.MCPProfiles, p.Steps[0].GetMCPProfiles(), p.Steps[0].GetTasks()[0].MCPProfiles)
	want := []string{"github-readonly", "jira-readonly", "slack-readonly"}
	if strings.Join(resolved, ",") != strings.Join(want, ",") {
		t.Fatalf("resolved MCP profiles = %v, want %v", resolved, want)
	}
}

func TestValidatePipelineMCPProfilesSkippedWhenLLMDisabled(t *testing.T) {
	disabled := false
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		LLMEnabled:     &disabled,
		MCPProfiles:    []string{"missing"},
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{Name: "test", MCPProfiles: []string{"missing-too"}},
					Script:   "go test ./...",
				},
			},
		},
	}

	err := ValidatePipelineMCPProfiles(p, MCPProfileValidationOptions{})
	if err != nil {
		t.Fatalf("expected MCP validation to be skipped, got %v", err)
	}
}

func TestValidatePipelineMCPProfilesRejectsScriptExplicitUse(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{Name: "test", MCPProfiles: []string{"github-readonly"}},
					Script:   "go test ./...",
				},
			},
		},
	}

	err := ValidatePipelineMCPProfiles(p, MCPProfileValidationOptions{
		Profiles: map[string]MCPProfileDefinition{"github-readonly": {Enabled: true}},
	})
	if err == nil {
		t.Fatal("expected script MCP profile validation error")
	}
	if !strings.Contains(err.Error(), `script step "test" cannot define mcp_profiles`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineMCPProfilesRejectsIncludeExplicitUse(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.IncludeStep{
					BaseStep: models.BaseStep{Name: "reuse", MCPProfiles: []string{"github-readonly"}},
					Include:  "step:shared/review",
				},
			},
		},
	}

	err := ValidatePipelineMCPProfiles(p, MCPProfileValidationOptions{
		Profiles: map[string]MCPProfileDefinition{"github-readonly": {Enabled: true}},
	})
	if err == nil {
		t.Fatal("expected include MCP profile validation error")
	}
	if !strings.Contains(err.Error(), `include step "reuse" cannot define mcp_profiles`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePipelineMCPProfilesRejectsUnknownAndScope(t *testing.T) {
	p := &models.Pipeline{
		Name:           "valid-name",
		ContainerImage: "ubuntu",
		Steps: []models.PipelineStep{
			{
				Step: &models.GoalStep{
					BaseStep: models.BaseStep{Name: "review", MCPProfiles: []string{"github-readonly"}},
					Goal:     "Review",
				},
			},
		},
	}

	err := ValidatePipelineMCPProfiles(p, MCPProfileValidationOptions{
		Scope:    "prod",
		Profiles: map[string]MCPProfileDefinition{"github-readonly": {Enabled: true, AllowedScopes: []string{"dev"}}},
	})
	if err == nil {
		t.Fatal("expected scope error")
	}
	if !strings.Contains(err.Error(), `MCP profile "github-readonly" is not allowed in scope "prod"`) {
		t.Fatalf("unexpected error: %v", err)
	}

	err = ValidatePipelineMCPProfiles(p, MCPProfileValidationOptions{})
	if err == nil {
		t.Fatal("expected unknown profile error")
	}
	if !strings.Contains(err.Error(), `MCP profile "github-readonly" referenced by step "review" is not configured`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
