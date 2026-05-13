# NopsAI Feature Reference

This is a feature-oriented view of what the current codebase supports.

## Pipeline Authoring

Supported pipeline features:

- YAML-defined pipelines with `name`, `version`, `description`, `timeout`, and `container_image`
- step-level images
- required scope variables through the top-level `variables` list
- step dependencies through `depends_on`
- multi-task steps
- single-task `goal` steps
- single-task `script` steps
- reusable step inclusion with `step:<identifier>`
- child pipeline inclusion with `pipeline:<identifier>`
- conditional execution with `condition`
- named Docker volume mounting
- per-step and per-task variable overrides
- per-step secret declaration
- ignored failures
- LLM content and output sharing controls
- GitHub display options

Example coverage:

- `sample-pipeline/5-pipeline.yaml` demonstrates LLM goals, scripts, secret usage, volumes, conditions, child pipelines, and reusable-step inclusion.

## Execution Semantics

The runtime supports:

- parallel execution of independent tasks
- dependency-aware execution order
- one shared workspace volume per run
- one reusable container session per step
- child pipeline triggering with inherited execution history
- optional asynchronous child pipeline monitoring
- pipeline-level timeout handling
- task-level secret masking in output/history

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

## Configuration Sync

GitOps-style configuration sync supports:

- `pipelines/` -> pipeline definitions
- `steps/` -> reusable step definitions
- `triggers/` -> trigger overrides
- `environments/` -> scoped variables
- `pipelineruns/structure.yaml` -> legacy UI group hierarchy for groups owned by the syncing repo
- `config-repositories/` -> group config repo bindings, group shells, and colocated group structure files

Sync behavior:

- upsert Git-sourced items into the database
- prune Git-sourced items removed from the config repo
- preserve non-Git groups to avoid deleting user-managed structure
- sync system/global config repositories before group config repositories, so group bindings defined in Git can be picked up during the same sync-all run
- group config repositories are authoritative for resources under their group path; parent repos prune their own managed resources in delegated groups
- `config-repositories/groups/structure.yaml` and `config-repositories/groups/<group>/structure.yaml` can place repositories under group shells and include inline `config:` blocks for group repo bindings
- global legacy `pipelineruns/structure.yaml` does not apply delegated group subtrees; those groups are created from `config-repositories/groups` and owned by their group repos

## API And Run Management

Core run-management capabilities:

- create run
- list runs
- fetch run details
- fetch run status
- rerun completed runs
- cancel active runs
- delete runs
- delete runs by repository branch
- ingest logs
- update task status
- finalize a run
- find a run by GitHub check ID

Configuration management capabilities:

- CRUD pipelines
- CRUD reusable steps
- CRUD trigger overrides
- CRUD scopes, variables, and secrets
- branch listing per repository
- config sync trigger and status
- dispatcher and runner status inspection
- runner dispatch pause/resume

## Authentication, Access, And Audit

Current auth/access features:

- local username/email plus password login
- access token plus refresh token flow
- password changes
- email updates
- login rate limiting
- login lockout after repeated failures
- standalone AAA service with `Check`, `BatchCheck`, `Filter`, and `Introspect`
- in-process AAA fallback in `nopsai` for short service outages
- route-level action/resource mapping for protected REST endpoints
- predefined product roles: `viewer`, `developer`, `owner`, `admin`
- access-grant management API for subject -> role -> resource bindings
- group-path inheritance for child pipelines, runs, repositories, triggers, secrets, variables, and steps
- deny-before-allow evaluation
- effective-permission introspection with human-readable reasons
- legacy Casbin-backed RBAC metadata compatibility
- admin user bootstrap
- audit logging for denied requests and sensitive allowed operations

Important behavior:

- dispatcher-internal calls are trusted only when they carry an internally minted JWT with the expected claims
- UI fetches automatically retry once with refresh-token renewal on `401`
- `developer` can write secret values but cannot read them
- `viewer` and `developer` cannot manage ACLs
- `owner` can manage permissions only inside owned scope
- `admin` remains observable through the normal AAA check path rather than bypassing authorization

## UI Features

Pages present in the current UI:

- `Pipeline runs`: run list, grouped views, recent runs, event grouping, details, logs, rerun, cancel, branch cleanup
- `Pipelines`: pipeline browser/editor, drafts, validation, dependency graphing
- `Triggers`: trigger override browser/editor
- `Scopes`: variable and secret management by scope and repository
- `Lab`: ad-hoc YAML editing and direct run execution
- `Steps`: reusable step library and usage inspection
- `System`: config, dispatcher, runner controls, user/role/access management
- `Profile`: email and password management
- `Login`: local authentication entrypoint

## Operational Features

Operational support already in the code:

- Docker Compose stack for local deployment
- dedicated `aaa` service for internal authorization decisions
- dedicated runner capacities and scope declarations
- dispatcher queue visibility
- active runner metadata display
- active-run inspection from runner metadata
- log batching from runner to API
- REST polling for run lists, details, and incremental log views
- automatic image pre-pull in the agent
- descriptive container naming for agents and step containers

## Current Constraints To Be Aware Of

These are real characteristics of the current implementation:

- gRPC uses insecure credentials by default, which is acceptable for local/dev but not a full production security story.
- Secrets are database-managed; config sync imports variables, steps, triggers, and pipelines, but not secrets.
- Reusable `step:` includes are resolved from the database, not directly from Git at execution time.
- The agent embeds its LLM client directly, even though `agent.proto` still defines a separate `LLMService`.
- Scoped runs intentionally do not fall back to unscoped defaults.

## Quick Summary

The codebase already supports a surprisingly complete CI/CD tool shape:

- GitHub-triggered automation
- manual and ad-hoc runs
- reusable steps and nested pipelines
- scoped runtime configuration
- distributed execution with runners
- LLM-assisted pipeline tasks
- UI-based operations and governance
