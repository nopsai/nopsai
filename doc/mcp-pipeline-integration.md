# MCP Integration

Nopsai uses MCP in two directions:

- During goal-based pipeline execution, the agent is an MCP client for approved
  external MCP profiles.
- For the AI Assistant and external clients, Nopsai exposes a first-party hosted
  MCP server at `POST /v1/mcp`.

The hosted MCP server is permission-bound through AAA, records tool audit
activity, and keeps write-oriented operations GitOps-compatible by returning
validated proposals instead of mutating production state directly.

Pipeline authors select approved MCP profiles:

```yaml
name: repo-review
container_image: ubuntu:latest

mcp_profiles:
  - github-readonly

steps:
  - name: review
    goal: Review this pull request
    mcp_profiles:
      - github-pr-review
```

MCP profiles only apply to goals. Explicit `mcp_profiles` on script steps,
script tasks, or include placeholders are rejected during validation.

## Registry Model

MCP servers are configured by admins under System > MCP. A server is an external
endpoint connection:

```yaml
mcp_servers:
  github:
    display_name: GitHub MCP
    enabled: true
    provider: github
    transport: streamable_http
    url: https://api.githubcopilot.com/mcp/x/all/readonly
    auth_type: bearer_token
    credential_ref: credential://system/mcp/github-readonly
    timeout: 30s
```

MCP profiles are approved bundles of server tools:

```yaml
mcp_profiles:
  github-pr-review:
    description: Read-only GitHub PR review tools
    enabled: true
    servers:
      - server: github
        tools:
          - "*"
```

Pipeline YAML must not define arbitrary MCP servers. It can only reference
configured profiles.

## GitOps

System config repositories can manage MCP the same way they manage LLM
profiles. Put the registry in `setting/system/mcp.yaml`:

```yaml
mcp_servers:
  github:
    display_name: GitHub MCP
    enabled: true
    provider: github
    transport: streamable_http
    url: https://api.githubcopilot.com/mcp/x/all/readonly
    auth_type: bearer_token
    credential_ref: credential://system/mcp/github-readonly
    timeout: 30s

mcp_profiles:
  github-pr-review:
    description: Read-only GitHub PR review tools
    enabled: true
    servers:
      - server: github
        tools:
          - "*"
```

Only system/global config repositories may define the MCP registry. Team config
repositories can reference approved `mcp_profiles` in their pipelines, but they
cannot define new MCP servers.

In database-backed deployments, Postgres and the system/global GitOps files are
the authoritative MCP registry. Runtime attempts to mirror MCP edits back into
the bootstrap `config.yml` are best-effort after database persistence, so an
immutable container image or read-only mounted bootstrap config does not block a
successful registry save or tool discovery. Operators should still commit the
desired MCP registry to the owning GitOps repository to avoid drift.

When a local MCP server runs outside the NopsAI pod or container, set `url` to an
address reachable from the API runtime, such as a LAN IP or
`host.docker.internal` when the platform provides it. `localhost` points at the
API container itself, and Docker Compose service names only work when the API is
attached to that Compose network.

MCP server/profile GitOps changes must also pass the commercial license policy
in [license-compliance.md](./license-compliance.md). A profile may reference a
customer-provided or externally hosted MCP server, but NopsAI must not bundle
third-party MCP server code, plugins, prompts, schemas, documentation, or data
unless the source license or commercial agreement allows enterprise redistribution
and the required notices are recorded.

System MCP server IDs can include an optional team path such as
`platform/ml/github`; the MCP page exposes the same placement control while
creating or editing servers and stores the resulting slash-scoped ID in the
system registry. Team-scoped MCP profile storage and REST APIs are available at
`GET|POST /v1/teams/{teamID}/mcp-profiles` and
`GET|PUT|DELETE /v1/teams/{teamID}/mcp-profiles/{profileName}` for callers with
`team.read` or `team.update` on the team resource. Team profiles compose approved
system MCP servers. Run preparation and agent launch merge team profiles over
the system catalog when the run belongs to that team. Team-owned MCP profile
definitions are managed through the team UI/API.

LLM, Agent, and knowledge-kind defaults live in
`config-repositories/teams/<team>/defaults.yaml`. MCP entries remain profile
definitions selected explicitly by pipelines, steps, or tasks; there is no
team-wide MCP runtime default selector.

Create the referenced bearer token under **Credentials** or sync its
encrypted envelope from `setting/system/credentials.yaml`. GitOps owns the
binding; plaintext remains write-only in the API/UI.

## Inheritance

Inheritance is additive and de-duplicated for goal execution:

```text
pipeline mcp_profiles + step mcp_profiles + task mcp_profiles
```

Task-step `mcp_profiles` act as defaults for goal tasks inside that step.
Pipeline-level defaults and task-step defaults do not make script tasks MCP
enabled.

When MCP profiles resolve for a goal, including pipeline-level defaults, the
agent requires at least one successful MCP tool call before accepting a final
action. The agent accepts only server/tool pairs in the resolved profile
allowlist; other MCP tool calls fail before a provider request is made.

## Runtime Flow

For a goal task, the agent:

1. Resolves the LLM profile.
2. Resolves allowed MCP profiles for the current scope.
3. Exposes selected MCP tools to the LLM as callable actions. A profile tool
   entry of `"*"` means all tools discovered from a configured read-only MCP
   server.
4. Executes requested MCP tool calls against external HTTP MCP servers.
5. Adds tool results back into the goal conversation.
6. Continues until the LLM returns a final Nopsai action.

The internal MCP action shape is:

```json
{
  "action": {
    "type": "CALL_MCP_TOOL",
    "mcp_tool_action": {
      "server": "github",
      "tool": "get_file",
      "arguments": { "path": "README.md" }
    }
  }
}
```

Final actions remain normal Nopsai actions such as `EXECUTE_COMMAND`,
`REPLACE_FILE`, or `RETURN_ANSWER`.

## Hosted MCP Server

The hosted MCP endpoint supports JSON-RPC `initialize`, `tools/list`,
`tools/call`, `resources/list`, and `resources/read`.
The assistant calls tools through the same hosted MCP JSON-RPC processor as
external clients. For curl-based debugging, an authenticated empty `POST` body
defaults to `tools/list`; explicit MCP clients should still send JSON-RPC
payloads with `Content-Type: application/json`.
Assistant conversation turns use the selected/default LLM profile to plan
hosted MCP calls first, then NopsAI validates the structured plan against the
current AAA subject, available tools, bounded arguments, and explicit mutation
confirmation before any tool executes.

High-value hosted tools include:

- guided setup:
  `nopsai.get_setup_status`, `nopsai.get_setup_preflight`,
  `nopsai.get_setup_templates`, `nopsai.plan_first_install_setup`, and
  confirmed `nopsai.bootstrap_first_install_setup`
- knowledge context search, list, and read:
  `nopsai.search_docs`, `nopsai.read_doc`,
  `nopsai.list_knowledge_contexts`, and `nopsai.get_knowledge_context`
- pipeline inventory and YAML inspection:
  `nopsai.list_pipelines`, `nopsai.search_pipelines`,
  `nopsai.get_pipeline`, and `nopsai.get_pipeline_knowledge_context`
- pipeline authoring proposals:
  `nopsai.validate_pipeline`, `nopsai.propose_pipeline_create`,
  `nopsai.propose_pipeline_update`, and reusable step create/update/delete
  GitOps plans
- run investigation:
  `nopsai.list_pipeline_runs`, `nopsai.get_pipeline_run`,
  `nopsai.get_pipeline_run_logs` with optional `include_children`, and
  `nopsai.analyze_pipeline_run_failure`
- confirmed run operations:
  `nopsai.run_pipeline`, `nopsai.list_run_approvals`,
  `nopsai.approve_run_approval`, `nopsai.reject_run_approval`,
  `nopsai.rerun_pipeline_run`, `nopsai.cancel_pipeline_run`, and
  `nopsai.delete_pipeline_run`
- GitOps-ready operational plans:
  schedule create/update/delete/enable/disable, knowledge context
  create/update/delete, Git webhook source changes, external trigger changes,
  notification mail/route changes, and config repository write/sync workflows
- confirmed operational actions:
  schedule run-now, external trigger invocation, SMTP test, monitoring saved
  views/alert rules/recommendations, and data backup/cleanup operations
- secrets, variables, credentials, runners, and admin operations:
  metadata-only secret/credential listing, secret encryption, confirmed
  value writes/rotations/deletes, scoped GitOps value plans, credential GitOps
  plans, runner compose/manifest/bootstrap generation, confirmed runner
  dispatch updates, access-grant workflows, audit reads, and admin user/service
  account/role/identity-provider workflows
- monitoring analytics:
  summary, run, pipeline, step, task, trigger, external-trigger, AI-usage,
  reliability, efficiency, security, runner-history, schedule AI usage,
  schedule/trigger performance, pipeline efficiency, pipeline/schedule
  comparisons, pipeline health, and optimization opportunity tools
- feature coverage:
  `nopsai.get_feature_capabilities` reports NopsAI feature areas, hosted MCP
  surfaces, REST/GitOps backing routes, required AAA actions, and current-user
  availability
- API bridge:
  `nopsai.call_api` calls guarded `/v1` REST routes as the current subject for
  compatibility coverage; mutating calls require `confirm:true`
- triggers, schedules, scopes, cost, statistics, LLM profiles, MCP profiles,
  system status, and dispatcher/runner status, dispatch, and ejection tools

Hosted MCP is always user-scoped: `tools/list`, `resources/list`, and
`tools/call` evaluate against the authenticated subject from the request. Tool
calls re-check the concrete resource when arguments identify one, and audit
records are written under the same subject/conversation.
`nopsai.list_pipeline_runs` mirrors the REST run list's lightweight
`final_output_status` summary for runs that define or store final outputs, so
MCP clients can distinguish waiting, not-generated, pending, generating,
success, failed, partial, cancelled, and partially cancelled output states
without reading generated content.
Pipeline final output reads stay on the existing `pipeline_run.read` boundary.
The output tool exposes stored contract-validated content together with
generation and render audit counts; PDF/HTML return `DocumentSpec` and Excel
returns `SpreadsheetSpec`. Dashboard outputs include `dashboard_target`
metadata when they publish into a dashboard section. It does not render
artifacts or bypass run authorization.
The API bridge rejects public/provider ingress and internal service routes, and
blocks default plaintext secret reads so secret/credential workflows stay
metadata-, reference-, encryption-, or explicit-write-oriented.
Dedicated policy tools explain why internal run callbacks, public webhook
delivery ingress, and UI rendering are intentionally not assistant mutation
surfaces.

Pipeline, reusable-step, schedule, knowledge-context, webhook-source,
external-trigger, notification, scoped secret/variable, and credential GitOps
tools validate input and return commit-ready file plans. For pipeline create
proposals, the explicit target `name` can fill a missing top-level YAML `name`;
if both are provided and disagree, validation still rejects the proposal. A
pipeline create response looks like:

```json
{
  "proposal_type": "pipeline_create",
  "applies": false,
  "pipeline_id": "team-1/services/api/deploy",
  "gitops": {
    "message": "Create Nopsai pipeline team-1/services/api/deploy",
    "files": [
      {
        "path": "pipelines/team-1/services/api/deploy.yaml",
        "content": "name: deploy\n...",
        "delete": false
      }
    ]
  }
}
```

The MCP response is not an applied change. Operators or automation should commit
the proposed files to the configured config repository review branch and run
GitOps sync, or use the existing REST API with the matching `pipeline.create` or
`pipeline.update` permission when direct database writes are intentionally
allowed.

Knowledge context traversal works at two levels:

- `nopsai.list_knowledge_contexts` and `nopsai.get_knowledge_context` enumerate
  managed knowledge documents the caller may read.
- `nopsai.get_pipeline_knowledge_context` parses stored or supplied pipeline
  YAML, walks pipeline-, step-, and task-level knowledge refs, returns readable
  managed documents, and marks repo-local docs as run-time-only because they are
  resolved from the run repository commit.

Hosted resources include `nopsai://docs`, `nopsai://knowledge-contexts`,
`nopsai://pipelines`, `nopsai://pipeline-runs`, `nopsai://triggers`,
`nopsai://schedules`, `nopsai://scopes`, `nopsai://statistics`,
`nopsai://costs`, `nopsai://features`, and system profile/status/dispatcher
resources. See [mcp-feature-coverage.md](./mcp-feature-coverage.md) for the
current full-feature coverage map and enterprise automation boundaries.

## Supported Scope

Current implementation:

- First-party hosted MCP server for assistant and external clients.
- External HTTP/streamable HTTP MCP servers.
- Admin-managed MCP servers and profiles.
- Bearer-token or no-auth server connections.
- Extra MCP server headers for provider-specific configuration.
- Tool discovery via `initialize` and `tools/list`.
- Tool execution via `tools/call`.
- Read-only profile enforcement by write-like tool-name rejection.
- Scope checks for profiles and servers.
- Hosted knowledge-context listing, reading, and pipeline context traversal.
- Hosted pipeline search across metadata and readable YAML definitions.
- Hosted dispatcher and runner health checks using dispatcher status and runner
  monitoring summaries.
- Hosted GitOps-ready pipeline create/update proposals with validation.
- Dedicated hosted tools for confirmed run mutations, schedule GitOps plans,
  knowledge context plans, webhook source/external trigger plans, config repo
  sync/drift/write, notifications, monitoring operations, and backup/cleanup.
- Guarded hosted API bridge for setup, reusable step, variable/secret,
  AAA/admin/audit, auth self-service, credential, runner-operation, and other
  compatible `/v1` routes.
- Runtime logs for selected profiles and called tools.

Future extensions can add stdio servers, sidecars, direct hosted write approvals,
rate limits, and richer per-tool audit records.
