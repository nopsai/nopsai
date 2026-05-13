# NopsAI Runtime Flows

This document explains how the tool works step by step.

## Request Authentication And Authorization

Most API routes pass through the same middleware stack before reaching a handler.

1. Public paths such as `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout`, and `/v1/git/events` skip bearer-token authentication.
2. Other requests must include a bearer token produced by the local auth service.
3. `nopsai` validates the token, enforces idle-session timeout when configured, and places claims in the request context.
4. Authenticated-only profile routes (`/v1/auth/me`, `/v1/auth/password`, `/v1/auth/email`) stop here.
5. Other protected routes are mapped by `routeauthz.MapRequest` to an action/resource pair.
6. `nopsai` calls the AAA service for a `Check`, or defers to handler-level `Filter` for list endpoints.
7. If the AAA service is unavailable, `nopsai` temporarily falls back to an in-process evaluator backed by the same Postgres tables.
8. Denied decisions return `403`; denied decisions and sensitive allowed decisions are written to `authz_decision_logs`.

## 1. GitHub Webhook To Pipeline Run

1. GitHub sends a webhook to `git-bot` at `/webhook`.
2. `git-bot` reads the raw body and validates the HMAC signature with `verifySignature`.
3. If valid, `git-bot` forwards the same payload and selected GitHub headers to `nopsai` at `/v1/git/events`.
4. `nopsai` parses the webhook, extracts repository, ref, commit SHA, pusher, PR info, check-run info, and delivery ID.
5. `nopsai` loads the trigger manifest, first checking DB overrides and then falling back to `.nopsai/triggers.yaml` through `git-bot`.
6. It matches the event against trigger rules, branches, tags, skipped branches, and skipped repositories.
7. For each matched pipeline source, `nopsai` loads or fetches the pipeline definition, creates a `pipeline_runs` record, and stores git context with it.
8. `nopsai` resolves reusable `step:` includes, validates the pipeline, and asynchronously creates or initializes the GitHub check run.
9. `nopsai` inserts `task_runs` rows so every task has durable tracking before execution starts.
10. `nopsai` prepares the agent job and submits it to the dispatcher.

## 2. Manual API Run

1. A user or UI calls `POST /v1/run` or `POST /v1/run/{pipeline}`.
2. `nopsai` authorizes the request for `pipeline.execute`, then accepts either a pipeline identifier, raw YAML, or a JSON payload with `pipeline`, `definition`, `scope`, and variable overrides.
3. The pipeline is parsed and normalized.
4. `nopsai` creates the initial `pipeline_runs` record in `pending`.
5. `step:` includes are expanded from the reusable `steps` table.
6. The pipeline is validated and task rows are created.
7. Secrets and variables are resolved.
8. The run is submitted to the dispatcher the same way a GitHub-triggered run is.

## 3. Dispatch And Runner Selection

1. `nopsai` calls `dispatcher.SubmitJob` with a `JobRequest`.
2. The dispatcher first checks the latest run status and rejects jobs for terminal runs.
3. The dispatcher tries to pick a runner with `pickRunnerForJobLocked`.
4. It filters runners by:
   - `allow_dispatch`
   - declared scope support
   - optional `dispatcher_routing`
   - optional `preferred_runner_id`
5. It preserves affinity using `runner_affinity_key`, usually derived from `trigger_event_id`, parent run, or run ID.
6. Among eligible runners, it chooses the least-loaded runner.
7. If no eligible runner is available, the job stays queued.
8. If a runner disconnects with inflight jobs, those jobs are requeued.

## 4. Runner Launch Flow

1. The runner stays connected to the dispatcher over a long-lived gRPC stream.
2. When a job arrives, the runner acknowledges it and increments its active-job count.
3. It ensures the requested agent image exists locally, pulling it if needed.
4. It creates a shared Docker volume for the run, normally `vol-<run_id>`.
5. It starts an agent container with:
   - the shared workspace mounted at `/workspace`
   - Docker socket access
   - the full runtime variable payload from `nopsai`
6. It starts two background loops:
   - log streaming back through `dispatcher.IngestLogs`
   - run-cancellation polling through `dispatcher.GetRunStatus`
7. When the agent exits, the runner reports `completed` or `failed` back to the dispatcher.

## 5. Agent Startup And Preparation

1. The agent reads its runtime variables, including:
   - run ID
   - pipeline definition
   - secrets payload
   - variables payload
   - git context
   - timeout settings
   - dispatcher address
   - LLM provider configuration
2. It connects to the dispatcher over gRPC.
3. It parses the pipeline definition from base64-encoded YAML.
4. It initializes the embedded `LLMClient` for Gemini or LM Studio.
5. It creates a Docker client.
6. It starts background cancellation and signal handlers.
7. It optionally starts asynchronous image pre-pulling for pipeline step images.
8. It initializes execution history, including inherited parent history for child pipelines.

## 6. Agent Execution Loop

The agent runs tasks in dependency order, not strictly line order.

1. It counts the total number of runnable tasks in the pipeline.
2. It repeatedly asks `getNextRunnableTasks` for all currently unblocked tasks.
3. Tasks from independent steps can run in parallel.
4. For each runnable task, the agent builds a step execution context from:
   - inherited runtime variables
   - resolved variables
   - resolved secrets
   - step-level overrides
   - task-level overrides
5. If the step has a `condition`, the agent asks the LLM for a boolean answer before doing any work in that step.
6. If the condition is false, all tasks in that step are marked `skipped`.
7. If the step is an included child pipeline, the agent fetches the child pipeline and triggers it through the dispatcher.
8. If the step is a normal execution step, the agent creates or reuses one step container for that step.
9. It picks the execution image from `step.image` or the pipeline default `container_image`.
10. It mounts the shared run volume plus any declared named volumes.
11. It decides the action:
   - `script` task: execute the script directly
   - `goal` task: ask the LLM to return a structured action
12. If LLM content sharing is enabled, it scans the workspace and includes file contents in the prompt, excluding ignored paths.
13. It executes the chosen action inside the step container.
14. It masks secret values from output before logging or saving history.
15. It updates task status through the dispatcher.
16. It appends a normalized history entry that later tasks and child pipelines can use.
17. If a task fails and `ignore_failure` is false, the pipeline stops with failure.
18. If a task fails and `ignore_failure` is true, the task becomes `failure (ignored)` and the pipeline continues.

## 7. How Goal-Based Tasks Work

For a goal-driven task:

1. The agent builds an LLM prompt from variables, optional directory contents, execution history, and the current goal.
2. The LLM must return one structured action:
   - `EXECUTE_COMMAND`
   - `REPLACE_FILE`
   - `RETURN_ANSWER`
3. The agent executes that action in the step container.
4. The command output becomes part of the run history unless output sharing is disabled.
5. That history can influence later tasks or child pipelines.

## 8. How Included Pipelines Work

For a `pipeline:<identifier>` include:

1. The agent fetches the child pipeline definition through the dispatcher.
2. The dispatcher fetches it from `nopsai`, which may read it from DB or fetch it from Git.
3. The agent sends `TriggerPipeline` to the dispatcher with:
   - parent run ID
   - parent step name
   - parent pipeline name
   - pipeline definition
   - current history snapshot
   - scope
   - git context
   - preferred runner ID
4. The dispatcher turns that into an internal `POST /v1/run` call to `nopsai`.
5. `nopsai` creates a child `pipeline_runs` record with parent metadata.
6. The child run is dispatched like any other run.
7. If the include is `sync: true`, the parent agent waits for the child result.
8. If the include is `sync: false`, the parent treats it as unblocking and continues.

## 9. How Reusable Steps Work

For a `step:<identifier>` include:

1. `nopsai` resolves it before dispatch, not during agent execution.
2. It loads the reusable step YAML from the `steps` table.
3. It replaces the placeholder with the stored step definition.
4. It preserves the calling step’s name and selected metadata like dependencies, volumes, secrets, variables, and failure flags.
5. The agent only sees the fully expanded pipeline.

## 10. Logs, Status, And GitHub Feedback

1. The runner reads agent container logs and batches them.
2. Logs go to `dispatcher.IngestLogs`.
3. The dispatcher makes an authenticated internal call to `nopsai` at `/v1/runs/{runID}/logs/ingest`.
4. The agent reports task status to `dispatcher.ReportTaskStatus`.
5. The dispatcher forwards that to `nopsai` at `/v1/runs/{runID}/steps/{step}/tasks/{task}`.
6. `nopsai` persists the update and asynchronously tells `git-bot` about the task status.
7. `git-bot` updates its in-memory check-run state and renders the GitHub check output.
8. When the agent finishes, it calls `dispatcher.FinalizeRun`.
9. The dispatcher forwards that to `nopsai`, which finalizes the run and notifies `git-bot` of the final result.
10. The UI refreshes run lists and details over REST polling, and log modals poll `/v1/runs/{runID}/logs?since_line=<id>` for incremental log lines.

## 11. Cancellation And Reruns

Cancellation:

1. A user calls `POST /v1/runs/{runID}/cancel`.
2. `nopsai` marks the run as cancelled and recursively cancels child runs.
3. The runner and agent both poll run status through the dispatcher.
4. Once they see `cancelled`, they stop the active container and mark active tasks cancelled where possible.

Rerun:

1. A user calls `POST /v1/runs/{runID}/rerun`.
2. `nopsai` loads the original stored pipeline definition and git context.
3. It creates a fresh run record with a new trigger event ID.
4. It resolves includes again, validates again, and dispatches again.

## 12. Config Sync From Git

1. A group owner calls `POST /v1/groups/{groupPath}/config-repo/sync`, or an admin calls `POST /v1/system/config-repos/sync`.
2. `nopsai` loads the scoped config repository binding and validates group ownership for group-scoped sync.
3. It asks `git-bot` to verify repository access.
4. It fetches directories from the config repo under the binding base path:
   - `pipelines/`
   - `steps/`
   - `triggers/`
   - `scopes/`
   - `pipelineruns/`
   - `config-repositories/`
5. It parses and validates each file class:
   - pipelines must parse and pass pipeline validation
   - reusable steps must parse and have matching names
   - triggers must parse as manifests
   - scope files are turned into scoped variables
   - legacy `pipelineruns/structure.yaml` becomes the run-group tree for groups owned by that repo
   - `config-repositories/groups/<group>.yaml` becomes a group config repo binding and group shell
   - `config-repositories/groups/structure.yaml` and `config-repositories/groups/<group>/structure.yaml` place repositories under group shells and can define inline group repo `config:` blocks
6. System/global repositories are synced before group repositories during sync-all, so newly defined group bindings can be used immediately.
7. Group-scoped resources are normalized under the bound group before writing.
8. It refuses to overwrite resources that are unmanaged or already managed by an unrelated config repository; delegated child group repos can override parent-managed resources in their group.
9. It upserts rows with config-source metadata into Postgres.
10. It prunes rows managed by the same config repository that disappeared from the repo.
11. Legacy global `pipelineruns/structure.yaml` is ignored for groups delegated via `config-repositories/groups`; colocated group structure under `config-repositories/groups` is still applied.
12. It does not prune user-created groups, even when syncing the run-group structure.
12. It records sync status per config repository for the UI.

## 13. Failure Boundaries

Where failures stop the flow:

- Missing or invalid bearer token: stopped in `nopsai`
- AAA denial: stopped in `nopsai` with `403`
- Invalid webhook signature: stopped at `git-bot`
- Trigger mismatch: stopped in `nopsai`
- Invalid pipeline YAML or validation failure: stopped in `nopsai`
- Missing scoped secret or variable: stopped in `nopsai`
- No available runner: run stays queued in dispatcher
- Agent image or step image pull failure: stopped in runner/agent
- Task failure without `ignore_failure`: stopped in agent
- Child pipeline sync failure: stops the parent only when `sync: true`

## 14. Short Mental Model

The simplest way to think about the tool is:

- `git-bot` knows GitHub
- `nopsai` knows configuration and state
- `aaa` knows authorization decisions
- `dispatcher` knows scheduling
- `runner` knows Docker hosts
- `agent` knows pipeline execution and LLM-assisted decisions
