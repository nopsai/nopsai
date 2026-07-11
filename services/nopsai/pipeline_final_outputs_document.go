package nopsai

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"nopsai/pkg/models"
)

var pipelineFinalOutputDocumentTemplate = template.Must(template.New("document").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
@page { size: A4; margin: 18mm 16mm 20mm; }
* { box-sizing: border-box; }
html { color: #172033; background: #fff; font-family: Inter, "Segoe UI", Arial, sans-serif; font-size: 10.5pt; line-height: 1.5; }
body { margin: 0; }
.document-header { border-bottom: 2px solid #1d4ed8; margin-bottom: 24px; padding-bottom: 18px; }
h1 { color: #111827; font-size: 25pt; line-height: 1.14; margin: 0; }
.subtitle { color: #526076; font-size: 12pt; margin: 8px 0 0; }
.metadata { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 20px; margin: 16px 0 0; }
.metadata-item { border-left: 3px solid #bfdbfe; padding-left: 9px; }
.metadata-label { color: #64748b; display: block; font-size: 8pt; font-weight: 700; letter-spacing: .04em; text-transform: uppercase; }
.metadata-value { color: #1f2937; display: block; overflow-wrap: anywhere; }
section { break-inside: auto; margin: 0 0 24px; }
h2 { color: #172033; font-size: 16pt; line-height: 1.25; margin: 0 0 12px; page-break-after: avoid; }
p { margin: 0 0 12px; white-space: pre-line; }
ul, ol { margin: 0 0 14px; padding-left: 24px; }
li { margin: 0 0 5px; }
.table-wrap { margin: 14px 0 18px; overflow: visible; }
table { border-collapse: collapse; font-size: 8.5pt; table-layout: auto; width: 100%; }
thead { display: table-header-team; }
tr { break-inside: avoid; }
th { background: #e8eef8; border: 1px solid #aebbd0; color: #172033; font-weight: 700; padding: 7px 8px; text-align: left; vertical-align: top; }
td { border: 1px solid #cbd5e1; overflow-wrap: anywhere; padding: 7px 8px; vertical-align: top; }
tbody tr:nth-child(even) td { background: #f8fafc; }
.callout { border: 1px solid #bfdbfe; border-left: 4px solid #2563eb; break-inside: avoid; margin: 12px 0 16px; padding: 10px 12px; }
.callout-title { font-weight: 700; margin-bottom: 3px; }
.callout.success { background: #f0fdf4; border-color: #86efac; border-left-color: #16a34a; }
.callout.warning { background: #fffbeb; border-color: #fde68a; border-left-color: #d97706; }
.callout.critical { background: #fef2f2; border-color: #fecaca; border-left-color: #dc2626; }
.legacy-content h1 { font-size: 20pt; }
.legacy-content h2 { margin-top: 20px; }
.legacy-content pre { background: #f1f5f9; border: 1px solid #cbd5e1; overflow-wrap: anywhere; padding: 10px; white-space: pre-wrap; }
.legacy-content code { font-family: "SFMono-Regular", Consolas, monospace; font-size: 9pt; }
@media screen { body { margin: 24px auto; max-width: 920px; padding: 0 24px 48px; } }
@media print { body { max-width: none; } }
</style>
</head>
<body>
{{if .LegacyHTML}}<main class="legacy-content">{{.LegacyHTML}}</main>{{else}}
<header class="document-header">
  <h1>{{.Title}}</h1>
  {{if .Subtitle}}<p class="subtitle">{{.Subtitle}}</p>{{end}}
  {{if .Metadata}}<div class="metadata">{{range .Metadata}}<div class="metadata-item"><span class="metadata-label">{{.Label}}</span><span class="metadata-value">{{.Value}}</span></div>{{end}}</div>{{end}}
</header>
<main>{{range .Sections}}<section><h2>{{.Title}}</h2>{{range .Blocks}}
{{if eq .Type "paragraph"}}<p>{{.Text}}</p>{{end}}
{{if eq .Type "bullet_list"}}<ul>{{range .Items}}<li>{{.}}</li>{{end}}</ul>{{end}}
{{if eq .Type "numbered_list"}}<ol>{{range .Items}}<li>{{.}}</li>{{end}}</ol>{{end}}
{{if eq .Type "table"}}<div class="table-wrap"><table><thead><tr>{{range .Table.Columns}}<th>{{.}}</th>{{end}}</tr></thead><tbody>{{range .Table.Rows}}<tr>{{range .}}<td>{{.}}</td>{{end}}</tr>{{end}}</tbody></table></div>{{end}}
{{if eq .Type "callout"}}<div class="callout {{.Tone}}">{{if .Title}}<div class="callout-title">{{.Title}}</div>{{end}}<div>{{.Text}}</div></div>{{end}}
{{end}}</section>{{end}}</main>{{end}}
</body>
</html>`))

type pipelineDocumentTemplateData struct {
	models.DocumentSpec
	LegacyHTML template.HTML
}

func renderPipelineFinalOutputDocumentHTML(fallbackTitle, content string) ([]byte, error) {
	spec, err := parseDocumentSpec(content)
	if err == nil {
		return executePipelineDocumentTemplate(pipelineDocumentTemplateData{DocumentSpec: spec})
	}
	return renderLegacyPipelineMarkdownHTML(fallbackTitle, content)
}

func renderPipelineFinalOutputHTML(content string) ([]byte, error) {
	spec, err := parseDocumentSpec(content)
	if err == nil {
		return executePipelineDocumentTemplate(pipelineDocumentTemplateData{DocumentSpec: spec})
	}
	legacy := sanitizePipelineFinalOutputHTML(content)
	if strings.TrimSpace(legacy) == "" {
		return nil, fmt.Errorf("HTML output is empty after sanitization")
	}
	return []byte(legacy), nil
}

func executePipelineDocumentTemplate(data pipelineDocumentTemplateData) ([]byte, error) {
	var output bytes.Buffer
	if err := pipelineFinalOutputDocumentTemplate.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("render document HTML: %w", err)
	}
	return output.Bytes(), nil
}

func renderLegacyPipelineMarkdownHTML(title, markdown string) ([]byte, error) {
	var body bytes.Buffer
	parser := goldmark.New(goldmark.WithExtensions(extension.GFM))
	if err := parser.Convert([]byte(markdown), &body); err != nil {
		return nil, fmt.Errorf("render legacy Markdown: %w", err)
	}
	data := pipelineDocumentTemplateData{
		DocumentSpec: models.DocumentSpec{Title: strings.TrimSpace(title)},
		// #nosec G203 -- Goldmark's default renderer omits raw HTML; unsafe rendering is not enabled.
		LegacyHTML: template.HTML(body.String()), // Goldmark omits raw HTML unless unsafe rendering is explicitly enabled.
	}
	return executePipelineDocumentTemplate(data)
}
