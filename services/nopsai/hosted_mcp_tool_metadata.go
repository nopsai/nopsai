package nopsai

import (
	"strings"
)

// MCP clients decide how much to trust a tool from its annotations: whether to
// run it without asking, whether to warn first, whether a retry is safe. NopsAI
// already knows all three — capability comes from the tool's action and schema,
// and the routing metadata classifies every tool — so this publishes what we
// compute rather than asking anyone to maintain a second list.
//
// Hints are advisory by spec and are never trusted for enforcement: the AAA
// check at call time is what actually stops a tool, and it does not read these.

type hostedMCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint"`
	DestructiveHint bool   `json:"destructiveHint"`
	IdempotentHint  bool   `json:"idempotentHint"`
	OpenWorldHint   bool   `json:"openWorldHint"`
}

// Verbs that remove or interrupt something. Everything else that mutates is
// additive, which a client may treat as less alarming.
var hostedMCPDestructiveVerbs = []string{
	"delete_", "remove_", "eject_", "revoke_", "cancel_", "disable_", "deactivate_",
	"reject_", "purge_", "cleanup", "drain_",
}

// Repeating these lands in the same end state; repeating a create or a run does
// not, so a client must not retry those on its own.
var hostedMCPIdempotentVerbs = []string{
	"delete_", "remove_", "write_", "set_", "update_", "enable_", "disable_",
	"activate_", "deactivate_", "propose_", "encrypt_",
}

// Tools that leave NopsAI: a Git provider, a mail server, an external trigger
// target, or a registered external MCP server.
var hostedMCPOpenWorldFragments = []string{
	"config_repo", "git_webhook", "external_trigger", "notification", "runner_bootstrap",
}

func hostedMCPToolTitle(name string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(name), "nopsai.")
	if trimmed == "" {
		return ""
	}
	words := strings.Split(strings.ReplaceAll(trimmed, "_", " "), " ")
	for index, word := range words {
		switch word {
		case "ai", "api", "mcp", "llm", "aaa", "yaml", "gitops", "ui":
			words[index] = strings.ToUpper(word)
		case "":
		default:
			if index == 0 {
				words[index] = strings.ToUpper(word[:1]) + word[1:]
			}
		}
	}
	return strings.Join(words, " ")
}

func hostedMCPToolAnnotationsFor(tool hostedMCPTool) hostedMCPToolAnnotations {
	name := strings.ToLower(tool.Name)
	capability := hostedMCPToolCapability(tool)
	readOnly := capability == hostedMCPCapabilityRead || capability == hostedMCPCapabilityProposal

	annotations := hostedMCPToolAnnotations{
		Title:         hostedMCPToolTitle(tool.Name),
		ReadOnlyHint:  readOnly,
		OpenWorldHint: hostedMCPToolReachesOutside(name),
	}
	if readOnly {
		// A read cannot destroy anything, and running it twice is the same as
		// running it once.
		annotations.IdempotentHint = true
		return annotations
	}
	annotations.DestructiveHint = hostedMCPToolNameHasAny(name, hostedMCPDestructiveVerbs)
	annotations.IdempotentHint = hostedMCPToolNameHasAny(name, hostedMCPIdempotentVerbs)
	return annotations
}

func hostedMCPToolReachesOutside(name string) bool {
	for _, fragment := range hostedMCPOpenWorldFragments {
		if strings.Contains(name, fragment) {
			return true
		}
	}
	return false
}

func hostedMCPToolNameHasAny(name string, verbs []string) bool {
	trimmed := strings.TrimPrefix(name, "nopsai.")
	for _, verb := range verbs {
		if strings.HasPrefix(trimmed, verb) || strings.Contains(trimmed, "_"+verb) || strings.Contains(trimmed, verb) {
			return true
		}
	}
	return false
}

// hostedMCPAnalysisOutputSchema describes what the analysis tools return. It is
// deliberately open (`additionalProperties: true`) and requires only the two
// fields every path sets, including the failure path: a schema that a real
// response can fail is worse than no schema, because clients validate it.
func hostedMCPAnalysisOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"analysis":     map[string]any{"type": "string", "description": "The analysed subject kind: team, pipeline, or run."},
			"ok":           map[string]any{"type": "boolean", "description": "False when a source could not be read; findings are then partial."},
			"health_score": map[string]any{"type": []string{"integer", "null"}, "description": "0-100, or null when no evidence could be read."},
			"summary":      map[string]any{"type": "string"},
			"findings": arraySchema(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":       map[string]any{"type": "string"},
					"category": map[string]any{"type": "string"},
					"severity": map[string]any{"type": "string", "enum": []string{"critical", "high", "medium", "low", "opportunity"}},
					"title":    map[string]any{"type": "string"},
					"summary":  map[string]any{"type": "string"},
					"evidence": arraySchema(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"label": map[string]any{"type": "string"},
							"value": map[string]any{"type": "string"},
							"kind":  map[string]any{"type": "string"},
						},
					}),
					"recommendations": arraySchema(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title":  map[string]any{"type": "string"},
							"detail": map[string]any{"type": "string"},
						},
					}),
					"confidence": map[string]any{"type": "number"},
				},
			}),
			"scores": arraySchema(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"category":      map[string]any{"type": "string"},
					"score":         map[string]any{"type": "integer"},
					"finding_count": map[string]any{"type": "integer"},
					"deduction":     map[string]any{"type": "integer"},
					"basis":         map[string]any{"type": "string"},
				},
			}),
			"next_actions": arraySchema(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"label": map[string]any{"type": "string"},
					"tool":  map[string]any{"type": "string"},
					"args":  map[string]any{"type": "object"},
				},
			}),
			"primary_diagnosis": map[string]any{
				"type":        "object",
				"description": "Run analysis only: the likely failure domain and confidence.",
				"properties": map[string]any{
					"domain":     map[string]any{"type": "string"},
					"confidence": map[string]any{"type": "number"},
				},
			},
			"limitations":  arraySchema(map[string]any{"type": "string"}),
			"data_sources": arraySchema(map[string]any{"type": "string"}),
			"error":        map[string]any{"type": "string", "description": "Present when the subject could not be resolved."},
		},
		"required":             []string{"analysis", "ok"},
		"additionalProperties": true,
	}
}

func hostedMCPToolOutputSchema(name string) map[string]any {
	switch name {
	case "nopsai.analyze_team", "nopsai.analyze_pipeline", "nopsai.analyze_run":
		return hostedMCPAnalysisOutputSchema()
	default:
		return nil
	}
}

// hostedMCPDescribeTool is what tools/list publishes: the tool plus everything a
// client needs to decide how to treat it.
func hostedMCPDescribeTool(tool hostedMCPTool) map[string]any {
	annotations := hostedMCPToolAnnotationsFor(tool)
	described := map[string]any{
		"name":        tool.Name,
		"description": tool.Description,
		"inputSchema": tool.InputSchema,
		"annotations": annotations,
	}
	if annotations.Title != "" {
		described["title"] = annotations.Title
	}
	if schema := hostedMCPToolOutputSchema(tool.Name); schema != nil {
		described["outputSchema"] = schema
	}
	return described
}
