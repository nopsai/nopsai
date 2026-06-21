# Pipeline Final Output Rendering

Pipeline final outputs are run-owned deliverables configured through
`output.items` in pipeline YAML. Output name, type, timing, prompt, and optional
LLM profile remain GitOps-owned. Generated source, validation audit, and render
audit remain attached to the run record.

## Generation Contract

Every provider receives a system instruction requiring exactly one envelope:

```text
<final_output>
deliverable content
</final_output>
```

NopsAI stores only the element content. Missing, malformed, duplicate, or empty
elements fail validation. A first contract or schema violation receives one
corrective retry; a second violation fails the output. Provider/network errors
are not retried by this feature. Output calls use temperature `0`, and usage is
recorded for accepted and rejected responses.

## Canonical Formats

| Type | Stored source | Download |
| --- | --- | --- |
| Markdown | Markdown | `.md` |
| JSON | Valid JSON | `.json` |
| PDF | `DocumentSpec` JSON | Gotenberg/Chromium PDF |
| HTML | `DocumentSpec` JSON | Server-owned HTML/CSS document |
| Excel | `SpreadsheetSpec` JSON | Excelize XLSX workbook |

PDF and HTML use this versioned shape:

```json
{
  "version": "1",
  "title": "Release report",
  "subtitle": "Production",
  "metadata": [{ "label": "Run", "value": "run-123" }],
  "sections": [{
    "title": "Summary",
    "blocks": [
      { "type": "paragraph", "text": "Everything passed." },
      { "type": "bullet_list", "items": ["Build complete"] },
      { "type": "table", "table": {
        "columns": ["Service", "Status"],
        "rows": [["API", "Healthy"]]
      }},
      { "type": "callout", "tone": "success", "title": "Result", "text": "Ready" }
    ]
  }]
}
```

Supported document blocks are `paragraph`, `bullet_list`, `numbered_list`,
`table`, and `callout`. Callout tones are `info`, `success`, `warning`, and
`critical`. Unknown fields, unsupported blocks, mismatched table rows, and
configured size-limit violations are rejected before persistence.

Excel uses typed object rows:

```json
{
  "version": "1",
  "title": "Operations",
  "sheets": [{
    "name": "Summary",
    "columns": [
      { "key": "service", "header": "Service", "width": 24, "number_format": "text" },
      { "key": "availability", "header": "Availability", "number_format": "percent" }
    ],
    "rows": [{ "service": "API", "availability": 0.999 }],
    "freeze_header": true,
    "auto_filter": true
  }]
}
```

Cells may be strings, numbers, booleans, or null. Object/array cells, unknown
column keys, duplicate sheets/columns, and formulas are not accepted. Supported
formats are `text`, `integer`, `decimal`, `percent`, `currency_usd`,
`currency_eur`, `date`, `datetime`, and `boolean`. XLSX output supports multiple
sheets, typed cells, widths, number formats, frozen headers, and filters.

## PDF Service

NopsAI renders `DocumentSpec` through an escaped server-owned HTML/CSS template
with real tables, repeated table headers, wrapping, and print page rules. It
posts only that self-contained `index.html` to Gotenberg's
`/forms/chromium/convert/html` route.

```text
FINAL_OUTPUT_PDF_RENDERER_URL=http://gotenberg:3000
FINAL_OUTPUT_PDF_TIMEOUT_SECONDS=45
```

Docker Compose includes a pinned, read-only Gotenberg service with a writable
temporary filesystem, dropped capabilities, a health check, and no published
host port. Other deployments must provision the service and set the URL through
deployment configuration. Pipeline YAML never contains infrastructure URLs.

## Preview Behavior

- Markdown is parsed with GFM support.
- PDF is fetched through the authorized download endpoint and displayed in an
  inline viewer with loading/error states.
- Excel renders typed sheet tabs and a bounded table preview.
- PDF/HTML `DocumentSpec` renders with sections, lists, callouts, and tables.
- JSON is pretty-printed; legacy HTML remains sandboxed.

Copy actions convert structured source to readable text. Downloads remain the
canonical complete artifacts.

## Compatibility

New PDF, HTML, and Excel generations must pass the structured schemas. Existing
stored PDF Markdown and Excel Markdown/CSV data remain downloadable through
render-only compatibility adapters. Existing stored HTML is sanitized before
download and stays sandboxed in preview. Compatibility adapters are not used
for new LLM responses.

## Authorization, MCP, And GitOps

No new AAA action is introduced. Run details, downloads, PDF previews, and
`nopsai.get_pipeline_run_output` remain protected by `pipeline_run.read` for the
requested run. Hosted MCP returns the same authorized structured source and
audit fields as REST.

The pipeline's `output.items` declaration remains ordinary GitOps YAML. The
versioned specs are generated run data, not configuration drift and not written
back to the configuration repository.

## Audit And Monitoring

Each output stores `generation_attempts`, `contract_violations`,
`render_attempts`, and `render_failures`. REST and hosted MCP expose these
fields. Prometheus exports:

- `nopsai_pipeline_final_outputs_total`
- `nopsai_pipeline_final_output_generation_attempts_total`
- `nopsai_pipeline_final_output_contract_violations_total`
- `nopsai_pipeline_final_output_retries_total`
- `nopsai_pipeline_final_output_render_attempts_total`
- `nopsai_pipeline_final_output_render_failures_total`

Labels are bounded to output type, pipeline path/name, and group.

## Code Ownership

- Shared spec DTOs: `pkg/models/final_output_specs.go`
- Envelope and schema enforcement: `pipeline_final_output_contract.go` and
  `pipeline_final_output_specs.go`
- Generation orchestration and persistence: `pipeline_final_outputs.go`
- Download dispatch: `pipeline_final_outputs_render.go`
- HTML document model: `pipeline_final_outputs_document.go`
- Gotenberg client: `pipeline_final_outputs_pdf.go`
- Excel workbook model: `pipeline_final_outputs_spreadsheet.go`
- UI action orchestration: `RunFinalOutputs.tsx`
- UI format rendering: `final-output-preview/`
- Route composition: `routes.go` and `RunDetailPanel.tsx`
