package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestPrepareSecretsForPipelineRejectsConflictingScopedRuntimeNames(t *testing.T) {
	app := &App{}
	pipeline := models.Pipeline{
		Steps: []models.PipelineStep{
			{
				Step: &models.ScriptStep{
					BaseStep: models.BaseStep{
						Name:    "deploy",
						Secrets: []string{"dev:API_TOKEN", "prod:API_TOKEN"},
					},
					Script: "env",
				},
			},
		},
	}

	_, err := app.prepareSecretsForPipeline("", pipeline, nil, "dev")
	if err == nil {
		t.Fatal("prepareSecretsForPipeline() error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "runtime name 'API_TOKEN'") {
		t.Fatalf("prepareSecretsForPipeline() error = %q, want runtime name conflict", err)
	}
}

func TestPrepareVariablesForPipelineRejectsConflictingScopedRuntimeNames(t *testing.T) {
	app := &App{}
	pipeline := models.Pipeline{
		Variables: []string{"dev:TEST_ENV", "prod:TEST_ENV"},
	}

	_, err := app.prepareVariablesForPipeline("", pipeline, nil, "dev", nil)
	if err == nil {
		t.Fatal("prepareVariablesForPipeline() error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), "runtime name 'TEST_ENV'") {
		t.Fatalf("prepareVariablesForPipeline() error = %q, want runtime name conflict", err)
	}
}
