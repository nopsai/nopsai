package resolver

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestBuildStepExecutionContextRedactsSecretsForPrompt(t *testing.T) {
	pipeline := &models.Pipeline{
		Variables: []string{"GLOBAL_VAR", "API_TOKEN"},
	}
	step := &models.PipelineStep{
		Step: &models.TaskStep{
			BaseStep: models.BaseStep{
				Variables: map[string]string{"STEP_FLAG": "enabled"},
				Secrets:   []string{"STEP_SECRET"},
			},
		},
	}

	context, missing := BuildStepContext(
		pipeline,
		step,
		[]string{"GIT_REPO_NAME=demo", "SCOPE=prod"},
		map[string]string{"GLOBAL_VAR": "plain", "API_TOKEN": "pipeline-token"},
		map[string]string{"STEP_SECRET": "super-secret-value"},
	)
	if len(missing) != 0 {
		t.Fatalf("expected no missing secrets, got %v", missing)
	}

	promptVariables := context.PromptVariables()
	if got := promptVariables["STEP_SECRET"]; got != "[redacted]" {
		t.Fatalf("promptVariables[STEP_SECRET] = %q, want [redacted]", got)
	}
	if got := promptVariables["API_TOKEN"]; got != "[redacted]" {
		t.Fatalf("promptVariables[API_TOKEN] = %q, want [redacted]", got)
	}

	runtimeDump := strings.Join(context.ContainerVariables(), "\n")
	for _, expected := range []string{
		"GLOBAL_VAR=plain",
		"API_TOKEN=pipeline-token",
		"STEP_FLAG=enabled",
		"STEP_SECRET=super-secret-value",
		"GIT_REPO_NAME=demo",
		"SCOPE=prod",
	} {
		if !strings.Contains(runtimeDump, expected) {
			t.Fatalf("expected container variables to contain %q, got %s", expected, runtimeDump)
		}
	}
}

func TestBuildActionRequestMasksSensitiveHistoryAndDirectoryContent(t *testing.T) {
	context := NewExecutionContext()
	context.SetValue("STEP_SECRET", "super-secret-value", true)
	context.SetValue("VISIBLE_VAR", "plain", false)

	req := context.BuildActionRequest(
		"deploy",
		"token is super-secret-value",
		map[string]string{"README.md": "secret: super-secret-value"},
		"guardrail says do not print super-secret-value",
		map[string]string{"ANOTHER_SECRET": "another-secret"},
	)

	if strings.Contains(req.GetHistory(), "super-secret-value") {
		t.Fatalf("expected history to be masked, got %q", req.GetHistory())
	}
	if strings.Contains(req.GetDirectoryListing()["README.md"], "super-secret-value") {
		t.Fatalf("expected directory listing to be masked, got %q", req.GetDirectoryListing()["README.md"])
	}
	if strings.Contains(req.GetKnowledgeContext(), "super-secret-value") {
		t.Fatalf("expected knowledge context to be masked, got %q", req.GetKnowledgeContext())
	}
	if got := req.GetVariables()["STEP_SECRET"]; got != "[redacted]" {
		t.Fatalf("expected prompt variable to be redacted, got %q", got)
	}
	if got := req.GetVariables()["VISIBLE_VAR"]; got != "plain" {
		t.Fatalf("expected visible prompt variable, got %q", got)
	}
}

func TestTaskExecutionContextTaskOverridesWin(t *testing.T) {
	base := NewExecutionContext()
	base.SetValue("SHARED", "step", false)
	base.SetValue("SAFE_VALUE", "safe", false)

	task := &models.Task{
		Variables: map[string]string{
			"SHARED": "task",
		},
	}
	context := base.WithTask(task)

	runtimeDump := strings.Join(context.ContainerVariables(), "\n")
	if strings.Contains(runtimeDump, "SHARED=step") {
		t.Fatalf("expected step value to be replaced, got %s", runtimeDump)
	}
	if !strings.Contains(runtimeDump, "SHARED=task") {
		t.Fatalf("expected task override in runtime variables, got %s", runtimeDump)
	}
}

func TestExecutionContextZeroValueCanBePopulated(t *testing.T) {
	var context ExecutionContext
	context.SetValue("SAFE_VALUE", "safe", false)

	if got := context.PromptVariables()["SAFE_VALUE"]; got != "safe" {
		t.Fatalf("prompt variable = %q, want safe", got)
	}
}

func TestBuildStepExecutionContextInjectsScopedRuntimeRefsByName(t *testing.T) {
	pipeline := &models.Pipeline{
		Variables: []string{"dev:TEST_ENV"},
	}
	step := &models.PipelineStep{
		Step: &models.ScriptStep{
			BaseStep: models.BaseStep{
				Secrets: []string{"prod:API_TOKEN"},
			},
			Script: "env",
		},
	}

	context, missing := BuildStepContext(
		pipeline,
		step,
		nil,
		map[string]string{"dev:TEST_ENV": "from-dev"},
		map[string]string{"prod:API_TOKEN": "from-prod"},
	)
	if len(missing) != 0 {
		t.Fatalf("expected no missing secrets, got %v", missing)
	}

	runtimeDump := strings.Join(context.ContainerVariables(), "\n")
	for _, expected := range []string{
		"TEST_ENV=from-dev",
		"API_TOKEN=from-prod",
	} {
		if !strings.Contains(runtimeDump, expected) {
			t.Fatalf("expected container variables to contain %q, got %s", expected, runtimeDump)
		}
	}
	if strings.Contains(runtimeDump, "dev:TEST_ENV=") || strings.Contains(runtimeDump, "prod:API_TOKEN=") {
		t.Fatalf("expected scoped references to inject bare runtime names, got %s", runtimeDump)
	}
}
