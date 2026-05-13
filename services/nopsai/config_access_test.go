package main

import (
	"testing"

	"nopsai/pkg/models"
	"nopsai/services/aaa/pkg/model"
)

func TestParseAccessSyncPlanGlobalManifest(t *testing.T) {
	files := map[string]string{
		"access/users.yaml": `
users:
  - sub: alice
    email: alice@example.com
    advanced_roles:
      - release-manager
  - sub: bob
    email: bob@example.com
    advanced_roles:
      - viewer
`,
		"access/advanced.yaml": `
advanced_roles:
  - name: release-manager
    policies:
      - resource: pipeline:team-1/*
        action: pipeline.execute
`,
		"access/basic.yaml": `
basic_roles:
  - user: alice
    role: owner
    resource: folder:team-1
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	if _, ok := plan.users["alice"]; !ok {
		t.Fatal("expected alice user")
	}
	if _, ok := plan.users["bob"]; !ok {
		t.Fatal("expected bob user")
	}
	if _, ok := plan.roles["release-manager"]; !ok {
		t.Fatal("expected release-manager role")
	}
	if len(plan.policies) != 1 {
		t.Fatalf("policies = %d, want 1", len(plan.policies))
	}
	if len(plan.roleBindings) != 2 {
		t.Fatalf("role bindings = %d, want user roles from users[].advanced_roles", len(plan.roleBindings))
	}
	if len(plan.grants) != 1 {
		t.Fatalf("grants = %d, want 1", len(plan.grants))
	}
}

func TestParseAccessSyncPlanRejectsAuthGroupDefinitions(t *testing.T) {
	files := map[string]string{
		"access/groups.yaml": `
groups:
  - name: team-1-developers
    members:
      - user: bob
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err == nil {
		t.Fatal("expected access manifest auth groups to be rejected")
	}
}

func TestParseAccessSyncPlanRejectsAuthGroupSubjects(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - group: team-1-developers
    role: developer
    resource: folder:team-1
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err == nil {
		t.Fatal("expected auth group grant subjects to be rejected")
	}
}

func TestParseAccessSyncPlanRejectsAmbiguousRoleKeys(t *testing.T) {
	tests := map[string]string{
		"top-level roles": `
roles:
  - name: release-manager
`,
		"top-level grants": `
grants:
  - user: alice
    role: owner
    resource: folder:team-1
`,
		"user roles": `
users:
  - sub: alice
    roles: [release-manager]
`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseAccessSyncPlan(map[string]string{"access/access.yaml": content}, "access", models.ConfigRepository{
				ScopeType: models.ConfigRepositoryScopeSystem,
				ScopeID:   models.ConfigRepositorySystemGlobalID,
			}, "")
			if err == nil {
				t.Fatal("expected ambiguous role key to be rejected")
			}
		})
	}
}

func TestParseAccessSyncPlanGroupRepoNormalizesScopedFolderGrant(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: developer
    resource_type: folder
    resource_id: dev
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceFolder,
		resourceID:   "team-1/dev",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected normalized grant key %#v, got %#v", key, plan.grants)
	}
}

func TestParseAccessSyncPlanGroupRepoDefaultsFolderGrantToBoundGroup(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: developer
    resource_type: folder
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceFolder,
		resourceID:   "team-1",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected bound-group grant key %#v, got %#v", key, plan.grants)
	}
}

func TestParseAccessSyncPlanGroupRepoRejectsGlobalIAM(t *testing.T) {
	files := map[string]string{
		"access/roles.yaml": `
advanced_roles:
  - name: release-manager
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected group-scoped repo to reject global role management")
	}
}

func TestParseAccessSyncPlanGroupRepoRejectsPlatformGrant(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: admin
    resource: platform:default
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected group-scoped repo to reject platform admin grant")
	}
}

func TestParseAccessSyncPlanGroupRepoNormalizesRepositoryGrantIntoScope(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: viewer
    resource_type: repository
    resource_id: other-team/service
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceRepo,
		resourceID:   "team-1/other-team/service",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected repository path to be normalized into group scope, got %#v", plan.grants)
	}
}

func TestFilterDelegatedAccessResourcesRemovesChildScopedGrant(t *testing.T) {
	plan := newAccessSyncPlan()
	parentKey := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "alice",
		resourceType: grantResourceFolder,
		resourceID:   "team-1",
	}
	childKey := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceFolder,
		resourceID:   "team-1/dev",
	}
	plan.grants[parentKey] = storedAccessGrant{resourceType: grantResourceFolder, resourceID: "team-1"}
	plan.grants[childKey] = storedAccessGrant{resourceType: grantResourceFolder, resourceID: "team-1/dev"}

	filterDelegatedAccessResources(plan, models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, []string{"team-1/dev"})

	if _, ok := plan.grants[parentKey]; !ok {
		t.Fatal("parent-scope grant should remain")
	}
	if _, ok := plan.grants[childKey]; ok {
		t.Fatal("child delegated grant should be filtered")
	}
}

func TestFilterDelegatedAccessResourcesKeepsSystemGrant(t *testing.T) {
	plan := newAccessSyncPlan()
	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "alice",
		resourceType: grantResourceFolder,
		resourceID:   "team-1",
	}
	plan.grants[key] = storedAccessGrant{resourceType: grantResourceFolder, resourceID: "team-1"}

	filterDelegatedAccessResources(plan, models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, []string{"team-1"})

	if _, ok := plan.grants[key]; !ok {
		t.Fatal("system access grant should remain even when the target folder has a delegated config repo")
	}
}
