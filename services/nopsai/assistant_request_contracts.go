package nopsai

import (
	"fmt"
	"strings"
)

type assistantRequestContract struct {
	ID                              string                        `json:"id"`
	Reason                          string                        `json:"reason"`
	RequiredAnyTools                []string                      `json:"required_any_tools,omitempty"`
	RequiredAllTools                []string                      `json:"required_all_tools,omitempty"`
	RequiredArgs                    []assistantRequestContractArg `json:"required_args,omitempty"`
	FinalAnswerRequiresToolEvidence bool                          `json:"final_answer_requires_tool_evidence"`
	WrongEvidenceNotes              []string                      `json:"wrong_evidence_notes,omitempty"`
}

type assistantRequestContractArg struct {
	Tool  string   `json:"tool"`
	Keys  []string `json:"keys"`
	Value string   `json:"value,omitempty"`
	Match string   `json:"match,omitempty"`
}

func assistantPlannerRequestRequirements(plan assistantTurnPlan) map[string]any {
	contracts := assistantRequestContractsForPlan(plan)
	requirements := map[string]any{
		"contracts": contracts,
	}
	for _, contract := range contracts {
		if contract.ID == "ai_token_usage" {
			requirements["ai_token_usage_evidence_required"] = true
			requirements["required_tool"] = "nopsai.get_monitoring_ai_usage"
			if plan.RunID != "" {
				requirements["required_run_filter"] = map[string]any{
					"run_id": plan.RunID,
					"args":   []string{"run_id", "runId"},
				}
			}
		}
	}
	return requirements
}

func assistantValidateToolPlanMatchesRequest(plan assistantTurnPlan) error {
	for _, contract := range assistantRequestContractsForPlan(plan) {
		if err := assistantValidatePlanAgainstContract(plan, contract); err != nil {
			return err
		}
	}
	return nil
}

func assistantValidatePlannerFinalAnswer(plan assistantTurnPlan, toolCalls []assistantToolActivity) error {
	for _, contract := range assistantRequestContractsForPlan(plan) {
		if !contract.FinalAnswerRequiresToolEvidence {
			continue
		}
		if !assistantToolCallsSatisfyContractEvidence(toolCalls, contract) {
			return fmt.Errorf("assistant planner cannot answer %s without successful evidence from %s", contract.ID, assistantContractEvidenceTools(contract))
		}
	}
	return nil
}

func assistantRequestContractsForPlan(plan assistantTurnPlan) []assistantRequestContract {
	lower := strings.ToLower(strings.TrimSpace(plan.LowerContent))
	contracts := []assistantRequestContract{}
	if lower == "" {
		return contracts
	}
	if assistantRequestRequiresAIUsageEvidence(lower) {
		contract := assistantRequestContract{
			ID:                              "ai_token_usage",
			Reason:                          "Token usage questions must be answered from monitoring AI usage analytics.",
			RequiredAnyTools:                []string{"nopsai.get_monitoring_ai_usage"},
			FinalAnswerRequiresToolEvidence: true,
			WrongEvidenceNotes:              []string{"Pipeline run status, logs, and failure-analysis tools do not report token counts."},
		}
		if plan.RunID != "" {
			contract.RequiredArgs = append(contract.RequiredArgs, assistantRequestContractArg{
				Tool:  "nopsai.get_monitoring_ai_usage",
				Keys:  []string{"run_id", "runId"},
				Value: plan.RunID,
				Match: "equals",
			})
		}
		contracts = append(contracts, contract)
	}
	if assistantRequestAsksFeatureCapabilities(lower) {
		contracts = append(contracts, assistantRequestContract{
			ID:                              "feature_capabilities",
			Reason:                          "Feature, permission, and sensitive-value policy questions must use the current-user capability catalog.",
			RequiredAnyTools:                []string{"nopsai.get_feature_capabilities"},
			FinalAnswerRequiresToolEvidence: true,
		})
	}
	if assistantRequestAsksVariableUsageMetadata(lower) {
		contracts = append(contracts, assistantRequestContract{
			ID:                              "variable_usage_metadata",
			Reason:                          "Repeated variable usage questions require metadata-only variable analysis.",
			RequiredAnyTools:                []string{"nopsai.analyze_variable_usage"},
			FinalAnswerRequiresToolEvidence: true,
			WrongEvidenceNotes:              []string{"Plain variable value reads are not needed to count repeated variable metadata."},
		})
	}
	if assistantRequestAsksScopeSecretCounts(lower) {
		contract := assistantRequestContract{
			ID:                              "scope_secret_counts",
			Reason:                          "Scope/secret count questions require secret-scope metadata.",
			RequiredAnyTools:                []string{"nopsai.list_secret_scopes"},
			FinalAnswerRequiresToolEvidence: true,
		}
		if assistantRequestAsksScopeInventoryWithSecretCounts(lower) {
			contract.RequiredAllTools = append(contract.RequiredAllTools, "nopsai.list_scopes")
		}
		contracts = append(contracts, contract)
	}
	if assistantRequestAsksPipelineSearch(lower) {
		contract := assistantRequestContract{
			ID:                              "pipeline_search",
			Reason:                          "Pipeline search questions must use searchable pipeline metadata/YAML evidence.",
			RequiredAnyTools:                []string{"nopsai.search_pipelines"},
			FinalAnswerRequiresToolEvidence: true,
		}
		if assistantRequestMentionsApproval(lower) {
			contract.RequiredArgs = append(contract.RequiredArgs, assistantRequestContractArg{
				Tool:  "nopsai.search_pipelines",
				Keys:  []string{"query"},
				Value: "approval",
				Match: "contains",
			})
		}
		contracts = append(contracts, contract)
	}
	if assistantRequestAsksPipelineGeneration(lower) {
		contracts = append(contracts, assistantRequestContract{
			ID:                              "pipeline_generation",
			Reason:                          "Pipeline draft requests must use the pipeline generator so YAML comes from the hosted MCP proposal path.",
			RequiredAnyTools:                []string{"nopsai.generate_pipeline"},
			RequiredArgs:                    []assistantRequestContractArg{{Tool: "nopsai.generate_pipeline", Keys: []string{"goal"}}},
			FinalAnswerRequiresToolEvidence: true,
			WrongEvidenceNotes:              []string{"Do not answer pipeline-generation requests by writing YAML directly from the planner."},
		})
	}
	if assistantRequestRequiresPipelineValidation(plan, lower) {
		contracts = append(contracts, assistantRequestContract{
			ID:                              "pipeline_yaml_validation",
			Reason:                          "Pipeline YAML validation questions must use the pipeline validator.",
			RequiredAnyTools:                []string{"nopsai.validate_pipeline"},
			RequiredArgs:                    []assistantRequestContractArg{{Tool: "nopsai.validate_pipeline", Keys: []string{"yaml"}}},
			FinalAnswerRequiresToolEvidence: true,
		})
	}
	if plan.APIMethod != "" && plan.APIPath != "" {
		contracts = append(contracts, assistantRequestContract{
			ID:               "explicit_api_call",
			Reason:           "Explicit REST route requests must use the guarded API bridge.",
			RequiredAnyTools: []string{"nopsai.call_api"},
			RequiredArgs: []assistantRequestContractArg{
				{Tool: "nopsai.call_api", Keys: []string{"method"}, Value: plan.APIMethod, Match: "equals"},
				{Tool: "nopsai.call_api", Keys: []string{"path"}, Value: plan.APIPath, Match: "equals"},
			},
			FinalAnswerRequiresToolEvidence: true,
		})
	}
	return contracts
}

func assistantValidatePlanAgainstContract(plan assistantTurnPlan, contract assistantRequestContract) error {
	if len(contract.RequiredAnyTools) > 0 && !assistantPlanIncludesAnyTool(plan, contract.RequiredAnyTools...) {
		return fmt.Errorf("assistant plan for %s must use one of %s", contract.ID, contract.RequiredAnyTools)
	}
	for _, tool := range contract.RequiredAllTools {
		if !assistantPlanIncludesTool(plan, tool) {
			return fmt.Errorf("assistant plan for %s must use %s", contract.ID, tool)
		}
	}
	for _, requiredArg := range contract.RequiredArgs {
		if !assistantPlanIncludesRequiredArg(plan, requiredArg) {
			return fmt.Errorf("assistant plan for %s must pass %s argument %s", contract.ID, requiredArg.Tool, assistantContractArgDescription(requiredArg))
		}
	}
	return nil
}

func assistantToolCallsSatisfyContractEvidence(toolCalls []assistantToolActivity, contract assistantRequestContract) bool {
	for _, tool := range contract.RequiredAllTools {
		if !assistantToolCallsIncludeSuccessfulTool(toolCalls, tool, contract.RequiredArgs) {
			return false
		}
	}
	if len(contract.RequiredAnyTools) == 0 {
		return true
	}
	for _, tool := range contract.RequiredAnyTools {
		if assistantToolCallsIncludeSuccessfulTool(toolCalls, tool, contract.RequiredArgs) {
			return true
		}
	}
	return false
}

func assistantContractEvidenceToolNames(contract assistantRequestContract) []string {
	seen := map[string]struct{}{}
	tools := []string{}
	for _, tool := range contract.RequiredAllTools {
		if strings.TrimSpace(tool) == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		tools = append(tools, tool)
	}
	for _, tool := range contract.RequiredAnyTools {
		if strings.TrimSpace(tool) == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		tools = append(tools, tool)
	}
	return tools
}

func assistantContractEvidenceTools(contract assistantRequestContract) []string {
	tools := assistantContractEvidenceToolNames(contract)
	if len(tools) == 0 {
		return []string{"validated hosted MCP tools"}
	}
	return tools
}

func assistantPlanIncludesAnyTool(plan assistantTurnPlan, toolNames ...string) bool {
	for _, toolName := range toolNames {
		if assistantPlanIncludesTool(plan, toolName) {
			return true
		}
	}
	return false
}

func assistantPlanIncludesTool(plan assistantTurnPlan, toolName string) bool {
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ToolName) == toolName {
			return true
		}
	}
	return false
}

func assistantPlanIncludesRequiredArg(plan assistantTurnPlan, required assistantRequestContractArg) bool {
	for _, step := range plan.Steps {
		if strings.TrimSpace(step.ToolName) != required.Tool {
			continue
		}
		if assistantArgMatchesRequirement(step.Args, required) {
			return true
		}
	}
	return false
}

func assistantToolCallsIncludeSuccessfulTool(toolCalls []assistantToolActivity, toolName string, requiredArgs []assistantRequestContractArg) bool {
	for _, call := range toolCalls {
		if call.Name != toolName || call.Status != assistantToolStatusSuccess {
			continue
		}
		matches := true
		for _, requiredArg := range requiredArgs {
			if requiredArg.Tool != toolName {
				continue
			}
			if !assistantArgMatchesRequirement(call.Input, requiredArg) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func assistantArgMatchesRequirement(args map[string]any, required assistantRequestContractArg) bool {
	value := assistantPlanArgString(args, required.Keys...)
	if strings.TrimSpace(required.Value) == "" {
		return value != ""
	}
	switch required.Match {
	case "contains":
		return strings.Contains(strings.ToLower(value), strings.ToLower(required.Value))
	default:
		return strings.EqualFold(value, required.Value)
	}
}

func assistantContractArgDescription(required assistantRequestContractArg) string {
	if strings.TrimSpace(required.Value) == "" {
		return fmt.Sprintf("%v with a non-empty value", required.Keys)
	}
	if required.Match == "contains" {
		return fmt.Sprintf("%v containing %q", required.Keys, required.Value)
	}
	return fmt.Sprintf("%v equal to %q", required.Keys, required.Value)
}

func assistantPlanArgString(args map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := args[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		default:
			rendered := strings.TrimSpace(fmt.Sprint(typed))
			if rendered != "" {
				return rendered
			}
		}
	}
	return ""
}

func assistantRequestRequiresAIUsageEvidence(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	if lower == "" {
		return false
	}
	if containsAny(lower, "access token", "personal token", "refresh token", "bearer token", "bootstrap token", "jwt", "oauth token", "api token") {
		return false
	}
	if containsAny(lower, "llm usage", "ai usage", "model usage", "llm token", "llm tokens", "ai token", "ai tokens") {
		return true
	}
	if !containsAny(lower, "token", "tokens") {
		return false
	}
	return containsAny(lower, "usage", "used", "use", "uses", "how many", "pipelinerun", "pipeline run", "run ", " run", "pipeline", "schedule", "model", "llm", "ai")
}

func assistantRequestAsksFeatureCapabilities(lower string) bool {
	return containsAny(
		lower,
		"mcp coverage",
		"mcp capability",
		"mcp capabilities",
		"mcp support",
		"hosted mcp",
		"support all with mcp",
		"support all nopsai features",
		"all nopsai features",
		"all features",
		"features with mcp",
		"feature coverage",
		"features can i use",
		"what features can i use",
		"assistant capabilities",
		"capabilities do i have",
		"tools/resources available",
		"tools available",
		"resources available",
		"available to my user",
	) || assistantRequestAsksSensitivePolicy(lower)
}

func assistantRequestAsksSensitivePolicy(lower string) bool {
	if !containsAny(lower, "policy", "prevent", "block", "hide", "showing", "show", "expose", "read") {
		return false
	}
	return containsAny(lower, "env", "envs", "environment variable", "environment variables", "secret", "secrets", "credential", "credentials", "token", "tokens", "password")
}

func assistantRequestAsksVariableUsageMetadata(lower string) bool {
	if !containsAny(lower, "variable", "variables", "env", "envs", "environment variable", "environment variables") {
		return false
	}
	return containsAny(lower, "repetitive", "repeated", "duplicate", "duplicates", "how many", "all scopes", "across scopes", "used in scopes")
}

func assistantRequestAsksScopeSecretCounts(lower string) bool {
	if !containsAny(lower, "scope", "scopes") || !containsAny(lower, "secret", "secrets") {
		return false
	}
	return containsAny(lower, "how many", "count", "counts", "total", "per scope", "by scope", "for each", "each scope", "every scope")
}

func assistantRequestAsksScopeInventoryWithSecretCounts(lower string) bool {
	return containsAny(lower, "how many scope", "how many scopes", "for each", "each scope", "every scope")
}

func assistantRequestAsksPipelineSearch(lower string) bool {
	if assistantRequestAsksPipelineGeneration(lower) {
		return false
	}
	if containsAny(lower, "search pipeline", "search pipelines", "find pipeline", "find pipelines", "look for pipeline", "look through pipeline", "search through pipeline") {
		return true
	}
	return containsAny(lower, "pipeline") && assistantRequestMentionsApproval(lower)
}

func assistantRequestAsksPipelineGeneration(lower string) bool {
	if !containsAny(lower, "pipeline", "yaml") {
		return false
	}
	if containsAny(lower, "pipeline goal", "goal is", "pipeline should", "pipeline that should", "generate pipeline", "create pipeline", "draft pipeline", "write pipeline") {
		return true
	}
	if containsAny(lower, "4 step", "4-step", "four step", "four-step", "last one is approval", "last step is approval", "last step approval", "final step is approval", "final approval") {
		return true
	}
	if containsAny(lower, "build and publish", "build & publish", "publish docker", "docker image", "container image") &&
		containsAny(lower, "give me", "create", "generate", "draft", "write", "make") {
		return true
	}
	return false
}

func assistantRequestMentionsApproval(lower string) bool {
	return containsAny(lower, "approval step", "approval gate", "manual approval", "has approval", "with approval", "needs approval", "requires approval")
}

func assistantRequestRequiresPipelineValidation(plan assistantTurnPlan, lower string) bool {
	if strings.TrimSpace(plan.YAML) == "" {
		return false
	}
	return containsAny(lower, "validate", "lint", "check", "schema", "semantic", "unsafe task")
}
