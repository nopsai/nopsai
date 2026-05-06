package main

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

	context, missing := buildStepExecutionContext(
		pipeline,
		step,
		[]string{"GIT_REPO_NAME=demo", "SCOPE=prod"},
		map[string]string{"GLOBAL_VAR": "plain", "API_TOKEN": "pipeline-token"},
		map[string]string{"STEP_SECRET": "super-secret-value"},
	)
	if len(missing) != 0 {
		t.Fatalf("expected no missing secrets, got %v", missing)
	}

	promptVariables := context.promptVariables()
	if got := promptVariables["STEP_SECRET"]; got != "[redacted]" {
		t.Fatalf("promptVariables[STEP_SECRET] = %q, want [redacted]", got)
	}
	if got := promptVariables["API_TOKEN"]; got != "[redacted]" {
		t.Fatalf("promptVariables[API_TOKEN] = %q, want [redacted]", got)
	}

	env := strings.Join(context.containerEnv(), "\n")
	for _, expected := range []string{
		"GLOBAL_VAR=plain",
		"API_TOKEN=pipeline-token",
		"STEP_FLAG=enabled",
		"STEP_SECRET=super-secret-value",
		"GIT_REPO_NAME=demo",
		"SCOPE=prod",
	} {
		if !strings.Contains(env, expected) {
			t.Fatalf("expected container env to contain %q, got %s", expected, env)
		}
	}
}

func TestBuildActionRequestMasksSensitiveHistoryAndDirectoryContent(t *testing.T) {
	context := newTaskExecutionContext()
	context.set("STEP_SECRET", "super-secret-value", taskEnvironmentSourceSecret)
	context.set("VISIBLE_VAR", "plain", taskEnvironmentSourceTaskVariable)

	req := context.buildActionRequest(
		"deploy",
		"token is super-secret-value",
		map[string]string{"README.md": "secret: super-secret-value"},
		map[string]string{"ANOTHER_SECRET": "another-secret"},
	)

	if strings.Contains(req.GetHistory(), "super-secret-value") {
		t.Fatalf("expected history to be masked, got %q", req.GetHistory())
	}
	if strings.Contains(req.GetDirectoryListing()["README.md"], "super-secret-value") {
		t.Fatalf("expected directory listing to be masked, got %q", req.GetDirectoryListing()["README.md"])
	}
	if got := req.GetVariables()["STEP_SECRET"]; got != "[redacted]" {
		t.Fatalf("expected prompt variable to be redacted, got %q", got)
	}
	if got := req.GetVariables()["VISIBLE_VAR"]; got != "plain" {
		t.Fatalf("expected visible prompt variable, got %q", got)
	}
}

func TestTaskExecutionContextTaskOverridesWin(t *testing.T) {
	base := newTaskExecutionContext()
	base.set("SHARED", "step", taskEnvironmentSourceStepVariable)
	base.set("SAFE_VALUE", "safe", taskEnvironmentSourceStepVariable)

	task := &models.Task{
		Variables: map[string]string{
			"SHARED": "task",
		},
	}
	context := base.withTask(task)

	env := strings.Join(context.containerEnv(), "\n")
	if strings.Contains(env, "SHARED=step") {
		t.Fatalf("expected step value to be replaced, got %s", env)
	}
	if !strings.Contains(env, "SHARED=task") {
		t.Fatalf("expected task override in env, got %s", env)
	}
}
