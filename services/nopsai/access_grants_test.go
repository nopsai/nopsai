package nopsai

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	aaaauthz "nopsai/services/aaa/pkg/authz"
	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

type fakeGrantRolePolicy struct {
	roleName     string
	resourceType string
	resourceID   string
	action       string
	effect       string
}

type fakeGrantACLPolicy struct {
	subjectType  string
	subjectID    string
	resourceType string
	resourceID   string
	action       string
	effect       string
	roleName     string
}

type fakeGrantBackend struct {
	resolved     *model.ResolvedSubject
	rolePolicies []fakeGrantRolePolicy
	aclPolicies  []fakeGrantACLPolicy
	inheritance  map[string][]model.InheritedResource
	logs         []model.DecisionLogEntry
}

type recordingExecRunner struct {
	statements []string
}

type fakeQueryRunner struct {
	row     pgx.Row
	queries []string
	args    [][]any
}

type fakeScanRow struct {
	values []string
	err    error
}

type fakeScanAnyRow struct {
	values []any
	err    error
}

func (r *recordingExecRunner) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	r.statements = append(r.statements, sql)
	return pgconn.NewCommandTag("DELETE 1"), nil
}

func (r *fakeQueryRunner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag(""), fmt.Errorf("unexpected exec")
}

func (r *fakeQueryRunner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected query")
}

func (r *fakeQueryRunner) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	r.queries = append(r.queries, sql)
	r.args = append(r.args, args)
	return r.row
}

func (r fakeScanRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for idx, value := range r.values {
		if idx >= len(dest) {
			break
		}
		ptr, ok := dest[idx].(*string)
		if !ok {
			return fmt.Errorf("unsupported scan destination %T", dest[idx])
		}
		*ptr = value
	}
	return nil
}

func (r fakeScanAnyRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for idx, value := range r.values {
		if idx >= len(dest) {
			break
		}
		switch ptr := dest[idx].(type) {
		case *int:
			typed, ok := value.(int)
			if !ok {
				return fmt.Errorf("unsupported int scan value %T", value)
			}
			*ptr = typed
		case *string:
			typed, ok := value.(string)
			if !ok {
				return fmt.Errorf("unsupported string scan value %T", value)
			}
			*ptr = typed
		default:
			return fmt.Errorf("unsupported scan destination %T", dest[idx])
		}
	}
	return nil
}

func (f *fakeGrantBackend) ResolveSubject(context.Context, model.Subject) (*model.ResolvedSubject, error) {
	return f.resolved, nil
}

func (f *fakeGrantBackend) FindRolePermissionMatch(_ context.Context, roleNames []string, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	for _, policy := range f.rolePolicies {
		if !containsStringValue(roleNames, policy.roleName) {
			continue
		}
		if !grantMatches(policy.effect, effect) || !grantMatches(policy.resourceType, resource.Type) || !grantMatches(policy.resourceID, resource.ID) || !grantMatches(policy.action, action) {
			continue
		}
		return &model.MatchedPolicy{
			Source:       "role_permission",
			RoleName:     policy.roleName,
			ResourceType: policy.resourceType,
			ResourceID:   policy.resourceID,
			Action:       policy.action,
			Effect:       policy.effect,
		}, nil
	}
	return nil, nil
}

func (f *fakeGrantBackend) FindACLMatch(_ context.Context, subject model.SubjectRef, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	for _, policy := range f.aclPolicies {
		if policy.subjectType != subject.Type || policy.subjectID != subject.ID {
			continue
		}
		if !grantMatches(policy.effect, effect) || !grantMatches(policy.resourceType, resource.Type) || !grantMatches(policy.resourceID, resource.ID) || !grantMatches(policy.action, action) {
			continue
		}
		return &model.MatchedPolicy{
			Source:       "resource_acl",
			RoleName:     policy.roleName,
			SubjectType:  policy.subjectType,
			SubjectID:    policy.subjectID,
			ResourceType: policy.resourceType,
			ResourceID:   policy.resourceID,
			Action:       policy.action,
			Effect:       policy.effect,
		}, nil
	}
	return nil, nil
}

func (f *fakeGrantBackend) ResolveResourceInheritance(_ context.Context, resource model.ResourceRef) ([]model.InheritedResource, error) {
	if inheritance, ok := f.inheritance[grantResourceKey(resource)]; ok {
		return inheritance, nil
	}
	return nil, nil
}

func (f *fakeGrantBackend) WriteDecisionLog(_ context.Context, entry model.DecisionLogEntry) error {
	f.logs = append(f.logs, entry)
	return nil
}

func (f *fakeGrantBackend) RecordAudit(_ context.Context, entry model.DecisionLogEntry) error {
	f.logs = append(f.logs, entry)
	return nil
}

func TestProductRolePermissions(t *testing.T) {
	t.Run("viewer", func(t *testing.T) {
		actions := actionSet(applicableProductRoleActions(productRoleViewer, grantResourceTeam))
		assertAction(t, actions, "pipeline.read", true)
		assertAction(t, actions, "pipeline_schedule.list", true)
		assertAction(t, actions, "pipeline_schedule.read", true)
		assertAction(t, actions, "trigger.read", true)
		assertAction(t, actions, "external_trigger.read", true)
		assertAction(t, actions, "git_webhook_source.read", true)
		assertAction(t, actions, "git_webhook_source.update", false)
		assertAction(t, actions, "pipeline_schedule.execute", false)
		assertAction(t, actions, "pipeline.update", false)
		assertAction(t, actions, "pipeline.execute", false)
		assertAction(t, actions, "pipeline.use", false)
		assertAction(t, actions, "scope.use", false)
		assertAction(t, actions, "config_repo.read", true)
		assertAction(t, actions, "config_repo.manage", false)
		assertAction(t, actions, "secret.read_value", false)
	})

	t.Run("developer", func(t *testing.T) {
		actions := actionSet(applicableProductRoleActions(productRoleDeveloper, grantResourceTeam))
		assertAction(t, actions, "pipeline.read", true)
		assertAction(t, actions, "pipeline_schedule.create", true)
		assertAction(t, actions, "pipeline_schedule.update", true)
		assertAction(t, actions, "pipeline_schedule.execute", true)
		assertAction(t, actions, "git_webhook_source.create", true)
		assertAction(t, actions, "git_webhook_source.update", true)
		assertAction(t, actions, "pipeline.update", true)
		assertAction(t, actions, "pipeline.execute", true)
		assertAction(t, actions, "pipeline.use", true)
		assertAction(t, actions, "scope.use", true)
		assertAction(t, actions, "secret.use", true)
		assertAction(t, actions, "variable.use", true)
		assertAction(t, actions, "secret.write_value", true)
		assertAction(t, actions, "step.create", true)
		assertAction(t, actions, "step.update", true)
		assertAction(t, actions, "step.use", true)
		assertAction(t, actions, "runner.use", true)
		assertAction(t, actions, "config_repo.read", true)
		assertAction(t, actions, "config_repo.use", true)
		assertAction(t, actions, "llm_profile.use", true)
		assertAction(t, actions, "agent_profile.use", true)
		assertAction(t, actions, "mcp_profile.use", true)
		assertAction(t, actions, "config_repo.manage", false)
		assertAction(t, actions, "config_repo.sync", false)
		assertAction(t, actions, "secret.read_value", false)
		assertAction(t, actions, "pipeline.delete", false)
		assertAction(t, actions, "pipeline.manage_acl", false)
		assertAction(t, actions, "step.delete", false)
		assertAction(t, actions, "step.manage_acl", false)
	})

	t.Run("owner", func(t *testing.T) {
		actions := actionSet(applicableProductRoleActions(productRoleOwner, grantResourceTeam))
		assertAction(t, actions, "team.delete", true)
		assertAction(t, actions, "pipeline.delete", true)
		assertAction(t, actions, "pipeline_schedule.delete", true)
		assertAction(t, actions, "pipeline_schedule.manage_acl", true)
		assertAction(t, actions, "git_webhook_source.delete", true)
		assertAction(t, actions, "git_webhook_source.manage_acl", true)
		assertAction(t, actions, "pipeline_run.delete", true)
		assertAction(t, actions, "pipeline_run.finalize", true)
		assertAction(t, actions, "pipeline_run.write_logs", true)
		assertAction(t, actions, "pipeline_run.task_update", true)
		assertAction(t, actions, "trigger.delete", true)
		assertAction(t, actions, "external_trigger.delete", true)
		assertAction(t, actions, "external_trigger.manage_acl", true)
		assertAction(t, actions, "dashboard.manage_acl", true)
		assertAction(t, actions, "scope.delete", true)
		assertAction(t, actions, "pipeline.manage_acl", true)
		assertAction(t, actions, "secret.read_value", true)
		assertAction(t, actions, "secret.delete", true)
		assertAction(t, actions, "variable.read_value", true)
		assertAction(t, actions, "variable.delete", true)
		assertAction(t, actions, "repository.delete", true)
		assertAction(t, actions, "step.delete", true)
		assertAction(t, actions, "step.manage_acl", true)
		assertAction(t, actions, "config_repo.read", true)
		assertAction(t, actions, "config_repo.manage", true)
		assertAction(t, actions, "config_repo.sync", true)
		assertAction(t, actions, "llm_profile.manage_acl", true)
		assertAction(t, actions, "agent_profile.manage_acl", true)
		assertAction(t, actions, "mcp_server.manage_acl", true)
		assertAction(t, actions, "mcp_profile.manage_acl", true)
	})

	t.Run("pipeline run", func(t *testing.T) {
		actions := actionSet(applicableProductRoleActions(productRoleViewer, grantResourceRun))
		assertAction(t, actions, "pipeline_run.list", true)
		assertAction(t, actions, "pipeline_run.read", true)
		assertAction(t, actions, "pipeline.read", false)
	})

	t.Run("repository related runs", func(t *testing.T) {
		viewerActions := actionSet(applicableProductRoleActions(productRoleViewer, grantResourceRepo))
		assertAction(t, viewerActions, "pipeline_run.list", true)
		assertAction(t, viewerActions, "pipeline_run.read", true)
		assertAction(t, viewerActions, "pipeline_run.read_logs", true)

		developerActions := actionSet(applicableProductRoleActions(productRoleDeveloper, grantResourceRepo))
		assertAction(t, developerActions, "pipeline_run.rerun", true)
		assertAction(t, developerActions, "pipeline_run.cancel", true)

		ownerActions := actionSet(applicableProductRoleActions(productRoleOwner, grantResourceRepo))
		assertAction(t, ownerActions, "pipeline_run.delete", true)
	})

	t.Run("admin", func(t *testing.T) {
		actions := applicableProductRoleActions(productRoleAdmin, grantResourcePlatform)
		if len(actions) != 1 || actions[0] != "*" {
			t.Fatalf("admin actions = %#v, want wildcard", actions)
		}
	})
}

func TestResolveAccessGrantServiceAccountCanonicalizesSub(t *testing.T) {
	runner := &fakeQueryRunner{
		row: fakeScanRow{values: []string{"webhook-deployer", "webhook-deployer"}},
	}

	subject, err := resolveAccessGrantSubject(context.Background(), runner, grantSubjectServiceAccount, "92343f56-9b7c-4267-bf28-b1846fe07d1f")
	if err != nil {
		t.Fatalf("resolveAccessGrantSubject() error = %v", err)
	}
	if subject.Type != model.SubjectTypeServiceAccount {
		t.Fatalf("subject type = %q, want service_account", subject.Type)
	}
	if subject.ID != "webhook-deployer" {
		t.Fatalf("subject ID = %q, want service account sub", subject.ID)
	}
	if len(runner.args) != 1 || len(runner.args[0]) == 0 || runner.args[0][0] != auth.ProviderServiceAccount {
		t.Fatalf("query args = %#v, want service account provider filter", runner.args)
	}
}

func TestResolveConfigSyncGrantResourceAllowsFutureGitOpsTargets(t *testing.T) {
	runner := &fakeQueryRunner{
		row: fakeScanRow{err: pgx.ErrNoRows},
	}

	tests := []struct {
		resourceType string
		resourceID   string
		wantID       string
	}{
		{resourceType: grantResourcePipeline, resourceID: "team-1/test", wantID: "team-1/test"},
		{resourceType: grantResourceTrigger, resourceID: "team-1/service-api", wantID: "team-1/service-api"},
		{resourceType: grantResourceScope, resourceID: "dev", wantID: "dev"},
	}
	for _, tt := range tests {
		resource, err := resolveConfigSyncGrantResource(context.Background(), runner, tt.resourceType, tt.resourceID)
		if err != nil {
			t.Fatalf("resolveConfigSyncGrantResource(%s:%s) error = %v", tt.resourceType, tt.resourceID, err)
		}
		if resource.Type != tt.resourceType || resource.ID != tt.wantID {
			t.Fatalf("resolveConfigSyncGrantResource(%s:%s) = %#v", tt.resourceType, tt.resourceID, resource)
		}
	}
	if len(runner.queries) != 0 {
		t.Fatalf("config sync grant resolution should not query for target existence, got %#v", runner.queries)
	}
}

func TestProductRoleHierarchy(t *testing.T) {
	assertRoleIncludesRole(t, productRoleDeveloper, productRoleViewer, grantResourceTeam)
	assertRoleIncludesRole(t, productRoleOwner, productRoleDeveloper, grantResourceTeam)
	assertRoleIncludesRole(t, productRoleOwner, productRoleViewer, grantResourceRepo)
}

func TestNormalizeAccessGrantResourceTypeSupportsPipelineRun(t *testing.T) {
	got, err := normalizeAccessGrantResourceType("pipeline_run")
	if err != nil {
		t.Fatalf("normalizeAccessGrantResourceType() error = %v", err)
	}
	if got != grantResourceRun {
		t.Fatalf("normalizeAccessGrantResourceType() = %q, want %q", got, grantResourceRun)
	}
}

func TestNormalizeAccessGrantResourceTypeSupportsPipelineSchedule(t *testing.T) {
	got, err := normalizeAccessGrantResourceType("pipeline_schedule")
	if err != nil {
		t.Fatalf("normalizeAccessGrantResourceType() error = %v", err)
	}
	if got != grantResourceSchedule {
		t.Fatalf("normalizeAccessGrantResourceType() = %q, want %q", got, grantResourceSchedule)
	}
}

func TestNormalizeAccessGrantResourceTypeSupportsGitWebhookSource(t *testing.T) {
	got, err := normalizeAccessGrantResourceType("git_webhook_source")
	if err != nil {
		t.Fatalf("normalizeAccessGrantResourceType() error = %v", err)
	}
	if got != grantResourceGitWebhookSource {
		t.Fatalf("normalizeAccessGrantResourceType() = %q, want %q", got, grantResourceGitWebhookSource)
	}
}

func TestNormalizeAccessGrantResourceTypeSupportsAIProfiles(t *testing.T) {
	tests := map[string]string{
		"llm_profile":   grantResourceLLMProfile,
		"agent_profile": grantResourceAgentProfile,
		"mcp_server":    grantResourceMCPServer,
		"mcp_profile":   grantResourceMCPProfile,
		"credential":    grantResourceCredential,
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			got, err := normalizeAccessGrantResourceType(raw)
			if err != nil {
				t.Fatalf("normalizeAccessGrantResourceType() error = %v", err)
			}
			if got != want {
				t.Fatalf("normalizeAccessGrantResourceType() = %q, want %q", got, want)
			}
		})
	}
}

func TestNormalizeAccessGrantSubjectTypeSupportsFirstClassCallers(t *testing.T) {
	tests := map[string]string{
		"auth_team":       model.SubjectTypeAuthTeam,
		"repository":      model.SubjectTypeRepository,
		"trigger":         model.SubjectTypeTrigger,
		"service_account": model.SubjectTypeServiceAccount,
	}
	for raw, want := range tests {
		got, err := normalizeAccessGrantSubjectType(raw)
		if err != nil {
			t.Fatalf("normalizeAccessGrantSubjectType(%q) error = %v", raw, err)
		}
		if got != want {
			t.Fatalf("normalizeAccessGrantSubjectType(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeAccessGrantSubjectTypeRejectsLegacyTeamAlias(t *testing.T) {
	if _, err := normalizeAccessGrantSubjectType("team"); err == nil {
		t.Fatal("expected legacy team subject_type to be rejected")
	}
}

func TestAccessGrantResponseUsesInternalSubjectID(t *testing.T) {
	response := accessGrantResponseFromRecord(accessGrantRecord{
		ID:             42,
		SubjectType:    model.SubjectTypeUser,
		SubjectID:      "user-uuid",
		SubjectDisplay: "alice",
		RoleName:       productRoleDeveloper,
		ResourceType:   grantResourceTeam,
		ResourceID:     generalGrantID,
		Inherit:        true,
	})

	if response.SubjectID != "user-uuid" {
		t.Fatalf("SubjectID = %q, want internal subject id", response.SubjectID)
	}
	if response.SubjectDisplay != "alice" {
		t.Fatalf("SubjectDisplay = %q, want display label", response.SubjectDisplay)
	}
}

func TestAccessGrantResponseIncludesGitOpsSource(t *testing.T) {
	response := accessGrantResponseFromRecord(accessGrantRecord{
		ID:                    7,
		SubjectType:           model.SubjectTypeRepository,
		SubjectID:             "acme/app",
		RoleName:              customUseGrantRole,
		ResourceType:          grantResourcePipeline,
		ResourceID:            "team-1/deploy",
		ManagedByConfig:       true,
		ConfigSourcePath:      "pipelines/deploy.yaml",
		ConfigSourceCommitSHA: "abc123",
	})

	if response.Source != grantSourceLocal || !response.ManagedByConfigRepo {
		t.Fatalf("source = (%q, %v), want local gitops managed", response.Source, response.ManagedByConfigRepo)
	}
	if response.ConfigSourcePath != "pipelines/deploy.yaml" {
		t.Fatalf("ConfigSourcePath = %q", response.ConfigSourcePath)
	}
}

func TestDeleteUserAccessArtifactsRemovesGrantRows(t *testing.T) {
	runner := &recordingExecRunner{}
	if err := deleteUserAccessArtifacts(context.Background(), runner, "user-uuid"); err != nil {
		t.Fatalf("deleteUserAccessArtifacts() error = %v", err)
	}

	joined := strings.Join(runner.statements, "\n")
	for _, want := range []string{"access_grants", "resource_acl", "resource_ownership", "auth_role_bindings", "user_roles"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("delete statements did not include %s: %s", want, joined)
		}
	}
}

func TestProductRoleTeamInheritance(t *testing.T) {
	developerBackend := newUserGrantBackend()
	developerBackend.aclPolicies = append(developerBackend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeUser, "user-1", grantResourceTeam, "payments")...)
	developerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: "payments"},
		Reason:   "team_inheritance",
	}}
	developerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "other-team/deploy-api"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: "other-team"},
		Reason:   "team_inheritance",
	}}
	developerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline_run", ID: "run-1"})] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"}, Reason: "pipeline_inheritance"},
		{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "payments"}, Reason: "team_inheritance"},
	}

	developerEvaluator := aaaauthz.NewEvaluator(developerBackend)

	decision, err := developerEvaluator.Check(context.Background(), developerBackend.resolved.Subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"}, nil)
	if err != nil {
		t.Fatalf("pipeline update Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("pipeline update decision = %#v, want allowed", decision)
	}

	decision, err = developerEvaluator.Check(context.Background(), developerBackend.resolved.Subject, "pipeline_run.rerun", model.ResourceRef{Type: "pipeline_run", ID: "run-1"}, nil)
	if err != nil {
		t.Fatalf("run rerun Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("run rerun decision = %#v, want allowed", decision)
	}

	decision, err = developerEvaluator.Check(context.Background(), developerBackend.resolved.Subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "other-team/deploy-api"}, nil)
	if err != nil {
		t.Fatalf("other-team pipeline update Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("other-team pipeline update decision = %#v, want denied", decision)
	}

	ownerBackend := newUserGrantBackend()
	ownerBackend.aclPolicies = append(ownerBackend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceTeam, "payments")...)
	ownerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: grantResourceTeam, ID: "payments/backend"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: "payments"},
		Reason:   "team_inheritance",
	}}
	ownerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: grantResourceTeam, ID: "other-team/backend"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: "other-team"},
		Reason:   "team_inheritance",
	}}

	ownerEvaluator := aaaauthz.NewEvaluator(ownerBackend)

	decision, err = ownerEvaluator.Check(context.Background(), ownerBackend.resolved.Subject, "team.manage_acl", model.ResourceRef{Type: grantResourceTeam, ID: "payments/backend"}, nil)
	if err != nil {
		t.Fatalf("team.manage_acl Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("team.manage_acl decision = %#v, want allowed", decision)
	}

	decision, err = ownerEvaluator.Check(context.Background(), ownerBackend.resolved.Subject, "team.manage_acl", model.ResourceRef{Type: grantResourceTeam, ID: "other-team/backend"}, nil)
	if err != nil {
		t.Fatalf("other-team team.manage_acl Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("other-team team.manage_acl decision = %#v, want denied", decision)
	}
}

func TestRepositoryRunInheritanceAllowsTeamOwnerToExploreRuns(t *testing.T) {
	backend := newUserGrantBackend()
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceTeam, "nopsai")...)
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline_run", ID: "run-1"})] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: grantResourceRepo, ID: "nopsai/test-app"}, Reason: "repository_inheritance"},
		{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "nopsai"}, Reason: "team_inheritance"},
	}

	evaluator := aaaauthz.NewEvaluator(backend)
	decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: "run-1"}, nil)
	if err != nil {
		t.Fatalf("pipeline run read Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("pipeline run read decision = %#v, want allowed through repository team inheritance", decision)
	}
}

func TestTeamOwnerCanReadAssignedTriggerResources(t *testing.T) {
	backend := newUserGrantBackend()
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceTeam, "nopsai")...)

	tests := []struct {
		name        string
		action      string
		resource    model.ResourceRef
		inheritance []model.InheritedResource
	}{
		{
			name:     "repository trigger",
			action:   "trigger.read",
			resource: model.ResourceRef{Type: grantResourceTrigger, ID: "nopsai/test-app"},
			inheritance: []model.InheritedResource{
				{Resource: model.ResourceRef{Type: grantResourceRepo, ID: "nopsai/test-app"}, Reason: "repository_inheritance"},
				{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "nopsai"}, Reason: "team_inheritance"},
			},
		},
		{
			name:     "external api trigger",
			action:   "external_trigger.read",
			resource: model.ResourceRef{Type: grantResourceExternalTrigger, ID: "production-release-hook"},
			inheritance: []model.InheritedResource{
				{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "nopsai"}, Reason: "team_inheritance"},
			},
		},
		{
			name:     "git webhook source",
			action:   "git_webhook_source.read",
			resource: model.ResourceRef{Type: grantResourceGitWebhookSource, ID: "github-main-source"},
			inheritance: []model.InheritedResource{
				{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "nopsai"}, Reason: "team_inheritance"},
			},
		},
	}

	evaluator := aaaauthz.NewEvaluator(backend)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend.inheritance[grantResourceKey(tt.resource)] = tt.inheritance
			decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, tt.action, tt.resource, nil)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if !decision.Allowed {
				t.Fatalf("decision = %#v, want team owner to read assigned %s", decision, tt.resource.Type)
			}
		})
	}

	otherTeamResource := model.ResourceRef{Type: grantResourceGitWebhookSource, ID: "github-other-source"}
	backend.inheritance[grantResourceKey(otherTeamResource)] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "other-team"}, Reason: "team_inheritance"},
	}
	decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "git_webhook_source.read", otherTeamResource, nil)
	if err != nil {
		t.Fatalf("other team Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("other team decision = %#v, want denied", decision)
	}
}

func TestPipelineRunListAllowsChildTeamOwnerThroughRunTeamInheritance(t *testing.T) {
	backend := newUserGrantBackend()
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceTeam, "team-1/dev")...)
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline_run", ID: "run-1"})] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "team-1/dev/t-app"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "team-1/dev"}, Reason: "team_inheritance"},
		{Resource: model.ResourceRef{Type: grantResourceTeam, ID: "team-1"}, Reason: "team_inheritance"},
	}

	evaluator := aaaauthz.NewEvaluator(backend)
	decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "pipeline_run.list", model.ResourceRef{Type: "pipeline_run", ID: "run-1"}, nil)
	if err != nil {
		t.Fatalf("pipeline run list Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("pipeline run list decision = %#v, want allowed through child team inheritance", decision)
	}
}

func TestGeneralTeamGrantInheritance(t *testing.T) {
	backend := newUserGrantBackend()
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeUser, "user-1", grantResourceTeam, model.TeamGeneralID)...)
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "root-pipeline"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: model.TeamGeneralID},
		Reason:   "team_inheritance",
	}}
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "dev/root-pipeline"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: "dev"},
		Reason:   "team_inheritance",
	}}

	evaluator := aaaauthz.NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "root-pipeline"}, nil)
	if err != nil {
		t.Fatalf("root pipeline Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("root pipeline decision = %#v, want allowed", decision)
	}

	decision, err = evaluator.Check(context.Background(), backend.resolved.Subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "dev/root-pipeline"}, nil)
	if err != nil {
		t.Fatalf("nested pipeline Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("nested pipeline decision = %#v, want denied", decision)
	}
}

func TestResolveAccessGrantTeamRoot(t *testing.T) {
	tests := []string{"root", "/root"}
	for _, raw := range tests {
		resource, err := resolveAccessGrantTeam(context.Background(), &noopQueryRunner{}, raw, false)
		if err != nil {
			t.Fatalf("resolveAccessGrantTeam(%q) error = %v", raw, err)
		}
		if resource.ID != model.TeamGeneralID || resource.Display != "root" {
			t.Fatalf("resolveAccessGrantTeam(%q) = %#v, want root team resource", raw, resource)
		}
	}
}

func TestResolveAccessGrantTeamDoesNotTreatLegacyAliasesAsRoot(t *testing.T) {
	tests := []string{"general", "/general", ".", model.TeamGeneralID}
	for _, raw := range tests {
		resource, err := resolveAccessGrantTeam(context.Background(), &noopQueryRunner{}, raw, false)
		if err != nil {
			t.Fatalf("resolveAccessGrantTeam(%q) error = %v", raw, err)
		}
		normalized := strings.Trim(strings.TrimSpace(raw), "/")
		if resource.ID != normalized || resource.Display != "/"+normalized {
			t.Fatalf("resolveAccessGrantTeam(%q) = %#v, want concrete team resource", raw, resource)
		}
	}
}

func TestValidateTeamOwnerGuardAllowsNonOwnerOnOwnerlessTeam(t *testing.T) {
	runner := &ownerGuardQueryRunner{}
	err := validateTeamOwnerGuard(context.Background(), runner, productRoleDeveloper, accessGrantResource{
		Type:    grantResourceTeam,
		ID:      "team-1/dev",
		Display: "/team-1/dev",
	}, 0)
	if err != nil {
		t.Fatalf("validateTeamOwnerGuard() error = %v, want ownerless team to allow non-owner grant", err)
	}
	if len(runner.queriedResourceIDs) != 0 {
		t.Fatalf("non-owner grant should not query owner count, got %#v", runner.queriedResourceIDs)
	}
}

func TestValidateTeamOwnerGuardAllowsNonOwnerWhenOnlyChildOwnerExists(t *testing.T) {
	runner := &ownerGuardQueryRunner{ownerCounts: map[string]int{"team-1/dev": 1}}
	err := validateTeamOwnerGuard(context.Background(), runner, productRoleDeveloper, accessGrantResource{
		Type:    grantResourceTeam,
		ID:      "team-1",
		Display: "/team-1",
	}, 0)
	if err != nil {
		t.Fatalf("validateTeamOwnerGuard() error = %v, want non-owner grant to be allowed", err)
	}
}

func TestValidateTeamOwnerGuardAllowsDeletingChildOwnerWhenParentOwnerRemains(t *testing.T) {
	runner := &ownerGuardQueryRunner{ownerCounts: map[string]int{"team-1": 1}}
	err := validateTeamOwnerGuard(context.Background(), runner, productRoleOwner, accessGrantResource{
		Type:    grantResourceTeam,
		ID:      "team-1/dev",
		Display: "/team-1/dev",
	}, 42)
	if err != nil {
		t.Fatalf("validateTeamOwnerGuard() error = %v, want parent owner to satisfy delete guard", err)
	}
}

func TestValidateTeamOwnerUpsertAllowsIdempotentOwnerRefresh(t *testing.T) {
	runner := &ownerGuardQueryRunner{}
	err := validateTeamOwnerUpsert(context.Background(), runner, productRoleOwner, productRoleOwner, accessGrantResource{
		Type:    grantResourceTeam,
		ID:      "team-1",
		Display: "/team-1",
	}, 42)
	if err != nil {
		t.Fatalf("validateTeamOwnerUpsert() error = %v, want idempotent owner refresh", err)
	}
	if len(runner.queriedResourceIDs) != 0 {
		t.Fatalf("owner refresh should not query owner count, got %#v", runner.queriedResourceIDs)
	}
}

func TestValidateTeamOwnerUpsertAllowsUpgradeToOwner(t *testing.T) {
	runner := &ownerGuardQueryRunner{}
	err := validateTeamOwnerUpsert(context.Background(), runner, productRoleDeveloper, productRoleOwner, accessGrantResource{
		Type:    grantResourceTeam,
		ID:      "team-1",
		Display: "/team-1",
	}, 42)
	if err != nil {
		t.Fatalf("validateTeamOwnerUpsert() error = %v, want upgrade to owner", err)
	}
}

func TestValidateTeamOwnerUpsertRejectsDowngradingLastOwner(t *testing.T) {
	runner := &ownerGuardQueryRunner{}
	err := validateTeamOwnerUpsert(context.Background(), runner, productRoleOwner, productRoleDeveloper, accessGrantResource{
		Type:    grantResourceTeam,
		ID:      "team-1",
		Display: "/team-1",
	}, 42)
	if err == nil || err.Error() != "every team must retain at least one owner" {
		t.Fatalf("validateTeamOwnerUpsert() error = %v, want last owner downgrade rejection", err)
	}
}

func TestExplicitDenyOverridesProductGrant(t *testing.T) {
	backend := newUserGrantBackend()
	backend.resolved.AuthTeams = []model.AuthTeamInfo{{ID: "team-1", Name: "payments-devs"}}
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeAuthTeam, "team-1", grantResourceTeam, "payments")...)
	backend.aclPolicies = append(backend.aclPolicies, fakeGrantACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   "payments/deploy-api",
		action:       "pipeline.update",
		effect:       "deny",
	})
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceTeam, ID: "payments"},
		Reason:   "team_inheritance",
	}}

	evaluator := aaaauthz.NewEvaluator(backend)
	decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "pipeline.update", model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if decision.Allowed || decision.Reason != "direct_acl_deny" {
		t.Fatalf("decision = %#v, want direct deny to win", decision)
	}
}

func TestSensitiveAccessDecisionsAreAudited(t *testing.T) {
	t.Run("denied decisions log", func(t *testing.T) {
		backend := newUserGrantBackend()
		evaluator := aaaauthz.NewEvaluator(backend)
		decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if decision.Allowed {
			t.Fatalf("decision = %#v, want denied", decision)
		}
		if len(backend.logs) != 1 || backend.logs[0].Reason != "default_deny" {
			t.Fatalf("logs = %#v, want default_deny entry", backend.logs)
		}
	})

	t.Run("secret read allowed logs sensitive", func(t *testing.T) {
		backend := newUserGrantBackend()
		backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceSecret, model.BuildNamedResourceID("", "", "TOKEN"))...)
		evaluator := aaaauthz.NewEvaluator(backend)
		decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "secret.read_value", model.ResourceRef{Type: "secret", ID: model.BuildNamedResourceID("", "", "TOKEN")}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("decision = %#v, want allowed", decision)
		}
		if len(backend.logs) != 1 || !backend.logs[0].Sensitive {
			t.Fatalf("logs = %#v, want sensitive entry", backend.logs)
		}
	})

	t.Run("secret write allowed logs sensitive", func(t *testing.T) {
		backend := newUserGrantBackend()
		backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeUser, "user-1", grantResourceSecret, model.BuildNamedResourceID("", "", "TOKEN"))...)
		evaluator := aaaauthz.NewEvaluator(backend)
		decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "secret.write_value", model.ResourceRef{Type: "secret", ID: model.BuildNamedResourceID("", "", "TOKEN")}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("decision = %#v, want allowed", decision)
		}
		if len(backend.logs) != 1 || !backend.logs[0].Sensitive {
			t.Fatalf("logs = %#v, want sensitive entry", backend.logs)
		}
	})

	t.Run("manage acl allowed logs sensitive", func(t *testing.T) {
		backend := newUserGrantBackend()
		backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceTeam, "payments")...)
		evaluator := aaaauthz.NewEvaluator(backend)
		decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "team.manage_acl", model.ResourceRef{Type: grantResourceTeam, ID: "payments"}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("decision = %#v, want allowed", decision)
		}
		if len(backend.logs) != 1 || !backend.logs[0].Sensitive {
			t.Fatalf("logs = %#v, want sensitive entry", backend.logs)
		}
	})

	t.Run("admin sensitive action logs", func(t *testing.T) {
		backend := newUserGrantBackend()
		backend.resolved.DirectRoles = []string{productRoleAdmin}
		backend.rolePolicies = append(backend.rolePolicies, fakeGrantRolePolicy{
			roleName:     productRoleAdmin,
			resourceType: "*",
			resourceID:   "*",
			action:       "*",
			effect:       "allow",
		})
		evaluator := aaaauthz.NewEvaluator(backend)
		decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "audit.read", model.ResourceRef{Type: "audit", ID: "authz"}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("decision = %#v, want allowed", decision)
		}
		if len(backend.logs) != 1 || !backend.logs[0].Sensitive {
			t.Fatalf("logs = %#v, want sensitive entry", backend.logs)
		}
	})
}

func TestAdminGrantRequiresPlatformAdmin(t *testing.T) {
	resource := accessGrantResource{Type: grantResourceTeam, ID: "payments", Display: "/payments"}
	err := authorizeGrantOperation(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1", Sub: "user-1"}, resource, productRoleAdmin, func(_ context.Context, _ model.Subject, action string, _ model.ResourceRef, _ map[string]any) (model.Decision, error) {
		if action != "iam.admin" {
			t.Fatalf("action = %q, want iam.admin", action)
		}
		return model.Decision{Allowed: false, Reason: "default_deny"}, nil
	}, nil)
	if err == nil || err.Error() != "forbidden" {
		t.Fatalf("authorizeGrantOperation() error = %v, want forbidden", err)
	}
}

func TestStepGrantManagementUsesManageACLAction(t *testing.T) {
	action, resource, err := managementActionForGrantResource(accessGrantResource{
		Type: grantResourceStep,
		ID:   "payments/build",
	})
	if err != nil {
		t.Fatalf("managementActionForGrantResource() error = %v", err)
	}
	if action != "step.manage_acl" {
		t.Fatalf("action = %q, want step.manage_acl", action)
	}
	if resource.Type != grantResourceStep || resource.ID != "payments/build" {
		t.Fatalf("resource = %#v, want step:payments/build", resource)
	}
}

func TestDashboardGrantManagementUsesManageACLAction(t *testing.T) {
	action, resource, err := managementActionForGrantResource(accessGrantResource{
		Type: grantResourceDashboard,
		ID:   "team-1/ops-dashboard",
	})
	if err != nil {
		t.Fatalf("managementActionForGrantResource() error = %v", err)
	}
	if action != "dashboard.manage_acl" {
		t.Fatalf("action = %q, want dashboard.manage_acl", action)
	}
	if resource.Type != grantResourceDashboard || resource.ID != "team-1/ops-dashboard" {
		t.Fatalf("resource = %#v, want dashboard:team-1/ops-dashboard", resource)
	}
}

func TestKnowledgeContextGrantManagementUsesManageAccessAction(t *testing.T) {
	action, resource, err := managementActionForGrantResource(accessGrantResource{
		Type: grantResourceKnowledgeContext,
		ID:   "guardrail/payments/repo-check",
	})
	if err != nil {
		t.Fatalf("managementActionForGrantResource() error = %v", err)
	}
	if action != "knowledge_context.manage_access" {
		t.Fatalf("action = %q, want knowledge_context.manage_access", action)
	}
	if resource.Type != grantResourceKnowledgeContext || resource.ID != "guardrail/payments/repo-check" {
		t.Fatalf("resource = %#v, want knowledge_context:guardrail/payments/repo-check", resource)
	}
}

func TestAIProfileGrantManagementUsesManageACLAction(t *testing.T) {
	action, resource, err := managementActionForGrantResource(accessGrantResource{
		Type: grantResourceMCPProfile,
		ID:   "github-pr-review",
	})
	if err != nil {
		t.Fatalf("managementActionForGrantResource() error = %v", err)
	}
	if action != "mcp_profile.manage_acl" {
		t.Fatalf("action = %q, want mcp_profile.manage_acl", action)
	}
	if resource.Type != grantResourceMCPProfile || resource.ID != "github-pr-review" {
		t.Fatalf("resource = %#v, want mcp_profile:github-pr-review", resource)
	}
}

func TestCredentialGrantManagementUsesManageACLAction(t *testing.T) {
	action, resource, err := managementActionForGrantResource(accessGrantResource{
		Type: grantResourceCredential,
		ID:   "system/llm/openai",
	})
	if err != nil {
		t.Fatalf("managementActionForGrantResource() error = %v", err)
	}
	if action != "credential.manage_acl" {
		t.Fatalf("action = %q, want credential.manage_acl", action)
	}
	if resource.Type != grantResourceCredential || resource.ID != "system/llm/openai" {
		t.Fatalf("resource = %#v, want credential:system/llm/openai", resource)
	}
}

func TestScopedProductGrantCapabilityUsesGrantRoleDefinition(t *testing.T) {
	if !productGrantIncludesAction(productRoleDeveloper, grantResourceTeam, "pipeline.create") {
		t.Fatal("developer team grant should include pipeline.create")
	}
	if !productGrantIncludesAction(productRoleDeveloper, grantResourceTeam, "scope.update") {
		t.Fatal("developer team grant should include scope.update")
	}
	if !productGrantIncludesAction(productRoleDeveloper, grantResourceTeam, "step.update") {
		t.Fatal("developer team grant should include step.update")
	}
	if !productGrantIncludesAction(productRoleDeveloper, grantResourceTeam, "credential.create") {
		t.Fatal("developer team grant should include credential.create")
	}
	if productGrantIncludesAction(productRoleViewer, grantResourceTeam, "pipeline.create") {
		t.Fatal("viewer team grant should not include pipeline.create")
	}
	if productGrantIncludesAction(productRoleDeveloper, grantResourceTeam, "pipeline.delete") {
		t.Fatal("developer team grant should not include pipeline.delete")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceTeam, "team.delete") {
		t.Fatal("owner team grant should include team.delete")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceTeam, "pipeline.delete") {
		t.Fatal("owner team grant should include pipeline.delete")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceTeam, "trigger.read") {
		t.Fatal("owner team grant should include trigger.read")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceTeam, "external_trigger.read") {
		t.Fatal("owner team grant should include external_trigger.read")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceTeam, "git_webhook_source.read") {
		t.Fatal("owner team grant should include git_webhook_source.read")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceTeam, "repository.delete") {
		t.Fatal("owner team grant should include repository.delete")
	}
	if !productGrantIncludesAction(productRoleOwner, grantResourceCredential, "credential.manage_acl") {
		t.Fatal("owner credential grant should include credential.manage_acl")
	}
}

func TestTeamDeleteAuthorizationTargetFromName(t *testing.T) {
	action, resource := teamDeleteAuthorizationTargetFromName("acme/widgets", "app", "acme/widgets", accessGrantResource{
		Type: grantResourceTeam,
		ID:   "platform/acme/widgets",
	})
	if action != "repository.delete" {
		t.Fatalf("action = %q, want repository.delete", action)
	}
	if resource.Type != grantResourceRepo || resource.ID != "acme/widgets" {
		t.Fatalf("resource = %#v, want repository:acme/widgets", resource)
	}

	action, resource = teamDeleteAuthorizationTargetFromName("platform", "team", "", accessGrantResource{
		Type: grantResourceTeam,
		ID:   "platform",
	})
	if action != "team.delete" {
		t.Fatalf("action = %q, want team.delete", action)
	}
	if resource.Type != grantResourceTeam || resource.ID != "platform" {
		t.Fatalf("resource = %#v, want team:platform", resource)
	}
}

func newUserGrantBackend() *fakeGrantBackend {
	return &fakeGrantBackend{
		resolved: &model.ResolvedSubject{
			Subject: model.Subject{
				Type: model.SubjectTypeUser,
				ID:   "user-1",
				Sub:  "user-1",
			},
			Status: "active",
		},
		inheritance: make(map[string][]model.InheritedResource),
	}
}

func grantACLPolicies(roleName, subjectType, subjectID, resourceType, resourceID string) []fakeGrantACLPolicy {
	actions := applicableProductRoleActions(roleName, resourceType)
	policies := make([]fakeGrantACLPolicy, 0, len(actions))
	for _, action := range actions {
		policies = append(policies, fakeGrantACLPolicy{
			subjectType:  subjectType,
			subjectID:    subjectID,
			resourceType: resourceType,
			resourceID:   resourceID,
			action:       action,
			effect:       "allow",
			roleName:     roleName,
		})
	}
	return policies
}

func actionSet(actions []string) map[string]struct{} {
	set := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		set[action] = struct{}{}
	}
	return set
}

func assertAction(t *testing.T, actions map[string]struct{}, action string, want bool) {
	t.Helper()
	_, ok := actions[action]
	if ok != want {
		t.Fatalf("action %q present = %t, want %t", action, ok, want)
	}
}

func assertRoleIncludesRole(t *testing.T, roleName, includedRoleName, resourceType string) {
	t.Helper()
	roleActions := actionSet(applicableProductRoleActions(roleName, resourceType))
	for _, action := range applicableProductRoleActions(includedRoleName, resourceType) {
		if _, ok := roleActions[action]; !ok {
			t.Fatalf("%s actions for %s missing inherited %s action %q", roleName, resourceType, includedRoleName, action)
		}
	}
}

func containsStringValue(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func grantMatches(policyValue, requestValue string) bool {
	return policyValue == "*" || policyValue == requestValue
}

func grantResourceKey(resource model.ResourceRef) string {
	return fmt.Sprintf("%s|%s", resource.Type, resource.ID)
}

func TestTeamGrantResourceForPathNormalizesRootAndTeamPaths(t *testing.T) {
	tests := []struct {
		name    string
		rawPath string
		wantOK  bool
		wantID  string
	}{
		{name: "empty path does not override fallback", rawPath: "", wantOK: false},
		{name: "root maps to general team grant", rawPath: "root", wantOK: true, wantID: generalGrantID},
		{name: "nested team trims slashes", rawPath: "/platform/prod/", wantOK: true, wantID: "platform/prod"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, ok, err := teamGrantResourceForPath(tt.rawPath)
			if err != nil {
				t.Fatalf("teamGrantResourceForPath() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if resource.Type != grantResourceTeam || resource.ID != tt.wantID {
				t.Fatalf("resource = %#v, want team:%s", resource, tt.wantID)
			}
		})
	}
}

func TestTeamOwnedEffectivePermissionResourceSupportsCreateScopes(t *testing.T) {
	tests := []struct {
		name         string
		action       string
		resourceType string
		wantOK       bool
	}{
		{name: "pipeline create", action: "pipeline.create", resourceType: grantResourcePipeline, wantOK: true},
		{name: "schedule create", action: "pipeline_schedule.create", resourceType: grantResourceSchedule, wantOK: true},
		{name: "trigger write create equivalent", action: "trigger.update", resourceType: grantResourceTrigger, wantOK: true},
		{name: "external trigger create", action: "external_trigger.create", resourceType: grantResourceExternalTrigger, wantOK: true},
		{name: "git webhook source create", action: "git_webhook_source.create", resourceType: grantResourceGitWebhookSource, wantOK: true},
		{name: "scope update create equivalent", action: "scope.update", resourceType: grantResourceScope, wantOK: true},
		{name: "step create", action: "step.create", resourceType: grantResourceStep, wantOK: true},
		{name: "pipeline update stays resource scoped", action: "pipeline.update", resourceType: grantResourcePipeline, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, ok, err := teamOwnedEffectivePermissionResource(tt.action, accessGrantResource{Type: tt.resourceType, ID: "probe"}, "platform")
			if err != nil {
				t.Fatalf("teamOwnedEffectivePermissionResource() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if tt.wantOK && (resource.Type != grantResourceTeam || resource.ID != "platform") {
				t.Fatalf("resource = %#v, want team:platform", resource)
			}
		})
	}
}

func TestRequireTeamOwnedCreateDecisionChecksPayloadTeam(t *testing.T) {
	var checkedResource model.ResourceRef
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				if action != "external_trigger.create" {
					t.Fatalf("action = %q, want external_trigger.create", action)
				}
				checkedResource = resource
				return model.Decision{Allowed: true}, nil
			},
		},
	}
	req := httptest.NewRequest("POST", "/v1/external-triggers", nil)
	req = req.WithContext(withAAASubject(req.Context(), model.Subject{Type: model.SubjectTypeUser, Sub: "alice"}))
	w := httptest.NewRecorder()

	ok := app.requireTeamOwnedCreateDecision(
		w,
		req,
		"external_trigger.create",
		model.ResourceRef{Type: grantResourceExternalTrigger, ID: "deploy-prod"},
		"platform/prod",
	)
	if !ok {
		t.Fatalf("requireTeamOwnedCreateDecision() denied with status %d", w.Code)
	}
	if checkedResource.Type != grantResourceTeam || checkedResource.ID != "platform/prod" {
		t.Fatalf("checked resource = %#v, want team:platform/prod", checkedResource)
	}
}

type ownerGuardQueryRunner struct {
	ownerCounts        map[string]int
	queriedResourceIDs []string
}

func (r *ownerGuardQueryRunner) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, fmt.Errorf("unsupported")
}

func (r *ownerGuardQueryRunner) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unsupported")
}

func (r *ownerGuardQueryRunner) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	resourceIDs, _ := args[1].([]string)
	r.queriedResourceIDs = append([]string(nil), resourceIDs...)

	count := 0
	for _, resourceID := range resourceIDs {
		count += r.ownerCounts[resourceID]
	}
	return ownerGuardCountRow{count: count}
}

type ownerGuardCountRow struct {
	count int
}

func (r ownerGuardCountRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("expected one destination, got %d", len(dest))
	}
	count, ok := dest[0].(*int)
	if !ok {
		return fmt.Errorf("expected *int destination")
	}
	*count = r.count
	return nil
}
