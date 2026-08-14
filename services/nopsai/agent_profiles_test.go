package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsAgentRolesReadsOneFilePerRole(t *testing.T) {
	plan, err := parseGitOpsAgentRoles(
		map[string]string{
			"agent-roles/release-manager.yaml": `
default: true
display_name: Release Manager
description: Coordinates release readiness.
instructions: |
  Check release evidence before rollout.
`,
			"agent-roles/platform/reviewer.yaml": "display_name: Security Reviewer\ninstructions: Review risky changes.\n",
		},
		"agent-roles",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
	)
	if err != nil {
		t.Fatalf("parseGitOpsAgentRoles() error = %v", err)
	}
	defaultRole, registryRoles := plan.registryRoles()
	if defaultRole != "release-manager" {
		t.Fatalf("default role = %q, want release-manager", defaultRole)
	}
	role := registryRoles["release-manager"]
	if role.ID != "release-manager" || !role.Enabled {
		t.Fatalf("role = %#v, want an enabled release-manager role", role)
	}
	if role.Instructions != "Check release evidence before rollout." {
		t.Fatalf("instructions = %q", role.Instructions)
	}
	if _, ok := registryRoles["platform/reviewer"]; !ok {
		t.Fatalf("registry roles = %#v, want the team-scoped platform/reviewer role", registryRoles)
	}
}

func TestParseGitOpsAgentRolesRejectsTwoDefaultsInOneScope(t *testing.T) {
	_, err := parseGitOpsAgentRoles(
		map[string]string{
			"agent-roles/one.yaml": "default: true\ndisplay_name: One\ninstructions: Do one thing.\n",
			"agent-roles/two.yaml": "default: true\ndisplay_name: Two\ninstructions: Do another thing.\n",
		},
		"agent-roles",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
	)
	if err == nil || !strings.Contains(err.Error(), "set default: true") {
		t.Fatalf("error = %v, want a single-default error", err)
	}
}

func TestParseGitOpsAgentRolesRejectsNameMismatch(t *testing.T) {
	_, err := parseGitOpsAgentRoles(
		map[string]string{
			"agent-roles/reviewer.yaml": "name: other\ndisplay_name: Reviewer\ninstructions: Review changes.\n",
		},
		"agent-roles",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
	)
	if err == nil || !strings.Contains(err.Error(), "declares name") {
		t.Fatalf("error = %v, want a file-name mismatch error", err)
	}
}

func TestValidateAgentProfileDefinitionAcceptsTeamScopedID(t *testing.T) {
	err := validateAgentProfileDefinition(models.AgentProfile{
		ID:           "team-1/security/reviewer",
		DisplayName:  "Security Reviewer",
		Instructions: "Review risky changes.",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("validateAgentProfileDefinition() error = %v", err)
	}
}

func TestValidateAgentProfileDefinitionRejectsEmptyScopedSegment(t *testing.T) {
	err := validateAgentProfileDefinition(models.AgentProfile{
		ID:           "team-1//reviewer",
		DisplayName:  "Security Reviewer",
		Instructions: "Review risky changes.",
		Enabled:      true,
	})
	if err == nil {
		t.Fatal("expected empty scoped segment error")
	}
}
