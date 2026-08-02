# Nopsai API Guide

The core service exposes its REST API on `http://localhost:8080`. This guide summarises the high-impact endpoints that power day-to-day automation. All examples assume local development defaults.

Except for login, SSO discovery/callback/session exchange, token refresh,
logout, setup preflight, and runner bootstrap, API calls require a bearer token:

```bash
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/runs
```

For full JWT behavior, personal-token handling, claims, config, refresh-token storage, and service-token details, see [jwt-authentication.md](./jwt-authentication.md).

Public platform identity is available before and after initial setup:

```bash
curl http://localhost:8080/version
```

`GET /version` returns product and API versions, supported CLI and runner
ranges, capability IDs, and optional legacy release-manifest digest. It contains no
deployment configuration or credentials. Released CLIs use this endpoint to
reject incompatible mutating requests before sending them. See
[release-bundles.md](./release-bundles.md).

The `nopsai` CLI provides the same authenticated API surface without embedding
server behavior:

```bash
nopsai context add local --api http://localhost:8080
NOPSAI_TOKEN=nopat_<secret> nopsai api request GET /v1/runs
nopsai api request POST /v1/system/config/sync --data sync.json
nopsai api routes --output json
nopsai api call GET '/v1/runs/{runID}' --path runID=<run-id>
```

The CLI treats bearer tokens as opaque and all AAA, audit, route, MCP, and
resource authorization remains enforced by this API. Its generated catalog is
parity-tested against all registered method/path combinations, including
public, streaming, download, and internal service routes. See
[cli.md](./cli.md).

---

## Authentication

```bash
# Login with the local development bootstrap admin account
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"identifier":"admin@example.com","password":"admin"}' \
  http://localhost:8080/v1/auth/login

# Refresh an access token
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"<refresh-token>"}' \
  http://localhost:8080/v1/auth/refresh

# List enabled login providers for the UI
curl http://localhost:8080/v1/auth/providers

# Discover whether an email domain maps to enterprise SSO
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@company.com"}' \
  http://localhost:8080/v1/auth/discover

# Browser OIDC starts with a redirect to the identity provider
open "http://localhost:8080/v1/auth/oidc/corporate/start?return_to=/pipelineruns/main"

# GitHub uses the OAuth2 start/callback route because it does not issue an OIDC id_token.
open "http://localhost:8080/v1/auth/oauth2/github/start?return_to=/pipelineruns/main"

# The external callback creates a one-time code. The UI exchanges it for Nopsai tokens.
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"code":"<one-time-session-code>"}' \
  http://localhost:8080/v1/auth/session/exchange

# Current user and profile updates
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/auth/me
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"email":"new@example.com"}' \
  http://localhost:8080/v1/auth/email

# Create a personal token for API automation from an interactive session token
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"deployment script","expires_in_days":90}' \
  http://localhost:8080/v1/auth/personal-tokens

# Or choose an exact date, or explicitly choose no expiry
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"monthly report","expires_at":"2026-06-30"}' \
  http://localhost:8080/v1/auth/personal-tokens
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"long lived integration","never_expires":true}' \
  http://localhost:8080/v1/auth/personal-tokens

# Use the returned personal token for API calls
curl -H "Authorization: Bearer nopat_<secret>" http://localhost:8080/v1/runs

# Use a service account token for integration calls
curl -H "Authorization: Bearer nopsat_<secret>" http://localhost:8080/v1/runs

# List and revoke personal tokens
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/auth/personal-tokens
curl -X DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/auth/personal-tokens/<token-id>
```

- Local auth issues an access token and optional refresh token.
- Enterprise SSO uses OIDC Authorization Code Flow with PKCE, then exchanges a
  short-lived Nopsai login code for the same access/refresh token family as
  local login.
- Personal tokens are created from Profile/API auth routes, are returned only once, support `expires_in_days`, exact `expires_at`, or explicit `never_expires`, and use the same authorization as the owning user.
- Nopsai stores only personal-token hashes plus metadata, not the raw token value.
- Protected UI calls automatically attach the access token and retry once after refresh on `401`.
- Profile routes require authentication but do not require an extra AAA resource decision.

Identity Provider administration is available under System Access and requires
`iam.admin`:

```bash
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/admin/identity-providers

curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"oidc_enabled":true,"local_enabled":true,"auto_create_users":true,"default_role":"","domain_mappings":{"company.com":"corporate"}}' \
  http://localhost:8080/v1/admin/identity-providers

# local_enabled=false is rejected; local sign-in and the break-glass admin stay enabled.

curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"corporate","type":"oidc","display_name":"Company SSO","issuer":"https://idp.company.com","client_id":"client-id","client_credential_ref":"credential://system/oidc/corporate/client-secret","scopes":["openid","email","profile"],"allowed_email_domains":["company.com"],"team_claim":"teams","entitlement_sync":{"mode":"keycloak_team_roles","admin_base_url":"https://keycloak.example.com","realm":"company","admin_client_id":"nopsai-admin","admin_client_credential_ref":"credential://system/oidc/corporate/admin-client-secret","client_id":"nopsai","target_resource_type":"team"},"enabled":true}' \
  http://localhost:8080/v1/admin/identity-providers/corporate
```

For local SSO testing against real users and teams, see
[local-keycloak-sso.md](./local-keycloak-sso.md) and
`examples/sso/keycloak`. The fixture provides a Keycloak realm, a confidential
`nopsai` client, and seeded `admin`, `owner`, and `viewer` client-role
mappings. Multi-provider scenario configs for mock Entra ID, Okta, Google,
Keycloak claim mapping, and GitHub OAuth2 live under
`examples/sso/idp-test-pack`.

External-provider-created or linked users are marked with `external_managed` in
`GET /v1/admin/users`. Their `display_name`, authentication/provisioning/
authorization ownership fields, external provider, subject, external teams,
mapped NopsAI auth teams, and externally sourced roles are returned for
administration views. Local user-role and basic-role mutation endpoints can add
or remove local grants; provider sync owns only IdP-sourced grants. With
Keycloak entitlement sync, direct client roles drive provider-managed global
access roles and team client roles drive provider-managed scoped Basic roles.
Leave `default_role` empty for least-privilege SSO providers; set it only when
every auto-created SSO user should intentionally receive the same global role.

---

## Quick Start

```bash
# Refresh config repositories, pipelines, reusable steps, schedules, scopes, triggers, knowledge contexts, and system settings from Git
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/internal/config/sync
```

- Use this after updating the configuration repository or when bootstrapping a fresh database.
- The call is idempotent and can be triggered manually or by Git events.
- The caller needs `system.update` on `system:config-sync`.
- For the global GitOps entrypoint, use `PUT /v1/system/config-repo` with `scope_id=global`.
- System LLM profiles can be managed in the global config repo at `setting/system/llm_profile.yaml`.
- LLM profile providers are `gemini`, `lmstudio`, `openai`, `anthropic`,
  `groq`, `mistral`, `ollama`, `openrouter`, and `azure-openai`. Profiles can
  also set `timeout_seconds`, `max_tokens`, `temperature`, and provider-specific
  string values under `extra`.
- System Agent Profiles and the default agent profile can be managed in the global config repo at `setting/system/agent-profiles.yaml`.
- System MCP profiles can be managed in the global config repo at `setting/system/mcp.yaml`.
- Mandatory local-login and external identity-provider settings can be managed in the global config repo at `setting/system/auth.yaml`.
- GitHub App IDs, credential references, and installations can be managed in the global config repo at `setting/git-apps/github.yaml`. The internal git-bot URL remains a system/service setting.
- Runner defaults, runtime defaults, dispatcher routing, and assistant settings can be managed in the global config repo at `setting/system/runner.yaml`.
- Scheduled cleanup rules can be managed in the global config repo at `setting/system/data-management.yaml`.
- Encrypted system credential envelopes can be managed in the global config repo at `setting/system/credentials.yaml`.
- Managed knowledge context markdown files can be synced from `knowledge/<kind>/<team>/<document>.md`.

## Teams

Teams own applications, GitOps bindings, notification policies, and
team-scoped AI profiles. The current implementation stores teams and
applications in the compatibility `teams` table, but clients should use
`/v1/teams` for team administration.

```bash
# List teams and applications
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  'http://localhost:8080/v1/teams?include=applications'

# Create a team under root
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"platform","description":"Platform engineering"}' \
  http://localhost:8080/v1/teams

# Create an application under a team
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"runner-api","repo_url":"https://github.com/acme/runner-api"}' \
  http://localhost:8080/v1/teams/platform/applications

# Rename or move a team. Omitted parent fields keep the current parent;
# an explicit null parent moves the team to Global.
curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"platform-core","parent_team_id":null,"description":"Platform engineering"}' \
  http://localhost:8080/v1/teams/platform

# Update or move an application by using the target parent in the URL.
curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"runner-api","repo_url":"https://github.com/acme/runner-api"}' \
  http://localhost:8080/v1/teams/platform-core/applications/42
```

Team config repository and notification endpoints mirror the legacy team
routes:

- `GET|PUT|DELETE /v1/teams/{teamID}/config-repository`
- `GET|POST /v1/teams/{teamID}/config-repository/sync`
- `GET /v1/teams/{teamID}/config-repository/drift`
- `POST /v1/teams/{teamID}/config-repository/write`
- `GET|PUT|DELETE /v1/teams/{teamID}/notifications`

Team-scoped AI profile APIs let delegated team owners define LLM, Agent, and
MCP profiles without taking over the system-owned catalogs:

- `GET|PUT /v1/teams/{teamID}/llm-profiles`
- `PUT /v1/teams/{teamID}/llm-profiles/default`
- `PUT|DELETE /v1/teams/{teamID}/llm-profiles/{profileName}`
- `GET|POST /v1/teams/{teamID}/agent-profiles`
- `PUT /v1/teams/{teamID}/agent-profiles/default`
- `GET|PUT|DELETE /v1/teams/{teamID}/agent-profiles/{profileID}`
- `GET|POST /v1/teams/{teamID}/mcp-profiles`
- `GET|PUT|DELETE /v1/teams/{teamID}/mcp-profiles/{profileName}`

Reads require `team.read` on the resolved team resource. Mutations require
`team.update` on the changed team or application. Moving a team or application
also requires `team.create` on the destination parent scope and rejects
hierarchy cycles. Team profile rows carry config-repository metadata columns
and team config repositories import/export root `ai-profiles.yaml`; system
profile GitOps remains under `setting/system/*` today. Run preparation and
agent launch merge team profile definitions over the system catalogs when a run
belongs to that team or one of its applications.

## Dashboards

Team dashboards are operational views populated by validated pipeline final
outputs.

```bash
# List visible dashboards, optionally scoped to a team path
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  'http://localhost:8080/v1/dashboards?team=platform&q=health'

# Create a dashboard with two sections
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"team_path":"platform","slug":"engineering-health","title":"Engineering Health","sections":[{"section_key":"overview","title":"Overview"},{"section_key":"deployments","title":"Deployments"}]}' \
  http://localhost:8080/v1/dashboards

# Read the composed dashboard view
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/dashboards/<dashboard-id>/view

# Inspect publication history
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  'http://localhost:8080/v1/dashboards/<dashboard-id>/history?limit=50'

# Remove a visible dashboard entry card
curl -X DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/dashboards/<dashboard-id>/publications/<publication-id>

# Start a strict section refresh
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: refresh-deployments-1" \
  -d '{"scope":{"type":"section","section_key":"deployments"},"mode":"strict","timeout":"45m","max_concurrency":4}' \
  http://localhost:8080/v1/dashboards/<dashboard-id>/refresh

# Track refresh progress
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/dashboards/<dashboard-id>/refreshes/<refresh-id>

# Create a scheduled refresh
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Hourly deployments","cron":"0 * * * *","timezone":"Europe/Amsterdam","enabled":true,"scope":{"type":"section","section_key":"deployments"},"mode":"best_effort","timeout":"45m","max_concurrency":4}' \
  http://localhost:8080/v1/dashboards/<dashboard-id>/refresh-schedules

# Run a scheduled refresh immediately
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"confirm":true}' \
  http://localhost:8080/v1/dashboards/<dashboard-id>/refresh-schedules/<schedule-id>/run

# Manage a source binding
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"section_key":"deployments","pipeline_id":"deploy/payments-api","output_name":"deployment-summary","entry_key":"payments-api","run_scope":"prod","enabled":true,"required_for_refresh":true}' \
  http://localhost:8080/v1/dashboards/<dashboard-id>/sources
```

Pipeline YAML publishes into a dashboard with `type: dashboard` final outputs:

```yaml
output:
  items:
    - name: deployment-summary
      type: dashboard
      dashboard:
        ref: platform/engineering-health
        section: deployments
        entry_key: payments-api
        mode: series
        preset: auto
```

REST paths use dashboard UUIDs. Pipeline YAML and hosted MCP may use
`team/dashboard-slug` refs. Reads require `dashboard.list` or
`dashboard.read`; create/update/delete/source management and refresh use the
matching `dashboard.*` actions. Pipeline publication re-checks
`dashboard.publish` as the effective run subject before current publications or
history are written. Removing a visible dashboard entry archives that current
publication and writes a history event; source bindings, refresh records, and
pipeline runs remain auditable. Dashboard output supports `replace`, `append`,
`snapshot`, and `series` modes; `series` merges chart or time-series blocks
while deduping and retaining bounded points. Series-mode final outputs must
include at least one `chart` or `series` block with chart points. Refresh
supports dashboard, section, and source scope; strict mode blocks required
unavailable sources while best-effort records skips and continues. Dashboard
source bindings include
`run_scope` as part of their identity. Empty `run_scope` is the exact
default/unscoped run, not a legacy any-scope match; scoped trigger or schedule
runs only publish when an enabled source binding has the same scope.
`dashboard.preset` stays a generation hint for shape: `report` is
narrative-first, `table` makes a table primary, `status` emphasizes current
state, `timeline` orders events or series chronologically, `comparison`
presents side-by-side differences, `metrics` starts with headline numbers and
charts, `mixed` composes complementary operator blocks, and `auto` chooses the
smallest useful layout.
GitOps-managed dashboards live under `dashboards/` and templates under
`dashboard-templates/`, with source path, source commit, team ownership, drift,
and managed-state pruning metadata preserved.

## Assistant and Hosted MCP

For a user-facing guide to assistant capabilities and example chat prompts, see
[assistant-capabilities.md](./assistant-capabilities.md).

- `GET /v1/assistant/config` returns safe assistant configuration for the authenticated subject: enabled state, docs defaults, retention/limits, feature flags, action confirmation policy, and whether a dedicated assistant credential is configured. It does not return credential refs, API key env names, base URLs, or provider extras.
- `GET /v1/assistant/llm-profiles` lists safe, selectable LLM profile metadata for the authenticated assistant user without exposing credential refs, base URLs, or provider extras.
- `POST /v1/analysis/evaluate` asks the selected/default LLM profile for a
  structured second-pass evaluation of a client-provided redacted analysis
  snapshot. It is authenticated, enforces Assistant feature flags and LLM
  profile scope/credential resolution, returns provider/model/usage metadata,
  and does not create an Assistant conversation, execute hosted MCP tools, or
  read extra run/pipeline resources server-side. The UI prompt asks for scored
  findings and category-score basis so the modal can cache and display an
  AI-reviewed score for the submitted snapshot, or reuse the latest older
  same-subject review as clearly labeled previous-snapshot context.
- `POST /v1/assistant/conversations` creates a persistent assistant conversation for the authenticated subject.
- `GET /v1/assistant/conversations` lists the subject's conversations.
- `GET /v1/assistant/conversations/{id}` reads a conversation, messages, conversation-scoped memory, and assistant usage rollups.
- `POST /v1/assistant/conversations/{id}/messages` appends a user message, asks the selected/default LLM profile for a structured hosted MCP plan, validates the plan against current-user AAA, tool availability, argument limits, and mutation confirmation rules, executes allowed hosted MCP tools, quality-gates final LLM synthesis against MCP evidence, records tool and LLM activity plus per-message token/duration usage, updates memory, and returns an assistant reply. Assistant turns do not use static normal-language routing: if the LLM planner is unavailable or returns an invalid plan, no hosted MCP tools run and no changes are applied. Generated YAML and trigger/schedule edits are proposals only.
- `POST /v1/assistant/conversations/{id}/summarize-memory` updates conversation-scoped memory.
- `POST /v1/mcp` exposes Nopsai-hosted MCP JSON-RPC operations: `initialize`, `tools/list`, `tools/call`, `resources/list`, and `resources/read`.

Assistant message payloads include a `usage` object with estimated visible
`content_tokens`, LLM `prompt_tokens`, `completion_tokens`, `total_tokens`,
`estimated`, `duration_ms`, and `llm_calls`. User messages use content-token
estimates. Assistant replies sum planner/synthesis provider usage when
available and fall back to estimates when the provider omits usage metadata.
Conversation payloads include matching usage rollups for monitoring views.
The Prometheus exporter also exposes `nopsai_assistant_tokens_total`,
`nopsai_assistant_message_duration_seconds_total`, and
`nopsai_assistant_llm_calls_total`.
Global monitoring AI usage also adds assistant chat message usage from these
stored fields as the `assistant_chat` feature, with
`assistant_chat_tokens` and `assistant_chat_messages` in the response. Run,
pipeline, step, task, schedule, provider, and model-scoped AI usage filters
remain scoped to recorded run AI usage events.

The hosted MCP is first-party and permission-bound. It is separate from the
external MCP registry under `setting/system/mcp.yaml`, which defines
third-party MCP servers Nopsai can connect to. Hosted MCP and assistant tool
execution use the current authenticated AAA subject; they do not elevate to a
global assistant/admin identity.
Assistant tool execution goes through the same hosted MCP JSON-RPC request
processor as external clients, so chat answers are grounded in the
permission-filtered Nopsai tool/resource surface.
Assistant feature flags in `setting/system/runner.yaml` decide which broad
capability families are globally available. AAA still decides the specific
resources and actions the current user can read or execute. Runtime and admin
execution tools are hidden unless `assistant.features.action_execution` is
enabled, and confirmed mutation tools still require existing API, AAA, and
audit checks.

```bash
# Quick debugging: an empty body defaults to tools/list instead of a parse error.
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/mcp

# Explicit JSON-RPC initialize.
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"dev"}}}' \
  http://localhost:8080/v1/mcp

# List tools available to the current authenticated subject.
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  http://localhost:8080/v1/mcp

# Ask for the current user's Nopsai feature coverage.
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nopsai.get_feature_capabilities","arguments":{"include_api_routes":false}}}' \
  http://localhost:8080/v1/mcp
```

Hosted MCP tools cover Nopsai operational context, including guided setup,
knowledge-context search/list/read/write plans, pipeline inventory/search,
pipeline knowledge-context traversal, pipeline YAML validation, reusable step
GitOps plans, GitOps-ready pipeline create/update proposals, pipeline run
status/log analysis and confirmed mutations, triggers, webhook sources,
external triggers, schedules, config repo sync/drift/write workflows,
notifications, monitoring analytics and operations, data backup/cleanup
operations, scopes, secret/variable metadata and safe write/GitOps flows,
usage/cost, LLM/MCP profiles, credential metadata/rotation/GitOps plans, runner
install/dispatch workflows, AAA/access/audit/admin workflows, system status, and
dispatcher/runner status.
`nopsai.get_feature_capabilities` reports the current user's full NopsAI
feature coverage map, including hosted MCP tools/resources, backing REST/GitOps
routes, and required AAA actions. The same data is available as the
`nopsai://features` MCP resource.
`nopsai.call_api` is the guarded compatibility bridge for REST-backed features
that do not need richer first-class UX. It calls allowed `/v1` routes as the
current authenticated subject, rejects public/provider ingress and internal
service routes, blocks default plaintext secret reads, and requires
`confirm:true` before mutating routes execute. Dedicated policy tools explain
why internal run callbacks, public webhook delivery ingress, and UI rendering
are not assistant mutation surfaces.
GitOps write tools return `applies:false` with a commit-ready `gitops.files`
payload; they do not save product state directly.

---

## First-install Setup

The UI setup wizard uses `/v1/setup/*` to bootstrap an empty database. The
public preflight endpoint is available before login so the UI can explain
missing database, master-key, or JWT configuration. Other `GET` setup routes
require `system.read` on `system:config`; `POST` setup routes require
`system.update` on `system:config`.

```bash
# Public readiness check before login
curl http://localhost:8080/v1/setup/preflight

# Current setup status, health checks, and GitHub guidance
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/setup/status

# Preview starter GitOps files for selected repositories
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/setup/templates?profile=team&repositories=acme/service-api"

# Apply starter setup
curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "profile": "team",
    "generate_secrets": true,
    "seed_starter_database": true,
    "seed_llm_profile": true,
    "sync_config_repository": false,
    "mcp_examples": false,
    "config_repository": {
      "repo_url": "https://github.com/acme/nopsai-config.git",
      "branch": "main",
      "base_path": "",
      "enabled": true
    },
    "repository_teams": [
      {"name": "platform", "repositories": ["acme/service-api"]},
      {"name": "applications", "repositories": ["acme/web-app"]}
    ],
    "repositories": ["acme/service-api"],
    "llm_profile": {
      "name": "standard",
      "provider": "lmstudio",
      "model": "qwen3-coder",
      "base_url": "http://lmstudio:1234",
      "allowed_scopes": ["dev", "prod"]
    },
    "users": [
      {
        "email": "alice@example.com",
        "role": "owner",
        "team": "platform",
        "password": "temporary-password"
      }
    ]
  }' \
  http://localhost:8080/v1/setup/bootstrap
```

The UI sends `profile: "team"` as a compatibility value, but the operator
experience no longer asks for a starter profile. Repository teams are used for
starter run teams and user role assignment. If `seed_llm_profile` is false,
the bootstrap response includes a warning that AI-enabled pipelines may not work
until an LLM profile is configured. For the full operator flow, see
[first-install-wizard.md](./first-install-wizard.md).

---

## System Data Management

System data management is exposed in the UI at **System > Data Management**.
The API requires `system.read` on `system:config` for list/download/preview
operations and `system.update` on `system:config` for backup creation, deletion,
cleanup execution, and schedule changes.

```bash
# List backup history
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/data/backups

# Create a downloadable backup: full, runs, or logs
curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"backup_type":"full"}' \
  http://localhost:8080/v1/system/data/backups

# Download or delete a backup
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/data/backups/<backup-id>/download
curl -X DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/data/backups/<backup-id>

# Preview and execute a cleanup
curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target":"runs","mode":"keep_last","keep_last":30,"backup_before_cleanup":true}' \
  http://localhost:8080/v1/system/data/cleanup/preview

curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target":"runs","mode":"keep_last","keep_last":30,"backup_before_cleanup":true}' \
  http://localhost:8080/v1/system/data/cleanup/run
```

Backup records are stored in `data_backups`; files are compressed JSONL exports
written to `data_backup_dir` / `DATA_BACKUP_DIR`, defaulting to `/data/backups`.
Supported backup scopes are `full`, `runs`, and `logs`. Each record stores
status, file path/name, size, SHA-256 checksum, requester, timestamps, and error
details.

Cleanup supports:

- `target: "runs"` with `mode: "keep_last"`, `older_than_days`, or `all_terminal_runs`
- `target: "logs"` with `mode: "older_than_days"` or `all_logs`
- run cleanup only deletes terminal runs; `pipeline_runs` cascade rules remove related tasks, steps, approvals, checkpoints, logs, and run knowledge snapshots
- cleanup jobs store preview counts, deleted row counts, optional backup ID, status, requester, and errors in `data_cleanup_jobs`

Scheduled cleanup rules are managed through:

```bash
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/data/cleanup/schedules

curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Weekly cleanup","target":"runs","mode":"keep_last","keep_last":30,"backup_before_cleanup":true,"cron_expression":"0 2 * * 0","timezone":"UTC","enabled":true}' \
  http://localhost:8080/v1/system/data/cleanup/schedules

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/data/cleanup/schedules/<schedule-id>/run
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/data/cleanup/schedules/<schedule-id>/disable
```

The cleanup worker polls due schedules, claims them with `FOR UPDATE SKIP
LOCKED`, advances `next_run_at`, executes the cleanup job, and stores the latest
status/counts on `data_cleanup_schedules`.

The system/global GitOps repository may own scheduled cleanup definitions in
`setting/system/data-management.yaml`:

```yaml
cleanup_schedules:
  - name: Weekly cleanup
    target: runs
    mode: keep_last
    keep_last: 30
    backup_before_cleanup: true
    cron_expression: "0 2 * * 0"
    timezone: UTC
    enabled: true
```

Only schedule definitions are GitOps-managed. Backup files, backup records, and
cleanup job history remain runtime data.

---

## Monitoring And Dispatcher Runtime

```bash
# Monitoring page data: dispatcher service health, runner summary, and visible active runs
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/monitoring/dispatcher

# Access-filtered monitoring aggregates
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/monitoring/summary?from=2026-06-01T00:00:00Z&to=2026-06-11T00:00:00Z&teamId=42"
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/monitoring/runs/analytics?status=failure&repo=acme/app"
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/monitoring/pipelines/performance?triggerSource=schedule"
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/monitoring/ai-usage?pipelineName=release"
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/monitoring/ai-usage?runId=<pipeline-run-id>"
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/monitoring/runners/history?from=2026-06-01T00:00:00Z&to=2026-06-11T00:00:00Z"

# Monitoring saved views, alert rules, alert events, and recommendations
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/monitoring/views
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Failure view","visibility":"private","filters":{"status":"failure","windowDays":30},"columns":[]}' \
  http://localhost:8080/v1/monitoring/views
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/monitoring/alert-rules
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Failure rate","enabled":true,"visibility":"workspace","severity":"warning","metric":"failure_rate","comparator":"gt","threshold":0.1,"window_seconds":3600,"filters":{"triggerSource":"external"}}' \
  http://localhost:8080/v1/monitoring/alert-rules
curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/monitoring/alert-rules/<rule-id>/evaluate
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/monitoring/alert-events
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/monitoring/recommendations?status=open

# Prometheus scrape endpoint
curl http://localhost:8080/metrics

# Allow-listed platform log sources and a bounded redacted tail
curl -H "Authorization: Bearer $NOPSAI_TOKEN" http://localhost:8080/v1/system/logs/sources
curl -H "Authorization: Bearer $NOPSAI_TOKEN" "http://localhost:8080/v1/system/logs/sources/dispatcher/tail?lines=500"

# Authenticated SSE (use fetch streaming in browser clients)
curl -N -H "Authorization: Bearer $NOPSAI_TOKEN" -H "Accept: text/event-stream" \
  "http://localhost:8080/v1/system/logs/sources/dispatcher/stream?tail=500"

# Scope choices for the runner install UI
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/dispatcher/scopes

# Generate a one-time runner install command
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/system/dispatcher/runner-bootstrap-command?runner_id=runner-prod-1&runner_scopes=prod,dev&runner_capacity=2&dispatcher_grpc_address=nopsai-dispatcher.example.com:9090"

# Generate a one-time runner install command with selected private registry access
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/system/dispatcher/runner-bootstrap-command?runner_id=runner-prod-1&runner_scopes=prod&registry_credential_ref=credential://system/registry/ghcr"

# Generate a one-time Kubernetes runner install command
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/system/dispatcher/kubernetes-runner-bootstrap-command?runner_id=k8s-runner-prod-1&runner_scopes=prod&runner_capacity=10&namespace=nopsai-runs&dispatcher_grpc_address=nopsai-dispatcher.example.com:9090"

# Generate raw Kubernetes runner YAML for GitOps automation
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/system/dispatcher/kubernetes-runner-manifest?runner_id=k8s-runner-prod-1&runner_scopes=prod&runner_capacity=10&namespace=nopsai-runs&dispatcher_grpc_address=nopsai-dispatcher.example.com:9090"
```

- `GET /v1/monitoring/dispatcher` is authenticated. It returns dispatcher-backed service status, runner totals, sanitized runner rows, queue depth, and active runs. Active run entries are filtered with `pipeline_run.list`, so users only see runs they can list through their team/repository access.
- `GET /v1/monitoring/summary` returns executive totals: run status counts, success/failure rates, duration percentiles, longest run, runtime consumed, step/task counts, external trigger invocations, notification failures, LLM token totals, and runner utilization.
- `GET /v1/monitoring/runners/history` returns hourly runner capacity, active-job, inflight-job, queued-job, and utilization buckets from `runner_metric_snapshots`. Snapshots are sampled opportunistically when Monitoring reads dispatcher status or the summary endpoint.
- `GET /v1/monitoring/runs/analytics` returns runs over time, status split, duration/queue/end-to-end percentiles, longest runs, rerun/timeout counts, failure buckets, heatmap cells, and a recent run table.
- `GET /v1/monitoring/pipelines/performance`, `/steps/performance`, and `/tasks/performance` return backend-computed average, median, p95, p99, max, total duration, success/failure rates, and queue-time metrics where applicable.
- `GET /v1/monitoring/triggers/analytics` and `/external-triggers/analytics` return trigger-source reliability plus external trigger invocation, caller, idempotency, last-fired, rate-limit violation, and error aggregates.
- `GET /v1/monitoring/ai-usage`, `/reliability`, `/efficiency`, and `/security` return LLM usage with exact/estimated token splits, incident/reliability, token efficiency, and governance aggregates for the same filtered run set. The LLM Usage response includes totals plus `by_pipeline`, `by_step`, `by_task`, `by_feature`, `by_profile`, `by_model`, `by_subject`, trend, and top-token-run rows. Global AI usage adds assistant chat message tokens as the `assistant_chat` feature and exposes `assistant_chat_tokens`/`assistant_chat_messages`; run-scoped drilldowns stay limited to run AI usage events. Efficiency recommendations are also persisted into `monitoring_recommendations`.
- `GET|POST|PUT|DELETE /v1/monitoring/views` manages owner-scoped saved views. Updating a config-repo-managed view stores a database override, and deleting one removes the database row; the next GitOps sync can replace or recreate it unless the change is pushed to GitOps.
- `GET|POST|PUT|DELETE /v1/monitoring/alert-rules`, `POST /v1/monitoring/alert-rules/{ruleID}/evaluate`, and `GET /v1/monitoring/alert-events` manage alert rules and persisted evaluation events. Updating a config-repo-managed alert rule stores a database override, and deleting one removes the database row; the next GitOps sync can replace or recreate it unless the change is pushed to GitOps. The first evaluator supports `failure_rate`, `p95_duration_seconds`, `queued_jobs`, `runner_utilization`, `ai_tokens`, and `external_trigger_failures`.
- `GET /v1/monitoring/recommendations`, `POST /v1/monitoring/recommendations/{recommendationID}/acknowledge`, and `POST /v1/monitoring/recommendations/{recommendationID}/resolve` manage persisted recommendation workflow status.
- Agents record LLM usage with `POST /v1/internal/runs/{runID}/ai-usage` using an agent service JWT. The endpoint stores run, step, task, provider, model, LLM profile, token totals, metadata, and a per-run usage summary. Run list/detail responses expose that summary as `ai_usage`, while detail step/task rows include their own `ai_usage` totals for API compatibility. Provider token metadata is used when available; otherwise the agent records an estimated token count with `metadata.estimated_tokens=true`. Agent-reported metadata may include `prompt_sha256`, `prompt_bytes`, `estimated_input_tokens`, `cached_input_tokens`, `uncached_input_tokens`, `cache_write_tokens`, `stable_prefix_tokens`, `dynamic_context_tokens`, `static_context_sha256`, `static_context_cache_key`, `cache_identity_sha256`, `prompt_schema_version`, `execution_mode`, `logical_session_id`, `provider_state_id`, `provider_state_supported`, `provider_state_used`, `provider_state_mode`, `prompt_cache_supported`, `prompt_cache_hit`, `prompt_cache_mode`, `history_revision`, `workspace_revision`, `knowledge_revision`, `policy_revision`, `effective_policy_snapshot_hash`, `policy_merge_mode`, `policy_precedence_version`, `shared_file_count`, `shared_file_bytes`, `workspace_tool_call_count`, and `workspace_tool_result_bytes`; prompt bodies and shared file contents are not stored in AI usage events. Pipeline final output generation is recorded as the `pipeline_final_output` feature.
- Agents record declared task runtime outputs with `POST /v1/internal/runs/{runID}/steps/{stepName}/tasks/{taskName}/outputs` using an agent service JWT. Normal run-detail/API responses expose only output metadata (`name`, `sensitive`, `size_bytes`) on task rows; values remain run-state/internal data, and sensitive values are encrypted before persistence. The handler writes an audit log entry with output names and sizes but never output values. Hosted MCP run-log reads see the same audit metadata and no plaintext runtime output values.
- `GET /v1/internal/runs/{runID}/policy-revision` remains available for diagnostics and audit. It returns `run_start_policy_revision`, `current_policy_revision`, `blocking_context_count`, and the current blocking knowledge snapshots used to calculate the revision. Active agent runs use their pinned scope snapshots as the correctness boundary; emergency policy response cancels the run instead of mutating policy in place.
- Monitoring aggregate endpoints accept shared query parameters: `from`, `to`, `teamId`, `pipelinePath`, `pipelineName`, `repo`, `runId`, `branch`/`ref`, `commitSHA`, `triggerSource`, `status`, `requestedByType`, `requestedById`, `effectiveSubjectType`, `effectiveSubjectId`, `externalTriggerId`, `scheduleId`, `minDurationSeconds`, `maxDurationSeconds`, and `compare=previous_period`. The UI fetches the shifted previous window and renders regression deltas on Monitoring tabs when comparison is enabled. Pipeline Runs usage links open `/monitoring/ai-usage?runId=<pipeline-run-id>` and use an all-time window for that run-scoped drilldown.
- Monitoring aggregate endpoints first load candidate run IDs in Postgres, filter them through AAA with `pipeline_run.list`, then aggregate only visible run IDs. External trigger analytics also filters trigger-only rows with `external_trigger.read` so failed invocations that did not create runs are still governed.
- `GET /metrics` emits Prometheus text format. Metrics are public by default for compatibility; set `metrics_require_auth: true` or `METRICS_REQUIRE_AUTH=true` to require a bearer token. Metrics include DB-backed identity provider configuration/capability, authorization grant ownership, pipeline, output, final-output generation duration, run duration, notification, trigger, LLM, runner, approval, Knowledge Context connection/sync, and audit series plus in-memory System Logs active/opened streams, provider reconnect/error, redaction, and dropped-line counters.
- System Logs source discovery is AAA-filtered with `system_log.read` on `system_log:<sourceID>`. Tail and stream endpoints accept registry IDs only. SSE emits `status`, signed-cursor `log`, and `reset` events; stream heartbeats are comments. See [system-logs.md](./system-logs.md).
- `GET /v1/system/dispatcher/scopes` returns existing scope names from runner defaults, dispatcher routing, variables, secrets, and run history. It is used by the runner install UI for multi-select scope choices.
- `GET /v1/internal/dispatcher/routing` is dispatcher-internal. The live dispatcher polls it with a service-auth JWT and updates its in-memory routing table without a restart.
- `POST /v1/internal/registry-auth/docker` remains available for compatibility with older runner binaries. Runner-specific internal service subjects are bound to the requested `runner_id`; legacy generic `runner` and `agent` subjects remain accepted during upgrades. Current Docker runner installs use the selected `docker_config_json` only at bootstrap time, pass the merged config to the runner as `NOPSAI_REGISTRY_DOCKER_CONFIG_B64`, and resolve per-image Docker `RegistryAuth` from that local env-carried config without calling NopsAI during image pulls.
- Runner install command generation, Kubernetes manifest generation, runner dispatch pause/resume, and runner ejection remain under `System > Dispatcher` and require dispatcher runner management access. `DELETE /v1/system/dispatcher/runners/{runnerID}` removes the dispatcher registration, removes configured dispatcher routing references for that runner ID, blocks the same runner ID from registering again, and disconnects any live runner stream. The ejected runner ID blocklist wins over later GitOps routing syncs at runtime; remove the ID from `setting/system/runner.yaml` to keep declarative routing clean. When replacing a control plane, rotate `SERVICE_JWT_SIGNING_KEY` and `DISPATCHER_TLS_SECRET` or carry forward `ejected_runner_ids`; a fresh dispatcher that reuses the old trust material still trusts old runner definitions.
- Docker and Kubernetes install commands use single-use download tokens. Both bootstrap-command endpoints download shell scripts; the Kubernetes script writes the generated YAML to a temporary file before `kubectl apply`.
- Docker and Kubernetes bootstrap-command endpoints accept repeated `registry_credential_ref` or comma-separated `registry_credential_refs` query parameters. The API response contains selected references and registry hosts, but not registry secret values.
- Runner install endpoints accept optional `dispatcher_grpc_address` to override the dispatcher endpoint for that generated command or manifest without changing persisted runtime config. Kubernetes install commands wait for rollout and print pod/deployment/log diagnostics when the runner does not become ready.

---

## Pipeline Notifications

System mail settings:

```bash
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/notifications/mail

curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "from": "nopsai@example.com",
    "smtp": {
      "host": "smtp.example.com",
      "port": 587,
      "start_tls": true,
      "username": "nopsai@example.com",
      "password_credential_ref": "credential://system/mail/smtp-primary"
    }
  }' \
  http://localhost:8080/v1/system/notifications/mail

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"to":"operator@example.com"}' \
  http://localhost:8080/v1/system/notifications/mail/test
```

The test endpoint sends the same multipart HTML/plain-text format used by
operational mail. Its configuration-verification message shows the SMTP
endpoint, security mode, authentication status, sender, recipient, environment,
and generation time without exposing credentials. Optional `subject` and
`body` fields remain supported; `body` is rendered as an escaped test note.

Pipeline notification messages use multipart HTML and plain text. Failure
messages include the failed or last active step/task, counts for passed and
total steps, per-step task progress, repository/run links, and up to five
deduplicated error lines. Log excerpts are length-limited, HTML-escaped, and
redact common password, secret, token, authorization, and API-key patterns.

Mail presentation and links are configured through **System > Config** or
`setting/system/runner.yaml`:

| Runtime setting | Purpose |
| --- | --- |
| `public_url` | Browser-reachable application URL used for run links and the default logo URL. |
| `runtime_output_max_bytes` | Per-file byte limit for task runtime outputs written under `/nopsai/outputs`; applies to new agent runs. |
| `notification_mail_logo_url` | Optional absolute mail logo URL. |
| `notification_mail_website_url` | Optional footer website; defaults to `public_url`. |
| `notification_mail_support_url` | Optional footer support link. |
| `notification_mail_footer_address` | Optional organization/legal address shown in the footer. |

Only absolute `http` and `https` URLs are rendered. When no public URL is
configured, the message remains complete but omits the run link and remote
logo.

Team notification routes:

```bash
curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "routes": [
      {
        "name": "release failures",
        "enabled": true,
        "recipients": {
          "include": {
            "teams": ["same_team"],
            "users": ["release@example.com"]
          },
          "exclude": {
            "users": ["quiet@example.com"]
          }
        },
        "events": {
          "failure": true,
          "success": false,
          "pending": false,
          "waiting_approval": false,
          "approval_requested": false,
          "approval_rejected": true,
          "cancelled": true
        },
        "filters": {
          "branches": {
            "include": ["main", "release/*"],
            "exclude": ["dependabot/*"]
          }
        },
        "delivery": {
          "channels": ["mail"],
          "throttle": {
            "dedupe_window": "10m",
            "max_per_run": 5
          }
        }
      },
      {
        "name": "approval requests",
        "enabled": true,
        "recipients": {
          "include": {
            "teams": ["team-1/release-approvers"]
          }
        },
        "events": {
          "waiting_approval": true,
          "approval_requested": true,
          "approval_rejected": true
        },
        "filters": {
          "pipelines": {
            "include": ["team-1/services/api/*"]
          }
        },
        "delivery": {
          "channels": ["mail"],
          "throttle": {
            "dedupe_window": "15m",
            "max_per_run": 3
          }
        }
      }
    ]
  }' \
  http://localhost:8080/v1/teams/team-1/notifications
```

GitOps:

- The global config repo may define `setting/system/auth.yaml`.
- The global config repo may define `setting/system/mail.yaml`.
- The global config repo may define team notification policies with one or more named routes at `config-repositories/teams/<team>/notifications.yaml`.
- A team-scoped config repo may define `config-repositories/teams/<team>/notifications.yaml` with one or more named routes for its bound team.
- SMTP passwords are never stored in GitOps; only
  `smtp.password_credential_ref` is synced.

---

## Access Control

NopsAI calls the internal AAA service for authorization decisions and layers product roles on top of the low-level policy model.

Predefined product roles:

- `viewer`: read-only access to team metadata, pipelines, schedules, runs, logs, triggers, repository metadata, step metadata, knowledge context metadata/content, config repository metadata, secret metadata, and variable metadata
- `developer`: viewer permissions plus pipeline create/update/execute, schedule create/update/execute, runtime `*.use` permissions, run approval, rerun/cancel, trigger updates, secret value writes, variable writes, repository updates, scope updates, reusable step usage, knowledge context usage, runner usage, and config repository usage
- `owner`: developer and viewer permissions plus all scoped non-admin actions, delete operations, secret and variable value reads, and permission management inside the owned scope
- `admin`: platform-wide access through the normal AAA `Check` path, with sensitive actions still audited

Important behavior:

- Product roles are expanded to low-level AAA permissions when the grant is created.
- Team grants inherit to child teams, apps, pipelines, schedules, runs, repository-associated runs, triggers, repositories, scoped secrets, scoped variables, reusable steps, and knowledge contexts under that team path. Repository-associated runs are resolved through app `repository_full_name` metadata and the run `team_id` when present.
- Runtime resource use is checked with the original caller identity: manual runs use the user, Git-triggered runs use the repository, and internal dispatcher calls do not inherit pipeline-owner permissions.
- Approval decisions check `approval.approve` against the teams assigned by the approval step. Pending approval runs are listable/readable by assigned approvers for decision-making without granting log read or unrelated pipeline ownership.
- `developer` can write secret values but cannot read them.
- `developer` and `viewer` cannot manage ACLs.
- `owner` cannot grant `admin`.
- `admin` is platform-scoped only.
- Deny rules still win before allows.
- Denied requests and sensitive allowed requests are still audit logged.
- If the standalone AAA service is briefly unavailable, `nopsai` falls back to an in-process evaluator that reads the same Postgres policy tables.

Supported access-grant subject types:

- `user`
- `auth_team`
- `repository`
- `trigger`
- `service_account`
- `internal_service`

Team grants may use `resource_type: "team"` and target team paths, not numeric `team_id` values. Example: `/payments/backend`.

---

## Service Accounts

Service accounts are token-only identities for integrations and automation.
They are stored alongside users for lifecycle and status management, but they
cannot use password login and do not have password hashes. Their bearer tokens
use the `nopsat_` prefix and authorize as AAA `service_account` subjects.
System/global GitOps repositories can declare service-account identities and
grants with the `service_accounts` key in `access/*.yaml`; token material is
still created, rotated, and revoked through these runtime admin APIs or the
System Access page.

```bash
# Create a service account and receive the initial token once
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "sub":"deploy-bot",
    "email":"platform@example.com",
    "role":"nopsai-deployer",
    "token_name":"github-actions"
  }' \
  http://localhost:8080/v1/admin/service-accounts

# List service accounts
curl http://localhost:8080/v1/admin/service-accounts

# Rotate a token
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"name":"june-rotation","expires_in_days":90}' \
  http://localhost:8080/v1/admin/service-accounts/{serviceAccountID}/tokens

# Revoke a token
curl -X DELETE \
  http://localhost:8080/v1/admin/service-accounts/{serviceAccountID}/tokens/{tokenID}
```

Service account administration endpoints require `iam.admin`:

- `GET /v1/admin/service-accounts`
- `POST /v1/admin/service-accounts`
- `PUT|PATCH /v1/admin/service-accounts/{serviceAccountID}`
- `DELETE /v1/admin/service-accounts/{serviceAccountID}`
- `GET|POST /v1/admin/service-accounts/{serviceAccountID}/tokens`
- `DELETE /v1/admin/service-accounts/{serviceAccountID}/tokens/{tokenID}`
- `POST|DELETE /v1/admin/service-account-roles`

---

## External Triggers

External triggers are authenticated pipeline entrypoints for integrations. They
accept normal user bearer tokens and service-account tokens; production
integrations should use service accounts.

```bash
# Create an authenticated trigger endpoint
curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id":"deploy-prod",
    "name":"Deploy production",
    "description":"ServiceNow-approved production deploy",
    "enabled":true,
    "pipeline":"platform/prod/platform-maintenance",
    "scope":"prod",
    "run_team_path":"platform/prod",
    "allowed_callers":[{"type":"service_account","id":"servicenow-prod"}],
    "variable_mapping":{"VERSION":"payload.version"},
    "payload_schema":{
      "type":"object",
      "required":["version"],
      "properties":{"version":{"type":"string"}}
    },
    "rate_limit":{"per_minute":30}
  }' \
  http://localhost:8080/v1/external-triggers

# Invoke it with a service account token
curl -X POST \
  -H "Authorization: Bearer nopsat_<secret>" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type":"servicenow.change.approved",
    "idempotency_key":"servicenow.change.approved:<SOURCE_EVENT_ID>",
    "variables":{"VERSION":"1.2.3"},
    "payload":{"version":"1.2.3"}
  }' \
  http://localhost:8080/v1/external-triggers/deploy-prod/invoke
```

CRUD endpoints:

- `GET /v1/external-triggers`
- `POST /v1/external-triggers`
- `GET /v1/external-triggers/{id}`
- `PUT|PATCH /v1/external-triggers/{id}`
- `DELETE /v1/external-triggers/{id}`
- `GET /v1/external-triggers/{id}/invocations`
- `POST /v1/external-triggers/{id}/invoke`

GitOps:

- External trigger manifests live under `external-triggers/*.yaml`.
- Config sync imports, updates, and prunes GitOps-managed external triggers.
- Config repository drift/push exports UI-created database triggers back to `external-triggers/`.
- Direct CRUD is allowed when AAA permits. Updating a GitOps-managed external
  trigger stores a database override, and deleting one removes the database row;
  the next GitOps sync can replace or recreate it unless the change is pushed to
  the config repository.

Example manifest:

```yaml
id: deploy-prod
name: Deploy prod from ServiceNow
enabled: true
pipeline: platform/prod/platform-maintenance
scope: prod
run_team_path: platform/prod
allowed_callers:
  - type: service_account
    id: servicenow-prod
variable_mapping:
  VERSION: payload.version
payload_schema:
  type: object
  required:
    - version
rate_limit:
  per_minute: 10
```

Invoke responses return:

```json
{
  "run_id": "9e40b9b1-9f67-41f5-9c7e-6d7e3ef6a991",
  "trigger_event_id": "6a14372d-4bb7-4634-9c2c-cc53b1f62e70",
  "status": "queued"
}
```

Authorization and controls:

- Management uses `external_trigger.read`, `external_trigger.create`, `external_trigger.update`, and `external_trigger.delete`.
- Invocation requires a valid bearer token, a matching `allowed_callers` entry, `external_trigger.invoke` on the trigger, `pipeline.execute` for the selected pipeline, and runtime `*.use` checks for the selected pipeline, scope, reusable steps, child pipelines, knowledge contexts, secrets, variables, and runners.
- `allowed_callers` supports `user`, `auth_team`, and `service_account`; use service accounts for external systems.
- `run_team_path` selects the Pipeline Runs team for invoked runs so team
  notification routes can deliver external-trigger run events. Use `root` for
  root runs with no team assignment; omitted or empty values normalize to
  `root`.
- Idempotency keys are scoped to trigger, caller type, and caller id. A repeated successful key returns the original run response; an in-flight key returns `409`.
- `payload_schema` supports object schemas with `required` and simple `properties.<name>.type` validation.
- `variable_mapping` maps invoke payload fields into run variables. For example, `"VERSION":"payload.version"` reads `{ "payload": { "version": "1.2.3" } }`.
- Invocation records store caller, status, run id, idempotency key, event type, source IP, timestamp, and error text.

---

## Git Webhook Sources

Git Webhook Sources receive repository events from GitLab, Bitbucket, Gitea, or
a normalized generic sender. Unlike External Triggers, a source is not bound to
one pipeline: the repository event is evaluated against its trigger manifest.

```bash
curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "id":"gitlab-platform",
    "name":"GitLab Platform",
    "provider":"gitlab",
    "enabled":true,
    "auth_mode":"static_token",
    "credential_ref":"credential://system/webhooks/gitlab-platform",
    "repository_allowlist":["platform/api","platform/*"],
    "rate_limit":{"per_minute":120}
  }' \
  http://localhost:8080/v1/git-webhook-sources
```

Management and audit endpoints:

- `GET|POST /v1/git-webhook-sources`
- `GET|PUT|PATCH|DELETE /v1/git-webhook-sources/{sourceID}`
- `GET /v1/git-webhook-sources/{sourceID}/deliveries`
- `POST /v1/git/webhooks/{sourceID}`: public provider delivery endpoint

Sources support `generic`, `gitlab`, `bitbucket`, and `gitea` providers with
`hmac`, `static_token`, or `none` authentication. Authenticated sources resolve
stable credential references from the encrypted registry. `none` is intended
only for trusted, network-isolated ingress.

GitOps manifests live under `git-webhook-sources/*.yaml`. Updating a
GitOps-managed source through the API stores a database override, and deleting
one removes the database row; the next GitOps sync can replace or recreate it
unless the change is pushed to the config repository. Database-created sources
can be exported through global config repository drift/write.

V1 generic sources load trigger overrides and pipeline definitions from the
NopsAI database. Synchronize them through `triggers/` and `pipelines/` before
enabling the source. GitHub's repository-file fallback and check-run behavior
remain GitHub-only.

See [git-webhook-sources.md](./git-webhook-sources.md) for provider headers,
the generic payload contract, path-filter semantics, and operations guidance.

---

## Git Apps

Git Apps manage provider app-level configuration separately from webhook source
or trigger ownership. The GitHub App resource is system-scoped and is guarded by
`system.read` / `system.update` on `system:config`.

```bash
curl -X PUT \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "provider":"github",
    "app_id":"123456",
    "private_key_credential_ref":"credential://system/github/app-private-key",
    "webhook_credential_ref":"credential://system/github/webhook-secret",
    "installations":[{
      "installation_id":"987654",
      "account_login":"nopsai",
      "account_type":"organization",
      "enabled":true
    }]
  }' \
  http://localhost:8080/v1/git-apps/github
```

Endpoints:

- `GET|PUT /v1/git-apps/github`
- `GET|POST /v1/git-apps/github/installations`
- `GET|DELETE /v1/git-apps/github/installations/{installationID}`
- `POST /v1/git-apps/github/installations/{installationID}/verify`
- `POST /v1/git-apps/github/installations/{installationID}/refresh`
- `GET /v1/git-apps/github/installations/{installationID}/repositories`

Installation records are provider/app scoped and do not accept `team_path`,
`visibility`, or `repository_allowlist`. Trigger ownership still lives on
triggers and Git webhook sources. GitHub webhooks must include
`installation.id`; git-bot rejects unknown or disabled installations before
forwarding events to NopsAI.

GitOps uses `setting/git-apps/github.yaml`:

```yaml
provider: github
app_id: "123456"
private_key_credential_ref: credential://system/github/app-private-key
webhook_credential_ref: credential://system/github/webhook-secret
installations:
  - installation_id: "987654"
    account_login: nopsai
    account_type: organization
    enabled: true
```

---

## Access Grants

Use these endpoints to assign product roles to subjects on resources.

```bash
# Grant developer to a user on a team subtree
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "subject_type":"user",
    "subject_id":"alice",
    "role":"developer",
    "resource_type":"team",
    "resource_id":"/payments",
    "inherit":true
  }' \
  http://localhost:8080/v1/access/grants

# List grants for a team
curl "http://localhost:8080/v1/access/grants?resource_type=team&resource_id=/payments"

# Delete a grant
curl -X DELETE http://localhost:8080/v1/access/grants/grant_123
```

Grant request fields:

- `subject_type`: `user`, `auth_team`, `repository`, `trigger`, `service_account`, or `internal_service`
- `subject_id`: user subject/email/UUID, auth team id/name, repository `owner/repo`, trigger id, service-account id, or service id
- `role`: `viewer`, `developer`, `owner`, or `admin`
- `resource_type`: `team` for teams, `pipeline`, `pipeline_schedule`, `trigger`, `external_trigger`, `git_webhook_source`, `secret`, `variable`, `scope`, `repository`, `step`, `knowledge_context`, `runner`, `config_repo`, or `platform`
- `resource_id`: team path such as `/payments`, pipeline id such as `team-1/dev/build`, repository id such as `owner/repo`, or `platform`
- `inherit`: required for team subtree grants; team grants should normally use `true`

Example response:

```json
{
  "id": "grant_123",
  "subject_type": "user",
  "subject_id": "alice",
  "role": "developer",
  "resource_type": "team",
  "resource_id": "/payments",
  "inherit": true,
  "granted_by": "admin"
}
```

Validation and guardrails:

- The target subject and resource must already exist.
- Every team must retain at least one `owner`.
- Only `owner` or `admin` can manage grants.
- `admin` grants are only valid on `platform`.
- `GET /v1/access/auth-teams` lists persisted SSO/AAA auth teams from `auth_teams` for subject selectors.
- `GET /v1/access/teams` lists product team resources for team sharing and resource selectors, excluding application and repository-backed nodes.

---

## Resource Access And Use Checks

Pipeline, reusable step, scope, and knowledge context pages expose an `Access` dialog for resource sharing. Resource sharing controls who may use a resource at runtime; it does not grant permission to manage the resource and it does not let shared pipelines or documents carry their owner's permissions.

Visibility modes:

- `team`: only callers in the same team boundary can use the resource, and only when they already have the required use action
- `restricted`: same-team use still works, and selected teams, repositories, or service accounts can also be granted use access
- `workspace`: shown as `Public` in the UI; authorized callers across the workspace can use the resource, but related scopes, secrets, variables, and runners are still checked separately

Resource access endpoints:

```bash
# Read access settings
curl http://localhost:8080/v1/resources/pipeline/team-1/build/access

# Set visibility
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"visibility":"restricted"}' \
  http://localhost:8080/v1/resources/pipeline/team-1/build/access

# Share with a repository
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"subject_type":"repository","subject_id":"nopsai/test-app","actions":["pipeline.use"]}' \
  http://localhost:8080/v1/resources/pipeline/team-1/build/grants

# Share a knowledge context with a repository
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"subject_type":"repository","subject_id":"nopsai/test-app","actions":["knowledge_context.use"]}' \
  http://localhost:8080/v1/resources/knowledge_context/guardrail/security/repo-check/grants

# Share with an existing team path
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"subject_type":"team","subject_id":"team-1/app","actions":["pipeline.use"]}' \
  http://localhost:8080/v1/resources/pipeline/team-1/build/grants

# Delete a sharing grant
curl -X DELETE http://localhost:8080/v1/resources/pipeline/team-1/build/grants/grant_123
```

The team dropdown in the UI is populated from `GET /v1/access/teams`, using resolved team paths rather than numeric team IDs. Application and repository-backed nodes are intentionally not selectable in these dropdowns. The default scope is addressed as `/v1/resources/scope/default/access`; secret and variable rows store the default scope as `default`.

Resource-use check endpoints:

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "caller_type":"repository",
    "caller_id":"nopsai/test-app",
    "action":"pipeline.use",
    "resource_type":"pipeline",
    "resource_id":"team-1/build",
    "event_type":"push",
    "ref":"refs/heads/main",
    "repo":"nopsai/test-app"
  }' \
  http://localhost:8080/v1/authz/resource-use/check
```

Batch checks use `POST /v1/authz/resource-use/batch-check` with top-level `caller_type`, `caller_id`, and a `checks` array.

---

## Knowledge Context

Knowledge Context documents provide architecture guidance, guardrails, policies,
ADRs, runbooks, references, and examples to LLM-backed pipeline steps.

```bash
# List readable documents
curl http://localhost:8080/v1/knowledge-contexts

# Inspect a document by kind/team/name
curl http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check

# Upsert a UI-managed document
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{
    "kind":"guardrail",
    "team":"security",
    "name":"repo-check",
    "description":"Baseline repository safety rules",
    "content":"# Repository Check Guardrail\n\n- Do not expose secrets in logs.\n"
  }' \
  http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check

# Create and test an external page provider connection
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "team":"security",
    "name":"security-confluence",
    "display_name":"Security Confluence",
    "provider":"confluence",
    "base_url":"https://confluence.example.com/wiki",
    "credential_ref":"credential://team/security/knowledge/confluence"
  }' \
  http://localhost:8080/v1/knowledge-connections

curl -X POST \
  http://localhost:8080/v1/knowledge-connections/security/security-confluence/test

# Search and preview provider pages before linking one
curl "http://localhost:8080/v1/knowledge-connections/security/security-confluence/pages?query=repository%20guardrails"

curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"page_url":"https://confluence.example.com/wiki/spaces/SEC/pages/12345/source-page"}' \
  http://localhost:8080/v1/knowledge-connections/security/security-confluence/resolve-page

# Create an external page document with provider-fetched cached text
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{
    "kind":"policy",
    "team":"security",
    "name":"source-page",
    "description":"Security source page",
    "content_source":"external",
    "connection_id":"security/security-confluence",
    "external_page_url":"https://confluence.example.com/wiki/spaces/SEC/pages/12345/source-page",
    "external_page_id":"12345",
    "sync_mode":"manual",
    "failure_mode":"fail",
    "content":"# Source Page\n\nProvider preview text returned by resolve-page.\n"
  }' \
  http://localhost:8080/v1/knowledge-contexts/policy/security/source-page

# Fetch the latest provider page into the cached Knowledge Context content
curl -X POST \
  http://localhost:8080/v1/knowledge-contexts/policy/security/source-page/sync

# Delete a UI-managed document
curl -X DELETE \
  http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check
```

Pipeline YAML can reference managed documents:

```yaml
knowledge_context:
  - kind: guardrail
    ref: security/repo-check
    required: true
```

Or repo-local markdown at the run commit:

```yaml
knowledge_context:
  - kind: architecture
    path: .nopsai/docs/backend.md
    required: true
```

Managed references are authorized with `knowledge_context.use` before dispatch.
Resolved content is stored with the run in `pipeline_run_knowledge_contexts`.
The run detail response includes `knowledge_contexts` snapshots.

See [knowledge-context.md](./knowledge-context.md) for the full schema and
GitOps layout.

---

## Effective Permissions

Use `GET /v1/access/effective-permissions` when you want to see why a request is allowed or denied.

```bash
curl "http://localhost:8080/v1/access/effective-permissions?action=pipeline.update&resource_type=pipeline&resource_id=payments/deploy-api"
```

Example response:

```json
{
  "allowed": true,
  "action": "pipeline.update",
  "resource": "pipeline:payments/deploy-api",
  "reason": "user alice has developer on team:/payments, inherited by pipeline:payments/deploy-api",
  "matched_role": "developer",
  "matched_subject": "user alice",
  "matched_resource": "team:/payments",
  "inherited": true,
  "source_parent_resource": "team:/payments",
  "low_level_permission": "pipeline.update"
}
```

This endpoint is the product-facing explanation layer on top of the existing AAA `Check` and inheritance logic.

---

## Internal AAA Service

The standalone `aaa` service is internal to the stack and listens on
`AAA_LISTEN_ADDRESS`, defaulting to `:8082`.

```bash
# From another container on the compose network
curl http://aaa:8082/healthz

curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-Internal-Token: $AAA_SHARED_INTERNAL_TOKEN" \
  -d '{
    "subject":{"type":"user","sub":"admin","email":"admin@example.com"},
    "action":"pipeline.read",
    "resource":{"type":"pipeline","id":"team-1/dev/main-pipeline"}
  }' \
  http://aaa:8082/v1/authz/check
```

Internal endpoints:

- `POST /v1/authn/introspect`
- `POST /v1/authz/check`
- `POST /v1/authz/batch-check`
- `POST /v1/authz/filter`
- `POST /v1/audit/record`

Normal clients should use the `nopsai` API rather than calling AAA directly.

---

## System Credentials

System integration credentials are encrypted, versioned, and write-only.
Metadata APIs never return plaintext, ciphertext, or wrapped keys.
GitOps drift/export can write the encrypted envelope records to
`setting/system/credentials.yaml`; feature config files still store only stable
credential references.

Credential references must point at the kind expected by the consuming feature:
LLM profiles and knowledge provider connections use `api_key`, MCP profiles and
config repositories use `bearer_token`, SMTP uses `password`, OIDC client/admin
credentials use `client_secret`, GitHub App private keys use `private_key`,
GitHub App and Git webhook secrets use `webhook_secret`, and runner private
registry access uses `docker_config_json`.

```bash
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/credentials

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reference":"credential://system/llm/openai-primary","kind":"api_key","description":"Primary OpenAI key","value":"<value>"}' \
  http://localhost:8080/v1/system/credentials

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"reference":"credential://system/registry/ghcr","kind":"docker_config_json","description":"GHCR pull config","value":"{\"auths\":{\"ghcr.io\":{\"auth\":\"<base64-user-pass>\"}}}"}' \
  http://localhost:8080/v1/system/credentials

curl -X PUT -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"value":"<rotated-value>"}' \
  http://localhost:8080/v1/system/credentials/<credential-id>/value

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/credentials/<credential-id>/versions/1/activate

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/credentials/<credential-id>/disable

curl -X POST -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/credentials/<credential-id>/enable

curl -X DELETE -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/system/credentials/<credential-id>/versions/1
```

Enabling restores the retained active version. Version deletion returns
`409 Conflict` for the active version or when fewer than two stored versions
exist.

Deletion returns `409 Conflict` while LLM, MCP, mail, OIDC, or GitHub
configuration still references the credential.

---

## Secrets

Secrets are encrypted at rest using the master key. They can be scoped globally, per scope, or per repository.

```bash
# List secrets (add ?scope=scope and ?include_source=true for metadata)
curl "http://localhost:8080/v1/secrets?scope=prod&include_source=true"

# Discover scopes that currently hold secrets
curl http://localhost:8080/v1/secrets/scopes

# Upsert a global secret (optionally scoped)
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value":"General level secret prod scope"}' \
  "http://localhost:8080/v1/secrets/TEST_SECRET?scope=prod"

# Delete a global secret
curl -X DELETE http://localhost:8080/v1/secrets/TEST_SECRET

# Repository-scoped secret (owner/repo path segments)
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value":"repo level secret"}' \
  "http://localhost:8080/v1/repositories/nopsai/test-app/secrets/TEST_SECRET"
```

- Repository endpoints also accept `?scope=` to target scoped values.
- Omitting `?scope=` targets the default scope and stores `scope = 'default'`.
- Repository-scoped entries returned by `GET /v1/secrets` are prefixed with `owner/repo/SECRET`, so the UI can team them under the same scope as global secrets.
- `GET /v1/secrets/scopes` reports only scopes (default, prod, etc.) to mirror the Scopes page.
- Secrets resolve in the following order for the requested scope: repo+scope -> global+scope. Default/unscoped runs resolve repo+default -> global+default.
- Pipeline YAML can reference a different scope with `scope:SECRET_NAME`, for example `dev:TEST_SECRET`; the step receives `TEST_SECRET`.
- Updating a GitOps-managed scoped secret through the UI/API stores a database override and clears GitOps ownership metadata. Deleting one removes the database row; the next GitOps sync can recreate it unless the change is pushed back to GitOps.
- Predefined product roles expose secret metadata broadly, but secret value reads remain owner/admin-level by default.

---

## Scope Variables

Scope variables mirror the scoping rules used for secrets.

```bash
# Global scope variable
curl -X PUT -d '{"value":"general"}' \
  "http://localhost:8080/v1/variables/TEST_SCOPE"

# Repository scope variable
curl -X PUT -d '{"value":"repo"}' \
  "http://localhost:8080/v1/repositories/nopsai/test-app/variables/TEST_SCOPE"

# Fetch scoped variables
curl "http://localhost:8080/v1/variables?scope=prod"
```

- The list endpoint now returns both global variables (e.g. `DATABASE_URL`) and repository-scoped entries in the form `owner/repo/NAME`; team-owned entries may also include the canonical team path, such as `team-1/owner/repo/NAME`.
- Omitting `?scope=` targets the default scope and stores `scope = 'default'`.
- Duplicate keys inside the same scope are rejected during config sync.
- The config repo may define scoped variables under `variables:` and secret keys under `secrets:` in `scopes/<scope>/scope.yaml`; the sync endpoint imports them automatically. Scope variables must be inside the `variables:` section; flat top-level variable entries are rejected. Team config repositories accept repository keys as local `owner/repo/NAME` shorthand or canonical `team/path/owner/repo/NAME`. GitOps secret values must be encrypted by this NopsAI instance, otherwise the key is imported with no value.
- To generate a GitOps secret value without storing it, call `POST /v1/secrets/encrypt` with `{"value":"plain"}` and commit the returned `encrypted_value`.
- Pipeline `variables` entries can use `scope:NAME` to resolve from that explicit scope while injecting `NAME` at runtime. Bare `NAME` resolves from the run's current scope.
- Updating a GitOps-managed scoped variable through the UI/API stores a database override and clears GitOps ownership metadata. Deleting one removes the database row; the next GitOps sync can recreate it unless the change is pushed back to GitOps.
- Predefined product roles allow variable metadata reads and writes, but do not grant variable value reads by default.

---

## Pipelines

```bash
# List pipelines (global + scoped names)
curl http://localhost:8080/v1/pipelines

# Inspect a specific pipeline
curl http://localhost:8080/v1/pipelines/main-pipeline
curl http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline

# Upsert a pipeline definition from disk
curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/main-pipeline

# Delete a pipeline
curl -X DELETE http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline
```

- Paths containing slashes map to nested teams (e.g. `team-1/dev`).
- Pipeline responses include metadata such as version, description, steps, tasks, timeout, container image, and LLM controls.
- Pipeline YAML may declare `knowledge_context` at pipeline, step, or task level.
- Team-level product grants inherit to pipelines below that team path.

---

## Pipeline Schedules

Schedules are first-class resources that run stored pipelines on one-time
timestamps or recurring cron expressions without requiring a Git repository
event. They are teamed by `path`, protected by `pipeline_schedule.*` actions,
and execute through a
schedule-owned service account that receives only the pipeline, scope, reusable
step, and child-pipeline permissions needed for that schedule.

```bash
# List schedules visible to the caller
curl http://localhost:8080/v1/schedules

# List schedules for one pipeline
curl "http://localhost:8080/v1/schedules?pipeline=team-1/services/api/deploy"

# Create a disabled nightly deployment schedule
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "name":"nightly-api-deploy",
    "pipeline":"team-1/services/api/deploy",
    "schedule_kind":"cron",
    "cron_expression":"0 2 * * *",
    "timezone":"UTC",
    "enabled":false,
    "scope":"team-1/prod",
    "run_team_path":"team-1/prod",
    "variables":{"RELEASE_CHANNEL":"nightly"}
  }' \
  http://localhost:8080/v1/schedules

# Create a one-time release schedule at a specific date, hour, and minute
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "name":"release-window",
    "pipeline":"team-1/services/api/deploy",
    "schedule_kind":"once",
    "run_at":"2030-03-15T09:45",
    "timezone":"Europe/Amsterdam",
    "scope":"team-1/prod",
    "run_team_path":"team-1/prod"
  }' \
  http://localhost:8080/v1/schedules

# Enable, disable, run now, inspect, update, or delete
curl -X POST http://localhost:8080/v1/schedules/<schedule-id>/enable
curl -X POST http://localhost:8080/v1/schedules/<schedule-id>/disable
curl -X POST http://localhost:8080/v1/schedules/<schedule-id>/run
curl http://localhost:8080/v1/schedules/<schedule-id>
curl -X PUT -H "Content-Type: application/json" -d @schedule.json http://localhost:8080/v1/schedules/<schedule-id>
curl -X DELETE http://localhost:8080/v1/schedules/<schedule-id>
```

Direct enable, disable, update, and delete operations are allowed when AAA
permits. For GitOps-managed schedules, those operations affect the database row
only; the next GitOps sync can replace or recreate the row unless the change is
pushed to the owning config repository. Authorized callers can still execute
schedules with the run-now endpoint.

Schedule payload fields:

- `path`: optional schedule resource path. If omitted, the schedule uses the
  target pipeline path. The UI uses this default; API and GitOps flows can still
  use paths such as `prod/scheduled` when an operational subteam is useful.
- `run_team_path`: optional Pipeline Runs team for runs started by the
  schedule. Use `root` for root runs with no team assignment; omitted or empty
  values normalize to `root`. Use a concrete team when the schedule should
  notify or appear under an operational team that is different from the
  pipeline definition path.
- `name`: schedule name, unique within `path`.
- `pipeline` or `pipeline_path` plus `pipeline_name`: target stored pipeline.
- `schedule_kind`: `cron` for recurring schedules or `once` for a specific
  date/time. If omitted, `run_at` implies `once`; otherwise the schedule is
  treated as `cron`.
- `run_at`: required for `schedule_kind: once`. Use RFC3339
  (`2030-03-15T08:45:00Z`) or local date-time (`2030-03-15T09:45`) interpreted
  in the schedule `timezone`.
- `cron_expression`: standard five-field cron expression for recurring
  schedules. `cron` is accepted as an alias. The UI provides specific date,
  interval, hourly, daily, weekday, multi-day weekly, multi-day monthly,
  yearly, and custom cron controls.
- `timezone`: IANA timezone such as `UTC` or `Europe/Amsterdam`.
- `enabled`: whether the schedule is active.
- `scope`: optional run scope. `default` is stored as the default/unscoped run.
- `variables`: optional run variable overrides.

Responses include `next_run_at`, `last_run_at`, `last_run_id`, `last_status`,
and `latest_run`. Timestamps are stored in UTC and should be displayed in the
schedule `timezone` when presenting the schedule. Pipeline run list/detail
responses include `trigger_source`, `schedule_id`, `schedule_name`, and
`schedule_path`, so the UI can badge scheduled runs and deep-link to the latest
run details.

GitOps schedules live under `schedules/`:

```yaml
name: nightly-api-deploy
description: Nightly API deployment into prod.
pipeline: services/api/deploy
schedule_kind: cron
cron_expression: "0 2 * * *"
timezone: UTC
enabled: false
scope: prod
run_team_path: prod
variables:
  RELEASE_CHANNEL: nightly
```

One-time GitOps schedule:

```yaml
name: release-window
description: One-time production release window.
pipeline: services/api/deploy
schedule_kind: once
run_at: "2030-03-15T09:45"
timezone: Europe/Amsterdam
enabled: true
scope: prod
run_team_path: prod
```

In a team-scoped config repository, schedule file paths and runtime references
are normalized under the bound team. For example,
`schedules/prod/scheduled/nightly-api-deploy.yaml` in the `team-1` repo becomes
schedule `team-1/prod/scheduled/nightly-api-deploy`, and `pipeline:
services/api/deploy`, `scope: prod`, and `run_team_path: prod` become
`team-1/services/api/deploy`, `team-1/prod`, and `team-1/prod`.
Use `run_team_path: root` when the resulting scheduled run should stay at the
Pipeline Runs root instead of being assigned to a team. A leading `root/`
prefix on runtime references means the root hierarchy, not a team named
`root`.

---

## Team And Run Structure

- The GitOps config repository can define the team and app hierarchy administered from Teams via scoped files such as `config-repositories/teams/team-1/structure.yaml`.
- Each top-level key is a team. Nest teams by adding child keys, assign apps under a team with an `apps:` list, and optionally delegate a team with a `config:` block.
- Schedule and external-trigger `run_team_path` values should reference teams
  from this team hierarchy, or `root` for unassigned root runs; their UI
  selectors are populated from Teams.
- App entries require `name` and `repo_url`; NopsAI normalizes that URL to the repository identity used by triggers and run metadata.
- Team repo bindings under `config-repositories/teams/...` always create matching team shells.
- Structure files colocated under `config-repositories/teams` are merged into those team shells, so repository placement can live next to the team binding.
- In a team-scoped repo, `structure.yaml` may define teams inside the bound team, except for nested teams that have their own config repo binding.
- Example:

```yaml
platform:
  description: Shared platform workflows
team-1:
  description: Team 1 workspace
  config:
    repo_url: git@github.com:nopsai/nopsai-team-1-config.git
    branch: main
    base_path: ""
    enabled: true
  apps:
    - name: general-app
      repo_url: https://github.com/nopsai/general-app
  dev:
    description: This is new
    apps:
      - name: test-app
        repo_url: https://github.com/nopsai/test-app
      - name: t-app
        repo_url: https://github.com/nopsai/t-app
team-2:
  bank:
    description: Handles bank-facing apps
    apps:
      - name: all-app
        repo_url: https://github.com/nopsai/all-app
```

- Running config sync ingests this file, creating or updating teams in the compatibility `teams` table and assigning apps to their Git-defined parents by normalized repository URL. Existing manual teams not referenced in the file are left untouched.

---

## Config Repositories

```bash
# Configure the global/system config repo
curl -X PUT -H "Content-Type: application/json" \
  -d '{"provider":"github","repo_url":"https://github.com/acme/nopsai-global-config","branch":"main","base_path":"nopsai","enabled":true,"write_enabled":true,"write_branch":"nopsai/ui-changes"}' \
  http://localhost:8080/v1/system/config-repo

# Sync only the global/system config repo
curl -X POST http://localhost:8080/v1/system/config-repo/sync

# Sync all enabled config repos; system repos run first, then team repos
curl -X POST http://localhost:8080/v1/system/config-repos/sync

# Compare Nopsai's current config with the sync branch before pushing
curl http://localhost:8080/v1/system/config-repo/drift

# Push generated config files to the configured review branch
curl -X POST -H "Content-Type: application/json" \
  -d '{"message":"Add API deploy pipeline","files":[{"path":"pipelines/services/api/deploy.yaml","content":"name: deploy\nsteps:\n  - name: deploy\n    script: echo deploy\n"}]}' \
  http://localhost:8080/v1/system/config-repo/write
```

- The global repo uses `scope_type=system` and `scope_id=global`.
- Config repositories support `provider` values `github`, `gitlab`, `bitbucket`, and `gitea`. GitHub without `credential_ref` uses the existing GitHub App/git-bot integration; GitHub with `credential_ref`, GitLab, Bitbucket Cloud-compatible repositories, and Gitea use a bearer-token credential reference for repository contents and commit APIs.
- System- and team-scoped repos may define team repo bindings under `config-repositories/teams/<team>.yaml`.
- System- and team-scoped repos may define pipeline schedules under `schedules/`.
- System- and team-scoped repos may define managed knowledge context markdown under `knowledge/`.
- System- and team-scoped repos may define team pipeline notification policies with named routes under `config-repositories/teams/<team>/notifications.yaml`; team repos keep the bound team path explicit.
- The system/global repo may define Agent Profiles and `default_profile` under `setting/system/agent-profiles.yaml`. Team-scoped Agent, LLM, and MCP profiles are managed through `/v1/teams/{teamID}/...` APIs, can be imported/exported by team config repositories in root `ai-profiles.yaml`, and are merged into run launch for runs owned by that team.
- The system/global repo may define mandatory local login and one enabled external identity provider under `setting/system/auth.yaml`; providers bind credential references whose encrypted values can be stored in `setting/system/credentials.yaml`.
- The system/global repo may define GitHub App IDs, credential references, and installations under `setting/git-apps/github.yaml`. The legacy `setting/system/github.yaml` file is read for one release to migrate `github_installation_id` into the first installation record, but exports stop writing it.
- The system/global repo may define runtime runner defaults, dispatcher routing, and `runner_registry_credentials` under `setting/system/runner.yaml`; dispatcher routing changes are synced into `nopsai` and applied by the live dispatcher.
- The system/global repo may define SMTP mail notification settings under `setting/system/mail.yaml`; only `smtp.password_credential_ref` is synced for credentials.
- The system/global repo may define scheduled cleanup rules under `setting/system/data-management.yaml`; backups and cleanup job history remain runtime-only.
- The system/global repo may define encrypted system credential envelopes under `setting/system/credentials.yaml`; plaintext is never exported.
- A binding file contains `repo_url`, optional `provider`, optional `credential_ref`, optional `branch`, optional `base_path`, optional `enabled`, optional `write_enabled`, and optional `write_branch`. `credential_ref` is required for non-GitHub providers and must point at a `bearer_token` credential.
- `branch` remains the read/sync source. When `write_enabled` is true, Nopsai can push generated GitOps changes to `write_branch` so they can be reviewed in the configured Git provider before merging back to the sync branch. For the GitHub App path, the app needs `contents: read and write`; token-backed providers need equivalent repository read/write scope.
- Drift compares the sync branch with Nopsai's current declarative state for pipelines, reusable steps, schedules, triggers, scopes, knowledge contexts, run team/config-repository structure, notification routes, access manifests, Agent Profiles, LLM profiles, MCP registry files, auth settings, mail settings, data cleanup schedules, runtime settings, and encrypted credential envelopes. UI-side resource Access changes for pipelines, reusable steps, scopes, and knowledge contexts are exported as embedded `access:` updates in the affected GitOps files. Pipeline run rows remain runtime/audit records rather than Git-owned resources.
- After generated files are merged into the sync branch, config sync can adopt matching database-owned resources inside the repository scope and mark them as GitOps-managed. Resources already owned by an unrelated config repo remain protected by config-repo precedence.
- Team repositories use the same drift and write endpoint shape at `GET /v1/teams/<team-path>/config-repository/drift` and `POST /v1/teams/<team-path>/config-repository/write`. File paths are relative to the configured `base_path`.
- Nested teams are represented by nested paths, for example `config-repositories/teams/team-2/platform.yaml` creates a binding for `team-2/platform`.
- Team bindings create matching team/application shells for Teams and Pipeline
  Runs compatibility. Pipelines, Steps, Triggers, Scopes, and Knowledge Context
  pages build trees from actual resources by default rather than showing empty
  team shells.
- Schedule paths can use those same team shells; `run_team_path` controls the
  Pipeline Runs team and notification route used by schedule and external
  trigger executions when it should differ from the resource path or target
  pipeline path.
- Once a team repo is assigned and synced, it is authoritative for resources under that team path. Parent or global repos skip and prune their own managed resources inside delegated teams.

## Internal Runtime Config

Internal services authenticate with service bearer tokens and can read
versioned runtime snapshots:

- `GET /internal/v1/runtime-config/{service}`
- `GET /internal/v1/runtime-config/{service}/watch?version=<n>`

Supported service names are `nopsai`, `git-bot`, `dispatcher`, `runner`, and
`agent`. The watch endpoint long-polls until the `runtime_settings.version`
changes or the request times out, then returns the current snapshot.

Example `git-bot` response:

```json
{
  "version": 42,
  "service": "git-bot",
  "reload_mode": "runtime_reload",
  "config": {
    "github_app_id": "123456",
    "github_private_key_ref": "credential://system/github/app-private-key",
    "github_webhook_secret_ref": "credential://system/github/webhook-secret",
    "nopsai_api_url": "http://nopsai:8080"
  }
}
```

The endpoint returns credential references and non-secret runtime values only.
Plaintext secret values remain behind the credential registry and existing
sealed bootstrap/credential resolution flows, even when encrypted envelopes are
GitOps-managed in `setting/system/credentials.yaml`. GitHub App installation
records are served to git-bot through `GET /v1/internal/git-bot/installations`
using the git-bot internal service identity.
- Only owners of the target team, including inherited parent owners, can sync that team repo.
- Complete examples live under `examples/sample-config-repo`.

---

## Reusable Steps

```bash
curl http://localhost:8080/v1/steps                     # list
curl http://localhost:8080/v1/steps?include_source=true # list with metadata
curl http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/shared/utilities/archive-step.yaml" \
  http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X DELETE http://localhost:8080/v1/steps/shared/utilities/archive-step
```

- Reusable steps can be referenced from pipelines through the `include:` directive.
- Reusable step default variables are merged with variables on the calling
  include step; include-step variables win key-by-key.
- When `include_source=true` each item includes `identifier`, `path`, `name`, `source`, and `updated_at`, allowing the UI to distinguish Git-managed definitions from database overrides.
- Using a reusable step from a pipeline requires `step.use`. Managing step definitions is effectively admin-only in the predefined role set.

---

## Trigger Overrides

```bash
curl http://localhost:8080/v1/overrides                               # list
curl http://localhost:8080/v1/overrides/nopsai/test-app      # inspect

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/nopsai/test-app

curl -X DELETE http://localhost:8080/v1/overrides/nopsai/test-app
```

- Overrides let you replace or augment the config-repo trigger manifest for a given repository.
- The payload mirrors the `.nopsai/triggers.yaml` schema (event, branches, skip branches, tags, skipped repositories, include paths, exclude paths, pipelines, and scope).
- When `include_source=true`, each item includes source/provider/team/ingress
  metadata plus a `scopes` array derived from the stored trigger manifest. Empty
  trigger scopes are reported as `default` so list clients can render scope
  coverage without loading every manifest.

---

## Run Lifecycle

```bash
# Start a run (pipeline name optional in path)
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"pipeline":"main-pipeline","variables":{"RELEASE_CHANNEL":"nightly"},"sensitive_variables":[]}' \
  http://localhost:8080/v1/run

curl -X POST http://localhost:8080/v1/run/team-1/dev/main-pipeline

# Fetch runs and details
curl http://localhost:8080/v1/runs
curl "http://localhost:8080/v1/runs?teamId=root"
curl http://localhost:8080/v1/runs/<run-id>
curl http://localhost:8080/v1/runs/<run-id>/status
curl http://localhost:8080/v1/runs/<run-id>/logs
curl -OJ http://localhost:8080/v1/runs/<run-id>/outputs/<output-id>/download
curl http://localhost:8080/v1/runs-by-check/<check-run-id>

# List and decide approvals
curl http://localhost:8080/v1/runs/<run-id>/approvals
curl -X POST -H 'Content-Type: application/json' \
  -d '{"comment":"Change window approved"}' \
  http://localhost:8080/v1/runs/<run-id>/approvals/<approval-id>/approve
curl -X POST -H 'Content-Type: application/json' \
  -d '{"comment":"Deployment window closed"}' \
  http://localhost:8080/v1/runs/<run-id>/approvals/<approval-id>/reject

# Rerun, cancel, or finalise
curl -X POST http://localhost:8080/v1/runs/<run-id>/rerun
curl -X POST http://localhost:8080/v1/runs/<run-id>/cancel
curl -X POST http://localhost:8080/v1/runs/<run-id>/outputs/<output-id>/cancel
curl -X POST -H 'Content-Type: application/json' \
  -d '{"status":"success"}' \
  http://localhost:8080/v1/runs/<run-id>/finalize

# Cleanup
curl -X DELETE http://localhost:8080/v1/runs/<run-id>
curl -X DELETE \
  http://localhost:8080/v1/repositories/<owner>/<repo>/branches/<branch>
```

- JSON run requests accept `pipeline`, `definition`, `scope`, `variables`, and
  `sensitive_variables`. `sensitive_variables` is a list of variable override
  names whose values should be used for launch but omitted from visible run
  metadata; NopsAI stores those values encrypted only for pending-run recovery.
- Step/task status updates are posted by the agent to `/v1/runs/{runID}/steps/{step}/tasks/{task}` (payload includes status, exit code, and LLM timing).
- Approval steps move runs to `waiting_approval`; approval resumes the stored checkpoint, while rejection marks the run `rejected`.
- Internal approval checkpoint, AI usage, and policy-revision endpoints under
  `/v1/internal/runs/...` are service-token protected for agents and are not
  user API endpoints.
- Run listings return summary metadata used by the UI cards and periodic
  refreshes. When a run has configured or stored final outputs, list items also
  include a lightweight `final_output_status` with aggregate statuses such as
  `waiting`, `not_generated`, `pending`, `generating`, `success`, `failure`,
  `partial_failure`, `cancelled`, and `partial_cancelled`, plus configured and
  stored output counts. It never includes generated output content. Run details
  additionally expose run-created/timeout timestamps, scope, team ID, trigger
  source/event, requested/effective subject, schedule/external trigger
  metadata, Git repository/commit/pusher metadata, and non-sensitive runtime
  variable overrides. Sensitive variable overrides are not exposed in run
  detail metadata; child-pipeline include triggers store them only as encrypted
  recovery launch inputs. Authorization snapshots remain server-side audit data.
- Pipeline YAML can define run-level final deliverables under `output.items`.
  When a run finalizes, NopsAI creates run-owned output records and generates
  matching items asynchronously from the full run context. Each item can set
  `when: success`, `when: failure`, or `when: always` to keep success reports
  and failure reports separate. Run detail responses include
  `final_outputs` with output ID, name, type, status, content, error,
  LLM profile, dashboard target metadata for dashboard outputs,
  `generation_attempts`, `contract_violations`, `render_attempts`,
  `render_failures`, created/started/updated timestamps,
  `generation_duration`, and `generation_duration_seconds`.
  `generation_duration` is measured from `generation_started_at`, not from row
  creation, so queued final outputs do not include time spent waiting behind
  prior outputs. PDF/HTML content is versioned `DocumentSpec` JSON; Excel
  content is versioned `SpreadsheetSpec` JSON.
- Final-output LLM calls use a provider-level system contract requiring one
  `<final_output>` element. NopsAI stores only the element content, retries one
  time for missing or invalid contract output, forces temperature `0`, and
  records AI usage for rejected and accepted attempts.
- `GET /v1/runs/{runID}/outputs/{outputID}/download` uses the same
  approval-aware run-read authorization as run details and returns rendered
  Markdown, JSON, server-templated HTML, Gotenberg/Chromium PDF, or typed
  Excelize XLSX content for successful final outputs. Existing pre-schema rich
  outputs use download-only compatibility adapters.
- `POST /v1/runs/{runID}/outputs/{outputID}/cancel` marks a `pending` or
  `generating` final output as `cancelled` without cancelling the pipeline run.
  Late background generation results do not overwrite the cancelled output or
  publish dashboard content. The route requires `pipeline_run.cancel`.
- `GET /v1/runs?teamId=<id>` returns runs for a Pipeline Runs team and its
  descendants, teamed by branch for the Main view. `teamId=root` returns runs
  with no team assignment.
- Repository-triggered runs resolve an existing team/application owner when
  one matches the repository, but runtime ingestion does not create or rewrite
  team/application records.
- Scheduled runs set `trigger_source: "schedule"` and include schedule metadata when the run came from a pipeline schedule.
- Run log access is authorized separately from run-detail access in the low-level AAA layer.
- Branch cleanup removes historical runs for the specified branch while leaving the repository intact.

---

## Teams

Team and application administration lives under **Teams**. Pipeline Runs uses
the same team records for runtime filtering and drilldown.

```bash
curl -X POST -H 'Content-Type: application/json' \
  -d '{"name":"team-1", "parent_id":null}' \
  http://localhost:8080/v1/teams

curl http://localhost:8080/v1/teams

curl -X PUT -H 'Content-Type: application/json' \
  -d '{"name":"team-1/platform"}' \
  http://localhost:8080/v1/teams/<team-id>

curl -X PUT -H 'Content-Type: application/json' \
  -d '{"parent_id":42}' \
  http://localhost:8080/v1/teams/<team-id>

curl -X PUT -H 'Content-Type: application/json' \
  -d '{"name":"api","repo_url":"https://github.com/acme/api"}' \
  http://localhost:8080/v1/teams/<target-parent-team-id>/applications/<application-id>

curl -X DELETE http://localhost:8080/v1/teams/<team-id>
```

- Teams power the “Main” dashboard hierarchy. Each run card can be assigned to a team path.
- Access grants should target the resolved team path, not numeric IDs.
- Application moves use the target parent in the URL; team moves use `parent_id` or `parent_team_id` in the body.
- Browser deep links for team-scoped pages use readable route segments, for example `#/pipelineruns/main/team/team-2/bank/account` or `#/pipelines/team/team-2/bank/account`, rather than numeric database IDs or `team=` query parameters.
- `GET /v1/teams` is filtered by the caller’s team visibility.

---

## Git & Repository Utilities

```bash
# Triggered by git-bot; requires a git-bot internal service token.
curl -X POST http://localhost:8080/v1/git/events \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $GIT_BOT_SERVICE_TOKEN" \
  --data-binary "@doc/sample-git-event.json"

# List branches known to the API (for branch clean-up helpers)
curl http://localhost:8080/v1/repositories/nopsai/test-app/branches
```

- The git-bot exposes additional HTTP helpers (from `services/git-bot`) for file content, directory listings, repository access checks, and child check-run creation.

---

## UI Refresh And Log Polling

```bash
# Fetch new log lines after the last line id you have seen
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/runs/<run-id>/logs?since_line=<last-line-id>"
```

- The current UI refreshes run lists and details with REST polling.
- The log modal polls the run logs endpoint with `since_line` to append new lines incrementally; visible tabs poll on a live cadence and hidden tabs back off.
- Run log entries always include `id`, `timestamp`, and `line`. Entries may also include structured `source`, `stream`, `level`, `step_name`, `task_name`, `runner_id`, `request_id`, `traceparent`, and `metadata` fields. Hosted MCP `nopsai.get_pipeline_run_logs` returns the same metadata when present.
- The ingest path derives missing `level`, `step_name`, and `task_name` from structured log fields such as `output_level`, `level`, `step`, and `task`. Levels are normalized to `info`, `warn`, `error`, or `debug` for UI filtering. The `stream` field remains independent from severity: plain stderr is not an error unless structured metadata or the line text says so.

---

## Local Reset (Optional)

For local testing, you can clear Docker state with:

```bash
docker-compose down -v
docker container prune -f -a
docker volume prune -f -a
docker image prune -f -a
```

> Warning: these commands remove **all** containers, volumes, and images on your machine. Use them only on disposable local systems.
