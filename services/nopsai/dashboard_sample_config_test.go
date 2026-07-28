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
	pipelineContent := readSampleConfigFile(t, "team-1-repo", "pipelines", "team-1", "dashboard-sample.yaml")
	multiOutputPipelineContent := readSampleConfigFile(t, "team-1-repo", "pipelines", "team-1", "dashboard-multi-output-sample.yaml")
	dashboardContent := readSampleConfigFile(t, "team-1-repo", "dashboards", "team-1", "ops-dashboard.yaml")

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
	if multiOutputPipeline.Name != "dashboard-sample" {
		t.Fatalf("multi-output sample pipeline name = %q, want dashboard-sample", multiOutputPipeline.Name)
	}
	if len(multiOutputPipeline.Output.Items) != 10 {
		t.Fatalf("multi-output sample pipeline output items = %d, want 10", len(multiOutputPipeline.Output.Items))
	}
	modes := map[string]bool{}
	presets := map[string]bool{}
	outputNames := map[string]bool{}
	outputPrompts := map[string]string{}
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
		outputPrompts[item.Name] = item.Prompt
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
	for _, outputName := range []string{"Readiness donut chart", "Boolean readiness matrix"} {
		if !outputNames[outputName] {
			t.Fatalf("multi-output sample missing dashboard output %q; got %#v", outputName, outputNames)
		}
	}
	if !strings.Contains(multiOutputPipelineContent, "donut chart") ||
		!strings.Contains(multiOutputPipelineContent, "pie chart") ||
		!strings.Contains(multiOutputPipelineContent, "boolean status rendering") {
		t.Fatalf("multi-output sample should exercise circular charts and boolean status rendering:\n%s", multiOutputPipelineContent)
	}
	for _, outputName := range []string{"Build duration metrics", "Build timeline"} {
		prompt := outputPrompts[outputName]
		if !strings.Contains(prompt, "mode: series") ||
			!strings.Contains(prompt, "chart or series block") {
			t.Fatalf("%s prompt should require chart-compatible series publication:\n%s", outputName, prompt)
		}
	}
	releaseReportPrompt := outputPrompts["Release report"]
	for _, want := range []string{
		"executive summary",
		"built artifacts",
		"Git changes",
		"production blockers",
		"supporting evidence after",
		"do not make a table the primary layout",
	} {
		if !strings.Contains(releaseReportPrompt, want) {
			t.Fatalf("release report prompt missing %q:\n%s", want, releaseReportPrompt)
		}
	}

	dashboards, err := parseGitOpsDashboards(
		map[string]string{"dashboards/team-1/ops-dashboard.yaml": dashboardContent},
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
	for _, section := range []string{
		"service-metrics",
		"technical-readiness",
		"image-builds",
		"release-readiness",
		"build-timeline",
		"operations-digest",
		"customer-success",
		"finance-operations",
		"people-operations",
	} {
		if !sections[section] {
			t.Fatalf("sample dashboard sections = %#v, missing %s", dashboard.input.Sections, section)
		}
	}
	if len(dashboard.sources) != 16 {
		t.Fatalf("sample dashboard sources = %d, want 16", len(dashboard.sources))
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
		if source.PipelineID != "team-1/dashboard-sample" {
			t.Fatalf("source for %q = %+v, want team-1/dashboard-sample", outputName, source)
		}
	}
}

func TestSampleConfigPurposeDashboardPipelinesAreExecutableAndBound(t *testing.T) {
	type expectedPurposePipeline struct {
		file       string
		name       string
		outputName string
		section    string
		entryKey   string
		mode       string
		preset     string
		technical  bool
	}

	expectedPipelines := []expectedPurposePipeline{
		{
			file:       "technical-api-readiness.yaml",
			name:       "technical-api-readiness",
			outputName: "API readiness dashboard",
			section:    "technical-readiness",
			entryKey:   "api-release-readiness",
			mode:       "replace",
			preset:     "status",
			technical:  true,
		},
		{
			file:       "technical-slo-burn-rate.yaml",
			name:       "technical-slo-burn-rate",
			outputName: "SLO burn-rate dashboard",
			section:    "technical-readiness",
			entryKey:   "slo-burn-rate",
			mode:       "series",
			preset:     "metrics",
			technical:  true,
		},
		{
			file:       "customer-onboarding-pulse.yaml",
			name:       "customer-onboarding-pulse",
			outputName: "Customer onboarding dashboard",
			section:    "customer-success",
			entryKey:   "enterprise-onboarding",
			mode:       "replace",
			preset:     "mixed",
		},
		{
			file:       "finance-close-snapshot.yaml",
			name:       "finance-close-snapshot",
			outputName: "Finance close dashboard",
			section:    "finance-operations",
			entryKey:   "month-end-close",
			mode:       "append",
			preset:     "report",
		},
		{
			file:       "people-capacity-plan.yaml",
			name:       "people-capacity-plan",
			outputName: "People capacity dashboard",
			section:    "people-operations",
			entryKey:   "capacity-plan",
			mode:       "replace",
			preset:     "comparison",
		},
	}

	dashboardContent := readSampleConfigFile(t, "team-1-repo", "dashboards", "team-1", "ops-dashboard.yaml")
	dashboards, err := parseGitOpsDashboards(
		map[string]string{"dashboards/team-1/ops-dashboard.yaml": dashboardContent},
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
	sections := map[string]bool{}
	for _, section := range dashboard.input.Sections {
		sections[section.SectionKey] = true
	}
	sources := map[string]dashboardSourceInput{}
	for _, source := range dashboard.sources {
		sources[source.PipelineID+"\x00"+source.OutputName] = source
	}

	technicalCount := 0
	businessCount := 0
	for _, tt := range expectedPipelines {
		if tt.technical {
			technicalCount++
		} else {
			businessCount++
		}
		if !sections[tt.section] {
			t.Fatalf("purpose pipeline %q section %q is not declared in ops dashboard", tt.name, tt.section)
		}

		content := readSampleConfigFile(t, "team-1-repo", "pipelines", "team-1", tt.file)
		var pipeline models.Pipeline
		if err := yaml.Unmarshal([]byte(content), &pipeline); err != nil {
			t.Fatalf("yaml.Unmarshal(%s) error = %v", tt.file, err)
		}
		if err := validatePipeline(&pipeline); err != nil {
			t.Fatalf("validatePipeline(%s) error = %v", tt.file, err)
		}
		if pipeline.Name != tt.name {
			t.Fatalf("%s name = %q, want %q", tt.file, pipeline.Name, tt.name)
		}
		if pipeline.ContainerImage != "alpine:3.20" {
			t.Fatalf("%s container_image = %q, want alpine:3.20", tt.file, pipeline.ContainerImage)
		}
		if pipeline.WorkingDirectory != "/workspace" {
			t.Fatalf("%s working_directory = %q, want /workspace", tt.file, pipeline.WorkingDirectory)
		}
		if len(pipeline.Variables) != 0 {
			t.Fatalf("%s declares variables but should be executable without runtime input: %#v", tt.file, pipeline.Variables)
		}
		if len(pipeline.MCPProfiles) != 0 {
			t.Fatalf("%s declares MCP profiles but should run without external MCP dependencies: %#v", tt.file, pipeline.MCPProfiles)
		}
		if len(pipeline.Output.Items) != 1 {
			t.Fatalf("%s output items = %d, want 1", tt.file, len(pipeline.Output.Items))
		}
		output := pipeline.Output.Items[0]
		if output.Name != tt.outputName || output.Type != "dashboard" || output.When != "success" {
			t.Fatalf("%s output = %+v, want %q/dashboard/success", tt.file, output, tt.outputName)
		}
		if output.Dashboard.Ref != "team-1/ops-dashboard" ||
			output.Dashboard.Section != tt.section ||
			output.Dashboard.EntryKey != tt.entryKey ||
			output.Dashboard.Mode != tt.mode ||
			output.Dashboard.Preset != tt.preset {
			t.Fatalf("%s dashboard target = %+v, want section=%s entry_key=%s mode=%s preset=%s", tt.file, output.Dashboard, tt.section, tt.entryKey, tt.mode, tt.preset)
		}
		if !strings.Contains(output.Prompt, "dashboard_evidence") {
			t.Fatalf("%s output prompt should ground dashboard content in dashboard_evidence:\n%s", tt.file, output.Prompt)
		}
		if !strings.Contains(content, "dashboard_evidence={") {
			t.Fatalf("%s should emit structured dashboard_evidence JSON:\n%s", tt.file, content)
		}
		if tt.mode == "series" && !strings.Contains(output.Prompt, "mode: series") {
			t.Fatalf("%s series output prompt should require series-compatible dashboard output:\n%s", tt.file, output.Prompt)
		}
		for _, step := range pipeline.Steps {
			if strings.TrimSpace(step.GetScript()) == "" {
				t.Fatalf("%s step %q should be an executable script step", tt.file, step.GetName())
			}
			if strings.TrimSpace(step.GetGoal()) != "" {
				t.Fatalf("%s step %q should not require goal execution", tt.file, step.GetName())
			}
			if _, ok := step.AsApprovalStep(); ok {
				t.Fatalf("%s step %q should not require approval", tt.file, step.GetName())
			}
			if len(step.GetSecrets()) != 0 {
				t.Fatalf("%s step %q declares secrets but should run now: %#v", tt.file, step.GetName(), step.GetSecrets())
			}
			if len(step.GetMCPProfiles()) != 0 {
				t.Fatalf("%s step %q declares MCP profiles but should run now: %#v", tt.file, step.GetName(), step.GetMCPProfiles())
			}
			if len(step.GetVariables()) != 0 {
				t.Fatalf("%s step %q declares variables but should run now: %#v", tt.file, step.GetName(), step.GetVariables())
			}
		}

		source, ok := sources["team-1/"+tt.name+"\x00"+tt.outputName]
		if !ok {
			t.Fatalf("ops dashboard missing source binding for %s/%s", tt.name, tt.outputName)
		}
		if source.SectionKey != tt.section ||
			source.EntryKey != tt.entryKey ||
			!source.Enabled ||
			!source.RequiredForRefresh {
			t.Fatalf("ops dashboard source for %s/%s = %+v, want section=%s entry_key=%s enabled and required", tt.name, tt.outputName, source, tt.section, tt.entryKey)
		}
	}

	if technicalCount != 2 || businessCount != 3 {
		t.Fatalf("purpose pipeline mix = %d technical/%d business, want 2 technical/3 business", technicalCount, businessCount)
	}
}

func readSampleConfigFile(t *testing.T, parts ...string) string {
	t.Helper()
	pathParts := append([]string{"..", "..", "examples", "sample-config-repo"}, parts...)
	content, err := os.ReadFile(filepath.Join(pathParts...))
	if err != nil {
		t.Fatalf("read sample config file %v: %v", parts, err)
	}
	return string(content)
}
