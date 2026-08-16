package perf

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

// fixturePath resolves a harness fixture relative to the repository root.
func fixturePath(elements ...string) string {
	return filepath.Join(append([]string{"..", "..", "test", "perf", "fixtures"}, elements...)...)
}

// TestProbePipelineParsesWithThePlatformParser guards the fixture against
// pipeline schema drift. The fixture is only useful if the platform can
// actually run it, so it is validated with the same parser the server uses
// rather than being trusted by eye.
func TestProbePipelineParsesWithThePlatformParser(t *testing.T) {
	raw, err := os.ReadFile(fixturePath("pipelines", "perf-load-probe.yaml"))
	if err != nil {
		t.Fatalf("read probe pipeline: %v", err)
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(raw, &pipeline); err != nil {
		t.Fatalf("the probe pipeline no longer parses: %v", err)
	}

	if pipeline.Name != "perf-load-probe" {
		t.Errorf("Name = %q, want perf-load-probe", pipeline.Name)
	}
	if pipeline.ContainerImage == "" {
		t.Error("the probe pipeline must pin a container image")
	}
	if len(pipeline.Steps) == 0 {
		t.Fatal("the probe pipeline has no steps")
	}
}

// TestProbePipelineCostsNothingToRun is the property that makes the fixture
// suitable for load testing: a measurement that invokes a model would be neither
// free nor repeatable, and its duration would be set by the provider rather than
// by the platform.
func TestProbePipelineCostsNothingToRun(t *testing.T) {
	raw, err := os.ReadFile(fixturePath("pipelines", "perf-load-probe.yaml"))
	if err != nil {
		t.Fatalf("read probe pipeline: %v", err)
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(raw, &pipeline); err != nil {
		t.Fatalf("parse probe pipeline: %v", err)
	}

	if pipeline.LLMEnabled == nil || *pipeline.LLMEnabled {
		t.Error("llm_enabled must be explicitly false so runs incur no model cost")
	}
	for _, step := range pipeline.Steps {
		script, ok := step.Step.AsScriptStep()
		if !ok {
			t.Errorf("step %q is not a script step, so it would invoke the agent", step.Step.GetName())
			continue
		}
		if script.Script == "" {
			t.Errorf("step %q has an empty script", script.Name)
		}
	}
}

// gitOpsExternalTrigger mirrors the fields the harness depends on in the trigger
// fixture. It is intentionally minimal: the test asserts the contract between
// the fixture and the harness, not the platform's full trigger schema.
type gitOpsExternalTrigger struct {
	ID              string            `yaml:"id"`
	Enabled         bool              `yaml:"enabled"`
	Pipeline        string            `yaml:"pipeline"`
	RunTeamPath     string            `yaml:"run_team_path"`
	AllowedCallers  []map[string]any  `yaml:"allowed_callers"`
	VariableMapping map[string]string `yaml:"variable_mapping"`
}

// TestProbeTriggerMatchesTheHarnessDefaults keeps the fixture and the tool from
// drifting apart: the harness invokes a trigger id by default, and the fixture
// has to be the thing that answers to it.
func TestProbeTriggerMatchesTheHarnessDefaults(t *testing.T) {
	raw, err := os.ReadFile(fixturePath("external-triggers", "perf-load-probe.yaml"))
	if err != nil {
		t.Fatalf("read probe trigger: %v", err)
	}
	var trigger gitOpsExternalTrigger
	if err := yaml.Unmarshal(raw, &trigger); err != nil {
		t.Fatalf("the probe trigger no longer parses: %v", err)
	}

	if trigger.ID != DefaultExternalTriggerID {
		t.Errorf("trigger id = %q, but the harness invokes %q by default", trigger.ID, DefaultExternalTriggerID)
	}
	if !trigger.Enabled {
		t.Error("the probe trigger must be enabled or the harness cannot invoke it")
	}
	if trigger.Pipeline == "" {
		t.Error("the probe trigger must reference a pipeline")
	}
	if len(trigger.AllowedCallers) == 0 {
		t.Error("the probe trigger must declare allowed_callers, or every invocation is rejected")
	}

	// The harness sends these payload fields, so the mapping has to consume
	// them or --pipeline-work-seconds silently does nothing.
	for _, variable := range []string{"PERF_WORK_SECONDS", "PERF_RUN_LABEL"} {
		if _, ok := trigger.VariableMapping[variable]; !ok {
			t.Errorf("variable_mapping is missing %s, which the harness sends on every invocation", variable)
		}
	}
}

// TestProbeFixturesAgreeOnTheVariableContract ties the two fixtures together:
// the trigger maps payload fields onto variables that the pipeline must declare.
func TestProbeFixturesAgreeOnTheVariableContract(t *testing.T) {
	pipelineRaw, err := os.ReadFile(fixturePath("pipelines", "perf-load-probe.yaml"))
	if err != nil {
		t.Fatalf("read probe pipeline: %v", err)
	}
	var pipeline models.Pipeline
	if err := yaml.Unmarshal(pipelineRaw, &pipeline); err != nil {
		t.Fatalf("parse probe pipeline: %v", err)
	}

	triggerRaw, err := os.ReadFile(fixturePath("external-triggers", "perf-load-probe.yaml"))
	if err != nil {
		t.Fatalf("read probe trigger: %v", err)
	}
	var trigger gitOpsExternalTrigger
	if err := yaml.Unmarshal(triggerRaw, &trigger); err != nil {
		t.Fatalf("parse probe trigger: %v", err)
	}

	declared := make(map[string]struct{}, len(pipeline.Variables))
	for _, variable := range pipeline.Variables {
		declared[variable] = struct{}{}
	}
	for variable := range trigger.VariableMapping {
		if _, ok := declared[variable]; !ok {
			t.Errorf("the trigger maps %s, but the pipeline does not declare it", variable)
		}
	}
}
