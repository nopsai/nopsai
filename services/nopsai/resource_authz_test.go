package nopsai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
)

func TestIsSameTeamBoundaryExamples(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		resource string
		want     bool
	}{
		{name: "same team", caller: "team-1", resource: "team-1", want: true},
		{name: "child resource", caller: "team-1", resource: "team-1/shared", want: true},
		{name: "sibling under team", caller: "team-1/app", resource: "team-1/shared", want: true},
		{name: "other team", caller: "team-1", resource: "team-2", want: false},
		{name: "platform boundary", caller: "team-1", resource: "platform/shared", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSameTeamBoundary(tt.caller, tt.resource)
			if got != tt.want {
				t.Fatalf("IsSameTeamBoundary(%q, %q) = %t, want %t", tt.caller, tt.resource, got, tt.want)
			}
		})
	}
}

func TestTeamGrantIncludesCallerTeam(t *testing.T) {
	tests := []struct {
		name       string
		grantTeam  string
		callerTeam string
		want       bool
	}{
		{name: "same team", grantTeam: "team-1", callerTeam: "team-1", want: true},
		{name: "child caller", grantTeam: "team-1", callerTeam: "team-1/app", want: true},
		{name: "sibling caller excluded", grantTeam: "team-1/shared", callerTeam: "team-1/app", want: false},
		{name: "other team", grantTeam: "team-1", callerTeam: "team-2/app", want: false},
		{name: "general excluded", grantTeam: generalGrantID, callerTeam: "team-1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := teamGrantIncludesCallerTeam(tt.grantTeam, tt.callerTeam)
			if got != tt.want {
				t.Fatalf("teamGrantIncludesCallerTeam(%q, %q) = %t, want %t", tt.grantTeam, tt.callerTeam, got, tt.want)
			}
		})
	}
}

func TestCollectReferencedPipelineIdentifiers(t *testing.T) {
	pipeline := &models.Pipeline{
		Steps: []models.PipelineStep{
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "build"}, Include: "pipeline:platform/shared/build"}},
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "notify"}, Include: "step:team-1/notify"}},
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "deploy"}, Include: "pipeline:/platform/shared/deploy/"}},
			{Step: &models.IncludeStep{BaseStep: models.BaseStep{Name: "dupe"}, Include: "pipeline:platform/shared/build"}},
		},
	}

	got := collectReferencedPipelineIdentifiers(pipeline)
	want := []string{"platform/shared/build", "platform/shared/deploy"}
	if len(got) != len(want) {
		t.Fatalf("collectReferencedPipelineIdentifiers() = %#v, want %#v", got, want)
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("collectReferencedPipelineIdentifiers()[%d] = %q, want %q", idx, got[idx], want[idx])
		}
	}
}

func TestRepositoryPipelineSourceForIdentifier(t *testing.T) {
	tests := []struct {
		name string
		path string
		file string
		ext  string
		want string
	}{
		{name: "adds nopsai prefix", file: "main-pipeline", ext: ".yaml", want: ".nopsai/main-pipeline.yaml"},
		{name: "defaults extension", file: "main-pipeline", want: ".nopsai/main-pipeline.yaml"},
		{name: "keeps nested path", path: "app", file: "build", ext: ".yml", want: ".nopsai/app/build.yml"},
		{name: "keeps existing nopsai prefix", path: ".nopsai", file: "deploy", ext: ".yaml", want: ".nopsai/deploy.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repositoryPipelineSourceForIdentifier(tt.path, tt.file, tt.ext)
			if got.Path != tt.want {
				t.Fatalf("repositoryPipelineSourceForIdentifier(%q, %q, %q).Path = %q, want %q", tt.path, tt.file, tt.ext, got.Path, tt.want)
			}
		})
	}
}

func TestTopLevelPipelineUseAuthorizationSkippedForRepositorySource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{source: "repository", want: false},
		{source: "git", want: false},
		{source: "database override", want: true},
		{source: "database owner override", want: true},
		{source: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			got := shouldAuthorizeTopLevelPipelineUse(tt.source)
			if got != tt.want {
				t.Fatalf("shouldAuthorizeTopLevelPipelineUse(%q) = %t, want %t", tt.source, got, tt.want)
			}
		})
	}
}

func TestExplicitResourceUseDenyClassification(t *testing.T) {
	tests := []struct {
		name     string
		decision model.Decision
		want     bool
	}{
		{
			name:     "default deny is not explicit",
			decision: model.Decision{Allowed: false, Reason: "default_deny"},
			want:     false,
		},
		{
			name:     "resource not found is not explicit",
			decision: model.Decision{Allowed: false, Reason: "resource_not_found"},
			want:     false,
		},
		{
			name:     "direct deny policy is explicit",
			decision: model.Decision{Allowed: false, Reason: "direct_acl_deny", MatchedPolicy: map[string]any{"id": "grant-1"}},
			want:     true,
		},
		{
			name:     "deny reason without matched policy is not explicit",
			decision: model.Decision{Allowed: false, Reason: "direct_acl_deny"},
			want:     false,
		},
		{
			name:     "allow decision is not explicit deny",
			decision: model.Decision{Allowed: true, Reason: "direct_acl_allow", MatchedPolicy: map[string]any{"id": "grant-1"}},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExplicitResourceUseDeny(tt.decision)
			if got != tt.want {
				t.Fatalf("isExplicitResourceUseDeny(%#v) = %t, want %t", tt.decision, got, tt.want)
			}
		})
	}
}

func TestParentScopeForRuntimeResourceUse(t *testing.T) {
	tests := []struct {
		name         string
		action       string
		resourceType string
		resourceID   string
		wantScope    string
		wantOK       bool
	}{
		{
			name:         "scoped secret",
			action:       "secret.use",
			resourceType: grantResourceSecret,
			resourceID:   model.BuildNamedResourceID("", "prod", "TEST_SECRET"),
			wantScope:    "prod",
			wantOK:       true,
		},
		{
			name:         "repo scoped variable",
			action:       "variable.use",
			resourceType: grantResourceVariable,
			resourceID:   model.BuildNamedResourceID("nopsai/test-app", "team-1/dev", "API_URL"),
			wantScope:    "team-1/dev",
			wantOK:       true,
		},
		{
			name:         "unscoped secret",
			action:       "secret.use",
			resourceType: grantResourceSecret,
			resourceID:   model.BuildNamedResourceID("", "", "TOKEN"),
			wantScope:    "default",
			wantOK:       true,
		},
		{
			name:         "read value is not runtime use",
			action:       "secret.read_value",
			resourceType: grantResourceSecret,
			resourceID:   model.BuildNamedResourceID("", "prod", "TEST_SECRET"),
			wantOK:       false,
		},
		{
			name:         "pipeline resource ignored",
			action:       "pipeline.use",
			resourceType: grantResourcePipeline,
			resourceID:   "prod/build",
			wantOK:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotScope, gotOK := parentScopeForRuntimeResourceUse(tt.action, tt.resourceType, tt.resourceID)
			if gotScope != tt.wantScope || gotOK != tt.wantOK {
				t.Fatalf("parentScopeForRuntimeResourceUse() = (%q, %t), want (%q, %t)", gotScope, gotOK, tt.wantScope, tt.wantOK)
			}
		})
	}
}

func TestSameTeamResourceUseAllowed(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		visibility   string
		want         bool
	}{
		{name: "knowledge context team", resourceType: grantResourceKnowledgeContext, visibility: resourceVisibilityTeam, want: true},
		{name: "knowledge context restricted", resourceType: grantResourceKnowledgeContext, visibility: resourceVisibilityRestricted, want: true},
		{name: "pipeline team", resourceType: grantResourcePipeline, visibility: resourceVisibilityTeam, want: true},
		{name: "scope team", resourceType: grantResourceScope, visibility: resourceVisibilityTeam, want: true},
		{name: "secret team", resourceType: grantResourceSecret, visibility: resourceVisibilityTeam, want: false},
		{name: "knowledge context public", resourceType: grantResourceKnowledgeContext, visibility: resourceVisibilityWorkspace, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sameTeamResourceUseAllowed(tt.resourceType, tt.visibility)
			if got != tt.want {
				t.Fatalf("sameTeamResourceUseAllowed(%q, %q) = %t, want %t", tt.resourceType, tt.visibility, got, tt.want)
			}
		})
	}
}

func TestAuthorizeResourceUseAllowsScopedSecretThroughScopeUse(t *testing.T) {
	app := &App{
		aaaClient: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				if action == "scope.use" && resource.Type == grantResourceScope && resource.ID == "prod" {
					return model.Decision{
						Allowed: true,
						Reason:  "direct_acl_allow",
						MatchedPolicy: map[string]any{
							"resource_type": grantResourceScope,
							"resource_id":   "prod",
						},
					}, nil
				}
				return model.Decision{Allowed: false, Reason: "default_deny"}, nil
			},
		},
	}

	result, err := app.AuthorizeResourceUse(context.Background(), ResourceUseAuthInput{
		CallerType:   model.SubjectTypeRepository,
		CallerID:     "nopsai/test-app",
		Action:       "secret.use",
		ResourceType: grantResourceSecret,
		ResourceID:   model.BuildNamedResourceID("", "prod", "TEST_SECRET"),
	})
	if err != nil {
		t.Fatalf("AuthorizeResourceUse() error = %v", err)
	}
	if !result.Allowed {
		t.Fatalf("AuthorizeResourceUse() allowed = false, result = %#v", result)
	}
	if result.Reason != resourceUseReasonScopeAccess {
		t.Fatalf("AuthorizeResourceUse() reason = %q, want %q", result.Reason, resourceUseReasonScopeAccess)
	}
	if result.MatchedResource != "scope:prod" {
		t.Fatalf("AuthorizeResourceUse() matched resource = %q, want scope:prod", result.MatchedResource)
	}
}

func TestAuthorizeResourceUseAllowsDefaultSecretThroughDefaultScopeUse(t *testing.T) {
	app := &App{
		aaaClient: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				if action == "scope.use" && resource.Type == grantResourceScope && resource.ID == "" {
					return model.Decision{
						Allowed: true,
						Reason:  "direct_acl_allow",
						MatchedPolicy: map[string]any{
							"resource_type": grantResourceScope,
							"resource_id":   "",
						},
					}, nil
				}
				return model.Decision{Allowed: false, Reason: "default_deny"}, nil
			},
		},
	}

	result, err := app.AuthorizeResourceUse(context.Background(), ResourceUseAuthInput{
		CallerType:   model.SubjectTypeRepository,
		CallerID:     "nopsai/test-app",
		Action:       "secret.use",
		ResourceType: grantResourceSecret,
		ResourceID:   model.BuildNamedResourceID("", "", "OTHER_SEC"),
	})
	if err != nil {
		t.Fatalf("AuthorizeResourceUse() error = %v", err)
	}
	if !result.Allowed {
		t.Fatalf("AuthorizeResourceUse() allowed = false, result = %#v", result)
	}
	if result.Reason != resourceUseReasonScopeAccess {
		t.Fatalf("AuthorizeResourceUse() reason = %q, want %q", result.Reason, resourceUseReasonScopeAccess)
	}
	if result.MatchedResource != "scope:default" {
		t.Fatalf("AuthorizeResourceUse() matched resource = %q, want scope:default", result.MatchedResource)
	}
}

func TestResourceUseFailureSummaryIncludesDecisionDetails(t *testing.T) {
	result := ResourceUseAuthResult{
		Allowed:      false,
		Reason:       resourceUseReasonDenied,
		Action:       "pipeline.use",
		ResourceType: grantResourcePipeline,
		ResourceID:   "platform/shared/deploy",
		CallerTeam:   "team-1",
		ResourceTeam: "platform",
		Visibility:   resourceVisibilityTeam,
		EventType:    "push",
		Ref:          "refs/heads/main",
		Repo:         "nopsai/test-app",
	}

	got := resourceUseFailureSummary("nopsai/test-app", result, nil)

	assertContains(t, got, "repository:nopsai/test-app is not allowed to use pipeline platform/shared/deploy")
	assertContains(t, got, "Caller: repository:nopsai/test-app")
	assertContains(t, got, "Repository: nopsai/test-app")
	assertContains(t, got, "Action: pipeline.use")
	assertContains(t, got, "Resource: pipeline:platform/shared/deploy")
	assertContains(t, got, "Event: push")
	assertContains(t, got, "Ref: refs/heads/main")
	assertContains(t, got, "Caller team: team-1")
	assertContains(t, got, "Resource team: platform")
	assertContains(t, got, "Visibility: team")
	assertContains(t, got, "Decision reason: denied")
	assertContains(t, got, "Why: cross-team use from team-1 to platform requires an explicit grant or public visibility")
}

func TestResourceUseFailureSummaryIncludesAuthorizationError(t *testing.T) {
	result := ResourceUseAuthResult{
		Allowed:      false,
		Action:       "scope.use",
		ResourceType: grantResourceScope,
		ResourceID:   "platform/prod",
	}

	got := resourceUseFailureSummary("nopsai/test-app", result, errors.New("aaa offline"))

	assertContains(t, got, "Authorization unavailable for scope:platform/prod: aaa offline")
	assertContains(t, got, "Caller: repository:nopsai/test-app")
	assertContains(t, got, "Action: scope.use")
	assertContains(t, got, "Resource: scope:platform/prod")
	assertContains(t, got, "Decision reason: authorization_error")
	assertContains(t, got, "Why: the authorization service could not complete the check")
	assertContains(t, got, "Error: aaa offline")
}

func TestResourceUseDeniedMessageIncludesNamedResourceScope(t *testing.T) {
	result := ResourceUseAuthResult{
		Allowed:      false,
		ResourceType: grantResourceSecret,
		ResourceID:   model.BuildNamedResourceID("", "", "OTHER_SEC"),
	}

	got := resourceUseDeniedMessage(model.SubjectTypeRepository, "nopsai/test-app", result)

	assertContains(t, got, "repository:nopsai/test-app is not allowed to use secret:name=OTHER_SEC scope=default")
}

func TestFormatResourceLabelIncludesNamedResourceScopeAndRepo(t *testing.T) {
	got := formatResourceLabel(grantResourceVariable, model.BuildNamedResourceID("nopsai/test-app", "dev", "TEST_ENV"))
	want := "variable:name=TEST_ENV scope=dev repo=nopsai/test-app"
	if got != want {
		t.Fatalf("formatResourceLabel() = %q, want %q", got, want)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected summary to contain %q, got:\n%s", want, got)
	}
}
