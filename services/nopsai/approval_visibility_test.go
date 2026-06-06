package nopsai

import (
	"context"
	"testing"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/routeauthz"
)

func TestApprovableRunSetFromApprovalsAllowsAssignedGroupApprover(t *testing.T) {
	checks := 0
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, subject model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				checks++
				if subject.ID != "alice-id" {
					t.Fatalf("subject id = %q, want alice-id", subject.ID)
				}
				if action != approvalActionApprove {
					t.Fatalf("action = %q, want %q", action, approvalActionApprove)
				}
				return model.Decision{Allowed: resource.Type == grantResourceFolder && resource.ID == "team-1"}, nil
			},
		},
	}

	visible, err := app.approvableRunSetFromApprovals(context.Background(), model.Subject{
		Type: model.SubjectTypeUser,
		ID:   "alice-id",
		Sub:  "alice",
	}, nil, []pendingApprovalVisibility{{
		RunID:             "run-1",
		AssignedGroups:    []string{"team-1", "prod"},
		AllowSelfApproval: false,
		RequestedByType:   model.SubjectTypeUser,
		RequestedByID:     "admin-id",
	}})
	if err != nil {
		t.Fatalf("approvableRunSetFromApprovals() error = %v", err)
	}
	if _, ok := visible[resourceKey(routeauthz.RunResource("run-1"))]; !ok {
		t.Fatalf("visible set = %#v, want run-1", visible)
	}
	if checks != 1 {
		t.Fatalf("AAA checks = %d, want 1", checks)
	}
}

func TestApprovableRunSetFromApprovalsHonorsSelfApprovalBlock(t *testing.T) {
	checks := 0
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error) {
				checks++
				return model.Decision{Allowed: true}, nil
			},
		},
	}

	visible, err := app.approvableRunSetFromApprovals(context.Background(), model.Subject{
		Type: model.SubjectTypeUser,
		ID:   "alice-id",
		Sub:  "alice",
	}, nil, []pendingApprovalVisibility{{
		RunID:             "run-1",
		AssignedGroups:    []string{"team-1"},
		AllowSelfApproval: false,
		RequestedByType:   model.SubjectTypeUser,
		RequestedByID:     "alice-id",
	}})
	if err != nil {
		t.Fatalf("approvableRunSetFromApprovals() error = %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("visible set = %#v, want empty", visible)
	}
	if checks != 0 {
		t.Fatalf("AAA checks = %d, want 0", checks)
	}
}
