package nopsai

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"nopsai/pkg/models"
)

func TestNormalizePipelineFinalOutputContentValidatesJSON(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("json", "```json\n{\"ok\": true}\n```")
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	if content != `{"ok": true}` {
		t.Fatalf("content = %q, want JSON without fence", content)
	}

	if _, err := normalizePipelineFinalOutputContent("json", "{not-json"); err == nil {
		t.Fatal("expected invalid JSON to fail")
	}
}

func TestNormalizePipelineFinalOutputContentSanitizesHTML(t *testing.T) {
	content, err := normalizePipelineFinalOutputContent("html", `<h1 onclick="steal()">Report</h1><script>alert(1)</script><a href="javascript:alert(1)" style="color:red">Open</a><iframe src="x"></iframe>`)
	if err != nil {
		t.Fatalf("normalizePipelineFinalOutputContent() error = %v", err)
	}
	for _, unsafe := range []string{"onclick", "<script", "javascript:", "style=", "<iframe"} {
		if strings.Contains(strings.ToLower(content), unsafe) {
			t.Fatalf("sanitized html still contains %q: %s", unsafe, content)
		}
	}
	if !strings.Contains(content, "<h1>Report</h1>") || !strings.Contains(content, ">Open</a>") {
		t.Fatalf("sanitized html lost expected content: %s", content)
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

func TestRenderPipelineFinalOutputDownloadFormats(t *testing.T) {
	pdfBytes, pdfType, pdfName, err := renderPipelineFinalOutputDownload(models.PipelineRunFinalOutput{
		Name:    "Comparison Report",
		Type:    "pdf",
		Status:  finalOutputStatusSuccess,
		Content: "## Security Issue Details\n\n| Severity | Issue Type |\n| --- | --- |\n| Critical | Disclosure |\n\nEverything passed.",
	})
	if err != nil {
		t.Fatalf("render pdf error = %v", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.4")) {
		t.Fatalf("pdf bytes missing header: %q", string(pdfBytes[:8]))
	}
	if bytes.Contains(pdfBytes, []byte("##")) || bytes.Contains(pdfBytes, []byte("| Severity | Issue Type |")) {
		t.Fatalf("pdf bytes still contain raw markdown: %s", string(pdfBytes))
	}
	if pdfType != "application/pdf" || pdfName != "comparison-report.pdf" {
		t.Fatalf("pdf metadata = %q %q", pdfType, pdfName)
	}

	xlsxBytes, xlsxType, xlsxName, err := renderPipelineFinalOutputDownload(models.PipelineRunFinalOutput{
		Name:    "Data Table",
		Type:    "excel",
		Status:  finalOutputStatusSuccess,
		Content: "| Name | Value |\n| --- | --- |\n| API | 42 |",
	})
	if err != nil {
		t.Fatalf("render xlsx error = %v", err)
	}
	if xlsxType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" || xlsxName != "data-table.xlsx" {
		t.Fatalf("xlsx metadata = %q %q", xlsxType, xlsxName)
	}
	reader, err := zip.NewReader(bytes.NewReader(xlsxBytes), int64(len(xlsxBytes)))
	if err != nil {
		t.Fatalf("xlsx zip error = %v", err)
	}
	var worksheetFound bool
	for _, file := range reader.File {
		if file.Name == "xl/worksheets/sheet1.xml" {
			worksheetFound = true
			break
		}
	}
	if !worksheetFound {
		t.Fatal("xlsx worksheet missing")
	}
}

func TestRenderPipelineFinalOutputDownloadRejectsUnreadyOutput(t *testing.T) {
	_, _, _, err := renderPipelineFinalOutputDownload(models.PipelineRunFinalOutput{
		Name:    "Summary",
		Type:    "markdown",
		Status:  finalOutputStatusRunning,
		Content: "still running",
	})
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("render error = %v, want not ready", err)
	}
}
