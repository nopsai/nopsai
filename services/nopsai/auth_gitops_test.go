package nopsai

import (
	"context"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/models"
)

func TestParseGitOpsAuthSettingsPlanSystemRepo(t *testing.T) {
	plan, err := parseGitOpsAuthSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/auth.yaml": `
local_enabled: true
oidc:
  enabled: true
  auto_create_users: true
  default_role: ""
  domain_mapping:
    Example.COM: NopsAI
  providers:
    NopsAI:
      type: generic
      display_name: Local Keycloak
      issuer: http://keycloak:8080/realms/nopsai/
      client_id: nopsai
      client_credential_ref: credential://system/oidc/nopsai/client-secret
      scopes: ["openid"]
      allowed_email_domains: ["@Example.COM"]
      basic_role_mapping:
        /team-1/dev:
          role: owner
          resource_type: folder
          resource_id: team-1/dev
      entitlement_sync:
        mode: keycloak
        admin_base_url: http://keycloak:8080/
        realm: nopsai
        client_id: nopsai
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsAuthSettingsPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsAuthSettingsPlan() = nil, want plan")
	}
	if !plan.settings.LocalEnabled || !plan.settings.OIDCEnabled || !plan.settings.AutoCreateUsers {
		t.Fatalf("settings = %#v, want local and OIDC enabled with auto-create", plan.settings)
	}
	if plan.settings.DefaultRole != "" {
		t.Fatalf("default role = %q, want empty", plan.settings.DefaultRole)
	}
	if got := plan.domainMappings["example.com"]; got != "nopsai" {
		t.Fatalf("domain mapping = %#v, want example.com -> nopsai", plan.domainMappings)
	}
	provider := plan.providers["nopsai"]
	if provider.ID != "nopsai" || provider.Type != "oidc" || provider.Issuer != "http://keycloak:8080/realms/nopsai" {
		t.Fatalf("provider = %#v, want normalized nopsai OIDC provider", provider)
	}
	if provider.ClientCredentialRef != "credential://system/oidc/nopsai/client-secret" {
		t.Fatalf("provider client credential ref = %q, want fixture reference", provider.ClientCredentialRef)
	}
	if got := provider.AllowedEmailDomains; len(got) != 1 || got[0] != "example.com" {
		t.Fatalf("allowed domains = %#v, want normalized example.com", provider.AllowedEmailDomains)
	}
	grant := provider.BasicRoleMapping["/team-1/dev"]
	if grant.Role != productRoleOwner || grant.ResourceType != grantResourceFolder || grant.ResourceID != "team-1/dev" {
		t.Fatalf("basic role mapping = %#v, want owner folder team-1/dev", provider.BasicRoleMapping)
	}
	if provider.EntitlementSync.Mode != "keycloak_group_roles" || provider.EntitlementSync.AdminBaseURL != "http://keycloak:8080" {
		t.Fatalf("entitlement sync = %#v, want normalized keycloak sync", provider.EntitlementSync)
	}
}

func TestParseGitOpsAuthSettingsFileAcceptsWrappedAuthConfig(t *testing.T) {
	plan, err := parseGitOpsAuthSettingsFile(`
auth:
  local_enabled: false
  oidc:
    enabled: false
`, "setting/system/auth.yaml")
	if err != nil {
		t.Fatalf("parseGitOpsAuthSettingsFile() error = %v", err)
	}
	if plan.settings.LocalEnabled {
		t.Fatalf("local enabled = true, want wrapped false")
	}
	if plan.settings.OIDCEnabled {
		t.Fatalf("oidc enabled = true, want false")
	}
}

func TestParseGitOpsAuthSettingsPlanRejectsGroupRepo(t *testing.T) {
	_, err := parseGitOpsAuthSettingsPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/auth.yaml": "local_enabled: true",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsAuthSettingsFileRejectsInvalidProviderReferences(t *testing.T) {
	_, err := parseGitOpsAuthSettingsFile(`
local_enabled: true
oidc:
  enabled: true
  domain_mapping:
    example.com: missing
  providers:
    nopsai:
      issuer: http://keycloak:8080/realms/nopsai
      client_id: nopsai
`, "setting/system/auth.yaml")
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown provider error, got %v", err)
	}

	_, err = parseGitOpsAuthSettingsFile(`
local_enabled: true
oidc:
  enabled: true
  providers:
    nopsai:
      issuer: http://keycloak:8080/realms/nopsai
`, "setting/system/auth.yaml")
	if err == nil || !strings.Contains(err.Error(), "requires issuer and client_id") {
		t.Fatalf("expected missing client_id error, got %v", err)
	}
}

func TestParseGitOpsAuthSettingsFileRejectsMixedWrapperAndTopLevel(t *testing.T) {
	_, err := parseGitOpsAuthSettingsFile(`
local_enabled: true
auth:
  local_enabled: true
`, "setting/system/auth.yaml")
	if err == nil || !strings.Contains(err.Error(), "either auth: or top-level auth fields") {
		t.Fatalf("expected mixed-wrapper error, got %v", err)
	}
}

func TestIsGitOpsAuthSettingsRelativePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "system/auth.yaml", want: true},
		{path: "/system/auth.yaml", want: true},
		{path: "system/auth.yml", want: false},
		{path: "setting/system/auth.yaml", want: false},
		{path: "../system/auth.yaml", want: false},
		{path: "system/runner.yaml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isGitOpsAuthSettingsRelativePath(tt.path); got != tt.want {
				t.Fatalf("isGitOpsAuthSettingsRelativePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestBuildAuthSettingsGitOpsFileExportsCredentialReferences(t *testing.T) {
	doc := buildAuthSettingsGitOpsFile(
		oidcSettings{
			LocalEnabled:      false,
			OIDCEnabled:       true,
			AutoCreateUsers:   true,
			AllowEmailLinking: true,
		},
		[]oidcProviderRecord{
			{
				ID:                  "nopsai",
				Type:                "oidc",
				DisplayName:         "Local Keycloak",
				Issuer:              "http://keycloak:8080/realms/nopsai",
				ClientID:            "nopsai",
				ClientCredentialRef: "credential://system/oidc/nopsai/client-secret",
				Enabled:             false,
				EntitlementSync: oidcEntitlementSyncConfig{
					Mode:                       "keycloak_group_roles",
					AdminBaseURL:               "http://keycloak:8080",
					AdminClientCredentialRef:   "credential://system/oidc/nopsai/admin-client-secret",
					AdminPasswordCredentialRef: "credential://system/oidc/nopsai/admin-password",
				},
			},
		},
		map[string]string{"Example.COM": "NopsAI"},
	)
	if doc.LocalEnabled == nil || *doc.LocalEnabled {
		t.Fatalf("local_enabled = %#v, want explicit false", doc.LocalEnabled)
	}
	if doc.OIDC == nil || !doc.OIDC.Enabled || !doc.OIDC.AutoCreateUsers || !doc.OIDC.AllowEmailLinking {
		t.Fatalf("oidc = %#v, want enabled auto-create/linking", doc.OIDC)
	}
	provider := doc.OIDC.Providers["nopsai"]
	if provider.ClientCredentialRef != "credential://system/oidc/nopsai/client-secret" {
		t.Fatalf("client credential ref = %q, want exported reference", provider.ClientCredentialRef)
	}
	if provider.EntitlementSync.AdminClientCredentialRef != "credential://system/oidc/nopsai/admin-client-secret" ||
		provider.EntitlementSync.AdminPasswordCredentialRef != "credential://system/oidc/nopsai/admin-password" {
		t.Fatalf("entitlement credential references = %#v", provider.EntitlementSync)
	}
	if provider.Enabled == nil || *provider.Enabled {
		t.Fatalf("provider enabled = %#v, want explicit false", provider.Enabled)
	}
	if got := doc.OIDC.DomainMapping["example.com"]; got != "nopsai" {
		t.Fatalf("domain mapping = %#v, want normalized example.com -> nopsai", doc.OIDC.DomainMapping)
	}
	if !authSettingsGitOpsFileHasState(doc) {
		t.Fatal("authSettingsGitOpsFileHasState() = false, want true")
	}
}

func TestExportConfigRepositoryAuthSettingsUsesCanonicalPathAndCredentialReferences(t *testing.T) {
	app := App{cfg: &config.Config{
		AuthProviderLocalEnabled: true,
		Auth: config.AuthConfig{
			OIDC: config.OIDCAuthConfig{
				Enabled:         true,
				AutoCreateUsers: true,
				DomainMapping: map[string]string{
					"example.com": "nopsai",
				},
				Providers: map[string]config.OIDCProviderConfig{
					"nopsai": {
						DisplayName:         "Local Keycloak",
						Issuer:              "http://keycloak:8080/realms/nopsai",
						ClientID:            "nopsai",
						ClientCredentialRef: "credential://system/oidc/nopsai/client-secret",
					},
				},
			},
		},
	}}
	files := map[string]string{}

	if err := app.exportConfigRepositoryAuthSettings(context.Background(), models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem}, files); err != nil {
		t.Fatalf("exportConfigRepositoryAuthSettings() error = %v", err)
	}
	content, ok := files["setting/system/auth.yaml"]
	if !ok {
		t.Fatalf("missing canonical auth settings export path: %#v", files)
	}
	if _, ok := files["settings/system/auth.yaml"]; ok {
		t.Fatalf("unexpected compatibility auth settings export path: %#v", files)
	}
	if !strings.Contains(content, "client_credential_ref: credential://system/oidc/nopsai/client-secret") {
		t.Fatalf("exported auth settings missing provider credential reference: %s", content)
	}
}
