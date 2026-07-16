package nopsai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nopsai/config"
	"nopsai/pkg/llmclient"
	"nopsai/pkg/models"
)

func TestNormalizePipelineFinalOutputContentValidatesJSON(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("json", "<final_output>\n```json\n{\"ok\": true}\n```\n</final_output>")
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if content != `{"ok": true}` {
		t.Fatalf("content = %q, want JSON without fence", content)
	}

	if _, err := normalizePipelineFinalOutputContent("json", "<final_output>{not-json</final_output>"); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestNormalizePipelineFinalOutputContentValidatesDocumentSpec(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("html", `<final_output>{"version":"1","title":"Report","sections":[{"title":"Summary","blocks":[{"type":"paragraph","text":"Everything passed."}]}]}</final_output>`)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if !strings.Contains(content, `"title": "Report"`) || strings.Contains(content, "<h1") {
		t.Fatalf("normalized DocumentSpec = %s", content)
	}
}

func TestNormalizePipelineFinalOutputContentValidatesDashboardSpec(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("dashboard", `<final_output>{"version":"1","title":"Ops Dashboard","blocks":[{"type":"status","label":"Health","status":"success","value":"Green"},{"type":"table","columns":[{"key":"name","label":"Name"}],"rows":[{"name":"api"}]}]}</final_output>`)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if !strings.Contains(content, `"title": "Ops Dashboard"`) || strings.Contains(content, "<table") {
		t.Fatalf("normalized DashboardSpec = %s", content)
	}

	if _, err := normalizePipelineFinalOutputContent("dashboard", `<final_output>{"version":"1","title":"Bad","blocks":[{"type":"link","href":"javascript:alert(1)","text":"Bad"}]}</final_output>`); err == nil {
		t.Fatal("expected unsafe dashboard link to fail")
	}
}

func TestNormalizePipelineFinalOutputContentValidatesDashboardChartSpec(t *testing.T) {
	spec := dashboardSeriesSpec("Ops Dashboard", "Latency", []models.DashboardChartSeries{
		{
			Key:         "api.p95",
			Label:       "API p95",
			Team:        "platform",
			Environment: "prod",
			Color:       "#2563eb",
			Points: []models.DashboardSeriesPoint{
				dashboardPoint("2026-07-15T10:00:00Z", "", 120),
				dashboardPoint("2026-07-15T10:05:00Z", "", 130),
			},
		},
	})
	raw, err := marshalFinalOutputSpec(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	content, err := normalizePipelineFinalOutputContent("dashboard", "<final_output>"+raw+"</final_output>")
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() chart error = %v", err)
	}
	if !strings.Contains(content, `"type": "series"`) || !strings.Contains(content, `"aggregation_interval": "5m"`) {
		t.Fatalf("normalized chart DashboardSpec = %s", content)
	}

	badScript := strings.Replace(raw, "API p95", "<script>alert(1)</script>", 1)
	if _, err := normalizePipelineFinalOutputContent("dashboard", "<final_output>"+badScript+"</final_output>"); err == nil {
		t.Fatal("expected executable chart content to fail")
	}

	badPieSeries := strings.Replace(raw, `"type": "line"`, `"type": "pie"`, 1)
	if _, err := normalizePipelineFinalOutputContent("dashboard", "<final_output>"+badPieSeries+"</final_output>"); err == nil {
		t.Fatal("expected pie series block to fail")
	}
}

func TestResolvePipelineFinalOutputProfileNameUsesItemOutputPipelineDefault(t *testing.T) {
	pipeline := models.Pipeline{
		LLMProfile: "pipeline-profile",
		Output:     models.PipelineOutput{LLMProfile: "output-profile"},
	}
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{LLMProfile: "item-profile"}); got != "item-profile" {
		t.Fatalf("item profile = %q, want item-profile", got)
	}
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{}); got != "output-profile" {
		t.Fatalf("output profile = %q, want output-profile", got)
	}
	pipeline.Output.LLMProfile = ""
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{}); got != "pipeline-profile" {
		t.Fatalf("pipeline profile = %q, want pipeline-profile", got)
	}
	pipeline.LLMProfile = ""
	if got := resolvePipelineFinalOutputProfileName("default", pipeline, models.PipelineOutputItem{}); got != "default" {
		t.Fatalf("default profile = %q, want default", got)
	}
}

func TestPipelineFinalOutputMatchesRunStatus(t *testing.T) {
	tests := []struct {
		name      string
		when      string
		runStatus string
		want      bool
	}{
		{name: "default always on success", runStatus: "success", want: true},
		{name: "default always on failure", runStatus: "failure", want: true},
		{name: "success matches success", when: "success", runStatus: "success", want: true},
		{name: "success skips failure", when: "success", runStatus: "failure", want: false},
		{name: "failure skips success", when: "failure", runStatus: "success", want: false},
		{name: "failure matches rejected", when: "failure", runStatus: "rejected", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pipelineFinalOutputMatchesRunStatus(tt.when, tt.runStatus); got != tt.want {
				t.Fatalf("pipelineFinalOutputMatchesRunStatus(%q, %q) = %v, want %v", tt.when, tt.runStatus, got, tt.want)
			}
		})
	}
}

func TestPipelineFinalOutputLLMClientForcesZeroTemperature(t *testing.T) {
	var temperature *float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/models":
			fmt.Fprint(w, `{"models":[{"type":"llm","key":"report-model","loaded_instances":[{"id":"report-model"}]}]}`)
		case "/api/v1/chat":
			var request struct {
				Temperature *float64 `json:"temperature"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
			}
			temperature = request.Temperature
			fmt.Fprint(w, `{"output":[{"type":"message","content":"<final_output>report</final_output>"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configuredTemperature := 0.8
	app := &App{cfg: &config.Config{
		LLMDefaultProfile: "reporting",
		LLMProfiles: map[string]config.LLMProfile{
			"reporting": {
				Provider:    config.LLMProviderLMStudio,
				BaseURL:     server.URL,
				Model:       "report-model",
				Temperature: &configuredTemperature,
			},
		},
	}}
	client, err := app.pipelineFinalOutputLLMClient(t.Context(), "reporting", "")
	if err != nil {
		t.Fatalf("pipelineFinalOutputLLMClient() error = %v", err)
	}
	if _, err := client.CompleteWithSystem(t.Context(), pipelineFinalOutputSystemInstruction, "Build report."); err != nil {
		t.Fatalf("CompleteWithSystem() error = %v", err)
	}
	if temperature == nil || *temperature != 0 {
		t.Fatalf("temperature = %#v, want 0", temperature)
	}
}

func TestPipelineFinalOutputAttemptUsageReportsIncludesRejectedRetry(t *testing.T) {
	output := pipelineFinalOutputRecord{
		PipelineRunFinalOutput: models.PipelineRunFinalOutput{
			ID:   "output-1",
			Name: "Executive summary",
			Type: "markdown",
		},
	}
	result := pipelineFinalOutputGenerationResult{
		Attempts: []pipelineFinalOutputGenerationAttempt{
			{
				Completion: llmclient.Completion{Usage: llmclient.Usage{
					Provider:         "openai",
					Model:            "gpt-test",
					Profile:          "reporting",
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				}},
			},
			{
				Completion: llmclient.Completion{Usage: llmclient.Usage{
					Provider:     "openai",
					Model:        "gpt-test",
					Profile:      "reporting",
					PromptTokens: 12,
					TotalTokens:  18,
				}},
				ContractValid: true,
			},
			{},
		},
	}
	reports := pipelineFinalOutputAttemptUsageReports(output, result)
	if len(reports) != 2 {
		t.Fatalf("reports = %#v", reports)
	}
	if reports[0].Metadata["attempt"] != 1 ||
		reports[0].Metadata["retry"] != false ||
		reports[0].Metadata["contract_valid"] != false ||
		reports[1].Metadata["attempt"] != 2 ||
		reports[1].Metadata["retry"] != true ||
		reports[1].Metadata["contract_valid"] != true {
		t.Fatalf("reports = %#v", reports)
	}
}

func TestPipelineFinalOutputSchemaIncludesGenerationAndRenderAuditConstraints(t *testing.T) {
	combined := strings.Join(pipelineFinalOutputSchemaStatements, "\n")
	for _, expected := range []string{
		"generation_attempts INTEGER NOT NULL DEFAULT 0",
		"contract_violations INTEGER NOT NULL DEFAULT 0",
		"pipeline_run_outputs_generation_audit_check",
		"contract_violations <= generation_attempts",
		"render_attempts INTEGER NOT NULL DEFAULT 0",
		"render_failures INTEGER NOT NULL DEFAULT 0",
		"pipeline_run_outputs_render_audit_check",
		"render_failures <= render_attempts",
	} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("schema statements missing %q", expected)
		}
	}
}

func TestRenderPipelineFinalOutputDownloadFormats(t *testing.T) {
	pdfConverter := &recordingPDFConverter{payload: []byte("%PDF-1.7\nrendered")}
	pdfBytes, pdfType, pdfName, err := renderPipelineFinalOutputDownload(t.Context(), models.PipelineRunFinalOutput{
		Name:    "Comparison Report",
		Type:    "pdf",
		Status:  finalOutputStatusSuccess,
		Content: `{"version":"1","title":"Security Issue Details","sections":[{"title":"Findings","blocks":[{"type":"table","table":{"columns":["Severity","Issue Type"],"rows":[["Critical","Disclosure"]]}},{"type":"paragraph","text":"Everything passed."}]}]}`,
	}, pdfConverter)
	if err != nil {
		t.Fatalf("render pdf error = %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.7")) {
		t.Fatalf("pdf bytes missing header: %q", string(pdfBytes[:8]))
	}
	if !bytes.Contains(pdfConverter.documentHTML, []byte("<table>")) || !bytes.Contains(pdfConverter.documentHTML, []byte("Disclosure")) {
		t.Fatalf("PDF HTML does not contain a real table: %s", pdfConverter.documentHTML)
	}
	if pdfType != "application/pdf" || pdfName != "comparison-report.pdf" {
		t.Fatalf("pdf metadata = %q %q", pdfType, pdfName)
	}

	xlsxBytes, xlsxType, xlsxName, err := renderPipelineFinalOutputDownload(t.Context(), models.PipelineRunFinalOutput{
		Name:    "Data Table",
		Type:    "excel",
		Status:  finalOutputStatusSuccess,
		Content: `{"version":"1","title":"Data","sheets":[{"name":"Results","columns":[{"key":"name","header":"Name","width":24,"number_format":"text"},{"key":"value","header":"Value","number_format":"integer"}],"rows":[{"name":"API","value":42}],"freeze_header":true,"auto_filter":true}]}`,
	}, nil)
	if err != nil {
		t.Fatalf("render xlsx error = %v", err)
	}
	if xlsxType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || xlsxName != "data-table.xlsx" {
		t.Fatalf("xlsx metadata = %q %q", xlsxType, xlsxName)
	}
	if !bytes.HasPrefix(xlsxBytes, []byte("PK")) {
		t.Fatal("xlsx archive header missing")
	}
}

func TestRenderPipelineFinalOutputDownloadRejectsUnreadyOutput(t *testing.T) {
	_, _, _, err := renderPipelineFinalOutputDownload(t.Context(), models.PipelineRunFinalOutput{
		Name:    "Summary",
		Type:    "markdown",
		Status:  finalOutputStatusRunning,
		Content: "still running",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("render error = %v, want not ready", err)
	}
}

type recordingPDFConverter struct {
	documentHTML []byte
	payload      []byte
}

func (c *recordingPDFConverter) ConvertHTML(_ context.Context, documentHTML []byte, _ string) ([]byte, error) {
	c.documentHTML = append([]byte(nil), documentHTML...)
	return c.payload, nil
}
