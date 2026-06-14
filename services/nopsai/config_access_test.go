package nopsai

import (
	"strings"
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

func TestParseAccessSyncPlanGlobalServiceAccounts(t *testing.T) {
	files := map[string]string{
		"access/service-accounts.yaml": `
service_accounts:
  - sub: webhook-deployer
    email: webhook-deployer@example.com
    advanced_roles:
      - webhook-runner

advanced_roles:
  - name: webhook-runner
    policies:
      - resource: trigger:acme/deploy-webhook
        action: trigger.read
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	if account, ok := plan.serviceAccounts["webhook-deployer"]; !ok {
		t.Fatal("expected webhook-deployer service account")
	} else if account.email != "webhook-deployer@example.com" || account.status != "active" {
		t.Fatalf("service account = %#v, want normalized email and active status", account)
	}
	key := accessRoleBindingKey{
		role:        "webhook-runner",
		subjectType: model.SubjectTypeServiceAccount,
		subjectID:   "webhook-deployer",
	}
	if _, ok := plan.roleBindings[key]; !ok {
		t.Fatalf("expected service account role binding %#v, got %#v", key, plan.roleBindings)
	}
}

func TestParseAccessSyncPlanSupportsServiceAccountGrantShortcut(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - service_account: webhook-deployer
    role: developer
    resource: trigger:acme/deploy-webhook
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeServiceAccount,
		subjectID:    "webhook-deployer",
		resourceType: grantResourceTrigger,
		resourceID:   "acme/deploy-webhook",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected service account grant key %#v, got %#v", key, plan.grants)
	}
}

func TestParseAccessSyncPlanSupportsCanonicalServiceAccountGrant(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - subject_type: service_account
    subject_id: webhook-deployer
    role: developer
    resource: trigger:acme/deploy-webhook
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeServiceAccount,
		subjectID:    "webhook-deployer",
		resourceType: grantResourceTrigger,
		resourceID:   "acme/deploy-webhook",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected service account grant key %#v, got %#v", key, plan.grants)
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

func TestParseAccessSyncPlanSupportsAuthGroupSubjects(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - group: team-1-developers
    role: developer
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

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeAuthGroup,
		subjectID:    "team-1-developers",
		resourceType: grantResourceFolder,
		resourceID:   "team-1",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected auth group grant key %#v, got %#v", key, plan.grants)
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

func TestParseAccessSyncPlanRejectsSSOManagedUsers(t *testing.T) {
	tests := map[string]string{
		"user sub": `
users:
  - sub: oidc:nopsai:alice
`,
		"user provider": `
users:
  - sub: alice
    provider: oidc:nopsai
`,
		"basic role user": `
basic_roles:
  - user: oidc:nopsai:alice
    role: viewer
    resource: folder:team-1
`,
		"advanced role binding user": `
advanced_role_bindings:
  - role: release-manager
    subject_type: user
    subject_id: oidc:nopsai:alice
`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseAccessSyncPlan(map[string]string{"access/access.yaml": content}, "access", models.ConfigRepository{
				ScopeType: models.ConfigRepositoryScopeSystem,
				ScopeID:   models.ConfigRepositorySystemGlobalID,
			}, "")
			if err == nil {
				t.Fatal("expected SSO-managed user to be rejected")
			}
			if !strings.Contains(err.Error(), "SSO-managed user") {
				t.Fatalf("error = %v, want SSO-managed user rejection", err)
			}
		})
	}
}

func TestNormalizeEmbeddedResourceUseGrantRejectsSSOManagedUser(t *testing.T) {
	_, _, err := normalizeEmbeddedResourceUseGrantSubject(embeddedResourceUseGrantFile{
		User: "oidc:nopsai:alice",
	})
	if err == nil {
		t.Fatal("expected SSO-managed resource access user to be rejected")
	}
	if !strings.Contains(err.Error(), "SSO-managed user") {
		t.Fatalf("error = %v, want SSO-managed user rejection", err)
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

func TestParseAccessSyncPlanGroupRepoRejectsServiceAccounts(t *testing.T) {
	files := map[string]string{
		"access/service-accounts.yaml": `
service_accounts:
  - sub: webhook-deployer
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected group-scoped repo to reject service account management")
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

func TestEmbeddedPipelineAccessAddsVisibilityAndUseGrants(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
name: deploy
steps:
  - name: ship
    script: echo ship
access:
  visibility: restricted
  use_access:
    grants:
      - subject_type: group
        subject_id: data-team
      - repository: hosein-yousefii/test-app
`, "pipelines/deploy.yaml", grantResourcePipeline, "team-1/deploy", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("addEmbeddedResourceAccess() error = %v", err)
	}

	accessKey := resourceAccessPlanKey{resourceType: grantResourcePipeline, resourceID: "team-1/deploy"}
	access, ok := plan.resourceAccess[accessKey]
	if !ok {
		t.Fatalf("expected resource access key %#v, got %#v", accessKey, plan.resourceAccess)
	}
	if !access.visibilitySet || access.visibility != resourceVisibilityRestricted {
		t.Fatalf("visibility = (%v, %q), want restricted", access.visibilitySet, access.visibility)
	}

	groupKey := accessGrantPlanKey{
		subjectType:  grantSubjectGroup,
		subjectID:    "data-team",
		resourceType: grantResourcePipeline,
		resourceID:   "team-1/deploy",
	}
	groupGrant, ok := plan.grants[groupKey]
	if !ok {
		t.Fatalf("expected group use grant key %#v, got %#v", groupKey, plan.grants)
	}
	if groupGrant.role != customUseGrantRole || len(groupGrant.actions) != 1 || groupGrant.actions[0] != "pipeline.use" {
		t.Fatalf("group grant = %#v, want pipeline use grant", groupGrant)
	}

	repoKey := accessGrantPlanKey{
		subjectType:  model.SubjectTypeRepository,
		subjectID:    "hosein-yousefii/test-app",
		resourceType: grantResourcePipeline,
		resourceID:   "team-1/deploy",
	}
	if _, ok := plan.grants[repoKey]; !ok {
		t.Fatalf("expected repository use grant key %#v, got %#v", repoKey, plan.grants)
	}
}

func TestEmbeddedAccessDefaultsToRestrictedWhenGrantsArePresent(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
name: checkout
script: git status
access:
  groups:
    - data-team
`, "steps/shared/checkout.yaml", grantResourceStep, "shared/checkout", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err != nil {
		t.Fatalf("addEmbeddedResourceAccess() error = %v", err)
	}

	access := plan.resourceAccess[resourceAccessPlanKey{resourceType: grantResourceStep, resourceID: "shared/checkout"}]
	if access.visibility != resourceVisibilityRestricted {
		t.Fatalf("visibility = %q, want restricted", access.visibility)
	}
}

func TestEmbeddedScopeAccessRejectsWorkspaceVisibility(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
access:
  visibility: public
`, "scopes/prod/scope.yaml", grantResourceScope, "team-1/prod", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected scope public visibility to be rejected")
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

func TestFilterDelegatedAccessResourcesRemovesChildResourceAccess(t *testing.T) {
	plan := newAccessSyncPlan()
	parentKey := resourceAccessPlanKey{resourceType: grantResourcePipeline, resourceID: "team-1/build"}
	childKey := resourceAccessPlanKey{resourceType: grantResourcePipeline, resourceID: "team-1/dev/deploy"}
	plan.resourceAccess[parentKey] = storedResourceAccess{resourceType: grantResourcePipeline, resourceID: "team-1/build", visibility: resourceVisibilityWorkspace, visibilitySet: true}
	plan.resourceAccess[childKey] = storedResourceAccess{resourceType: grantResourcePipeline, resourceID: "team-1/dev/deploy", visibility: resourceVisibilityWorkspace, visibilitySet: true}

	filterDelegatedAccessResources(plan, models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeFolder,
		ScopeID:   "team-1",
	}, []string{"team-1/dev"})

	if _, ok := plan.resourceAccess[parentKey]; !ok {
		t.Fatal("parent resource access should remain")
	}
	if _, ok := plan.resourceAccess[childKey]; ok {
		t.Fatal("child delegated resource access should be filtered")
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

func TestFilterDelegatedAccessResourcesRemovesSystemEmbeddedKnowledgeAccess(t *testing.T) {
	plan := newAccessSyncPlan()
	resourceKey := resourceAccessPlanKey{
		resourceType: grantResourceKnowledgeContext,
		resourceID:   "guardrail/team-1/repo-check",
	}
	grantKey := accessGrantPlanKey{
		subjectType:  model.SubjectTypeRepository,
		subjectID:    "hosein-yousefii/test-app",
		resourceType: grantResourceKnowledgeContext,
		resourceID:   "guardrail/team-1/repo-check",
	}
	plan.resourceAccess[resourceKey] = storedResourceAccess{
		resourceType:  grantResourceKnowledgeContext,
		resourceID:    "guardrail/team-1/repo-check",
		visibility:    resourceVisibilityRestricted,
		visibilitySet: true,
		sourcePath:    "knowledge/guardrail/team-1/repo-check.yaml",
	}
	plan.grants[grantKey] = storedAccessGrant{
		subjectType:  model.SubjectTypeRepository,
		subjectID:    "hosein-yousefii/test-app",
		role:         customUseGrantRole,
		resourceType: grantResourceKnowledgeContext,
		resourceID:   "guardrail/team-1/repo-check",
		sourcePath:   "knowledge/guardrail/team-1/repo-check.yaml",
	}

	filterDelegatedAccessResources(plan, models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, []string{"team-1"})

	if _, ok := plan.resourceAccess[resourceKey]; ok {
		t.Fatal("delegated knowledge resource access should be filtered from system sync")
	}
	if _, ok := plan.grants[grantKey]; ok {
		t.Fatal("delegated knowledge use grant should be filtered from system sync")
	}
}

func TestAccessGrantResourceInConfigBindingScope(t *testing.T) {
	systemBinding := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID}
	if !accessGrantResourceInConfigBindingScope(grantResourcePlatform, platformGrantID, systemBinding) {
		t.Fatal("system config repo should cover platform access grants")
	}

	groupBinding := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeFolder, ScopeID: "team-1"}
	if !accessGrantResourceInConfigBindingScope(grantResourceFolder, "team-1/dev", groupBinding) {
		t.Fatal("group config repo should cover access grants in its folder subtree")
	}
	if accessGrantResourceInConfigBindingScope(grantResourceFolder, "team-2", groupBinding) {
		t.Fatal("group config repo should not cover access grants outside its folder subtree")
	}
}
