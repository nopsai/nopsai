package nopsai

import (
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestParseGitOpsDataManagementPlan(t *testing.T) {
	plan, err := parseGitOpsDataManagementPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeSystem, ScopeID: models.ConfigRepositorySystemGlobalID},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/data-management.yaml": `
cleanup_schedules:
  - name: Weekly logs
    description: Trim old log rows
    enabled: false
    target: logs
    mode: older_than_days
    older_than_days: 14
    backup_before_cleanup: true
    cron_expression: "0 3 * * 0"
    timezone: Europe/Amsterdam
`,
			},
		},
	)
	if err != nil {
		t.Fatalf("parseGitOpsDataManagementPlan() error = %v", err)
	}
	if plan == nil {
		t.Fatal("parseGitOpsDataManagementPlan() = nil, want plan")
	}
	schedule, ok := plan.schedules["Weekly logs"]
	if !ok {
		t.Fatalf("schedules = %#v, want Weekly logs", plan.schedules)
	}
	if schedule.input.Enabled || schedule.input.Plan.Target != dataCleanupTargetLogs || schedule.input.Plan.OlderThanDays != 14 {
		t.Fatalf("schedule input = %#v, want disabled logs older_than_days", schedule.input)
	}
	if schedule.sourcePath != "setting/system/data-management.yaml" || plan.sourcePath != "setting/system/data-management.yaml" {
		t.Fatalf("source paths = (%q, %q), want data management path", schedule.sourcePath, plan.sourcePath)
	}
}

func TestParseGitOpsDataManagementPlanRejectsNonSystemRepo(t *testing.T) {
	_, err := parseGitOpsDataManagementPlan(
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		gitOpsRuntimeSettingsDirectory{
			root: "setting",
			files: map[string]string{
				"setting/system/data-management.yaml": "cleanup_schedules: []",
			},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "system config repository") {
		t.Fatalf("expected system-scope error, got %v", err)
	}
}

func TestParseGitOpsDataManagementPlanRejectsDuplicateSchedules(t *testing.T) {
	_, err := parseGitOpsDataManagementFile(`
cleanup_schedules:
  - name: Weekly cleanup
    target: runs
    mode: keep_last
    keep_last: 30
    cron_expression: "0 2 * * 0"
  - name: Weekly cleanup
    target: logs
    mode: older_than_days
    older_than_days: 30
    cron_expression: "0 3 * * 0"
`, "setting/system/data-management.yaml")
	if err == nil || !strings.Contains(err.Error(), "duplicate data cleanup schedule") {
		t.Fatalf("expected duplicate schedule error, got %v", err)
	}
}

func TestParseGitOpsDataManagementPlanRequiresScheduleName(t *testing.T) {
	_, err := parseGitOpsDataManagementFile(`
cleanup_schedules:
  - target: runs
    mode: keep_last
    keep_last: 30
    cron_expression: "0 2 * * 0"
`, "setting/system/data-management.yaml")
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("expected missing name error, got %v", err)
	}
}

func TestBuildDataManagementGitOpsFile(t *testing.T) {
	doc := buildDataManagementGitOpsFile([]dataCleanupScheduleRecord{
		{Name: "B", Enabled: true, Target: dataCleanupTargetRuns, Mode: dataCleanupModeKeepLast, KeepLast: 10, BackupBeforeCleanup: true, CronExpression: "0 2 * * 0", Timezone: "UTC"},
		{Name: "A", Enabled: false, Target: dataCleanupTargetLogs, Mode: dataCleanupModeOlderThanDays, OlderThanDays: 7, BackupBeforeCleanup: false, CronExpression: "0 3 * * 0", Timezone: "Europe/Amsterdam"},
	})
	if len(doc.CleanupSchedules) != 2 || doc.CleanupSchedules[0].Name != "A" || doc.CleanupSchedules[1].Name != "B" {
		t.Fatalf("cleanup schedules = %#v, want sorted schedules", doc.CleanupSchedules)
	}
	if doc.CleanupSchedules[0].Enabled == nil || *doc.CleanupSchedules[0].Enabled {
		t.Fatalf("enabled pointer = %#v, want explicit false", doc.CleanupSchedules[0].Enabled)
	}
	if doc.CleanupSchedules[0].BackupBeforeCleanup == nil || *doc.CleanupSchedules[0].BackupBeforeCleanup {
		t.Fatalf("backup pointer = %#v, want explicit false", doc.CleanupSchedules[0].BackupBeforeCleanup)
	}
}

func TestDataManagementSchemaAddsConfigProvenance(t *testing.T) {
	joined := strings.Join(dataManagementSchemaStatements, "\n")
	for _, want := range []string{
		"ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS source",
		"ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS config_repo_id",
		"ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS config_source_path",
		"ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS config_source_commit_sha",
		"ALTER TABLE data_cleanup_schedules ADD COLUMN IF NOT EXISTS managed_by_config_repo",
		"CREATE INDEX IF NOT EXISTS idx_data_cleanup_schedules_config_repo",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("data management schema missing %q in:\n%s", want, joined)
		}
	}
}
