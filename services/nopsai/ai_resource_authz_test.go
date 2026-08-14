package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
)

func TestAIResourceAuthzAllowsTeamScopedResourceOnly(t *testing.T) {
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				allowed := resource.Type == grantResourceTeam &&
					resource.ID == "team-1" &&
					(action == "model.read" || action == "team.update")
				return model.Decision{Allowed: allowed}, nil
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/models", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))

	visible, err := app.aiResourceVisible(req, llmProfileAccessSpec, "team-1/alice")
	if err != nil || !visible {
		t.Fatalf("team-1 profile visible = %v, err = %v, want visible", visible, err)
	}
	visible, err = app.aiResourceVisible(req, llmProfileAccessSpec, "team-2/bob")
	if err != nil || visible {
		t.Fatalf("team-2 profile visible = %v, err = %v, want hidden", visible, err)
	}
	visible, err = app.aiResourceVisible(req, llmProfileAccessSpec, "global")
	if err != nil || visible {
		t.Fatalf("global profile visible = %v, err = %v, want hidden", visible, err)
	}

	writable, err := app.aiResourceWriteAllowed(req, llmProfileAccessSpec, "team-1/alice")
	if err != nil || !writable {
		t.Fatalf("team-1 profile writable = %v, err = %v, want writable", writable, err)
	}
	writable, err = app.aiResourceWriteAllowed(req, llmProfileAccessSpec, "global")
	if err != nil || writable {
		t.Fatalf("global profile writable = %v, err = %v, want not writable", writable, err)
	}
}

func TestHandleListLLMProfilesFiltersByTeamResourceAccess(t *testing.T) {
	app := &App{
		cfg: &config.Config{
			LLMDefaultProfile: "global",
			LLMProfiles: map[string]config.LLMProfile{
				"global":       {Provider: "openai", Model: "gpt-4o"},
				"team-1/alice": {Provider: "openai", Model: "gpt-4o-mini"},
				"team-2/bob":   {Provider: "openai", Model: "gpt-4o-mini"},
			},
		},
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				allowed := action == "model.read" && resource.Type == grantResourceTeam && resource.ID == "team-1"
				return model.Decision{Allowed: allowed}, nil
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/models", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()

	app.handleListLLMProfiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload llmProfilesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.DefaultProfile != "" {
		t.Fatalf("default profile = %q, want empty when global default is hidden", payload.DefaultProfile)
	}
	if len(payload.Profiles) != 1 || payload.Profiles[0].Name != "team-1/alice" {
		t.Fatalf("profiles = %#v, want only team-1/alice", payload.Profiles)
	}
}

func TestHandleListAgentProfilesIncludesBuiltInsWithoutResourceGrant(t *testing.T) {
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, _ string, _ model.ResourceRef, _ map[string]any) (model.Decision, error) {
				return model.Decision{Allowed: false}, nil
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/agent-roles", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()

	app.handleListAgentProfiles(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload agentProfilesResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.DefaultProfile != models.DefaultAgentProfileID {
		t.Fatalf("default profile = %q, want %q", payload.DefaultProfile, models.DefaultAgentProfileID)
	}
	if len(payload.Profiles) != len(models.BuiltInAgentProfiles()) {
		t.Fatalf("profiles = %d, want built-ins only (%d)", len(payload.Profiles), len(models.BuiltInAgentProfiles()))
	}
	for _, profile := range payload.Profiles {
		if !profile.BuiltIn || !profile.ReadOnly {
			t.Fatalf("profile %#v, want read-only built-in", profile)
		}
	}
}

func TestHandleGetAgentProfileAllowsBuiltInWithoutResourceGrant(t *testing.T) {
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, _ string, _ model.ResourceRef, _ map[string]any) (model.Decision, error) {
				return model.Decision{Allowed: false}, nil
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/system/agent-roles/devops-engineer", nil)
	req.SetPathValue("profileID", models.DefaultAgentProfileID)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, ID: "alice"}))
	rec := httptest.NewRecorder()

	app.handleGetAgentProfile(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload agentProfileView
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID != models.DefaultAgentProfileID || !payload.BuiltIn {
		t.Fatalf("profile = %#v, want built-in default", payload)
	}
}

func TestAIResourceTeamPath(t *testing.T) {
	tests := map[string]string{
		"global":                  "",
		"/team-1/reviewer/":       "team-1",
		"team-1/platform/github":  "team-1/platform",
		" team-1 / platform / x ": "team-1/platform",
	}
	for input, want := range tests {
		if got := aiResourceTeamPath(input); got != want {
			t.Fatalf("aiResourceTeamPath(%q) = %q, want %q", input, got, want)
		}
	}
}
