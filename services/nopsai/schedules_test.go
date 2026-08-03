package nopsai

import (
	"strings"
	"testing"
	"time"

	"nopsai/pkg/models"
)

func TestScheduleWriteSQLSetsDefaultVisibility(t *testing.T) {
	if scheduleDefaultVisibility != resourceVisibilityTeam {
		t.Fatalf("scheduleDefaultVisibility = %q, want %q", scheduleDefaultVisibility, resourceVisibilityTeam)
	}

	for _, tc := range []struct {
		name   string
		query  string
		needle string
	}{
		{
			name:   "api create",
			query:  createScheduleSQL,
			needle: "source, visibility, config_source_path",
		},
		{
			name:   "gitops",
			query:  configSyncScheduleUpsertSQL,
			needle: "source, visibility, run_team_path",
		},
	} {
		query := strings.Join(strings.Fields(tc.query), " ")
		if !strings.Contains(query, tc.needle) {
			t.Fatalf("%s schedule write query does not set visibility explicitly:\n%s", tc.name, tc.query)
		}
	}

	if strings.Contains(configSyncScheduleUpsertSQL, "visibility = EXCLUDED.visibility") {
		t.Fatal("gitops schedule update should preserve existing schedule visibility")
	}
}

func TestNextScheduleRunAtUsesTimezone(t *testing.T) {
	from := time.Date(2026, time.June, 1, 9, 30, 0, 0, time.UTC)
	got, err := nextScheduleRunAt("0 12 * * *", "Europe/Amsterdam", from)
	if err != nil {
		t.Fatalf("nextScheduleRunAt() error = %v", err)
	}
	want := time.Date(2026, time.June, 1, 10, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("nextScheduleRunAt() = %s, want %s", got, want)
	}
}

func TestNextScheduleRunAtDailySlotUsesScheduleTimezone(t *testing.T) {
	tests := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{
			name: "before 02:00 Amsterdam",
			from: time.Date(2026, time.May, 31, 23, 30, 0, 0, time.UTC),
			want: time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "after 02:00 Amsterdam",
			from: time.Date(2026, time.June, 1, 0, 30, 0, 0, time.UTC),
			want: time.Date(2026, time.June, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nextScheduleRunAt("0 2 * * *", "Europe/Amsterdam", tt.from)
			if err != nil {
				t.Fatalf("nextScheduleRunAt() error = %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("nextScheduleRunAt() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestNormalizeScheduleInput(t *testing.T) {
	enabled := false
	got, err := normalizeScheduleInput(scheduleRequest{
		Path:           "prod/scheduled",
		Name:           "Nightly Deploy",
		Pipeline:       "prod/services/api/deploy",
		CronExpression: "0 2 * * *",
		Timezone:       "UTC",
		Enabled:        &enabled,
		Scope:          "default",
		RunTeamPath:    "platform/prod",
		Variables: map[string]string{
			"RELEASE_CHANNEL": "nightly",
		},
	})
	if err != nil {
		t.Fatalf("normalizeScheduleInput() error = %v", err)
	}
	if got.Path != "prod/scheduled" || got.Name != "Nightly-Deploy" {
		t.Fatalf("schedule identity = %q/%q, want prod/scheduled/Nightly-Deploy", got.Path, got.Name)
	}
	if got.PipelinePath != "prod/services/api" || got.PipelineName != "deploy" {
		t.Fatalf("pipeline = %q/%q, want prod/services/api/deploy", got.PipelinePath, got.PipelineName)
	}
	if got.Scope != "" {
		t.Fatalf("scope = %q, want default scope stored as empty", got.Scope)
	}
	if got.RunTeamPath != "platform/prod" {
		t.Fatalf("RunTeamPath = %q, want platform/prod", got.RunTeamPath)
	}
	if got.Enabled {
		t.Fatal("enabled = true, want false")
	}
	if got.Variables["RELEASE_CHANNEL"] != "nightly" {
		t.Fatalf("RELEASE_CHANNEL = %q, want nightly", got.Variables["RELEASE_CHANNEL"])
	}
}

func TestNormalizeScheduleInputTreatsGlobalAsGlobalScope(t *testing.T) {
	got, err := normalizeScheduleInput(scheduleRequest{
		Path:           "global/scheduled",
		Name:           "Global Deploy",
		Pipeline:       "global/platform/deploy",
		CronExpression: "0 2 * * *",
		Timezone:       "UTC",
		Scope:          "global/prod",
		RunTeamPath:    "global",
	})
	if err != nil {
		t.Fatalf("normalizeScheduleInput() error = %v", err)
	}
	if got.Path != "scheduled" {
		t.Fatalf("Path = %q, want scheduled", got.Path)
	}
	if got.PipelinePath != "platform" || got.PipelineName != "deploy" {
		t.Fatalf("pipeline = %q/%q, want platform/deploy", got.PipelinePath, got.PipelineName)
	}
	if got.Scope != "prod" {
		t.Fatalf("Scope = %q, want prod", got.Scope)
	}
	if got.RunTeamPath != "global" {
		t.Fatalf("RunTeamPath = %q, want global", got.RunTeamPath)
	}
}

func TestNormalizeRunTeamPathDefaultsToGlobal(t *testing.T) {
	got, err := normalizeRunTeamPath("")
	if err != nil {
		t.Fatalf("normalizeRunTeamPath() error = %v", err)
	}
	if got != "global" {
		t.Fatalf("normalizeRunTeamPath() = %q, want global", got)
	}
}

func TestNormalizeRunTeamPathRejectsRetiredGlobalAliases(t *testing.T) {
	for _, raw := range []string{"root", "general", "__general__"} {
		if _, err := normalizeRunTeamPath(raw); err == nil {
			t.Fatalf("normalizeRunTeamPath(%q) error = nil, want retired alias error", raw)
		}
	}
}

func TestNormalizeScheduleInputOneTimeUsesRunAt(t *testing.T) {
	enabled := false
	got, err := normalizeScheduleInput(scheduleRequest{
		Name:         "Production Freeze",
		Pipeline:     "prod/services/api/deploy",
		ScheduleKind: "once",
		RunAt:        "2030-03-15T09:45",
		Timezone:     "Europe/Amsterdam",
		Enabled:      &enabled,
	})
	if err != nil {
		t.Fatalf("normalizeScheduleInput() one-time error = %v", err)
	}
	if got.ScheduleKind != scheduleKindOnce {
		t.Fatalf("ScheduleKind = %q, want once", got.ScheduleKind)
	}
	if got.RunAt == nil {
		t.Fatal("RunAt is nil")
	}
	want := time.Date(2030, time.March, 15, 8, 45, 0, 0, time.UTC)
	if !got.RunAt.Equal(want) {
		t.Fatalf("RunAt = %s, want %s", got.RunAt, want)
	}
	if got.CronExpression != "" {
		t.Fatalf("CronExpression = %q, want empty for one-time schedule", got.CronExpression)
	}
}

func TestNormalizeScheduleInputRejectsEnabledPastOneTime(t *testing.T) {
	enabled := true
	_, err := normalizeScheduleInput(scheduleRequest{
		Name:         "Past",
		Pipeline:     "prod/services/api/deploy",
		ScheduleKind: "once",
		RunAt:        "2020-03-15T09:45Z",
		Timezone:     "UTC",
		Enabled:      &enabled,
	})
	if err == nil {
		t.Fatal("expected past enabled one-time schedule to fail")
	}
}

func TestParseGitOpsSchedulesNormalizesTeamScope(t *testing.T) {
	files := map[string]string{
		"config/schedules/scheduled/nightly.yaml": `
pipeline: services/api/deploy
cron_expression: "0 2 * * *"
timezone: UTC
enabled: false
scope: prod
run_team_path: prod/scheduled
`,
	}
	got, err := parseGitOpsSchedules(
		files,
		"config/schedules",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam},
		"team-1",
	)
	if err != nil {
		t.Fatalf("parseGitOpsSchedules() error = %v", err)
	}
	schedule, ok := got["team-1/scheduled/nightly"]
	if !ok {
		t.Fatalf("missing normalized schedule key, got %#v", got)
	}
	if schedule.input.PipelinePath != "team-1/services/api" || schedule.input.PipelineName != "deploy" {
		t.Fatalf("pipeline = %q/%q, want team-1/services/api/deploy", schedule.input.PipelinePath, schedule.input.PipelineName)
	}
	if schedule.input.Scope != "team-1/prod" {
		t.Fatalf("scope = %q, want team-1/prod", schedule.input.Scope)
	}
	if schedule.input.RunTeamPath != "team-1/prod/scheduled" {
		t.Fatalf("RunTeamPath = %q, want team-1/prod/scheduled", schedule.input.RunTeamPath)
	}
}

func TestParseGitOpsSchedulesKeepsGlobalRunTeamForTeamRepo(t *testing.T) {
	files := map[string]string{
		"config/schedules/scheduled/nightly.yaml": `
pipeline: global/platform/deploy
cron_expression: "0 2 * * *"
timezone: UTC
scope: global
run_team_path: global
`,
	}
	got, err := parseGitOpsSchedules(
		files,
		"config/schedules",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam},
		"team-1",
	)
	if err != nil {
		t.Fatalf("parseGitOpsSchedules() error = %v", err)
	}
	schedule, ok := got["team-1/scheduled/nightly"]
	if !ok {
		t.Fatalf("missing normalized schedule key, got %#v", got)
	}
	if schedule.input.Scope != "" {
		t.Fatalf("scope = %q, want global/default scope", schedule.input.Scope)
	}
	if schedule.input.PipelinePath != "platform" || schedule.input.PipelineName != "deploy" {
		t.Fatalf("pipeline = %q/%q, want platform/deploy", schedule.input.PipelinePath, schedule.input.PipelineName)
	}
	if schedule.input.RunTeamPath != "global" {
		t.Fatalf("RunTeamPath = %q, want global", schedule.input.RunTeamPath)
	}
}

func TestEffectiveScheduleRunTeamPathDefaultsToGlobal(t *testing.T) {
	got := effectiveScheduleRunTeamPath(scheduleRecord{
		Path: "prod/scheduled",
	})
	if got != "global" {
		t.Fatalf("effectiveScheduleRunTeamPath() = %q, want global", got)
	}

	got = effectiveScheduleRunTeamPath(scheduleRecord{
		Path:        "prod/scheduled",
		RunTeamPath: "platform/prod",
	})
	if got != "platform/prod" {
		t.Fatalf("effectiveScheduleRunTeamPath() explicit = %q, want platform/prod", got)
	}
}

func TestParseRunIDFromCreatedMessage(t *testing.T) {
	got := parseRunIDFromCreatedMessage("Pipeline run created. ID: 9f4a")
	if got != "9f4a" {
		t.Fatalf("parseRunIDFromCreatedMessage() = %q, want 9f4a", got)
	}
}
