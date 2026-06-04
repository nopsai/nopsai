package main

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestNormalizeExternalTriggerInput(t *testing.T) {
	enabled := false
	got, err := normalizeExternalTriggerInput(externalTriggerInput{
		Name:         "Deploy Prod",
		Enabled:      &enabled,
		Pipeline:     ".nopsai/pipelines/platform/deploy.yaml",
		Scope:        "/prod/",
		RunGroupPath: "/platform/prod/",
		AllowedCallers: []externalTriggerAllowedCaller{
			{Type: "group", ID: "platform-ops"},
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
	if got.RunGroupPath != "platform/prod" {
		t.Fatalf("RunGroupPath = %q, want platform/prod", got.RunGroupPath)
	}
	if got.Enabled {
		t.Fatal("Enabled = true, want false")
	}
	if len(got.AllowedCallers) != 2 || got.AllowedCallers[0].Type != "auth_group" || got.AllowedCallers[1].Type != "service_account" {
		t.Fatalf("AllowedCallers = %#v, want normalized auth_group and service_account", got.AllowedCallers)
	}
}

func TestNormalizeExternalTriggerInputTreatsRootAsRootScope(t *testing.T) {
	got, err := normalizeExternalTriggerInput(externalTriggerInput{
		Name:         "Deploy Root",
		Pipeline:     "pipelines/root/platform/deploy.yaml",
		Scope:        "root/prod",
		RunGroupPath: "root",
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
	if got.RunGroupPath != "root" {
		t.Fatalf("RunGroupPath = %q, want root", got.RunGroupPath)
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

func TestParseGitOpsExternalTriggersNormalizesFolderReferences(t *testing.T) {
	enabled := true
	triggers, err := parseGitOpsExternalTriggers(map[string]string{
		"external-triggers/deploy-prod.yaml": `
name: Deploy prod
pipeline: platform-maintenance
scope: prod
run_group_path: prod/webhooks
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
		ScopeType: models.ConfigRepositoryScopeFolder,
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
	if got.input.RunGroupPath != "team-1/prod/webhooks" {
		t.Fatalf("RunGroupPath = %q, want team-1/prod/webhooks", got.input.RunGroupPath)
	}
	if len(got.input.AllowedCallers) != 1 || got.input.AllowedCallers[0].ID != "servicenow-prod" {
		t.Fatalf("AllowedCallers = %#v, want servicenow-prod", got.input.AllowedCallers)
	}
	if got.input.VariableMapping["VERSION"] != "payload.version" {
		t.Fatalf("VariableMapping = %#v, want VERSION mapping", got.input.VariableMapping)
	}
}

func TestParseGitOpsExternalTriggersKeepsRootRunGroupForFolderRepo(t *testing.T) {
	triggers, err := parseGitOpsExternalTriggers(map[string]string{
		"external-triggers/deploy-prod.yaml": `
name: Deploy prod
pipeline: root/platform-maintenance
scope: root
run_group_path: root
`,
	}, "external-triggers", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
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
		t.Fatalf("Scope = %q, want root/default scope", got.input.Scope)
	}
	if got.input.Pipeline != "platform-maintenance" {
		t.Fatalf("Pipeline = %q, want platform-maintenance", got.input.Pipeline)
	}
	if got.input.RunGroupPath != "root" {
		t.Fatalf("RunGroupPath = %q, want root", got.input.RunGroupPath)
	}
}

func TestExternalTriggerConfigScopePrefersRunGroup(t *testing.T) {
	got := externalTriggerConfigScope(externalTriggerRecord{
		ID:           "deploy-prod",
		Pipeline:     "shared/platform/deploy",
		Scope:        "shared/prod",
		RunGroupPath: "team-1/prod",
	})
	if got != "team-1/prod" {
		t.Fatalf("externalTriggerConfigScope() = %q, want team-1/prod", got)
	}

	got = externalTriggerConfigScope(externalTriggerRecord{
		ID:       "deploy-prod",
		Pipeline: "shared/platform/deploy",
		Scope:    "shared/prod",
	})
	if got != "root" {
		t.Fatalf("externalTriggerConfigScope() without run group = %q, want root", got)
	}
}
