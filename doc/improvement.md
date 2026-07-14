## Platform Improvements Overview

This document captures the current feature set and hardening work that has landed across the Nopsai stack. Use it as a quick orientation guide when exploring the services, deployment story, and day-to-day operations.

### Core Orchestrator (`services/nopsai`)
- **Authenticated REST control plane**: Handles login, token refresh, route authorization, config sync, run creation, and system management through the REST API.
- **AAA integration**: Maps routes to action/resource pairs, calls the internal AAA service, filters list responses, and falls back to a local evaluator during short AAA outages.
- **Dispatcher handoff**: Creates run records and submits jobs to the dispatcher instead of launching agent containers directly.
- **Secrets & scope resolution**: Derives scoped secrets and scope variables (global, scope-specific, repo-specific) and encrypts secrets at rest with the `NOPSAI_MASTER_KEY` before persisting them.
- **Pipeline & trigger APIs**: Exposes CRUD endpoints for pipelines, reusable steps, trigger overrides, run teams, run logs, reruns, cancellations, and branch-level clean-up.
- **Git-driven orchestration**: Receives Git events, ensures the config repository is synced (pipelines, steps, scopes, triggers), and coordinates with the git-bot for GitHub check updates.

### AAA (`services/aaa`)
- **Policy decision service**: Provides internal introspection, check, batch-check, filter, and audit-record endpoints protected by `X-Internal-Token`.
- **Product-role backing model**: Uses Postgres tables for role bindings, role permissions, access grants, expanded ACLs, ownership, and authz decision logs.
- **Auditability**: Logs denied decisions and sensitive allowed decisions while preserving deny-before-allow semantics.

### Agent (`services/agent`)
- **LLM-assisted execution**: Resolves natural-language goals via the embedded Gemini client, while falling back to explicit scripts when provided.
- **Task graph engine**: Understands nested step/task dependencies, honours per-task `depends_on`, and tracks partial progress so multiple independent tasks can run without blocking each other.
- **Workspace sharing controls**: Shares a sanitised directory listing with the LLM, respecting `llm_content_include` and `llm_content_ignore`, and honours both pipeline-level and task-level `llm_output_sharing` settings.
- **Container orchestration**: Ensures required images exist, reuses warm containers per step, supports additional volume mounts, and injects scoped secrets/variables safely.
- **Operational telemetry**: Masks secrets in command output, batches logs back to the server, and reports granular task status (with exit codes and LLM latency) to the API.

### Git Bot (`services/git-bot`)
- **Secure webhooks**: Verifies GitHub signatures, normalises incoming events, and forwards run intents to the core service.
- **Rich check runs**: Maintains hierarchical check-run state (steps ↦ tasks), renders markdown trees, and updates GitHub with real-time summaries, rerun links, and failure context.
- **Developer utilities**: Provides endpoints for repo access checks, file/directory content resolution, pipeline source retrieval, child check-run creation, and stale check-run cancellation.

### Web UI (`services/ui`)
- **Run dashboards**: Uses authenticated REST polling for run lists, details, teamed views, and recent activity.
- **Run explorer**: Offers branch-aware run cards, team organization, inline branch clean-up, selection-driven bulk actions, and deep links that preserve UI state in the hash.
- **Pipeline graph**: Visualises pipeline steps and intra-step tasks with pan/zoom support, expandable nodes, and live status colouring.
- **Log experience**: Ships an on-demand log modal with level filters, agent-only toggle, structured/short views, search with navigation, clipboard/download helpers, and follow mode, powered by incremental REST log polling.
- **Access management**: Adds system-page controls for users, roles, product grants, and effective-permission inspection.
- **Runner visibility**: Shows every runner registered with the dispatcher during its lifetime, marking disconnected records as unreachable while live routing and dispatch only use reachable connections.

### Configuration & Deployment
- **Single compose stack**: Docker Compose provisions Postgres, the Go services, the AAA service, the UI (served via nginx), plus build-only helper images (`base`, `agent`, `pipeline`, `k8s-runner`).
- **Shared network & volumes**: All services join `nopsai-net`; run workspaces use throwaway Docker volumes, and Postgres writes to a named volume for persistence.
- **Unified config loader**: `config/config.go` reads YAML defaults and lets OS variables override per deployment, covering ports, service URLs, Gemini credentials, Docker preferences, and timeout knobs.

### Feature Highlights
- **Pipeline DSL**: Supports step includes, per-step containers, secret injection, volume mounts, multi-task steps, AI gating via `condition`, and fine-grained LLM sharing controls.
- **Trigger routing**: `.nopsai/triggers.yaml` can target events by branch/tag globs, apply scope overrides, and fan out to multiple pipelines.
- **Run lifecycle**: Manual kicks, reruns, cancellation, GitHub-driven runs, real-time summaries, and log history retention are all first-class.
- **Auditability**: Every run stores its logs, history, and metadata in Postgres, giving operators a replayable record aligned with GitHub check results.
