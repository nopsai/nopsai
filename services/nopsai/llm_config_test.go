package nopsai

import (
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestContainerReachableLMStudioBaseURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "localhost rewritten", raw: "http://127.0.0.1:1234", want: "http://host.docker.internal:1234"},
		{name: "localhost hostname rewritten", raw: "http://localhost:1234/v1", want: "http://host.docker.internal:1234/v1"},
		{name: "remote host preserved", raw: "http://lmstudio.internal:1234", want: "http://lmstudio.internal:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containerReachableLMStudioBaseURL(tt.raw); got != tt.want {
				t.Fatalf("containerReachableLMStudioBaseURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseGitOpsLLMProfilePlanFromSettingDirectory(t *testing.T) {
	plan, err := parseGitOpsLLMProfilePlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsLLMProfileDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/llm_profile.yaml": `
default_profile: reasoning
profiles:
  - name: fast
    provider: gemini
    model: gemini-2.5-flash
    api_key_secret: GEMINI_API_KEY
    allowed_scopes: ["dev"]
  - name: reasoning
    provider: lmstudio
    model: google/gemma-4-26b-a4b
    base_url: http://lmstudio:1234
    reasoning: high
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsLLMProfilePlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("expected GitOps LLM profile plan")
	}
	if plan.defaultProfile != "reasoning" {
		t.Fatalf("defaultProfile = %q, want reasoning", plan.defaultProfile)
	}
	if plan.sourcePath != "setting/system/llm_profile.yaml" {
		t.Fatalf("sourcePath = %q", plan.sourcePath)
	}
	if got := plan.profiles["fast"].Provider; got != config.LLMProviderGemini {
		t.Fatalf("fast provider = %q, want gemini", got)
	}
	if got := plan.profiles["reasoning"].Reasoning; got != "high" {
		t.Fatalf("reasoning profile reasoning = %q, want high", got)
	}
}

func TestParseGitOpsLLMProfilePlanRejectsGroupScopedRepo(t *testing.T) {
	_, err := parseGitOpsLLMProfilePlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsLLMProfileDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/llm_profile.yaml": `
llm_default_profile: standard
llm_profiles:
  standard:
    provider: lmstudio
    base_url: http://lmstudio:1234
`,
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsLLMProfilePlanRejectsMissingDefault(t *testing.T) {
	_, err := parseGitOpsLLMProfileFile(`
default_profile: reasoning
llm_profiles:
  fast:
    provider: gemini
    model: gemini-2.5-flash
    api_key_secret: GEMINI_API_KEY
`, "setting/system/llm_profile.yaml")
	if err == nil || !strings.Contains(err.Error(), `default profile "reasoning"`) {
		t.Fatalf("expected missing default error, got %v", err)
	}
}
