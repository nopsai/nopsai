package nopsai

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"nopsai/config"
)

const (
	assistantLLMPlannerToolName    = "nopsai.llm.plan"
	assistantMaxPlannerIterations  = 4
	assistantMaxPlannerAttempts    = 2
	assistantMaxPlannerSchemaTools = 18
	// Schema repair rounds allowed per planner decision. The planner is asked to
	// name the tool its goal needs, so more than one round of "here is the schema
	// you asked for" is normal rather than a sign of a confused plan.
	assistantMaxPlannerSchemaRepairs = 2

	// How many tools relevance alone may contribute. Structural context and mode
	// policy add on top of this; the rest of the budget stays for them.
	assistantPlannerMaxRoutedTools = 6

	// Discovery needs the purpose of a tool, not its full argument prose.
	assistantPlannerCatalogDescriptionLimit = 140
)

type assistantPlannerResult struct {
	Plan      assistantTurnPlan
	ToolCalls []assistantToolActivity
	Handled   bool
}

type assistantPlannerDecision struct {
	Goal               string                 `json:"goal"`
	Intent             string                 `json:"intent"`
	SuccessCriteria    string                 `json:"success_criteria"`
	Steps              []assistantPlannerStep `json:"steps"`
	FinalAnswer        string                 `json:"final_answer"`
	ClarifyingQuestion string                 `json:"clarifying_question"`
	NeedsMoreTools     *bool                  `json:"needs_more_tools"`
}

type assistantPlannerStep struct {
	Tool   string         `json:"tool"`
	Args   map[string]any `json:"args"`
	Reason string         `json:"reason"`
}

func assistantLLMPlannerActivity(profileName string, profile config.LLMProfile, status string, output map[string]any) *assistantToolActivity {
	if output == nil {
		output = map[string]any{}
	}
	output["profile"] = profileName
	output["provider"] = profile.Provider
	output["model"] = profile.Model
	return &assistantToolActivity{
		Name: assistantLLMPlannerToolName,
		Input: map[string]any{
			"profile":  profileName,
			"provider": profile.Provider,
			"model":    profile.Model,
		},
		Output:       output,
		Status:       status,
		ResourceURIs: []string{"nopsai://features"},
		Source:       "llm",
		Phase:        "planning",
		Confidence:   "medium",
		Purpose:      "Create and validate a safe execution plan before any MCP evidence calls are run.",
	}
}

func assistantBaseTurnPlan(content string) assistantTurnPlan {
	return assistantBaseTurnPlanWithPageContext(content, assistantConversationMemory{}, assistantPageContext{})
}

func assistantBaseTurnPlanWithPageContext(content string, memory assistantConversationMemory, pageContext assistantPageContext) assistantTurnPlan {
	content = strings.TrimSpace(content)
	lower := strings.ToLower(content)
	pageContext = normalizeAssistantPageContext(pageContext)
	plan := assistantTurnPlan{
		Intent:        "llm_planned",
		Goal:          content,
		LowerContent:  lower,
		PageContext:   pageContext,
		RunID:         assistantFirstUUID(content),
		YAML:          assistantYAMLFromMessage(content),
		PipelineName:  assistantPipelineNameFromMessage(content),
		PipelineID:    assistantPipelineIDFromMessage(content),
		Repository:    assistantFirstPatternTeam(assistantRepositoryPattern, content),
		ScheduleID:    assistantFirstPatternTeam(assistantScheduleIDPattern, content),
		Scope:         assistantScopeFromMessage(content),
		UserConfirmed: assistantFeatureConfirmed(lower),
	}
	plan.APIMethod, plan.APIPath = assistantAPICallFromMessage(content)
	if plan.RunID == "" {
		plan.RunID = assistantPageContextRunID(pageContext)
	}
	if plan.RunID == "" {
		plan.RunID = strings.TrimSpace(memory.SelectedRun)
	}
	if plan.PipelineID == "" {
		plan.PipelineID = assistantPageContextPipelineID(pageContext)
	}
	if plan.PipelineID == "" {
		plan.PipelineID = strings.Trim(strings.TrimSpace(memory.SelectedPipeline), "/")
	}
	if plan.Scope == "" {
		plan.Scope = assistantPageContextScope(pageContext)
	}
	if plan.Scope == "" {
		plan.Scope = strings.Trim(strings.TrimSpace(memory.SelectedScope), "/")
	}
	if plan.Repository == "" {
		plan.Repository = assistantPageContextRepository(pageContext)
	}
	if plan.ScheduleID == "" {
		plan.ScheduleID = assistantPageContextScheduleID(pageContext)
	}
	if plan.PipelineName == "" {
		plan.PipelineName = "generated-pipeline"
	}
	return plan
}

func assistantFillPlanStepContextArgs(plan assistantTurnPlan) assistantTurnPlan {
	if len(plan.Steps) == 0 {
		return plan
	}
	for idx := range plan.Steps {
		step := plan.Steps[idx]
		args := cloneAssistantArgs(step.Args)
		if args == nil {
			args = map[string]any{}
		}
		switch {
		case assistantToolAcceptsPipelineContext(step.ToolName):
			if !assistantPlanStepHasPipelineArg(args) {
				if pipeline := assistantPlanPipelineContext(plan); pipeline != "" {
					args["pipeline"] = pipeline
				}
			}
		case assistantToolAcceptsRunContext(step.ToolName):
			if stringArg(args, "run_id") == "" && plan.RunID != "" {
				args["run_id"] = plan.RunID
			}
		case assistantToolAcceptsScheduleContext(step.ToolName):
			if stringArg(args, "schedule_id") == "" && plan.ScheduleID != "" {
				args["schedule_id"] = plan.ScheduleID
			}
		case assistantToolAcceptsRepositoryContext(step.ToolName):
			if stringArg(args, "repository") == "" && plan.Repository != "" {
				args["repository"] = plan.Repository
			}
		}
		step.Args = args
		plan.Steps[idx] = step
	}
	return plan
}

func assistantPlanStepHasPipelineArg(args map[string]any) bool {
	_, name := splitPipelineArg(args)
	return strings.TrimSpace(name) != ""
}

func assistantPlanPipelineContext(plan assistantTurnPlan) string {
	pipeline := strings.Trim(strings.TrimSpace(plan.PipelineID), "/")
	if pipeline == "" {
		return ""
	}
	if strings.Contains(pipeline, "/") || strings.TrimSpace(plan.Scope) == "" {
		return pipeline
	}
	return strings.Trim(strings.TrimSpace(plan.Scope), "/") + "/" + pipeline
}

func assistantToolAcceptsPipelineContext(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "nopsai.analyze_pipeline",
		"nopsai.get_pipeline",
		"nopsai.get_pipeline_knowledge_context",
		"nopsai.get_pipeline_efficiency",
		"nopsai.compare_pipelines",
		"nopsai.explain_pipeline_health",
		"nopsai.find_optimization_opportunities",
		"nopsai.get_monitoring_summary",
		"nopsai.get_monitoring_run_analytics",
		"nopsai.get_monitoring_pipeline_performance",
		"nopsai.get_monitoring_step_performance",
		"nopsai.get_monitoring_task_performance",
		"nopsai.get_monitoring_ai_usage",
		"nopsai.get_monitoring_reliability",
		"nopsai.get_monitoring_efficiency",
		"nopsai.get_monitoring_security",
		"nopsai.get_monitoring_runner_history":
		return true
	default:
		return false
	}
}

func assistantToolAcceptsRunContext(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "nopsai.get_pipeline_run",
		"nopsai.get_pipeline_run_output",
		"nopsai.get_pipeline_run_logs",
		"nopsai.analyze_pipeline_run_failure",
		"nopsai.analyze_run",
		"nopsai.get_lab_item",
		"nopsai.explain_lab_result":
		return true
	default:
		return false
	}
}

func assistantToolAcceptsScheduleContext(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "nopsai.get_schedule":
		return true
	default:
		return false
	}
}

func assistantToolAcceptsRepositoryContext(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "nopsai.get_trigger":
		return true
	default:
		return false
	}
}

func assistantUserExplicitlyTargetsTeam(lower string) bool {
	return assistantTextHasAny(lower, "team", "teams", "squad", "organisation", "organization", "group")
}

func assistantPlannerSchemaToolCatalog(tools []hostedMCPTool, schemaToolNames map[string]bool) []map[string]any {
	items := make([]map[string]any, 0, len(schemaToolNames))
	for _, tool := range tools {
		if !schemaToolNames[tool.Name] {
			continue
		}
		items = append(items, map[string]any{
			"name":         tool.Name,
			"input_schema": assistantPlannerCompactInputSchema(tool.InputSchema),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return fmt.Sprint(items[i]["name"]) < fmt.Sprint(items[j]["name"])
	})
	return items
}

func assistantPlannerWantsPipelineProposalSchema(lower string, plan assistantTurnPlan) bool {
	if assistantPlannerWantsGitOpsProposalSchema(lower) {
		return true
	}
	if plan.UserConfirmed {
		return true
	}
	return assistantTextHasAny(lower,
		"create pipeline",
		"new pipeline",
		"save pipeline",
		"update pipeline",
		"edit pipeline",
		"change pipeline",
		"delete pipeline",
		"add step",
		"remove step",
	)
}

// assistantPlannerWantsExampleSchema recognises "show me what this should look
// like", which must be answered from current docs rather than from memory.
func assistantPlannerWantsExampleSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"example", "examples", "sample", "samples", "template", "templates",
		"looks like", "look like", "how it is implemented", "how is it implemented",
		"working example", "starter", "boilerplate", "what should this",
	)
}

func assistantPlannerWantsGitOpsProposalSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"gitops",
		"git ops",
		"proposal",
		"propose",
		"file plan",
		"commit-ready",
		"commit ready",
		"pull request",
		"merge request",
		"without applying",
	)
}

func assistantPlannerWantsExposurePolicySchema(lower string) bool {
	return assistantTextHasAny(lower,
		"policy",
		"policies",
		"guardrail",
		"guardrails",
		"prevent",
		"block",
		"hide",
		"showing",
		"expose",
		"redact",
		"plaintext",
	) && assistantTextHasAny(lower,
		"env",
		"envs",
		"environment variable",
		"environment variables",
		"secret",
		"secrets",
		"credential",
		"credentials",
		"token",
		"password",
		"sensitive",
	)
}

func assistantPlannerWantsChangeSchema(lower string) bool {
	return assistantPlannerWantsWriteSchema(lower) || assistantPlannerWantsDeleteSchema(lower)
}

func assistantPlannerWantsWriteSchema(lower string) bool {
	return strings.Contains(lower, "=") || assistantTextHasAny(lower,
		"add ",
		"change ",
		"create ",
		"enable ",
		"set ",
		"update ",
		"write ",
		// Operational verbs are change requests too. Missing one used to be
		// harmless because a fallback re-admitted the tool anyway; now that mode
		// selection decides what may appear at all, the verb list is the decision.
		"pause ",
		"resume ",
		"drain ",
		"eject ",
		"cancel ",
		"stop ",
		"start ",
		"restart ",
		"rotate ",
		"disable ",
		"approve ",
		"reject ",
		"trigger ",
		"rerun ",
		"retry ",
		"sync ",
		"refresh ",
		"activate ",
		"deactivate ",
	)
}

func assistantPlannerWantsDeleteSchema(lower string) bool {
	return assistantTextHasAny(lower,
		"delete ",
		"disable ",
		"remove ",
	)
}

func assistantTextHasAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func assistantPlannerCompactInputSchema(schema map[string]any) map[string]any {
	compact := map[string]any{"type": "object"}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return compact
	}
	propertyTypes := map[string]string{}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		property, _ := properties[key].(map[string]any)
		propertyTypes[key] = assistantPlannerSchemaTypeLabel(property)
	}
	compact["properties"] = propertyTypes
	return compact
}

func assistantPlannerSchemaTypeLabel(schema map[string]any) string {
	schemaType, _ := schema["type"].(string)
	schemaType = strings.TrimSpace(schemaType)
	if schemaType == "" {
		return "any"
	}
	return schemaType
}

func parseAssistantPlannerDecision(text string) (assistantPlannerDecision, error) {
	raw := assistantExtractJSONObject(text)
	if raw == "" {
		return assistantPlannerDecision{}, fmt.Errorf("planner response did not contain a JSON object")
	}
	var decision assistantPlannerDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return assistantPlannerDecision{}, fmt.Errorf("parse planner JSON: %w", err)
	}
	for idx := range decision.Steps {
		if decision.Steps[idx].Args == nil {
			decision.Steps[idx].Args = map[string]any{}
		}
	}
	return decision, nil
}

func assistantExtractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "```") {
		parts := strings.Split(text, "```")
		for _, part := range parts {
			candidate := strings.TrimSpace(part)
			if candidate == "" {
				continue
			}
			lines := strings.Split(candidate, "\n")
			if len(lines) > 1 && !strings.HasPrefix(strings.TrimSpace(lines[0]), "{") {
				candidate = strings.TrimSpace(strings.Join(lines[1:], "\n"))
			}
			if strings.HasPrefix(candidate, "{") {
				text = candidate
				break
			}
		}
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return ""
	}
	return strings.TrimSpace(text[start : end+1])
}

func assistantEvidenceToolCalls(toolCalls []assistantToolActivity) []assistantToolActivity {
	filtered := make([]assistantToolActivity, 0, len(toolCalls))
	for _, call := range toolCalls {
		switch call.Name {
		case assistantLLMPlannerToolName, assistantLLMToolName, assistantExecutionPlanToolName, "nopsai.assistant_plan":
			continue
		default:
			filtered = append(filtered, call)
		}
	}
	return filtered
}
