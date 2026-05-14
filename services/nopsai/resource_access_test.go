package main

import "testing"

func TestParseResourceAccessPath(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		wantOperation string
		wantType      string
		wantID        string
		wantGrantID   string
	}{
		{
			name:          "access",
			path:          "/v1/resources/pipeline/team-1/build/access",
			wantOperation: "access",
			wantType:      "pipeline",
			wantID:        "team-1/build",
		},
		{
			name:          "create grant",
			path:          "/v1/resources/scope/team-1/dev/grants",
			wantOperation: "grants",
			wantType:      "scope",
			wantID:        "team-1/dev",
		},
		{
			name:          "delete grant",
			path:          "/v1/resources/step/platform/docker-build/grants/grant_42",
			wantOperation: "grant",
			wantType:      "step",
			wantID:        "platform/docker-build",
			wantGrantID:   "grant_42",
		},
		{
			name:          "encoded path segment",
			path:          "/v1/resources/pipeline/team%201/build/access",
			wantOperation: "access",
			wantType:      "pipeline",
			wantID:        "team 1/build",
		},
		{
			name:          "default scope",
			path:          "/v1/resources/scope/default/access",
			wantOperation: "access",
			wantType:      "scope",
			wantID:        "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseResourceAccessPath(tt.path)
			if err != nil {
				t.Fatalf("parseResourceAccessPath() error = %v", err)
			}
			if got.Operation != tt.wantOperation || got.ResourceType != tt.wantType || got.ResourceID != tt.wantID || got.GrantID != tt.wantGrantID {
				t.Fatalf("parseResourceAccessPath() = %#v", got)
			}
		})
	}
}

func TestNormalizeScopeGrantResourceID(t *testing.T) {
	id, lookup, display := normalizeScopeGrantResourceID("default")
	if id != "" || lookup != "" || display != "default" {
		t.Fatalf("normalizeScopeGrantResourceID(default) = (%q, %q, %q), want empty lookup with default display", id, lookup, display)
	}
	if !isDefaultScopeGrantResource(id, display) {
		t.Fatal("default scope should be recognized as a logical scope resource")
	}

	id, lookup, display = normalizeScopeGrantResourceID("/team-1/dev/")
	if id != "team-1/dev" || lookup != "team-1/dev" || display != "team-1/dev" {
		t.Fatalf("normalizeScopeGrantResourceID(team scope) = (%q, %q, %q)", id, lookup, display)
	}
	if isDefaultScopeGrantResource(id, display) {
		t.Fatal("non-default scope was recognized as default")
	}
}

func TestResolveDefaultScopeGrantResourceDoesNotRequireRows(t *testing.T) {
	resource, err := resolveAccessGrantResource(t.Context(), &noopQueryRunner{}, grantResourceScope, "default", true)
	if err != nil {
		t.Fatalf("resolveAccessGrantResource(default scope) error = %v", err)
	}
	if resource.Type != grantResourceScope || resource.ID != "" || resource.Display != "default" {
		t.Fatalf("default scope resource = %#v", resource)
	}
}

func TestValidateResourceVisibilityPolicy(t *testing.T) {
	if err := validateResourceVisibilityPolicy(grantResourcePipeline, resourceVisibilityWorkspace); err != nil {
		t.Fatalf("pipeline workspace visibility error = %v", err)
	}
	if err := validateResourceVisibilityPolicy(grantResourceStep, resourceVisibilityWorkspace); err != nil {
		t.Fatalf("step workspace visibility error = %v", err)
	}
	for _, resourceType := range []string{grantResourceScope, grantResourceSecret, grantResourceVariable, grantResourceRunner} {
		t.Run(resourceType, func(t *testing.T) {
			if err := validateResourceVisibilityPolicy(resourceType, resourceVisibilityWorkspace); err == nil {
				t.Fatalf("validateResourceVisibilityPolicy(%q, workspace) error = nil, want error", resourceType)
			}
			if err := validateResourceVisibilityPolicy(resourceType, resourceVisibilityRestricted); err != nil {
				t.Fatalf("validateResourceVisibilityPolicy(%q, restricted) error = %v", resourceType, err)
			}
		})
	}
}

func TestNormalizeUseGrantActions(t *testing.T) {
	actions, err := normalizeUseGrantActions(grantResourcePipeline, nil)
	if err != nil {
		t.Fatalf("normalizeUseGrantActions() error = %v", err)
	}
	if len(actions) != 1 || actions[0] != "pipeline.use" {
		t.Fatalf("normalizeUseGrantActions() = %#v, want pipeline.use", actions)
	}

	if _, err := normalizeUseGrantActions(grantResourcePipeline, []string{"pipeline.execute"}); err == nil {
		t.Fatal("normalizeUseGrantActions() accepted non-use action")
	}
	if _, err := normalizeUseGrantActions(grantResourcePipeline, []string{"scope.use"}); err == nil {
		t.Fatal("normalizeUseGrantActions() accepted mismatched use action")
	}
}
