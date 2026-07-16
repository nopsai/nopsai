# Team Dashboards

Team dashboards are team-owned operational views populated by validated
pipeline final outputs. They support dashboard CRUD, sections, source
discovery, publication storage, chart/series rendering, scheduled refreshes,
GitOps ownership, provenance, refresh orchestration, history, stale-state
metadata, AAA enforcement, MCP visibility, monitoring, and UI rendering.

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
accepted for standard dashboard content.

Supported block types:

- `status`
- `text`
- `callout`
- `list`
- `properties`
- `table`
- `progress`
- `link`
- `chart`
- `series`

Supported publication modes:

- `replace`: keeps the latest current publication for `section + entry_key`
  and archives previous revisions in history.
- `append`: adds each accepted output to a feed while preserving run-output
  provenance.
- `snapshot`: archives the current publications for a section and publishes a
  new section snapshot while preserving history.
- `series`: merges incoming chart or series points into the current publication
  for `section + entry_key`, dedupes by timestamp or label, and retains the
  latest bounded point window per series.

`ttl` accepts Go durations such as `168h` and day shorthand such as `7d`, up
to the platform maximum retention. Expired content remains visible but is
marked stale until replaced or removed.

## DashboardSpec

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
line, bar, and area charts.

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

Refresh requests accept dashboard, section, and source scope:

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

Refresh schedules run under a dashboard-owned service account. A schedule
stores cron, timezone, enabled state, strict/best-effort mode, scope,
max concurrency, timeout, variables, next run, and latest refresh status.
Service-account grants are synced for the dashboard, selected pipeline sources,
and referenced scopes so scheduled execution stays auditable and AAA-compatible.

Source management:

```text
GET    /v1/dashboards/{dashboardID}/sources
POST   /v1/dashboards/{dashboardID}/sources
PUT    /v1/dashboards/{dashboardID}/sources/{sourceID}
PATCH  /v1/dashboards/{dashboardID}/sources/{sourceID}
DELETE /v1/dashboards/{dashboardID}/sources/{sourceID}
```

Use UUID dashboard IDs on REST paths. Pipeline YAML and hosted MCP accept
dashboard refs in `team/dashboard-slug` form.

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
subject before it writes dashboard publications.

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
tools.

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
- UI route composition: `services/ui/src/pages/Dashboards.tsx`
