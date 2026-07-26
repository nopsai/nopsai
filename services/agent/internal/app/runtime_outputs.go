package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
	"nopsai/services/agent/internal/executor"
)

const defaultRuntimeOutputMaxBytes int64 = 64 * 1024

type RuntimeOutputValue struct {
	Name      string
	Value     string
	Sensitive bool
	StepName  string
	TaskName  string
	SizeBytes int64
}

type runtimeOutputStore struct {
	mu      sync.RWMutex
	outputs map[string]RuntimeOutputValue
}

func newRuntimeOutputStore() *runtimeOutputStore {
	return &runtimeOutputStore{outputs: map[string]RuntimeOutputValue{}}
}

func (s *runtimeOutputStore) Set(stepName, taskName string, values map[string]RuntimeOutputValue) {
	if s == nil || len(values) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, value := range values {
		value.Name = strings.TrimSpace(firstNonEmpty(value.Name, name))
		value.StepName = strings.TrimSpace(firstNonEmpty(value.StepName, stepName))
		value.TaskName = strings.TrimSpace(firstNonEmpty(value.TaskName, taskName))
		s.outputs[models.RuntimeOutputRefKey(value.StepName, value.TaskName, value.Name)] = value
	}
}

func (s *runtimeOutputStore) Resolve(ref models.RuntimeOutputRef) (RuntimeOutputValue, bool) {
	if s == nil {
		return RuntimeOutputValue{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.outputs[ref.Key()]
	return value, ok
}

func (s *runtimeOutputStore) SensitiveValues() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	values := make([]string, 0, len(s.outputs))
	for _, output := range s.outputs {
		if output.Sensitive && output.Value != "" {
			values = append(values, output.Value)
		}
	}
	return values
}

func resolveRuntimeOutputVariables(variables map[string]string, store *runtimeOutputStore) (map[string]RuntimeOutputValue, error) {
	if len(variables) == 0 {
		return nil, nil
	}
	resolved := map[string]RuntimeOutputValue{}
	for name, raw := range variables {
		ref, found, err := models.ParseRuntimeOutputRef(raw)
		if err != nil {
			return nil, err
		}
		if !found {
			continue
		}
		value, ok := store.Resolve(ref)
		if !ok {
			return nil, fmt.Errorf("runtime output %s.%s.outputs.%s has not been produced", ref.StepName, ref.TaskName, ref.OutputName)
		}
		value.Name = strings.TrimSpace(name)
		resolved[name] = value
	}
	if len(resolved) == 0 {
		return nil, nil
	}
	return resolved, nil
}

func referencedRuntimeOutputs(pipeline *models.Pipeline) map[string]bool {
	required := map[string]bool{}
	if pipeline == nil {
		return required
	}
	for _, step := range pipeline.Steps {
		recordRuntimeOutputReferences(required, step.GetVariables())
		for _, task := range step.GetTasks() {
			recordRuntimeOutputReferences(required, task.Variables)
		}
	}
	return required
}

func recordRuntimeOutputReferences(required map[string]bool, variables map[string]string) {
	for _, value := range variables {
		ref, found, err := models.ParseRuntimeOutputRef(value)
		if err == nil && found {
			required[ref.Key()] = true
		}
	}
}

func collectRuntimeOutputFiles(ctx context.Context, runtime StepRuntime, sessionID string, outputs []models.TaskOutput, required map[string]bool, maxBytes int64) (map[string]RuntimeOutputValue, error) {
	if len(outputs) == 0 {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultRuntimeOutputMaxBytes
	}
	collected := map[string]RuntimeOutputValue{}
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		if name == "" {
			continue
		}
		requiredOutput := required[name]
		value, produced, err := collectOneRuntimeOutputFile(ctx, runtime, sessionID, output, maxBytes)
		if err != nil {
			return nil, err
		}
		if !produced {
			if requiredOutput {
				return nil, fmt.Errorf("required output file %s/%s was not produced", models.RuntimeOutputsMountPath, name)
			}
			continue
		}
		collected[name] = value
	}
	if len(collected) == 0 {
		return nil, nil
	}
	return collected, nil
}

func collectOneRuntimeOutputFile(ctx context.Context, runtime StepRuntime, sessionID string, output models.TaskOutput, maxBytes int64) (RuntimeOutputValue, bool, error) {
	name := strings.TrimSpace(output.Name)
	filePath := models.RuntimeOutputsMountPath + "/" + name
	command := fmt.Sprintf(
		`path=%s; if [ ! -f "$path" ]; then exit 42; fi; size=$(wc -c < "$path" | tr -d ' '); if [ "$size" -gt %d ]; then echo "output file exceeds %d bytes" >&2; exit 43; fi; printf '%%s\n' "$size"; base64 "$path" | tr -d '\n'`,
		executor.ShellQuote(filePath),
		maxBytes,
		maxBytes,
	)
	action := &proto.Action{
		Type:    "EXECUTE_COMMAND",
		Payload: &proto.Action_CommandAction{CommandAction: &proto.CommandAction{Command: command}},
	}
	stdout, stderr, exitCode := runtime.ExecuteAction(ctx, sessionID, action, nil, models.DefaultPipelineWorkingDirectory, nil)
	switch exitCode {
	case 0:
	case 42:
		return RuntimeOutputValue{}, false, nil
	default:
		return RuntimeOutputValue{}, false, fmt.Errorf("collect output %s: %s", name, strings.TrimSpace(stderr+stdout))
	}

	sizeLine, encoded, _ := strings.Cut(stdout, "\n")
	sizeBytes, err := strconv.ParseInt(strings.TrimSpace(sizeLine), 10, 64)
	if err != nil {
		return RuntimeOutputValue{}, false, fmt.Errorf("collect output %s: invalid size %q", name, sizeLine)
	}
	var decoded []byte
	if strings.TrimSpace(encoded) != "" {
		decoded, err = base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return RuntimeOutputValue{}, false, fmt.Errorf("collect output %s: decode payload: %w", name, err)
		}
	}
	return RuntimeOutputValue{
		Name:      name,
		Value:     string(decoded),
		Sensitive: output.Sensitive,
		SizeBytes: sizeBytes,
	}, true, nil
}

func outputRequiredByName(requiredRefs map[string]bool, stepName, taskName string) map[string]bool {
	required := map[string]bool{}
	for ref := range requiredRefs {
		parts := strings.Split(ref, "/")
		if len(parts) != 3 {
			continue
		}
		if parts[0] == stepName && parts[1] == taskName {
			required[parts[2]] = true
		}
	}
	return required
}

func sortedRuntimeOutputNames(values map[string]RuntimeOutputValue) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
