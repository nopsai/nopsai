package nopsai

import (
	"reflect"
	"testing"
)

func TestRepositoryTriggerOverrideKeysPreferTeamedRepository(t *testing.T) {
	specific, ownerWide := repositoryTriggerOverrideKeys("nopsai", "test-app", []string{
		"team-1/nopsai/test-app",
	})

	wantSpecific := []string{"team-1/nopsai/test-app", "nopsai/test-app"}
	if !reflect.DeepEqual(specific, wantSpecific) {
		t.Fatalf("specific keys = %#v, want %#v", specific, wantSpecific)
	}
	wantOwnerWide := []string{"team-1/nopsai/all", "nopsai/all"}
	if !reflect.DeepEqual(ownerWide, wantOwnerWide) {
		t.Fatalf("owner-wide keys = %#v, want %#v", ownerWide, wantOwnerWide)
	}
}

func TestRepositoryTriggerOverrideKeysDeduplicateRootRepository(t *testing.T) {
	specific, ownerWide := repositoryTriggerOverrideKeys("owner", "repo", []string{"owner/repo"})

	wantSpecific := []string{"owner/repo"}
	if !reflect.DeepEqual(specific, wantSpecific) {
		t.Fatalf("specific keys = %#v, want %#v", specific, wantSpecific)
	}
	wantOwnerWide := []string{"owner/all"}
	if !reflect.DeepEqual(ownerWide, wantOwnerWide) {
		t.Fatalf("owner-wide keys = %#v, want %#v", ownerWide, wantOwnerWide)
	}
}

func TestRepositoryTriggerOverrideKeysUseExplicitAppParent(t *testing.T) {
	specific, ownerWide := repositoryTriggerOverrideKeys("owner", "repo", []string{"platform/service-api"})

	wantSpecific := []string{"platform/owner/repo", "owner/repo"}
	if !reflect.DeepEqual(specific, wantSpecific) {
		t.Fatalf("specific keys = %#v, want %#v", specific, wantSpecific)
	}
	wantOwnerWide := []string{"platform/owner/all", "owner/all"}
	if !reflect.DeepEqual(ownerWide, wantOwnerWide) {
		t.Fatalf("owner-wide keys = %#v, want %#v", ownerWide, wantOwnerWide)
	}
}
