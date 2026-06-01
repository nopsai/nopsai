# NopsAI Service Reference

This document explains what each service does, which interfaces it owns, and where its main implementation lives.

## `services/nopsai`

Primary role:

- Main API and control-plane brain.

Responsibilities:

- Exposes REST endpoints for auth, runs, pipelines, steps, triggers, knowledge contexts, secrets, variables, groups, and system operations.
- Exposes product access-management endpoints for role grants and effective-permission inspection.
- Stores and reads all durable state from Postgres.
- Validates pipelines and resolves reusable `step:` includes.
- Resolves required knowledge context, secrets, and variables before a run is dispatched.
- Creates run records, task records, and log records.
- Starts config sync from the Git-backed config repo.
- Applies system GitOps runtime settings for runner install defaults, supported
  runtime URLs, agent defaults, and dispatcher routing.
- Seeds predefined product roles and expands role grants into low-level AAA ACLs.
- Talks to the dispatcher as a gRPC client.
- Talks to `git-bot` over HTTP for GitHub checks and repository content access.

Key files:

- `services/nopsai/main.go`
- `services/nopsai/routes.go`
- `services/nopsai/run_handlers.go`
- `services/nopsai/pipeline_handlers.go`
- `services/nopsai/knowledge_context.go`
- `services/nopsai/knowledge_context_schema.go`
- `services/nopsai/secrets_variables_handlers.go`
- `services/nopsai/github_integration.go`
- `services/nopsai/auth_handlers.go`

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
- `git-bot` forwarded GitHub events
- Internal dispatcher-authenticated HTTP calls for logs, task updates, finalization, pipeline fetches, and child pipeline creation

Authorization notes:

- Product roles are seeded as predefined templates: `viewer`, `developer`, `owner`, and `admin`.
- Access grants are written at grant time into existing AAA tables instead of changing evaluator behavior.
- Folder-targeted grants inherit by path to child resources.
- Runtime resource-use checks are caller-based, so Git runs are authorized as repositories, manual runs as users, and dispatcher calls do not inherit resource-owner permissions.
- Managed knowledge context references are checked with `knowledge_context.use` before dispatch.
- Sensitive allowed actions and all denied actions are written to authorization decision logs.

Outbound interfaces:

- gRPC to `dispatcher`
- HTTP to `git-bot`
- HTTP to `aaa`
- Postgres

## `services/aaa`

Primary role:

- Internal policy decision point for authorization.

Responsibilities:

- Serves internal HTTP endpoints for subject introspection, single checks, batch checks, resource filtering, and audit recording.
- Enforces deny-before-allow behavior across direct roles, auth-group roles, direct ACLs, auth-group ACLs, and inherited ACLs.
- Resolves users, groups, roles, resource ACLs, ownership, and inheritance from Postgres.
- Writes authorization decision logs for denied decisions and sensitive allowed decisions.
- Ensures the AAA schema and default internal roles exist at startup.

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
- `nopsai` keeps an in-process evaluator fallback using the same store, so short AAA service outages do not have to stop authorization checks.

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
- Exposes status and runner dispatch controls for the system UI.
- Proxies agent-originated logs, task updates, final status, child pipeline triggers, and pipeline fetches back into `nopsai`.

Key files:

- `services/dispatcher/main.go`
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

## `services/runner`

Primary role:

- Long-lived execution daemon attached to a Docker runtime.

Responsibilities:

- Connects to the dispatcher and registers its `runner_id`, scopes, capacity, and metadata.
- Receives job assignments over gRPC.
- Ensures the agent image exists locally.
- Creates a shared Docker volume for the run.
- Starts the agent container with Docker socket access and the shared workspace volume.
- Streams container logs back through the dispatcher.
- Polls run status so a cancelled run stops the agent container quickly.

Key files:

- `services/runner/main.go`

Inbound interfaces:

- gRPC stream from the dispatcher

Outbound interfaces:

- Docker Engine API
- gRPC to the dispatcher

Operational assumptions:

- The runner has access to `/var/run/docker.sock`.
- The runner can pull images that the pipeline or agent needs.

## `services/k8s-runner`

Primary role:

- Long-lived execution daemon installed in a Kubernetes namespace.

Responsibilities:

- Connects to the dispatcher and registers as a `runtime=kubernetes` runner.
- Starts an agent pod with Kubernetes runtime variables and a run workspace volume.
- Streams agent pod logs back through the dispatcher.
- Polls run status so a cancelled run deletes the active agent pod quickly.
- Passes namespace, service account, storage, affinity, and runtime pool settings to the agent.

Key files:

- `services/k8s-runner/main.go`
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
- Decodes secrets and variables already prepared by `nopsai`.
- Decodes the run knowledge context snapshot prepared by `nopsai`.
- Evaluates step conditions with the configured LLM provider.
- Resolves goal-based tasks into structured actions.
- Executes commands or file writes inside step containers.
- Maintains execution history that later tasks and child pipelines can use.
- Injects effective pipeline + step + task knowledge context into LLM prompts.
- Masks secrets before writing output into the shared history/log path.
- Sends task status and final status back through the dispatcher.

Key files:

- `services/agent/main.go`
- `services/agent/llm.go`
- `pkg/proto/agent.proto`

Inbound interfaces:

- Runtime variables injected by the runner/job payload

Outbound interfaces:

- gRPC to dispatcher
- Docker Engine API
- Gemini or LM Studio HTTP APIs

Notable implementation detail:

- `agent.proto` defines an `LLMService`, but the current runtime uses an embedded `LLMClient` directly in the agent instead of a separate LLM microservice.

## `services/git-bot`

Primary role:

- GitHub App edge service and GitHub-specific adapter.

Responsibilities:

- Validates GitHub webhook signatures.
- Forwards webhook payloads to `nopsai`.
- Reads repository files and directories from GitHub on behalf of `nopsai`.
- Checks repository access and whether a branch has an open PR.
- Creates, initializes, finds, and updates GitHub check runs.
- Tracks step/task state for rich check-run rendering.
- Creates child check runs for included pipelines.

Key files:

- `services/git-bot/main.go`

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
- System pages for config sync, dispatcher status, runner controls, and access management
- Access-grant management for product roles and effective-permission inspection
- Resource Access dialogs on pipelines, scopes, reusable steps, and knowledge contexts for use visibility and group/repository sharing
- Profile page for email and password changes

Key files:

- `services/ui/src/App.tsx`
- `services/ui/src/pages/*.tsx`
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

- Stores runs, tasks, logs, configuration, knowledge context, groups, users, roles, refresh tokens, and audit logs.
- Stores AAA subjects, role bindings, grant metadata, resource visibility, expanded ACLs, ownership metadata, run authorization snapshots, and authorization decision logs.
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
