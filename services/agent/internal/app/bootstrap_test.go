package app

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestLoadRuntimeConfigDecodesRuntimeInputs(t *testing.T) {
	pipelineYAML := "name: release\nworking_directory: /workspace\nsteps: []\n"
	env := map[string]string{
		"RUN_ID":              "run-1",
		"PIPELINE_NAME":       "release",
		"PIPELINE_DEFINITION": base64.StdEncoding.EncodeToString([]byte(pipelineYAML)),
		"SHARED_VOLUME_NAME":  "workspace-run-1",
		"NOPSAI_SECRETS":      base64.StdEncoding.EncodeToString([]byte(`{"TOKEN":"secret"}`)),
		"NOPSAI_VARIABLES":    base64.StdEncoding.EncodeToString([]byte(`{"ENV":"prod"}`)),
		"NOPSAI_RUNTIME":      "k8s",
		"SCOPE":               "prod",
	}

	config, warnings, err := LoadRuntimeConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if config.RunID != "run-1" || config.PipelineName != "release" || config.Pipeline.Name != "release" {
		t.Fatalf("config identifiers = %#v", config)
	}
	if config.TriggerEventID != "N/A" {
		t.Fatalf("trigger event = %q, want N/A", config.TriggerEventID)
	}
	if config.LLMTimeout != 2*time.Minute {
		t.Fatalf("llm timeout = %s, want 2m", config.LLMTimeout)
	}
	if config.RuntimeMode != "kubernetes" {
		t.Fatalf("runtime mode = %q, want kubernetes", config.RuntimeMode)
	}
	if got := config.Secrets["TOKEN"]; got != "secret" {
		t.Fatalf("secret TOKEN = %q, want secret", got)
	}
	if got := config.Variables["ENV"]; got != "prod" {
		t.Fatalf("variable ENV = %q, want prod", got)
	}
	if string(config.PipelineDefinitionYAML) != pipelineYAML {
		t.Fatalf("pipeline yaml = %q, want original yaml", string(config.PipelineDefinitionYAML))
	}
}

func TestLoadRuntimeConfigReportsPayloadWarningsAndKeepsVariablesUsable(t *testing.T) {
	pipelineYAML := "name: release\nsteps: []\n"
	env := map[string]string{
		"RUN_ID":              "run-1",
		"PIPELINE_NAME":       "release",
		"PIPELINE_DEFINITION": base64.StdEncoding.EncodeToString([]byte(pipelineYAML)),
		"SHARED_VOLUME_NAME":  "workspace-run-1",
		"NOPSAI_SECRETS":      "not-base64",
		"NOPSAI_VARIABLES":    base64.StdEncoding.EncodeToString([]byte(`not-json`)),
	}

	config, warnings, err := LoadRuntimeConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want 2", warnings)
	}
	if warnings[0].Kind != WarningDecodeSecrets || warnings[0].LogMessage() != "Failed to decode secrets payload" {
		t.Fatalf("first warning = %#v", warnings[0])
	}
	if warnings[1].Kind != WarningUnmarshalVariables || warnings[1].LogMessage() != "Failed to unmarshal variables payload" {
		t.Fatalf("second warning = %#v", warnings[1])
	}
	if config.Variables == nil {
		t.Fatal("variables map is nil after invalid variables payload")
	}
}

func TestLoadRuntimeConfigClassifiesLoadFailures(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantKind   FailureKind
		wantLogMsg string
	}{
		{
			name: "invalid llm timeout",
			env: map[string]string{
				"RUN_ID":              "run-1",
				"PIPELINE_NAME":       "release",
				"PIPELINE_DEFINITION": base64.StdEncoding.EncodeToString([]byte("name: release\n")),
				"SHARED_VOLUME_NAME":  "workspace-run-1",
				"LLM_AGENT_TIMEOUT":   "nope",
			},
			wantKind:   FailureInvalidLLMTimeout,
			wantLogMsg: "Invalid LLM timeout duration",
		},
		{
			name:       "missing required",
			env:        map[string]string{"RUN_ID": "run-1"},
			wantKind:   FailureMissingRequiredRuntimeValue,
			wantLogMsg: "Missing one or more required runtime variables",
		},
		{
			name: "invalid pipeline base64",
			env: map[string]string{
				"RUN_ID":              "run-1",
				"PIPELINE_NAME":       "release",
				"PIPELINE_DEFINITION": "not-base64",
				"SHARED_VOLUME_NAME":  "workspace-run-1",
			},
			wantKind:   FailureDecodePipelineDefinition,
			wantLogMsg: "Failed to decode pipeline definition",
		},
		{
			name: "invalid pipeline yaml",
			env: map[string]string{
				"RUN_ID":              "run-1",
				"PIPELINE_NAME":       "release",
				"PIPELINE_DEFINITION": base64.StdEncoding.EncodeToString([]byte("name: [")),
				"SHARED_VOLUME_NAME":  "workspace-run-1",
			},
			wantKind:   FailureUnmarshalPipelineDefinition,
			wantLogMsg: "Failed to unmarshal pipeline definition",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := LoadRuntimeConfig(func(key string) string { return tt.env[key] })
			var loadErr LoadError
			if !errors.As(err, &loadErr) {
				t.Fatalf("error = %v, want LoadError", err)
			}
			if loadErr.Kind != tt.wantKind {
				t.Fatalf("kind = %s, want %s", loadErr.Kind, tt.wantKind)
			}
			if got := LoadFailureLogMessage(err); got != tt.wantLogMsg {
				t.Fatalf("log message = %q, want %q", got, tt.wantLogMsg)
			}
		})
	}
}
