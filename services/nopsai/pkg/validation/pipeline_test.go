package validation

import (
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
