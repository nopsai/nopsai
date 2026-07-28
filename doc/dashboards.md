# Team Dashboards

Team dashboards are team-owned operational views populated by validated
pipeline final outputs. They support dashboard CRUD, pipeline-output source
assignment, generated sections, publication storage, chart/series rendering,
scheduled refreshes, GitOps ownership, provenance, refresh orchestration,
history, stale-state metadata, AAA enforcement, MCP visibility, monitoring,
and UI rendering.

## Pipeline Publication

Pipelines publish to a dashboard by declaring an output item with
`type: dashboard`:

```yaml
output:
  items:
    - name: deployment-summary
      type: dashboard
      when: always
      dashboard:
        ref: platform/engineering-health
        section: deployments
        entry_key: payments-api
        mode: replace
        preset: auto
        ttl: 7d
      prompt: |
        Show the current deployment result for payments-api.
        Include status, summary, deployed version, checks, and attention items.
```

The pipeline prompt describes the desired content. NopsAI requires the LLM to
return a validated `DashboardSpec`; generated HTML, CSS, and JavaScript are not
accepted for standard dashboard content. Pipeline authors do not need to know
or target the dashboard renderer schema; they describe the dashboard intent and
NopsAI chooses the structure from the available run evidence.

Supported publication modes:

- `replace`: keeps the latest current publication for `section + entry_key`
  and archives previous revisions in history.
- `append`: adds each accepted output to a feed while preserving run-output
  provenance.
- `snapshot`: archives the current publications for a section and publishes a
  new section snapshot while preserving history.
- `series`: merges incoming chart or series points into the current publication
  for `section + entry_key`, dedupes by timestamp or label, and retains the
  latest bounded point window per series. Incoming series-mode outputs must
  include at least one `chart` or `series` block with chart points.

Supported dashboard presets are generation hints. They do not change the
validated `DashboardSpec` schema, publication modes, GitOps ownership, AAA, MCP,
or monitoring behavior. They tell the final-output generator which block mix
should be primary:

| Preset | Expected dashboard shape |
| --- | --- |
| `auto` | Smallest useful shape for the prompt and evidence; one or two blocks for simple answers, richer layouts only when the data calls for them. |
| `report` | Narrative report first: executive summary or callout, then short text/list blocks for changes, blockers or risks, and next action. Tables are supporting evidence after the narrative, not the primary or first block unless the prompt asks for an appendix. |
| `table` | One scannable table is primary, with stable column keys for repeated records and at most a short status or callout summary before it. |
| `status` | Current health/readiness first, using a status block or callout, then properties, progress, or a short attention list. |
| `timeline` | Chronological output, using a `series` line/area chart for timestamped numeric data or an ordered list/table for discrete events. When `mode: series` is used, include a chart or series block with ordered points. |
| `comparison` | Side-by-side comparison of services, environments, versions, or options, usually as a comparison table or properties blocks with a callout for the key difference. |
| `metrics` | Numbers first, using properties/status blocks for headline values, explicit units and ratios, and charts for categorical or trend data. |
| `mixed` | Cohesive operational digest with complementary blocks such as headline properties/status, charts, risk callouts, tables, and next-action lists. |

`ttl` accepts Go durations such as `168h` and day shorthand such as `7d`, up
to the platform maximum retention. Expired content remains visible but is
marked stale until replaced or removed.

## DashboardSpec Contract Reference

`DashboardSpec` is the internal renderer contract used after generation and
validation. Pipeline authors generally do not write this JSON by hand; the
configured LLM creates it from the run context and output prompt.

```json
{
  "version": "1",
  "title": "Payments API deployment",
  "blocks": [
    {
      "type": "callout",
      "tone": "success",
      "title": "Deployment completed",
      "text": "Version 4.12.0 was deployed."
    },
    {
      "type": "table",
      "title": "Checks",
      "columns": [
        { "key": "name", "label": "Check" },
        { "key": "status", "label": "Status" }
      ],
      "rows": [
        { "name": "Smoke tests", "status": "Passed" }
      ]
    },
    {
      "type": "series",
      "title": "Latency",
      "chart": {
        "type": "line",
        "unit": "ms",
        "aggregation_interval": "5m",
        "missing_values": "gap",
        "time_window": { "from": "now-1h", "to": "now" },
        "dimensions": { "team": "platform", "environment": "prod" },
        "series": [
          {
            "key": "api.p95",
            "label": "API p95",
            "team": "platform",
            "environment": "prod",
            "color": "#2563eb",
            "points": [
              { "timestamp": "2026-07-15T10:00:00Z", "value": 120 },
              { "timestamp": "2026-07-15T10:05:00Z", "value": 130 }
            ]
          }
        ]
      }
    }
  ]
}
```

Validation rejects unknown fields, unsupported block types, unsafe link
schemes, HTML/CSS/JavaScript/iframes/forms/executable links, object or array
table cells, invalid keys, unsupported chart types, non-finite values, too many
series or points, and oversized content before publication is persisted. Chart
types are `line`, `bar`, `area`, `pie`, and `donut`; `series` blocks support
line, bar, and area charts. A declared chart series may have an empty `points`
array so the UI can render an empty-state chart instead of rejecting otherwise
valid generated output.

Generated dashboard output is normalized before strict validation for common
LLM wrappers: a top-level `widgets` array, `sections[].blocks`,
`sections[].widgets`, or section-like entries inside `blocks` with nested
`blocks`/`widgets` are folded into the required flat top-level `blocks` array.
Generated `properties` arrays or objects are folded into `items`, and display
item or point `key` aliases are folded into `label`. Common model chart aliases
are also normalized: `type_name`/`typeName` become `type`, `chartType` or
`chart_type` become `chart.type`, chart `shape` aliases such as `doughnut`
become supported chart types, chart block type aliases such as `bar` or `line`
become canonical `chart` blocks, and block-level `series` and chart units are
moved under the `chart` object for chart blocks. Object-shaped display values
are converted to display strings, object-shaped chart `series` values are
wrapped or expanded into the required series array, and chart type aliases such
as `timeline`, `column`, and `doughnut` are normalized to supported chart types.
`data` and `points` aliases at the root, block, chart, or series level are
normalized into `chart.series[].points` based on whether the payload looks like
series objects or raw points. Status aliases such as `failure`, `failed`,
`blocked`, `true`, and `False` are normalized into supported dashboard status
tones before validation. Harmless publication-target metadata accidentally
included at the root, such as `entryKey`, is ignored. If a chart declares a
series but leaves `points` empty or `null`, NopsAI can infer points from the
nearest table by matching the chart or series label to a numeric table column
and using a text-like column for labels. The renderer contract remains the flat
`DashboardSpec` shape above, and block fields still use strict validation. If
generated content omits the root title, NopsAI
derives one from the first block title or label and otherwise uses `Dashboard
output`. Missing, numeric `1`, and `1.0` generated versions are normalized to
the current `DashboardSpec` version `1`; other versions remain invalid.

## Example: Intent-Driven Dashboard Publication

This pipeline publishes one dashboard output. The step creates deterministic
evidence and the final output prompt asks the configured LLM profile to turn
that evidence into a validated `DashboardSpec`.

```yaml
name: service-health-dashboard
version: "1.0"
description: Publish service health evidence into Engineering Health.
container_image: alpine:3.20
llm_profile: standard

steps:
  - name: collect-evidence
    script: |
      cat > dashboard-evidence.json <<'JSON'
      {
        "service": "payments-api",
        "summary": "Payments API is healthy. Build and deploy passed; latency is within target.",
        "actions": [
          {"label": "Review slow checkout test", "status": "watch", "tone": "warning"},
          {"label": "Keep deployment canary at 25 percent for 30 minutes", "status": "open", "tone": "info"},
          {"label": "Archive successful release notes", "status": "done", "tone": "success"}
        ],
        "stage_results": [
          {"label": "build", "value": 42},
          {"label": "test", "value": 39},
          {"label": "deploy", "value": 37}
        ]
      }
      JSON
      cat dashboard-evidence.json

output:
  items:
    - name: service-health-widgets
      type: dashboard
      when: always
      dashboard:
        ref: platform/engineering-health
        section: service-health
        entry_key: payments-api
        mode: replace
        preset: auto
        ttl: 7d
      prompt: |
        Build a dashboard summary for payments-api from the run evidence.
        Show the current service summary, next actions, and stage throughput.
```

Pipeline authors describe the dashboard they want. NopsAI sends emitted step
output evidence first, then run metadata, recent same-pipeline run history,
pipeline context, step and task durations, child runs, and dashboard intent to
the configured LLM. Emitted stdout/stderr from steps, including plain `echo`
output and structured JSON/NDJSON, is authoritative for business facts such as
artifact names, versions, durations, services, and subjects. Runtime/container
metadata, image-pull logs, and prior run history must not replace values that
are present in the emitted step output. NopsAI then validates and repairs the
generated `DashboardSpec` before publication. Each output item is generated
with its own output name, dashboard ref, section, entry key, publication mode,
and preset, so a single pipeline can publish multiple dashboard entries without
collapsing them into one generic summary.

For example, this is enough for a dashboard output prompt:

```yaml
      prompt: |
        Show how many images were built, which version each image used, how long
        each image build took, and the most important subject in this pipeline.
```

NopsAI chooses the dashboard structure dynamically from the prompt and evidence.

For operational overview dashboards, emit one structured evidence line such as
`dashboard_evidence=<json>` and ask for the composed view: KPI cards first,
categorical build-duration bars, circular readiness/configuration coverage,
risk callouts, and a readiness matrix. NopsAI treats that structured evidence
as the source of truth for counts, ratios, chart points, image tags,
environments, and boolean status cells, so dashboard math and status rendering
stay stable even when the LLM wording varies. The dashboard UI renders that
shape as a wide operational overview with the duration comparison and coverage
donuts grouped together for quick scanning.
The values still come from run metadata and logged evidence. When the pipeline
builds multiple images inside one script, plain log lines such as
`name:version` image lists and matching duration lists are summarized for the
model; structured summaries such as JSON are still recommended for richer or
nested facts.

The team sample config repo includes five immediately runnable dashboard
pipelines under `examples/sample-config-repo/team-1-repo/pipelines/`: two technical
checks (`technical-api-readiness` and `technical-slo-burn-rate`) and three
business-facing workflows (`customer-onboarding-pulse`,
`finance-close-snapshot`, and `people-capacity-plan`). They use only
`alpine:3.20` shell scripts and structured `dashboard_evidence=<json>`, so they
can be launched directly or through the bound `team-1/ops-dashboard` sources
without secrets, approvals, variables, or external MCP profiles.

When the prompt does not request a specific visualization, NopsAI guides the
model to choose by data shape: text or callout for narrative conclusions,
status/progress/properties for current state and scalar facts, tables for
repeated records, bar charts for categorical counts, durations, and rankings,
line or area charts for time series, and pie or donut charts only for bounded
part-to-whole data.

Dashboard rendering displays `pie` and `donut` chart types as circular charts
with point-level slice colors. Bar charts render compact category labels when
points provide labels. Tables render boolean-like values such as `true`/`false`,
`yes`/`no`, and `passed`/`failed` as compact status chips, with risk-oriented
columns such as vulnerabilities or missing configuration colored by inverse
meaning.

The dashboard itself can be managed by GitOps under
`dashboards/platform/engineering-health.yaml`:

```yaml
title: Engineering Health
description: Team-owned operational health view for deployment and service status.
visibility: team

sections:
  - section_key: service-health
    title: Service Health
    description: Current service status, actions, and stage throughput.
    display_order: 10
    layout:
      columns: 2

sources:
  - section_key: service-health
    pipeline_id: service-health-dashboard
    output_name: service-health-widgets
    entry_key: payments-api
    run_scope: prod
    enabled: true
    required_for_refresh: true
    refresh_order: 10

refresh_schedules:
  - name: Hourly service health
    cron_expression: "0 * * * *"
    timezone: Europe/Amsterdam
    enabled: true
    scope:
      type: section
      section_key: service-health
    mode: best_effort
    timeout: 45m
    max_concurrency: 2
```

`run_scope` is part of the dashboard source identity. A source with
`run_scope: prod` only accepts successful pipeline runs launched with scope
`prod`; omitting `run_scope` means the exact default/unscoped run, not any
scope. This prevents a trigger running the same pipeline in `dev` from
overwriting a dashboard entry intended for `prod`.

## REST API

Dashboard management:

```text
GET    /v1/dashboards
POST   /v1/dashboards
GET    /v1/dashboards/{dashboardID}
PUT    /v1/dashboards/{dashboardID}
PATCH  /v1/dashboards/{dashboardID}
DELETE /v1/dashboards/{dashboardID}
GET    /v1/dashboards/{dashboardID}/view
GET    /v1/dashboards/{dashboardID}/history
DELETE /v1/dashboards/{dashboardID}/publications/{publicationID}
```

Refresh orchestration:

```text
POST   /v1/dashboards/{dashboardID}/refresh
GET    /v1/dashboards/{dashboardID}/refreshes
GET    /v1/dashboards/{dashboardID}/refreshes/{refreshID}
POST   /v1/dashboards/{dashboardID}/refreshes/{refreshID}/cancel
POST   /v1/dashboards/{dashboardID}/refreshes/{refreshID}/retry-failed
GET    /v1/dashboards/{dashboardID}/refresh-schedules
POST   /v1/dashboards/{dashboardID}/refresh-schedules
PUT    /v1/dashboards/{dashboardID}/refresh-schedules/{scheduleID}
PATCH  /v1/dashboards/{dashboardID}/refresh-schedules/{scheduleID}
DELETE /v1/dashboards/{dashboardID}/refresh-schedules/{scheduleID}
POST   /v1/dashboards/{dashboardID}/refresh-schedules/{scheduleID}/enable
POST   /v1/dashboards/{dashboardID}/refresh-schedules/{scheduleID}/disable
POST   /v1/dashboards/{dashboardID}/refresh-schedules/{scheduleID}/run
```

Refresh requests accept dashboard, section, and legacy source scope:

```json
{
  "scope": { "type": "section", "section_key": "deployments" },
  "mode": "strict",
  "timeout": "45m",
  "max_concurrency": 4
}
```

`strict` mode blocks required sources that cannot be launched. `best_effort`
skips unavailable sources and records them in the refresh. Only one refresh can
run per dashboard at a time; `Idempotency-Key` safely deduplicates duplicate
start requests. Existing current publications stay visible while selected
sources run. Refresh status is `complete`, `partial`, `failed`, `cancelled`, or
`timed_out`, with per-source progress and retry failed support. Refreshes can
be triggered manually, by API, by assistant, or by schedules.
The UI starts manual refreshes for a whole dashboard or section only. Legacy
source-scoped refresh records remain readable for compatibility, but source
bindings are not offered as a manual refresh target because one pipeline run can
publish several dashboard outputs together.

Dashboard refreshes group selected source bindings by effective `pipeline_id`
and `run_scope`. One pipeline run is launched for each unique group, even when
that pipeline publishes several outputs into the same dashboard. The refresh
keeps one progress row per source binding so sections can track their own
dashboard output generation, publication success, and publication failure. The
progress row follows the matching `pipeline_run_outputs` item, so a successful
pipeline run does not mark the dashboard source successful unless its dashboard
output was generated and published. If a finished pipeline run never produces
the expected output item, reconciliation marks the source failed instead of
leaving it stuck as generating. A refresh request or schedule may set
`run_scope` to override the source scopes for that run, but publication still
requires an enabled source binding with the same exact scope.

Refresh source responses keep the historical source `status` as the refresh
rollup and also expose `pipeline_status`, `pipeline_started_at`,
`pipeline_finished_at`, `output_status`, `output_created_at`,
`output_updated_at`, `output_duration`, and `output_duration_seconds`. This
lets operators see a pipeline that has finished separately from an output that
is still pending, generating, failed, cancelled, or published successfully.

Refresh schedules run under a dashboard-owned service account. A schedule
stores cron, timezone, enabled state, strict/best-effort mode, scope,
max concurrency, timeout, variables, next run, and latest refresh status.
Service-account grants are synced for the dashboard, selected source bindings'
pipelines, and referenced run scopes so scheduled execution stays auditable and
AAA-compatible.
The UI creates schedules for a whole dashboard or section only. Individual
source bindings remain the publication and progress-tracking contract for each
dashboard output, but they are not offered as recurring schedule targets because
dashboard outputs from the same pipeline are generated by the same pipeline run
and cannot be rerun independently as isolated cards.

Section and source management:

```text
GET    /v1/dashboards/{dashboardID}/sections
POST   /v1/dashboards/{dashboardID}/sections
PUT    /v1/dashboards/{dashboardID}/sections/{sectionID}
PATCH  /v1/dashboards/{dashboardID}/sections/{sectionID}
DELETE /v1/dashboards/{dashboardID}/sections/{sectionID}

GET    /v1/dashboards/{dashboardID}/sources
POST   /v1/dashboards/{dashboardID}/sources
PUT    /v1/dashboards/{dashboardID}/sources/{sourceID}
PATCH  /v1/dashboards/{dashboardID}/sources/{sourceID}
DELETE /v1/dashboards/{dashboardID}/sources/{sourceID}
```

Use UUID dashboard IDs on REST paths. Pipeline YAML and hosted MCP accept
dashboard refs in `team/dashboard-slug` form. Deleting a publication archives
the current section card and writes a `removed` history event; it does not
delete source bindings, refresh rows, pipeline runs, or final output records.

## UI Source Mapping

The dashboard UI uses a dropdown selector for dashboards the caller can access,
then shows the selected dashboard with a compact toolbar, metadata header,
section tabs generated from the dashboard sections returned by the API, and an
inline attention indicator before the dashboard title when the latest refresh,
schedule, publication, or required source needs operator review. Hovering or
focusing that indicator explains what happened and what to do next. Each
section is one tab; selecting a tab renders only that section's cards, and the
UI does not infer static categories
such as release, monitoring, or activity from section names. Dashboard/tab URLs
use `#/dashboards?dashboard=<dashboard-id>&tab=<section_key>` so each selected
dashboard section can be opened directly, copied, or shared. Refresh, schedule
refresh, edit, Access, details, and delete actions stay grouped under the
dashboard action menu so the primary screen remains focused on operational
state. Section tabs show section titles only, while the active section surface
exposes only the cards and optional section description without repeating the
active tab title, completion count, entry count, source count, or latest refresh
status pill. Dashboard-level metadata, sources, schedules, refresh history, and
latest runs open in a dashboard details modal as dashboard-level tabs. Published
entries link back to their originating
pipeline run detail when run provenance is available, using direct run-detail
routes so the exact run opens even when it is not visible in the recent list.
Delete actions use an in-app confirmation dialog with target-specific impact
copy and backend errors instead of browser confirmation prompts. Operators with
dashboard write access can remove an individual section entry card from the
card header; the publication is archived and can be recreated by a future
refresh. Every operator can drag a section card by its grab handle to reorder
cards within the active section tab, and drag the card's right edge to resize it
between compact, standard, and wide widths. This card arrangement is remembered
in the browser per dashboard and section, so it does not mutate GitOps YAML,
publication records, refresh orchestration, AAA, MCP, or monitoring contracts.

The dashboard UI keeps source binding at the dashboard level. New-dashboard and
edit-dashboard modals group fields by purpose and include one-line descriptions
for each group. The pipeline picker only lists pipelines with `dashboard`
outputs whose `dashboard.ref` matches the dashboard being created or edited.
Selecting those pipelines creates the needed section records and source
bindings from `dashboard.section`, `dashboard.entry_key`, the output name, and
the chosen exact run scope.
New dashboards choose from existing team paths, and broader sharing is assigned
through the dashboard Access button after creation, matching the pipeline
access-management pattern.

Dashboard sections are created from selected pipeline dashboard outputs, then
render as tabs on the dashboard surface. Sources, existing schedules,
refreshes, and latest run history are shown in the dashboard details modal,
not from individual section tabs, because one pipeline can publish several
section outputs in the same run.
Cancelled refreshes are explained from the dashboard-title attention indicator
so operators understand whether a user or automation stopped the refresh before
new output was published. Latest run history is based on refresh source
attempts, so failed runs and failed publication attempts remain visible even
when no publication event was created. Section edit and delete operations live in
dashboard-edit flows rather than per-section action menus. Editing a section
updates its title, description, and display order while keeping `section_key`
stable. Deleting a section also removes its source bindings, current
publications, publication events, and section-scoped refresh schedules. When a
refresh is running, each section with queued or generating dashboard outputs
shows an inline generation panel with output status and direct run links to the
single pipeline run that is producing those outputs.
Operators with dashboard write access can cancel the active refresh from that
panel; cancellation also marks pending or generating dashboard final outputs as
cancelled.

Refresh actions open an extra-wide modal with separate target and execution
guardrail sections. The manual refresh modal explains dashboard and section
scope; strict versus best-effort behavior; timeout; and concurrency before
starting pipeline runs.

Scheduled refreshes are created from dashboard-level actions. The schedule modal
is extra-wide and separates identity, cadence, target, and execution guardrails.
Operators use the same frequency builder as pipeline schedules:
every-N-minutes, hourly, daily, weekdays, weekly, monthly, yearly, or custom
five-field cron. They can set an IANA timezone, enable or disable the schedule,
and target the whole dashboard or a section. Existing schedules can be run now,
edited, enabled/disabled, or deleted from the dashboard-level schedules tab.
Delete uses the same in-app confirmation pattern as dashboard cleanup.

Existing source bindings can still be edited with guided dropdowns. The source
modal reads the selected dashboard sections, the dashboard-capable pipeline
catalog, and the selected pipeline YAML, then offers only dashboard final
outputs whose `dashboard.ref` targets the current dashboard and whose
`dashboard.section` is present on the dashboard. Selecting an output maps the
section from `dashboard.section`, and maps entry choices from
`dashboard.entry_key`, existing entries, or the output-name fallback. Choosing
the output-name fallback saves an empty source `entry_key`, which removes the
explicit entry-key binding while preserving publication compatibility with
pipeline outputs. The run scope dropdown comes from existing secret/variable
scopes and defaults to the exact default/unscoped run. The REST payload remains
the same
`dashboard_source_bindings` contract, so GitOps YAML, AAA checks, refresh
orchestration, monitoring, and MCP behavior stay compatible.
The larger mapping review panel shows the dashboard target and selected
pipeline output, including run scope, side by side before saving edits.

## Authorization

Dashboards use the `dashboard` resource type. Actions are:

- `dashboard.list`
- `dashboard.read`
- `dashboard.create`
- `dashboard.update`
- `dashboard.delete`
- `dashboard.publish`
- `dashboard.refresh`
- `dashboard.manage_sources`
- `dashboard.manage_acl`

Product role defaults:

- viewer: `dashboard.list`, `dashboard.read`
- developer: viewer plus create, update, publish, refresh, and manage sources
- owner: developer plus delete and manage ACL

Pipeline publication re-checks `dashboard.publish` as the effective run
subject before it writes dashboard publications. Publication then matches
`dashboard + section + pipeline_id + output_name + entry_key + run_scope`.
If the run scope does not match an enabled source binding, NopsAI records a
publication skip event and leaves current dashboard content unchanged.

## GitOps Boundary

Dashboard output declarations remain normal pipeline YAML. Dashboard records
can also be managed from `dashboards/` and reusable dashboard templates from
`dashboard-templates/`. GitOps imports validate dashboard YAML, preserve source
path, source commit, team ownership, managed state, sections, sources, and
refresh schedules, and prune only records still owned by the same config
repository. Manual dashboard edits detach GitOps ownership to avoid unsafe
overwrites. Generated dashboard content, current publications, and publication
events remain runtime state in Postgres and are not written back to the config
repository.

GitOps source bindings should include `run_scope` whenever the pipeline is
expected to publish from a non-default scope. Leaving it empty keeps the default
scope exact and does not preserve legacy any-scope behavior.

## Monitoring And MCP

Prometheus exports:

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
- `nopsai_pipeline_final_output_generation_duration_seconds`

Hosted MCP exposes:

- `nopsai.list_dashboards`
- `nopsai.get_dashboard`
- `nopsai.list_dashboard_refreshes`
- `nopsai.refresh_dashboard`
- `nopsai.list_dashboard_refresh_schedules`
- `nopsai.run_dashboard_refresh_schedule`
- `nopsai://dashboards`

Dashboard creation and source mutation are available through guarded REST via
`nopsai.call_api`; refresh and schedule run-now have dedicated confirmed MCP
tools. The dashboard-level pipeline assignment and source-edit UI reuse
existing REST reads and pipeline YAML metadata, so they do not add new MCP tools
or mutate GitOps-owned resources outside the existing dashboard source contract.

## Code Ownership

- Shared models: `pkg/models/final_output_specs.go` and `pkg/models/model.go`
- Output validation: `services/nopsai/pipeline_final_output_specs.go`
- Publication orchestration: `services/nopsai/dashboard_publication.go`
- Refresh orchestration: `services/nopsai/dashboard_refresh.go` and
  `services/nopsai/dashboard_refresh_schedules.go`
- GitOps import/export: `services/nopsai/dashboard_gitops.go` and
  `services/nopsai/config_sync_*.go`
- Storage and schema: `services/nopsai/dashboard_store.go` and
  `services/nopsai/dashboard_schema.go`
- API handlers and routes: `services/nopsai/dashboard_handlers.go` and
  `services/nopsai/routes.go`
- AAA route/resource mapping: `services/nopsai/pkg/routeauthz/routeauthz.go`,
  `services/nopsai/resource_authz.go`, and `services/aaa/pkg/store/postgres.go`
- Hosted MCP: `services/nopsai/hosted_mcp_dashboards.go`,
  `services/nopsai/hosted_mcp_tools.go`, and
  `services/nopsai/hosted_mcp_resources.go`
- UI model/API/rendering: `services/ui/src/features/dashboards/`
  (`model.ts`, `api.ts`, `sourceOptions.ts`, `pipelineAssignments.ts`,
  `routes.ts`,
  `dashboardAttention.ts`, `dashboardCardLayout.ts`,
  `useDashboardCardLayout.ts`, `DashboardWorkspace.tsx`,
  `DashboardPublicationGrid.tsx`, `DashboardModals.tsx`, and
  `blocks/DashboardBlocks.tsx`)
- UI route composition: `services/ui/src/pages/Dashboards.tsx`
