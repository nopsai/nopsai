package nopsai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"nopsai/services/aaa/pkg/model"
	"nopsai/services/nopsai/pkg/auth"
)

func TestRequireDashboardTargetWriteDecisionSkipsUnchangedTarget(t *testing.T) {
	calls := 0
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(context.Context, model.Subject, string, model.ResourceRef, map[string]any) (model.Decision, error) {
				calls++
				return model.Decision{Allowed: false}, nil
			},
		},
	}

	req := withClaimsRequest(http.MethodPut, "/v1/dashboards/dashboard-id", "", &auth.Claims{Sub: "editor", Provider: "local"})
	rec := httptest.NewRecorder()
	ok := app.requireDashboardTargetWriteDecision(rec, req, dashboardRecord{
		TeamPath: "platform/ops",
		Slug:     "release-health",
	}, dashboardInput{
		TeamPath: "platform/ops",
		Slug:     "release-health",
	})

	if !ok {
		t.Fatal("requireDashboardTargetWriteDecision() denied unchanged dashboard target")
	}
	if calls != 0 {
		t.Fatalf("AAA checks = %d, want 0 for unchanged dashboard target", calls)
	}
}

func TestRequireDashboardTargetWriteDecisionRequiresCreateOnMovedTarget(t *testing.T) {
	var checkedAction string
	var checkedResource model.ResourceRef
	app := &App{
		aaaLocal: stubAAAAuthorizer{
			checkFn: func(_ context.Context, _ model.Subject, action string, resource model.ResourceRef, _ map[string]any) (model.Decision, error) {
				checkedAction = action
				checkedResource = resource
				return model.Decision{Allowed: false, Reason: "deny"}, nil
			},
		},
	}

	req := withClaimsRequest(http.MethodPut, "/v1/dashboards/dashboard-id", "", &auth.Claims{Sub: "editor", Provider: "local"})
	rec := httptest.NewRecorder()
	ok := app.requireDashboardTargetWriteDecision(rec, req, dashboardRecord{
		TeamPath: "platform/ops",
		Slug:     "release-health",
	}, dashboardInput{
		TeamPath: "platform/security",
		Slug:     "release-health",
	})

	if ok {
		t.Fatal("requireDashboardTargetWriteDecision() allowed dashboard move without target create permission")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if checkedAction != "dashboard.create" {
		t.Fatalf("action = %q, want dashboard.create", checkedAction)
	}
	if checkedResource.Type != grantResourceTeam || checkedResource.ID != "platform/security" {
		t.Fatalf("resource = %#v, want team:platform/security", checkedResource)
	}
}
