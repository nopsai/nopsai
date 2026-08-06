package nopsai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type validationIssue struct {
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Code    string `json:"code,omitempty"`
	File    string `json:"file,omitempty"`
}

type validationResponse struct {
	Valid    bool              `json:"valid"`
	Errors   []validationIssue `json:"errors"`
	Warnings []validationIssue `json:"warnings"`
}

func validValidationResponse() validationResponse {
	return validationResponse{
		Valid:    true,
		Errors:   []validationIssue{},
		Warnings: []validationIssue{},
	}
}

func invalidValidationResponse(issues ...validationIssue) validationResponse {
	if len(issues) == 0 {
		issues = []validationIssue{{Message: "validation failed", Code: "validation_failed"}}
	}
	return validationResponse{
		Valid:    false,
		Errors:   issues,
		Warnings: []validationIssue{},
	}
}

func writeValidationResponse(w http.ResponseWriter, status int, result validationResponse) {
	if result.Errors == nil {
		result.Errors = []validationIssue{}
	}
	if result.Warnings == nil {
		result.Warnings = []validationIssue{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Warn().Err(err).Msg("Failed to encode validation response")
	}
}

func validationBadRequest(w http.ResponseWriter, message, code string) {
	writeValidationResponse(w, http.StatusBadRequest, invalidValidationResponse(validationIssue{
		Message: message,
		Code:    firstNonEmptyString(code, "invalid_request"),
	}))
}

func validationIssueFromError(rawYAML string, resourceKind string, err error) validationIssue {
	message := strings.TrimSpace(fmt.Sprint(err))
	if message == "" {
		message = "validation failed"
	}
	path, line := inferValidationPathAndLine(rawYAML, resourceKind, message)
	return validationIssue{
		Message: message,
		Path:    path,
		Line:    line,
		Code:    validationCodeForMessage(message),
	}
}

func validationParseIssue(rawYAML string, message string, err error) validationIssue {
	if strings.TrimSpace(message) == "" {
		message = "YAML is malformed"
	}
	if err != nil {
		message = fmt.Sprintf("%s: %v", strings.TrimSuffix(message, ":"), err)
	}
	return validationIssue{
		Message: message,
		Line:    yamlErrorLine(err),
		Code:    "yaml_parse_error",
	}
}

func validationCodeForMessage(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "yaml") && (strings.Contains(lower, "parse") || strings.Contains(lower, "malformed") || strings.Contains(lower, "unmarshal")):
		return "yaml_parse_error"
	case strings.Contains(lower, "without a valid dependency"):
		return "runtime_output_missing_dependency"
	case strings.Contains(lower, "undefined dependency") || strings.Contains(lower, "invalid dependency"):
		return "dependency_missing"
	case strings.Contains(lower, "dependency cycle"):
		return "dependency_cycle"
	case strings.Contains(lower, "duplicate"):
		return "duplicate"
	case strings.Contains(lower, "required") || strings.Contains(lower, "missing"):
		return "required"
	case strings.Contains(lower, "name") && strings.Contains(lower, "match"):
		return "name_mismatch"
	case strings.Contains(lower, "webhook_source"):
		return "webhook_source_invalid"
	case strings.Contains(lower, "access"):
		return "access_invalid"
	default:
		return "validation_failed"
	}
}

var yamlLineRE = regexp.MustCompile(`(?i)(?:line|at line)\s+(\d+)`)

func yamlErrorLine(err error) int {
	if err == nil {
		return 0
	}
	matches := yamlLineRE.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return 0
	}
	line, _ := strconv.Atoi(matches[1])
	return line
}

func inferValidationPathAndLine(rawYAML, resourceKind, message string) (string, int) {
	root := decodeValidationYAMLRoot(rawYAML)
	if root == nil {
		return "", 0
	}
	lower := strings.ToLower(message)
	switch resourceKind {
	case "pipeline":
		return inferPipelineValidationPathAndLine(root, lower)
	case "step":
		return inferStepValidationPathAndLine(root, lower)
	case "trigger":
		return inferTriggerValidationPathAndLine(root, lower)
	default:
		return "", 0
	}
}

func inferPipelineValidationPathAndLine(root *yaml.Node, lower string) (string, int) {
	if strings.Contains(lower, "container_image") {
		return "container_image", yamlMappingKeyLine(root, "container_image")
	}
	if strings.Contains(lower, "pipeline name") || strings.Contains(lower, "'name'") {
		return "name", yamlMappingKeyLine(root, "name")
	}
	if strings.Contains(lower, "at least one step") {
		return "steps", yamlMappingKeyLine(root, "steps")
	}
	if strings.Contains(lower, "consumes output") {
		if ref := firstTokenAfter(lower, "consumes output "); ref != "" {
			if path, line := yamlPipelineVariableRefPathAndLine(root, ref); path != "" {
				return path, line
			}
		}
	}
	stepName := firstQuotedValueAfter(lower, "step ")
	if stepName != "" {
		stepPath, stepLine := yamlStepPathAndLine(root, stepName)
		if strings.Contains(lower, "variable") {
			if line := yamlStepKeyLine(root, stepName, "variables"); line > 0 {
				return stepPath + ".variables", line
			}
		}
		if strings.Contains(lower, "dependency") {
			if line := yamlStepKeyLine(root, stepName, "depends_on"); line > 0 {
				return stepPath + ".depends_on", line
			}
		}
		if stepPath != "" {
			return stepPath, stepLine
		}
	}
	return "", 0
}

func yamlPipelineVariableRefPathAndLine(root *yaml.Node, ref string) (string, int) {
	ref = strings.TrimSpace(ref)
	steps := yamlMappingValue(root, "steps")
	if ref == "" || steps == nil || steps.Kind != yaml.SequenceNode {
		return "", 0
	}
	for stepIndex, step := range steps.Content {
		if path, line := yamlVariableRefPathAndLine(yamlMappingValue(step, "variables"), ref, fmt.Sprintf("steps[%d].variables", stepIndex)); path != "" {
			return path, line
		}
		tasks := yamlMappingValue(step, "tasks")
		if tasks == nil || tasks.Kind != yaml.SequenceNode {
			continue
		}
		for taskIndex, task := range tasks.Content {
			if path, line := yamlVariableRefPathAndLine(yamlMappingValue(task, "variables"), ref, fmt.Sprintf("steps[%d].tasks[%d].variables", stepIndex, taskIndex)); path != "" {
				return path, line
			}
		}
	}
	return "", 0
}

func yamlVariableRefPathAndLine(variables *yaml.Node, ref string, basePath string) (string, int) {
	if variables == nil || variables.Kind != yaml.MappingNode {
		return "", 0
	}
	for i := 0; i+1 < len(variables.Content); i += 2 {
		keyNode := variables.Content[i]
		valueNode := variables.Content[i+1]
		if keyNode == nil || valueNode == nil {
			continue
		}
		if strings.Contains(strings.ToLower(valueNode.Value), ref) {
			return basePath + "." + keyNode.Value, keyNode.Line
		}
	}
	return "", 0
}

func inferStepValidationPathAndLine(root *yaml.Node, lower string) (string, int) {
	if strings.Contains(lower, "name") {
		return "name", yamlMappingKeyLine(root, "name")
	}
	if strings.Contains(lower, "variable") {
		return "variables", yamlMappingKeyLine(root, "variables")
	}
	if strings.Contains(lower, "dependency") {
		return "depends_on", yamlMappingKeyLine(root, "depends_on")
	}
	if strings.Contains(lower, "task") {
		return "tasks", yamlMappingKeyLine(root, "tasks")
	}
	return "", 0
}

func inferTriggerValidationPathAndLine(root *yaml.Node, lower string) (string, int) {
	if strings.Contains(lower, "webhook_source") {
		return "webhook_source", yamlMappingKeyLine(root, "webhook_source")
	}
	if strings.Contains(lower, "management") {
		return "management", yamlMappingKeyLine(root, "management")
	}
	if strings.Contains(lower, "provider") {
		return "provider", yamlMappingKeyLine(root, "provider")
	}
	if strings.Contains(lower, "trigger") || strings.Contains(lower, "pipelines") {
		return "triggers", yamlMappingKeyLine(root, "triggers")
	}
	return "", 0
}

func decodeValidationYAMLRoot(raw string) *yaml.Node {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return &doc
}

func yamlMappingKeyLine(node *yaml.Node, key string) int {
	key = strings.TrimSpace(key)
	if node == nil || node.Kind != yaml.MappingNode || key == "" {
		return 0
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i].Line
		}
	}
	return 0
}

func yamlScalarValue(node *yaml.Node) string {
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.Value)
}

func yamlStepPathAndLine(root *yaml.Node, stepName string) (string, int) {
	steps := yamlMappingValue(root, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return "", 0
	}
	for index, step := range steps.Content {
		if yamlScalarValue(yamlMappingValue(step, "name")) == stepName {
			return fmt.Sprintf("steps[%d]", index), step.Line
		}
	}
	return "", 0
}

func yamlStepKeyLine(root *yaml.Node, stepName string, key string) int {
	steps := yamlMappingValue(root, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return 0
	}
	for _, step := range steps.Content {
		if yamlScalarValue(yamlMappingValue(step, "name")) == stepName {
			return yamlMappingKeyLine(step, key)
		}
	}
	return 0
}

func firstQuotedValueAfter(message, prefix string) string {
	idx := strings.Index(message, prefix+"'")
	if idx < 0 {
		idx = strings.Index(message, prefix+"\"")
	}
	if idx < 0 {
		return ""
	}
	rest := message[idx+len(prefix):]
	if len(rest) < 2 {
		return ""
	}
	quote := rest[0]
	end := strings.IndexByte(rest[1:], quote)
	if end < 0 {
		return ""
	}
	return rest[1 : 1+end]
}

func firstTokenAfter(message, prefix string) string {
	idx := strings.Index(message, prefix)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(message[idx+len(prefix):])
	if rest == "" {
		return ""
	}
	token := strings.Fields(rest)[0]
	return strings.Trim(token, ".,;:'\"")
}
