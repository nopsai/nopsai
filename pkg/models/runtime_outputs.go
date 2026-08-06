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
		return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must use $steps.<step>.outputs.<name> or $steps.<step>.<task>.outputs.<name>")
	}

	producer := body[:outputIdx]
	outputName := body[outputIdx+len(outputMarker):]
	taskIdx := strings.LastIndex(producer, ".")
	if taskIdx == len(producer)-1 {
		return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must include a producing task")
	}

	if taskIdx <= 0 {
		producer = strings.TrimSpace(producer)
		if producer == "" {
			return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must include a producing step")
		}
		ref := RuntimeOutputRef{
			StepName:   producer,
			TaskName:   producer,
			OutputName: strings.TrimSpace(outputName),
		}
		if ref.OutputName == "" {
			return RuntimeOutputRef{}, true, fmt.Errorf("runtime output reference must include non-empty step and output names")
		}
		if !IsValidTaskOutputName(ref.OutputName) {
			return RuntimeOutputRef{}, true, fmt.Errorf("runtime output name %q is invalid", ref.OutputName)
		}
		return ref, true, nil
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

func ParseRuntimeOutputRefCandidates(raw string) ([]RuntimeOutputRef, bool, error) {
	ref, found, err := ParseRuntimeOutputRef(raw)
	if err != nil || !found {
		return nil, found, err
	}
	candidates := []RuntimeOutputRef{ref}

	value := strings.TrimSpace(raw)
	body := strings.TrimPrefix(value, RuntimeOutputReferencePrefix)
	outputMarker := ".outputs."
	outputIdx := strings.LastIndex(body, outputMarker)
	if outputIdx <= 0 || outputIdx+len(outputMarker) >= len(body) {
		return candidates, true, nil
	}
	producer := strings.TrimSpace(body[:outputIdx])
	outputName := strings.TrimSpace(body[outputIdx+len(outputMarker):])
	if producer == "" || outputName == "" || !IsValidTaskOutputName(outputName) {
		return candidates, true, nil
	}
	stepLevel := RuntimeOutputRef{
		StepName:   producer,
		TaskName:   producer,
		OutputName: outputName,
	}
	if stepLevel.Key() != ref.Key() {
		candidates = append(candidates, stepLevel)
	}
	return candidates, true, nil
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
