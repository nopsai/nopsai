package nopsai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

func TestSampleConfigDashboardPublicationTargetsOpsDashboard(t *testing.T) {
	pipelineContent := readSampleConfigFile(t, "team-1-repo", "pipelines", "dashboard-sample.yaml")
	dashboardContent := readSampleConfigFile(t, "team-1-repo", "dashboards", "ops-dashboard.yaml")

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineContent), &pipeline); err != nil {
		t.Fatalf("yaml.Unmarshal(sample pipeline) error = %v", err)
	}
	if err := validatePipeline(&pipeline); err != nil {
		t.Fatalf("validatePipeline(sample pipeline) error = %v", err)
	}
	if pipeline.Name != "dashboard-sample" {
		t.Fatalf("sample pipeline name = %q, want dashboard-sample", pipeline.Name)
	}
	if len(pipeline.Output.Items) != 1 {
		t.Fatalf("sample pipeline output items = %d, want 1", len(pipeline.Output.Items))
	}
	output := pipeline.Output.Items[0]
	if output.Name != "Service metrics" || output.Type != "dashboard" {
		t.Fatalf("sample dashboard output = %q/%q, want Service metrics/dashboard", output.Name, output.Type)
	}
	if output.Dashboard.Ref != "team-1/ops-dashboard" ||
		output.Dashboard.Section != "service-metrics" ||
		output.Dashboard.EntryKey != "dashboard-sample" {
		t.Fatalf("sample dashboard target = %+v, want team-1/ops-dashboard service-metrics dashboard-sample", output.Dashboard)
	}
	if !strings.Contains(output.Prompt, "If a visualization is not specified") {
		t.Fatalf("sample prompt should delegate unspecified visualization choice to NopsAI:\n%s", output.Prompt)
	}

	dashboards, err := parseGitOpsDashboards(
		map[string]string{"dashboards/ops-dashboard.yaml": dashboardContent},
		"dashboards",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "team-1"},
		"team-1",
	)
	if err != nil {
		t.Fatalf("parseGitOpsDashboards(sample dashboard) error = %v", err)
	}
	dashboard, ok := dashboards[dashboardResourceID("team-1", "ops-dashboard")]
	if !ok {
		t.Fatalf("sample dashboard was not parsed as team-1/ops-dashboard: %#v", dashboards)
	}
	if dashboard.input.Title != "Ops Dashboard" {
		t.Fatalf("sample dashboard title = %q, want Ops Dashboard", dashboard.input.Title)
	}
	if len(dashboard.input.Sections) != 1 || dashboard.input.Sections[0].SectionKey != "service-metrics" {
		t.Fatalf("sample dashboard sections = %#v, want service-metrics", dashboard.input.Sections)
	}
	if len(dashboard.sources) != 1 {
		t.Fatalf("sample dashboard sources = %d, want 1", len(dashboard.sources))
	}
	source := dashboard.sources[0]
	if source.SectionKey != "service-metrics" ||
		source.PipelineID != "team-1/dashboard-sample" ||
		source.OutputName != "Service metrics" ||
		source.EntryKey != "dashboard-sample" {
		t.Fatalf("sample dashboard source = %+v, want service-metrics team-1/dashboard-sample Service metrics dashboard-sample", source)
	}
}

func readSampleConfigFile(t *testing.T, parts ...string) string {
	t.Helper()
	pathParts := append([]string{"..", "..", "doc", "sample-config-repo"}, parts...)
	content, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("read sample config file %v: %v", parts, err)
	}
	return string(content)
}
