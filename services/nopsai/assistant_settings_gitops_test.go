package nopsai

import (
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestParseGitOpsAssistantSettingsPlan(t *testing.T) {
	plan, err := parseGitOpsAssistantSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/assistant.yaml": `
assistant:
  enabled: true
  provider: gemini
  model: gemini-2.5-pro
  credential_ref: credential://system/assistant/api-key
  features:
    docs: true
    action_execution: false
`,
				"setting/system/runner.yaml": "runner_id: runner-general",
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsAssistantSettingsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsAssistantSettingsPlan() = nil, want plan")
	}
	if plan.sourcePath != "setting/system/assistant.yaml" {
		t.Fatalf("source path = %q, want setting/system/assistant.yaml", plan.sourcePath)
	}
	if plan.payload.Assistant == nil {
		t.Fatal("assistant payload = nil, want assistant config")
	}
	if !plan.payload.Assistant.Enabled || plan.payload.Assistant.Provider != "gemini" {
		t.Fatalf("assistant payload = %#v, want enabled gemini assistant", plan.payload.Assistant)
	}
	if plan.payload.Assistant.CredentialRef != "credential://system/assistant/api-key" {
		t.Fatalf("assistant credential ref = %q", plan.payload.Assistant.CredentialRef)
	}
	if plan.payload.RunnerID != nil {
		t.Fatalf("assistant plan must not carry runner settings: %#v", plan.payload.RunnerID)
	}
}

func TestParseGitOpsAssistantSettingsPlanRequiresSystemRepository(t *testing.T) {
	_, err := parseGitOpsAssistantSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root:  "setting",
			files: map[string]string{"setting/system/assistant.yaml": "assistant:\n  enabled: true\n"},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("error = %v, want system config repository error", err)
	}
}

func TestParseGitOpsAssistantSettingsFileRejectsForeignSettings(t *testing.T) {
	_, err := parseGitOpsAssistantSettingsFile("assistant:\n  enabled: true\nrunner_id: runner-general\n", "setting/system/assistant.yaml")
	if err == nil || !strings.Contains(err.Error(), "runner_id") {
		t.Fatalf("error = %v, want unsupported setting error", err)
	}
}

func TestParseGitOpsAssistantSettingsFileRequiresAssistantBlock(t *testing.T) {
	_, err := parseGitOpsAssistantSettingsFile("# no settings yet\n", "setting/system/assistant.yaml")
	if err == nil || !strings.Contains(err.Error(), "missing the assistant block") {
		t.Fatalf("error = %v, want missing assistant block error", err)
	}
}

func TestParseGitOpsRuntimeSettingsFileRejectsAssistantBlock(t *testing.T) {
	_, err := parseGitOpsRuntimeSettingsFile("runner_id: runner-general\nassistant:\n  enabled: true\n", "setting/system/runner.yaml")
	if err == nil || !strings.Contains(err.Error(), "setting/system/assistant.yaml") {
		t.Fatalf("error = %v, want assistant migration error pointing at setting/system/assistant.yaml", err)
	}
}

func TestAssistantSettingsIsGitOpsRelativePath(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: "system/assistant.yaml", want: true},
		{path: "/system/assistant.yaml", want: true},
		{path: "system/assistant.yml", want: false},
		{path: "system/runner.yaml", want: false},
		{path: "../system/assistant.yaml", want: false},
	} {
		if got := isGitOpsAssistantSettingsRelativePath(tc.path); got != tc.want {
			t.Fatalf("isGitOpsAssistantSettingsRelativePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestBuildAssistantSettingsGitOpsFileCarriesEffectiveConfig(t *testing.T) {
	cfg := config.Config{
		RunnerID: "runner-general",
		Assistant: config.AssistantConfig{
			Enabled:  true,
			Provider: "gemini",
			Model:    "gemini-2.5-pro",
		},
	}
	file := buildAssistantSettingsGitOpsFile(cfg)
	if file.Assistant == nil {
		t.Fatal("assistant file = nil, want assistant block")
	}
	if file.Assistant.Provider != cfg.Assistant.Provider {
		t.Fatalf("assistant provider = %q, want %q", file.Assistant.Provider, cfg.Assistant.Provider)
	}
	content, err := marshalConfigRepositoryYAML(file)
	if err != nil {
		t.Fatalf("marshalConfigRepositoryYAML() error = %v", err)
	}
	if !strings.Contains(string(content), "assistant:") {
		t.Fatalf("assistant export = %s, want assistant block", content)
	}
	if strings.Contains(string(content), "runner_id") {
		t.Fatalf("assistant export must not contain runner settings:\n%s", content)
	}
}
