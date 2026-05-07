# NopsAI Architecture Overview

This document explains the current architecture implemented in the repository, not a future-state design.

## What NopsAI Is

NopsAI is a Git-aware pipeline orchestration platform built around a control-plane/data-plane split:

- The control plane accepts events and run requests, stores state, validates configuration, and decides where work should run.
- The data plane runs pipeline work inside Docker containers and reports progress back to the control plane.
- LLM-backed tasks are resolved inside the per-run agent, not by a separate always-on LLM service.

## System Layers

- `services/nopsai`: Main API, source-of-truth database access, auth, RBAC, config sync, run creation, run tracking.
- `services/git-bot`: GitHub App integration, webhook ingress, repository file access, check-run updates.
- `services/dispatcher`: Scheduler and bridge between HTTP-oriented control-plane APIs and gRPC-oriented runners and agents.
- `services/runner`: Long-lived worker that starts agent containers on Docker-capable hosts.
- `services/agent`: Per-run orchestrator that executes pipeline logic and talks to the configured LLM provider.
- `services/ui`: Operator UI for runs, pipelines, triggers, scopes, lab runs, steps, and system management.
- `db/init.sql`: Persistent storage schema for runs, configuration, auth, and audit data.

## High-Level Flow

```text
GitHub / User
    |
    v
git-bot or UI/API
    |
    v
nopsai (validation, DB, config resolution)
    |
    v
dispatcher (queue, affinity, scope routing)
    |
    v
runner (Docker host)
    |
    v
agent container
    |
    v
step containers + child pipeline runs
```

Feedback flows in the opposite direction:

- Agent -> dispatcher -> nopsai for logs, task status, and final status
- nopsai -> git-bot for GitHub check-run updates
- nopsai -> UI through REST reads and live updates

## Core Runtime Model

### Control plane

The control plane lives mostly in `services/nopsai`, `services/git-bot`, and `services/dispatcher`.

- `nopsai` receives manual run requests and forwarded GitHub events.
- It resolves reusable step includes, validates pipeline shape, creates DB records, resolves secrets and variables, and submits jobs to the dispatcher.
- `git-bot` is the GitHub-facing edge. It validates webhook signatures, proxies webhook payloads to `nopsai`, fetches repository contents for config-driven features, and keeps GitHub checks in sync.
- `dispatcher` keeps runners connected over gRPC, chooses an eligible runner, and forwards agent updates back into protected `nopsai` endpoints using an internal JWT.

### Data plane

The data plane lives in `services/runner` and `services/agent`.

- A runner is long-lived and bound to a Docker environment.
- For each run, the runner starts one agent container.
- The agent creates or reuses one step container per step and runs commands inside it.
- Pipeline state is therefore durable in Postgres, while execution state is transient in containers and shared Docker volumes.

## Pipeline Execution Model

The pipeline schema is defined in `pkg/models/model.go`.

Top-level pipeline features:

- `name`, `version`, `description`
- `container_image` as the default execution image
- `variables` as required scope variables
- `timeout`
- `llm_content_sharing`, `llm_output_sharing`, `llm_content_ignore`
- `display_options.github_view`

Step types:

- `include`: Includes a reusable step (`step:...`) or triggers a child pipeline (`pipeline:...`).
- `tasks`: A multi-task step where tasks can depend on earlier tasks in the same step.
- `goal`: A legacy single-task LLM-driven step.
- `script`: A legacy single-task script-driven step.

Step/task controls:

- `depends_on`
- `condition`
- `secrets`
- `volumes`
- `variables`
- `ignore_failure`
- `llm_output_sharing`

## Data Model

Main tables from `db/init.sql`:

- `pipeline_runs`: One record per run, including git context, pipeline definition snapshot, status, scope, parent/child relationships, and GitHub check ID.
- `task_runs`: One record per executable task, including task ordering and final exit code.
- `step_runs`: Step-level tracking table for higher-level summaries.
- `pipeline_run_logs`: Durable log lines ingested from runner/agent activity.
- `pipelines`, `steps`, `triggers`: Stored configuration and overrides.
- `variables`, `secrets`: Runtime configuration data, with secrets encrypted before storage.
- `groups`: Folder/repository tree used by the UI’s pipeline-runs organization.
- `users`, `user_roles`, `role_permissions`, `refresh_tokens`, `audit_logs`: Local auth, RBAC, session, and audit data.

## Configuration Sources

NopsAI mixes Git-backed configuration and database-backed configuration.

- Pipelines can come from the database or be fetched from Git through `git-bot`.
- Reusable steps are stored in the database and can be synchronized from a config repository.
- Trigger manifests can come from Git or be overridden in the database.
- Variables can be synchronized from a config repository or managed directly in the UI/API.
- Secrets are database-managed and encrypted at rest.

Important precedence rules in the current code:

- Reusable `step:` includes are resolved from the `steps` table before execution.
- Scoped variables resolve as `repo+scope -> global+scope`; scoped runs do not fall back to unscoped values.
- Scoped secrets resolve as `repo+scope -> global+scope`; scoped runs do not fall back to unscoped values.
- Unscoped variables and secrets resolve as `repo+default -> global+default`.

## Scheduling Model

The dispatcher uses a few simple but important rules:

- Route by `scope` and optional `dispatcher_routing` config.
- Respect `preferred_runner_id` when a child pipeline should stay near its parent.
- Preserve affinity using `runner_affinity_key`, usually derived from `trigger_event_id`, parent run, or run ID.
- Prefer the least-loaded eligible runner.
- Queue when no eligible runner is currently available.
- Requeue inflight work if a runner disconnects.

## Deployment Shape

`docker-compose.yaml` builds and starts:

- `nopsai`
- `dispatcher`
- `runner`
- `git-bot`
- `nopsai-ui`
- `db`
- build-only images for `agent`, `pipeline`, and `base`

The main operational assumption is that runners have Docker access and can start agent containers that themselves start step containers.

## Supporting Packages

- `config`: Central config loading and env override behavior.
- `pkg/models`: Pipeline DSL and API payload models.
- `pkg/proto`: gRPC contracts between control and data plane.
- `services/nopsai/pkg/auth`: Local JWT auth, refresh tokens, password hashing, rate limiting, and lockout logic.
- `services/nopsai/pkg/authz`: Casbin-backed RBAC enforcement.
- `services/nopsai/pkg/validation`: Pipeline validation rules.

## Architectural Summary

NopsAI currently behaves like a lightweight CI/CD system with:

- GitHub-aware control-plane APIs
- database-backed run durability
- runner-based remote execution
- per-run agent orchestration
- optional LLM-driven step resolution
- a GitOps-style configuration sync path

The design is intentionally modular enough to scale runners independently from the core API while still keeping the current implementation easy to inspect in one repository.
