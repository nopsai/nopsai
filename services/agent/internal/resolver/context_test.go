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

func TestMaskRuntimeTextMasksSensitiveValuesOnly(t *testing.T) {
	context := NewExecutionContext()
	context.SetValue("VISIBLE_VAR", "plain-runtime-value", false)
	context.SetValue("ENVIRONMENT", "production", false)
	context.SetValue("CHANGE_ID", "Nothing-so-far", false)
	context.SetValue("APPLICATION_NAME", "dont ask me", false)
	context.SetValue("IMAGE_REFERENCE", "not me", false)
	context.SetValue("SERVICE", `{"change_id":"Nothing-so-far","application":"dont ask me","image_reference":"not me","environment":"production","strategy":"rolling-update"}`, false)
	context.SetValue("STEP_SECRET", "super-secret-value", true)
	context.SetValue("API_TOKEN", "token-by-name", false)

	if got := context.MaskText("value=plain-runtime-value secret=super-secret-value", nil); !strings.Contains(got, "plain-runtime-value") {
		t.Fatalf("MaskText should leave non-sensitive prompt values visible, got %q", got)
	}

	output := `value=plain-runtime-value {"change_id":"Nothing-so-far","application":"dont ask me","image_reference":"not me","environment":"production","strategy":"rolling-update"} secret=super-secret-value API_TOKEN=token-by-name external=external-secret`
	masked := context.MaskRuntimeText(output, map[string]string{"EXTERNAL_SECRET": "external-secret"})
	for _, want := range []string{
		"plain-runtime-value",
		`"change_id":"Nothing-so-far"`,
		`"application":"dont ask me"`,
		`"image_reference":"not me"`,
		`"environment":"production"`,
		`"strategy":"rolling-update"`,
	} {
		if !strings.Contains(masked, want) {
			t.Fatalf("expected non-sensitive runtime value %q to remain visible, got %q", want, masked)
		}
	}
	for _, forbidden := range []string{"super-secret-value", "token-by-name", "external-secret"} {
		if strings.Contains(masked, forbidden) {
			t.Fatalf("expected sensitive value %q to be masked, got %q", forbidden, masked)
		}
	}
	if strings.Count(masked, "*****") != 3 {
		t.Fatalf("masked output = %q, want exactly three sensitive markers", masked)
	}
}

func TestMaskRuntimeTextMasksFlattenedMultilineSecrets(t *testing.T) {
	context := NewExecutionContext()
	context.SetValue("PRIVATE_KEY", "line-one\nline-two", true)

	masked := context.MaskRuntimeText("pem=line-one line-two", nil)
	if strings.Contains(masked, "line-one") || strings.Contains(masked, "line-two") {
		t.Fatalf("expected flattened multiline secret to be masked, got %q", masked)
	}
	if !strings.Contains(masked, "*****") {
		t.Fatalf("expected masking marker, got %q", masked)
	}
}

func TestMaskRuntimeTextKeepsNonSensitiveRuntimeOutputValueVisible(t *testing.T) {
	context := NewExecutionContext()
	output := `{"change_id":"Nothing-so-far","application":"dont ask me","image_reference":"not me","environment":"production","namespace":"","strategy":"rolling-update"}`
	context.SetValue("SERVICE", output, false)

	if got := context.MaskRuntimeText(output, nil); got != output {
		t.Fatalf("MaskRuntimeText() = %q, want non-sensitive runtime output unchanged", got)
	}
}

func TestMaskRuntimeTextMasksSensitiveRuntimeOutputValue(t *testing.T) {
	context := NewExecutionContext()
	output := `{"token":"runtime-output-secret"}`
	context.SetValue("SERVICE_TOKEN", output, true)

	masked := context.MaskRuntimeText(output, nil)
	if strings.Contains(masked, output) || strings.Contains(masked, "runtime-output-secret") {
		t.Fatalf("expected sensitive runtime output to be masked, got %q", masked)
	}
	if masked != "*****" {
		t.Fatalf("masked output = %q, want full sensitive output marker", masked)
	}
}

func TestMaskRuntimeTextDoesNotMaskShortNonSensitiveValues(t *testing.T) {
	context := NewExecutionContext()
	context.SetValue("ENVIRONMENT", "prod", false)
	context.SetValue("FLAG", "true", false)

	masked := context.MaskRuntimeText("ENVIRONMENT=prod FLAG=true production_ready", nil)
	for _, want := range []string{"ENVIRONMENT=prod", "FLAG=true", "production_ready"} {
		if !strings.Contains(masked, want) {
			t.Fatalf("expected %q to remain visible, got %q", want, masked)
		}
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

func TestSelectedVariableOverridesReturnsRawValuesAndSensitiveNames(t *testing.T) {
	context := NewExecutionContext()
	context.SetValue("CHANNEL", "stable", false)
	context.SetValue("TOKEN", "secret", true)
	context.SetValue("API_KEY", "literal-sensitive-by-name", false)

	variables, sensitive := context.SelectedVariableOverrides(map[string]string{
		"CHANNEL": "ignored",
		"TOKEN":   "ignored",
		"API_KEY": "ignored",
	})

	if variables["CHANNEL"] != "stable" || variables["TOKEN"] != "secret" || variables["API_KEY"] != "literal-sensitive-by-name" {
		t.Fatalf("variables = %#v, want raw runtime values", variables)
	}
	if len(sensitive) != 2 || sensitive[0] != "API_KEY" || sensitive[1] != "TOKEN" {
		t.Fatalf("sensitive = %#v, want API_KEY and TOKEN", sensitive)
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
