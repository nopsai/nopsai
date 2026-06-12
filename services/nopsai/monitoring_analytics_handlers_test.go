package nopsai

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseMonitoringAnalyticsFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/monitoring/summary?from=2026-06-01&to=2026-06-11&groupId=42&pipelinePath=platform&pipelineName=release&repo=acme/app&branch=main&triggerSource=schedule&status=failure&compare=previous_period&minDurationSeconds=5&maxDurationSeconds=60", nil)

	filters, err := parseMonitoringAnalyticsFilters(req)
	if err != nil {
		t.Fatalf("parseMonitoringAnalyticsFilters() error = %v", err)
	}
	if filters.GroupID == nil || *filters.GroupID != 42 {
		t.Fatalf("GroupID = %#v, want 42", filters.GroupID)
	}
	if filters.PipelinePath != "platform" || filters.PipelineName != "release" {
		t.Fatalf("pipeline filters = %q/%q, want platform/release", filters.PipelinePath, filters.PipelineName)
	}
	if filters.Repo != "acme/app" || filters.Ref != "refs/heads/main" {
		t.Fatalf("repo/ref = %q/%q, want acme/app refs/heads/main", filters.Repo, filters.Ref)
	}
	if !filters.ComparePreviousPeriod {
		t.Fatal("ComparePreviousPeriod = false, want true")
	}
	if filters.MinDurationSeconds == nil || *filters.MinDurationSeconds != 5 || filters.MaxDurationSeconds == nil || *filters.MaxDurationSeconds != 60 {
		t.Fatalf("duration filters = %#v/%#v, want 5/60", filters.MinDurationSeconds, filters.MaxDurationSeconds)
	}
}

func TestBuildMonitoringCandidateRunIDsQueryUsesParameterizedFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/monitoring/summary?from=2026-06-01T00:00:00Z&to=2026-06-11T00:00:00Z&groupId=42&repo=acme/app&externalTriggerId=deploy&scheduleId=00000000-0000-0000-0000-000000000001", nil)
	filters, err := parseMonitoringAnalyticsFilters(req)
	if err != nil {
		t.Fatalf("parseMonitoringAnalyticsFilters() error = %v", err)
	}

	query, args := buildMonitoringCandidateRunIDsQuery(filters)
	for _, fragment := range []string{"WITH RECURSIVE selected_groups", "pr.group_id IN (SELECT id FROM selected_groups)", "LOWER(COALESCE(eti.trigger_id, '')) = LOWER($5)", "pr.schedule_id::text = $6"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing %q:\n%s", fragment, query)
		}
	}
	if len(args) != 6 {
		t.Fatalf("args len = %d, want 6 (%#v)", len(args), args)
	}
}

func TestMonitoringAIUsageQueriesCastTokenSumsForIntegerScans(t *testing.T) {
	queries := map[string]string{
		"totals":         monitoringAIUsageTotalsQuery(),
		"by pipeline":    monitoringAIUsageByPipelineQuery(),
		"top token runs": monitoringAITopTokenRunsQuery(),
		"by feature":     monitoringAIUsageGroupQuery("feature"),
		"trend":          monitoringAIUsageTrendQuery(),
	}
	for name, query := range queries {
		if !strings.Contains(query, "COALESCE(SUM(total_tokens), 0)::bigint") {
			t.Fatalf("%s query does not cast total token sums to bigint:\n%s", name, query)
		}
	}
	for _, fragment := range []string{
		"COALESCE(SUM(prompt_tokens), 0)::bigint",
		"COALESCE(SUM(completion_tokens), 0)::bigint",
	} {
		if !strings.Contains(monitoringAIUsageTotalsQuery(), fragment) {
			t.Fatalf("totals query missing %q:\n%s", fragment, monitoringAIUsageTotalsQuery())
		}
	}
}

func TestMonitoringAIUsageTotalsQuerySplitsExactAndEstimatedTokens(t *testing.T) {
	query := monitoringAIUsageTotalsQuery()
	for _, fragment := range []string{
		"metadata->>'estimated_tokens'",
		"SUM(total_tokens) FILTER (WHERE NOT",
		"SUM(total_tokens) FILTER (WHERE",
		"COUNT(*) FILTER (WHERE NOT",
		"COUNT(*) FILTER (WHERE",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("totals query missing %q:\n%s", fragment, query)
		}
	}
}

func TestMonitoringAITopTokenRunsQueryOrdersBySelectedColumns(t *testing.T) {
	query := monitoringAITopTokenRunsQuery()
	if strings.Contains(query, "ORDER BY 6") {
		t.Fatalf("query orders by an unselected column:\n%s", query)
	}
	if !strings.Contains(query, "ORDER BY 4 DESC, 3 DESC") {
		t.Fatalf("query should order by tokens then events:\n%s", query)
	}
}

func TestMonitoringEfficiencyRecommendations(t *testing.T) {
	recommendations := monitoringEfficiencyRecommendations(monitoringEfficiencyResponse{
		TotalAITokens: 4200,
		TokenHeavyLowSuccessPipelines: []monitoringPerformanceRow{{
			Key:         "platform/release",
			SuccessRate: 0.38,
			TotalRuns:   8,
		}},
		HighQueueGroups: []monitoringNamedCount{{Label: "Platform", Seconds: 420}},
		TokenByPipeline: []monitoringNamedCount{{Label: "platform/release", Tokens: 4200}},
	})
	if len(recommendations) != 3 {
		t.Fatalf("recommendations = %#v, want three", recommendations)
	}
	if !strings.Contains(recommendations[0], "platform/release") {
		t.Fatalf("first recommendation = %q, want pipeline name", recommendations[0])
	}
}

func TestMonitoringAlertComparatorMatched(t *testing.T) {
	tests := []struct {
		comparator string
		value      float64
		threshold  float64
		want       bool
	}{
		{comparator: "gt", value: 2, threshold: 1, want: true},
		{comparator: "gte", value: 2, threshold: 2, want: true},
		{comparator: "lt", value: 1, threshold: 2, want: true},
		{comparator: "lte", value: 2, threshold: 2, want: true},
		{comparator: "eq", value: 2.0000001, threshold: 2.0000001, want: true},
		{comparator: "unknown", value: 2, threshold: 1, want: false},
	}
	for _, tt := range tests {
		if got := monitoringAlertComparatorMatched(tt.comparator, tt.value, tt.threshold); got != tt.want {
			t.Fatalf("monitoringAlertComparatorMatched(%q, %v, %v) = %v, want %v", tt.comparator, tt.value, tt.threshold, got, tt.want)
		}
	}
}

func TestMonitoringAlertRuleFilters(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	filters, err := monitoringAlertRuleFilters(monitoringAlertRuleRecord{
		WindowSeconds: 3600,
		Filters: map[string]any{
			"groupId":            float64(42),
			"pipelinePath":       "platform",
			"pipelineName":       "release",
			"repo":               "acme/app",
			"branch":             "main",
			"triggerSource":      "external",
			"status":             "failure",
			"minDurationSeconds": 5,
		},
	}, now)
	if err != nil {
		t.Fatalf("monitoringAlertRuleFilters() error = %v", err)
	}
	if !filters.From.Equal(now.Add(-time.Hour)) || !filters.To.Equal(now) {
		t.Fatalf("window = %s/%s, want previous hour", filters.From, filters.To)
	}
	if filters.GroupID == nil || *filters.GroupID != 42 {
		t.Fatalf("GroupID = %#v, want 42", filters.GroupID)
	}
	if filters.PipelinePath != "platform" || filters.PipelineName != "release" || filters.Repo != "acme/app" {
		t.Fatalf("filters = %#v", filters)
	}
	if filters.Ref != "refs/heads/main" {
		t.Fatalf("Ref = %q, want refs/heads/main", filters.Ref)
	}
	if filters.MinDurationSeconds == nil || *filters.MinDurationSeconds != 5 {
		t.Fatalf("MinDurationSeconds = %#v, want 5", filters.MinDurationSeconds)
	}
}

func TestMonitoringRecommendationFingerprintStable(t *testing.T) {
	first := monitoringRecommendationFingerprint("efficiency", "Pipeline release is token-heavy.")
	second := monitoringRecommendationFingerprint("efficiency", "Pipeline release is token-heavy.")
	third := monitoringRecommendationFingerprint("reliability", "Pipeline release is token-heavy.")
	if first == "" || first != second {
		t.Fatalf("fingerprint not stable: %q %q", first, second)
	}
	if first == third {
		t.Fatal("fingerprint should include category")
	}
}
