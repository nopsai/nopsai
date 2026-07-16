package nopsai

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseMonitoringAnalyticsFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/monitoring/summary?from=2026-06-01&to=2026-06-11&teamId=42&pipelinePath=platform&pipelineName=release&repo=acme/app&runId=00000000-0000-0000-0000-000000000002&branch=main&triggerSource=schedule&status=failure&compare=previous_period&provider=openai&model=gpt-4.1&llm_profile=standard&feature=goal_resolution&step_name=plan&task_name=summarize&minDurationSeconds=5&maxDurationSeconds=60", nil)

	filters, err := parseMonitoringAnalyticsFilters(req)
	if err != nil {
		t.Fatalf("parseMonitoringAnalyticsFilters() error = %v", err)
	}
	if filters.TeamID == nil || *filters.TeamID != 42 {
		t.Fatalf("TeamID = %#v, want 42", filters.TeamID)
	}
	if filters.PipelinePath != "platform" || filters.PipelineName != "release" {
		t.Fatalf("pipeline filters = %q/%q, want platform/release", filters.PipelinePath, filters.PipelineName)
	}
	if filters.Repo != "acme/app" || filters.Ref != "refs/heads/main" {
		t.Fatalf("repo/ref = %q/%q, want acme/app refs/heads/main", filters.Repo, filters.Ref)
	}
	if filters.RunID != "00000000-0000-0000-0000-000000000002" {
		t.Fatalf("RunID = %q, want requested run ID", filters.RunID)
	}
	if !filters.ComparePreviousPeriod {
		t.Fatal("ComparePreviousPeriod = false, want true")
	}
	if filters.MinDurationSeconds == nil || *filters.MinDurationSeconds != 5 || filters.MaxDurationSeconds == nil || *filters.MaxDurationSeconds != 60 {
		t.Fatalf("duration filters = %#v/%#v, want 5/60", filters.MinDurationSeconds, filters.MaxDurationSeconds)
	}
	if filters.Provider != "openai" || filters.Model != "gpt-4.1" || filters.LLMProfile != "standard" || filters.Feature != "goal_resolution" || filters.StepName != "plan" || filters.TaskName != "summarize" {
		t.Fatalf("ai filters = provider %q model %q profile %q feature %q step %q task %q", filters.Provider, filters.Model, filters.LLMProfile, filters.Feature, filters.StepName, filters.TaskName)
	}
}

func TestParseMonitoringAnalyticsFiltersAcceptsTeamID(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/monitoring/summary?from=2026-06-01&to=2026-06-11&teamId=42", nil)

	filters, err := parseMonitoringAnalyticsFilters(req)
	if err != nil {
		t.Fatalf("parseMonitoringAnalyticsFilters() error = %v", err)
	}
	if filters.TeamID == nil || *filters.TeamID != 42 {
		t.Fatalf("TeamID = %#v, want 42", filters.TeamID)
	}
}

func TestBuildMonitoringCandidateRunIDsQueryUsesParameterizedFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/monitoring/summary?from=2026-06-01T00:00:00Z&to=2026-06-11T00:00:00Z&teamId=42&repo=acme/app&runId=00000000-0000-0000-0000-000000000002&externalTriggerId=deploy&scheduleId=00000000-0000-0000-0000-000000000001", nil)
	filters, err := parseMonitoringAnalyticsFilters(req)
	if err != nil {
		t.Fatalf("parseMonitoringAnalyticsFilters() error = %v", err)
	}

	query, args := buildMonitoringCandidateRunIDsQuery(filters)
	for _, fragment := range []string{"WITH RECURSIVE selected_teams", "pr.team_id IN (SELECT id FROM selected_teams)", "LOWER(COALESCE(pr.run_id::text, '')) = LOWER($4)", "LOWER(COALESCE(eti.trigger_id, '')) = LOWER($6)", "pr.schedule_id::text = $7"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing %q:\n%s", fragment, query)
		}
	}
	if len(args) != 7 {
		t.Fatalf("args len = %d, want 7 (%#v)", len(args), args)
	}
}

func TestBuildMonitoringCandidateRunIDsQueryAppliesAIUsageFilters(t *testing.T) {
	req := httptest.NewRequest("GET", "/v1/monitoring/summary?provider=openai&model=qwen&llmProfile=standard&feature=goal_resolution&stepName=plan&taskName=rank", nil)
	filters, err := parseMonitoringAnalyticsFilters(req)
	if err != nil {
		t.Fatalf("parseMonitoringAnalyticsFilters() error = %v", err)
	}

	query, args := buildMonitoringCandidateRunIDsQuery(filters)
	for _, fragment := range []string{
		"EXISTS (",
		"FROM ai_usage_events au",
		"LOWER(COALESCE(au.provider, '')) = LOWER($3)",
		"LOWER(COALESCE(au.model, '')) = LOWER($4)",
		"LOWER(COALESCE(au.llm_profile, '')) = LOWER($5)",
		"LOWER(COALESCE(au.feature, '')) = LOWER($6)",
		"LOWER(COALESCE(au.step_name, '')) = LOWER($7)",
		"LOWER(COALESCE(au.task_name, '')) = LOWER($8)",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing %q:\n%s", fragment, query)
		}
	}
	if len(args) != 8 {
		t.Fatalf("args len = %d, want 8 (%#v)", len(args), args)
	}
}

func TestMonitoringAIUsageQueriesCastTokenSumsForIntegerScans(t *testing.T) {
	queries := map[string]string{
		"totals":             monitoringAIUsageTotalsQuery(),
		"by pipeline":        monitoringAIUsageByPipelineQuery(),
		"by schedule":        monitoringAIUsageByScheduleQuery(false),
		"lowest by schedule": monitoringAIUsageByScheduleQuery(true),
		"by task":            monitoringAIUsageByTaskQuery(),
		"top token runs":     monitoringAITopTokenRunsQuery(),
		"by feature":         monitoringAIUsageTeamQuery("feature"),
		"by provider":        monitoringAIUsageTeamQuery("provider"),
		"by profile":         monitoringAIUsageTeamQuery("llm_profile"),
		"trend":              monitoringAIUsageTrendQuery(),
	}
	for name, query := range queries {
		if !strings.Contains(query, "COALESCE(SUM(total_tokens), 0)::bigint") &&
			!strings.Contains(query, "COALESCE(SUM(au.total_tokens), 0)::bigint") {
			t.Fatalf("%s query does not cast total token sums to bigint:\n%s", name, query)
		}
	}
	for _, fragment := range []string{
		"COALESCE(SUM(prompt_tokens), 0)::bigint",
		"COALESCE(SUM(completion_tokens), 0)::bigint",
		"LOWER(COALESCE(model, '')) = LOWER($5)",
		"LOWER(COALESCE(llm_profile, '')) = LOWER($6)",
		"LOWER(COALESCE(step_name, '')) = LOWER($8)",
	} {
		if !strings.Contains(monitoringAIUsageTotalsQuery(), fragment) {
			t.Fatalf("totals query missing %q:\n%s", fragment, monitoringAIUsageTotalsQuery())
		}
	}
}

func TestMonitoringAIUsageByScheduleQueryRanksSchedules(t *testing.T) {
	descending := monitoringAIUsageByScheduleQuery(false)
	ascending := monitoringAIUsageByScheduleQuery(true)
	for _, fragment := range []string{
		"JOIN pipeline_runs pr ON pr.run_id = au.run_id",
		"LEFT JOIN pipeline_schedules ps ON ps.id = pr.schedule_id",
		"WHERE pr.schedule_id IS NOT NULL",
		"CONCAT_WS('/', NULLIF(ps.path, ''), NULLIF(ps.name, ''))",
	} {
		if !strings.Contains(descending, fragment) {
			t.Fatalf("schedule query missing %q:\n%s", fragment, descending)
		}
	}
	if !strings.Contains(descending, "ORDER BY 4 DESC, 2") {
		t.Fatalf("descending schedule query should rank highest tokens first:\n%s", descending)
	}
	if !strings.Contains(ascending, "ORDER BY 4 ASC, 2") {
		t.Fatalf("ascending schedule query should rank lowest tokens first:\n%s", ascending)
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

func TestMonitoringAssistantChatUsageAllowedOnlyForGlobalChatCompatibleFilters(t *testing.T) {
	base := monitoringAnalyticsFilters{}
	if !monitoringAssistantChatUsageAllowed(base) {
		t.Fatal("assistant chat usage should be included for global AI usage")
	}
	if !monitoringAssistantChatUsageAllowed(monitoringAnalyticsFilters{Feature: "assistant_chat"}) {
		t.Fatal("assistant_chat feature filter should include assistant chat usage")
	}
	if monitoringAssistantChatUsageAllowed(monitoringAnalyticsFilters{Feature: "log_analysis"}) {
		t.Fatal("pipeline feature filter should not include assistant chat usage")
	}
	if monitoringAssistantChatUsageAllowed(monitoringAnalyticsFilters{RunID: "run-1"}) {
		t.Fatal("run-scoped usage should not include unrelated assistant chat")
	}
	if !monitoringAssistantChatUsageAllowed(monitoringAnalyticsFilters{Provider: "gemini"}) {
		t.Fatal("provider-scoped usage should include assistant chat when the selected profile resolves to that provider")
	}
	if !monitoringAssistantChatUsageAllowed(monitoringAnalyticsFilters{Model: "gemini-2.5-flash"}) {
		t.Fatal("model-scoped usage should include assistant chat when the selected profile resolves to that model")
	}
}

func TestMonitoringAssistantChatUsageQueriesUseStoredMessages(t *testing.T) {
	totals := monitoringAssistantChatUsageTotalsQuery()
	for _, fragment := range []string{
		"FROM assistant_messages am",
		"JOIN assistant_conversations ac ON ac.id = am.conversation_id",
		"LEFT JOIN llm_profiles lp ON LOWER(lp.name) = LOWER(ac.selected_llm_profile)",
		"COALESCE(SUM(am.total_tokens), 0)::bigint",
		"COUNT(*) FILTER (WHERE am.total_tokens > 0",
		"COUNT(*)::bigint",
		"LOWER(COALESCE(ac.selected_llm_profile, '')) = LOWER($3)",
		"LOWER(COALESCE(lp.provider, '')) = LOWER($8)",
		"LOWER(COALESCE(lp.model, '')) = LOWER($9)",
		"SPLIT_PART(ac.user_id, ':', 1)",
	} {
		if !strings.Contains(totals, fragment) {
			t.Fatalf("assistant chat totals query missing %q:\n%s", fragment, totals)
		}
	}

	team := monitoringAssistantChatUsageTeamQuery("ac.user_id")
	if !strings.Contains(team, "COUNT(*) FILTER (WHERE am.total_tokens > 0") ||
		!strings.Contains(team, "COALESCE(SUM(am.total_tokens), 0)::bigint") ||
		!strings.Contains(team, "HAVING COALESCE(SUM(am.total_tokens), 0) > 0") {
		t.Fatalf("assistant chat team query should expose message count and tokens:\n%s", team)
	}
}

func TestMonitoringAIUsageResponseAddsAssistantChatUsage(t *testing.T) {
	resp := monitoringAIUsageResponse{
		TotalPromptTokens: 100,
		TotalTokens:       150,
		ByFeature:         []monitoringNamedCount{{Key: "log_analysis", Label: "log_analysis", Count: 1, Tokens: 150}},
		ByProvider:        []monitoringNamedCount{{Key: "lmstudio", Label: "lmstudio", Count: 1, Tokens: 150}},
		ByProfile:         []monitoringNamedCount{{Key: "standard", Label: "standard", Count: 1, Tokens: 150}},
		ByModel:           []monitoringNamedCount{{Key: "lmstudio/qwen", Label: "lmstudio/qwen", Count: 1, Tokens: 150}},
		Trend:             []monitoringTimeBucket{{Key: "2026-06-20", Label: "2026-06-20", Runs: 150}},
	}

	resp.addAssistantChatUsage(monitoringAssistantChatUsage{
		PromptTokens:     25,
		CompletionTokens: 10,
		TotalTokens:      35,
		EstimatedTokens:  35,
		EstimatedEvents:  2,
		MessageCount:     2,
		ByProvider:       []monitoringNamedCount{{Key: "gemini", Label: "gemini", Count: 2, Tokens: 35}},
		ByProfile:        []monitoringNamedCount{{Key: "standard", Label: "standard", Count: 2, Tokens: 35}},
		ByModel:          []monitoringNamedCount{{Key: "gemini/gemini-2.5-flash", Label: "gemini/gemini-2.5-flash", Count: 2, Tokens: 35}},
		BySubject:        []monitoringNamedCount{{Key: "user:viewer", Label: "user:viewer", Count: 2, Tokens: 35}},
		Trend:            []monitoringTimeBucket{{Key: "2026-06-20", Label: "2026-06-20", Runs: 35}},
	})

	if resp.TotalPromptTokens != 125 || resp.TotalCompletionTokens != 10 || resp.TotalTokens != 185 {
		t.Fatalf("totals = %#v, want pipeline and assistant chat sums", resp)
	}
	if resp.AssistantChatTokens != 35 || resp.AssistantChatMessages != 2 || resp.EstimatedTokenEvents != 2 {
		t.Fatalf("assistant chat metadata = %#v", resp)
	}
	if len(resp.ByFeature) != 2 || resp.ByFeature[1].Key != "assistant_chat" {
		t.Fatalf("by_feature = %#v, want assistant_chat merged", resp.ByFeature)
	}
	if len(resp.ByProvider) != 2 || resp.ByProvider[0].Key != "lmstudio" || resp.ByProvider[1].Key != "gemini" {
		t.Fatalf("by_provider = %#v, want pipeline and assistant chat providers", resp.ByProvider)
	}
	if len(resp.ByModel) != 2 || resp.ByModel[0].Key != "lmstudio/qwen" || resp.ByModel[1].Key != "gemini/gemini-2.5-flash" {
		t.Fatalf("by_model = %#v, want pipeline and assistant chat models", resp.ByModel)
	}
	if resp.ByProfile[0].Tokens != 185 || resp.Trend[0].Runs != 185 {
		t.Fatalf("profile/trend merge failed: profiles=%#v trend=%#v", resp.ByProfile, resp.Trend)
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

func TestMonitoringAIUsageByTaskQueryFiltersTasklessEvents(t *testing.T) {
	query := monitoringAIUsageByTaskQuery()
	for _, fragment := range []string{
		"COALESCE(task_name, '') <> ''",
		"CONCAT_WS('/', NULLIF(step_name, ''), NULLIF(task_name, ''))",
		"ORDER BY 4 DESC, 2",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("task query missing %q:\n%s", fragment, query)
		}
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
		HighQueueTeams:  []monitoringNamedCount{{Label: "Platform", Seconds: 420}},
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
			"teamId":             float64(42),
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
	if filters.TeamID == nil || *filters.TeamID != 42 {
		t.Fatalf("TeamID = %#v, want 42", filters.TeamID)
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

func TestMonitoringAlertRuleFiltersAcceptsTeamID(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	filters, err := monitoringAlertRuleFilters(monitoringAlertRuleRecord{
		WindowSeconds: 3600,
		Filters: map[string]any{
			"team_id": float64(42),
		},
	}, now)
	if err != nil {
		t.Fatalf("monitoringAlertRuleFilters() error = %v", err)
	}
	if filters.TeamID == nil || *filters.TeamID != 42 {
		t.Fatalf("TeamID = %#v, want 42", filters.TeamID)
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
