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
| Dashboard | `DashboardSpec` JSON | Dashboard publication and JSON fallback |

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

Dashboard outputs use this versioned shape:

```json
{
  "version": "1",
  "title": "Service health",
  "blocks": [
    { "type": "status", "label": "API", "status": "success", "value": "Green" },
    {
      "type": "table",
      "title": "Checks",
      "columns": [{ "key": "name", "label": "Check" }],
      "rows": [{ "name": "Smoke tests" }]
    }
  ]
}
```

Dashboard block types are `status`, `text`, `callout`, `list`, `properties`,
`table`, `progress`, `link`, `chart`, and `series`. Chart types are `line`,
`bar`, `area`, `pie`, and `donut`; `series` blocks support line, bar, and area
charts with time windows, aggregation intervals, missing-value policy, team and
environment dimensions, and bounded point retention. Pie and donut charts render
as circular dashboard visuals with point-level slice colors, bar charts show
compact category labels, and boolean-like table values render as compact status
chips. Chart points may be inferred from a nearby table when a generated chart
declares an empty or null series. Common status, chart type, chart `shape`,
block-level chart `unit`, object-shaped display value, object-shaped series,
and root publication-metadata aliases are normalized before strict validation.
Table cells must be JSON scalars. Links must be relative or use
`http`/`https`. Common generated
wrappers, including top-level `widgets` and `sections[].blocks` /
`sections[].widgets` and section-like entries inside `blocks` with nested
`blocks`/`widgets`, are normalized into the required flat top-level `blocks`
array before strict validation. Generated `properties` aliases are normalized
to `items`, and display item or point `key` aliases are normalized to `label`.
Dashboard presets are shape hints on top of the same `DashboardSpec` contract:
`report` is narrative-first with tables only as supporting evidence, `table`
makes one table primary, `status` starts with current health/readiness,
`timeline` orders events or series chronologically, `comparison` presents
side-by-side differences, `metrics` starts with headline numbers and charts,
`mixed` composes complementary operator blocks, and `auto` chooses the smallest
useful layout.
For dashboard generation, emitted step stdout/stderr is supplied before metadata
and history and is treated as authoritative for business facts. Structured
emitted evidence lines such as `dashboard_evidence={...}` are promoted into a
compact evidence block before raw operational logs. This applies to plain log
lines as well as JSON/NDJSON; configured container images, runner/runtime
metadata, image-pull logs, and recent-history values must not replace artifact
names, versions, durations, services, or subjects already present in emitted
step output. Final output generation may copy non-secret operational labels
from emitted evidence, such as image tags, environment names, versions,
statuses, and JSON field names; the ban on raw environment values applies to
environment variable values and secrets. Dashboard outputs are
published to team-owned dashboards when their `output.items[].dashboard` target
is valid and the run subject has `dashboard.publish`. Dashboard publication
modes are `replace`, `append`, `snapshot`, and `series`; snapshot archives the
current content for the target section before publishing the new section
snapshot, while series mode merges chart points, dedupes by timestamp or label,
and retains the latest bounded point window. Series-mode outputs must include
at least one `chart` or `series` block with chart points so the publisher can
merge them.

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

- Dashboard outputs render in Dashboards and fall back to pretty JSON through
  run-output download.
- Markdown is parsed with GFM support.
- PDF is fetched through the authorized download endpoint and displayed in an
  inline viewer with loading/error states.
- Excel renders typed sheet tabs and a bounded table preview.
- PDF/HTML `DocumentSpec` renders with sections, lists, callouts, and tables.
- JSON is pretty-printed; legacy HTML remains sandboxed.

Copy actions convert structured source to readable text. Downloads remain the
canonical complete artifacts.

Operators can cancel final outputs while they are `pending` or `generating`
from run details or with
`POST /v1/runs/{runID}/outputs/{outputID}/cancel`. Cancellation marks the output
`cancelled` and prevents later content writes or dashboard publication when the
background LLM request returns. It does not cancel or change the status of the
pipeline run itself. Cancelling an active dashboard refresh also cancels any
pending or generating dashboard final outputs attached to that refresh, which
lets operators clear missed publication handoffs without changing completed
pipeline run status.

## Compatibility

New PDF, HTML, Excel, and dashboard generations must pass the structured
schemas. Existing stored PDF Markdown and Excel Markdown/CSV data remain
downloadable through render-only compatibility adapters. Existing stored HTML
is sanitized before download and stays sandboxed in preview. Compatibility
adapters are not used for new LLM responses.

## Authorization, MCP, And GitOps

Dashboard publication introduces `dashboard.publish` on dashboard resources.
Dashboard refresh introduces `dashboard.refresh` for dashboard, section, and
source refresh orchestration.
Run details, downloads, PDF previews, and `nopsai.get_pipeline_run_output`
remain protected by `pipeline_run.read` for the requested run. Cancelling output
generation uses `pipeline_run.cancel` for the requested run. Hosted MCP returns
the same authorized structured source and audit fields as REST.

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
- `nopsai_pipeline_final_output_generation_duration_seconds`
- `nopsai_dashboard_publications`
- `nopsai_dashboard_publication_events_total`
- `nopsai_dashboard_publications_total`
- `nopsai_dashboard_publication_failures_total`
- `nopsai_dashboard_refreshes`
- `nopsai_dashboard_refresh_sources`
- `nopsai_dashboard_refreshes_total`
- `nopsai_dashboard_refresh_failures_total`
- `nopsai_dashboard_refresh_duration_seconds`
- `nopsai_dashboard_refresh_sources_total`
- `nopsai_dashboard_stale_publications_total`
- `nopsai_dashboard_render_failures_total`
- `nopsai_dashboard_series_points_total`

Labels are bounded to output type, pipeline path/name, and team.

## Code Ownership

- Shared spec DTOs: `pkg/models/final_output_specs.go`
- Envelope and schema enforcement: `pipeline_final_output_contract.go` and
  `pipeline_final_output_specs.go`
- Generation orchestration and persistence: `pipeline_final_outputs.go`
- Download dispatch: `pipeline_final_outputs_render.go`
- HTML document model: `pipeline_final_outputs_document.go`
- Gotenberg client: `pipeline_final_outputs_pdf.go`
- Excel workbook model: `pipeline_final_outputs_spreadsheet.go`
- Dashboard model and publication: `dashboard_publication.go`,
  `dashboard_store.go`, and `dashboard_handlers.go`
- UI action orchestration: `RunFinalOutputs.tsx`
- UI format rendering: `final-output-preview/`
- Dashboard UI rendering: `features/dashboards/`
- Route composition: `routes.go` and `RunDetailPanel.tsx`
