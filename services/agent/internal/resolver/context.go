package main

import (
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

type taskRuntimeSource string

const (
	taskRuntimeSourcePipelineVariable taskRuntimeSource = "pipeline_variable"
	taskRuntimeSourceInherited        taskRuntimeSource = "inherited"
	taskRuntimeSourceStepVariable     taskRuntimeSource = "step_variable"
	taskRuntimeSourceTaskVariable     taskRuntimeSource = "task_variable"
	taskRuntimeSourceSecret           taskRuntimeSource = "secret"
)

type taskRuntimeValue struct {
	Value     string
	Sensitive bool
}

type taskExecutionContext struct {
	values map[string]taskRuntimeValue
}

func buildStepExecutionContext(pipeline *models.Pipeline, step *models.PipelineStep, inheritedVars []string, variables, secrets map[string]string) (taskExecutionContext, []string) {
	context := newTaskExecutionContext()
	requiredRefs := make(map[string]models.ScopedRuntimeRef, len(pipeline.Variables))
	requiredRuntimeNames := make(map[string]models.ScopedRuntimeRef, len(pipeline.Variables))
	for _, key := range pipeline.Variables {
		ref, err := models.ParseScopedRuntimeRef(key, "")
		if err != nil {
			continue
		}
		requiredRefs[ref.Key()] = ref
		requiredRuntimeNames[ref.Name] = ref
	}

	for key, value := range variables {
		ref, err := models.ParseScopedRuntimeRef(key, "")
		if err != nil {
			continue
		}
		if requiredRef, ok := requiredRefs[ref.Key()]; ok {
			context.set(requiredRef.Name, value, taskRuntimeSourcePipelineVariable)
		}
	}

	for _, entry := range inheritedVars {
		key, value, ok := splitRuntimeEntry(entry)
		if !ok {
			continue
		}
		if requiredRef, isRequired := requiredRefs[key]; isRequired {
			context.set(requiredRef.Name, value, taskRuntimeSourceInherited)
			continue
		}
		if requiredRef, isRequired := requiredRuntimeNames[key]; isRequired {
			context.set(requiredRef.Name, value, taskRuntimeSourceInherited)
			continue
		}
		if strings.HasPrefix(key, "GIT_") || key == "SCOPE" {
			context.set(key, value, taskRuntimeSourceInherited)
		}
	}

	for key, value := range step.GetVariables() {
		context.set(key, value, taskRuntimeSourceStepVariable)
	}

	missingSecrets := make([]string, 0)
	for _, secretName := range step.GetSecrets() {
		ref, err := models.ParseScopedRuntimeRef(secretName, "")
		if err != nil {
			continue
		}
		secretValue, ok := secrets[ref.Key()]
		if !ok {
			missingSecrets = append(missingSecrets, ref.Key())
			continue
		}
		context.set(ref.Name, secretValue, taskRuntimeSourceSecret)
	}
	sort.Strings(missingSecrets)

	return context, missingSecrets
}

func newTaskExecutionContext() taskExecutionContext {
	return taskExecutionContext{
		values: make(map[string]taskRuntimeValue),
	}
}

func (c taskExecutionContext) withTask(task *models.Task) taskExecutionContext {
	cloned := newTaskExecutionContext()
	for key, value := range c.values {
		cloned.values[key] = value
	}
	if task == nil {
		return cloned
	}
	for key, value := range task.Variables {
		cloned.set(key, value, taskRuntimeSourceTaskVariable)
	}
	return cloned
}

func (c taskExecutionContext) containerVariables() []string {
	if len(c.values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.values))
	for key := range c.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	runtimeVars := make([]string, 0, len(keys))
	for _, key := range keys {
		runtimeVars = append(runtimeVars, key+"="+c.values[key].Value)
	}
	return runtimeVars
}

func (c taskExecutionContext) promptVariables() map[string]string {
	if len(c.values) == 0 {
		return map[string]string{}
	}
	variables := make(map[string]string, len(c.values))
	for key, value := range c.values {
		if value.Sensitive && value.Value != "" {
			variables[key] = "[redacted]"
			continue
		}
		variables[key] = value.Value
	}
	return variables
}

func (c taskExecutionContext) buildConditionRequest(goal, history, knowledgeContext string, secrets map[string]string) *proto.ConditionRequest {
	return &proto.ConditionRequest{
		Goal:             goal,
		History:          c.maskText(history, secrets),
		Variables:        c.promptVariables(),
		KnowledgeContext: c.maskText(knowledgeContext, secrets),
	}
}

func (c taskExecutionContext) buildActionRequest(goal, history string, directoryListing map[string]string, knowledgeContext string, secrets map[string]string) *proto.GetActionRequest {
	maskValues := c.promptMaskValues(secrets)
	return &proto.GetActionRequest{
		Goal:             goal,
		History:          maskSensitiveValues(history, maskValues),
		DirectoryListing: maskDirectoryListing(directoryListing, maskValues),
		Variables:        c.promptVariables(),
		KnowledgeContext: maskSensitiveValues(knowledgeContext, maskValues),
	}
}

func (c taskExecutionContext) maskText(input string, secrets map[string]string) string {
	return maskSensitiveValues(input, c.promptMaskValues(secrets))
}

func (c taskExecutionContext) promptMaskValues(secrets map[string]string) []string {
	values := make([]string, 0, len(c.values)+len(secrets))
	for _, value := range c.values {
		if value.Sensitive {
			values = append(values, value.Value)
		}
	}
	for _, value := range secrets {
		values = append(values, value)
	}
	return uniqueSensitiveValues(values)
}

func (c taskExecutionContext) set(name, value string, source taskRuntimeSource) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	c.values[trimmed] = taskRuntimeValue{
		Value:     value,
		Sensitive: source == taskRuntimeSourceSecret || isSensitiveVariableName(trimmed),
	}
}

func splitRuntimeEntry(entry string) (string, string, bool) {
	parts := strings.SplitN(entry, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false
	}
	return key, parts[1], true
}

func maskDirectoryListing(directoryListing map[string]string, maskValues []string) map[string]string {
	if len(directoryListing) == 0 {
		return map[string]string{}
	}
	masked := make(map[string]string, len(directoryListing))
	for name, content := range directoryListing {
		masked[name] = maskSensitiveValues(content, maskValues)
	}
	return masked
}

func maskSensitiveValues(input string, values []string) string {
	if input == "" || len(values) == 0 {
		return input
	}
	masked := input
	for _, value := range values {
		if len(value) < 4 {
			continue
		}
		masked = strings.ReplaceAll(masked, value, "*****")

		if strings.Contains(value, "\n") {
			flattened := strings.ReplaceAll(value, "\r", "")
			flattened = strings.ReplaceAll(flattened, "\n", " ")
			if len(flattened) >= 4 {
				masked = strings.ReplaceAll(masked, flattened, "*****")
			}
		}
	}
	return masked
}

func uniqueSensitiveValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	sort.SliceStable(unique, func(i, j int) bool {
		return len(unique[i]) > len(unique[j])
	})
	return unique
}

func isSensitiveVariableName(name string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return false
	}
	normalized = strings.NewReplacer("-", "_", ".", "_").Replace(normalized)
	tokens := strings.Split(normalized, "_")
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		if token == "" {
			continue
		}
		tokenSet[token] = struct{}{}
	}

	has := func(token string) bool {
		_, ok := tokenSet[token]
		return ok
	}

	switch {
	case has("SECRET"):
		return true
	case has("TOKEN"):
		return true
	case has("PASSWORD"):
		return true
	case has("PASSWD"):
		return true
	case has("CREDENTIAL") || has("CREDENTIALS"):
		return true
	case has("PRIVATE") && has("KEY"):
		return true
	case has("API") && has("KEY"):
		return true
	case has("ACCESS") && has("KEY"):
		return true
	case has("ACCESS") && has("TOKEN"):
		return true
	default:
		return false
	}
}
