# NopsAI Runtime Flows

This document explains how the tool works step by step.

## Request Authentication And Authorization

Most API routes pass through the same middleware stack before reaching a handler.

1. Public paths such as `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout`, `/v1/git/events`, and `/v1/git/webhooks/{sourceID}` skip bearer-token authentication.
2. Other requests must include a bearer token produced by the local auth service, service-auth credentials, or a user-created personal access token.
3. `nopsai` validates service tokens first, then user/API JWTs with signature, expiration, issuer, and audience checks, or hashes opaque `nopat_` personal tokens and `nopsat_` service account tokens against their token tables.
4. Session JWTs enforce idle-session timeout when configured; service tokens, personal tokens, and service account tokens rely on expiry and revocation semantics. Valid credentials place claims in the request context.
5. Authenticated-only profile routes (`/v1/auth/me`, `/v1/auth/password`, `/v1/auth/email`, `/v1/auth/personal-tokens`, `/v1/assistant/llm-profiles`) stop here.
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
6. It matches the event against trigger rules, branches, tags, skipped branches, skipped repositories, and changed-file include/exclude filters. If changed files are unavailable, path matching fails open.
7. For each matched pipeline source, `nopsai` treats the repository as the caller, for example `repository:hosein-yousefii/test-app`.
8. Before loading the pipeline definition, it checks `pipeline.use` for that repository against the matched pipeline resource.
9. If the repository is not allowed to use the pipeline, `nopsai` does not fetch the pipeline and does not create a real run; it creates or updates the GitHub check with a clear failure message and audits the denial.
10. If the pipeline is allowed, `nopsai` loads or fetches the definition, checks the selected scope with `scope.use`, validates referenced reusable steps or child pipelines, and checks managed knowledge context references with the original repository identity.
11. `nopsai` creates a `pipeline_runs` record, stores git context and the authorization snapshot with it, and asynchronously creates or initializes the GitHub check run.
12. It resolves required knowledge context, stores run snapshots, and inserts `task_runs` rows so every task has durable tracking before execution starts.
13. `nopsai` prepares the agent job and submits it to the dispatcher.

## Git Webhook Source To Pipeline Run

1. GitLab, Bitbucket, Gitea, or a generic sender posts the raw request body to
   `/v1/git/webhooks/{sourceID}`.
2. `nopsai` loads the enabled source and resolves its credential reference.
3. It validates the provider token or HMAC signature before normalizing JSON.
4. The adapter normalizes repository, event type, ref, target ref, commit,
   changed files, actor, URLs, and delivery ID.
5. The repository must match the source allowlist.
6. `nopsai` inserts an idempotent delivery audit record and evaluates the source
   rate limit.
7. It loads the database trigger override for the repository. Generic sources
   do not fetch `.nopsai/triggers.yaml` from the provider in v1.
8. The provider-neutral matcher evaluates event, branch/tag, skip, and path
   rules.
9. Each referenced pipeline must already exist in the database, normally from
   `pipelines/` GitOps sync.
10. The normal run handler starts each pipeline with the repository as caller,
    preserving runtime `*.use` authorization.
11. Runs record `pipeline_source=git_webhook` and
    `trigger_source=git_webhook_<provider>`. GitHub checks are not created.
12. The delivery is finalized with status, matched pipelines, run IDs, and
    errors. A repeated source/delivery ID is acknowledged without another run.

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

## External Trigger Invoke

External triggers can be created through the API/UI or managed by GitOps in
`external-triggers/*.yaml`. Config sync imports those manifests; config
repository drift/push exports database-created triggers back to the same
directory.

1. An external system calls `POST /v1/external-triggers/{id}/invoke` with a user bearer token or service-account token.
2. The normal authentication middleware validates the token. Service-account tokens authorize as AAA `service_account` subjects.
3. `nopsai` loads the external trigger, creates an invocation record, and rejects disabled triggers.
4. The caller must match the trigger's `allowed_callers` list and pass `external_trigger.invoke` on that trigger resource, for example `external_trigger:deploy-prod`. The list supports direct users, service accounts, and auth groups.
5. If an idempotency key is present, `nopsai` checks prior active invocations for the same trigger and caller. A queued duplicate returns the original run response; an in-flight duplicate returns `409`. Failed pre-run attempts remain in the audit log but do not reserve the key forever.
6. Rate limits and the trigger payload schema are evaluated before a run is started.
7. Request variables are merged with `variable_mapping` values derived from `payload`, `variables`, or `event_type`.
8. `nopsai` starts the configured pipeline through the existing JSON run path with `trigger_source=external_trigger`.
9. The existing run path checks `pipeline.execute`, `pipeline.use`, `scope.use`, reusable `step.use`, child `pipeline.use`, managed `knowledge_context.use`, and runtime resources with the original caller identity.
10. The `pipeline_runs.trigger_event_id` is set to the invocation id, so the run detail can show the trigger name, caller, event type, and idempotency key.
11. The invocation record is updated with the queued run id or a failure status and error text.

## 3. Dispatch And Runner Selection

1. `nopsai` calls `dispatcher.SubmitJob` with a `JobRequest`.
2. The dispatcher first checks the latest run status and only dispatches `pending` or `running` runs. Paused approval runs (`waiting_approval`), rejected runs, terminal runs, and unknown statuses are not dispatchable.
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
   - optional approval checkpoint ID for resumed runs
2. It connects to the dispatcher over gRPC.
3. It parses the pipeline definition from base64-encoded YAML.
4. If `RESUME_CHECKPOINT_ID` is present, it fetches the checkpoint from `nopsai`, restores the compressed workspace archive into `/workspace`, loads the saved execution history, and marks already completed task keys in memory.
5. It initializes the embedded `LLMClient` and selected provider adapter:
   Gemini, LM Studio, OpenAI-compatible, Anthropic, or Azure OpenAI.
6. It creates a Docker client for Docker runtime, or a Kubernetes client for Kubernetes runtime.
7. It starts background cancellation and signal handlers.
8. Docker runtime optionally starts asynchronous image pre-pulling for pipeline step images.
9. It initializes execution history, including inherited parent history for child pipelines when the run is not resuming from an approval checkpoint.

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
7. If the step is an approval step, the agent treats it as a scheduling barrier, captures execution history, completed task keys, variables, pipeline definition, and a compressed workspace archive, then calls `nopsai` with an `agent` service token to create a pending approval and checkpoint.
8. After the approval checkpoint is stored, `nopsai` marks the task and run `waiting_approval`, and the agent exits without finalizing the run. No runner stays occupied while the run waits.
9. When an authorized user approves the pending approval, `nopsai` records the decision, marks the approval task successful, moves the run back to `pending`, and launches a fresh agent with the checkpoint ID. Each dispatch uses a unique agent container name so resumed runs do not collide with stopped containers retained for debugging.
10. If the user rejects the approval, `nopsai` records the decision, marks the approval task failed, marks the run `rejected`, and sends final failure feedback.
11. If the step is an included child pipeline, the agent fetches the child pipeline and triggers it through the dispatcher.
12. If the step is a normal execution step, the agent creates or reuses one step container or step pod for that step.
13. It picks the execution image from `step.image` or the pipeline default `container_image`.
14. Kubernetes runtime resolves `step.runtime_pool` or the pipeline default `runtime_pool` and applies the matching runtime pool to the step pod. Docker runtime ignores this directive.
15. Kubernetes runtime resolves the pipeline-level `affinity_enabled` directive, falling back to the runner default, and uses it to decide whether step pods must stay on the agent pod's node. Docker runtime ignores this directive.
16. Docker runtime mounts the shared run volume at the pipeline `working_directory` plus any declared named volumes. Kubernetes runtime mounts the agent-owned workspace PVC at the step pod's pipeline `working_directory` and maps declared volumes to PVCs in the runner namespace.
17. It decides the action:
   - `script` task: execute the script directly, unless effective guardrail or
     policy context exists; in that case the LLM validates the exact script
     before execution and the task fails closed if validation is unavailable or
     returns a conflict
   - `goal` task: ask the LLM to return a structured action
18. For goal tasks, the LLM prompt includes the resolved Agent Profile role/instructions, variables, effective knowledge context, optional workspace contents, MCP tools, execution history, and the current goal.
19. If LLM content sharing is enabled, it scans the workspace and includes file contents in the prompt, excluding ignored paths.
20. It executes the chosen action inside the step container or pod.
21. It masks known secret values and NopsAI-provided runtime variable values
    from action summaries and output before logging or saving history.
22. It updates task status through the dispatcher.
23. It appends a normalized history entry that later tasks and child pipelines can use.
24. If a task fails and `ignore_failure` is false, the pipeline stops with failure.
25. If a task fails and `ignore_failure` is true, the task becomes `failure (ignored)` and the pipeline continues.
26. When a run finalizes as failed, task rows that never started are closed as `skipped`; started task rows without a terminal update are closed as `failure` with a finish timestamp so run graphs show bounded step time instead of an open-ended pipeline age.

## 7. How Goal-Based Tasks Work

For a goal-driven task:

1. The agent resolves `agent_profile` from step, pipeline, then the configured system default, and builds an LLM prompt from that profile persona/instructions, variables, effective knowledge context, optional directory contents, MCP tools, execution history, and the current goal.
2. The LLM must return one structured action:
   - `EXECUTE_COMMAND`
   - `REPLACE_FILE`
   - `RETURN_ANSWER`
3. If effective guardrail or policy context exists, the prompt instructs the LLM
   to inspect the exact structured action before returning it. For
   `EXECUTE_COMMAND`, that includes checking the generated command text,
   scripts, arguments, and any stdout/stderr-producing operation against the
   guardrail or policy. Generated file writes, MCP/tool actions, and tool
   arguments are also covered by this prompt-level check.
4. The agent executes the returned action in the step container. If a guardrail
   or policy caused a blocking `RETURN_ANSWER`, the agent records that
   explanation as a task failure.
5. The command output becomes part of the run history unless output sharing is disabled.
6. That history can influence later tasks or child pipelines.

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
7. If the include is `sync: true`, the include step waits for the child result.
   A child failure marks the include step and parent pipeline failed before
   downstream parent tasks are allowed to run.
8. If the include is `sync: false`, the parent treats it as unblocking and continues.
9. Run detail and run list responses expose an aggregate lineage status for display:
   a parent is shown as running while a direct child run is still active, and a
   successful parent is shown as failed if a direct child later fails. The stored
   parent run result remains separate so dispatcher polling and rerun lifecycle
   logic keep using the run's own terminal state.

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
3. The dispatcher makes an authenticated internal service call to `nopsai` at `/v1/runs/{runID}/logs/ingest`.
4. The agent reports task status to `dispatcher.ReportTaskStatus`.
5. The dispatcher forwards that to `nopsai` at `/v1/runs/{runID}/steps/{step}/tasks/{task}`.
6. `nopsai` persists the update and asynchronously tells `git-bot` about the task status.
7. `git-bot` updates its in-memory check-run state and renders the GitHub check output.
8. When the agent finishes, it calls `dispatcher.FinalizeRun`. An agent that has paused for approval exits without finalizing, and late finalization attempts are ignored while the run is `waiting_approval`.
9. The dispatcher forwards final status to `nopsai`, which finalizes the run and notifies `git-bot` of the final result.
10. If the runner reports a job failure, including an agent container startup failure or nonzero agent exit, the dispatcher writes the runner error into the run logs and finalizes the run as `failure` with the same failure reason.
11. The UI refreshes run lists and details over REST polling, and log modals poll `/v1/runs/{runID}/logs?since_line=<id>` for incremental log lines.

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
   - `config-repositories/`
   - `access/`
   - `setting/`
5. It parses and validates each file class:
   - pipelines must parse and pass pipeline validation
   - reusable steps must parse and have matching names
   - triggers must parse as manifests
   - scope files turn `variables:` entries into scoped variables and `secrets:`
     entries into GitOps secret keys
   - knowledge markdown files are turned into `knowledge_contexts`
   - `config-repositories/groups/<group>.yaml` becomes a group config repo binding and group shell
   - `config-repositories/groups/<group>/structure.yaml` places apps under group shells with `name` and `repo_url`, and can define inline group repo `config:` blocks
   - `access/*.yaml` declares GitOps-managed users, service accounts, advanced roles, policies, role bindings, and scoped product-role grants; service-account token material is created at runtime, not synced from Git
   - `config-repositories/groups/<group>/notifications.yaml` in a system repo, or `notifications.yaml` in a group repo, becomes a pipeline notification policy with one or more named routes for that run group
   - `setting/system/llm_profile.yaml` becomes the system LLM profile registry, only from a system/global config repo
   - `setting/system/agent-profiles.yaml` becomes the system Agent Profile persona registry and default profile setting, only from a system/global config repo
   - `setting/system/mcp.yaml` becomes the system MCP server/profile registry, only from a system/global config repo
   - `setting/system/auth.yaml` becomes local-login and OIDC SSO settings, only from a system/global config repo, with provider credential references resolved from the encrypted registry
   - `setting/system/github.yaml` becomes GitHub App IDs, credential references, and git-bot URLs, only from a system/global config repo
   - `setting/system/runner.yaml` becomes runner install defaults, runtime defaults, and dispatcher routing, only from a system/global config repo
   - `setting/system/mail.yaml` becomes SMTP mail notification settings, only from a system/global config repo, with password plaintext kept out of the mail file
   - `setting/system/credentials.yaml` becomes encrypted system credential envelopes, only from a system/global config repo
6. System/global repositories are synced before group repositories during sync-all, so newly defined group bindings can be used immediately.
7. Group-scoped resources are normalized under the bound group before writing.
8. It adopts matching database-owned resources that are inside the syncing repository scope, then marks them GitOps-managed; resources already managed by an unrelated config repository remain protected by config-repo precedence.
9. It upserts rows with config-source metadata into Postgres.

For Git push, `nopsai` loads the same system or group config repository binding,
validates that `write_enabled` and `write_branch` are set, prefixes requested
file paths with the binding `base_path`, and asks `git-bot` to commit those
files to the review branch. The sync branch is not updated directly. The drift
endpoint exports the current declarative Nopsai config and compares it with the
sync branch so the UI can show exact changes for pipelines, steps, schedules,
triggers, scopes, knowledge contexts, run group/config-repository structure,
notification routes, access manifests, Agent Profiles, LLM profiles, MCP
registry files, auth settings, mail settings, runtime settings, and encrypted
credential envelopes before
pushing. After those files are merged into the sync branch, the next config sync
can adopt the matching database-owned resources and switch their UI source to
GitOps. Pipeline run rows remain runtime/audit records, not Git-owned resources.
Runtime settings sync persists supported operational defaults to the
`runtime_settings` database row. `config.yml`, `.env`, Docker Compose, and
deployment secrets are bootstrap inputs only. Credential GitOps stores encrypted
envelopes in the credential registry and never exports plaintext.
`dispatcher_routing` is exposed through a service-token protected internal
NopsAI endpoint; the dispatcher polls that endpoint and swaps its in-memory
routing table while it is running, so new scheduling decisions use the updated
table without a dispatcher restart. New service integrations should read
versioned snapshots from
`/internal/v1/runtime-config/{service}` or long-poll
`/internal/v1/runtime-config/{service}/watch?version=<n>`.
10. It prunes rows managed by the same config repository that disappeared from the repo.
11. It does not prune user-created groups, even when syncing the run-group structure.
12. It records sync status per config repository for the UI.

## 13. Failure Boundaries

Where failures stop the flow:

- Missing or invalid bearer token: stopped in `nopsai`
- AAA denial: stopped in `nopsai` with `403`
- Resource-use denial: stopped in `nopsai` before execution; GitHub-triggered denials create a failed check instead of a real run
- Invalid webhook signature: stopped at `git-bot`
- Invalid generic Git webhook signature or token: stopped in `nopsai` before payload normalization
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

- `git-bot` knows GitHub repository APIs and checks
- `nopsai` knows provider-neutral webhook sources and normalized Git events
- `nopsai` knows configuration and state
- `aaa` knows authorization decisions
- `dispatcher` knows scheduling
- `runner` knows Docker hosts
- `agent` knows pipeline execution and LLM-assisted decisions
