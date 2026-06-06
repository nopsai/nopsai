package nopsai

import (
	"testing"

	"nopsai/services/aaa/pkg/model"
)

func TestRuntimeScopeStorageAndResourceMapping(t *testing.T) {
	tests := []struct {
		raw          string
		wantStorage  string
		wantResource string
		wantDisplay  string
	}{
		{raw: "", wantStorage: "default", wantResource: "", wantDisplay: "default"},
		{raw: "default", wantStorage: "default", wantResource: "", wantDisplay: "default"},
		{raw: "/default/", wantStorage: "default", wantResource: "", wantDisplay: "default"},
		{raw: "prod", wantStorage: "prod", wantResource: "prod", wantDisplay: "prod"},
		{raw: "data-team/dev", wantStorage: "data-team/dev", wantResource: "data-team/dev", wantDisplay: "data-team/dev"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := runtimeScopeForStorage(tt.raw); got != tt.wantStorage {
				t.Fatalf("runtimeScopeForStorage() = %q, want %q", got, tt.wantStorage)
			}
			if got := runtimeScopeForResource(tt.raw); got != tt.wantResource {
				t.Fatalf("runtimeScopeForResource() = %q, want %q", got, tt.wantResource)
			}
			if got := runtimeScopeForDisplay(tt.raw); got != tt.wantDisplay {
				t.Fatalf("runtimeScopeForDisplay() = %q, want %q", got, tt.wantDisplay)
			}
		})
	}
}

func TestRuntimeScopeEqualsSQLUsesCanonicalScopeOnly(t *testing.T) {
	if got := runtimeScopeEqualsSQL("scope", 1, "default"); got != "scope = $1" {
		t.Fatalf("runtimeScopeEqualsSQL(default) = %q", got)
	}
	if got := runtimeScopeEqualsSQL("variables.scope", 3, "prod"); got != "variables.scope = $3" {
		t.Fatalf("runtimeScopeEqualsSQL(prod) = %q", got)
	}
}

func TestRuntimeNamedResourceIDForResourceNormalizesDefaultScope(t *testing.T) {
	raw := model.BuildNamedResourceID("hosein-yousefii/test-app", "default", "TOKEN")
	want := model.BuildNamedResourceID("hosein-yousefii/test-app", "", "TOKEN")
	if got := runtimeNamedResourceIDForResource(raw); got != want {
		t.Fatalf("runtimeNamedResourceIDForResource() = %q, want %q", got, want)
	}
}
