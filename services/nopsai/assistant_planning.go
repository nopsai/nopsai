package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"nopsai/config"
	aaamodel "nopsai/services/aaa/pkg/model"
)

const (
	assistantMaxPlanToolCalls      = 8
	assistantMaxPlanArgKeys        = 40
	assistantMaxPlanArgDepth       = 8
	assistantMaxPlanArgStringBytes = 256 * 1024
	assistantMaxPlanArgListItems   = 200
)

type assistantPlanStep struct {
	Thought  string
	ToolName string
	Args     map[string]any
}

type assistantAnswerQuality struct {
	HasDirectAnswer      bool
	UsedRelevantTools    bool
	PipelineGrounded     bool
	EmptyResultExplained bool
	SuggestedNextStep    bool
	NoFakeData           bool
}

func (a *App) validateAssistantToolPlan(ctx context.Context, subject aaamodel.Subject, plan assistantTurnPlan) error {
	if len(plan.Steps) == 0 {
		return nil
	}
	if len(plan.Steps) > assistantMaxPlanToolCalls {
		return fmt.Errorf("assistant plan has %d tool calls; max allowed is %d", len(plan.Steps), assistantMaxPlanToolCalls)
	}
	for idx, step := range plan.Steps {
		toolName := strings.TrimSpace(step.ToolName)
		if toolName == "" {
			return fmt.Errorf("assistant plan step %d has no tool", idx+1)
		}
		if len(step.Args) > assistantMaxPlanArgKeys {
			return fmt.Errorf("assistant plan step %d has too many arguments", idx+1)
		}
		if err := assistantValidatePlanArgs(step.Args, 0); err != nil {
			return fmt.Errorf("assistant plan step %d has unsafe arguments: %w", idx+1, err)
		}
		tool, ok := a.hostedMCPToolByName(ctx, subject, toolName)
		if !ok {
			return fmt.Errorf("assistant plan step %d requested unavailable tool %q", idx+1, toolName)
		}
		if err := assistantValidatePlanArgsAgainstToolSchema(step.Args, tool.InputSchema); err != nil {
			return fmt.Errorf("assistant plan step %d has arguments that do not match %s input schema: %w", idx+1, toolName, err)
		}
		userConfirmed := plan.UserConfirmed || assistantFeatureConfirmed(plan.LowerContent)
		if assistantToolRequiresActionExecution(tool) &&
			boolArg(step.Args, "confirm", false) &&
			!assistantPlannedToolIsProposal(tool.Name) &&
			!userConfirmed {
			return fmt.Errorf("assistant plan step %d requested mutating tool %q with confirm:true but the user did not explicitly confirm", idx+1, toolName)
		}
		if assistantToolRequiresActionExecution(tool) &&
			configuredAssistantRequiresConfirm(a) &&
			!assistantPlannedToolIsProposal(tool.Name) &&
			!boolArg(step.Args, "confirm", false) {
			return fmt.Errorf("assistant plan step %d requested mutating tool %q without confirm:true", idx+1, toolName)
		}
	}
	return nil
}

func assistantValidatePlanArgsAgainstToolSchema(args map[string]any, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if schemaType, _ := schema["type"].(string); schemaType != "" && !assistantSchemaTypeMatches(schemaType, args) {
		return fmt.Errorf("root must be %s", schemaType)
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	for key, value := range args {
		rawProperty, ok := properties[key]
		if !ok {
			continue
		}
		property, _ := rawProperty.(map[string]any)
		if len(property) == 0 {
			continue
		}
		if err := assistantValidateValueAgainstSchema(value, property, key); err != nil {
			return err
		}
	}
	return nil
}

func assistantValidateValueAgainstSchema(value any, schema map[string]any, path string) error {
	schemaType, _ := schema["type"].(string)
	if schemaType == "" {
		return nil
	}
	if !assistantSchemaTypeMatches(schemaType, value) {
		return fmt.Errorf("%s must be %s", path, schemaType)
	}
	if schemaType != "object" {
		return nil
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for key, nestedValue := range mapped {
		rawProperty, ok := properties[key]
		if !ok {
			continue
		}
		property, _ := rawProperty.(map[string]any)
		if len(property) == 0 {
			continue
		}
		if err := assistantValidateValueAgainstSchema(nestedValue, property, path+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func assistantSchemaTypeMatches(schemaType string, value any) bool {
	switch strings.ToLower(strings.TrimSpace(schemaType)) {
	case "", "any":
		return true
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		switch value.(type) {
		case []any, []string:
			return true
		default:
			return false
		}
	case "string":
		_, ok := value.(string)
		return ok
	case "number", "integer":
		switch value.(type) {
		case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	default:
		return true
	}
}

func assistantValidatePlanArgs(value any, depth int) error {
	if depth > assistantMaxPlanArgDepth {
		return fmt.Errorf("max depth %d exceeded", assistantMaxPlanArgDepth)
	}
	switch typed := value.(type) {
	case nil, bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return nil
	case string:
		if len(typed) > assistantMaxPlanArgStringBytes {
			return fmt.Errorf("string argument exceeds %d bytes", assistantMaxPlanArgStringBytes)
		}
		return nil
	case map[string]any:
		if len(typed) > assistantMaxPlanArgKeys {
			return fmt.Errorf("object has %d keys; max allowed is %d", len(typed), assistantMaxPlanArgKeys)
		}
		for key, item := range typed {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("blank argument key")
			}
			if err := assistantValidatePlanArgs(item, depth+1); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
		return nil
	case []any:
		if len(typed) > assistantMaxPlanArgListItems {
			return fmt.Errorf("array has %d items; max allowed is %d", len(typed), assistantMaxPlanArgListItems)
		}
		for idx, item := range typed {
			if err := assistantValidatePlanArgs(item, depth+1); err != nil {
				return fmt.Errorf("[%d]: %w", idx, err)
			}
		}
		return nil
	case []string:
		if len(typed) > assistantMaxPlanArgListItems {
			return fmt.Errorf("array has %d items; max allowed is %d", len(typed), assistantMaxPlanArgListItems)
		}
		for idx, item := range typed {
			if err := assistantValidatePlanArgs(item, depth+1); err != nil {
				return fmt.Errorf("[%d]: %w", idx, err)
			}
		}
		return nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Errorf("value is not JSON-serializable")
		}
		if len(encoded) > assistantMaxPlanArgStringBytes {
			return fmt.Errorf("encoded value exceeds %d bytes", assistantMaxPlanArgStringBytes)
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			return fmt.Errorf("value is not valid JSON")
		}
		return assistantValidatePlanArgs(decoded, depth+1)
	}
}

func configuredAssistantRequiresConfirm(a *App) bool {
	return a == nil || config.AssistantRequireConfirmation(a.assistantConfig().Actions)
}

func assistantFeatureConfirmed(lower string) bool {
	return containsAny(lower, "confirm", "confirmed", "with confirmation", "i approve", "approved to execute", "apply it", "execute it")
}

func assistantPlannedToolIsProposal(name string) bool {
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, "nopsai.propose_") ||
		strings.HasPrefix(name, "nopsai.plan_") ||
		strings.HasPrefix(name, "nopsai.preview_") ||
		name == "nopsai.validate_pipeline"
}

func assistantAssessAnswerQuality(plan assistantTurnPlan, toolCalls []assistantToolActivity, reply string) assistantAnswerQuality {
	reply = strings.TrimSpace(reply)
	lower := strings.ToLower(reply)
	quality := assistantAnswerQuality{
		HasDirectAnswer:      reply != "",
		UsedRelevantTools:    plan.Intent == "clarify" || plan.FinalAnswer != "" || len(assistantEvidenceToolCalls(toolCalls)) > 0 || assistantHasPlanDenial(toolCalls),
		PipelineGrounded:     true,
		EmptyResultExplained: true,
		SuggestedNextStep:    true,
		NoFakeData:           !assistantReplyClaimsApplied(lower) || assistantAnyToolApplied(toolCalls),
	}
	if assistantPlanRequiresPipelineGrounding(plan, toolCalls) {
		quality.PipelineGrounded = assistantHasSuccessfulPipelineEvidence(toolCalls)
	}
	if plan.Intent == "ai_token_usage" && !assistantAnyAIUsageCallHasEvents(assistantToolCallsByName(toolCalls, "nopsai.get_monitoring_ai_usage")) {
		quality.EmptyResultExplained = containsAny(lower, "no visible ai usage", "no visible usage", "no visible token", "0 tokens", "zero")
	}
	if assistantPlanNeedsProposalSafetyLanguage(plan) {
		quality.SuggestedNextStep = containsAny(lower, "no changes were applied") &&
			containsAny(lower, "review", "commit", "gitops", "confirm", "proposal", "write plan")
	}
	return quality
}

func assistantHasPlanDenial(toolCalls []assistantToolActivity) bool {
	_, ok := assistantFirstPlanDenial(toolCalls)
	return ok
}

func assistantAnswerQualityPasses(quality assistantAnswerQuality) bool {
	return quality.HasDirectAnswer &&
		quality.UsedRelevantTools &&
		quality.PipelineGrounded &&
		quality.EmptyResultExplained &&
		quality.SuggestedNextStep &&
		quality.NoFakeData
}

func assistantPlanRequiresPipelineGrounding(plan assistantTurnPlan, toolCalls []assistantToolActivity) bool {
	switch plan.Intent {
	case "pipeline", "search_pipelines", "validate_pipeline", "propose_pipeline_create", "propose_pipeline_update", "pipeline_knowledge_context":
		return true
	}
	if plan.PipelineID != "" || (plan.PipelineName != "" && plan.PipelineName != "generated-pipeline") || plan.YAML != "" {
		return true
	}
	for _, step := range plan.Steps {
		if assistantToolProvidesPipelineEvidence(step.ToolName) {
			return true
		}
	}
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		if assistantToolProvidesPipelineEvidence(call.Name) {
			return true
		}
	}
	return false
}

func assistantHasSuccessfulPipelineEvidence(toolCalls []assistantToolActivity) bool {
	for _, call := range assistantEvidenceToolCalls(toolCalls) {
		if call.Status == assistantToolStatusSuccess && assistantToolProvidesPipelineEvidence(call.Name) {
			return true
		}
	}
	return false
}

func assistantToolProvidesPipelineEvidence(name string) bool {
	switch strings.TrimSpace(name) {
	case "nopsai.list_pipelines",
		"nopsai.search_pipelines",
		"nopsai.get_pipeline",
		"nopsai.validate_pipeline",
		"nopsai.propose_pipeline_create",
		"nopsai.propose_pipeline_update",
		"nopsai.get_pipeline_knowledge_context",
		"nopsai.propose_reusable_step_create",
		"nopsai.propose_reusable_step_update",
		"nopsai.propose_reusable_step_delete":
		return true
	default:
		return false
	}
}

func assistantPlanNeedsProposalSafetyLanguage(plan assistantTurnPlan) bool {
	switch plan.Intent {
	case "propose_pipeline_create", "propose_pipeline_update":
		return true
	case "feature_tool":
		for _, step := range plan.Steps {
			if assistantFeatureToolNeedsSafetyLanguage(step.ToolName) {
				return true
			}
		}
	case "trigger", "schedule":
		for _, step := range plan.Steps {
			if strings.HasPrefix(strings.TrimSpace(step.ToolName), "nopsai.propose_") {
				return true
			}
		}
	}
	return false
}

func assistantFeatureToolNeedsSafetyLanguage(name string) bool {
	name = strings.TrimSpace(name)
	if assistantPlannedToolIsProposal(name) {
		return true
	}
	withoutPrefix := strings.TrimPrefix(name, "nopsai.")
	for _, prefix := range []string{
		"acknowledge_",
		"activate_",
		"approve_",
		"bootstrap_",
		"cancel_",
		"create_",
		"delete_",
		"disable_",
		"enable_",
		"encrypt_",
		"invoke_",
		"reject_",
		"resolve_",
		"rerun_",
		"rotate_",
		"run_",
		"sync_",
		"test_",
		"update_",
		"write_",
	} {
		if strings.HasPrefix(withoutPrefix, prefix) {
			return true
		}
	}
	return false
}

func assistantReplyClaimsApplied(lowerReply string) bool {
	lowerReply = strings.ToLower(strings.TrimSpace(lowerReply))
	if lowerReply == "" ||
		containsAny(lowerReply, "no changes were applied", "nothing was applied", "not applied", "was not applied", "were not applied") {
		return false
	}
	return containsAny(
		lowerReply,
		"changes were applied",
		"change was applied",
		"i applied",
		"i updated",
		"i deleted",
		"i created",
		"i rotated",
		"i cancelled",
		"i reran",
		"has been applied",
		"have been applied",
	)
}

func assistantAnyToolApplied(toolCalls []assistantToolActivity) bool {
	for _, call := range toolCalls {
		if assistantOutputBool(call.Output, "applied") || assistantOutputBool(call.Output, "applies") {
			return true
		}
	}
	return false
}

func cloneAssistantArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func assistantAIUsageCallHasEvents(call assistantToolActivity) bool {
	if call.Status != assistantToolStatusSuccess {
		return false
	}
	output := assistantAIUsageOutput(call)
	if assistantOutputFloat(output, "total_tokens") > 0 {
		return true
	}
	if assistantOutputFloat(output, "exact_token_events")+assistantOutputFloat(output, "estimated_token_events") > 0 {
		return true
	}
	return len(assistantMapSlice(output["by_pipeline"])) > 0 || len(assistantMapSlice(output["top_token_runs"])) > 0
}

func assistantAIUsageOutput(call assistantToolActivity) map[string]any {
	if response, ok := call.Output["response"].(map[string]any); ok {
		return response
	}
	return call.Output
}

func assistantAIUsageFilterSummary(args map[string]any) string {
	labels := []string{}
	for _, key := range []string{"provider", "model", "llm_profile", "feature", "step_name", "task_name"} {
		if value := strings.TrimSpace(fmt.Sprint(args[key])); value != "" && value != "<nil>" {
			labels = append(labels, strings.ReplaceAll(key, "_", " ")+"="+value)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ", ")
}

func assistantToolCallsByName(toolCalls []assistantToolActivity, name string) []assistantToolActivity {
	matches := []assistantToolActivity{}
	for _, call := range toolCalls {
		if call.Name == name {
			matches = append(matches, call)
		}
	}
	return matches
}

func assistantBestAIUsageCall(calls []assistantToolActivity) assistantToolActivity {
	if len(calls) == 0 {
		return assistantToolActivity{Name: "nopsai.get_monitoring_ai_usage", Output: map[string]any{}}
	}
	for _, call := range calls {
		if assistantAIUsageCallHasEvents(call) {
			return call
		}
	}
	return calls[len(calls)-1]
}

func assistantAnyAIUsageCallHasEvents(calls []assistantToolActivity) bool {
	for _, call := range calls {
		if assistantAIUsageCallHasEvents(call) {
			return true
		}
	}
	return false
}

func assistantAIUsageWindowLabel(call assistantToolActivity) string {
	from := strings.TrimSpace(fmt.Sprint(call.Input["from"]))
	to := strings.TrimSpace(fmt.Sprint(call.Input["to"]))
	if from == "" || from == "<nil>" {
		return "default monitoring window"
	}
	if to == "" || to == "<nil>" {
		return "window from " + from
	}
	return "window from " + from + " to " + to
}

func assistantAppendTokenTeams(lines []string, call assistantToolActivity) []string {
	output := assistantAIUsageOutput(call)
	lines = assistantAppendTokenTeam(lines, "Highest token steps:", output["by_step"])
	lines = assistantAppendTokenTeam(lines, "Highest token tasks:", output["by_task"])
	lines = assistantAppendTokenTeam(lines, "Usage by provider:", output["by_provider"])
	lines = assistantAppendTokenTeam(lines, "Usage by model:", output["by_model"])
	lines = assistantAppendTokenTeam(lines, "Usage by LLM profile:", output["by_profile"])
	lines = assistantAppendTokenTeam(lines, "Usage by feature:", output["by_feature"])
	return lines
}

func assistantAppendTokenTeam(lines []string, title string, value any) []string {
	items := assistantMapSlice(value)
	if len(items) == 0 {
		return lines
	}
	lines = append(lines, "", title)
	for idx, item := range items {
		if idx >= 5 {
			break
		}
		label := firstNonEmptyString(assistantOutputString(item, "label"), assistantOutputString(item, "key"))
		if label == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %.0f tokens across %.0f events", label, assistantOutputFloat(item, "tokens"), assistantOutputFloat(item, "count")))
	}
	return lines
}

func assistantVisibleRunCount(toolCalls []assistantToolActivity) float64 {
	for _, call := range toolCalls {
		switch call.Name {
		case "nopsai.get_monitoring_summary":
			if call.Status == assistantToolStatusSuccess {
				if count := assistantOutputFloat(call.Output, "total_runs"); count > 0 {
					return count
				}
			}
		case "nopsai.list_pipeline_runs":
			if call.Status == assistantToolStatusSuccess {
				return float64(len(assistantMapSlice(call.Output["runs"])))
			}
		}
	}
	return 0
}
