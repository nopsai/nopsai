package nopsai

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeDashboardRefreshRequestScopesAndPolicy(t *testing.T) {
	input, err := normalizeDashboardRefreshRequest(dashboardRefreshRequest{
		Scope: dashboardRefreshScopeRequest{
			Type:       "section",
			SectionKey: "deployments",
		},
		Mode:           "best-effort",
		MaxConcurrency: 12,
		Timeout:        "10m",
		IdempotencyKey: " refresh-1 ",
	}, map[string]any{"max_concurrency": float64(6), "timeout": "20m"})
	if err != nil {
		t.Fatalf("normalizeDashboardRefreshRequest() error = %v", err)
	}
	if input.ScopeType != dashboardRefreshScopeSection {
		t.Fatalf("scope type = %q", input.ScopeType)
	}
	if input.Mode != dashboardRefreshModeBestEffort {
		t.Fatalf("mode = %q", input.Mode)
	}
	if input.MaxConcurrency != 12 {
		t.Fatalf("max concurrency = %d, want 12", input.MaxConcurrency)
	}
	if input.Timeout != 10*time.Minute {
		t.Fatalf("timeout = %s", input.Timeout)
	}
	if input.IdempotencyKey != "refresh-1" {
		t.Fatalf("idempotency key = %q", input.IdempotencyKey)
	}
	if got := strings.Join(input.SectionKeys, ","); got != "deployments" {
		t.Fatalf("section keys = %q", got)
	}
}

func TestNormalizeDashboardRefreshRequestRejectsStepThreeLimits(t *testing.T) {
	_, err := normalizeDashboardRefreshRequest(dashboardRefreshRequest{MaxConcurrency: dashboardRefreshMaxConcurrency + 1}, nil)
	if err == nil || !strings.Contains(err.Error(), "max_concurrency") {
		t.Fatalf("max concurrency error = %v", err)
	}
	_, err = normalizeDashboardRefreshRequest(dashboardRefreshRequest{Timeout: (dashboardRefreshMaxTimeout + time.Second).String()}, nil)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestNormalizeDashboardRefreshRequestRejectsMissingScopedTargets(t *testing.T) {
	for _, req := range []dashboardRefreshRequest{
		{Scope: dashboardRefreshScopeRequest{Type: "section"}},
		{Scope: dashboardRefreshScopeRequest{Type: "source"}},
	} {
		if _, err := normalizeDashboardRefreshRequest(req, nil); err == nil {
			t.Fatalf("normalizeDashboardRefreshRequest(%#v) expected error", req)
		}
	}
}

func TestSelectDashboardRefreshSources(t *testing.T) {
	sources := []dashboardSourceRecord{
		{ID: "s1", SectionKey: "overview", Enabled: true},
		{ID: "s2", SectionKey: "overview", Enabled: false},
		{ID: "s3", SectionKey: "deployments", Enabled: true},
	}

	all := selectDashboardRefreshSources(dashboardRefreshInput{ScopeType: dashboardRefreshScopeDashboard}, sources)
	if len(all) != 2 || all[0].ID != "s1" || all[1].ID != "s3" {
		t.Fatalf("dashboard selection = %#v", all)
	}

	section := selectDashboardRefreshSources(dashboardRefreshInput{ScopeType: dashboardRefreshScopeSection, SectionKeys: []string{"overview"}}, sources)
	if len(section) != 1 || section[0].ID != "s1" {
		t.Fatalf("section selection = %#v", section)
	}

	source := selectDashboardRefreshSources(dashboardRefreshInput{ScopeType: dashboardRefreshScopeSource, SourceIDs: []string{"s2"}}, sources)
	if len(source) != 1 || source[0].ID != "s2" {
		t.Fatalf("source selection should allow explicitly selected disabled source for strict/best-effort handling, got %#v", source)
	}
}

func TestDashboardRefreshRunStatusFromPipelineStatus(t *testing.T) {
	tests := map[string]string{
		"success":           dashboardRefreshRunStatusSuccess,
		"failure":           dashboardRefreshRunStatusFailed,
		"failure (ignored)": dashboardRefreshRunStatusFailed,
		"rejected":          dashboardRefreshRunStatusFailed,
		"cancelled":         dashboardRefreshRunStatusCancelled,
		"timed_out":         dashboardRefreshRunStatusTimedOut,
		"skipped":           dashboardRefreshRunStatusSkipped,
		"pending":           dashboardRefreshRunStatusRunning,
		"waiting_approval":  dashboardRefreshRunStatusRunning,
	}
	for status, want := range tests {
		if got := dashboardRefreshRunStatusFromPipelineStatus(status); got != want {
			t.Fatalf("status %q => %q, want %q", status, got, want)
		}
	}
}
