# Hosted MCP Feature Coverage

Hosted MCP exposes NopsAI as the current authenticated user. The assistant and
external MCP clients use the same JSON-RPC request processor and receive only
the tools/resources allowed by that user's AAA subject. Each tool call
re-checks the specific resource identified by the arguments.

Use `nopsai.get_feature_capabilities` or read `nopsai://features` to inspect
the current user's coverage. The response includes:

- the permission model used for hosted MCP and assistant calls
- available tools/resources for the current subject
- feature areas from the product inventory
- first-class MCP tools/resources, backing REST routes, and required AAA
  actions for each area
- per-area user access state for tools, resources, and permissions

For a user-facing capability catalog with example chat prompts, see
[assistant-capabilities.md](./assistant-capabilities.md).

The assistant LLM planner maps normal-language requests to first-party MCP
tools from the live permission-filtered tool list, descriptions, and input
schemas when the target is clear, and asks a clarifying question before tool
execution when a broad term such as "usage" could mean AI/LLM tokens, runner
cost, pipeline runs, variables, or another product area. In addition to
specialist intents, the planner covers setup, config repositories, notification
settings/routes, monitoring views/alerts/recommendations, credentials, runners,
AAA/admin/audit, data backup/cleanup, webhook sources, external triggers,
reusable steps, scoped secrets/variables, and explicit `nopsai.*` tool names.
Assistant conversation turns do not use static normal-language routing. If the
LLM planner is unavailable or returns an invalid plan, NopsAI executes no
hosted MCP tools and reports that no changes were applied.
AI/LLM usage requests are answered through planner-selected monitoring tools
such as `nopsai.get_monitoring_ai_usage` and supporting summary/efficiency
reads when more context is needed. Empty usage answers should explain the
evidence and likely recording or permission gaps instead of returning a bare
zero.

Every planned turn is validated against the current AAA subject, available
hosted MCP tools, tool input schemas, a bounded tool-call count, bounded
argument shape, and explicit user confirmation for mutating tools before any
planned step runs. Final planner answers require successful hosted MCP evidence
from the current turn; NopsAI does not use static request contracts to
reinterpret intent or route tools.
Final LLM synthesis is also quality-gated: if the model claims unapplied
changes or misses required proposal-safe wording, NopsAI falls back to the
deterministic MCP-grounded summary.

`nopsai.call_api` is the broad compatibility bridge for remaining product
features. It calls guarded `/v1` REST routes as the current authenticated
subject, rejects public/provider ingress and internal service routes, blocks
default plaintext secret reads, applies route-level AAA mappings such as
`secret.write_value` for GitOps secret encryption, and requires `confirm:true`
before mutating routes execute.

Coverage states:

- `first_class`: hosted MCP has direct tools/resources for the workflow
- `partial`: hosted MCP covers part of the workflow; remaining operations use
  REST/GitOps today
- `api_backed`: product APIs are reachable through `nopsai.call_api`, but a
  dedicated hosted MCP tool is still pending
- `contextual`: MCP provides backing data; UI rendering remains UI-owned

Current first-class coverage includes setup status/preflight/template planning
and confirmed bootstrap, pipeline inventory/search, pipeline YAML inspection and
validation, backend pipeline/reusable-step draft validation endpoints, reusable step GitOps plans, pipeline knowledge-context traversal,
LLM-drafted YAML checked by validation/proposal tools, GitOps-ready pipeline
create/update proposals, managed knowledge reads and write
plans, run/log analysis with explicit run-status and bounded-log chaining,
confirmed run mutations, team dashboard reads with current publications,
source bindings, provenance, and history, schedule inventory and GitOps write plans, webhook
source and external trigger plans, repository trigger provider/team/ingress
metadata reads, webhook-ingress policy explanations, config
repo sync/drift/write workflows with dry-run draft-bundle validation, notification mail/route plans, monitoring
analytics/views/alerts/recommendation actions including AI token usage by
pipeline/schedule/run/model/profile/feature/step/task, schedule/pipeline
comparison aliases, pipeline health explanations, optimization opportunity
discovery, data backup/cleanup operations, scope inventory, secret/variable
metadata and repeated variable-name analysis plus safe write/GitOps plans,
cost/statistics, LLM/MCP profile reads, system status,
credential metadata/rotation/access/GitOps plans, runner install/dispatch/ejection operations,
AAA/access/audit/admin workflows, dispatcher/runner health, and permission-bound
System Log source discovery plus bounded redacted tails. Long-lived System Log
SSE remains UI-only.
Data cleanup schedule provenance is visible through the backing data management
APIs; schedule definitions can be represented in GitOps while backup files and
cleanup job history remain runtime state.
Natural-language scope inventory requests can combine visible scope listing with
metadata-only secret counts by scope without reading plaintext secret values.
The API bridge remains the compatibility surface for auth self-service and other
guarded `/v1` routes when the current user has the matching AAA permissions.

The Teams, Pipeline Detail, and Run Detail UI also expose read-only reviewer
buttons: Analyse Resources, Analyse Pipeline, and Analyse Run. Deterministic
scoring uses visible page snapshots and the shared UI finding model; optional
AI Evaluation uses `POST /v1/analysis/evaluate`, a usable selectable LLM
profile, and the redacted reviewer report rather than direct browser-to-provider
calls or hosted MCP planner chains. The AI result is normalized into structured
Problem, Why This Score, scored finding impacts, Suggested Fixes, and More
Evidence Needed sections. When structured scored findings are present, the UI
shows an AI-reviewed health score and metric-score basis. Exact snapshot matches
are shown as current cached reviews; the latest older same-subject review can be
shown as previous-snapshot context until the current evidence is regenerated.
These reviewers preserve the same evidence shape and read-only posture as MCP
analysis tools, so a future server-side/MCP-backed reviewer can reuse the
category, severity, evidence, recommendation, confidence, score-basis, and
snapshot fields without changing the user contract.

The operator CLI adds a second client-side compatibility path without changing
hosted MCP authorization or tool schemas:

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  nopsai api request POST /v1/mcp --data -
```

This uses the selected CLI context and bearer token. The hosted MCP server still
resolves the caller subject and permissions through the existing API/AAA
middleware; the CLI introduces no privileged MCP transport.

`POST /v1/mcp` is also included in the generated CLI route catalog, so route
parity protects hosted MCP transport availability alongside the rest of the API:

```bash
nopsai api describe POST /v1/mcp
```

`nopsai.get_platform_version` returns the public `/version` build and
compatibility payload through authenticated hosted MCP. It has no arguments and
does not expose runtime configuration. This keeps assistant-side deployment
advice capability-aware without granting a new permission path.

Important enterprise boundaries:

- Hosted MCP does not elevate to admin. It uses the user's AAA subject, scoped
  grants, resource visibility, and route-compatible permissions.
- `nopsai.get_pipeline_run_output` remains guarded by `pipeline_run.read` and
  returns contract-validated `DocumentSpec`/`SpreadsheetSpec` source where
  applicable, plus generation timestamps, generation duration, and generation
  and render counts for operational auditing.
- Runtime-generated task outputs are visible to hosted MCP only as normal run
  detail/log metadata: output names, sensitivity flags, and byte sizes. Stored
  output values, including non-sensitive values, are not exposed through MCP
  run-log or run-detail tools.
- Pipeline run metadata exposes only non-sensitive runtime variable overrides.
  Sensitive include-variable overrides for child pipelines are omitted from MCP
  run detail output.
- `nopsai.list_dashboards`, `nopsai.get_dashboard`,
  `nopsai.list_dashboard_refreshes`,
  `nopsai.list_dashboard_refresh_schedules`, and `nopsai://dashboards` are
  guarded by `dashboard.list`/`dashboard.read` and return only dashboards
  visible to the current subject. `nopsai.refresh_dashboard` and
  `nopsai.run_dashboard_refresh_schedule` require `dashboard.refresh` and
  `confirm:true`. Dashboard write/source management remains available through
  guarded REST via `nopsai.call_api`, including publication-entry deletion for
  removing visible section cards while preserving source bindings and run audit
  records. Dashboard refresh source rows expose the refresh rollup status
  separately from the launched pipeline status and final output status.
- Pipeline, schedule, knowledge, webhook source, external trigger, and
  notification write-plan tools return `applies:false` plus GitOps file plans;
  they do not directly mutate product state.
- `nopsai.call_api` can mutate state only when `confirm:true` is supplied and
  the existing route/handler authorization allows the current subject.
- Dedicated runtime mutation tools such as run creation, approval decisions,
  rerun/cancel/delete, external trigger invocation, config sync/write, SMTP
  test, monitoring actions, and backup/cleanup operations also require
  `confirm:true`.
- Secret and credential domains should use metadata, encrypted GitOps payloads,
  references, or explicit write/rotation flows. Plaintext secret retrieval is
  not exposed as default assistant context.
- Internal run-service callbacks, public webhook delivery ingress, and UI
  rendering remain intentionally outside assistant mutation tools. Dedicated MCP
  tools explain those boundaries and point to safe alternatives.
- Destructive or external side-effect operations such as cleanup execution,
  backup deletion, SMTP testing, and approval decisions are exposed only through
  dedicated tools with explicit confirmation semantics. Runner dispatch
  pause/resume is now a dedicated confirmed tool.
