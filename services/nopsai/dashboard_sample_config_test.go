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
	multiOutputPipelineContent := readSampleConfigFile(t, "team-1-repo", "pipelines", "dashboard-multi-output-sample.yaml")
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

	var multiOutputPipeline models.Pipeline
	if err := yaml.Unmarshal([]byte(multiOutputPipelineContent), &multiOutputPipeline); err != nil {
		t.Fatalf("yaml.Unmarshal(multi-output sample pipeline) error = %v", err)
	}
	if err := validatePipeline(&multiOutputPipeline); err != nil {
		t.Fatalf("validatePipeline(multi-output sample pipeline) error = %v", err)
	}
	if multiOutputPipeline.Name != "dashboard-multi-output-sample" {
		t.Fatalf("multi-output sample pipeline name = %q, want dashboard-multi-output-sample", multiOutputPipeline.Name)
	}
	if len(multiOutputPipeline.Output.Items) != 8 {
		t.Fatalf("multi-output sample pipeline output items = %d, want 8", len(multiOutputPipeline.Output.Items))
	}
	modes := map[string]bool{}
	presets := map[string]bool{}
	outputNames := map[string]bool{}
	for _, item := range multiOutputPipeline.Output.Items {
		if item.Type != "dashboard" {
			t.Fatalf("multi-output item %q type = %q, want dashboard", item.Name, item.Type)
		}
		if item.Dashboard.Ref != "team-1/ops-dashboard" {
			t.Fatalf("multi-output item %q dashboard ref = %q, want team-1/ops-dashboard", item.Name, item.Dashboard.Ref)
		}
		modes[item.Dashboard.Mode] = true
		presets[item.Dashboard.Preset] = true
		outputNames[item.Name] = true
	}
	for _, mode := range []string{"replace", "append", "snapshot", "series"} {
		if !modes[mode] {
			t.Fatalf("multi-output sample missing dashboard mode %q; got %#v", mode, modes)
		}
	}
	for _, preset := range []string{"auto", "report", "table", "status", "timeline", "comparison", "metrics", "mixed"} {
		if !presets[preset] {
			t.Fatalf("multi-output sample missing dashboard preset %q; got %#v", preset, presets)
		}
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
	sections := map[string]bool{}
	for _, section := range dashboard.input.Sections {
		sections[section.SectionKey] = true
	}
	for _, section := range []string{"service-metrics", "image-builds", "release-readiness", "build-timeline", "operations-digest"} {
		if !sections[section] {
			t.Fatalf("sample dashboard sections = %#v, missing %s", dashboard.input.Sections, section)
		}
	}
	if len(dashboard.sources) != 9 {
		t.Fatalf("sample dashboard sources = %d, want 9", len(dashboard.sources))
	}
	sources := map[string]dashboardSourceInput{}
	for _, source := range dashboard.sources {
		sources[source.OutputName] = source
	}
	serviceSource := sources["Service metrics"]
	if serviceSource.SectionKey != "service-metrics" ||
		serviceSource.PipelineID != "team-1/dashboard-sample" ||
		serviceSource.EntryKey != "dashboard-sample" {
		t.Fatalf("sample dashboard source = %+v, want service-metrics team-1/dashboard-sample dashboard-sample", serviceSource)
	}
	for outputName := range outputNames {
		source := sources[outputName]
		if source.PipelineID != "team-1/dashboard-multi-output-sample" {
			t.Fatalf("source for %q = %+v, want team-1/dashboard-multi-output-sample", outputName, source)
		}
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
