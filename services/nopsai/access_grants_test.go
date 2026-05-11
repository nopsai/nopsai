package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	aaaauthz "nopsai/services/aaa/pkg/authz"
	"nopsai/services/aaa/pkg/model"
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
		actions := actionSet(applicableProductRoleActions(productRoleViewer, grantResourceFolder))
		assertAction(t, actions, "pipeline.read", true)
		assertAction(t, actions, "pipeline.update", false)
		assertAction(t, actions, "pipeline.execute", false)
		assertAction(t, actions, "secret.read_value", false)
	})

	t.Run("developer", func(t *testing.T) {
		actions := actionSet(applicableProductRoleActions(productRoleDeveloper, grantResourceFolder))
		assertAction(t, actions, "pipeline.read", true)
		assertAction(t, actions, "pipeline.update", true)
		assertAction(t, actions, "pipeline.execute", true)
		assertAction(t, actions, "secret.write_value", true)
		assertAction(t, actions, "secret.read_value", false)
		assertAction(t, actions, "pipeline.delete", false)
		assertAction(t, actions, "pipeline.manage_acl", false)
	})

	t.Run("owner", func(t *testing.T) {
		actions := actionSet(applicableProductRoleActions(productRoleOwner, grantResourceFolder))
		assertAction(t, actions, "pipeline.delete", true)
		assertAction(t, actions, "pipeline.manage_acl", true)
		assertAction(t, actions, "secret.read_value", true)
	})

	t.Run("admin", func(t *testing.T) {
		actions := applicableProductRoleActions(productRoleAdmin, grantResourcePlatform)
		if len(actions) != 1 || actions[0] != "*" {
			t.Fatalf("admin actions = %#v, want wildcard", actions)
		}
	})
}

func TestProductRoleFolderInheritance(t *testing.T) {
	developerBackend := newUserGrantBackend()
	developerBackend.aclPolicies = append(developerBackend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeUser, "user-1", grantResourceFolder, "payments")...)
	developerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: "payments"},
		Reason:   "folder_inheritance",
	}}
	developerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "other-team/deploy-api"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: "other-team"},
		Reason:   "folder_inheritance",
	}}
	developerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline_run", ID: "run-1"})] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"}, Reason: "pipeline_inheritance"},
		{Resource: model.ResourceRef{Type: grantResourceFolder, ID: "payments"}, Reason: "folder_inheritance"},
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
	ownerBackend.aclPolicies = append(ownerBackend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceFolder, "payments")...)
	ownerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: grantResourceFolder, ID: "payments/backend"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: "payments"},
		Reason:   "folder_inheritance",
	}}
	ownerBackend.inheritance[grantResourceKey(model.ResourceRef{Type: grantResourceFolder, ID: "other-team/backend"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: "other-team"},
		Reason:   "folder_inheritance",
	}}

	ownerEvaluator := aaaauthz.NewEvaluator(ownerBackend)

	decision, err = ownerEvaluator.Check(context.Background(), ownerBackend.resolved.Subject, "folder.manage_acl", model.ResourceRef{Type: grantResourceFolder, ID: "payments/backend"}, nil)
	if err != nil {
		t.Fatalf("folder.manage_acl Check() error = %v", err)
	}
	if !decision.Allowed {
		t.Fatalf("folder.manage_acl decision = %#v, want allowed", decision)
	}

	decision, err = ownerEvaluator.Check(context.Background(), ownerBackend.resolved.Subject, "folder.manage_acl", model.ResourceRef{Type: grantResourceFolder, ID: "other-team/backend"}, nil)
	if err != nil {
		t.Fatalf("other-team folder.manage_acl Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("other-team folder.manage_acl decision = %#v, want denied", decision)
	}
}

func TestGeneralFolderGrantInheritance(t *testing.T) {
	backend := newUserGrantBackend()
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeUser, "user-1", grantResourceFolder, model.FolderGeneralID)...)
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "root-pipeline"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: model.FolderGeneralID},
		Reason:   "folder_inheritance",
	}}
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "dev/root-pipeline"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: "dev"},
		Reason:   "folder_inheritance",
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

func TestResolveAccessGrantFolderGeneralAlias(t *testing.T) {
	tests := []string{"general", "/general", "/", ".", model.FolderGeneralID}
	for _, raw := range tests {
		resource, err := resolveAccessGrantFolder(context.Background(), &noopQueryRunner{}, raw, false)
		if err != nil {
			t.Fatalf("resolveAccessGrantFolder(%q) error = %v", raw, err)
		}
		if resource.ID != model.FolderGeneralID || resource.Display != "general" {
			t.Fatalf("resolveAccessGrantFolder(%q) = %#v, want general folder resource", raw, resource)
		}
	}
}

func TestValidateFolderOwnerGuardUsesInheritedFolderOwner(t *testing.T) {
	runner := &ownerGuardQueryRunner{ownerCounts: map[string]int{"team-1": 1}}
	err := validateFolderOwnerGuard(context.Background(), runner, productRoleDeveloper, accessGrantResource{
		Type:    grantResourceFolder,
		ID:      "team-1/dev",
		Display: "/team-1/dev",
	}, 0)
	if err != nil {
		t.Fatalf("validateFolderOwnerGuard() error = %v, want inherited owner to satisfy guard", err)
	}

	wantQueried := []string{"team-1/dev", "team-1"}
	if !reflect.DeepEqual(runner.queriedResourceIDs, wantQueried) {
		t.Fatalf("queried owner resource IDs = %#v, want %#v", runner.queriedResourceIDs, wantQueried)
	}
}

func TestValidateFolderOwnerGuardRejectsWithoutExactOrParentOwner(t *testing.T) {
	runner := &ownerGuardQueryRunner{ownerCounts: map[string]int{"team-1/dev": 1}}
	err := validateFolderOwnerGuard(context.Background(), runner, productRoleDeveloper, accessGrantResource{
		Type:    grantResourceFolder,
		ID:      "team-1",
		Display: "/team-1",
	}, 0)
	if err == nil || err.Error() != "grant an owner on /team-1 before assigning other roles" {
		t.Fatalf("validateFolderOwnerGuard() error = %v, want owner guard rejection", err)
	}
}

func TestValidateFolderOwnerGuardAllowsDeletingChildOwnerWhenParentOwnerRemains(t *testing.T) {
	runner := &ownerGuardQueryRunner{ownerCounts: map[string]int{"team-1": 1}}
	err := validateFolderOwnerGuard(context.Background(), runner, productRoleOwner, accessGrantResource{
		Type:    grantResourceFolder,
		ID:      "team-1/dev",
		Display: "/team-1/dev",
	}, 42)
	if err != nil {
		t.Fatalf("validateFolderOwnerGuard() error = %v, want parent owner to satisfy delete guard", err)
	}
}

func TestExplicitDenyOverridesProductGrant(t *testing.T) {
	backend := newUserGrantBackend()
	backend.resolved.AuthGroups = []model.AuthGroupInfo{{ID: "group-1", Name: "payments-devs"}}
	backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleDeveloper, model.SubjectTypeAuthGroup, "group-1", grantResourceFolder, "payments")...)
	backend.aclPolicies = append(backend.aclPolicies, fakeGrantACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   "payments/deploy-api",
		action:       "pipeline.update",
		effect:       "deny",
	})
	backend.inheritance[grantResourceKey(model.ResourceRef{Type: "pipeline", ID: "payments/deploy-api"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: grantResourceFolder, ID: "payments"},
		Reason:   "folder_inheritance",
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
		backend.aclPolicies = append(backend.aclPolicies, grantACLPolicies(productRoleOwner, model.SubjectTypeUser, "user-1", grantResourceFolder, "payments")...)
		evaluator := aaaauthz.NewEvaluator(backend)
		decision, err := evaluator.Check(context.Background(), backend.resolved.Subject, "folder.manage_acl", model.ResourceRef{Type: grantResourceFolder, ID: "payments"}, nil)
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
	resource := accessGrantResource{Type: grantResourceFolder, ID: "payments", Display: "/payments"}
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
