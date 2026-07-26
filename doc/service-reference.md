# NopsAI Service Reference

This document explains what each service does, which interfaces it owns, and where its main implementation lives.

## Service Topology Naming

Use `*_LISTEN_ADDRESS` for the socket a service binds inside its own container
or pod, and use `*_URL` or `*_GRPC_ADDRESS` for the endpoint other services
dial. For example, the API can listen on `0.0.0.0:8080` while agents and
git-bot use `NOPSAI_API_URL=http://nopsai:8080` in Compose or a cluster DNS name
such as `http://nopsai.nopsai:8080` in Kubernetes. The dispatcher similarly
listens on `DISPATCHER_LISTEN_ADDRESS` and is reached through
`DISPATCHER_GRPC_ADDRESS`.

The service-topology config contract uses `NOPSAI_API_URL`,
`DISPATCHER_GRPC_ADDRESS`, `GIT_BOT_API_URL`, and `AAA_LISTEN_ADDRESS`.

## `services/nopsai`

Primary role:

- Main API and control-plane brain.

Operational logging defaults to `info` when `log_level`/`LOG_LEVEL` is omitted
or blank. Use the explicit `disabled` level when a deployment intentionally
suppresses service container logs. Routine events (`trace` through `info`) are
written to stdout; warnings and errors are written to stderr.

Shared service logging adds stable `service` and deployment `environment`
fields when `NOPSAI_SERVICE_NAME`/`SERVICE_NAME` and `NOPSAI_ENVIRONMENT` are
available. HTTP and gRPC service boundaries log method, path/RPC method, status
or gRPC code, byte count where applicable, `duration_ms`, `request_id`, and
`traceparent` when provided. `X-Request-ID` is accepted or generated at ingress
and propagated over internal HTTP and gRPC metadata.

Responsibilities:

- Enforces optional enterprise startup gates for production deployments,
  surfacing unmet hardening requirements through the public setup preflight
  response before the full API starts.
- Shares production startup-gate validation logic with other service binaries
  through `pkg/startupgates`.
- Uses shared HTTP server timeout defaults from `pkg/httpapi` for production
  request hardening.
- Exposes REST endpoints for auth, runs, pipelines, steps, triggers, Git webhook sources, knowledge contexts, notifications, metrics, secrets, variables, teams, and system operations.
- Exposes product access-management endpoints for role grants and effective-permission inspection.
- Stores and reads all durable state from Postgres.
- Validates pipelines and resolves reusable `step:` includes.
- Resolves required knowledge context, secrets, and variables before a run is dispatched.
- Creates run records, task records, and log records through run-service and
  run-query boundaries that keep persistence, launch orchestration, and run
  detail shaping out of HTTP handlers.
- Builds runner install artifacts through a dedicated runner-install boundary,
  while one-time bootstrap token storage remains on the application.
- Uses a consumer-owned `GitProvider` boundary for GitHub repository reads,
  config commits, branch PR status, check-run lifecycle operations, and
  run/task status updates; the git-bot HTTP client is the first concrete
  implementation wired during service bootstrap.
- Uses a consumer-owned `DispatcherClient` boundary for job submission,
  dispatcher status lookup, and runner dispatch controls; the generated gRPC
  client is adapted during service bootstrap.
- Uses a consumer-owned `RunLauncher` boundary for prepared agent-run launch
  handoff, keeping run orchestration and approval resume flows independent of
  concrete dispatcher/environment launch plumbing.
- Uses a consumer-owned `ConfigSyncStore` boundary for config repository
  listing, sync-status persistence, and config-sync apply/prune handoff.
- Uses a consumer-owned `SecretCodec` boundary for secret encryption and
  decryption; AES-256-GCM remains the default implementation wired during
  service bootstrap.
- Owns the encrypted system credential registry through
  `internal/credentials`, `pkg/store/credentials.go`,
  `credential_service.go`, and `credential_handlers.go`. GitOps stores stable
  references; runtime consumers use the narrow resolver and never query
  credential tables directly.
- Uses a consumer-owned `AAAClient` boundary for subject introspection,
  authorization checks, batch checks, resource filtering, and audit decision
  recording; the AAA HTTP client and in-process evaluator fallback are wired
  during service bootstrap.
- Builds auth, AAA, audit, internal HTTP, and default Git provider dependencies
  through a focused security-runtime constructor instead of keeping that wiring
  in the app assembly path.
- Shares config-sync path normalization, repository identifier parsing,
  pipeline-run team-structure parsing, binding-file validation/defaults,
  config-repository request shaping, write-path validation, and
  config-repository drift ownership/path, file-diffing, and team-structure
  export rules through a dedicated internal config-sync package.
- Starts Git-backed config repository sync through a dedicated
  `config_sync_runner.go` runner/status boundary, a shared
  `config_repository_git_paths.go` directory-layout boundary, a
  `config_sync_fetch.go` repository discovery boundary, a
  `config_sync_parse.go` parse-plan boundary, a `config_sync.go` coordinator
  boundary, and a `config_sync_apply.go` transactional apply/prune boundary,
  with scope-entry parsing and delegated team synchronization split into
  separate files.
- Exports config repository desired state through separate drift, resource,
  scope, knowledge, team-structure, access, embedded resource-access,
  path-rule, and runtime/settings export boundaries.
- Keeps high-churn NopsAI product domains in focused file families for access
  grants, config access, setup wizard, data management, MCP, auth/admin, and
  schedules, preserving package-local behavior while reducing mixed handler,
  persistence, validation, and worker responsibilities.
- Keeps MCP registry validation, read-only tool policy, tool selection, and
  GitOps parsing in `services/nopsai/internal/mcpregistry`; root MCP files keep
  the `*App` HTTP, config, and persistence wiring.
- Applies system GitOps runtime settings through shared system-config helpers
  and `runtime_settings_store.go`, persisting runner install defaults, runner
  runtime defaults, agent defaults, runner registry credential assignments, and
  dispatcher routing in the database as the source of truth. GitHub App IDs,
  credential references, and installation records are owned by
  `setting/git-apps/github.yaml`; git-bot URLs remain system/service settings.
  `config.yml`, `.env`, Docker Compose, and deployment secrets are bootstrap
  inputs. Mail notification settings stay in their dedicated notification
  settings store.
- Exposes versioned internal runtime snapshots at
  `/internal/v1/runtime-config/{service}` and a long-poll watch endpoint for
  services that can reload clients or reconnect without a container restart.
- Records product access grants and delegates predefined role, binding,
  permission, and ACL policy mutations to AAA-owned store helpers.
- Talks to the dispatcher as a gRPC client.
- Talks to `git-bot` over HTTP for GitHub checks and repository content access.
- Owns provider-neutral Git webhook source records, provider adapters,
  authentication, repository allowlists, delivery audit/idempotency, trigger
  matching, and run orchestration in focused `git_webhook_*` files and
  `internal/gitwebhook`.

Key files:

- `services/nopsai/cmd/nopsai/main.go`
- `services/nopsai/internal/app/app.go`
- `services/nopsai/app.go`
- `services/nopsai/bootstrap.go`
- `services/nopsai/app_security.go`
- `services/nopsai/bootstrap_schema.go`
- `services/nopsai/enterprise_gates.go`
- `pkg/startupgates`
- `pkg/httpapi/server.go`
- `services/nopsai/dispatcher_client.go`
- `services/nopsai/gitbot_client.go`
- `services/nopsai/aaa_helpers.go`
- `services/nopsai/config_sync_store.go`
- `services/nopsai/secret_codec.go`
- `services/nopsai/auth_models.go`
- `services/nopsai/http_middleware.go`
- `services/nopsai/config_runtime.go`
- `services/nopsai/config_sync_status.go`
- `services/nopsai/pipeline_runtime.go`
- `services/nopsai/run_helpers.go`
- `services/nopsai/repository_branch_handlers.go`
- `services/nopsai/routes.go`
- `services/nopsai/run_handlers.go`
- `services/nopsai/git_webhook_sources_handlers.go`
- `services/nopsai/git_webhook_orchestrator.go`
- `services/nopsai/internal/gitwebhook`
- `pkg/gittrigger`
- `services/nopsai/run_team_resolution.go`
- `services/nopsai/run_failure_records.go`
- `services/nopsai/run_lifecycle_handlers.go`
- `services/nopsai/run_internal_handlers.go`
- `services/nopsai/team_store_handlers.go`
- `services/nopsai/config_sync_runner.go`
- `services/nopsai/config_repository_git_paths.go`
- `services/nopsai/config_sync_fetch.go`
- `services/nopsai/config_sync_parse.go`
- `services/nopsai/config_sync.go`
- `services/nopsai/config_sync_apply.go`
- `services/nopsai/config_sync_scope_entries.go`
- `services/nopsai/config_sync_teams.go`
- `services/nopsai/config_repository_drift.go`
- `services/nopsai/config_repository_paths.go`
- `services/nopsai/config_repository_resource_export.go`
- `services/nopsai/config_repository_scope_export.go`
- `services/nopsai/config_repository_knowledge_export.go`
- `services/nopsai/config_repository_team_export.go`
- `services/nopsai/config_repository_access_export.go`
- `services/nopsai/config_repository_resource_access.go`
- `services/nopsai/config_repository_settings_export.go`
- `services/nopsai/run_service.go`
- `services/nopsai/run_launcher.go`
- `services/nopsai/internal/configsync`
- `services/nopsai/internal/gitbot`
- `services/nopsai/internal/runs`
- `services/nopsai/internal/runnerinstall`
- `services/nopsai/internal/systemconfig`
- `services/nopsai/internal/mcpregistry`
- `services/nopsai/system_handlers.go`
- `services/nopsai/gitbot_client.go`
- `services/nopsai/runner_bootstrap_tokens.go`
- `services/nopsai/pipeline_handlers.go`
- `services/nopsai/knowledge_context.go`
- `services/nopsai/knowledge_context_schema.go`
- `services/nopsai/pipeline_final_output_contract.go`
- `services/nopsai/pipeline_final_output_specs.go`
- `services/nopsai/pipeline_final_outputs.go`
- `services/nopsai/pipeline_final_outputs_render.go`
- `services/nopsai/pipeline_final_outputs_document.go`
- `services/nopsai/pipeline_final_outputs_pdf.go`
- `services/nopsai/pipeline_final_outputs_spreadsheet.go`
- `services/nopsai/pipeline_final_outputs_schema.go`
- `services/nopsai/metrics.go`
- `services/nopsai/monitoring_handlers.go`
- `services/nopsai/monitoring_analytics_handlers.go`
- `services/nopsai/monitoring_operations_handlers.go`
- `services/nopsai/monitoring_analytics_schema.go`
- `services/nopsai/notification_mail.go`
- `services/nopsai/notification_routes.go`
- `services/nopsai/notification_schema.go`
- `services/nopsai/secrets_variables_handlers.go`
- `services/nopsai/github_integration.go`
- `services/nopsai/auth_handlers.go`
- `services/nopsai/auth_subjects.go`
- `services/nopsai/auth_profile_handlers.go`
- `services/nopsai/auth_bootstrap.go`
- `services/nopsai/admin_user_handlers.go`
- `services/nopsai/admin_role_handlers.go`
- `services/nopsai/access_grants*.go`
- `services/nopsai/config_access*.go`
- `services/nopsai/setup_wizard*.go`
- `services/nopsai/data_management*.go`
- `services/nopsai/mcp*.go`
- `services/nopsai/schedules*.go`
- `services/nopsai/system_logs_handlers.go`
- `services/nopsai/internal/systemlogs`: Allow-listed source registry, signed cursors, redaction, bounded replay/fan-out broker, metrics, Docker provider, and Kubernetes provider.

The entrypoint above builds as `nopsai-api`. The user-facing `nopsai` binary is
owned independently by `cmd/nopsai-cli` and `internal/cli`; it reaches this
service through authenticated REST and does not import service internals.

Important subpackages:

- `pkg/auth`: Local JWT auth, refresh tokens, password hashing, rate limits, lockout rules.
- `pkg/authz`: Casbin-backed legacy RBAC metadata compatibility.
- `pkg/audit`: Audit-log persistence.
- `pkg/store`: Small storage abstraction, currently centered on Postgres.
- `pkg/validation`: Pipeline validation.
- `pkg/aaaclient`: HTTP client for the internal AAA service.
- `pkg/routeauthz`: Route-to-action/resource mapping used by the authorization middleware.

Inbound interfaces:

- Browser/UI HTTP traffic
- Authenticated System Logs SSE traffic through the shared API client
- `git-bot` forwarded GitHub events
- GitLab, Bitbucket, Gitea, and generic Git webhook deliveries
- Internal dispatcher-authenticated HTTP calls for logs, task updates, finalization, pipeline fetches, and child pipeline creation

Authorization notes:

- Product roles are seeded as predefined templates: `viewer`, `developer`, `owner`, and `admin`.
- Access grants are recorded as product intent; low-level role, binding,
  permission, and ACL rows are mutated through AAA-owned store helpers instead
  of product-service SQL.
- Team-targeted grants inherit by path to child resources.
- Runtime resource-use checks are caller-based, so Git runs are authorized as repositories, manual runs as users, and dispatcher calls do not inherit resource-owner permissions.
- Managed knowledge context references are checked with `knowledge_context.use` before dispatch.
- Sensitive allowed actions and all denied actions are written to authorization decision logs.

Outbound interfaces:

- gRPC to `dispatcher`
- HTTP to `git-bot`
- HTTP to `aaa`
- Postgres

## Operator CLI

Primary role:

- One user-facing binary for API and platform operations.

Current responsibilities:

- Stores API contexts separately from opaque credentials with restrictive file
  permissions and atomic replacement.
- Sends user JWT, personal access, or service-account tokens through the normal
  bearer authentication and AAA middleware.
- Provides a generic request escape hatch for all public REST and hosted MCP
  routes.
- Compiles a generated, parity-tested catalog of every registered operator,
  public, internal, streaming, and download route and safely expands route
  templates for scripted invocation.
- Checks API preflight, authentication, metrics, dispatcher monitoring, and
  local platform tools through `platform doctor`.
- Generates versioned Docker Compose and Helm values for first installs, can
  deploy generated Kubernetes values with Helm, and keeps the lower-level
  manifest deployer available for advanced internal release workflows.
- Supports GitOps/CI use through declarative context files and environment
  secret injection.

Key files:

- `cmd/nopsai-cli/main.go`
- `internal/cli/config`
- `internal/cli/client`
- `internal/cli/apicatalog`
- `internal/cli/platform`
- `internal/cli/command`
- `pkg/buildinfo`
- `pkg/compatibility`
- `release/compatibility.yaml`

## `services/aaa`

Primary role:

- Internal policy decision point for authorization.

Responsibilities:

- Serves internal HTTP endpoints for subject introspection, single checks, batch checks, resource filtering, and audit recording.
- Enforces deny-before-allow behavior across direct roles, auth-team roles, direct ACLs, auth-team ACLs, and inherited ACLs.
- Resolves users, teams, roles, resource ACLs, ownership, and inheritance from Postgres.
- Owns the AAA policy schema and table-specific policy mutation helpers used by
  product workflows.
- Writes authorization decision logs for denied decisions and sensitive allowed decisions.
- Ensures the AAA schema and default internal roles exist at startup.
- Fails closed in production gate mode when the database URL or shared internal
  token are not production-ready.

Key files:

- `services/aaa/main.go`
- `services/aaa/pkg/server/server.go`
- `services/aaa/pkg/authz/evaluator.go`
- `services/aaa/pkg/store/postgres.go`
- `services/aaa/pkg/store/schema.go`
- `services/aaa/pkg/model/model.go`

Inbound interfaces:

- HTTP from `nopsai` with `X-Internal-Token`

Outbound interfaces:

- Postgres

Notable behavior:

- `GET /healthz` is public for health checks.
- All `/v1/authn/*`, `/v1/authz/*`, and `/v1/audit/*` endpoints require the shared internal token.
- `nopsai` keeps an in-process evaluator fallback behind its `AAAClient`
  boundary using the same store, so short AAA service outages do not have to
  stop authorization checks.

## `services/dispatcher`

Primary role:

- Scheduling hub and internal bridge between runners/agents and `nopsai`.

Responsibilities:

- Accepts `SubmitJob` requests from `nopsai`.
- Maintains long-lived gRPC streams from runners.
- Tracks connected runners, capacity, scopes, metadata, heartbeat freshness, and inflight jobs.
- Selects a runner based on scope, routing config, runner affinity, preferred runner ID, and current load.
- Polls `nopsai` for the current dispatcher routing snapshot and updates its in-memory routing table without restart.
- Queues jobs when no runner is available.
- Requeues inflight jobs if a runner disconnects.
- Exposes status, runner dispatch, and runner ejection controls for the system UI.
- Proxies agent-originated logs, task updates, final status, child pipeline
  triggers, and pipeline fetches back into `nopsai` through a dedicated NopsAI
  callback client boundary.
- Uses a thin `cmd/dispatcher` command entrypoint, with process bootstrap and
  service wiring in `internal/app` and the gRPC scheduling/control service in
  `internal/service`.

Key files:

- `services/dispatcher/cmd/dispatcher/main.go`
- `services/dispatcher/internal/app`
- `services/dispatcher/internal/service`
- `services/dispatcher/internal/service/nopsai_client.go`
- `services/dispatcher/internal/service/queue.go`
- `services/dispatcher/internal/service/scheduling.go`
- `services/dispatcher/internal/service/routing.go`
- `services/dispatcher/internal/service/metadata.go`
- `pkg/proto/dispatcher.proto`

Inbound interfaces:

- gRPC from `nopsai`
- gRPC streaming from runners
- gRPC calls from agents through the same dispatcher service

Outbound interfaces:

- HTTP back into `nopsai`
- Internal JWT generation for trusted service-to-service calls

Notable behavior:

- Scope-aware scheduling
- least-loaded eligible runner selection
- best-effort affinity by trigger or run
- manual dispatch pause/resume per runner
- production startup gates for service JWT isolation, dispatcher TLS, and the
  NopsAI callback URL

## `services/docker-runner`

Primary role:

- Long-lived execution daemon attached to a Docker runtime.

Responsibilities:

- Connects to the dispatcher and registers its `runner_id`, scopes, capacity, and metadata.
- Receives job assignments over gRPC.
- Ensures the agent image exists locally.
- Resolves assigned private registry auth from NopsAI before Docker API image
  pulls.
- Creates a shared Docker volume for the run.
- Starts the agent container with Docker socket access and the shared workspace volume.
- Streams container logs back through the dispatcher.
- Polls run status so a cancelled run stops the agent container quickly.
- Fails closed in production gate mode when dispatcher address, service JWT, or
  dispatcher TLS settings are not production-ready.
- Uses a thin `cmd/docker-runner` command entrypoint, with process bootstrap and Docker
  client/auth/TLS wiring in `internal/app` and dispatcher stream/job execution
  behavior in `internal/service`.

Key files:

- `services/docker-runner/cmd/docker-runner/main.go`
- `services/docker-runner/internal/app/app.go`
- `services/docker-runner/internal/service/runner.go`
- `container/Dockerfile.docker-runner`

Inbound interfaces:

- gRPC stream from the dispatcher

Outbound interfaces:

- Docker Engine API
- gRPC to the dispatcher
- Env-carried local Docker config for assigned private image pulls

Operational assumptions:

- The runner has access to `/var/run/docker.sock`.
- The runner can reach NopsAI and the required registries. Host `docker login`
  is not enough for runner-owned pulls because the runner calls the Docker API;
  selected `docker_config_json` credentials are passed as
  `NOPSAI_REGISTRY_DOCKER_CONFIG_B64` so runner and agent pulls can supply
  matching per-image `RegistryAuth` locally.

## `services/k8s-runner`

Primary role:

- Long-lived execution daemon installed in a Kubernetes namespace.

Responsibilities:

- Connects to the dispatcher and registers as a `runtime=kubernetes` runner.
- Starts an agent pod with Kubernetes runtime variables and a run workspace volume.
- Uses ServiceAccount `imagePullSecrets` for private runner, agent, and step
  image pulls when infra or the one-time bootstrap command attaches them.
- Streams agent pod logs back through the dispatcher.
- Polls run status so a cancelled run deletes the active agent pod quickly.
- Passes namespace, service account, storage, affinity, and runtime pool settings to the agent.
- Fails closed in production gate mode when dispatcher address, service JWT, or
  dispatcher TLS settings are not production-ready.

Key files:

- `services/k8s-runner/cmd/k8s-runner/main.go`
- `services/k8s-runner/internal/app/app.go`
- `services/k8s-runner/internal/service/runner.go`
- `services/k8s-runner/internal/service/workspace.go`
- `services/k8s-runner/internal/service/pod.go`
- `services/k8s-runner/internal/service/scheduling.go`
- `container/Dockerfile.k8s-runner`
- `doc/kubernetes-runner.md`

Outbound interfaces:

- Kubernetes API for pods, pod logs, and PVCs
- gRPC to the dispatcher

Operational assumptions:

- One runner is deployed per namespace.
- PVC mode is used for full agent/step pod workspace compatibility.
- The runner can be scheduled anywhere; node affinity, when enabled, keeps step pods on the agent pod's node.

## `services/agent`

Primary role:

- Per-run orchestrator that actually executes pipeline logic.

Responsibilities:

- Starts once per run with the resolved pipeline definition and runtime context encoded in OS variables.
- Connects to the dispatcher for status reporting, pipeline fetches, child pipeline triggers, and cancellation polling.
- Validates production dispatcher connectivity/auth/TLS environment before
  starting a run.
- Decodes secrets and variables already prepared by `nopsai`.
- Decodes the run knowledge context snapshot prepared by `nopsai`.
- Evaluates step conditions with the configured LLM provider.
- Resolves goal-based tasks into structured actions.
- Executes commands or file writes inside step containers.
- Maintains execution history that later tasks and child pipelines can use.
- Injects effective pipeline + step + task knowledge context into LLM prompts.
- Adds deterministic `knowledge_revision`, `policy_revision`,
  `effective_policy_snapshot_hash`, `policy_merge_mode`, and
  `policy_precedence_version` metadata to blocking knowledge prompts.
- Pins policy snapshots by scope and recomputes effective policy as pipeline,
  step, and task scopes start. Emergency policy response uses run cancellation
  instead of live mutation of already-resolved policy.
- Logs LLM prompt metadata, hashes, sizes, and token estimates without logging
  prompt bodies.
- Records prompt/cache/session/revision/retrieval telemetry in AI usage
  metadata, including logical session IDs, provider-state IDs/support, cache
  identity hashes, cached input tokens, and cache-write tokens when providers
  return them.
- Exposes bounded internal workspace tools (`list_files`, `search_code`, and
  `read_file`) to LLM goal resolution with cursor and byte-range pagination.
- Adds file identity metadata to explicitly shared workspace files and rejects
  stale `REPLACE_FILE` actions before execution.
- Compacts oversized LLM prompt history to a stable summary plus recent task
  events while preserving full durable history for approvals and child runs.
- Masks secrets before writing output into the shared history/log path.
- Sends task status and final status back through the dispatcher.
- Keeps logger construction, workspace directory listing, and dispatcher
  reporting helpers outside the main execution loop.
- Delegates runtime payload loading, dispatcher client auth/TLS setup,
  execution runtime client setup, pipeline timeout cancellation, signal
  handling, active-task tracking, and step session tracking/cleanup
  orchestration to `internal/app`.
- Keeps LLM provider behavior and MCP tool-call action runtime in
  `internal/llm`, with focused files for shared contracts, profiles, action
  generation, condition prompts, Gemini, LM Studio, OpenAI-compatible,
  Anthropic, Azure OpenAI, and response decoding.
  The app pipeline runtime remains split into request DTOs, the run loop, and
  request helpers.

Key files:

- `services/agent/cmd/agent/main.go`
- `services/agent/app.go`
- `services/agent/runtime_wiring.go`
- `services/agent/agent_logging.go`
- `services/agent/workspace_listing.go`
- `services/agent/dispatcher_reports.go`
- `services/agent/approval_checkpoint.go`
- `services/agent/nopsai_client.go`
- `services/agent/policy_revision.go`
- `services/agent/internal/app`
- `services/agent/internal/scheduler`
- `services/agent/internal/executor`
- `services/agent/internal/dockerexec`
- `services/agent/internal/kubernetesexec`
- `services/agent/internal/resolver`
- `services/agent/internal/approval`
- `services/agent/internal/include`
- `services/agent/internal/llm`
- `services/agent/internal/app/pipeline*.go`
- `pkg/proto/agent.proto`

Inbound interfaces:

- Runtime variables injected by the runner/job payload

Outbound interfaces:

- gRPC to dispatcher
- Docker Engine API
- Gemini, LM Studio, OpenAI-compatible, Anthropic, or Azure OpenAI HTTP APIs

Notable implementation detail:

- `agent.proto` defines an `LLMService`, but the current runtime uses an embedded `LLMClient` directly in the agent instead of a separate LLM microservice.

## `services/git-bot`

Primary role:

- GitHub App edge service and GitHub-specific adapter.

Responsibilities:

- Validates GitHub webhook signatures.
- Forwards webhook payloads to `nopsai` through a narrow webhook-forwarder
  boundary.
- Reads repository files and directories from GitHub on behalf of `nopsai`.
- Checks repository access and whether a branch has an open PR.
- Keeps repository reads, repository access checks, branch PR checks,
  installation repository listing, and pipeline content fetches behind a
  GitHub repository provider boundary.
- Resolves GitHub clients per installation with `GitHubClientResolver`, using
  the repository owner or webhook `installation.id` to choose the correct
  installation token.
- Creates, initializes, finds, and updates GitHub check runs.
- Keeps check-run create/update/list operations behind a dedicated GitHub checks
  provider boundary.
- Tracks step/task state for rich check-run rendering, with summary rendering
  delegated to an internal check-render package.
- Creates child check runs for included pipelines.
- Uses a thin `cmd/git-bot` command entrypoint, with process bootstrap and
  GitHub App/client wiring in `internal/app` and GitHub/webhook/check-run HTTP
  behavior in `internal/service`.

Key files:

- `services/git-bot/cmd/git-bot/main.go`
- `services/git-bot/internal/app`
- `services/git-bot/internal/service`
- `services/git-bot/internal/service/nopsai_forwarder.go`
- `services/git-bot/internal/service/github_resolver.go`
- `services/git-bot/internal/service/github_repository.go`
- `services/git-bot/internal/service/github_checks.go`
- `services/git-bot/internal/checkrender`

Inbound interfaces:

- GitHub webhooks
- HTTP calls from `nopsai`

Outbound interfaces:

- GitHub REST API through the GitHub App installation transport
- HTTP back to `nopsai`

## `services/ui`

Primary role:

- Operator-facing interface for managing the platform and inspecting execution.

Responsibilities:

- Login and session refresh
- Pipeline runs view with main, recent, and events views
- Pipeline editor and pipeline drafts
- Trigger override editor
- Knowledge Context browser, markdown editor/preview, source metadata, access settings, and usage inspection
- Scope manager for secrets and variables
- Reusable step library
- Lab for ad-hoc YAML execution and quick runs
- System pages for config sync, data backups/cleanup, dispatcher status, runner controls, and access management
- Monitoring page orchestration for backend-computed run analytics, pipeline
  performance, trigger activity, runner health/history, LLM usage,
  reliability, efficiency, governance/security views, saved views, alert
  evaluation, alert events, and persisted recommendations
- LLM-backed agents report provider/model/profile token usage to Nopsai through
  internal service-auth endpoints so monitoring can aggregate usage by run,
  pipeline, feature, and subject.
- System feature modules for config, Agent Profiles, LLM profiles, MCP,
  dispatcher, and access keep the route page focused on data loading and
  mutation orchestration.
- Access-grant management for product roles and effective-permission inspection
- Resource Access dialogs on pipelines, scopes, reusable steps, and knowledge contexts for use visibility and team/repository sharing
- Profile page for email and password changes

Key files:

- `services/ui/src/App.tsx`
- `services/ui/src/pages/*.tsx`
- `services/ui/src/features/system`
- `services/ui/src/lib/api.ts`

Inbound interfaces:

- Browser navigation and user interaction

Outbound interfaces:

- REST calls to `nopsai`
- Authenticated fetch with automatic token refresh

## PostgreSQL

Primary role:

- Durable state store for both configuration and execution history.

Responsibilities:

- Stores runs, tasks, logs, configuration, knowledge context, teams (in the compatibility `teams` table), users, roles, refresh tokens, audit logs, backup records, and cleanup job/schedule history.
- Stores product grant metadata, resource visibility, ownership metadata, run authorization snapshots, and AAA policy data; AAA owns the policy schema and policy-table mutations.
- Stores LLM usage events and per-run usage summaries for monitoring and Prometheus token metrics.
- Keeps the execution record durable even though agents and step containers are ephemeral.

Key files:

- `db/init.sql`

## Shared Contracts And Models

`pkg/proto`:

- Defines the gRPC contract between control plane and data plane.

`pkg/models`:

- Defines the pipeline DSL, trigger manifest structure, action schema, and API payload models.

`config`:

- Central config loading from `config.yml` with OS override support.

## Service Relationship Summary

- `git-bot` is the GitHub-facing adapter.
- `nopsai` is the durable control-plane API and state owner.
- `aaa` is the internal authorization decision service.
- `dispatcher` is the scheduler and bridge.
- `runner` is the long-lived Docker worker.
- `agent` is the ephemeral per-run orchestrator.
- `ui` is the operator console.

That separation is the core architectural pattern of the codebase.
