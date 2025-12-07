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
