# NopsAI Service Reference

This document explains what each service does, which interfaces it owns, and where its main implementation lives.

## `services/nopsai`

Primary role:

- Main API and control-plane brain.

Responsibilities:

- Exposes REST endpoints for auth, runs, pipelines, steps, triggers, secrets, variables, groups, and system operations.
- Stores and reads all durable state from Postgres.
- Validates pipelines and resolves reusable `step:` includes.
- Resolves required secrets and variables before a run is dispatched.
- Creates run records, task records, and log records.
- Starts config sync from the Git-backed config repo.
- Talks to the dispatcher as a gRPC client.
- Talks to `git-bot` over HTTP for GitHub checks and repository content access.

Key files:

- `services/nopsai/main.go`
- `services/nopsai/routes.go`
- `services/nopsai/run_handlers.go`
- `services/nopsai/pipeline_handlers.go`
- `services/nopsai/secrets_variables_handlers.go`
- `services/nopsai/github_integration.go`
- `services/nopsai/auth_handlers.go`

Important subpackages:

- `pkg/auth`: Local JWT auth, refresh tokens, password hashing, rate limits, lockout rules.
- `pkg/authz`: Casbin RBAC policy loading and enforcement.
- `pkg/audit`: Audit-log persistence.
- `pkg/store`: Small storage abstraction, currently centered on Postgres.
- `pkg/validation`: Pipeline validation.

Inbound interfaces:

- Browser/UI HTTP traffic
- `git-bot` forwarded GitHub events
- Internal dispatcher-authenticated HTTP calls for logs, task updates, finalization, pipeline fetches, and child pipeline creation

Outbound interfaces:

- gRPC to `dispatcher`
- HTTP to `git-bot`
- Postgres

## `services/dispatcher`

Primary role:

- Scheduling hub and internal bridge between runners/agents and `nopsai`.

Responsibilities:

- Accepts `SubmitJob` requests from `nopsai`.
- Maintains long-lived gRPC streams from runners.
- Tracks connected runners, capacity, scopes, metadata, heartbeat freshness, and inflight jobs.
- Selects a runner based on scope, routing config, runner affinity, preferred runner ID, and current load.
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
- affinity preservation by trigger or run
- manual dispatch pause/resume per runner

## `services/runner`

Primary role:

- Long-lived execution daemon attached to a Docker environment.

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

## `services/agent`

Primary role:

- Per-run orchestrator that actually executes pipeline logic.

Responsibilities:

- Starts once per run with the resolved pipeline definition and runtime context encoded in environment variables.
- Connects to the dispatcher for status reporting, pipeline fetches, child pipeline triggers, and cancellation polling.
- Decodes secrets and variables already prepared by `nopsai`.
- Evaluates step conditions with the configured LLM provider.
- Resolves goal-based tasks into structured actions.
- Executes commands or file writes inside step containers.
- Maintains execution history that later tasks and child pipelines can use.
- Masks secrets before writing output into the shared history/log path.
- Sends task status and final status back through the dispatcher.

Key files:

- `services/agent/main.go`
- `services/agent/llm.go`
- `pkg/proto/agent.proto`

Inbound interfaces:

- Environment variables injected by the runner/job payload

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
- Scope manager for secrets and variables
- Reusable step library
- Lab for ad-hoc YAML execution and quick runs
- System pages for config sync, dispatcher status, runner controls, and access management
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

- Stores runs, tasks, logs, configuration, groups, users, roles, refresh tokens, and audit logs.
- Keeps the execution record durable even though agents and step containers are ephemeral.

Key files:

- `db/init.sql`

## Shared Contracts And Models

`pkg/proto`:

- Defines the gRPC contract between control plane and data plane.

`pkg/models`:

- Defines the pipeline DSL, trigger manifest structure, action schema, and API payload models.

`config`:

- Central config loading from `config.yml` with environment override support.

## Service Relationship Summary

- `git-bot` is the GitHub-facing adapter.
- `nopsai` is the durable control-plane API and state owner.
- `dispatcher` is the scheduler and bridge.
- `runner` is the long-lived Docker worker.
- `agent` is the ephemeral per-run orchestrator.
- `ui` is the operator console.

That separation is the core architectural pattern of the codebase.
