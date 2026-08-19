package nopsai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
	"nopsai/services/nopsai/pkg/validation"
)

const codexDevelopmentLoopRoot = "examples/codex-development-loop"

// The loop is only reproducible if every piece of it is committed: the two
// pipelines, the shared checkout step, the trigger manifests that let the
// pipelines call each other, the scope holding their settings, the access role
// they run as, the prompts, and the scripts that make every non-LLM decision.
func TestCodexDevelopmentLoopExampleLayout(t *testing.T) {
	for _, path := range []string{
		"README.md",
		"pipelines/platform/development-task-runner.yaml",
		"pipelines/platform/development-task-reviewer.yaml",
		"steps/platform/shared/dev-loop-checkout.yaml",
		"external-triggers/platform/development-task-runner.yaml",
		"external-triggers/platform/development-task-reviewer.yaml",
		"scopes/platform/dev-loop/scope.yaml",
		"access/grants.yaml",
		"runner-image/Dockerfile",
		"prompts/planning.md",
		"prompts/implementation.md",
		"prompts/review.md",
		"scripts/task-lib.sh",
		"scripts/find-next-task.sh",
		"scripts/mark-task-done.sh",
		"scripts/validate-repo.sh",
		"scripts/run-codex.sh",
		"scripts/parse-verdict.sh",
		"scripts/render-prompt.sh",
		"scripts/git-env.sh",
		"scripts/invoke-trigger.sh",
		"scripts/stage-lib.sh",
		"repo-template/development-task.md",
		"tests/run-script-tests.sh",
		"tests/run-loop-integration-test.sh",
		"tests/fake-codex",
	} {
		full := filepath.Join(codexDevelopmentLoopRoot, path)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("expected development loop artifact %q: %v", full, err)
		}
	}

	for _, script := range []string{
		"find-next-task.sh",
		"mark-task-done.sh",
		"validate-repo.sh",
		"run-codex.sh",
		"parse-verdict.sh",
		"render-prompt.sh",
		"invoke-trigger.sh",
	} {
		full := filepath.Join(codexDevelopmentLoopRoot, "scripts", script)
		info, err := os.Stat(full)
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("%s must be executable; the pipelines invoke it directly", full)
		}
	}
}

func TestCodexDevelopmentLoopPipelinesValidate(t *testing.T) {
	for _, name := range []string{"development-task-runner", "development-task-reviewer"} {
		pipeline := readCodexDevelopmentLoopPipeline(t, name)

		if err := validation.ValidatePipeline(&pipeline); err != nil {
			t.Fatalf("ValidatePipeline(%s) error = %v", name, err)
		}
		if pipeline.Name != name {
			t.Fatalf("pipeline %s declares name %q; the file name and pipeline name must match", name, pipeline.Name)
		}
		// Which task runs next, whether the planner stayed in its lane, and
		// whether the loop continues are all script decisions. Leaving the LLM
		// enabled would let a model reach the pipeline's control flow.
		if models.PipelineLLMEnabled(&pipeline) {
			t.Fatalf("pipeline %s must keep llm_enabled false; the loop's control flow is deterministic", name)
		}
	}
}

// Every variable a step reads has to be declared on the pipeline, or the run
// silently sees an empty value and takes a default nobody chose.
func TestCodexDevelopmentLoopDeclaresTheVariablesItReads(t *testing.T) {
	sharedStep := readCodexDevelopmentLoopFile(t, filepath.Join("steps", "platform", "shared", "dev-loop-checkout.yaml"))

	for _, tc := range []struct {
		pipeline string
		required []string
	}{
		{
			pipeline: "development-task-runner",
			required: []string{
				"DEV_LOOP_REPOSITORY_URL",
				"DEV_LOOP_BASE_BRANCH",
				"DEV_LOOP_TASK_FILE",
				"DEV_LOOP_AGENTS_FILE",
				"DEV_LOOP_TOOLKIT_DIR",
				"DEV_LOOP_PLAN_DIR",
				"DEV_LOOP_BRANCH_PREFIX",
				"DEV_LOOP_ALLOW_EXISTING_BRANCH",
				"DEV_LOOP_API_URL",
				"DEV_LOOP_REVIEWER_TRIGGER_ID",
			},
		},
		{
			pipeline: "development-task-reviewer",
			required: []string{
				"DEV_LOOP_REPOSITORY_URL",
				"DEV_LOOP_BASE_BRANCH",
				"DEV_LOOP_CHECKOUT_REF",
				"DEV_LOOP_TASK_BRANCH",
				"DEV_LOOP_TASK_ID",
				"DEV_LOOP_TASK_NUMBER",
				"DEV_LOOP_TASK_TITLE",
				"DEV_LOOP_TASK_SLUG",
				"DEV_LOOP_TASK_PLAN_PATH",
				"DEV_LOOP_COMMIT_SHA",
				"DEV_LOOP_REVIEW_DIR",
				"DEV_LOOP_ON_PASS",
				"DEV_LOOP_VALIDATE_COMMAND",
				"DEV_LOOP_API_URL",
				"DEV_LOOP_RUNNER_TRIGGER_ID",
			},
		},
	} {
		pipeline := readCodexDevelopmentLoopPipeline(t, tc.pipeline)
		declared := map[string]bool{}
		for _, variable := range pipeline.Variables {
			declared[strings.TrimSpace(variable)] = true
		}
		for _, want := range tc.required {
			if !declared[want] {
				t.Fatalf("pipeline %s must declare variable %q", tc.pipeline, want)
			}
		}
	}

	// The shared checkout step reads these from whichever pipeline includes it.
	for _, want := range []string{"DEV_LOOP_REPOSITORY_URL", "DEV_LOOP_CHECKOUT_REF", "DEV_LOOP_BASE_BRANCH"} {
		if !strings.Contains(sharedStep, want) {
			t.Fatalf("shared checkout step should read %q", want)
		}
	}
}

// Secrets are declared per step so a step only ever holds the credentials it
// needs: the planner never sees the API token, the loop caller never sees the
// Git token.
func TestCodexDevelopmentLoopScopesSecretsToTheStepsThatNeedThem(t *testing.T) {
	for _, tc := range []struct {
		pipeline string
		step     string
		secrets  []string
	}{
		{"development-task-runner", "plan-task", []string{"DEV_LOOP_CODEX_API_KEY", "DEV_LOOP_GIT_TOKEN"}},
		{"development-task-runner", "implement-task", []string{"DEV_LOOP_CODEX_API_KEY", "DEV_LOOP_GIT_TOKEN"}},
		{"development-task-runner", "trigger-review", []string{"DEV_LOOP_NOPSAI_TOKEN"}},
		{"development-task-reviewer", "codex-review", []string{"DEV_LOOP_CODEX_API_KEY"}},
		{"development-task-reviewer", "record-review", []string{"DEV_LOOP_GIT_TOKEN"}},
		{"development-task-reviewer", "promote-task-state", []string{"DEV_LOOP_GIT_TOKEN"}},
		{"development-task-reviewer", "continue-loop", []string{"DEV_LOOP_NOPSAI_TOKEN"}},
	} {
		pipeline := readCodexDevelopmentLoopPipeline(t, tc.pipeline)
		step := codexDevelopmentLoopStep(t, pipeline, tc.step)
		declared := map[string]bool{}
		for _, secret := range step.GetSecrets() {
			declared[strings.TrimSpace(secret)] = true
		}
		for _, want := range tc.secrets {
			if !declared[want] {
				t.Fatalf("step %q in %s must declare secret %q", tc.step, tc.pipeline, want)
			}
		}
	}

	// The review stage reads code and judges it. Handing it a push credential
	// would make "the reviewer does not modify the branch" a convention rather
	// than a property of the run.
	reviewer := readCodexDevelopmentLoopPipeline(t, "development-task-reviewer")
	for _, secret := range codexDevelopmentLoopStep(t, reviewer, "codex-review").GetSecrets() {
		if strings.TrimSpace(secret) == "DEV_LOOP_GIT_TOKEN" {
			t.Fatal("the codex-review step must not receive a Git push credential")
		}
	}
}

// The runner hands the reviewer everything it needs through the trigger, and
// the reviewer hands the next round back. A field missing from a mapping would
// surface only as an empty variable at run time.
func TestCodexDevelopmentLoopTriggersCarryTheLoopState(t *testing.T) {
	runner := readCodexDevelopmentLoopExternalTrigger(t, "development-task-runner")
	if runner.Pipeline != "platform/development-task-runner" {
		t.Fatalf("runner trigger targets %q", runner.Pipeline)
	}
	for name, source := range map[string]string{
		"DEV_LOOP_REPOSITORY_URL": "payload.repository_url",
		"DEV_LOOP_BASE_BRANCH":    "payload.base_branch",
	} {
		if runner.VariableMapping[name] != source {
			t.Fatalf("runner trigger should map %s from %s, got %q", name, source, runner.VariableMapping[name])
		}
	}

	reviewer := readCodexDevelopmentLoopExternalTrigger(t, "development-task-reviewer")
	if reviewer.Pipeline != "platform/development-task-reviewer" {
		t.Fatalf("reviewer trigger targets %q", reviewer.Pipeline)
	}
	for name, source := range map[string]string{
		"DEV_LOOP_REPOSITORY_URL": "payload.repository_url",
		"DEV_LOOP_BASE_BRANCH":    "payload.base_branch",
		"DEV_LOOP_TASK_BRANCH":    "payload.task_branch",
		// The shared checkout step checks out whatever CHECKOUT_REF names, so
		// the task branch has to arrive under both names.
		"DEV_LOOP_CHECKOUT_REF":   "payload.task_branch",
		"DEV_LOOP_TASK_ID":        "payload.task_id",
		"DEV_LOOP_TASK_NUMBER":    "payload.task_number",
		"DEV_LOOP_TASK_TITLE":     "payload.task_title",
		"DEV_LOOP_TASK_SLUG":      "payload.task_slug",
		"DEV_LOOP_TASK_PLAN_PATH": "payload.plan_path",
		"DEV_LOOP_COMMIT_SHA":     "payload.commit_sha",
	} {
		if reviewer.VariableMapping[name] != source {
			t.Fatalf("reviewer trigger should map %s from %s, got %q", name, source, reviewer.VariableMapping[name])
		}
	}

	// Anonymous invocation would let anything on the network drive the loop.
	for name, trigger := range map[string]codexDevelopmentLoopTriggerDocument{
		"development-task-runner":   runner,
		"development-task-reviewer": reviewer,
	} {
		if len(trigger.AllowedCallers) == 0 {
			t.Fatalf("trigger %s must restrict its allowed callers", name)
		}
	}
}

// The scope is where an operator configures the loop, so it has to name every
// setting and every credential the pipelines expect to find.
func TestCodexDevelopmentLoopScopeDeclaresSettingsAndSecrets(t *testing.T) {
	var scope struct {
		Variables map[string]string `yaml:"variables"`
		Secrets   map[string]any    `yaml:"secrets"`
	}
	raw := readCodexDevelopmentLoopFile(t, filepath.Join("scopes", "platform", "dev-loop", "scope.yaml"))
	if err := yaml.Unmarshal([]byte(raw), &scope); err != nil {
		t.Fatalf("yaml.Unmarshal(scope) error = %v", err)
	}

	for _, name := range []string{
		"DEV_LOOP_REPOSITORY_URL",
		"DEV_LOOP_BASE_BRANCH",
		"DEV_LOOP_TASK_FILE",
		"DEV_LOOP_AGENTS_FILE",
		"DEV_LOOP_PLAN_DIR",
		"DEV_LOOP_REVIEW_DIR",
		"DEV_LOOP_BRANCH_PREFIX",
		"DEV_LOOP_TOOLKIT_DIR",
		"DEV_LOOP_ON_PASS",
		"DEV_LOOP_API_URL",
		"DEV_LOOP_RUNNER_TRIGGER_ID",
		"DEV_LOOP_REVIEWER_TRIGGER_ID",
	} {
		if _, ok := scope.Variables[name]; !ok {
			t.Fatalf("dev-loop scope should set %q", name)
		}
	}

	for _, name := range []string{"DEV_LOOP_GIT_TOKEN", "DEV_LOOP_CODEX_API_KEY", "DEV_LOOP_NOPSAI_TOKEN"} {
		if _, ok := scope.Secrets[name]; !ok {
			t.Fatalf("dev-loop scope should declare secret %q", name)
		}
	}

	// Only encrypted envelopes belong in Git, so a scope declares secret keys
	// and never their values.
	for name, value := range scope.Secrets {
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			t.Fatalf("scope secret %q carries an inline value; secrets resolve from the credential store", name)
		}
	}
}

// The review prompt teaches the model one exact verdict line and parse-verdict.sh
// accepts only that line. If the two drift apart, every review fails - or worse,
// a malformed one passes.
func TestCodexDevelopmentLoopVerdictContractIsShared(t *testing.T) {
	prompt := readCodexDevelopmentLoopFile(t, filepath.Join("prompts", "review.md"))
	parser := readCodexDevelopmentLoopFile(t, filepath.Join("scripts", "parse-verdict.sh"))

	for _, verdict := range []string{"VERDICT: PASS", "VERDICT: FAIL"} {
		if !strings.Contains(prompt, verdict) {
			t.Fatalf("the review prompt must specify the exact %q line", verdict)
		}
	}
	if !strings.Contains(parser, "VERDICT:") {
		t.Fatal("parse-verdict.sh must match on the VERDICT: line the review prompt specifies")
	}
}

// The pipelines are deliberately thin: each step calls one stage script, so the
// logic that decides what happens can be tested end to end without a running
// platform. This keeps the two halves from drifting - a renamed stage script
// would otherwise fail only at run time.
func TestCodexDevelopmentLoopStepsCallTheirStageScripts(t *testing.T) {
	for pipelineName, steps := range map[string]map[string]string{
		"development-task-runner": {
			"select-task":        "stage-select-task.sh",
			"create-task-branch": "stage-create-branch.sh",
			"plan-task":          "stage-plan.sh",
			"implement-task":     "stage-implement.sh",
			"trigger-review":     "stage-trigger-review.sh",
		},
		"development-task-reviewer": {
			"collect-evidence":    "stage-collect-evidence.sh",
			"validate-repository": "stage-validate.sh",
			"codex-review":        "stage-review.sh",
			"record-review":       "stage-record-review.sh",
			"promote-task-state":  "stage-promote-state.sh",
			"continue-loop":       "stage-continue-loop.sh",
		},
	} {
		pipeline := readCodexDevelopmentLoopPipeline(t, pipelineName)
		for stepName, stageScript := range steps {
			step := codexDevelopmentLoopStep(t, pipeline, stepName)
			scriptStep, ok := step.AsScriptStep()
			if !ok {
				t.Fatalf("step %q in %s should be a script step", stepName, pipelineName)
			}
			if !strings.Contains(scriptStep.Script, stageScript) {
				t.Fatalf("step %q in %s should call %s", stepName, pipelineName, stageScript)
			}

			path := filepath.Join(codexDevelopmentLoopRoot, "scripts", stageScript)
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat %s: %v", path, err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Fatalf("%s must be executable; a pipeline step runs it directly", path)
			}
		}
	}
}

// An example nothing links to is an example nobody finds, so the indexes and
// the wiki are held to mentioning this one.
func TestCodexDevelopmentLoopDocsPointToTheExample(t *testing.T) {
	for path, required := range map[string][]string{
		"examples/README.md": {"codex-development-loop/"},
		"Readme.md":          {"examples/codex-development-loop/README.md"},
		"doc/README.md":      {"../examples/codex-development-loop/README.md"},
		"doc/wiki":           {"examples/codex-development-loop", "llm_enabled: false"},
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, want := range required {
			if !strings.Contains(string(contents), want) {
				t.Fatalf("%s should mention %q", path, want)
			}
		}
	}
}

func readCodexDevelopmentLoopFile(t *testing.T, relative string) string {
	t.Helper()
	path := filepath.Join(codexDevelopmentLoopRoot, relative)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func readCodexDevelopmentLoopPipeline(t *testing.T, name string) models.Pipeline {
	t.Helper()
	raw := readCodexDevelopmentLoopFile(t, filepath.Join("pipelines", "platform", name+".yaml"))
	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(raw), &pipeline); err != nil {
		t.Fatalf("yaml.Unmarshal(%s) error = %v", name, err)
	}
	return pipeline
}

type codexDevelopmentLoopTriggerDocument struct {
	ID              string            `yaml:"id"`
	Pipeline        string            `yaml:"pipeline"`
	Scope           string            `yaml:"scope"`
	RunTeamPath     string            `yaml:"run_team_path"`
	AllowedCallers  []map[string]any  `yaml:"allowed_callers"`
	VariableMapping map[string]string `yaml:"variable_mapping"`
}

func readCodexDevelopmentLoopExternalTrigger(t *testing.T, name string) codexDevelopmentLoopTriggerDocument {
	t.Helper()
	raw := readCodexDevelopmentLoopFile(t, filepath.Join("external-triggers", "platform", name+".yaml"))
	var trigger codexDevelopmentLoopTriggerDocument
	if err := yaml.Unmarshal([]byte(raw), &trigger); err != nil {
		t.Fatalf("yaml.Unmarshal(%s trigger) error = %v", name, err)
	}
	return trigger
}

func codexDevelopmentLoopStep(t *testing.T, pipeline models.Pipeline, name string) models.Step {
	t.Helper()
	for _, step := range pipeline.Steps {
		if step.GetName() == name {
			return step.Step
		}
	}
	t.Fatalf("pipeline %s has no step named %q", pipeline.Name, name)
	return nil
}
