package models

import (
	"fmt"
	"regexp"
	"strings"
)

const RuntimeOutputReferencePrefix = "$steps."

var taskOutputNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type RuntimeOutputRef struct {
	StepName   string
	TaskName   string
	OutputName string
}

func IsValidTaskOutputName(name string) bool {
	return taskOutputNamePattern.MatchString(strings.TrimSpace(name))
}

func ParseRuntimeOutputRef(raw string) (RuntimeOutputRef, bool, error) {
	value := strings.TrimSpace(raw)
	if !strings.HasPrefix(value, RuntimeOutputReferencePrefix) {
		return RuntimeOutputRef{}, false, nil
	}

	body := strings.TrimPrefix(value, RuntimeOutputReferencePrefix)
	outputMarker := ".outputs."
	outputIdx := strings.LastIndex(body, outputMarker)
	if outputIdx <= 0 || outputIdx+len(outputMarker) >= len(body) {
		return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must use $steps.<step>.<task>.outputs.<name>")
	}

	producer := body[:outputIdx]
	outputName := body[outputIdx+len(outputMarker):]
	taskIdx := strings.LastIndex(producer, ".")
	if taskIdx <= 0 || taskIdx == len(producer)-1 {
		return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must include a producing step and task")
	}

	ref := RuntimeOutputRef{
		StepName:   strings.TrimSpace(producer[:taskIdx]),
		TaskName:   strings.TrimSpace(producer[taskIdx+1:]),
		OutputName: strings.TrimSpace(outputName),
	}
	if ref.StepName == "" || ref.TaskName == "" || ref.OutputName == "" {
		return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must include non-empty step, task and output names")
	}
	if !IsValidTaskOutputName(ref.OutputName) {
		return RuntimeOutputRef{}, true, fmt.Errorf("runtime output name %q is invalid", ref.OutputName)
	}
	return ref, true, nil
}

func RuntimeOutputRefKey(stepName, taskName, outputName string) string {
	return strings.TrimSpace(stepName) + "/" + strings.TrimSpace(taskName) + "/" + strings.TrimSpace(outputName)
}

func (r RuntimeOutputRef) Key() string {
	return RuntimeOutputRefKey(r.StepName, r.TaskName, r.OutputName)
}

func (r RuntimeOutputRef) ProducerTaskKey() string {
	return strings.TrimSpace(r.StepName) + "/" + strings.TrimSpace(r.TaskName)
}
