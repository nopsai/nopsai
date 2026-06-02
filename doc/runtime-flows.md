# NopsAI Runtime Flows

This document explains how the tool works step by step.

## Request Authentication And Authorization

Most API routes pass through the same middleware stack before reaching a handler.

1. Public paths such as `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout`, and `/v1/git/events` skip bearer-token authentication.
2. Other requests must include a bearer token produced by the local auth service or a user-created personal access token.
3. `nopsai` validates JWT bearer tokens by signature, or hashes opaque `nopat_` personal tokens and checks `personal_access_tokens`.
4. Session JWTs enforce idle-session timeout when configured; personal tokens rely on expiry when configured and revocation. Valid credentials place claims in the request context.
5. Authenticated-only profile routes (`/v1/auth/me`, `/v1/auth/password`, `/v1/auth/email`, `/v1/auth/personal-tokens`) stop here.
6. Other protected routes are mapped by `routeauthz.MapRequest` to an action/resource pair.
7. `nopsai` calls the AAA service for a `Check`, or defers to handler-level `Filter` for list endpoints.
8. If the AAA service is unavailable, `nopsai` temporarily falls back to an in-process evaluator backed by the same Postgres tables.
9. Denied decisions return `403`; denied decisions and sensitive allowed decisions are written to `authz_decision_logs`.

## 1. GitHub Webhook To Pipeline Run

1. GitHub sends a webhook to `git-bot` at `/webhook`.
2. `git-bot` reads the raw body and validates the HMAC signature with `verifySignature`.
3. If valid, `git-bot` forwards the same payload and selected GitHub headers to `nopsai` at `/v1/git/events`.
4. `nopsai` parses the webhook, extracts repository, ref, commit SHA, pusher, PR info, check-run info, and delivery ID.
5. `nopsai` loads the trigger manifest, first checking DB overrides and then falling back to `.nopsai/triggers.yaml` through `git-bot`.
6. It matches the event against trigger rules, branches, tags, skipped branches, and skipped repositories.
7. For each matched pipeline source, `nopsai` treats the repository as the caller, for example `repository:hosein-yousefii/test-app`.
8. Before loading the pipeline definition, it checks `pipeline.use` for that repository against the matched pipeline resource.
9. If the repository is not allowed to use the pipeline, `nopsai` does not fetch the pipeline and does not create a real run; it creates or updates the GitHub check with a clear failure message and audits the denial.
10. If the pipeline is allowed, `nopsai` loads or fetches the definition, checks the selected scope with `scope.use`, validates referenced reusable steps or child pipelines, and checks managed knowledge context references with the original repository identity.
11. `nopsai` creates a `pipeline_runs` record, stores git context and the authorization snapshot with it, and asynchronously creates or initializes the GitHub check run.
12. It resolves required knowledge context, stores run snapshots, and inserts `task_runs` rows so every task has durable tracking before execution starts.
13. `nopsai` prepares the agent job and submits it to the dispatcher.

## 2. Manual API Run

1. A user or UI calls `POST /v1/run` or `POST /v1/run/{pipeline}`.
2. From the UI, the Pipeline detail Execute action redirects to Lab with the selected pipeline preloaded for review and execution.
3. `nopsai` authorizes the user for `pipeline.execute`, then accepts either a pipeline identifier, raw YAML, or a JSON payload with `pipeline`, `definition`, `scope`, and variable overrides.
4. The user remains the caller for runtime resource-use checks.
5. The pipeline is parsed and normalized.
6. `nopsai` checks `pipeline.use`, selected `scope.use`, reusable `step.use`, child `pipeline.use`, managed `knowledge_context.use`, and other referenced runtime resources with the user identity.
7. `nopsai` creates the initial `pipeline_runs` record in `pending` and stores the authorization snapshot.
8. `step:` includes are expanded from the reusable `steps` table.
9. The pipeline is validated and task rows are created.
10. Knowledge context, secrets, and variables are resolved and snapshotted where applicable.
11. The run is submitted to the dispatcher the same way a GitHub-triggered run is.

## 3. Dispatch And Runner Selection

1. `nopsai` calls `dispatcher.SubmitJob` with a `JobRequest`.
2. The dispatcher first checks the latest run status and rejects jobs for terminal runs.
3. The dispatcher tries to pick a runner with `pickRunnerForJobLocked`.
4. It filters runners by:
   - `allow_dispatch`
   - declared scope support
   - optional `dispatcher_routing`
5. It prefers `preferred_runner_id` and affinity using `runner_affinity_key`, usually derived from `trigger_event_id`, parent run, or run ID.
6. If the preferred or affinity runner is unavailable or full, it falls back to another eligible runner.
7. Among eligible runners, it chooses the least-loaded runner.
8. If no eligible runner is available, the job stays queued.
9. If a runner disconnects with inflight jobs, those jobs are requeued.

## 4. Runner Launch Flow

1. A Docker runner or Kubernetes runner stays connected to the dispatcher over a long-lived gRPC stream.
2. When a job arrives, the runner acknowledges it and increments its active-job count.
3. A Docker runner ensures the requested agent image exists locally, pulling it if needed.
4. A Docker runner creates a shared Docker volume for the run, normally `vol-<run_id>`.
5. A Kubernetes runner starts an agent pod with a pod-owned workspace volume in its namespace.
6. A Docker runner starts an agent container with:
   - the shared workspace mounted at `/workspace`
   - Docker socket access
   - the full runtime variable payload from `nopsai`
7. A Kubernetes runner starts an agent pod with:
   - `NOPSAI_RUNTIME=kubernetes`
   - the shared workspace mounted at `/workspace`
   - Kubernetes namespace, service account, storage, affinity, and runtime pool settings
8. The runner starts two background loops:
   - log streaming back through `dispatcher.IngestLogs`
   - run-cancellation polling through `dispatcher.GetRunStatus`
9. When the agent exits, the runner reports `completed` or `failed` back to the dispatcher.

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
   - resolved knowledge context snapshot
2. It connects to the dispatcher over gRPC.
3. It parses the pipeline definition from base64-encoded YAML.
4. It initializes the embedded `LLMClient` for Gemini or LM Studio.
5. It creates a Docker client for Docker runtime, or a Kubernetes client for Kubernetes runtime.
6. It starts background cancellation and signal handlers.
7. Docker runtime optionally starts asynchronous image pre-pulling for pipeline step images.
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
5. If the step has a `condition`, the agent asks the LLM for a boolean answer with the step's effective knowledge context before doing any work in that step.
6. If the condition is false, all tasks in that step are marked `skipped`, except false conditions under an effective guardrail or policy context fail the current task.
7. If the step is an included child pipeline, the agent fetches the child pipeline and triggers it through the dispatcher.
8. If the step is a normal execution step, the agent creates or reuses one step container or step pod for that step.
9. It picks the execution image from `step.image` or the pipeline default `container_image`.
10. Kubernetes runtime resolves `step.runtime_pool` or the pipeline default `runtime_pool` and applies the matching runtime pool to the step pod. Docker runtime ignores this directive.
11. Kubernetes runtime resolves the pipeline-level `affinity_enabled` directive, falling back to the runner default, and uses it to decide whether step pods must stay on the agent pod's node. Docker runtime ignores this directive.
12. Docker runtime mounts the shared run volume at the pipeline `working_directory` plus any declared named volumes. Kubernetes runtime mounts the agent-owned workspace PVC at the step pod's pipeline `working_directory` and maps declared volumes to PVCs in the runner namespace.
13. It decides the action:
   - `script` task: execute the script directly
   - `goal` task: ask the LLM to return a structured action
14. For goal tasks, the LLM prompt includes variables, effective knowledge context, optional workspace contents, MCP tools, execution history, and the current goal.
15. If LLM content sharing is enabled, it scans the workspace and includes file contents in the prompt, excluding ignored paths.
16. It executes the chosen action inside the step container or pod.
17. It masks secret values from output before logging or saving history.
18. It updates task status through the dispatcher.
19. It appends a normalized history entry that later tasks and child pipelines can use.
20. If a task fails and `ignore_failure` is false, the pipeline stops with failure.
21. If a task fails and `ignore_failure` is true, the task becomes `failure (ignored)` and the pipeline continues.

## 7. How Goal-Based Tasks Work

For a goal-driven task:

1. The agent builds an LLM prompt from variables, effective knowledge context, optional directory contents, MCP tools, execution history, and the current goal.
2. The LLM must return one structured action:
   - `EXECUTE_COMMAND`
   - `REPLACE_FILE`
   - `RETURN_ANSWER`
3. The agent executes that action in the step container. If a guardrail or policy caused a blocking `RETURN_ANSWER`, the agent records that explanation as a task failure.
4. The command output becomes part of the run history unless output sharing is disabled.
5. That history can influence later tasks or child pipelines.

## 8. How Included Pipelines Work

For a `pipeline:<identifier>` include:

1. The agent fetches the child pipeline definition through the dispatcher.
2. The dispatcher fetches it from `nopsai`, which may read it from DB or fetch it from Git after `nopsai` checks `pipeline.use` with the original run caller.
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
5. `nopsai` creates a child `pipeline_runs` record with parent metadata and the original authorization context; child pipelines never gain permissions from the parent pipeline owner, child pipeline owner, or dispatcher identity.
6. The child run is dispatched like any other run.
7. If the include is `sync: true`, the parent agent waits for the child result.
8. If the include is `sync: false`, the parent treats it as unblocking and continues.

## 9. How Reusable Steps Work

For a `step:<identifier>` include:

1. `nopsai` resolves it before dispatch, not during agent execution.
2. It loads the reusable step YAML from the `steps` table.
3. It checks `step.use` with the original run caller before replacing the placeholder.
4. It replaces the placeholder with the stored step definition.
5. It preserves the calling step's name and selected metadata like dependencies, volumes, secrets, variables, and failure flags.
6. The agent only sees the fully expanded pipeline.

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
3. Manual reruns require the current user to have rerun permission and re-check runtime resources with that user; GitHub check reruns use the original repository identity and re-check current grants.
4. It creates a fresh run record with a new trigger event ID and authorization snapshot.
5. It resolves includes again, validates again, and dispatches again.

## 12. Config Sync From Git

1. A group owner calls `POST /v1/groups/{groupPath}/config-repo/sync`, or an admin calls `POST /v1/system/config-repos/sync`.
2. `nopsai` loads the scoped config repository binding and validates group ownership for group-scoped sync.
3. It asks `git-bot` to verify repository access.
4. It fetches directories from the config repo under the binding base path:
   - `pipelines/`
   - `steps/`
   - `triggers/`
   - `scopes/`
   - `knowledge/`
   - `pipelineruns/`
   - `config-repositories/`
   - `setting/` and `settings/`
5. It parses and validates each file class:
   - pipelines must parse and pass pipeline validation
   - reusable steps must parse and have matching names
   - triggers must parse as manifests
   - scope files turn `variables:` entries into scoped variables and `secrets:`
     entries into GitOps secret keys
   - knowledge markdown files are turned into `knowledge_contexts`
   - legacy `pipelineruns/structure.yaml` becomes the run-group tree for groups owned by that repo
   - `config-repositories/groups/<group>.yaml` becomes a group config repo binding and group shell
   - `config-repositories/groups/structure.yaml` and `config-repositories/groups/<group>/structure.yaml` place apps under group shells with `name` and `repo_url`, keep legacy `repos:` lists readable, and can define inline group repo `config:` blocks
   - `setting/system/llm_profile.yaml` becomes the system LLM profile registry, only from a system/global config repo
   - `setting/system/mcp.yaml` becomes the system MCP server/profile registry, only from a system/global config repo
   - `setting/system/runner.yaml` becomes runner install defaults, runtime URLs, and dispatcher routing, only from a system/global config repo
6. System/global repositories are synced before group repositories during sync-all, so newly defined group bindings can be used immediately.
7. Group-scoped resources are normalized under the bound group before writing.
8. It refuses to overwrite resources that are unmanaged or already managed by an unrelated config repository; delegated child group repos can override parent-managed resources in their group.
9. It upserts rows with config-source metadata into Postgres.

For Git push, `nopsai` loads the same system or group config repository binding,
validates that `write_enabled` and `write_branch` are set, prefixes requested
file paths with the binding `base_path`, and asks `git-bot` to commit those
files to the review branch. The sync branch is not updated directly. The drift
endpoint exports the current Nopsai config and compares it with the sync branch
so the UI can show the exact file changes before pushing.
Runtime settings sync persists supported operational defaults back to the local
runtime config files. `dispatcher_routing` is exposed through an authenticated
internal NopsAI endpoint; the dispatcher polls that endpoint and swaps its
in-memory routing table while it is running, so new scheduling decisions use the
updated table without a dispatcher restart.
10. It prunes rows managed by the same config repository that disappeared from the repo.
11. Legacy global `pipelineruns/structure.yaml` is ignored for groups delegated via `config-repositories/groups`; colocated group structure under `config-repositories/groups` is still applied.
12. It does not prune user-created groups, even when syncing the run-group structure.
13. It records sync status per config repository for the UI.

## 13. Failure Boundaries

Where failures stop the flow:

- Missing or invalid bearer token: stopped in `nopsai`
- AAA denial: stopped in `nopsai` with `403`
- Resource-use denial: stopped in `nopsai` before execution; GitHub-triggered denials create a failed check instead of a real run
- Invalid webhook signature: stopped at `git-bot`
- Trigger mismatch: stopped in `nopsai`
- Invalid pipeline YAML or validation failure: stopped in `nopsai`
- Missing scoped secret or variable: stopped in `nopsai`
- Missing required knowledge context, denied `knowledge_context.use`, or unreadable required repo-local knowledge: stopped in `nopsai`
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
