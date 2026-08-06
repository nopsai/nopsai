package include

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

type includeFinalization struct {
	stepName      string
	taskName      string
	status        string
	exitCode      int
	llmDurationMs int64
}

func TestRunnerInvalidIncludeFinalizesFailure(t *testing.T) {
	logger := zerolog.Nop()
	var finalized []includeFinalization
	result := NewRunner(Config{}).Run(context.Background(), Request{
		Logger:        &logger,
		StepName:      "include",
		IncludeTarget: "bad-format",
		LLMDurationMs: 12,
		FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
			finalized = append(finalized, includeFinalization{stepName, taskName, status, exitCode, llmDurationMs})
		},
	})

	if !result.Handled || result.Success {
		t.Fatalf("result = %#v, want handled failure", result)
	}
	if len(finalized) != 1 || finalized[0].status != "failure" || finalized[0].exitCode != 1 {
		t.Fatalf("finalized = %#v, want failure/1", finalized)
	}
	if finalized[0].llmDurationMs != 12 {
		t.Fatalf("llm duration = %d, want 12", finalized[0].llmDurationMs)
	}
}

func TestRunnerNotFoundFinalizesNotFound(t *testing.T) {
	logger := zerolog.Nop()
	var finalized []includeFinalization
	runner := NewRunner(Config{
		FetchDefinition: func(context.Context, string) ([]byte, error) {
			return nil, errors.New("nopsai api returned non-200 status 404: missing")
		},
		IsNotFound: func(err error) bool {
			return strings.Contains(err.Error(), "non-200 status 404")
		},
	})

	result := runner.Run(context.Background(), Request{
		Logger:        &logger,
		StepName:      "include",
		IncludeTarget: "pipeline:missing-child",
		FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
			finalized = append(finalized, includeFinalization{stepName, taskName, status, exitCode, llmDurationMs})
		},
	})

	if !result.Handled || result.Success {
		t.Fatalf("result = %#v, want handled failure", result)
	}
	if len(finalized) != 1 || finalized[0].status != "not_found" || finalized[0].exitCode != 0 {
		t.Fatalf("finalized = %#v, want not_found/0", finalized)
	}
}

func TestRunnerSyncMonitorFinalizesAndMarksFailure(t *testing.T) {
	logger := zerolog.Nop()
	var mu sync.Mutex
	var finalized []includeFinalization
	markedFailed := false
	var markedStatus string
	var triggeredHistory string
	var triggeredDef string
	var triggeredVariables map[string]string
	var triggeredSensitive []string
	runner := NewRunner(Config{
		FetchDefinition: func(_ context.Context, pipelineName string) ([]byte, error) {
			if pipelineName != "release-child" {
				t.Fatalf("pipelineName = %q, want release-child", pipelineName)
			}
			return []byte("name: release-child"), nil
		},
		TriggerPipeline: func(_ context.Context, parentRunID, parentPipelineName, parentStepName, pipelineIdentifier string, pipelineDef []byte, history string, variables map[string]string, sensitiveVariables []string) (string, error) {
			if parentRunID != "run-1" || parentPipelineName != "release" || parentStepName != "include" || pipelineIdentifier != "release-child" {
				t.Fatalf("trigger args = %q/%q/%q/%q", parentRunID, parentPipelineName, parentStepName, pipelineIdentifier)
			}
			triggeredDef = string(pipelineDef)
			triggeredHistory = history
			triggeredVariables = variables
			triggeredSensitive = sensitiveVariables
			return "child-run-1", nil
		},
		MonitorPipeline: func(_ context.Context, _ *zerolog.Logger, runID string) (string, error) {
			if runID != "child-run-1" {
				t.Fatalf("monitor run ID = %q, want child-run-1", runID)
			}
			return "failure", nil
		},
	})

	result := runner.Run(context.Background(), Request{
		Logger:             &logger,
		ParentRunID:        "run-1",
		ParentPipelineName: "release",
		StepName:           "include",
		IncludeTarget:      "pipeline:release-child",
		History:            "- Goal: build",
		Variables:          map[string]string{"CHANNEL": "stable", "TOKEN": "secret"},
		SensitiveVariables: []string{"TOKEN"},
		Sync:               true,
		LLMDurationMs:      34,
		FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
			mu.Lock()
			defer mu.Unlock()
			finalized = append(finalized, includeFinalization{stepName, taskName, status, exitCode, llmDurationMs})
		},
		MarkPipelineFailed: func(status string) {
			markedFailed = true
			markedStatus = status
		},
	})
	if !result.Handled || result.Success {
		t.Fatalf("result = %#v, want handled failure after sync child failure", result)
	}

	if triggeredDef != "name: release-child" || triggeredHistory != "- Goal: build" {
		t.Fatalf("triggered def/history = %q/%q", triggeredDef, triggeredHistory)
	}
	if triggeredVariables["CHANNEL"] != "stable" || triggeredVariables["TOKEN"] != "secret" {
		t.Fatalf("triggered variables = %#v, want CHANNEL and TOKEN", triggeredVariables)
	}
	if len(triggeredSensitive) != 1 || triggeredSensitive[0] != "TOKEN" {
		t.Fatalf("triggered sensitive variables = %#v, want TOKEN", triggeredSensitive)
	}
	if !markedFailed {
		t.Fatal("sync child failure did not mark pipeline failed")
	}
	if markedStatus != "failure" {
		t.Fatalf("marked status = %q, want failure", markedStatus)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(finalized) != 1 || finalized[0].status != "failure" || finalized[0].exitCode != 0 || finalized[0].llmDurationMs != 34 {
		t.Fatalf("finalized = %#v, want child failure/0 with llm duration", finalized)
	}
}

func TestRunnerSyncFetchesChildOutputsBeforeFinalizingSuccess(t *testing.T) {
	logger := zerolog.Nop()
	var finalized []includeFinalization
	var fetchedParentRunID, fetchedParentStepName, fetchedChildRunID string
	var fetchedNames []string
	runner := NewRunner(Config{
		FetchDefinition: func(context.Context, string) ([]byte, error) {
			return []byte("name: release-child"), nil
		},
		TriggerPipeline: func(context.Context, string, string, string, string, []byte, string, map[string]string, []string) (string, error) {
			return "child-run-1", nil
		},
		MonitorPipeline: func(context.Context, *zerolog.Logger, string) (string, error) {
			return "success", nil
		},
		FetchOutputs: func(_ context.Context, parentRunID, parentStepName, childRunID string, names []string) (map[string]RuntimeOutput, error) {
			fetchedParentRunID = parentRunID
			fetchedParentStepName = parentStepName
			fetchedChildRunID = childRunID
			fetchedNames = append([]string(nil), names...)
			return map[string]RuntimeOutput{
				"image_tag": {Name: "image_tag", Value: "v1.2.3"},
			}, nil
		},
	})

	result := runner.Run(context.Background(), Request{
		Logger:        &logger,
		ParentRunID:   "parent-run-1",
		StepName:      "child",
		IncludeTarget: "pipeline:release-child",
		Sync:          true,
		OutputNames:   []string{"image_tag", "image_tag"},
		FinalizeTask: func(stepName, taskName, status string, exitCode int, llmDurationMs int64) {
			finalized = append(finalized, includeFinalization{stepName, taskName, status, exitCode, llmDurationMs})
		},
	})

	if !result.Handled || !result.Success || result.Status != "success" {
		t.Fatalf("result = %#v, want handled success", result)
	}
	if result.Outputs["image_tag"].Value != "v1.2.3" {
		t.Fatalf("outputs = %#v, want image_tag", result.Outputs)
	}
	if fetchedParentRunID != "parent-run-1" || fetchedParentStepName != "child" || fetchedChildRunID != "child-run-1" {
		t.Fatalf("fetch args = %q/%q/%q", fetchedParentRunID, fetchedParentStepName, fetchedChildRunID)
	}
	if len(fetchedNames) != 1 || fetchedNames[0] != "image_tag" {
		t.Fatalf("fetched names = %#v, want deduplicated image_tag", fetchedNames)
	}
	if len(finalized) != 1 || finalized[0].status != "success" || finalized[0].exitCode != 0 {
		t.Fatalf("finalized = %#v, want one success finalization", finalized)
	}
}
