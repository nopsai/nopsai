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

func TestPrefixFolderResourcesIncludesContainingFolderWhenRequested(t *testing.T) {
	got := prefixFolderResources([]string{"team-1", "dev"}, true)
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "folder", ID: "team-1/dev"}, Reason: "folder_inheritance"},
		{Resource: model.ResourceRef{Type: "folder", ID: "team-1"}, Reason: "folder_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixFolderResources(includeSelf=true) = %#v, want %#v", got, want)
	}
}

func TestPrefixFolderAncestorsExcludesCurrentFolder(t *testing.T) {
	got := prefixFolderAncestors([]string{"team-1", "dev"})
	want := []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "folder", ID: "team-1"}, Reason: "folder_inheritance"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixFolderAncestors() = %#v, want %#v", got, want)
	}
}
