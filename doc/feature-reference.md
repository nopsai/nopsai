# NopsAI Feature Reference

This is a feature-oriented view of what the current codebase supports.

## First-install Setup

The UI exposes a first-install wizard at **System > Setup** for turning an empty
database into a runnable workspace.

Supported setup capabilities:

- setup status and health checks for database, admin bootstrap, local secrets,
  GitHub App settings, git-bot service configuration, access bootstrap,
  LLM/MCP state, starter pipeline presence, and runner health
- public setup preflight for missing database, master-key, and JWT prerequisites
  before login
- step-by-step setup modal with required gates and skippable optional steps, shown
  after the first admin password change
- persistent setup reference page after completion, including generated runtime
  env groups, GitOps zip download, and starter file preview
- local generation of missing signing, webhook, AAA, and dispatcher secret values
- global GitOps config repository creation and optional sync kickoff
- generated runtime variable output for container environment, secret-manager
  entries, or environment files
- starter GitOps template preview for pipelines, reusable steps, scopes,
  triggers, access bootstrap, knowledge docs, Agent Profiles, LLM profiles, MCP
  settings, and run group structure
- direct starter database seeding for groups, starter pipeline, reusable step,
  triggers, variables, knowledge context, optional LLM profile, optional MCP
  examples, and optional users
- GitHub App and git-bot installation guidance
- one or two starter repository groups with selected repositories underneath
- starter users with group role assignment, password creation, and forced first
  password change
- guardrails that flag insecure default admin state and missing webhook
  verification

See [first-install-wizard.md](./first-install-wizard.md) for the operator flow
and setup endpoint examples.

## Pipeline Authoring

Supported pipeline features:

- YAML-defined pipelines with `name`, `version`, `description`, `timeout`, and `container_image`
- step-level images
- required scope variables through the top-level `variables` list
- step dependencies through `depends_on`
- multi-task steps
- single-task `goal` steps
- single-task `script` steps
- approval steps with `approval.type`, assigned `approval.groups`, and optional `approval.allow_self_approval`
- reusable step inclusion with `step:<identifier>`
- child pipeline inclusion with `pipeline:<identifier>`
- conditional execution with `condition`
- named Docker volume mounting
- per-step and per-task variable overrides
- per-step secret declaration
- ignored failures
- LLM content and output sharing controls
- pipeline- and step-level Agent Profile selection through `agent_profile`
- knowledge context references for architecture docs, guardrails, policies, ADRs, guidelines, runbooks, references, and examples
- GitHub display options

Example coverage:

- `sample-pipeline/5-pipeline.yaml` demonstrates LLM goals, scripts, secret usage, volumes, conditions, child pipelines, reusable-step inclusion, and knowledge context.

Approval step example:

```yaml
steps:
  - name: deploy-gate
    depends_on: [build]
    approval:
      type: production-deploy
      groups:
        - platform/prod
      allow_self_approval: false
```

Approval group paths are relative folder paths. Any assigned group where the caller has `approval.approve` can approve or reject the pending gate. Pending approval runs are visible to assigned approvers even when the pipeline itself belongs to another folder, so approval queues do not depend on broad pipeline ownership.

## Execution Semantics

The runtime supports:

- parallel execution of independent tasks
- dependency-aware execution order
- one shared workspace volume per run
- one reusable container session per step
- child pipeline triggering with inherited execution history
- optional asynchronous child pipeline monitoring
- approval checkpoints that pause a run without holding an agent or runner
- approval resume from stored variables, execution history, completed task keys, and compressed workspace archive
- pipeline-level timeout handling
- task-level secret masking in output/history
- pre-dispatch knowledge context resolution and run snapshots

## Pipeline Scheduling

Pipeline schedules are first-class resources for time-based automation:

- schedules target stored pipelines and do not require a Git repository event
- each schedule has a resource group path, optional run group path, schedule
  kind, cron expression or one-time timestamp, timezone, enabled state,
  optional scope, and optional variable overrides
- the schedule page lists visible schedules with next run time, latest run
  status, GitOps source, and links to the latest pipeline run
- UI-created schedules derive their path from the selected pipeline; API and
  GitOps schedules may still set explicit organizational paths
- the schedule form uses existing pipelines and scopes as dropdown choices and
  offers friendly specific-date, interval, hourly, daily, weekday, weekly,
  monthly, yearly, or custom timing controls; weekly and monthly modes support
  multiple selected days
- schedules can be enabled, disabled, run immediately, edited, or deleted when
  the caller has the matching `pipeline_schedule.*` action
- scheduled runs are tagged with `trigger_source: schedule`, linked to
  `schedule_id`, and badged in Pipeline runs
- execution uses a schedule-owned service account with explicit pipeline,
  scope, reusable-step, and child-pipeline grants

Schedule resource grouping is organizational. A path such as `prod/scheduled`
is a good way to present production automation, while `run_group_path` controls
where scheduled runs appear in Pipeline Runs and which notification policy
lineage receives their events. The nearest policy on the run group or one of
its ancestors wins.

## Knowledge Context

Knowledge Context lets pipelines attach project knowledge to LLM-backed work.
Context can be declared at the pipeline, step, or task level; the agent injects
the merged effective context for the current task.

Supported references:

- managed documents by `kind` plus `ref`, such as `guardrail` and `security/repo-check`
- repo-local markdown by `kind` plus `path`, such as `architecture` and `.nopsai/docs/backend.md`

Managed documents are first-class resources with `knowledge_context.use`
runtime checks. Repo-local documents are loaded from the run repository at the
run commit. Resolved content is stored in `pipeline_run_knowledge_contexts` so
each run records exactly what the agent saw.

See [knowledge-context.md](./knowledge-context.md) for the YAML schema, GitOps
layout, access behavior, and API examples.

## Secrets And Variables

Secrets:

- Stored in Postgres
- Encrypted at rest with AES-GCM derived from `NOPSAI_MASTER_KEY`
- Declared by step name through `secrets`
- Resolved before the run is submitted
- Masked from logs and history output inside the agent

Variables:

- Stored in Postgres
- Can be synchronized from a config repo
- Declared as required pipeline inputs in the top-level `variables` array
- Can be overridden at run time through the run API
- Can be overridden at step and task level for container execution context

Scope behavior:

- Scoped runs look only at scoped values for the requested scope.
- Unscoped values are used only for unscoped runs.
- Repository-specific values take precedence over global values inside the same scope layer.
- A pipeline can explicitly reference another scope with `scope:NAME`, such as `dev:TEST_ENV`; the value is resolved from that scope and injected as runtime variable `TEST_ENV`.
- `default:NAME` explicitly targets the unscoped/default value.
- Runtime authorization is checked against the concrete scoped secret or variable that was resolved.

## Triggering And GitHub Integration

Git-aware features:

- GitHub webhook ingestion through `git-bot`
- trigger manifests with:
  - event matching
  - branch globs
  - tag globs
  - skipped branches
  - skipped repositories
  - pipeline lists
  - scope selection
- repository-specific trigger overrides in the database
- owner-wide `owner/all` trigger override support
- GitHub check-run creation and updates
- check-run re-request handling for reruns
- stale check cancellation
- branch open-PR checks
- repository access verification for config sync
- config Git push commits to a configured review branch

## Configuration Sync

GitOps-style configuration sync supports:

- `pipelines/` -> pipeline definitions
- `steps/` -> reusable step definitions
- `schedules/` -> one-time and recurring pipeline schedules
- `triggers/` -> trigger overrides
- `scopes/` -> scoped variables declared under `variables:` and GitOps secret
  keys declared under `secrets:`
- `knowledge/` -> managed knowledge context markdown documents
- `config-repositories/` -> group config repo bindings, group shells, colocated group structure files, and system-repo group notification policies
- `setting/system/auth.yaml` -> local-login and OIDC SSO settings from a global config repo
- `setting/system/mail.yaml` -> SMTP mail notification settings from a global config repo
- `setting/system/llm_profile.yaml` -> system LLM profile registry from a global config repo
- `setting/system/agent-profiles.yaml` -> system Agent Profile persona registry and default profile setting from a global config repo
- `setting/system/mcp.yaml` -> system MCP server and profile registry from a global config repo
- `setting/system/runner.yaml` -> runner install defaults, runtime URLs, and dispatcher routing from a global config repo

Sync behavior:

- upsert Git-sourced items into the database
- prune Git-sourced items removed from the config repo
- preserve non-Git groups to avoid deleting user-managed structure
- reject flat top-level variable entries in scope files; scoped variables must
  be nested under `variables:`
- import GitOps secret values only when they decrypt with the current NopsAI
  master key; otherwise keep the secret key with no value
- sync system/global config repositories before group config repositories, so group bindings defined in Git can be picked up during the same sync-all run
- group config repositories are authoritative for resources under their group path; parent repos prune their own managed resources in delegated groups
- config repository bindings can enable Git push to a review branch with `write_enabled` and `write_branch`
- config repository drift compares both directions across syncable declarative resources: pipelines, reusable steps, schedules, triggers, scopes, knowledge contexts, notification routes, run group/config-repository structure, access manifests, Agent Profiles, LLM profiles, MCP registry files, auth settings, mail settings, and runtime settings. UI-side Access dialog changes for pipelines, reusable steps, scopes, and knowledge contexts are exported back into embedded GitOps `access:` blocks; pipeline run rows remain runtime/audit state.
- config sync can adopt matching database-owned resources inside the syncing repo scope after the generated files are present in the sync branch, then mark them as GitOps-managed
- `config-repositories/groups/<group>/structure.yaml` can place apps under group shells with `name` and `repo_url`; these files can also include inline `config:` blocks for group repo bindings
- auth settings GitOps is system/global only and can omit provider secret fields to preserve locally stored values
- runtime settings GitOps is system/global only; `dispatcher_routing` changes are persisted and applied by the live dispatcher through the control-plane sync path
- mail settings GitOps is system/global only and stores `smtp.password_secret_ref` rather than the SMTP password value

## Notifications And Metrics

Pipeline notifications include:

- Prometheus-friendly `GET /metrics` with DB-backed pipeline run, duration,
  queue duration, end-to-end duration, active/pending/approval, step, task,
  external trigger, LLM usage, runner capacity, approval wait, audit, and
  notification delivery metrics
- backend-computed monitoring analytics under **Monitoring** with tabs for
  Overview, Runs, Pipelines, Steps & Tasks, Triggers, External Triggers,
  Runners, LLM Usage, Reliability, Efficiency, and Security
- monitoring step analytics fall back to task-run grouping for historical runs
  that do not have explicit `step_runs`, and monitoring tables link supported
  object references back to pipelines, pipeline runs, external triggers, and
  dispatcher runner status
- shared monitoring filters for time range, group, pipeline, repository,
  pipeline run ID, trigger source, status, subject identity, external trigger, schedule,
  duration range, and previous-period comparison with tab-level regression
  deltas
- access-filtered aggregate endpoints that reuse `pipeline_run.list` so normal
  users see only permitted run analytics while admins can see global metrics
- LLM usage accounting through agent-recorded `ai_usage_events` and
  `pipeline_run_usage_summary`; provider token metadata is stored when
  available, with estimated token counts marked in metadata and reported
  separately when providers omit exact usage
- Pipeline Runs shows compact per-run LLM token totals on cards and run detail,
  step/task token totals in the step detail modal, and a run-scoped LLM Usage
  Monitoring link for deeper filtering by pipeline, step, task, model, profile,
  feature, subject, and exact/estimated token source
- runner trend sampling through `runner_metric_snapshots` with hourly capacity,
  active-job, inflight-job, queued-job, and utilization timelines
- GitOps-ready monitoring saved views and alert rules through
  `monitoring_saved_views`, `monitoring_alert_rules`, and
  `monitoring_alert_events`, including database UI workflows, evaluator events,
  and config repository source metadata for managed definitions
- external trigger last-fired and rate-limit violation analytics in Monitoring
- persisted monitoring recommendations through `monitoring_recommendations`,
  with open/acknowledged/resolved workflow status
- system-level mail settings under **System > Config** and
  `GET|PUT /v1/system/notifications/mail`
- `POST /v1/system/notifications/mail/test` for validating SMTP delivery with
  a branded configuration summary and no credential values
- multipart HTML and plain-text pipeline mail with glanceable status headers,
  failed step/task details, step/task progress, repository/run links, optional
  NopsAI footer branding, and bounded redacted error excerpts
- group-level notification routing under
  `GET|PUT|DELETE /v1/groups/{group}/notifications`
- GitOps support for global `config-repositories/groups/<group>/notifications.yaml`
  files and delegated group-repo `notifications.yaml` files at the configured
  repository base path; review drift from the Pipeline Runs group settings so
  the repository that owns the group performs the export
- one or more named routes per group policy, each with same-group recipients,
  explicit users/groups, excludes, event selection, pipeline/repository/branch
  filters, mail channels, and dedupe/max-per-run throttling
- explicit schedule and external-trigger `run_group_path` selection so runtime
  notifications can target the operational group even when the pipeline is
  defined elsewhere; selectable groups come from the Pipeline Runs hierarchy
- asynchronous mail delivery for running, pending, success, failure, cancelled,
  approval requested, approval approved, and approval rejected events when a
  saved or GitOps-managed route exists for the run group

## API And Run Management

Core run-management capabilities:

- create run
- list runs
- list runs for a selected group/folder including descendant folders and apps
- list root runs that are not assigned to any group
- fetch run details
- fetch run status
- list run approvals
- approve or reject pending run approvals
- rerun completed runs
- cancel active runs
- delete runs
- delete runs by repository branch
- ingest logs
- update task status
- finalize a run
- find a run by GitHub check ID

Run organization behavior:

- folder/group path is the stable product boundary for pipelines, schedules,
  external triggers, repositories, and runs
- the Pipeline Runs root shows top-level groups, top-level applications, and
  runs without a group assignment
- pipeline path is used as the run owner when a run has no repository or
  explicit group path
- repository metadata remains a source/runtime identity for Git-triggered runs,
  not a mandatory parent for every pipeline
- scope remains a runtime environment/context attribute and filter; it is not a
  navigation parent under pipeline runs

Configuration management capabilities:

- CRUD pipelines
- CRUD pipeline schedules, enable/disable schedules, and run schedules on demand
- CRUD reusable steps
- CRUD trigger overrides
- CRUD scopes, variables, and secrets
- CRUD knowledge context documents
- branch listing per repository
- config sync trigger and status
- dispatcher and runner status inspection
- monitoring page service status, runner summaries, and active runs filtered by
  the caller's pipeline-run access
- runner dispatch pause/resume

## Authentication, Access, And Audit

Current auth/access features:

- local username/email plus password login
- access token plus refresh token flow
- profile-managed personal access tokens for API automation
- password changes
- email updates
- login rate limiting
- login lockout after repeated failures
- standalone AAA service with `Check`, `BatchCheck`, `Filter`, and `Introspect`
- in-process AAA fallback in `nopsai` for short service outages
- route-level action/resource mapping for protected REST endpoints
- predefined product roles: `viewer`, `developer`, `owner`, `admin`
- `developer`, `owner`, and `admin` can approve assigned approval steps through `approval.approve`
- access-grant management API for subject -> role -> resource bindings
- schedule resources with `pipeline_schedule.list/read/create/update/execute/delete/manage_acl`
- per-resource Access controls for pipeline, reusable step, scope, and knowledge context usage
- caller-based runtime use checks for manual, Git-triggered, and child-pipeline runs
- resource visibility modes: group, restricted, and UI-labeled Public
- group-path inheritance for child pipelines, runs, repository-associated apps, triggers, secrets, variables, steps, and knowledge contexts
- deny-before-allow evaluation
- effective-permission introspection with human-readable reasons
- legacy Casbin-backed RBAC metadata compatibility
- admin user bootstrap
- audit logging for denied requests and sensitive allowed operations

Important behavior:

- dispatcher-internal and agent-internal calls are trusted only when they carry a service-auth JWT with the expected service identity and role
- UI fetches automatically retry once with refresh-token renewal on `401`
- `developer` can write secret values but cannot read them
- `viewer` and `developer` cannot manage ACLs
- `owner` can manage permissions only inside owned scope
- `admin` remains observable through the normal AAA check path rather than bypassing authorization
- sharing a pipeline or step does not share scopes, secrets, variables, runners, or the resource owner's permissions

## UI Features

Pages present in the current UI:

- `Pipeline runs`: subgroup/application/run panels, source-grouped runs, recent runs, event grouping, details, logs, rerun, cancel, branch cleanup
- `Pipeline runs`: pending approval records with assigned groups and approve/reject actions inside run details
- `Pipelines`: pipeline browser/editor, drafts, validation, dependency graphing, and Execute handoff to Lab
- `Schedules`: schedule browser, pipeline-filtered schedule view, enable/disable, run now, latest-run link, and GitOps markers
- `Triggers`: trigger override browser/editor
- `Scopes`: variable and secret management by scope and repository, including scope use-access controls
- `Lab`: ad-hoc YAML editing, preselected pipeline handoff, and direct run execution
- `Steps`: reusable step library, usage inspection, and step use-access controls
- `Knowledge Context`: kind/group/document browser, markdown editor/preview, source metadata, access settings, and usage inspection
- `System`: config, data management, dispatcher, runner controls, user/role/access management
- `Profile`: email and password management
- `Login`: local authentication entrypoint

## Operational Features

Operational support already in the code:

- Docker Compose stack for local deployment
- dedicated `aaa` service for internal authorization decisions
- dedicated runner capacities and scope declarations
- Kubernetes runner one-time install commands, GitOps manifests, namespace-scoped RBAC, agent-owned PVC workspaces, and agent-to-step node-affinity controls
- dispatcher queue visibility
- active runner metadata display
- active-run inspection from runner metadata
- data backups for full database, runs, or logs, stored as downloadable compressed JSONL files
- manual data cleanup with dry-run preview for run retention and log deletion
- scheduled cleanup rules with due-time polling, backup-before-cleanup support, job history, and deleted-row counts
- log batching from runner to API
- REST polling for run lists, details, and incremental log views
- automatic image pre-pull in the agent
- descriptive container naming for agents and step containers

## Current Constraints To Be Aware Of

These are real characteristics of the current implementation:

- gRPC uses insecure credentials by default, which is acceptable for local/dev but not a full production security story.
- Secrets are database-managed; config sync imports variables, steps, triggers, pipelines, and knowledge contexts, but not secrets.
- Reusable `step:` includes are resolved from the database, not directly from Git at execution time.
- The agent embeds its LLM client directly, even though `agent.proto` still defines a separate `LLMService`.
- Scoped runs intentionally do not fall back to unscoped defaults.

## Quick Summary

The codebase already supports a surprisingly complete CI/CD tool shape:

- GitHub-triggered automation
- manual and ad-hoc runs
- reusable steps and nested pipelines
- scoped runtime configuration
- knowledge-guided LLM execution
- distributed execution with runners
- LLM-assisted pipeline tasks
- UI-based operations and governance
