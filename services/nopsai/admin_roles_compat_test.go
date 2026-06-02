package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

func TestParseAdminRolePermissionParsesAAAAllowRule(t *testing.T) {
	permission, err := parseAdminRolePermission(createRoleRequest{
		Role:   "nopsai-viewer",
		Object: "pipeline:team/build",
		Action: "pipeline.read",
	})
	if err != nil {
		t.Fatalf("parseAdminRolePermission() error = %v", err)
	}
	if permission.ResourceType != "pipeline" || permission.ResourceID != "team/build" {
		t.Fatalf("resource = %s:%s, want pipeline:team/build", permission.ResourceType, permission.ResourceID)
	}
	if permission.Action != "pipeline.read" || permission.Effect != "allow" {
		t.Fatalf("action/effect = %s/%s, want pipeline.read/allow", permission.Action, permission.Effect)
	}
}

func TestParseAdminRolePermissionParsesExplicitDeny(t *testing.T) {
	permission, err := parseAdminRolePermission(createRoleRequest{
		Role:   "nopsai-viewer",
		Object: "pipeline:*",
		Action: "deny pipeline.delete",
	})
	if err != nil {
		t.Fatalf("parseAdminRolePermission() error = %v", err)
	}
	if permission.Action != "pipeline.delete" || permission.Effect != "deny" {
		t.Fatalf("action/effect = %s/%s, want pipeline.delete/deny", permission.Action, permission.Effect)
	}
}

func TestParseAdminRolePermissionRejectsLegacyPathPolicies(t *testing.T) {
	_, err := parseAdminRolePermission(createRoleRequest{
		Role:   "nopsai-viewer",
		Object: "/v1/pipelines/*",
		Action: "GET",
	})
	if err == nil {
		t.Fatal("expected legacy path policy to be rejected")
	}
}

func TestProtectedAdminRoleNames(t *testing.T) {
	for _, role := range []string{"viewer", "developer", "owner", "admin", defaultAdminRole} {
		if !isProtectedAdminRoleName(role) {
			t.Fatalf("isProtectedAdminRoleName(%q) = false, want true", role)
		}
	}
	if isProtectedAdminRoleName("release-manager") {
		t.Fatal("custom role should not be protected")
	}
}

func TestDefaultAdminAllAccessPermission(t *testing.T) {
	permission, err := parseAdminRolePermission(createRoleRequest{
		Role:   defaultAdminRole,
		Object: "*:*",
		Action: "*",
	})
	if err != nil {
		t.Fatalf("parseAdminRolePermission() error = %v", err)
	}
	if !isDefaultAdminAllAccessPermission(permission) {
		t.Fatal("default admin all-access permission should be recognized")
	}
	permission.Action = "pipeline.read"
	if isDefaultAdminAllAccessPermission(permission) {
		t.Fatal("non-wildcard default admin permission should not be recognized")
	}
}

func TestRoleHandlersRejectProtectedDefaultRoles(t *testing.T) {
	app := &App{}
	for _, tt := range []struct {
		name   string
		method string
		body   string
		handle func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "create product role policy",
			method: http.MethodPost,
			body:   `{"role":"viewer","obj":"pipeline:*","act":"pipeline.read"}`,
			handle: app.handleCreateRole,
		},
		{
			name:   "create default admin policy",
			method: http.MethodPost,
			body:   `{"role":"` + defaultAdminRole + `","obj":"*:*","act":"*"}`,
			handle: app.handleCreateRole,
		},
		{
			name:   "delete product role policy",
			method: http.MethodDelete,
			body:   `{"role":"owner","obj":"pipeline:*","act":"pipeline.delete"}`,
			handle: app.handleDeleteRole,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/v1/admin/roles", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			tt.handle(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if !strings.Contains(rec.Body.String(), "default roles cannot be") {
				t.Fatalf("body = %q, want protected default role message", rec.Body.String())
			}
		})
	}
}

func TestResolveAAARolesUsesAAAIntrospection(t *testing.T) {
	client := newInMemoryAAAClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/authn/introspect":
			_ = json.NewEncoder(w).Encode(model.IntrospectResponse{
				ID:    "user-1",
				Sub:   "alice",
				Email: "alice@example.com",
				Roles: []string{"nopsai-runner", "nopsai-viewer"},
			})
		case "/v1/authz/check":
			_ = json.NewEncoder(w).Encode(model.Decision{Allowed: false, Reason: "default_deny"})
		default:
			http.NotFound(w, r)
		}
	}))

	app := &App{aaaClient: client}
	claims := &auth.Claims{
		Sub:      "alice",
		Email:    "alice@example.com",
		Provider: "local",
		Roles:    []string{"stale-role"},
	}

	roles := app.resolveAAARoles(context.Background(), claims)
	if len(roles) != 2 || roles[0] != "nopsai-runner" || roles[1] != "nopsai-viewer" {
		t.Fatalf("roles = %#v, want AAA roles", roles)
	}
}

func TestResolveAAARolesFallsBackToClaims(t *testing.T) {
	app := &App{}
	claims := &auth.Claims{Roles: []string{"local-role"}}

	roles := app.resolveAAARoles(context.Background(), claims)
	if len(roles) != 1 || roles[0] != "local-role" {
		t.Fatalf("roles = %#v, want claims fallback", roles)
	}
}

func TestBuildAAASubjectMapsServiceAccountToken(t *testing.T) {
	app := &App{}
	subject := app.buildAAASubject(&auth.Claims{
		Sub:      "deploy-bot",
		Provider: auth.ProviderServiceAccountToken,
	})

	if subject.Type != model.SubjectTypeServiceAccount {
		t.Fatalf("subject.Type = %q, want %q", subject.Type, model.SubjectTypeServiceAccount)
	}
	if subject.ID != "deploy-bot" {
		t.Fatalf("subject.ID = %q, want deploy-bot", subject.ID)
	}
}
