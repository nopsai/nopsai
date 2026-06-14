package nopsai

import "testing"

func TestKeycloakEntitlementProvidersByIDFiltersAndNormalizes(t *testing.T) {
	providers := keycloakEntitlementProvidersByID([]oidcProviderRecord{
		{
			ID: " Local Keycloak ",
			EntitlementSync: oidcEntitlementSyncConfig{
				Mode:         "keycloak",
				AdminBaseURL: " http://keycloak:8080/ ",
				Realm:        "nopsai",
			},
		},
		{
			ID:              "legacy",
			EntitlementSync: oidcEntitlementSyncConfig{},
		},
	})

	provider, ok := providers["local-keycloak"]
	if !ok {
		t.Fatalf("providers = %#v, want normalized local-keycloak entry", providers)
	}
	if provider.EntitlementSync.Mode != "keycloak_group_roles" {
		t.Fatalf("mode = %q, want keycloak_group_roles", provider.EntitlementSync.Mode)
	}
	if provider.EntitlementSync.AdminBaseURL != "http://keycloak:8080" {
		t.Fatalf("admin base URL = %q, want trimmed URL", provider.EntitlementSync.AdminBaseURL)
	}
	if _, ok := providers["legacy"]; ok {
		t.Fatalf("providers = %#v, want non-entitlement provider filtered out", providers)
	}
}
