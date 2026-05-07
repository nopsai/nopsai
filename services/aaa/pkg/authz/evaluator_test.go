package authz

import (
	"context"
	"testing"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/aaa/pkg/store"
)

type fakeRolePolicy struct {
	roleName     string
	resourceType string
	resourceID   string
	action       string
	effect       string
}

type fakeACLPolicy struct {
	subjectType  string
	subjectID    string
	resourceType string
	resourceID   string
	action       string
	effect       string
}

type fakeBackend struct {
	resolved     *model.ResolvedSubject
	resolveErr   error
	rolePolicies []fakeRolePolicy
	aclPolicies  []fakeACLPolicy
	inheritance  map[string][]model.InheritedResource
	logs         []model.DecisionLogEntry
}

func (f *fakeBackend) ResolveSubject(context.Context, model.Subject) (*model.ResolvedSubject, error) {
	return f.resolved, f.resolveErr
}

func (f *fakeBackend) FindRolePermissionMatch(_ context.Context, roleNames []string, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	for _, policy := range f.rolePolicies {
		if !contains(roleNames, policy.roleName) {
			continue
		}
		if !matches(policy.effect, effect) || !matches(policy.resourceType, resource.Type) || !matches(policy.resourceID, resource.ID) || !matches(policy.action, action) {
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

func (f *fakeBackend) FindACLMatch(_ context.Context, subject model.SubjectRef, resource model.ResourceRef, action, effect string) (*model.MatchedPolicy, error) {
	for _, policy := range f.aclPolicies {
		if policy.subjectType != subject.Type || policy.subjectID != subject.ID {
			continue
		}
		if !matches(policy.effect, effect) || !matches(policy.resourceType, resource.Type) || !matches(policy.resourceID, resource.ID) || !matches(policy.action, action) {
			continue
		}
		return &model.MatchedPolicy{
			Source:       "resource_acl",
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

func (f *fakeBackend) ResolveResourceInheritance(_ context.Context, resource model.ResourceRef) ([]model.InheritedResource, error) {
	if inheritance, ok := f.inheritance[resourceKey(resource)]; ok {
		return inheritance, nil
	}
	return nil, nil
}

func (f *fakeBackend) WriteDecisionLog(_ context.Context, entry model.DecisionLogEntry) error {
	f.logs = append(f.logs, entry)
	return nil
}

func (f *fakeBackend) RecordAudit(_ context.Context, entry model.DecisionLogEntry) error {
	f.logs = append(f.logs, entry)
	return nil
}

func TestEvaluatorDefaultDenyLogsDecision(t *testing.T) {
	backend := newUserBackend()
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, map[string]any{"request_id": "deny-1"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatal("Check() allowed = true, want false")
	}
	if decision.Reason != "default_deny" {
		t.Fatalf("Check() reason = %q, want default_deny", decision.Reason)
	}
	if len(backend.logs) != 1 || backend.logs[0].Reason != "default_deny" {
		t.Fatalf("deny decision was not logged correctly: %#v", backend.logs)
	}
}

func TestEvaluatorAdminAllowLogsSensitiveDecision(t *testing.T) {
	backend := newUserBackend()
	backend.resolved.DirectRoles = []string{model.RoleNameAdmin}
	backend.rolePolicies = append(backend.rolePolicies, fakeRolePolicy{
		roleName:     model.RoleNameAdmin,
		resourceType: "*",
		resourceID:   "*",
		action:       "*",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.execute", model.ResourceRef{Type: "pipeline", ID: "team/build"}, map[string]any{"request_id": "allow-1"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "admin_role_allow" {
		t.Fatalf("Check() = %#v, want sensitive admin allow", decision)
	}
	if len(backend.logs) != 1 || !backend.logs[0].Sensitive {
		t.Fatalf("sensitive allow was not logged correctly: %#v", backend.logs)
	}
}

func TestEvaluatorDirectACLAllow(t *testing.T) {
	backend := newUserBackend()
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline.read",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "direct_acl_allow" {
		t.Fatalf("Check() = %#v, want direct ACL allow", decision)
	}
}

func TestEvaluatorExplicitDenyOverridesAllow(t *testing.T) {
	backend := newUserBackend()
	backend.rolePolicies = append(backend.rolePolicies, fakeRolePolicy{
		roleName:     "viewer",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline.read",
		effect:       "allow",
	})
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline.read",
		effect:       "deny",
	})
	backend.resolved.DirectRoles = []string{"viewer"}
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if decision.Allowed || decision.Reason != "direct_acl_deny" {
		t.Fatalf("Check() = %#v, want direct ACL deny", decision)
	}
}

func TestEvaluatorAuthGroupACLAllow(t *testing.T) {
	backend := newUserBackend()
	backend.resolved.AuthGroups = []model.AuthGroupInfo{{ID: "group-1", Name: "ops"}}
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeAuthGroup,
		subjectID:    "group-1",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline.read",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "auth_group_acl_allow" {
		t.Fatalf("Check() = %#v, want auth group ACL allow", decision)
	}
}

func TestEvaluatorRoleBindingAllow(t *testing.T) {
	backend := newUserBackend()
	backend.resolved.DirectRoles = []string{"runner"}
	backend.rolePolicies = append(backend.rolePolicies, fakeRolePolicy{
		roleName:     "runner",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline.execute",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.execute", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "direct_role_allow" {
		t.Fatalf("Check() = %#v, want direct role allow", decision)
	}
}

func TestEvaluatorFolderInheritance(t *testing.T) {
	backend := newUserBackend()
	backend.inheritance[resourceKey(model.ResourceRef{Type: "pipeline", ID: "team/build"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: "folder", ID: "team"},
		Reason:   "folder_inheritance",
	}}
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "folder",
		resourceID:   "team",
		action:       "pipeline.read",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "inherited_folder_acl_allow" {
		t.Fatalf("Check() = %#v, want folder inheritance allow", decision)
	}
}

func TestEvaluatorPipelineRunInheritance(t *testing.T) {
	backend := newUserBackend()
	backend.inheritance[resourceKey(model.ResourceRef{Type: "pipeline_run", ID: "run-1"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: "pipeline", ID: "team/build"},
		Reason:   "pipeline_inheritance",
	}}
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline_run.read",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline_run.read", model.ResourceRef{Type: "pipeline_run", ID: "run-1"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "pipeline_acl_inheritance_allow" {
		t.Fatalf("Check() = %#v, want pipeline run inheritance allow", decision)
	}
}

func TestEvaluatorRepositoryInheritance(t *testing.T) {
	backend := newUserBackend()
	backend.inheritance[resourceKey(model.ResourceRef{Type: "trigger", ID: "owner/repo"})] = []model.InheritedResource{{
		Resource: model.ResourceRef{Type: "repository", ID: "owner/repo"},
		Reason:   "repository_inheritance",
	}}
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "repository",
		resourceID:   "owner/repo",
		action:       "trigger.read",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "trigger.read", model.ResourceRef{Type: "trigger", ID: "owner/repo"}, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !decision.Allowed || decision.Reason != "repository_acl_inheritance_allow" {
		t.Fatalf("Check() = %#v, want repository inheritance allow", decision)
	}
}

func TestEvaluatorSecretAndVariableInheritance(t *testing.T) {
	backend := newUserBackend()
	secret := model.ResourceRef{Type: "secret", ID: model.BuildNamedResourceID("owner/repo", "prod", "TOKEN")}
	variable := model.ResourceRef{Type: "variable", ID: model.BuildNamedResourceID("owner/repo", "prod", "TIMEOUT")}
	backend.inheritance[resourceKey(secret)] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "repository", ID: "owner/repo"}, Reason: "repository_inheritance"},
		{Resource: model.ResourceRef{Type: "scope", ID: "prod"}, Reason: "scope_inheritance"},
	}
	backend.inheritance[resourceKey(variable)] = []model.InheritedResource{
		{Resource: model.ResourceRef{Type: "repository", ID: "owner/repo"}, Reason: "repository_inheritance"},
		{Resource: model.ResourceRef{Type: "scope", ID: "prod"}, Reason: "scope_inheritance"},
	}
	backend.aclPolicies = append(backend.aclPolicies,
		fakeACLPolicy{
			subjectType:  model.SubjectTypeUser,
			subjectID:    "user-1",
			resourceType: "repository",
			resourceID:   "owner/repo",
			action:       "secret.read_value",
			effect:       "allow",
		},
		fakeACLPolicy{
			subjectType:  model.SubjectTypeUser,
			subjectID:    "user-1",
			resourceType: "scope",
			resourceID:   "prod",
			action:       "variable.read_value",
			effect:       "allow",
		},
	)
	evaluator := NewEvaluator(backend)

	secretDecision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "secret.read_value", secret, nil)
	if err != nil {
		t.Fatalf("secret Check() error = %v", err)
	}
	if !secretDecision.Allowed || secretDecision.Reason != "repository_acl_inheritance_allow" {
		t.Fatalf("secret Check() = %#v, want repository inheritance allow", secretDecision)
	}

	variableDecision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "variable.read_value", variable, nil)
	if err != nil {
		t.Fatalf("variable Check() error = %v", err)
	}
	if !variableDecision.Allowed || variableDecision.Reason != "scope_acl_inheritance_allow" {
		t.Fatalf("variable Check() = %#v, want scope inheritance allow", variableDecision)
	}
}

func TestEvaluatorSecretMetadataDoesNotImplyReadValue(t *testing.T) {
	backend := newUserBackend()
	resource := model.ResourceRef{Type: "secret", ID: model.BuildNamedResourceID("", "", "TOKEN")}
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "secret",
		resourceID:   resource.ID,
		action:       "secret.list_metadata",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "secret.read_value", resource, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if decision.Allowed {
		t.Fatalf("Check() allowed = true, want false; decision=%#v", decision)
	}
}

func TestEvaluatorBatchCheckPreservesOrder(t *testing.T) {
	backend := newUserBackend()
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   "team/build",
		action:       "pipeline.read",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	decisions, err := evaluator.BatchCheck(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, []model.BatchCheckItem{
		{Action: "pipeline.delete", Resource: model.ResourceRef{Type: "pipeline", ID: "team/build"}},
		{Action: "pipeline.read", Resource: model.ResourceRef{Type: "pipeline", ID: "team/build"}},
	}, nil)
	if err != nil {
		t.Fatalf("BatchCheck() error = %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("BatchCheck() len = %d, want 2", len(decisions))
	}
	if decisions[0].Allowed || !decisions[1].Allowed {
		t.Fatalf("BatchCheck() order mismatch: %#v", decisions)
	}
}

func TestEvaluatorFilterReturnsOnlyAllowedResources(t *testing.T) {
	backend := newUserBackend()
	alpha := model.ResourceRef{Type: "pipeline", ID: "team/alpha"}
	beta := model.ResourceRef{Type: "pipeline", ID: "team/beta"}
	backend.aclPolicies = append(backend.aclPolicies, fakeACLPolicy{
		subjectType:  model.SubjectTypeUser,
		subjectID:    "user-1",
		resourceType: "pipeline",
		resourceID:   alpha.ID,
		action:       "pipeline.list",
		effect:       "allow",
	})
	evaluator := NewEvaluator(backend)

	allowed, err := evaluator.Filter(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.list", []model.ResourceRef{alpha, beta}, nil)
	if err != nil {
		t.Fatalf("Filter() error = %v", err)
	}
	if len(allowed) != 1 || allowed[0].ID != alpha.ID {
		t.Fatalf("Filter() = %#v, want only alpha", allowed)
	}
}

func TestEvaluatorSubjectResolutionErrorsDeny(t *testing.T) {
	t.Run("inactive user denied", func(t *testing.T) {
		backend := newUserBackend()
		backend.resolveErr = store.ErrSubjectInactive
		evaluator := NewEvaluator(backend)

		decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if decision.Allowed || decision.Reason != "subject_inactive" {
			t.Fatalf("Check() = %#v, want inactive subject deny", decision)
		}
	})

	t.Run("missing user denied", func(t *testing.T) {
		backend := newUserBackend()
		backend.resolved = nil
		backend.resolveErr = store.ErrSubjectNotFound
		evaluator := NewEvaluator(backend)

		decision, err := evaluator.Check(context.Background(), model.Subject{Type: model.SubjectTypeUser, ID: "user-1"}, "pipeline.read", model.ResourceRef{Type: "pipeline", ID: "team/build"}, nil)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if decision.Allowed || decision.Reason != "subject_not_found" {
			t.Fatalf("Check() = %#v, want subject not found deny", decision)
		}
	})
}

func newUserBackend() *fakeBackend {
	return &fakeBackend{
		resolved: &model.ResolvedSubject{
			Subject: model.Subject{
				Type: model.SubjectTypeUser,
				ID:   "user-1",
				Sub:  "user-1",
			},
			Status:      "active",
			DirectRoles: nil,
		},
		inheritance: make(map[string][]model.InheritedResource),
	}
}

func matches(policyValue, requestValue string) bool {
	return policyValue == "*" || policyValue == requestValue
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func resourceKey(resource model.ResourceRef) string {
	return resource.Type + "|" + resource.ID
}
