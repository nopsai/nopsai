# Hosted MCP Feature Coverage

Hosted MCP exposes NopsAI as the current authenticated user. The assistant and
external MCP clients receive only the tools/resources allowed by that user's AAA
subject, and each tool call re-checks the specific resource identified by the
arguments.

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

`nopsai.call_api` is the broad compatibility bridge for remaining product
features. It calls guarded `/v1` REST routes as the current authenticated
subject, rejects public/provider ingress and internal service routes, blocks
default plaintext secret reads, and requires `confirm:true` before mutating
routes execute.

Coverage states:

- `first_class`: hosted MCP has direct tools/resources for the workflow
- `partial`: hosted MCP covers part of the workflow; remaining operations use
  REST/GitOps today
- `api_backed`: product APIs are reachable through `nopsai.call_api`, but a
  dedicated hosted MCP tool is still pending
- `contextual`: MCP provides backing data; UI rendering remains UI-owned

Current first-class coverage includes setup status/preflight/template planning
and confirmed bootstrap, pipeline inventory/search, pipeline YAML inspection and
validation, reusable step GitOps plans, pipeline knowledge-context traversal,
GitOps-ready pipeline create/update proposals, managed knowledge reads and write
plans, run/log analysis, confirmed run mutations, schedule inventory and GitOps
write plans, webhook source and external trigger plans, webhook-ingress policy
explanations, config repo sync/drift/write workflows, notification mail/route
plans, monitoring analytics/views/alerts/recommendation actions, data
backup/cleanup operations, scope inventory, secret/variable metadata and safe
write/GitOps plans, cost/statistics, LLM/MCP profile reads, system status,
credential metadata/rotation/GitOps plans, runner install/dispatch operations,
AAA/access/audit/admin workflows, and dispatcher/runner health.
The API bridge remains the compatibility surface for auth self-service and other
guarded `/v1` routes when the current user has the matching AAA permissions.

Important enterprise boundaries:

- Hosted MCP does not elevate to admin. It uses the user's AAA subject, scoped
  grants, resource visibility, and route-compatible permissions.
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
