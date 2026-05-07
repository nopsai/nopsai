package routeauthz

import (
	"net/http/httptest"
	"testing"
)

func TestMapRequestUsesFilterForTriggerAndScopeLists(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "trigger overrides", path: "/v1/overrides"},
		{name: "secret scopes", path: "/v1/secrets/scopes"},
		{name: "variable scopes", path: "/v1/variables/scopes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			action, _, requiresFilter, err := MapRequest(req)
			if err != nil {
				t.Fatalf("MapRequest() error = %v", err)
			}
			if action == "" {
				t.Fatal("MapRequest() action = empty, want authz action")
			}
			if !requiresFilter {
				t.Fatal("MapRequest() requiresFilter = false, want true")
			}
		})
	}
}

func TestMapRequestDefersRunByCheckAuthorizationToConcreteRun(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/runs-by-check/check-123", nil)
	action, resource, requiresFilter, err := MapRequest(req)
	if err != nil {
		t.Fatalf("MapRequest() error = %v", err)
	}
	if action != "" {
		t.Fatalf("MapRequest() action = %q, want empty", action)
	}
	if requiresFilter {
		t.Fatal("MapRequest() requiresFilter = true, want false")
	}
	if resource.Type != "" || resource.ID != "" {
		t.Fatalf("MapRequest() resource = %#v, want empty", resource)
	}
}
