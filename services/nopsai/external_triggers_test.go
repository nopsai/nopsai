package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestNormalizeExternalTriggerInput(t *testing.T) {
	enabled := false
	got, err := normalizeExternalTriggerInput(externalTriggerInput{
		Name:        "Deploy Prod",
		Enabled:     &enabled,
		Pipeline:    ".nopsai/pipelines/platform/deploy.yaml",
		Scope:       "/prod/",
		RunTeamPath: "/platform/prod/",
		AllowedCallers: []externalTriggerAllowedCaller{
			{Type: "team", ID: "platform-ops"},
			{Type: "service_account", ID: "servicenow-prod"},
		},
		VariableMapping: map[string]string{
			"VERSION": "payload.version",
		},
	}, "")
	if err != nil {
		t.Fatalf("normalizeExternalTriggerInput() error = %v", err)
	}
	if got.ID != "deploy-prod" {
		t.Fatalf("ID = %q, want deploy-prod", got.ID)
	}
	if got.Pipeline != "platform/deploy" {
		t.Fatalf("Pipeline = %q, want platform/deploy", got.Pipeline)
	}
	if got.Scope != "prod" {
		t.Fatalf("Scope = %q, want prod", got.Scope)
	}
	if got.RunTeamPath != "platform/prod" {
		t.Fatalf("RunTeamPath = %q, want platform/prod", got.RunTeamPath)
	}
	if got.Enabled {
		t.Fatal("Enabled = true, want false")
	}
	if len(got.AllowedCallers) != 2 || got.AllowedCallers[0].Type != "auth_team" || got.AllowedCallers[1].Type != "service_account" {
		t.Fatalf("AllowedCallers = %#v, want normalized auth_team and service_account", got.AllowedCallers)
	}
}

func TestNormalizeExternalTriggerInputTreatsGlobalAsGlobalScope(t *testing.T) {
	got, err := normalizeExternalTriggerInput(externalTriggerInput{
		Name:        "Deploy Global",
		Pipeline:    "pipelines/global/platform/deploy.yaml",
		Scope:       "global/prod",
		RunTeamPath: "global",
	}, "")
	if err != nil {
		t.Fatalf("normalizeExternalTriggerInput() error = %v", err)
	}
	if got.Pipeline != "platform/deploy" {
		t.Fatalf("Pipeline = %q, want platform/deploy", got.Pipeline)
	}
	if got.Scope != "prod" {
		t.Fatalf("Scope = %q, want prod", got.Scope)
	}
	if got.RunTeamPath != "global" {
		t.Fatalf("RunTeamPath = %q, want global", got.RunTeamPath)
	}
}

func TestApplyExternalTriggerVariableMapping(t *testing.T) {
	got, err := applyExternalTriggerVariableMapping(externalTriggerInvokeRequest{
		EventType: "servicenow.change.approved",
		Variables: map[string]string{
			"VERSION": "1.0.0",
		},
		Payload: map[string]any{
			"version": "1.2.3",
			"change": map[string]any{
				"id": "chg-123",
			},
		},
	}, map[string]string{
		"VERSION":   "payload.version",
		"CHANGE_ID": "payload.change.id",
		"EVENT":     "event_type",
		"STATIC":    "literal:prod",
	})
	if err != nil {
		t.Fatalf("applyExternalTriggerVariableMapping() error = %v", err)
	}
	want := map[string]string{
		"VERSION":   "1.2.3",
		"CHANGE_ID": "chg-123",
		"EVENT":     "servicenow.change.approved",
		"STATIC":    "prod",
	}
	for key, expected := range want {
		if got[key] != expected {
			t.Fatalf("%s = %q, want %q in %#v", key, got[key], expected, got)
		}
	}
}

func TestValidateExternalTriggerPayloadSchema(t *testing.T) {
	err := validateExternalTriggerPayloadSchema(map[string]any{
		"type":     "object",
		"required": []any{"version"},
		"properties": map[string]any{
			"version": map[string]any{"type": "string"},
			"count":   map[string]any{"type": "integer"},
		},
	}, map[string]any{
		"version": "1.2.3",
		"count":   float64(3),
	})
	if err != nil {
		t.Fatalf("validateExternalTriggerPayloadSchema() error = %v", err)
	}

	err = validateExternalTriggerPayloadSchema(map[string]any{
		"type":     "object",
		"required": []any{"version"},
	}, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("missing required field error = %v, want version error", err)
	}
}

func TestExternalTriggerRateLimitCountExceededAllowsCurrentAuditRow(t *testing.T) {
	if externalTriggerRateLimitCountExceeded(1, 1) {
		t.Fatal("count 1 with limit 1 should allow the current audited invocation")
	}
	if !externalTriggerRateLimitCountExceeded(2, 1) {
		t.Fatal("count 2 with limit 1 should exceed the rate limit")
	}
}

func TestParseGitOpsExternalTriggersNormalizesTeamReferences(t *testing.T) {
	enabled := true
	triggers, err := parseGitOpsExternalTriggers(map[string]string{
		"external-triggers/deploy-prod.yaml": `
name: Deploy prod
pipeline: platform-maintenance
scope: prod
run_team_path: prod/webhooks
enabled: true
allowed_callers:
  - type: service_account
    id: servicenow-prod
variable_mapping:
  VERSION: payload.version
payload_schema:
  type: object
  required:
    - version
rate_limit:
  per_minute: 10
`,
	}, "external-triggers", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseGitOpsExternalTriggers() error = %v", err)
	}
	got, ok := triggers["team-1-deploy-prod"]
	if !ok {
		t.Fatalf("missing normalized trigger id, got %#v", triggers)
	}
	if got.input.Enabled != enabled {
		t.Fatalf("Enabled = %v, want %v", got.input.Enabled, enabled)
	}
	if got.input.Pipeline != "team-1/platform-maintenance" {
		t.Fatalf("Pipeline = %q, want team-1/platform-maintenance", got.input.Pipeline)
	}
	if got.input.Scope != "team-1/prod" {
		t.Fatalf("Scope = %q, want team-1/prod", got.input.Scope)
	}
	if got.input.RunTeamPath != "team-1/prod/webhooks" {
		t.Fatalf("RunTeamPath = %q, want team-1/prod/webhooks", got.input.RunTeamPath)
	}
	if len(got.input.AllowedCallers) != 1 || got.input.AllowedCallers[0].ID != "servicenow-prod" {
		t.Fatalf("AllowedCallers = %#v, want servicenow-prod", got.input.AllowedCallers)
	}
	if got.input.VariableMapping["VERSION"] != "payload.version" {
		t.Fatalf("VariableMapping = %#v, want VERSION mapping", got.input.VariableMapping)
	}
}

func TestParseGitOpsExternalTriggersKeepsGlobalRunTeamForTeamRepo(t *testing.T) {
	triggers, err := parseGitOpsExternalTriggers(map[string]string{
		"external-triggers/deploy-prod.yaml": `
name: Deploy prod
pipeline: global/platform-maintenance
scope: global
run_team_path: global
`,
	}, "external-triggers", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseGitOpsExternalTriggers() error = %v", err)
	}
	got, ok := triggers["team-1-deploy-prod"]
	if !ok {
		t.Fatalf("missing normalized trigger id, got %#v", triggers)
	}
	if got.input.Scope != "" {
		t.Fatalf("Scope = %q, want global/default scope", got.input.Scope)
	}
	if got.input.Pipeline != "platform-maintenance" {
		t.Fatalf("Pipeline = %q, want platform-maintenance", got.input.Pipeline)
	}
	if got.input.RunTeamPath != "global" {
		t.Fatalf("RunTeamPath = %q, want global", got.input.RunTeamPath)
	}
}

func TestExternalTriggerConfigScopePrefersRunTeam(t *testing.T) {
	got := externalTriggerConfigScope(externalTriggerRecord{
		ID:          "deploy-prod",
		Pipeline:    "shared/platform/deploy",
		Scope:       "shared/prod",
		RunTeamPath: "team-1/prod",
	})
	if got != "team-1/prod" {
		t.Fatalf("externalTriggerConfigScope() = %q, want team-1/prod", got)
	}

	got = externalTriggerConfigScope(externalTriggerRecord{
		ID:       "deploy-prod",
		Pipeline: "shared/platform/deploy",
		Scope:    "shared/prod",
	})
	if got != "global" {
		t.Fatalf("externalTriggerConfigScope() without run team = %q, want global", got)
	}
}
