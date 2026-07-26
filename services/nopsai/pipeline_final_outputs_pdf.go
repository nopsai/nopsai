package nopsai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxRenderedPDFBytes = 64 << 20

const pipelineFinalOutputPDFFooter = `<!doctype html>
<html><head><meta charset="utf-8"><style>
* { box-sizing: border-box; }
html { color: #64748b; font-family: Arial, sans-serif; font-size: 9px; }
body { margin: 0; padding: 0 16mm; width: 100%; }
.footer { border-top: 1px solid #cbd5e1; display: flex; justify-content: space-between; padding-top: 5px; width: 100%; }
</style></head><body><div class="footer"><span>NopsAI final output</span><span>Page <span class="pageNumber"></span> of <span class="totalPages"></span></span></div></body></html>`

type gotenbergPDFConverter struct {
	baseURL    string
	httpClient *http.Client
}

func (a *App) pipelineFinalOutputPDFConverter() (pipelinePDFConverter, error) {
	if a == nil || a.cfg == nil {
		return nil, fmt.Errorf("PDF renderer configuration is unavailable")
	}
	timeoutSeconds := a.cfg.FinalOutputPDFTimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 45
	}
	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	if a.httpClient != nil {
		client.Transport = a.httpClient.Transport
		client.CheckRedirect = a.httpClient.CheckRedirect
	}
	return newGotenbergPDFConverter(a.cfg.FinalOutputPDFRendererURL, client)
}

func newGotenbergPDFConverter(baseURL string, httpClient *http.Client) (*gotenbergPDFConverter, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("final_output_pdf_renderer_url is not configured")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("final_output_pdf_renderer_url must be an HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("final_output_pdf_renderer_url must not contain credentials, query parameters, or fragments")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &gotenbergPDFConverter{baseURL: baseURL, httpClient: httpClient}, nil
}

func (c *gotenbergPDFConverter) ConvertHTML(ctx context.Context, documentHTML []byte, filename string) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range []struct {
		name    string
		payload []byte
	}{
		{name: "index.html", payload: documentHTML},
		{name: "footer.html", payload: []byte(pipelineFinalOutputPDFFooter)},
	} {
		part, err := writer.CreateFormFile("files", file.name)
		if err != nil {
			return nil, fmt.Errorf("create Gotenberg %s form: %w", file.name, err)
		}
		if _, err := part.Write(file.payload); err != nil {
			return nil, fmt.Errorf("write Gotenberg %s form: %w", file.name, err)
		}
	}
	for key, value := range map[string]string{
		"printBackground":         "true",
		"preferCssPageSize":       "true",
		"generateDocumentOutline": "true",
		"generateTaggedPdf":       "true",
	} {
		if err := writer.WriteField(key, value); err != nil {
			return nil, fmt.Errorf("write Gotenberg option %s: %w", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close Gotenberg form: %w", err)
	}

	// #nosec G704 -- baseURL is deployment-owned, validated as HTTP(S), and never comes from a pipeline or request.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/forms/chromium/convert/html", &body)
	if err != nil {
		return nil, fmt.Errorf("create Gotenberg request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Gotenberg-Output-Filename", strings.TrimSuffix(filename, ".pdf"))
	// #nosec G704 -- the request target is the validated deployment-owned Gotenberg endpoint above.
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call Gotenberg: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("gotenberg returned %d: %s", response.StatusCode, strings.TrimSpace(string(message)))
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxRenderedPDFBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Gotenberg response: %w", err)
	}
	if len(payload) > maxRenderedPDFBytes {
		return nil, fmt.Errorf("gotenberg response exceeds %d bytes", maxRenderedPDFBytes)
	}
	if !bytes.HasPrefix(payload, []byte("%PDF-")) {
		return nil, fmt.Errorf("gotenberg response is not a PDF")
	}
	return payload, nil
}
