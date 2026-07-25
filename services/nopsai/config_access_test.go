package nopsai

import (
	"database/sql"
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
    resource: team:team-1
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

func TestParseAccessSyncPlanRejectsAuthTeamDefinitions(t *testing.T) {
	files := map[string]string{
		"access/teams.yaml": `
teams:
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
		t.Fatal("expected access manifest auth teams to be rejected")
	}
}

func TestParseAccessSyncPlanSupportsAuthTeamSubjects(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - team: team-1-developers
    role: developer
    resource: team:team-1
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
		subjectType:  model.SubjectTypeAuthTeam,
		subjectID:    "team-1-developers",
		resourceType: grantResourceTeam,
		resourceID:   "team-1",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected auth team grant key %#v, got %#v", key, plan.grants)
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
    resource: team:team-1
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
    resource: team:team-1
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

func TestAccessGrantConfigWritableAdoptsOrphanManagedGrantInScope(t *testing.T) {
	binding := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	resource := accessGrantResource{Type: grantResourceKnowledgeContext, ID: "guardrail/team-1/repo-check"}
	writable, err := accessGrantConfigWritableDecision(nil, nil, binding, configSyncGrantResourceScope(resource), "repository:nopsai/test-app knowledge_context:guardrail/team-1/repo-check", sql.NullInt64{}, true)
	if err != nil {
		t.Fatalf("accessGrantConfigWritableDecision() error = %v", err)
	}
	if !writable {
		t.Fatal("team repo should adopt orphaned managed access grant in its scope")
	}
}

func TestAccessGrantConfigWritableRejectsOrphanManagedGrantOutsideScope(t *testing.T) {
	binding := models.ConfigRepository{ID: 2, ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	resource := accessGrantResource{Type: grantResourceKnowledgeContext, ID: "guardrail/team-2/repo-check"}
	writable, err := accessGrantConfigWritableDecision(nil, nil, binding, configSyncGrantResourceScope(resource), "repository:nopsai/test-app knowledge_context:guardrail/team-2/repo-check", sql.NullInt64{}, true)
	if err == nil {
		t.Fatal("expected out-of-scope orphaned managed access grant to fail")
	}
	if writable {
		t.Fatal("out-of-scope orphaned managed access grant should not be writable")
	}
	if !strings.Contains(err.Error(), "unknown config repository") {
		t.Fatalf("error = %v, want unknown owner message", err)
	}
}

func TestConfigSyncGrantResourceScopeUsesKnowledgeContextTeam(t *testing.T) {
	got := configSyncGrantResourceScope(accessGrantResource{
		Type: grantResourceKnowledgeContext,
		ID:   "guardrail/team-1/repo-check",
	})
	if got != "team-1" {
		t.Fatalf("configSyncGrantResourceScope() = %q, want team-1", got)
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

func TestParseAccessSyncPlanTeamRepoNormalizesScopedTeamGrant(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: developer
    resource_type: team
    resource_id: dev
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceTeam,
		resourceID:   "team-1/dev",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected normalized grant key %#v, got %#v", key, plan.grants)
	}
}

func TestParseAccessSyncPlanTeamRepoDefaultsTeamGrantToBoundTeam(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: developer
    resource_type: team
`,
	}

	plan, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("parseAccessSyncPlan() error = %v", err)
	}

	key := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceTeam,
		resourceID:   "team-1",
	}
	if _, ok := plan.grants[key]; !ok {
		t.Fatalf("expected bound-team grant key %#v, got %#v", key, plan.grants)
	}
}

func TestParseAccessSyncPlanTeamRepoRejectsGlobalIAM(t *testing.T) {
	files := map[string]string{
		"access/roles.yaml": `
advanced_roles:
  - name: release-manager
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected team-scoped repo to reject global role management")
	}
}

func TestParseAccessSyncPlanTeamRepoRejectsServiceAccounts(t *testing.T) {
	files := map[string]string{
		"access/service-accounts.yaml": `
service_accounts:
  - sub: webhook-deployer
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected team-scoped repo to reject service account management")
	}
}

func TestParseAccessSyncPlanTeamRepoRejectsPlatformGrant(t *testing.T) {
	files := map[string]string{
		"access/grants.yaml": `
basic_roles:
  - user: bob
    role: admin
    resource: platform:default
`,
	}

	_, err := parseAccessSyncPlan(files, "access", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err == nil {
		t.Fatal("expected team-scoped repo to reject platform admin grant")
	}
}

func TestParseAccessSyncPlanTeamRepoNormalizesRepositoryGrantIntoScope(t *testing.T) {
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
		ScopeType: models.ConfigRepositoryScopeTeam,
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
		t.Fatalf("expected repository path to be normalized into team scope, got %#v", plan.grants)
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
      - subject_type: team
        subject_id: data-team
      - repository: nopsai/test-app
`, "pipelines/deploy.yaml", grantResourcePipeline, "team-1/deploy", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
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

	teamKey := accessGrantPlanKey{
		subjectType:  grantSubjectTeam,
		subjectID:    "data-team",
		resourceType: grantResourcePipeline,
		resourceID:   "team-1/deploy",
	}
	teamGrant, ok := plan.grants[teamKey]
	if !ok {
		t.Fatalf("expected team use grant key %#v, got %#v", teamKey, plan.grants)
	}
	if teamGrant.role != customUseGrantRole || len(teamGrant.actions) != 1 || teamGrant.actions[0] != "pipeline.use" {
		t.Fatalf("team grant = %#v, want pipeline use grant", teamGrant)
	}

	repoKey := accessGrantPlanKey{
		subjectType:  model.SubjectTypeRepository,
		subjectID:    "nopsai/test-app",
		resourceType: grantResourcePipeline,
		resourceID:   "team-1/deploy",
	}
	if _, ok := plan.grants[repoKey]; !ok {
		t.Fatalf("expected repository use grant key %#v, got %#v", repoKey, plan.grants)
	}
}

func TestEmbeddedDashboardAccessAddsVisibilityAndReadGrant(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
title: Ops Dashboard
access:
  visibility: workspace
  use_access:
    grants:
      - team: data-team
`, "dashboards/team-1/ops-dashboard.yaml", grantResourceDashboard, "team-1/ops-dashboard", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
		ScopeID:   "team-1",
	}, "team-1")
	if err != nil {
		t.Fatalf("addEmbeddedResourceAccess(dashboard) error = %v", err)
	}

	accessKey := resourceAccessPlanKey{resourceType: grantResourceDashboard, resourceID: "team-1/ops-dashboard"}
	access, ok := plan.resourceAccess[accessKey]
	if !ok {
		t.Fatalf("expected dashboard resource access key %#v, got %#v", accessKey, plan.resourceAccess)
	}
	if !access.visibilitySet || access.visibility != resourceVisibilityWorkspace {
		t.Fatalf("dashboard visibility = (%v, %q), want workspace", access.visibilitySet, access.visibility)
	}

	grantKey := accessGrantPlanKey{
		subjectType:  grantSubjectTeam,
		subjectID:    "data-team",
		resourceType: grantResourceDashboard,
		resourceID:   "team-1/ops-dashboard",
	}
	grant, ok := plan.grants[grantKey]
	if !ok {
		t.Fatalf("expected dashboard read grant key %#v, got %#v", grantKey, plan.grants)
	}
	if grant.role != customUseGrantRole || len(grant.actions) != 1 || grant.actions[0] != "dashboard.read" {
		t.Fatalf("dashboard grant = %#v, want dashboard read grant", grant)
	}
}

func TestEmbeddedAccessDefaultsToRestrictedWhenGrantsArePresent(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
name: checkout
script: git status
access:
  teams:
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

func TestEmbeddedAccessRejectsUnknownGrantKey(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
name: deploy
access:
  use_access:
    grants:
      - teem: data-team
`, "pipelines/deploy.yaml", grantResourcePipeline, "deploy", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, "")
	if err == nil {
		t.Fatal("expected unknown access grant key to be rejected")
	}
	if !strings.Contains(err.Error(), `unsupported resource access grant key "teem"`) {
		t.Fatalf("error = %v, want unsupported grant key", err)
	}
}

func TestEmbeddedScopeAccessRejectsWorkspaceVisibility(t *testing.T) {
	plan := newAccessSyncPlan()
	err := plan.addEmbeddedResourceAccess(`
access:
  visibility: public
`, "scopes/prod/scope.yaml", grantResourceScope, "team-1/prod", models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
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
		resourceType: grantResourceTeam,
		resourceID:   "team-1",
	}
	childKey := accessGrantPlanKey{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "bob",
		resourceType: grantResourceTeam,
		resourceID:   "team-1/dev",
	}
	plan.grants[parentKey] = storedAccessGrant{resourceType: grantResourceTeam, resourceID: "team-1"}
	plan.grants[childKey] = storedAccessGrant{resourceType: grantResourceTeam, resourceID: "team-1/dev"}

	filterDelegatedAccessResources(plan, models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeTeam,
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
		ScopeType: models.ConfigRepositoryScopeTeam,
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
		resourceType: grantResourceTeam,
		resourceID:   "team-1",
	}
	plan.grants[key] = storedAccessGrant{resourceType: grantResourceTeam, resourceID: "team-1"}

	filterDelegatedAccessResources(plan, models.ConfigRepository{
		ScopeType: models.ConfigRepositoryScopeSystem,
		ScopeID:   models.ConfigRepositorySystemGlobalID,
	}, []string{"team-1"})

	if _, ok := plan.grants[key]; !ok {
		t.Fatal("system access grant should remain even when the target team has a delegated config repo")
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
		subjectID:    "nopsai/test-app",
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
		subjectID:    "nopsai/test-app",
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
	if !accessGrantResourceInConfigBindingScope(grantResourceCredential, "system/llm/openai", systemBinding) {
		t.Fatal("system config repo should cover credential access grants")
	}

	teamBinding := models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"}
	if !accessGrantResourceInConfigBindingScope(grantResourceTeam, "team-1/dev", teamBinding) {
		t.Fatal("team config repo should cover access grants in its team subtree")
	}
	if accessGrantResourceInConfigBindingScope(grantResourceTeam, "team-2", teamBinding) {
		t.Fatal("team config repo should not cover access grants outside its team subtree")
	}
	if !accessGrantResourceInConfigBindingScope(grantResourceCredential, "team-1/llm/openai", teamBinding) {
		t.Fatal("team config repo should cover credential access grants in its team subtree")
	}
	if accessGrantResourceInConfigBindingScope(grantResourceCredential, "team-2/llm/openai", teamBinding) {
		t.Fatal("team config repo should not cover credential access grants outside its team subtree")
	}
}

func TestResourceTypeForUseActionsInfersCredential(t *testing.T) {
	if got := resourceTypeForUseActions([]string{"credential.use"}); got != grantResourceCredential {
		t.Fatalf("resourceTypeForUseActions() = %q, want %q", got, grantResourceCredential)
	}
}
