package nopsai

import (
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestParseTeamLLMProfilesPayloadAcceptsTeamDefault(t *testing.T) {
	payload := llmProfilesRequest{
		DefaultProfile: "standard",
		Profiles: []llmProfileForm{{
			Name:     "standard",
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
		}},
	}

	defaultProfile, profiles, err := parseTeamLLMProfilesPayload(payload)
	if err != nil {
		t.Fatalf("parseTeamLLMProfilesPayload() error = %v", err)
	}
	if defaultProfile != "standard" {
		t.Fatalf("default profile = %q, want standard", defaultProfile)
	}
	if _, ok := profiles["standard"]; !ok {
		t.Fatalf("profiles missing standard: %#v", profiles)
	}
}

func TestParseTeamLLMProfilesPayloadPreservesDefaultForTeamScopeValidation(t *testing.T) {
	payload := llmProfilesRequest{
		DefaultProfile: "standard",
		Profiles: []llmProfileForm{{
			Name:     "sandbox",
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
		}},
	}

	defaultProfile, profiles, err := parseTeamLLMProfilesPayload(payload)
	if err != nil {
		t.Fatalf("parseTeamLLMProfilesPayload() error = %v", err)
	}
	if defaultProfile != "standard" {
		t.Fatalf("default profile = %q, want standard", defaultProfile)
	}
	if _, ok := profiles["sandbox"]; !ok {
		t.Fatalf("profiles missing sandbox: %#v", profiles)
	}
}

func TestParseTeamLLMProfilesPayloadRejectsDuplicateProfiles(t *testing.T) {
	payload := llmProfilesRequest{
		LLMProfiles: map[string]config.LLMProfile{
			"standard": {
				Provider: config.LLMProviderLMStudio,
				BaseURL:  "http://lmstudio:1234",
			},
		},
		Profiles: []llmProfileForm{{
			Name:     "standard",
			Provider: config.LLMProviderLMStudio,
			BaseURL:  "http://lmstudio:1234",
		}},
	}

	_, _, err := parseTeamLLMProfilesPayload(payload)
	if err == nil {
		t.Fatal("parseTeamLLMProfilesPayload() error = nil, want duplicate error")
	}
}

func TestAIResourceLocalNameMatchesScopedUIBehavior(t *testing.T) {
	if got := aiResourceLocalName("platform/ml/reasoning"); got != "reasoning" {
		t.Fatalf("aiResourceLocalName() = %q, want reasoning", got)
	}
	if got := aiResourceLocalName("platform/ml/"); got != "" {
		t.Fatalf("aiResourceLocalName() = %q, want empty local name for trailing slash", got)
	}
}

func TestCanonicalTeamLLMDefaultProfileValueAcceptsScopedCatalogProfile(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	catalogProfiles := map[string]config.LLMProfile{
		"platform/ml/chatgpt": {Provider: config.LLMProviderOpenAI, Model: "gpt-4.1-mini", CredentialRef: "credential://system/llm/openai"},
	}

	canonical, ok := canonicalTeamLLMDefaultProfileValue(record, "platform/ml/chatgpt", nil, catalogProfiles)
	if !ok {
		t.Fatal("canonicalTeamLLMDefaultProfileValue() rejected scoped catalog profile")
	}
	if canonical != "platform/ml/chatgpt" {
		t.Fatalf("canonical default = %q, want platform/ml/chatgpt", canonical)
	}
}

func TestCanonicalTeamLLMDefaultProfileValueMapsLocalDefaultToScopedCatalogProfile(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	catalogProfiles := map[string]config.LLMProfile{
		"platform/ml/chatgpt": {Provider: config.LLMProviderOpenAI, Model: "gpt-4.1-mini", CredentialRef: "credential://system/llm/openai"},
	}

	canonical, ok := canonicalTeamLLMDefaultProfileValue(record, "chatgpt", nil, catalogProfiles)
	if !ok {
		t.Fatal("canonicalTeamLLMDefaultProfileValue() rejected local catalog default")
	}
	if canonical != "platform/ml/chatgpt" {
		t.Fatalf("canonical default = %q, want platform/ml/chatgpt", canonical)
	}
}

func TestCanonicalTeamLLMDefaultProfileValueCanonicalizesScopedTeamProfileToLocal(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	teamProfiles := map[string]config.LLMProfile{
		"chatgpt": {Provider: config.LLMProviderOpenAI, Model: "gpt-4.1-mini", CredentialRef: "credential://team/platform/ml/llm/openai"},
	}

	canonical, ok := canonicalTeamLLMDefaultProfileValue(record, "platform/ml/chatgpt", teamProfiles, nil)
	if !ok {
		t.Fatal("canonicalTeamLLMDefaultProfileValue() rejected scoped team profile")
	}
	if canonical != "chatgpt" {
		t.Fatalf("canonical default = %q, want chatgpt", canonical)
	}
}

func TestCanonicalTeamLLMDefaultProfileValueRejectsOtherTeam(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	catalogProfiles := map[string]config.LLMProfile{
		"platform/security/chatgpt": {Provider: config.LLMProviderOpenAI, Model: "gpt-4.1-mini", CredentialRef: "credential://system/llm/openai"},
	}

	if canonical, ok := canonicalTeamLLMDefaultProfileValue(record, "platform/security/chatgpt", nil, catalogProfiles); ok {
		t.Fatalf("canonicalTeamLLMDefaultProfileValue() = %q, want rejection", canonical)
	}
}

func TestCanonicalTeamAgentDefaultProfileValueAcceptsScopedCatalogProfile(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	catalogProfiles := map[string]models.AgentProfile{
		"platform/ml/reviewer": {ID: "platform/ml/reviewer", DisplayName: "Reviewer", Instructions: "Review changes.", Enabled: true},
	}

	canonical, ok := canonicalTeamAgentDefaultProfileValue(record, "platform/ml/reviewer", nil, catalogProfiles)
	if !ok {
		t.Fatal("canonicalTeamAgentDefaultProfileValue() rejected scoped catalog profile")
	}
	if canonical != "platform/ml/reviewer" {
		t.Fatalf("canonical default = %q, want platform/ml/reviewer", canonical)
	}
}

func TestCanonicalTeamAgentDefaultProfileValueMapsLocalDefaultToScopedCatalogProfile(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	catalogProfiles := map[string]models.AgentProfile{
		"platform/ml/reviewer": {ID: "platform/ml/reviewer", DisplayName: "Reviewer", Instructions: "Review changes.", Enabled: true},
	}

	canonical, ok := canonicalTeamAgentDefaultProfileValue(record, "reviewer", nil, catalogProfiles)
	if !ok {
		t.Fatal("canonicalTeamAgentDefaultProfileValue() rejected local catalog default")
	}
	if canonical != "platform/ml/reviewer" {
		t.Fatalf("canonical default = %q, want platform/ml/reviewer", canonical)
	}
}

func TestCanonicalTeamAgentDefaultProfileValueCanonicalizesScopedTeamProfileToLocal(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	teamProfiles := map[string]models.AgentProfile{
		"reviewer": {ID: "reviewer", DisplayName: "Reviewer", Instructions: "Review changes.", Enabled: true},
	}

	canonical, ok := canonicalTeamAgentDefaultProfileValue(record, "platform/ml/reviewer", teamProfiles, nil)
	if !ok {
		t.Fatal("canonicalTeamAgentDefaultProfileValue() rejected scoped team profile")
	}
	if canonical != "reviewer" {
		t.Fatalf("canonical default = %q, want reviewer", canonical)
	}
}

func TestCanonicalTeamAgentDefaultProfileValueRejectsDisabledScopedCatalogProfile(t *testing.T) {
	record := teamPathRecord{Path: "platform/ml"}
	catalogProfiles := map[string]models.AgentProfile{
		"platform/ml/reviewer": {ID: "platform/ml/reviewer", DisplayName: "Reviewer", Instructions: "Review changes.", Enabled: false},
	}

	if canonical, ok := canonicalTeamAgentDefaultProfileValue(record, "platform/ml/reviewer", nil, catalogProfiles); ok {
		t.Fatalf("canonicalTeamAgentDefaultProfileValue() = %q, want rejection", canonical)
	}
}

func TestParseGitOpsTeamDefaultsFileAllowsCatalogResolvedLLMDefault(t *testing.T) {
	plan, err := parseGitOpsTeamDefaultsFile("llm_profile: chatgpt\n", "defaults.yaml", "platform/ml")
	if err != nil {
		t.Fatalf("parseGitOpsTeamDefaultsFile() error = %v", err)
	}
	if plan.llmDefaultProfile == nil || *plan.llmDefaultProfile != "chatgpt" {
		t.Fatalf("llm default = %#v, want chatgpt", plan.llmDefaultProfile)
	}
}

func TestParseGitOpsTeamDefaultsFileRejectsLLMDefaultOutsideTeam(t *testing.T) {
	_, err := parseGitOpsTeamDefaultsFile("llm_profile: platform/security/chatgpt\n", "defaults.yaml", "platform/ml")
	if err == nil {
		t.Fatal("parseGitOpsTeamDefaultsFile() error = nil, want outside-team default error")
	}
}

func TestParseGitOpsTeamDefaultsFileLoadsDefaults(t *testing.T) {
	content := `
llm_profile: chatgpt
agent_profile: reviewer
knowledge_context:
  guardrail: runtime-output-safety
  runbook: runbook/platform/ml/restart-service
`
	plan, err := parseGitOpsTeamDefaultsFile(content, "defaults.yaml", "platform/ml")
	if err != nil {
		t.Fatalf("parseGitOpsTeamDefaultsFile() error = %v", err)
	}
	if plan.llmDefaultProfile == nil || *plan.llmDefaultProfile != "chatgpt" {
		t.Fatalf("llm default = %#v, want chatgpt", plan.llmDefaultProfile)
	}
	if plan.agentDefaultProfile == nil || *plan.agentDefaultProfile != "reviewer" {
		t.Fatalf("agent default = %#v, want reviewer", plan.agentDefaultProfile)
	}
	if got := plan.knowledgeDefaults["guardrail"]; got != "platform/ml/runtime-output-safety" {
		t.Fatalf("guardrail default = %q, want platform/ml/runtime-output-safety", got)
	}
	if got := plan.knowledgeDefaults["runbook"]; got != "platform/ml/restart-service" {
		t.Fatalf("runbook default = %q, want platform/ml/restart-service", got)
	}
}

func TestParseGitOpsTeamDefaultsFileRejectsKnowledgeDefaultOutsideTeam(t *testing.T) {
	content := `
knowledge_context:
  guardrail: platform/security/runtime-output-safety
`
	_, err := parseGitOpsTeamDefaultsFile(content, "defaults.yaml", "platform/ml")
	if err == nil {
		t.Fatal("parseGitOpsTeamDefaultsFile() error = nil, want outside-team default error")
	}
	if !strings.Contains(err.Error(), "owned by platform/ml") {
		t.Fatalf("parseGitOpsTeamDefaultsFile() error = %v, want owner validation", err)
	}
}

func TestParseGitOpsTeamDefaultsFileRejectsLegacyDefaultKeys(t *testing.T) {
	_, err := parseGitOpsTeamDefaultsFile("llm_default_profile: chatgpt\n", "defaults.yaml", "platform/ml")
	if err == nil || !strings.Contains(err.Error(), "must define at least one default") {
		t.Fatalf("parseGitOpsTeamDefaultsFile() error = %v, want legacy key rejection", err)
	}
}
