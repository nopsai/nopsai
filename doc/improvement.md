## Platform Improvements Overview

This document captures the current feature set and hardening work that has landed across the Nopsai stack. Use it as a quick orientation guide when exploring the services, deployment story, and day-to-day operations.

### Core Orchestrator (`services/nopsai`)
- **Real-time hub**: A dedicated WebSocket hub manages per-run subscriptions and global broadcasts (including the `new_run_started` toast feed) so the UI stays in sync without polling.
- **Agent lifecycle management**: Launches agent containers on demand, wires them into the shared Docker network, mounts a per-run workspace volume, and tails container logs back into PostgreSQL and WebSocket streams.
- **Secrets & environment resolution**: Derives scoped secrets and environment variables (global, environment-specific, repo-specific) and encrypts secrets at rest with the `NOPSAI_MASTER_KEY` before persisting them.
- **Pipeline & trigger APIs**: Exposes CRUD endpoints for pipelines, reusable steps, trigger overrides, run groups, run logs, reruns, cancellations, and branch-level clean-up.
- **Git-driven orchestration**: Receives Git events, ensures the config repository is synced (pipelines, steps, environments, triggers), and coordinates with the git-bot for GitHub check updates.

### Agent (`services/agent`)
- **LLM-assisted execution**: Resolves natural-language goals via the LLM agent, while falling back to explicit scripts when provided.
- **Task graph engine**: Understands nested step/task dependencies, honours per-task `depends_on`, and tracks partial progress so multiple independent tasks can run without blocking each other.
- **Workspace sharing controls**: Shares a sanitised directory listing with the LLM, respecting `llm_content_ignore`, and honours both pipeline-level and task-level `llm_output_sharing` settings.
- **Container orchestration**: Ensures required images exist, reuses warm containers per step, supports additional volume mounts, and injects scoped secrets/environment variables safely.
- **Operational telemetry**: Masks secrets in command output, streams logs back to the server in real time, and reports granular task status (with exit codes and LLM latency) to the API.

### LLM Agent (`services/llm-agent`)
- **Gemini integration**: Translates execution context into structured prompts for Google Gemini, with separate flows for action generation and boolean condition evaluation.
- **Structured actions**: Returns strongly-typed actions (`EXECUTE_COMMAND`, `REPLACE_FILE`, `RETURN_ANSWER`) that the agent can execute without further parsing.
- **Config-driven runtime**: Loads service addresses, model configuration, and API keys from the shared config loader, keeping deployment knobs consistent.

### Git Bot (`services/git-bot`)
- **Secure webhooks**: Verifies GitHub signatures, normalises incoming events, and forwards run intents to the core service.
- **Rich check runs**: Maintains hierarchical check-run state (steps ↦ tasks), renders markdown trees, and updates GitHub with real-time summaries, rerun links, and failure context.
- **Developer utilities**: Provides endpoints for repo access checks, file/directory content resolution, pipeline source retrieval, child check-run creation, and stale check-run cancellation.

### Web UI (`services/ui`)
- **Live dashboards**: Uses a resilient WebSocket client that auto-reconnects, resubscribes to open runs, and pushes toast notifications whenever new runs start.
- **Run explorer**: Offers branch-aware run cards, group/folder organisation, inline branch clean-up, selection-driven bulk actions, and deep links that preserve UI state in the hash.
- **Pipeline graph**: Visualises pipeline steps and intra-step tasks with pan/zoom support, expandable nodes, and live status colouring.
- **Log experience**: Ships an on-demand log modal with level filters, agent-only toggle, structured/short views, search with navigation, clipboard/download helpers, and follow mode—all powered by WebSocket streaming.

### Configuration & Deployment
- **Single compose stack**: Docker Compose provisions Postgres, the Go services, the UI (served via nginx), plus build-only helper images (`base`, `agent`, `pipeline`).
- **Shared network & volumes**: All services join `nopsai-net`; run workspaces use throwaway Docker volumes, and Postgres writes to a named volume for persistence.
- **Unified config loader**: `config/config.go` reads YAML defaults and lets environment variables override per deployment, covering ports, service URLs, Gemini credentials, Docker preferences, and timeout knobs.

### Feature Highlights
- **Pipeline DSL**: Supports step includes, per-step containers, secret injection, volume mounts, multi-task steps, AI gating via `condition`, and fine-grained LLM sharing controls.
- **Trigger routing**: `.nopsai/triggers.yaml` can target events by branch/tag globs, apply scope overrides, and fan out to multiple pipelines.
- **Run lifecycle**: Manual kicks, reruns, cancellation, GitHub-driven runs, real-time summaries, and log history retention are all first-class.
- **Auditability**: Every run stores its logs, history, and metadata in Postgres, giving operators a replayable record aligned with GitHub check results.
