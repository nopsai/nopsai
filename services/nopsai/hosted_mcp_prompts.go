package nopsai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
)

// Prompts are the flows NopsAI answers well, written down so a client does not
// have to discover them from 224 tool descriptions. Each one names the tool that
// does the work and states how the answer should be shaped, which is the part a
// general model gets wrong when left to improvise.
//
// A prompt is listed only when the subject may call the tool behind it: offering
// a workflow that will be refused is worse than not offering it.

type hostedMCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type hostedMCPPrompt struct {
	Name        string                         `json:"name"`
	Title       string                         `json:"title,omitempty"`
	Description string                         `json:"description"`
	Arguments   []hostedMCPPromptArgument      `json:"arguments,omitempty"`
	Tool        string                         `json:"-"`
	Render      func(map[string]string) string `json:"-"`
}

func allHostedMCPPrompts() []hostedMCPPrompt {
	return []hostedMCPPrompt{
		{
			Name:        "review-team",
			Title:       "Review a team",
			Description: "Delivery and ownership review of one team: what is wrong, ranked, with the next thing to look at.",
			Arguments: []hostedMCPPromptArgument{
				{Name: "team", Description: "Team id, path, or slug. Use * for every team the user can see.", Required: true},
				{Name: "days", Description: "Window length in days. Defaults to 30."},
			},
			Tool: "nopsai.analyze_team",
			Render: func(args map[string]string) string {
				return fmt.Sprintf(
					"Review the team %q using nopsai.analyze_team%s.\n\n"+
						"Report the health score, then the findings in severity order, each with the evidence behind it. "+
						"Say plainly which one to fix first and why. End with the tool call from next_actions that investigates it.\n"+
						"Do not restate every metric, and do not invent a severity or score of your own: the analysis computed them.",
					args["team"], hostedMCPPromptWindowClause(args["days"]))
			},
		},
		{
			Name:        "explain-run-failure",
			Title:       "Explain a failed run",
			Description: "Diagnose one pipeline run: the likely domain, the first failure point, and what changed since it last worked.",
			Arguments: []hostedMCPPromptArgument{
				{Name: "run_id", Description: "The run id to diagnose.", Required: true},
			},
			Tool: "nopsai.analyze_run",
			Render: func(args map[string]string) string {
				return fmt.Sprintf(
					"Diagnose run %q using nopsai.analyze_run.\n\n"+
						"Lead with whose problem it is — the primary_diagnosis domain and its confidence — then the first failure point, "+
						"then what changed since the last successful run. If the analysis could not read the logs, say so rather than guessing at the cause.\n"+
						"Finish with the single next call worth making.",
					args["run_id"])
			},
		},
		{
			Name:        "review-pipeline",
			Title:       "Review a pipeline",
			Description: "Review one pipeline's reliability, duration, spend, and its stored definition before you change it.",
			Arguments: []hostedMCPPromptArgument{
				{Name: "pipeline", Description: "Pipeline id as path/name.", Required: true},
				{Name: "days", Description: "Window length in days. Defaults to 30."},
			},
			Tool: "nopsai.analyze_pipeline",
			Render: func(args map[string]string) string {
				return fmt.Sprintf(
					"Review the pipeline %q using nopsai.analyze_pipeline%s.\n\n"+
						"Separate what the runs show from what the definition shows: a slow pipeline and an unsafe one are different problems. "+
						"Quote the redacted evidence for any security finding rather than the value behind it.\n"+
						"End with the change you would make first, and whether it needs a GitOps proposal or a runtime change.",
					args["pipeline"], hostedMCPPromptWindowClause(args["days"]))
			},
		},
		{
			Name:        "platform-spend",
			Title:       "Explain platform AI spend",
			Description: "Where the AI spend went in a window, and which change would move it.",
			Arguments: []hostedMCPPromptArgument{
				{Name: "days", Description: "Window length in days. Defaults to 30."},
			},
			Tool: "nopsai.get_monitoring_ai_usage",
			Render: func(args map[string]string) string {
				return fmt.Sprintf(
					"Explain where AI spend went%s, using nopsai.get_monitoring_ai_usage.\n\n"+
						"Report money, not tokens: tokens are the input to the figure, not the figure. "+
						"Name the pipeline, model, or feature that dominates, and say what would actually reduce it.\n"+
						"If any turn could not be priced, say how many rather than treating them as free.",
					hostedMCPPromptWindowClause(args["days"]))
			},
		},
		{
			Name:        "prepare-gitops-change",
			Title:       "Prepare a reviewable change",
			Description: "Turn an intent into a GitOps file plan that changes nothing until a human applies it.",
			Arguments: []hostedMCPPromptArgument{
				{Name: "intent", Description: "What should change, in plain language.", Required: true},
			},
			Tool: "nopsai.get_feature_capabilities",
			Render: func(args map[string]string) string {
				return fmt.Sprintf(
					"Prepare a reviewable change for this intent: %q.\n\n"+
						"Read the current definition first, then produce a file plan with a nopsai.propose_* tool. Do not call a runtime write tool: "+
						"the point of this flow is that nothing is applied until a person reviews the plan.\n"+
						"Validate any pipeline YAML with nopsai.validate_pipeline before proposing it, and show what the change alters.",
					args["intent"])
			},
		},
	}
}

func hostedMCPPromptWindowClause(days string) string {
	days = strings.TrimSpace(days)
	if days == "" {
		return ""
	}
	return " over the last " + days + " days"
}

func (a *App) hostedMCPPromptsForSubject(ctx context.Context, subject aaamodel.Subject) []hostedMCPPrompt {
	available := map[string]bool{}
	for _, tool := range a.hostedMCPToolsForSubject(ctx, subject) {
		available[tool.Name] = true
	}
	all := allHostedMCPPrompts()
	prompts := make([]hostedMCPPrompt, 0, len(all))
	for _, prompt := range all {
		if available[prompt.Tool] {
			prompts = append(prompts, prompt)
		}
	}
	return prompts
}

func (a *App) hostedMCPPromptByName(ctx context.Context, subject aaamodel.Subject, name string) (hostedMCPPrompt, bool) {
	for _, prompt := range a.hostedMCPPromptsForSubject(ctx, subject) {
		if prompt.Name == name {
			return prompt, true
		}
	}
	return hostedMCPPrompt{}, false
}

type hostedMCPPromptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
}

func hostedMCPPromptGetParamsFrom(params json.RawMessage) hostedMCPPromptGetParams {
	decoded := hostedMCPPromptGetParams{Arguments: map[string]string{}}
	if len(params) == 0 {
		return decoded
	}
	if err := json.Unmarshal(params, &decoded); err != nil {
		return hostedMCPPromptGetParams{Arguments: map[string]string{}}
	}
	if decoded.Arguments == nil {
		decoded.Arguments = map[string]string{}
	}
	decoded.Name = strings.TrimSpace(decoded.Name)
	return decoded
}

func hostedMCPPromptMissingArguments(prompt hostedMCPPrompt, args map[string]string) []string {
	missing := []string{}
	for _, argument := range prompt.Arguments {
		if !argument.Required {
			continue
		}
		if strings.TrimSpace(args[argument.Name]) == "" {
			missing = append(missing, argument.Name)
		}
	}
	return missing
}

func hostedMCPPromptMessages(prompt hostedMCPPrompt, args map[string]string) map[string]any {
	return map[string]any{
		"description": prompt.Description,
		"messages": []map[string]any{{
			"role": "user",
			"content": map[string]any{
				"type": "text",
				"text": prompt.Render(args),
			},
		}},
	}
}
