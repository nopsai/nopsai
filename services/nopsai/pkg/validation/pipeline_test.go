package validation

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
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
