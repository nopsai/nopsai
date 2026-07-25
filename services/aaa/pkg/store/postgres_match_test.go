package store

import (
	"reflect"
	"testing"

	"nopsai/services/aaa/pkg/model"
)

func TestNamedResourceSubsetMatchMatchesScopeOnly(t *testing.T) {
	requestID := model.BuildNamedResourceID("owner/repo", "prod", "TOKEN")

	if !namedResourceSubsetMatch("scope=prod", requestID) {
		t.Fatal("expected scope-only selector to match secret in same scope")
	}
	if !namedResourceSubsetMatch("repo=owner%2Frepo&scope=prod", requestID) {
		t.Fatal("expected repo+scope selector to match secret in same repo and scope")
	}
}

func TestNamedResourceSubsetMatchMatchesDefaultScope(t *testing.T) {
	requestID := model.BuildNamedResourceID("owner/repo", "", "TOKEN")

	if !namedResourceSubsetMatch("scope=&name=TOKEN", requestID) {
		t.Fatal("expected default-scope selector to match secret without explicit scope")
	}
	if namedResourceSubsetMatch("scope=prod", requestID) {
		t.Fatal("did not expect prod scope selector to match default-scope secret")
	}
}

func TestNamedResourceSubsetMatchAllowsOptionalName(t *testing.T) {
	requestID := model.BuildNamedResourceID("", "prod", "TIMEOUT")

	if !namedResourceSubsetMatch("scope=prod", requestID) {
		t.Fatal("expected scope-only variable selector to match any variable name in scope")
	}
	if !namedResourceSubsetMatch("*", requestID) {
		t.Fatal("expected wildcard selector to match any named resource")
	}
}

func TestRolePermissionResourceMatchesUsesNamedSubsetForSecretsAndVariables(t *testing.T) {
	secret := model.ResourceRef{Type: "secret", ID: model.BuildNamedResourceID("owner/repo", "prod", "TOKEN")}
	variable := model.ResourceRef{Type: "variable", ID: model.BuildNamedResourceID("owner/repo", "prod", "TIMEOUT")}

	if !rolePermissionResourceMatches("secret", "scope=prod", secret) {
		t.Fatal("expected secret scope selector to match")
	}
	if !rolePermissionResourceMatches("variable", "repo=owner%2Frepo", variable) {
		t.Fatal("expected repository selector to match variable resource")
	}
	if rolePermissionResourceMatches("pipeline", "scope=prod", secret) {
		t.Fatal("did not expect unrelated resource type to match")
	}
}

func TestRolePermissionSpecificityPrefersMoreSpecificNamedSelector(t *testing.T) {
	resource := model.ResourceRef{Type: "secret", ID: model.BuildNamedResourceID("owner/repo", "prod", "TOKEN")}
	scopePolicy := model.MatchedPolicy{
		RoleName:     "viewer",
		ResourceType: "secret",
		ResourceID:   "scope=prod",
		Action:       "secret.read_value",
	}
	exactPolicy := model.MatchedPolicy{
		RoleName:     "viewer",
		ResourceType: "secret",
		ResourceID:   model.BuildNamedResourceID("owner/repo", "prod", "TOKEN"),
		Action:       "secret.read_value",
	}

	scopeSpecificity := rolePermissionSpecificity(scopePolicy, resource, "secret.read_value")
	exactSpecificity := rolePermissionSpecificity(exactPolicy, resource, "secret.read_value")
	if !exactSpecificity.betterThan(scopeSpecificity) {
		t.Fatal("expected exact named resource selector to outrank scope-only selector")
	}
}

func TestPrefixTeamResourcesIncludesContainingTeamWhenRequested(t *testing.T) {
	got := prefixTeamResources([]string{"team-1", "dev"}, true)
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1/dev"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixTeamResources(includeSelf=true) = %#v, want %#v", got, want)
	}
}

func TestScopeTeamAncestorsUsesScopePathAsContainingTeam(t *testing.T) {
	got := scopeTeamAncestors("team-1/dev")
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1/dev"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopeTeamAncestors() = %#v, want %#v", got, want)
	}
}

func TestScopeTeamAncestorsUsesGeneralTeamForDefaultScope(t *testing.T) {
	got := scopeTeamAncestors("")
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: model.TeamGeneralID}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopeTeamAncestors(default) = %#v, want %#v", got, want)
	}
}

func TestRepositoryIDTeamAncestorsUsesRepositoryPathPrefix(t *testing.T) {
	got := repositoryIDTeamAncestors("team-1/dev/app")
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1/dev"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("repositoryIDTeamAncestors() = %#v, want %#v", got, want)
	}
}

func TestRepositoryIDTeamAncestorsUsesGeneralForRootRepository(t *testing.T) {
	got := repositoryIDTeamAncestors("app")
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: model.TeamGeneralID}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("repositoryIDTeamAncestors(root) = %#v, want %#v", got, want)
	}
}

func TestRepositoryResourceIDRequiresRepositoryName(t *testing.T) {
	if got := repositoryResourceID("nopsai", "test-app"); got != "nopsai/test-app" {
		t.Fatalf("repositoryResourceID() = %q, want nopsai/test-app", got)
	}
	if got := repositoryResourceID("", "test-app"); got != "test-app" {
		t.Fatalf("repositoryResourceID(root) = %q, want test-app", got)
	}
	if got := repositoryResourceID("nopsai", ""); got != "" {
		t.Fatalf("repositoryResourceID(missing name) = %q, want empty", got)
	}
}

func TestAppendInheritedResourcesDedupesResources(t *testing.T) {
	team := model.InheritedResource{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"}
	got := appendInheritedResources([]model.InheritedResource{team}, []model.InheritedResource{team})
	if len(got) != 1 {
		t.Fatalf("appendInheritedResources() len = %d, want 1", len(got))
	}
}

func TestTeamSelfAndParentTeamAncestorsIncludesRepositoryLeaf(t *testing.T) {
	got := teamSelfAndParentTeamAncestors("nopsai/test-app", []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	})
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1/nopsai/test-app"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("teamSelfAndParentTeamAncestors() = %#v, want %#v", got, want)
	}
}

func TestTeamSelfAndParentTeamAncestorsIncludesAppLeaf(t *testing.T) {
	got := teamSelfAndParentTeamAncestors("test-app", []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	})
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1/test-app"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("teamSelfAndParentTeamAncestors() = %#v, want %#v", got, want)
	}
}

func TestPrefixTeamAncestorsExcludesCurrentTeam(t *testing.T) {
	got := prefixTeamAncestors([]string{"team-1", "dev"})
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "team", ID: "team-1"}, Reason: "team_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixTeamAncestors() = %#v, want %#v", got, want)
	}
}
