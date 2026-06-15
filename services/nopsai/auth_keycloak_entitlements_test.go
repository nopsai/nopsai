package nopsai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestEnrichOIDCIdentityEntitlementsUsesKeycloakUserAndGroupClientRoles(t *testing.T) {
	ctx := context.Background()
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/master/protocol/openid-connect/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			tokenForm = r.PostForm
			_ = json.NewEncoder(w).Encode(keycloakTokenResponse{AccessToken: "admin-token"})
		case "/admin/realms/nopsai/clients":
			if !requireKeycloakTestBearer(w, r) {
				return
			}
			if got := r.URL.Query().Get("clientId"); got != "nopsai" {
				http.Error(w, "unexpected clientId "+got, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakClientRepresentation{{ID: "client-uuid", ClientID: "nopsai"}})
		case "/admin/realms/nopsai/users/user-1/role-mappings/clients/client-uuid":
			if !requireKeycloakTestBearer(w, r) {
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakRoleRepresentation{{Name: "nopsai-admin"}, {Name: "ignored"}})
		case "/admin/realms/nopsai/users/user-1/groups":
			if !requireKeycloakTestBearer(w, r) {
				return
			}
			if got := r.URL.Query().Get("briefRepresentation"); got != "false" {
				http.Error(w, "unexpected briefRepresentation "+got, http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakGroupRepresentation{
				{ID: "team-1-id", Name: "team-1", Path: "/team-1"},
				{ID: "team-1-dev-id", Name: "dev"},
			})
		case "/admin/realms/nopsai/groups/team-1-dev-id":
			if !requireKeycloakTestBearer(w, r) {
				return
			}
			_ = json.NewEncoder(w).Encode(keycloakGroupRepresentation{ID: "team-1-dev-id", Name: "dev", Path: "/team-1/dev"})
		case "/admin/realms/nopsai/groups/team-1-id/role-mappings/clients/client-uuid/composite":
			if !requireKeycloakTestBearer(w, r) {
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakRoleRepresentation{{Name: "owner"}, {Name: "nopsai-admin"}})
		case "/admin/realms/nopsai/groups/team-1-dev-id/role-mappings/clients/client-uuid/composite":
			if !requireKeycloakTestBearer(w, r) {
				return
			}
			_ = json.NewEncoder(w).Encode([]keycloakRoleRepresentation{{Name: "viewer"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{
		httpClient: server.Client(),
		credentialResolver: staticCredentialResolver{
			"credential://system/oidc/nopsai/admin-password": "admin",
		},
	}
	provider := oidcProviderRecord{
		ID:       "nopsai",
		ClientID: "nopsai",
		EntitlementSync: oidcEntitlementSyncConfig{
			Mode:                       "keycloak_group_roles",
			AdminBaseURL:               server.URL,
			Realm:                      "nopsai",
			AdminRealm:                 "master",
			AdminClientID:              "admin-cli",
			AdminUsername:              "admin",
			AdminPasswordCredentialRef: "credential://system/oidc/nopsai/admin-password",
			TargetResourceType:         grantResourceFolder,
		},
	}

	identity, err := app.enrichOIDCIdentityEntitlements(ctx, provider, oidcVerifiedIdentity{
		Subject: "user-1",
		Email:   "alice@example.com",
		Groups:  []string{"/legacy"},
	})
	if err != nil {
		t.Fatalf("enrichOIDCIdentityEntitlements() error = %v", err)
	}
	if tokenForm.Get("grant_type") != "password" || tokenForm.Get("client_id") != "admin-cli" || tokenForm.Get("username") != "admin" {
		t.Fatalf("token form = %#v, want password-grant admin token request", tokenForm)
	}
	if !reflect.DeepEqual(identity.AccessRoles, []string{defaultAdminRole}) {
		t.Fatalf("access roles = %#v, want direct Keycloak client role as global access role", identity.AccessRoles)
	}
	if !reflect.DeepEqual(identity.Groups, []string{"/legacy", "/team-1", "/team-1/dev"}) {
		t.Fatalf("groups = %#v, want existing groups plus Keycloak group paths", identity.Groups)
	}

	grantsByTarget := map[string]oidcDesiredBasicRoleGrant{}
	for _, grant := range identity.BasicRoles {
		grantsByTarget[grant.ResourceType+":"+grant.ResourceID] = grant
	}
	teamGrant := grantsByTarget["folder:team-1"]
	if teamGrant.Role != productRoleOwner || teamGrant.ExternalGroup != "/team-1" || teamGrant.RequireResourceExists || !teamGrant.Inherit {
		t.Fatalf("team grant = %#v, want owner basic role from Keycloak group client role without pre-existing target requirement", teamGrant)
	}
	devGrant := grantsByTarget["folder:team-1/dev"]
	if devGrant.Role != productRoleViewer || devGrant.ExternalGroup != "/team-1/dev" || devGrant.RequireResourceExists || !devGrant.Inherit {
		t.Fatalf("dev grant = %#v, want viewer basic role from Keycloak subgroup detail path without pre-existing target requirement", devGrant)
	}
}

func TestKeycloakAdminTokenSupportsClientCredentials(t *testing.T) {
	var tokenForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/master/protocol/openid-connect/token" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tokenForm = r.PostForm
		_ = json.NewEncoder(w).Encode(keycloakTokenResponse{AccessToken: "service-token"})
	}))
	defer server.Close()

	app := &App{
		httpClient: server.Client(),
		credentialResolver: staticCredentialResolver{
			"credential://system/oidc/nopsai/admin-client-secret": "secret",
		},
	}
	token, err := app.keycloakAdminToken(context.Background(), oidcEntitlementSyncConfig{
		AdminBaseURL:             server.URL,
		AdminRealm:               "master",
		AdminClientID:            "nopsai-admin",
		AdminClientCredentialRef: "credential://system/oidc/nopsai/admin-client-secret",
	})
	if err != nil {
		t.Fatalf("keycloakAdminToken() error = %v", err)
	}
	if token != "service-token" {
		t.Fatalf("token = %q, want service-token", token)
	}
	if tokenForm.Get("grant_type") != "client_credentials" || tokenForm.Get("client_id") != "nopsai-admin" || tokenForm.Get("client_secret") != "secret" {
		t.Fatalf("token form = %#v, want client credentials token request", tokenForm)
	}
}

func TestKeycloakGroupTargetMapsPathsToFolderTargets(t *testing.T) {
	sync := oidcEntitlementSyncConfig{GroupPathPrefix: "/teams/"}
	tests := map[string]struct {
		group keycloakGroupRepresentation
		want  string
	}{
		"prefixed nested path": {
			group: keycloakGroupRepresentation{Name: "dev", Path: "/teams/team-1/dev"},
			want:  "team-1/dev",
		},
		"prefix root only": {
			group: keycloakGroupRepresentation{Name: "teams", Path: "/teams"},
			want:  "",
		},
		"name fallback": {
			group: keycloakGroupRepresentation{Name: "operations"},
			want:  "operations",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := keycloakGroupTarget(sync, tc.group); got != tc.want {
				t.Fatalf("keycloakGroupTarget() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeExternalRoleNamesKeepAccessAndBasicRolesSeparate(t *testing.T) {
	accessRole, ok := normalizeExternalAccessRoleName("nopsai-admin")
	if !ok || accessRole != defaultAdminRole {
		t.Fatalf("access role = %q/%v, want nopsai-admin preserved as global access role", accessRole, ok)
	}
	accessRole, ok = normalizeExternalAccessRoleName("admin")
	if !ok || accessRole != productRoleAdmin {
		t.Fatalf("access role = %q/%v, want admin -> admin", accessRole, ok)
	}
	accessRole, ok = normalizeExternalAccessRoleName("Owner")
	if !ok || accessRole != productRoleOwner {
		t.Fatalf("access role = %q/%v, want Owner -> owner", accessRole, ok)
	}
	basicRole, ok := normalizeExternalBasicRoleName("owner")
	if !ok || basicRole != productRoleOwner {
		t.Fatalf("basic role = %q/%v, want owner -> owner", basicRole, ok)
	}
	if role, ok := normalizeExternalBasicRoleName("nopsai-admin"); ok || role != "" {
		t.Fatalf("basic role = %q/%v, want nopsai-admin rejected for scoped basic grants", role, ok)
	}
}

func requireKeycloakTestBearer(w http.ResponseWriter, r *http.Request) bool {
	if got := r.Header.Get("Authorization"); got != "Bearer admin-token" {
		http.Error(w, "unexpected authorization "+got, http.StatusUnauthorized)
		return false
	}
	return true
}
