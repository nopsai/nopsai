package nopsai

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	aaamodel "nopsai/services/aaa/pkg/model"
)

// Completion turns "type the pipeline id exactly right" into a list the user
// picks from. The candidates come from the same permission-filtered tools a call
// would use, so a completion never reveals the existence of something the caller
// could not otherwise read.

const hostedMCPCompletionLimit = 100

type hostedMCPCompletionRef struct {
	Type string `json:"type"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

type hostedMCPCompletionParams struct {
	Ref      hostedMCPCompletionRef `json:"ref"`
	Argument struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"argument"`
}

func hostedMCPCompletionParamsFrom(params json.RawMessage) hostedMCPCompletionParams {
	var decoded hostedMCPCompletionParams
	if len(params) == 0 {
		return decoded
	}
	if err := json.Unmarshal(params, &decoded); err != nil {
		return hostedMCPCompletionParams{}
	}
	decoded.Ref.Type = strings.TrimSpace(decoded.Ref.Type)
	decoded.Ref.Name = strings.TrimSpace(decoded.Ref.Name)
	decoded.Ref.URI = strings.TrimSpace(decoded.Ref.URI)
	decoded.Argument.Name = strings.TrimSpace(decoded.Argument.Name)
	return decoded
}

// hostedMCPCompletionKind maps a prompt argument or a resource template argument
// onto the inventory that answers it. Both surfaces share the vocabulary, so
// "pipeline" means the same list wherever it is asked for.
func hostedMCPCompletionKind(ref hostedMCPCompletionRef, argument string) string {
	switch argument {
	case "pipeline", "pipeline_id":
		return "pipeline"
	case "run_id":
		return "run"
	case "team", "team_id", "team_path":
		return "team"
	case "schedule_id":
		return "schedule"
	case "repository":
		return "trigger"
	case "subject_type":
		return "subject_type"
	case "days":
		return "days"
	}
	// A templated URI names its own subject even when the argument does not.
	switch {
	case strings.Contains(ref.URI, "pipeline-runs"):
		return "run"
	case strings.Contains(ref.URI, "pipelines"):
		return "pipeline"
	case strings.Contains(ref.URI, "teams"):
		return "team"
	}
	return ""
}

func (a *App) hostedMCPCompletionCandidates(ctx context.Context, subject aaamodel.Subject, kind string) []string {
	switch kind {
	case "subject_type":
		return []string{"team", "pipeline", "run"}
	case "days":
		return []string{"7", "14", "30", "90"}
	case "pipeline":
		return hostedMCPCompletionStrings(a.hostedMCPCompletionPayload(ctx, subject, "nopsai.list_pipelines", map[string]any{"limit": hostedMCPCompletionLimit}), "pipelines", "id", "name")
	case "run":
		return hostedMCPCompletionStrings(a.hostedMCPCompletionPayload(ctx, subject, "nopsai.list_pipeline_runs", map[string]any{"limit": hostedMCPCompletionLimit}), "runs", "run_id", "id")
	case "team":
		return hostedMCPCompletionStrings(a.hostedMCPCompletionPayload(ctx, subject, "nopsai.list_teams", map[string]any{"limit": hostedMCPCompletionLimit}), "teams", "path", "id")
	case "schedule":
		return hostedMCPCompletionStrings(a.hostedMCPCompletionPayload(ctx, subject, "nopsai.list_schedules", map[string]any{"limit": hostedMCPCompletionLimit}), "schedules", "identifier", "id")
	case "trigger":
		return hostedMCPCompletionStrings(a.hostedMCPCompletionPayload(ctx, subject, "nopsai.list_triggers", map[string]any{"limit": hostedMCPCompletionLimit}), "triggers", "repository", "id")
	}
	return nil
}

// A completion is a convenience: a source that cannot be read yields no values
// rather than an error, because failing the keystroke is worse than offering
// nothing to pick from.
func (a *App) hostedMCPCompletionPayload(ctx context.Context, subject aaamodel.Subject, tool string, args map[string]any) map[string]any {
	if a == nil || a.db == nil {
		return nil
	}
	result, err := a.callHostedMCPToolByName(ctx, subject, tool, args)
	if err != nil {
		return nil
	}
	return result
}

func hostedMCPCompletionStrings(payload map[string]any, listKey string, valueKeys ...string) []string {
	values := []string{}
	for _, row := range analysisRows(payload, listKey) {
		for _, key := range valueKeys {
			if value := analysisString(row, key); value != "" {
				values = append(values, value)
				break
			}
		}
	}
	return values
}

// hostedMCPCompletionValues filters, de-duplicates, and bounds the candidates.
// Matching is a prefix first, then a contains match, because a person typing
// "deploy" wants deploy-api before platform/redeploy.
func hostedMCPCompletionValues(candidates []string, value string, limit int) ([]string, bool) {
	needle := strings.ToLower(strings.TrimSpace(value))
	seen := map[string]bool{}
	prefixed := []string{}
	contained := []string{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		lower := strings.ToLower(candidate)
		switch {
		case needle == "" || strings.HasPrefix(lower, needle):
			prefixed = append(prefixed, candidate)
		case strings.Contains(lower, needle):
			contained = append(contained, candidate)
		}
	}
	sort.Strings(prefixed)
	sort.Strings(contained)

	matches := append(prefixed, contained...)
	if limit > 0 && len(matches) > limit {
		return matches[:limit], true
	}
	return matches, false
}

func (a *App) hostedMCPCompletion(ctx context.Context, subject aaamodel.Subject, params hostedMCPCompletionParams) map[string]any {
	kind := hostedMCPCompletionKind(params.Ref, params.Argument.Name)
	candidates := a.hostedMCPCompletionCandidates(ctx, subject, kind)
	values, hasMore := hostedMCPCompletionValues(candidates, params.Argument.Value, hostedMCPCompletionLimit)
	return map[string]any{
		"completion": map[string]any{
			"values":  values,
			"total":   len(values),
			"hasMore": hasMore,
		},
	}
}
