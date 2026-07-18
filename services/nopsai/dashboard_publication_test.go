package nopsai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"nopsai/pkg/models"
)

func TestNormalizeDashboardPublicationTargetDefaultsAndValidates(t *testing.T) {
	output := pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{
			ID:   "run-output-1",
			Name: "deployment-summary",
			Type: "dashboard",
		},
		Dashboard: models.DashboardOutputTarget{
			Ref:     " platform/engineering-health ",
			Section: "deployments",
			Mode:    "",
			TTL:     "7d",
		},
	}

	target, err := normalizeDashboardPublicationTarget(output)
	if err != nil {
		t.Fatalf("normalizeDashboardPublicationTarget() error = %v", err)
	}
	if target.Ref != "platform/engineering-health" {
		t.Fatalf("ref = %q", target.Ref)
	}
	if target.EntryKey != "deployment-summary" {
		t.Fatalf("entry key = %q, want output name", target.EntryKey)
	}
	if target.Mode != dashboardPublishModeReplace {
		t.Fatalf("mode = %q, want replace", target.Mode)
	}
	if target.ExpiresAt == nil || time.Until(*target.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("expires_at = %#v, want roughly seven days in the future", target.ExpiresAt)
	}
}

func TestNormalizeDashboardPublicationTargetRejectsUnsafeInputs(t *testing.T) {
	tests := []struct {
		name    string
		target  models.DashboardOutputTarget
		message string
	}{
		{
			name:    "missing ref",
			target:  models.DashboardOutputTarget{Section: "overview"},
			message: "dashboard.ref is required",
		},
		{
			name:    "invalid section",
			target:  models.DashboardOutputTarget{Ref: "platform/health", Section: "bad section"},
			message: "dashboard.section is invalid",
		},
		{
			name:    "invalid entry key",
			target:  models.DashboardOutputTarget{Ref: "platform/health", Section: "overview", EntryKey: "bad entry"},
			message: "dashboard.entry_key is invalid",
		},
		{
			name:    "bad ttl",
			target:  models.DashboardOutputTarget{Ref: "platform/health", Section: "overview", EntryKey: "api", TTL: "0d"},
			message: "dashboard.ttl must be a positive duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeDashboardPublicationTarget(pipelineFinalOutputRecord{
				PipelineRunFinalOutput: models.PipelineRunFinalOutput{Name: "summary", Type: "dashboard"},
				Dashboard:              tt.target,
			})
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("error = %v, want %q", err, tt.message)
			}
		})
	}
}

func TestDashboardTargetFromOutputItemNormalizesPublicationFields(t *testing.T) {
	target := dashboardTargetFromOutputItem(models.PipelineOutputItem{
		Name: "Security overview",
		Dashboard: models.DashboardOutputTarget{
			Ref:      "security/security-overview",
			Section:  "findings",
			EntryKey: "dependency-scan",
			Mode:     dashboardPublishModeAppend,
			Preset:   "table",
			TTL:      "90d",
		},
	})

	if target.Ref != "security/security-overview" ||
		target.Section != "findings" ||
		target.EntryKey != "dependency-scan" ||
		target.Mode != dashboardPublishModeAppend ||
		target.Preset != "table" ||
		target.TTL != "90d" {
		t.Fatalf("target = %#v", target)
	}
}

func TestNormalizeDashboardPublishModeSupportsSnapshot(t *testing.T) {
	if got := normalizeDashboardPublishMode(" snapshot "); got != dashboardPublishModeSnapshot {
		t.Fatalf("mode = %q, want snapshot", got)
	}
}

func TestNormalizeDashboardPublishModeSupportsSeries(t *testing.T) {
	if got := normalizeDashboardPublishMode(" series "); got != dashboardPublishModeSeries {
		t.Fatalf("mode = %q, want series", got)
	}
}

func TestMergeDashboardSeriesSpecDedupesRetainsAndCarriesSeries(t *testing.T) {
	existing := dashboardSeriesSpec("Ops", "Latency", []models.DashboardChartSeries{
		{
			Key:   "api",
			Label: "API",
			Points: []models.DashboardSeriesPoint{
				dashboardPoint("2026-07-15T10:00:00Z", "", 120),
				dashboardPoint("2026-07-15T10:05:00Z", "", 125),
			},
		},
		{
			Key:    "worker",
			Label:  "Worker",
			Points: []models.DashboardSeriesPoint{dashboardPoint("2026-07-15T10:00:00Z", "", 95)},
		},
	})
	existingContent, err := marshalFinalOutputSpec(existing)
	if err != nil {
		t.Fatalf("marshal existing spec: %v", err)
	}
	incoming := dashboardSeriesSpec("Ops", "Latency", []models.DashboardChartSeries{
		{
			Key:   "api",
			Label: "API",
			Points: []models.DashboardSeriesPoint{
				dashboardPoint("2026-07-15T10:05:00Z", "", 130),
				dashboardPoint("2026-07-15T10:10:00Z", "", 140),
			},
		},
	})

	merged, err := mergeDashboardSeriesSpec(existingContent, incoming)
	if err != nil {
		t.Fatalf("mergeDashboardSeriesSpec() error = %v", err)
	}
	if len(merged.Blocks) != 1 || merged.Blocks[0].Chart == nil {
		t.Fatalf("merged blocks = %#v", merged.Blocks)
	}
	series := merged.Blocks[0].Chart.Series
	if len(series) != 2 {
		t.Fatalf("series count = %d, want carried existing series", len(series))
	}
	api := seriesByKey(series, "api")
	if api == nil {
		t.Fatal("merged API series missing")
	}
	if len(api.Points) != 3 {
		t.Fatalf("api points = %d, want deduped merged points", len(api.Points))
	}
	if got := valueOfPoint(api.Points[1]); got != 130 {
		t.Fatalf("deduped point value = %v, want incoming overwrite 130", got)
	}
	if seriesByKey(series, "worker") == nil {
		t.Fatal("existing worker series was not carried forward")
	}
}

func TestMergeDashboardSeriesSpecCollapsesBuildDurationAliases(t *testing.T) {
	existing := dashboardSeriesSpec("Ops", "Build Duration by Image", []models.DashboardChartSeries{
		{
			Key:    "build_duration_seconds",
			Label:  "Build Duration",
			Points: []models.DashboardSeriesPoint{dashboardPoint("", "nopsai-dashboard", 1)},
		},
		{
			Key:    "duration",
			Label:  "Seconds",
			Points: []models.DashboardSeriesPoint{dashboardPoint("", "seed-static", 2)},
		},
	})
	existingContent, err := marshalFinalOutputSpec(existing)
	if err != nil {
		t.Fatalf("marshal existing spec: %v", err)
	}
	incoming := dashboardSeriesSpec("Ops", "Build Duration by Image", []models.DashboardChartSeries{
		{
			Key:   "duration",
			Label: "Seconds",
			Points: []models.DashboardSeriesPoint{
				dashboardPoint("", "nopsai-dashboard", 24),
				dashboardPoint("", "git-sample", 55),
				dashboardPoint("", "app-finance", 60),
				dashboardPoint("", "seed-static", 12),
			},
		},
	})

	merged, err := mergeDashboardSeriesSpec(existingContent, incoming)
	if err != nil {
		t.Fatalf("mergeDashboardSeriesSpec() error = %v", err)
	}
	series := merged.Blocks[0].Chart.Series
	if len(series) != 1 {
		t.Fatalf("series count = %d, want alias series collapsed: %#v", len(series), series)
	}
	if series[0].Key != "duration" {
		t.Fatalf("series key = %q, want incoming key", series[0].Key)
	}
	if len(series[0].Points) != 4 ||
		valueOfPointByLabel(series[0].Points, "nopsai-dashboard") != 24 ||
		valueOfPointByLabel(series[0].Points, "git-sample") != 55 ||
		valueOfPointByLabel(series[0].Points, "app-finance") != 60 ||
		valueOfPointByLabel(series[0].Points, "seed-static") != 12 {
		t.Fatalf("points = %#v", series[0].Points)
	}
}

func TestTrimDashboardSeriesPointsRetainsLatestSortedPoints(t *testing.T) {
	points := make([]models.DashboardSeriesPoint, 0, dashboardSeriesRetentionPoints+5)
	for index := 0; index < dashboardSeriesRetentionPoints+5; index++ {
		points = append(points, dashboardPoint("", fmt.Sprintf("point-%04d", index), float64(index)))
	}
	trimmed := trimDashboardSeriesPoints(points)
	if len(trimmed) != dashboardSeriesRetentionPoints {
		t.Fatalf("trimmed points = %d, want %d", len(trimmed), dashboardSeriesRetentionPoints)
	}
	if got := valueOfPoint(trimmed[0]); got != 5 {
		t.Fatalf("first retained value = %v, want oldest five trimmed", got)
	}
}

func TestArchiveDashboardPublicationEntryArchivesCurrentRowAndWritesRemovedEvent(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	runner := &archivePublicationRunner{
		record: dashboardPublicationRecord{
			ID:          "publication-1",
			DashboardID: "dashboard-1",
			SectionKey:  "overview",
			EntryKey:    "service-health",
			Mode:        dashboardPublishModeReplace,
			Content:     json.RawMessage(`{"title":"Service Health"}`),
			Revision:    3,
			RunID:       "00000000-0000-0000-0000-000000000010",
			RunOutputID: "00000000-0000-0000-0000-000000000020",
			PipelineID:  "platform/service-health",
			OutputName:  "Service Health",
			RunScope:    "prod",
			RefreshID:   "00000000-0000-0000-0000-000000000030",
			PublishedAt: now,
			Status:      "archived",
			CreatedAt:   now.Add(-time.Hour),
			UpdatedAt:   now,
		},
	}

	publication, err := archiveDashboardPublicationEntry(context.Background(), runner, "dashboard-1", " publication-1 ")
	if err != nil {
		t.Fatalf("archiveDashboardPublicationEntry() error = %v", err)
	}
	if publication.ID != "publication-1" || publication.Status != "archived" {
		t.Fatalf("publication = %#v", publication)
	}
	if !strings.Contains(runner.query, "SET status = 'archived'") {
		t.Fatalf("query did not archive publication: %s", runner.query)
	}
	if len(runner.queryArgs) != 2 || runner.queryArgs[0] != "dashboard-1" || runner.queryArgs[1] != "publication-1" {
		t.Fatalf("query args = %#v", runner.queryArgs)
	}
	if len(runner.execArgs) < 9 {
		t.Fatalf("exec args = %#v", runner.execArgs)
	}
	if runner.execArgs[1] != "overview" || runner.execArgs[2] != "service-health" || runner.execArgs[5] != "removed" {
		t.Fatalf("publication event args = %#v", runner.execArgs)
	}
	var eventContent map[string]any
	if err := json.Unmarshal([]byte(runner.execArgs[6].(string)), &eventContent); err != nil {
		t.Fatalf("event content JSON = %v", err)
	}
	if eventContent["removed_publication_id"] != "publication-1" || eventContent["pipeline_id"] != "platform/service-health" {
		t.Fatalf("event content = %#v", eventContent)
	}
}

func TestArchiveDashboardPublicationEntryRejectsEmptyPublicationID(t *testing.T) {
	_, err := archiveDashboardPublicationEntry(context.Background(), &archivePublicationRunner{}, "dashboard-1", " ")
	if !dashboardNotFound(err) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func dashboardSeriesSpec(title, blockTitle string, series []models.DashboardChartSeries) models.DashboardSpec {
	return models.DashboardSpec{
		Version: models.FinalOutputSpecVersion,
		Title:   title,
		Blocks: []models.DashboardBlock{
			{
				Type:  "series",
				Title: blockTitle,
				Chart: &models.DashboardChart{
					Type:                "line",
					AggregationInterval: "5m",
					MissingValues:       "gap",
					TimeWindow:          &models.DashboardTimeWindow{From: "now-1h", To: "now"},
					Dimensions:          map[string]string{"team": "platform", "environment": "prod"},
					Series:              series,
				},
			},
		},
	}
}

func dashboardPoint(timestamp, label string, value float64) models.DashboardSeriesPoint {
	return models.DashboardSeriesPoint{
		Timestamp: timestamp,
		Label:     label,
		Value:     &value,
	}
}

func seriesByKey(series []models.DashboardChartSeries, key string) *models.DashboardChartSeries {
	for index := range series {
		if series[index].Key == key {
			return &series[index]
		}
	}
	return nil
}

func valueOfPoint(point models.DashboardSeriesPoint) float64 {
	if point.Value == nil {
		return 0
	}
	return *point.Value
}

func valueOfPointByLabel(points []models.DashboardSeriesPoint, label string) float64 {
	for _, point := range points {
		if point.Label == label {
			return valueOfPoint(point)
		}
	}
	return 0
}

type archivePublicationRunner struct {
	record    dashboardPublicationRecord
	query     string
	queryArgs []any
	execArgs  []any
}

func (r *archivePublicationRunner) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	r.execArgs = append([]any(nil), args...)
	return pgconn.NewCommandTag("INSERT 1"), nil
}

func (r *archivePublicationRunner) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("unexpected query")
}

func (r *archivePublicationRunner) QueryRow(_ context.Context, query string, args ...any) pgx.Row {
	r.query = query
	r.queryArgs = append([]any(nil), args...)
	return archivePublicationRow{record: r.record}
}

type archivePublicationRow struct {
	record dashboardPublicationRecord
}

func (r archivePublicationRow) Scan(dest ...any) error {
	if len(dest) != 20 {
		return fmt.Errorf("scan destination count = %d, want 20", len(dest))
	}
	content := string(r.record.Content)
	var sourceFinishedAt, expiresAt sql.NullTime
	if r.record.SourceFinishedAt != nil {
		sourceFinishedAt.Valid = true
		sourceFinishedAt.Time = *r.record.SourceFinishedAt
	}
	if r.record.ExpiresAt != nil {
		expiresAt.Valid = true
		expiresAt.Time = *r.record.ExpiresAt
	}
	*(dest[0].(*string)) = r.record.ID
	*(dest[1].(*string)) = r.record.DashboardID
	*(dest[2].(*string)) = r.record.SectionKey
	*(dest[3].(*string)) = r.record.EntryKey
	*(dest[4].(*string)) = r.record.Mode
	*(dest[5].(*string)) = content
	*(dest[6].(*int)) = r.record.Revision
	*(dest[7].(*string)) = r.record.RunID
	*(dest[8].(*string)) = r.record.RunOutputID
	*(dest[9].(*string)) = r.record.PipelineID
	*(dest[10].(*string)) = r.record.OutputName
	*(dest[11].(*string)) = r.record.RunScope
	*(dest[12].(*string)) = r.record.RefreshID
	*(dest[13].(*sql.NullTime)) = sourceFinishedAt
	*(dest[14].(*time.Time)) = r.record.PublishedAt
	*(dest[15].(*sql.NullTime)) = expiresAt
	*(dest[16].(*string)) = r.record.Status
	*(dest[17].(*bool)) = r.record.Stale
	*(dest[18].(*time.Time)) = r.record.CreatedAt
	*(dest[19].(*time.Time)) = r.record.UpdatedAt
	return nil
}
