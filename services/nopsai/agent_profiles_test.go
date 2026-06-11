package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsAgentProfileFile(t *testing.T) {
	const raw = `
default_profile: release-manager
agent_profiles:
  - id: release-manager
    display_name: Release Manager
    description: Coordinates release readiness.
    instructions: |
      Check release evidence before rollout.
`

	plan, err := parseGitOpsAgentProfileFile(raw, "setting/system/agent-profiles.yaml")
	if err != nil {
		t.Fatalf("parseGitOpsAgentProfileFile() error = %v", err)
	}
	if plan.defaultProfile != "release-manager" {
		t.Fatalf("defaultProfile = %q, want release-manager", plan.defaultProfile)
	}
	profile := plan.profiles["release-manager"]
	if profile.ID != "release-manager" || profile.Source != "gitops" || !profile.Enabled {
		t.Fatalf("profile = %#v, want enabled gitops release-manager", profile)
	}
	if profile.Role != "" {
		t.Fatalf("role = %q, want optional empty role", profile.Role)
	}
	if profile.Instructions != "Check release evidence before rollout." {
		t.Fatalf("instructions = %q", profile.Instructions)
	}
}

func TestParseGitOpsAgentProfileFileAcceptsBuiltInDefaultOnly(t *testing.T) {
	const raw = `default_profile: sre`
	plan, err := parseGitOpsAgentProfileFile(raw, "setting/system/agent-profiles.yaml")
	if err != nil {
		t.Fatalf("parseGitOpsAgentProfileFile() error = %v", err)
	}
	if plan.defaultProfile != "sre" || len(plan.profiles) != 0 {
		t.Fatalf("plan = %#v, want built-in default only", plan)
	}
}

func TestParseGitOpsAgentProfileFileRejectsUnknownDefault(t *testing.T) {
	const raw = `
default_profile: missing-profile
agent_profiles:
  - id: release-manager
    display_name: Release Manager
    instructions: Check release evidence.
`
	_, err := parseGitOpsAgentProfileFile(raw, "setting/system/agent-profiles.yaml")
	if err == nil {
		t.Fatal("expected unknown default profile error")
	}
	if !strings.Contains(err.Error(), `sets default profile "missing-profile"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGitOpsAgentProfileFileRejectsDuplicates(t *testing.T) {
	const raw = `
profiles:
  - id: sre
    display_name: SRE
    role: Senior Site Reliability Engineer
    instructions: Protect reliability.
  - id: sre
    display_name: Duplicate SRE
    role: Senior Site Reliability Engineer
    instructions: Duplicate profile.
`
	_, err := parseGitOpsAgentProfileFile(raw, "setting/system/agent-profiles.yaml")
	if err == nil {
		t.Fatal("expected duplicate profile error")
	}
	if !strings.Contains(err.Error(), `defines profile "sre" more than once`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseGitOpsAgentProfilePlanRequiresSystemRepository(t *testing.T) {
	_, err := parseGitOpsAgentProfilePlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsAgentProfileDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/agent-profiles.yaml": `
agent_profiles:
  - id: sre
    display_name: SRE
    role: Senior Site Reliability Engineer
    instructions: Protect reliability.
`,
			},
		},
	)
	if err == nil {
		t.Fatal("expected non-system repository error")
	}
	if !strings.Contains(err.Error(), "agent profiles can only be configured from a system config repository") {
		t.Fatalf("unexpected error: %v", err)
	}
}
