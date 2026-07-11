package nopsai

import (
	"reflect"
	"testing"
)

func TestRepositoryTriggerOverrideKeysPreferTeamedRepository(t *testing.T) {
	specific, ownerWide := repositoryTriggerOverrideKeys("hosein-yousefii", "test-app", []string{
		"team-1/hosein-yousefii/test-app",
	})

	wantSpecific := []string{"team-1/hosein-yousefii/test-app", "hosein-yousefii/test-app"}
	if !reflect.DeepEqual(specific, wantSpecific) {
		t.Fatalf("specific keys = %#v, want %#v", specific, wantSpecific)
	}
	wantOwnerWide := []string{"team-1/hosein-yousefii/all", "hosein-yousefii/all"}
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
