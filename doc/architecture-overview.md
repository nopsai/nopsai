# NopsAI Architecture Overview

This document explains the current architecture implemented in the repository, not a future-state design.

## What NopsAI Is

NopsAI is a Git-aware pipeline orchestration platform built around a control-plane/data-plane split:

- The control plane accepts events and run requests, stores state, validates configuration, and decides where work should run.
- The data plane runs pipeline work inside Docker containers and reports progress back to the control plane.
- LLM-backed tasks are resolved inside the per-run agent, not by a separate always-on LLM service.

## System Layers

- `services/nopsai`: Main API, source-of-truth database access, auth, AAA-backed product access control, config sync, Git webhook source ingress, run creation, run tracking.
- `services/aaa`: Internal authorization service for subject introspection, checks, filtering, and authz decision audit writes.
- `services/git-bot`: GitHub App integration, webhook ingress, repository file access, check-run updates.
- `services/dispatcher`: Scheduler and bridge between HTTP-oriented control-plane APIs and gRPC-oriented runners and agents.
- `services/docker-runner`: Long-lived worker that starts agent containers on Docker-capable hosts.
- `services/agent`: Per-run orchestrator that executes pipeline logic and talks to the configured LLM provider.
- `services/ui`: Operator UI for runs, pipelines, triggers, scopes, lab runs, steps, knowledge context, and system management.
- `db/init.sql`: Persistent storage schema for runs, configuration, auth, and audit data.

## High-Level Flow

```text
Git providers / User
    |
    v
git-bot or UI/API
    |
    v
nopsai (auth, validation, DB, config resolution)
    | \
    |  -> aaa (authorization checks)
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
- nopsai -> aaa for authorization checks and filtered list decisions
- nopsai -> git-bot for GitHub check-run updates
- nopsai -> UI through authenticated REST reads, periodic polling, and authenticated SSE fetch streams for System Logs

## Core Runtime Model

### Control plane

The control plane lives mostly in `services/nopsai`, `services/aaa`, `services/git-bot`, and `services/dispatcher`.

- `nopsai` receives manual run requests, forwarded GitHub events, and
  authenticated provider-normalized Git webhook deliveries.
- It authenticates requests, asks AAA for route-level decisions, resolves reusable step includes, validates pipeline shape, creates DB records, resolves knowledge context, secrets, and variables, and submits jobs to the dispatcher.
- `aaa` is the internal policy decision service. It handles introspection, check, batch-check, filter, and audit-record requests behind a shared internal token.
- `git-bot` is the GitHub-facing edge. It validates webhook signatures, proxies webhook payloads to `nopsai`, fetches repository contents for config-driven features, and keeps GitHub checks in sync.
- GitLab, Bitbucket, Gitea, and generic webhook adapters live in `nopsai`.
  They normalize and audit ingress but intentionally do not own provider
  repository reads or status/check APIs.
- `dispatcher` keeps runners connected over gRPC, chooses an eligible runner, and forwards agent updates back into protected `nopsai` endpoints using a service-auth JWT.
- System Logs uses a bounded in-memory broker and an allow-listed provider contract. Docker deployments read through a least-privilege socket proxy; the NopsAI API never mounts the Docker socket and platform logs are not persisted in pipeline history.

### Data plane

The data plane lives in `services/docker-runner`, `services/k8s-runner`, and `services/agent`.

- A Docker runner is long-lived and bound to a Docker runtime.
- A Kubernetes runner is long-lived inside one namespace and starts agent pods with an agent-owned PVC workspace.
- For each run, the runner starts one agent container or agent pod.
- The agent creates or reuses one step container or step pod per step and runs commands inside it.
- Pipeline state is therefore durable in Postgres, while execution state is transient in containers, pods, Docker volumes, or Kubernetes PVCs.

## Pipeline Execution Model

The pipeline schema is defined in `pkg/models/model.go`.

Top-level pipeline features:

- `name`, `version`, `description`
- `container_image` as the default execution image
- `variables` as required scope variables
- `timeout`
- `llm_enabled` to disable all LLM-backed behavior for script-only pipelines
- `llm_content_sharing`, `llm_output_sharing`, `llm_content_include`, `llm_content_ignore`
- `knowledge_context` for managed or repo-local project knowledge
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

- `pipeline_runs`: One record per run, including git context, pipeline definition snapshot, status, scope, parent/child relationships, GitHub check ID, trigger source, requested/effective subject, and an authorization snapshot.
- `task_runs`: One record per executable task, including task ordering and final exit code.
- `step_runs`: Step-level tracking table for higher-level summaries.
- `pipeline_run_logs`: Durable log lines ingested from runner/agent activity.
- `pipelines`, `steps`, `triggers`: Stored configuration and overrides. Pipelines and reusable steps also carry resource visibility for runtime sharing.
- `variables`, `secrets`: Runtime configuration data, with secrets encrypted before storage.
- `knowledge_contexts`: Managed markdown knowledge documents grouped by kind/group/name, with GitOps source metadata.
- `pipeline_run_knowledge_contexts`: Per-run snapshots of resolved knowledge content.
- `groups`: Folder/repository tree used by the UI’s pipeline-runs organization.
- `users`, `user_roles`, `role_permissions`, `refresh_tokens`, `personal_access_tokens`, `service_account_tokens`, `audit_logs`: Local auth, legacy RBAC metadata, session, personal API credentials, service account credentials, and audit data.
- `credentials`, `credential_versions`, `credential_access_logs`: Encrypted,
  versioned system integration credentials and purpose-bound consumer audit.
- `auth_groups`, `auth_group_members`, `auth_roles`, `auth_role_bindings`, `auth_role_permissions`: AAA-owned policy data used by the policy engine; product access grants can target users, auth groups, repositories, triggers, service accounts, and internal services.
- `resource_visibility`: Visibility settings for reusable resources, including knowledge contexts.
- `access_grants`, `resource_ownership`: Product-owned grant intent and ownership metadata used by the access UI/API.
- `resource_acl`, `authz_decision_logs`: AAA-owned expanded ACL rows and authorization decision audit logs.

### Run Organization Model

The canonical navigation model for enterprise pipeline activity is:

```text
Workspace
  -> folder/team/product area
      -> pipelines
      -> schedules
      -> external triggers
      -> repositories
      -> runs
      -> scopes, secrets, variables, and access
```

In the current schema, `groups.kind = 'group'` represents folder/team/product
area nodes and `groups.kind = 'app'` represents application or repository
nodes. The stable product boundary is the folder/group path. Repositories are
one possible source of code or events, and one possible runtime identity; they
are not required parents for pipelines. A pipeline without a repository should
still have a logical path such as `platform/prod/deploy-prod`, with
`platform/prod` as the owning folder.

`pipeline_runs.group_id` points at the best existing owner for the run. Run
creation resolves that owner in this order:

1. explicit `X-Nopsai-Group-Path`, used by schedules and other first-class
   owners
2. the pipeline path, so manual, schedule, external-trigger, and repo-less runs
   still attach to a product folder
3. repository metadata, for raw Git-sourced runs without a logical pipeline
   path

Selecting a folder in the run list includes runs attached to that folder and
all descendants, so parent folders act as dashboards rather than empty
navigation shells.

`pipeline_runs.scope` is separate from this hierarchy. A scope is the runtime
environment or context for a run, such as `dev`, `staging`, `prod`,
`customer-a`, or `region-eu`; it should be exposed as a run attribute and
filter, not as a folder under pipeline runs.

Pipeline run rows are runtime/audit records and should not create the
navigation structure. Config repositories, setup, or explicit UI/API actions
define folders, apps, schedules, triggers, and pipeline paths; runs then
reference that structure:

- manual pipeline runs use the pipeline path/folder and the user subject
- GitHub webhook runs use the owning folder plus repository metadata, with the
  repository as runtime identity
- generic Git webhook runs use synchronized trigger/pipeline configuration plus
  repository metadata, with the repository as runtime identity
- schedule runs use the schedule group path and schedule service account
- external-trigger runs use the target pipeline or trigger group path and the
  allowed user/service account caller
- runs with no path, repository, schedule, or trigger owner remain ungrouped
  until the UI presents source-based buckets

## Authorization Model

NopsAI now uses a two-layer authorization shape:

- The standalone AAA service is the primary policy decision point, with an in-process evaluator fallback in `nopsai` for temporary AAA service outages.
- Product roles such as `viewer`, `developer`, `owner`, and `admin` are a UX layer that expands into low-level AAA permissions when a grant is created.

Important evaluator properties that remain unchanged:

- default deny
- deny before allow
- direct role permissions
- direct resource ACLs
- auth-group expansion
- resource inheritance
- audit logging for denied and sensitive allowed decisions

Important product-layer properties:

- Folder grants are stored once at the parent folder path and inherited by child folders, pipelines, runs, repositories, scoped secrets, scoped variables, triggers, reusable steps, and knowledge contexts.
- Product grants do not require evaluator-specific awareness; `nopsai` records grant intent and delegates low-level role, binding, permission, and ACL mutations to `services/aaa/pkg/store`.
- Platform admin still flows through normal `Check` decisions, so sensitive admin actions remain visible in audit logs.
- Runtime resource-use authorization is caller-based: manual runs use the user, Git-triggered runs use the repository, and internal dispatcher calls do not gain permissions from resource owners.
- Reusable resources can be `group`, `restricted`, or `workspace` visible. The UI labels workspace visibility as `Public`, but callers still need the appropriate use action and related resources such as scopes and secrets are checked separately.

## Configuration Sources

NopsAI mixes Git-backed configuration and database-backed configuration.

- Pipelines can come from the database or be fetched from Git through `git-bot`.
- Reusable steps are stored in the database and can be synchronized from a config repository.
- Trigger manifests can come from Git or be overridden in the database.
- Variables can be synchronized from a config repository or managed directly in the UI/API.
- Knowledge contexts can be synchronized from `knowledge/` in a config repository or managed directly in the UI/API.
- Secrets are database-managed and encrypted at rest.

Important precedence rules in the current code:

- Reusable `step:` includes are resolved from the `steps` table before execution.
- Scoped variables resolve as `repo+scope -> global+scope`; scoped runs do not fall back to unscoped values.
- Scoped secrets resolve as `repo+scope -> global+scope`; scoped runs do not fall back to unscoped values.
- Default-scope variables and secrets are stored as `scope = 'default'` and resolve as `repo+default -> global+default`.
- Runtime references may override the run scope with `scope:NAME`. For example, `dev:TEST_ENV` resolves from `dev` and is injected as `TEST_ENV`; authorization is checked against the resolved `dev` resource.
- Managed knowledge references resolve from `knowledge_contexts` with `knowledge_context.use`; repo-local knowledge paths resolve through `git-bot` at the run commit. Both forms are snapshotted before dispatch.

## Scheduling Model

The dispatcher uses a few simple but important rules:

- Route by `scope` and optional `dispatcher_routing` config. Runner scope
  registration is the first filter; `dispatcher_routing` can further allow-list
  runner IDs for a scope.
- Prefer `preferred_runner_id` when a child pipeline should stay near its parent.
- Prefer affinity using `runner_affinity_key`, usually derived from `trigger_event_id`, parent run, or run ID.
- Prefer the least-loaded eligible runner.
- Queue when no eligible runner is currently available.
- Requeue inflight work if a runner disconnects.

## Deployment Shape

`docker-compose.yaml` builds and starts:

- `nopsai`
- `aaa`
- `dispatcher`
- `runner`
- `git-bot`
- `nopsai-ui`
- `db`
- build-only images for `agent`, `pipeline`, `k8s-runner`, and `base`

The main operational assumption is that runners have Docker access and can start agent containers that themselves start step containers.

## Supporting Packages

- `config`: Central config loading and OS override behavior.
- `pkg/models`: Pipeline DSL and API payload models.
- `pkg/proto`: gRPC contracts between control and data plane.
- `services/nopsai/pkg/auth`: Local JWT auth, refresh tokens, password hashing, rate limiting, and lockout logic.
- `services/nopsai/pkg/authz`: Casbin-backed legacy RBAC enforcement still used by older role metadata paths.
- `services/nopsai/pkg/aaaclient`: HTTP client for the internal AAA service.
- `services/nopsai/pkg/routeauthz`: HTTP route to action/resource mapping.
- `services/aaa/pkg/authz`, `services/aaa/pkg/server`, and `services/aaa/pkg/store`: The AAA server, evaluator, policy schema ownership, policy mutation helpers, inheritance resolution, and decision logging implementation.
- `services/nopsai/pkg/validation`: Pipeline validation rules.

## Architectural Summary

NopsAI currently behaves like a lightweight CI/CD system with:

- GitHub-aware control-plane APIs
- database-backed run durability
- runner-based remote execution
- per-run agent orchestration
- optional LLM-driven step resolution
- knowledge-guided LLM prompts with run snapshots
- a GitOps-style configuration sync path

The design is intentionally modular enough to scale runners independently from the core API while still keeping the current implementation easy to inspect in one repository.
