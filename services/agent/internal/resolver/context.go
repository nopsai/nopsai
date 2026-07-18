package resolver

import (
	"regexp"
	"sort"
	"strings"

	"nopsai/pkg/models"
	"nopsai/pkg/proto"
)

type runtimeSource string

const (
	runtimeSourcePipelineVariable runtimeSource = "pipeline_variable"
	runtimeSourceInherited        runtimeSource = "inherited"
	runtimeSourceStepVariable     runtimeSource = "step_variable"
	runtimeSourceTaskVariable     runtimeSource = "task_variable"
	runtimeSourceSecret           runtimeSource = "secret"
)

type runtimeValue struct {
	Value     string
	Sensitive bool
}

type namedRuntimeValue struct {
	Name  string
	Value string
}

type ExecutionContext struct {
	values map[string]runtimeValue
}

func BuildStepContext(pipeline *models.Pipeline, step *models.PipelineStep, inheritedVars []string, variables, secrets map[string]string) (ExecutionContext, []string) {
	context := NewExecutionContext()
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
			context.set(requiredRef.Name, value, runtimeSourcePipelineVariable)
		}
	}

	for _, entry := range inheritedVars {
		key, value, ok := splitRuntimeEntry(entry)
		if !ok {
			continue
		}
		if requiredRef, isRequired := requiredRefs[key]; isRequired {
			context.set(requiredRef.Name, value, runtimeSourceInherited)
			continue
		}
		if requiredRef, isRequired := requiredRuntimeNames[key]; isRequired {
			context.set(requiredRef.Name, value, runtimeSourceInherited)
			continue
		}
		if strings.HasPrefix(key, "GIT_") || key == "SCOPE" {
			context.set(key, value, runtimeSourceInherited)
		}
	}

	for key, value := range step.GetVariables() {
		context.set(key, value, runtimeSourceStepVariable)
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
		context.set(ref.Name, secretValue, runtimeSourceSecret)
	}
	sort.Strings(missingSecrets)

	return context, missingSecrets
}

func NewExecutionContext() ExecutionContext {
	return ExecutionContext{
		values: make(map[string]runtimeValue),
	}
}

func (c ExecutionContext) WithTask(task *models.Task) ExecutionContext {
	cloned := NewExecutionContext()
	for key, value := range c.values {
		cloned.values[key] = value
	}
	if task == nil {
		return cloned
	}
	for key, value := range task.Variables {
		cloned.set(key, value, runtimeSourceTaskVariable)
	}
	return cloned
}

func (c ExecutionContext) ContainerVariables() []string {
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

func (c ExecutionContext) PromptVariables() map[string]string {
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

func (c ExecutionContext) BuildConditionRequest(goal, history, knowledgeContext string, secrets map[string]string) *proto.ConditionRequest {
	return &proto.ConditionRequest{
		Goal:             goal,
		History:          c.MaskText(history, secrets),
		Variables:        c.PromptVariables(),
		KnowledgeContext: c.MaskText(knowledgeContext, secrets),
	}
}

func (c ExecutionContext) BuildActionRequest(goal, history string, directoryListing map[string]string, knowledgeContext string, secrets map[string]string) *proto.GetActionRequest {
	maskValues := c.promptMaskValues(secrets)
	return &proto.GetActionRequest{
		Goal:             goal,
		History:          maskSensitiveValues(history, maskValues),
		DirectoryListing: maskDirectoryListing(directoryListing, maskValues),
		Variables:        c.PromptVariables(),
		KnowledgeContext: maskSensitiveValues(knowledgeContext, maskValues),
	}
}

func (c ExecutionContext) MaskText(input string, secrets map[string]string) string {
	return maskSensitiveValues(input, c.promptMaskValues(secrets))
}

func (c ExecutionContext) MaskRuntimeText(input string, secrets map[string]string) string {
	if input == "" {
		return input
	}
	sensitiveValues := make([]string, 0, len(c.values)+len(secrets))
	plainValues := make([]namedRuntimeValue, 0, len(c.values))
	for name, value := range c.values {
		if value.Sensitive {
			sensitiveValues = append(sensitiveValues, value.Value)
			continue
		}
		plainValues = append(plainValues, namedRuntimeValue{Name: name, Value: value.Value})
	}
	for _, value := range secrets {
		sensitiveValues = append(sensitiveValues, value)
	}
	masked := maskSensitiveValues(input, sensitiveValues)
	return maskPlainRuntimeValues(masked, plainValues)
}

func (c ExecutionContext) promptMaskValues(secrets map[string]string) []string {
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

func (c *ExecutionContext) SetValue(name, value string, sensitive bool) {
	source := runtimeSourceTaskVariable
	if sensitive {
		source = runtimeSourceSecret
	}
	c.set(name, value, source)
}

func (c *ExecutionContext) set(name, value string, source runtimeSource) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	if c.values == nil {
		c.values = make(map[string]runtimeValue)
	}
	c.values[trimmed] = runtimeValue{
		Value:     value,
		Sensitive: source == runtimeSourceSecret || isSensitiveVariableName(trimmed),
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

func maskPlainRuntimeValues(input string, values []namedRuntimeValue) string {
	if input == "" || len(values) == 0 {
		return input
	}
	unique := uniqueNamedRuntimeValues(values)
	sort.SliceStable(unique, func(i, j int) bool {
		return len(unique[i].Value) > len(unique[j].Value)
	})
	masked := input
	for _, value := range unique {
		if len(value.Value) < 4 {
			continue
		}
		if shouldMaskPlainRuntimeValueOnlyInAssignments(value.Value) {
			masked = maskRuntimeAssignmentValue(masked, value.Name, value.Value)
			continue
		}
		masked = maskSensitiveValues(masked, []string{value.Value})
	}
	return masked
}

func uniqueNamedRuntimeValues(values []namedRuntimeValue) []namedRuntimeValue {
	seen := make(map[string]struct{}, len(values))
	unique := make([]namedRuntimeValue, 0, len(values))
	for _, value := range values {
		value.Name = strings.TrimSpace(value.Name)
		if value.Name == "" || value.Value == "" {
			continue
		}
		key := value.Name + "\x00" + value.Value
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func shouldMaskPlainRuntimeValueOnlyInAssignments(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 8 {
		return true
	}
	if len(trimmed) <= 16 && plainRuntimeIdentifierPattern.MatchString(trimmed) {
		return true
	}
	switch strings.ToLower(trimmed) {
	case "prod", "production", "dev", "development", "staging", "stage", "test", "testing", "qa", "dashboard", "latest", "main", "master", "true", "false", "enabled", "disabled":
		return true
	default:
		return false
	}
}

var plainRuntimeIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

func maskRuntimeAssignmentValue(input, name, value string) string {
	if input == "" || strings.TrimSpace(name) == "" || value == "" {
		return input
	}
	masked := input
	namePattern := regexp.QuoteMeta(strings.TrimSpace(name))
	valuePattern := regexp.QuoteMeta(value)
	trailer := `($|[^A-Za-z0-9_./:-])`
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(\b` + namePattern + `\s*=\s*)` + valuePattern + trailer),
		regexp.MustCompile(`(?i)(\b` + namePattern + `\s*:\s*)` + valuePattern + trailer),
		regexp.MustCompile(`(?i)("` + namePattern + `"\s*:\s*")` + valuePattern + `(")`),
	}
	for _, pattern := range patterns {
		masked = pattern.ReplaceAllString(masked, "${1}*****${2}")
	}
	if strings.Contains(value, "\n") {
		flattened := strings.ReplaceAll(value, "\r", "")
		flattened = strings.ReplaceAll(flattened, "\n", " ")
		if flattened != value && len(flattened) >= 4 {
			masked = maskRuntimeAssignmentValue(masked, name, flattened)
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
