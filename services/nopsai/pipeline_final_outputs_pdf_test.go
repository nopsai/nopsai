package nopsai

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGotenbergPDFConverterPostsHTMLAndValidatesPDF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/chromium/convert/html" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm() error = %v", err)
		}
		file, _, err := r.FormFile("files")
		if err != nil {
			t.Errorf("FormFile() error = %v", err)
		} else {
			defer file.Close()
			payload, _ := io.ReadAll(file)
			if !bytes.Contains(payload, []byte("<h1>Report</h1>")) {
				t.Errorf("uploaded HTML = %s", payload)
			}
		}
		if r.FormValue("printBackground") != "true" || r.FormValue("preferCssPageSize") != "true" {
			t.Errorf("render options = %#v", r.MultipartForm.Value)
		}
		if r.FormValue("generateDocumentOutline") != "true" || r.FormValue("generateTaggedPdf") != "true" {
			t.Errorf("accessibility options = %#v", r.MultipartForm.Value)
		}
		if files := r.MultipartForm.File["files"]; len(files) != 2 || files[1].Filename != "footer.html" {
			t.Errorf("uploaded files = %#v", files)
		}
		if r.Header.Get("Gotenberg-Output-Filename") != "report" {
			t.Errorf("output filename = %q", r.Header.Get("Gotenberg-Output-Filename"))
		}
		fmt.Fprint(w, "%PDF-1.7\nrendered")
	}))
	defer server.Close()
	converter, err := newGotenbergPDFConverter(server.URL, server.Client())
	if err != nil {
		t.Fatalf("newGotenbergPDFConverter() error = %v", err)
	}
	payload, err := converter.ConvertHTML(t.Context(), []byte("<h1>Report</h1>"), "report.pdf")
	if err != nil || !bytes.HasPrefix(payload, []byte("%PDF-")) {
		t.Fatalf("ConvertHTML() payload = %q error = %v", payload, err)
	}
}

func TestPipelineFinalOutputPDFIntegration(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("GOTENBERG_TEST_URL"))
	if baseURL == "" {
		t.Skip("GOTENBERG_TEST_URL is not set")
	}
	content := `{"version":"1","title":"Production Readiness Report","subtitle":"Release 2026.06","metadata":[{"label":"Run","value":"run-123"},{"label":"Environment","value":"production"}],"sections":[{"title":"Executive Summary","blocks":[{"type":"paragraph","text":"The release completed successfully. All required controls passed and no blocking incidents were detected."},{"type":"callout","tone":"success","title":"Decision","text":"Ready for production traffic."}]},{"title":"Control Results","blocks":[{"type":"table","table":{"columns":["Control","Owner","Status","Evidence"],"rows":[["Build integrity","Platform","Passed","Signed image and reproducible build"],["Security scan","Security","Passed","No critical or high findings"],["Deployment health","Operations","Passed","All services healthy across three zones"],["Rollback","Release","Passed","Rollback completed in staging within four minutes"]]}},{"type":"bullet_list","items":["Error budget remains above target.","Monitoring alerts are active.","On-call handoff is complete."]}]}]}`
	documentHTML, err := renderPipelineFinalOutputDocumentHTML("Report", content)
	if err != nil {
		t.Fatalf("render document HTML: %v", err)
	}
	converter, err := newGotenbergPDFConverter(baseURL, &http.Client{})
	if err != nil {
		t.Fatalf("new Gotenberg converter: %v", err)
	}
	payload, err := converter.ConvertHTML(t.Context(), documentHTML, "production-readiness.pdf")
	if err != nil {
		t.Fatalf("convert document: %v", err)
	}
	if outputPath := strings.TrimSpace(os.Getenv("FINAL_OUTPUT_PDF_QA_PATH")); outputPath != "" {
		if err := os.WriteFile(outputPath, payload, 0o600); err != nil {
			t.Fatalf("write QA PDF: %v", err)
		}
	}
}

func TestGotenbergPDFConverterRejectsInvalidConfigurationAndResponses(t *testing.T) {
	for _, raw := range []string{"", "file:///tmp/pdf", "https://user:secret@example.com"} {
		if _, err := newGotenbergPDFConverter(raw, nil); err == nil {
			t.Fatalf("newGotenbergPDFConverter(%q) expected error", raw)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "busy")
	}))
	defer server.Close()
	converter, _ := newGotenbergPDFConverter(server.URL, server.Client())
	if _, err := converter.ConvertHTML(t.Context(), []byte("html"), "report.pdf"); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("ConvertHTML() error = %v, want 503", err)
	}
}
