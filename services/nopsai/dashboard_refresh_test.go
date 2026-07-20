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

func TestNormalizeDashboardSourceInputKeepsDefaultScopeExact(t *testing.T) {
	input, err := normalizeDashboardSourceInput(dashboardSourceRequest{
		SectionKey: "overview",
		PipelineID: "team-1/dashboard",
		OutputName: "Service Health",
		EntryKey:   "health",
		RunScope:   "default",
	})
	if err != nil {
		t.Fatalf("normalizeDashboardSourceInput() error = %v", err)
	}
	if input.RunScope != "" {
		t.Fatalf("run scope = %q, want empty exact default scope", input.RunScope)
	}

	input, err = normalizeDashboardSourceInput(dashboardSourceRequest{
		SectionKey: "overview",
		PipelineID: "team-1/dashboard",
		OutputName: "Service Health",
		EntryKey:   "health",
		RunScope:   "/prod/",
	})
	if err != nil {
		t.Fatalf("normalizeDashboardSourceInput(scoped) error = %v", err)
	}
	if input.RunScope != "prod" {
		t.Fatalf("run scope = %q, want prod", input.RunScope)
	}
}

func TestNormalizeDashboardSourceInputAllowsEmptyEntryKey(t *testing.T) {
	input, err := normalizeDashboardSourceInput(dashboardSourceRequest{
		SectionKey: "overview",
		PipelineID: "team-1/dashboard",
		OutputName: "Service Health",
		EntryKey:   " ",
	})
	if err != nil {
		t.Fatalf("normalizeDashboardSourceInput() error = %v", err)
	}
	if input.EntryKey != "" {
		t.Fatalf("entry key = %q, want empty output-name binding", input.EntryKey)
	}
}

func TestDashboardRefreshRunResponseIncludesPipelineAndOutputStatus(t *testing.T) {
	startedAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
	pipelineFinishedAt := startedAt.Add(2 * time.Minute)
	outputCreatedAt := pipelineFinishedAt.Add(time.Second)
	outputGenerationStartedAt := outputCreatedAt.Add(30 * time.Second)
	outputUpdatedAt := outputGenerationStartedAt.Add(59 * time.Second)
	outputDuration, outputDurationSeconds := pipelineOutputGenerationDuration(&outputGenerationStartedAt, outputUpdatedAt)

	response := dashboardRefreshRunResponseFromRecord(dashboardRefreshRunRecord{
		ID:                        "refresh-run-1",
		RefreshID:                 "refresh-1",
		PipelineID:                "team-1/dashboard",
		OutputName:                "Service Health",
		SectionKey:                "overview",
		Required:                  true,
		Status:                    "running",
		CreatedAt:                 startedAt,
		UpdatedAt:                 outputUpdatedAt,
		PipelineStatus:            "success",
		PipelineStartedAt:         &startedAt,
		PipelineFinishedAt:        &pipelineFinishedAt,
		OutputStatus:              "generating",
		OutputCreatedAt:           &outputCreatedAt,
		OutputGenerationStartedAt: &outputGenerationStartedAt,
		OutputUpdatedAt:           &outputUpdatedAt,
		OutputDuration:            outputDuration,
		OutputDurationSeconds:     outputDurationSeconds,
	})

	if response.Status != "running" {
		t.Fatalf("status = %q, want refresh source rollup running", response.Status)
	}
	if response.PipelineStatus != "success" || response.PipelineFinishedAt == nil {
		t.Fatalf("pipeline fields = %#v", response)
	}
	if response.OutputStatus != "generating" || response.OutputDuration != "59s" || response.OutputDurationSeconds != 59 {
		t.Fatalf("output fields = %#v", response)
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

func TestGroupDashboardRefreshLaunchSourcesDedupesPipelineAndScope(t *testing.T) {
	plans := []dashboardRefreshSourcePlan{
		{
			Source: dashboardSourceRecord{ID: "s1", PipelineID: "team/dashboard-sample", RunScope: "prod"},
			Launch: true,
		},
		{
			Source: dashboardSourceRecord{ID: "s2", PipelineID: "team/dashboard-sample", RunScope: "prod"},
			Launch: true,
		},
		{
			Source: dashboardSourceRecord{ID: "s3", PipelineID: "team/dashboard-sample", RunScope: "dev"},
			Launch: true,
		},
		{
			Source: dashboardSourceRecord{ID: "s4", PipelineID: "team/other", RunScope: "prod"},
			Launch: false,
		},
	}

	groups := groupDashboardRefreshLaunchSources(dashboardRefreshInput{}, plans)
	if len(groups) != 2 {
		t.Fatalf("groups = %#v, want 2 launch groups", groups)
	}
	if groups[0].PipelineID != "team/dashboard-sample" || groups[0].RunScope != "prod" || len(groups[0].Sources) != 2 {
		t.Fatalf("first group = %#v, want two prod sources for team/dashboard-sample", groups[0])
	}
	if groups[1].PipelineID != "team/dashboard-sample" || groups[1].RunScope != "dev" || len(groups[1].Sources) != 1 {
		t.Fatalf("second group = %#v, want one dev source for team/dashboard-sample", groups[1])
	}
}

func TestGroupDashboardRefreshLaunchSourcesUsesRequestScopeOverride(t *testing.T) {
	plans := []dashboardRefreshSourcePlan{
		{
			Source: dashboardSourceRecord{ID: "s1", PipelineID: "team/dashboard-sample", RunScope: "prod"},
			Launch: true,
		},
		{
			Source: dashboardSourceRecord{ID: "s2", PipelineID: "team/dashboard-sample", RunScope: "dev"},
			Launch: true,
		},
	}

	groups := groupDashboardRefreshLaunchSources(dashboardRefreshInput{RunScope: "/staging/"}, plans)
	if len(groups) != 1 {
		t.Fatalf("groups = %#v, want one launch group after run scope override", groups)
	}
	if groups[0].RunScope != "staging" || len(groups[0].Sources) != 2 {
		t.Fatalf("override group = %#v, want two staging sources", groups[0])
	}
}

func TestDashboardRefreshRunStatusFromPipelineStatus(t *testing.T) {
	tests := map[string]string{
		"success":           "",
		"failure":           dashboardRefreshRunStatusFailed,
		"failure (ignored)": dashboardRefreshRunStatusFailed,
		"rejected":          dashboardRefreshRunStatusFailed,
		"cancelled":         dashboardRefreshRunStatusCancelled,
		"timed_out":         dashboardRefreshRunStatusTimedOut,
		"skipped":           dashboardRefreshRunStatusSkipped,
		"pending":           "",
		"waiting_approval":  "",
	}
	for status, want := range tests {
		if got := dashboardRefreshRunStatusFromPipelineStatus(status); got != want {
			t.Fatalf("status %q => %q, want %q", status, got, want)
		}
	}
}

func TestDashboardRefreshRunStatusFromPipelineOutputStatus(t *testing.T) {
	tests := []struct {
		name             string
		runStatus        string
		outputStatus     string
		outputError      string
		runFinishedStale bool
		wantStatus       string
		wantMessage      string
	}{
		{
			name:         "pending output keeps dashboard row running",
			runStatus:    "success",
			outputStatus: finalOutputStatusPending,
			wantStatus:   dashboardRefreshRunStatusRunning,
		},
		{
			name:         "generating output keeps dashboard row running",
			runStatus:    "success",
			outputStatus: finalOutputStatusRunning,
			wantStatus:   dashboardRefreshRunStatusRunning,
		},
		{
			name:         "successful output completes dashboard row",
			runStatus:    "success",
			outputStatus: finalOutputStatusSuccess,
			wantStatus:   dashboardRefreshRunStatusSuccess,
		},
		{
			name:         "failed output fails dashboard row with output error",
			runStatus:    "success",
			outputStatus: finalOutputStatusFailure,
			outputError:  "invalid DashboardSpec",
			wantStatus:   dashboardRefreshRunStatusFailed,
			wantMessage:  "invalid DashboardSpec",
		},
		{
			name:         "cancelled output cancels dashboard row",
			runStatus:    "success",
			outputStatus: finalOutputStatusCancelled,
			wantStatus:   dashboardRefreshRunStatusCancelled,
			wantMessage:  "dashboard output generation cancelled",
		},
		{
			name:             "stale successful run without output fails dashboard row",
			runStatus:        "success",
			runFinishedStale: true,
			wantStatus:       dashboardRefreshRunStatusFailed,
			wantMessage:      "pipeline run completed without producing dashboard output",
		},
		{
			name:        "fresh successful run without output waits for final output preparation",
			runStatus:   "success",
			wantStatus:  "",
			wantMessage: "",
		},
		{
			name:        "pipeline failure fails dashboard row",
			runStatus:   "failure",
			wantStatus:  dashboardRefreshRunStatusFailed,
			wantMessage: "pipeline run failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotStatus, gotMessage := dashboardRefreshRunStatusFromPipelineOutputStatus(tt.runStatus, tt.outputStatus, tt.outputError, tt.runFinishedStale)
			if gotStatus != tt.wantStatus || gotMessage != tt.wantMessage {
				t.Fatalf("status/message = %q/%q, want %q/%q", gotStatus, gotMessage, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}
