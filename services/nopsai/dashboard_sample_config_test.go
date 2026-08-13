package nopsai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"nopsai/pkg/models"
)

func TestQuickstartSampleDashboardPublicationIsBound(t *testing.T) {
	pipelineContent := readQuickstartSampleFile(t, "team-repo", "pipelines", "platform", "service-health-dashboard.yaml")
	dashboardContent := readQuickstartSampleFile(t, "team-repo", "dashboards", "platform", "service-health.yaml")

	var pipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(pipelineContent), &pipeline); err != nil {
		t.Fatalf("yaml.Unmarshal(sample pipeline) error = %v", err)
	}
	if err := validatePipeline(&pipeline); err != nil {
		t.Fatalf("validatePipeline(sample pipeline) error = %v", err)
	}
	if pipeline.Name != "service-health-dashboard" {
		t.Fatalf("sample pipeline name = %q, want service-health-dashboard", pipeline.Name)
	}
	if len(pipeline.Output.Items) != 1 {
		t.Fatalf("sample pipeline output items = %d, want 1", len(pipeline.Output.Items))
	}
	output := pipeline.Output.Items[0]
	if output.Name != "Service health" || output.Type != "dashboard" || output.When != "success" {
		t.Fatalf("sample dashboard output = %+v, want Service health/dashboard/success", output)
	}
	if output.Dashboard.Ref != "platform/service-health" ||
		output.Dashboard.Section != "service-health" ||
		output.Dashboard.EntryKey != "service-health" ||
		output.Dashboard.Mode != "replace" {
		t.Fatalf("sample dashboard target = %+v, want platform/service-health service-health replace", output.Dashboard)
	}
	if !strings.Contains(pipelineContent, "dashboard_evidence=") {
		t.Fatalf("sample pipeline should emit structured dashboard_evidence JSON:\n%s", pipelineContent)
	}
	for _, step := range pipeline.Steps {
		if strings.TrimSpace(step.GetScript()) == "" {
			t.Fatalf("sample dashboard step %q should be an executable script step", step.GetName())
		}
	}
	if len(pipeline.Variables) != 0 {
		t.Fatalf("sample dashboard pipeline should run without runtime input: %#v", pipeline.Variables)
	}

	dashboards, err := parseGitOpsDashboards(
		map[string]string{"dashboards/platform/service-health.yaml": dashboardContent},
		"dashboards",
		models.ConfigRepository{ScopeType: models.ConfigRepositoryScopeTeam, ScopeID: "platform"},
		"platform",
	)
	if err != nil {
		t.Fatalf("parseGitOpsDashboards(sample dashboard) error = %v", err)
	}
	dashboard, ok := dashboards[dashboardResourceID("platform", "service-health")]
	if !ok {
		t.Fatalf("sample dashboard was not parsed as platform/service-health: %#v", dashboards)
	}
	if dashboard.input.Title != "Service Health" {
		t.Fatalf("sample dashboard title = %q, want Service Health", dashboard.input.Title)
	}
	if len(dashboard.input.Sections) != 1 || dashboard.input.Sections[0].SectionKey != "service-health" {
		t.Fatalf("sample dashboard sections = %#v, want a single service-health section", dashboard.input.Sections)
	}
	if len(dashboard.sources) != 1 {
		t.Fatalf("sample dashboard sources = %d, want 1", len(dashboard.sources))
	}
	source := dashboard.sources[0]
	if source.SectionKey != output.Dashboard.Section ||
		source.PipelineID != "platform/service-health-dashboard" ||
		source.OutputName != output.Name ||
		source.EntryKey != output.Dashboard.EntryKey ||
		!source.Enabled ||
		!source.RequiredForRefresh {
		t.Fatalf("sample dashboard source = %+v, want the service-health-dashboard output binding", source)
	}
}

func readQuickstartSampleFile(t *testing.T, parts ...string) string {
	t.Helper()
	pathParts := append([]string{"..", "..", "examples", "gitops-quickstart"}, parts...)
	content, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("read quickstart sample file %v: %v", parts, err)
	}
	return string(content)
}
