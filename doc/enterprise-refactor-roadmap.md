# Enterprise Refactor Roadmap

This roadmap keeps clean-code work incremental and reviewable. The target is an
enterprise-grade product structure where handlers, runtime preparation, provider
integrations, execution logic, and UI shells have clear ownership.

## Current Branch

This branch starts with behavior-preserving extraction:

- `services/nopsai` is now an importable service package instead of the binary
  package.
- `services/nopsai/app.go` owns NopsAI application state and shared service DTO
  aliases used across handlers.
- `services/nopsai/bootstrap.go` owns the service construction boundary:
  application dependency assembly, database bootstrap sequencing, setup
  preflight delegation, handler handoff, and background worker startup.
- `services/nopsai/app_security.go` owns auth service construction, RBAC
  enforcer loading, AAA client/fallback construction, audit logger setup,
  internal HTTP client creation, and the default git-bot provider wiring used
  by NopsAI service bootstrap.
- `services/nopsai/bootstrap_schema.go` owns database bootstrap step
  registration so schema and seed additions have a single review point.
- `services/nopsai/enterprise_gates.go` owns the Batch 4 enterprise startup
  gate checks for production-grade secrets, service-token isolation, dispatcher
  transport security, GitHub webhook signing readiness, and default-admin
  password hardening.
- `.github/workflows/enterprise-gates.yml` and `scripts/enterprise-gates.sh`
  define the Batch 4 enterprise verification gates for tests, race tests, vet,
  lint, security scanning, vulnerability scanning, and Docker build checks.
- `pkg/httpapi/server.go` owns shared HTTP server timeout defaults used by the
  NopsAI API, setup preflight mode, AAA, and git-bot servers.
- `doc/package-ownership.md` documents the package ownership rules for
  handlers, services, stores, domain/internal packages, DTOs, provider clients,
  and command bootstrap.
- `services/nopsai/cmd/nopsai/main.go` is the thin command entrypoint for the
  NopsAI API binary.
- `services/nopsai/internal/app` owns NopsAI API process bootstrap and runtime
  wiring: config loading/defaulting, logging setup, env-file resolution,
  setup-preflight mode selection, database connection retry policy, dispatcher
  client auth/TLS setup, service-token authenticator setup, HTTP server
  lifecycle, background worker context ownership, signal handling, and graceful
  shutdown.
- `services/nopsai/auth_models.go` owns auth, user, service-account, token, and
  role request/response DTOs used by the HTTP handlers.
- `services/nopsai/http_middleware.go` owns CORS, request IDs, request logging,
  recovery, authentication, authorization, service-token authentication,
  password-change enforcement, and audit middleware.
- `services/nopsai/config_runtime.go` owns runtime config snapshot accessors and
  container-reachable local LLM endpoint normalization.
- `services/nopsai/config_sync_status.go` owns config-sync status state,
  cloning, and running-sync guards.
- `services/nopsai/pipeline_runtime.go` owns pipeline validation delegation and
  runtime secret preparation/authorization before dispatch.
- `services/nopsai/run_helpers.go` owns shared run naming, branch-pattern
  matching, trigger-event IDs, run status normalization, and Git fetch sentinel
  errors.
- `services/nopsai/repository_branch_handlers.go` owns the repository branch
  listing endpoint.
- `services/nopsai/run_launcher.go` defines the consumer-side `RunLauncher`
  boundary and owns agent launch preparation, runtime environment construction,
  dispatcher submission, and launch failure handling. Manual run, rerun, and
  approval-resume orchestration hand off through the launcher interface instead
  of calling concrete launch plumbing directly.
- `services/nopsai/run_service.go` owns shared run orchestration concerns:
  initial manual-run and rerun record insertion, trigger-event normalization,
  run group resolution, resolved pipeline preparation, authorization snapshot
  refreshes, task-run initialization, launch handoff, and durable failure
  marking.
- `services/nopsai/internal/runs` owns run query/detail shaping concerns:
  run-list SQL construction and scanning, branch grouping, parent/child/task
  detail loading, run-detail step status/duration derivation, run-detail ETags,
  and group-resolution candidate selection.
- `services/nopsai/internal/gitbot` owns the nopsai-to-git-bot HTTP client:
  repository file/directory reads, config commits, repository access checks,
  branch PR checks, pipeline fetches, check-run creation/initialization,
  stale-check cancellation, and run/task status notifications.
- `services/nopsai/gitbot_client.go` defines the consumer-side `GitProvider`
  boundary for repository reads, config commits, branch PR status, pipeline
  fetches, check-run lifecycle operations, stale-check cancellation, and
  run/task status notifications. The concrete git-bot HTTP provider is wired in
  service bootstrap, keeping handlers and orchestration independent of raw
  URL/JSON/response plumbing.
- `services/nopsai/dispatcher_client.go` defines the consumer-side
  `DispatcherClient` boundary for job submission, dispatcher status lookup, and
  runner dispatch controls. The concrete generated gRPC client is adapted in
  bootstrap, keeping handlers and run orchestration independent of generated
  gRPC call plumbing.
- `services/nopsai/aaa_helpers.go` defines the consumer-side `AAAClient`
  boundary for subject introspection, single and batch authorization checks,
  resource filtering, and audit decision recording. The concrete AAA HTTP
  client and in-process evaluator fallback are wired in service bootstrap,
  keeping handlers independent of HTTP/Postgres authorization plumbing.
- `services/nopsai/internal/configsync` owns shared config-sync structure and
  ownership helpers: repository base-path normalization, relative path
  matching, folder-bound resource normalization, YAML identifier helpers,
  config repository binding parsing/validation/write-setting defaults,
  config repository request shaping, write-path validation, repository
  URL/full-name normalization, pipeline-run group-structure parsing, config
  resource scope checks, config-repository drift path/ownership decisions,
  drift file diffing/content normalization, config-repository group-structure
  export shaping, and config-repository overwrite/adoption rules.
- NopsAI config-sync callers now import `services/nopsai/internal/configsync`
  directly; the temporary `services/nopsai/configsync_aliases.go` migration
  shim has been removed.
- `services/nopsai/config_repository_git_paths.go` owns shared GitOps
  directory layout construction for config repository sync and drift loading.
- `services/nopsai/config_sync_fetch.go` owns Git-backed config repository
  discovery for sync: binding repository normalization, git-bot directory/file
  reads, repository access checks, and the folder-scoped root notification
  compatibility fetch behind a narrow reader interface.
- `services/nopsai/config_sync_parse.go` owns Git-backed config repository
  parse planning for sync: pipeline-run structures, nested config repository
  bindings, access manifests plus embedded resource access, GitOps settings,
  schedules, external triggers, notification routes, pipelines, reusable steps,
  scope variables/secrets, and trigger manifests.
- `services/nopsai/config_sync.go` owns the Git-backed config repository sync
  coordinator: sync-detail initialization, repository context creation, fetch
  handoff, parse handoff, apply handoff, and completion logging.
- `services/nopsai/config_sync_store.go` defines the consumer-side
  `ConfigSyncStore` boundary for config repository listing, sync-status
  updates, and apply/prune handoff, keeping config-sync coordination
  independent of concrete Postgres/apply plumbing.
- `services/nopsai/config_sync_apply.go` owns the Git-backed config repository
  sync apply boundary: delegated-scope filtering, transactional upsert/prune,
  access/settings/notification application, transaction commit, and
  post-commit runtime/profile registry updates.
- `services/nopsai/config_sync_runner.go` owns the HTTP trigger, global
  repository iteration, per-repository sync status updates, and aggregate sync
  status construction.
- `services/nopsai/config_sync_scope_entries.go` owns scope variable/secret
  config-entry parsing, encrypted GitOps secret handling, and inline config
  repository binding extraction from pipeline run structures.
- `services/nopsai/config_sync_groups.go` owns delegated-scope filtering,
  config-sync write ownership checks, repository structure filtering, and group
  application during sync.
- `services/nopsai/config_repository_drift.go` owns config-repository drift
  endpoints, desired-state composition, Git file loading, drift comparison, and
  knowledge drift canonicalization.
- `services/nopsai/config_repository_paths.go` owns config-repository export
  paths, drift path options, desired-file content normalization, and shared
  export path constants.
- `services/nopsai/config_repository_resource_export.go` owns config-repository
  pipeline, step, trigger, schedule, and notification route export collectors.
- `services/nopsai/config_repository_scope_export.go` owns config-repository
  scope variable, scope secret, and embedded scope-access export documents.
- `services/nopsai/config_repository_knowledge_export.go` owns
  config-repository knowledge context export rendering.
- `services/nopsai/config_repository_group_export.go` owns config-repository
  group-structure export rendering.
- `services/nopsai/config_repository_access_export.go` owns config-repository
  user, service-account, role, policy, binding, and basic-role grant exports.
- `services/nopsai/config_repository_resource_access.go` owns embedded resource
  access planning, access YAML merge/canonicalization, table visibility export,
  and resource-use grant export.
- `services/nopsai/config_repository_settings_export.go` owns config-repository
  LLM profile, MCP registry, runtime settings, and mail settings exports.
- `services/nopsai/secret_codec.go` defines the consumer-side `SecretCodec`
  boundary for secret encryption/decryption. AES-256-GCM remains the concrete
  default wired during service bootstrap, while handlers, runtime preparation,
  setup, and GitOps parsing use the boundary.
- `services/nopsai/internal/runnerinstall` owns runner bootstrap artifact
  generation: Docker compose snippets, one-time Docker install scripts,
  Kubernetes runner manifests, Kubernetes install commands, dispatcher address
  adaptation, and install-response DTOs.
- `services/nopsai/internal/systemconfig` owns reusable system configuration
  shaping helpers: system config responses, dispatcher-routing normalization,
  runner-scope normalization, runner limit validation, JSON env encoding, and
  `.env` file updates.
- `services/nopsai/system_handlers.go` owns system configuration mutation,
  runner install/bootstrap endpoints, runtime scope listing, dispatcher status,
  internal dispatcher routing, and runner dispatch controls.
- `services/nopsai/runner_bootstrap_tokens.go` keeps one-time runner bootstrap
  token lifecycle state on the application without keeping install artifact
  generation in `main.go`.
- `services/nopsai/run_handlers.go` keeps run list/detail/start/rerun HTTP
  flows and still calls the same launch entrypoints through service/internal
  package boundaries.
- `services/nopsai/run_group_resolution.go` owns no-store response headers,
  run-state transition helpers, repository/group resolution, repository-name
  parsing, and Git check-run ID normalization used by run creation flows.
- `services/nopsai/run_failure_records.go` owns synthetic failed-run records
  for missing pipelines and authorization-denied Git-triggered runs.
- `services/nopsai/run_lifecycle_handlers.go` owns run cancellation and run
  deletion endpoints.
- `services/nopsai/run_internal_handlers.go` owns dispatcher-facing task
  updates, finalization, run status lookup, check-run lookup, and log ingest/read
  endpoints.
- `services/nopsai/group_handlers.go` owns group/app normalization, group CRUD,
  move, and delete authorization target resolution.
- `services/ui/src/app/types.ts` centralizes shell-level UI contracts.
- `services/ui/src/app/navigation.tsx` owns primary and system navigation
  definitions.
- `services/ui/src/app/constants.ts` owns app-shell constants.
- `services/ui/src/app/icons.tsx` owns app-shell SVG components.
- `services/ui/src/app/runSidebarUtils.ts` owns pure run-sidebar formatting,
  matching, grouping, and status helpers.
- `services/agent/internal/scheduler` owns pure pipeline scheduling decisions:
  runnable task selection, task counting, approval prioritization, completed-task
  snapshots, and image pre-pull ordering.
- `services/agent/internal/executor` owns deterministic action preparation,
  action file-path safety, shell quoting, return-answer handling, and Docker
  exec result collection.
- `services/agent/internal/dockerexec` owns Docker runtime integration helpers:
  image pull checks, step container creation, volume preparation, container
  cleanup, image pre-pull, and Docker step container naming.
- `services/agent/internal/kubernetesexec` owns Kubernetes runtime integration:
  runtime initialization from environment, workspace PVC preparation, step pod
  lifecycle, Kubernetes exec, runtime-pool scheduling, and Kubernetes-specific
  name/env/volume helpers.
- `services/agent/internal/resolver` owns execution-context construction,
  secret masking, prompt request construction, direct-script and LLM-backed
  action resolution, condition evaluation, retry policy, cancellation checks,
  and action/condition outcomes.
- `services/agent/action_session.go` adapts the concrete package-level LLM and
  MCP registries to the narrow action-session interface consumed by
  `internal/resolver`.
- `services/agent/internal/approval` owns approval pause checkpoint
  orchestration: workspace archive capture, checkpoint payload construction,
  pause endpoint submission, and staged pause failure reporting.
- `services/agent/approval_adapter.go` adapts concrete workspace and API
  functions to the narrow approval orchestration contracts.
- `services/agent/internal/include` owns child-pipeline include orchestration:
  include target validation, context-aware child definition fetches and trigger
  dispatch, sync/async monitor startup, child final-status reporting, and sync
  failure propagation.
- `services/agent/include_adapter.go` adapts concrete child-pipeline API
  functions and not-found classification to the include workflow contracts.
- `services/agent/approval_checkpoint.go` owns approval checkpoint archive size
  limits, workspace archive/restore safety, and archive tests.
- `services/agent/nopsai_client.go` owns authenticated NopsAI API requests,
  approval pause/checkpoint calls, child-pipeline trigger/monitor helpers, run
  cancellation watching, and pipeline definition fetches used by adapters.
- `services/agent/agent_logging.go` owns agent/step logger construction and
  pipeline identifier splitting.
- `services/agent/workspace_listing.go` owns workspace include/ignore matcher
  parsing and directory listing for LLM task context.
- `services/agent/dispatcher_reports.go` owns the dispatcher client handle and
  final/task status reporting helpers.
- `services/agent/internal/app` owns application bootstrap and runtime wiring
  concerns for the per-run agent: logging setup, runtime environment parsing,
  payload decode-warning classification, pipeline definition decoding, timeout
  parsing, startup failure log messages, dispatcher client auth/TLS setup,
  dispatcher request wrappers, execution runtime client setup, active-task
  tracking, pipeline timeout cancellation, termination signal handling, and
  step session tracking/cleanup orchestration. It also owns the per-run pipeline
  execution loop: runnable-task scheduling, approval pauses, child-pipeline
  includes, inherited/resumed execution history, action retries, task/final
  status reporting, and cancellation/timeout cleanup behind explicit runner
  interfaces.
- `services/agent/cmd/agent/main.go` is the thin command entrypoint for the
  agent binary and delegates to the importable agent package.
- `services/agent/app.go` now stays focused on concrete process wiring:
  runtime config loading, approval checkpoint restore, LLM/MCP registry
  construction, dispatcher client setup, knowledge snapshot loading, execution
  runtime construction, and the `internal/app.RunPipeline` handoff.
- `services/git-bot/internal/checkrender` owns GitHub check-run summary
  rendering for flat markdown lists, dependency trees, and Mermaid graphs.
- Git-bot check-run state callers now use `internal/checkrender` directly; the
  temporary `services/git-bot/checkrender_aliases.go` migration shim has been
  removed.
- `services/git-bot/cmd/git-bot/main.go` is the thin command entrypoint for the
  GitHub App edge-service binary.
- `services/git-bot/internal/app` owns git-bot process bootstrap and runtime
  wiring: config loading, optional private-key file materialization, logging
  setup, GitHub App transport creation, internal-aware HTTP client setup, and
  HTTP server startup.
- `services/git-bot/internal/service` owns the GitHub/webhook/check-run HTTP
  service: webhook HMAC validation and forwarding, repository file/directory
  reads, config commits, repository/branch checks, installation repository
  listing, pipeline content reads, check-run lifecycle endpoints, child
  check-run creation, route registration, and check-run state updates.
- `services/git-bot/internal/service/nopsai_forwarder.go` owns webhook
  forwarding to NopsAI behind a narrow forwarder interface.
- `services/git-bot/internal/service/github_repository.go` owns the GitHub
  repository provider boundary for file/directory reads, repository access
  checks, branch PR checks, installation repository listing, and pipeline
  content reads.
- `services/dispatcher/cmd/dispatcher/main.go` is the thin command entrypoint
  for the dispatcher binary.
- `services/dispatcher/internal/app` owns dispatcher process bootstrap and
  runtime wiring: config loading, logging setup, internal credentials,
  service-token authentication, transport TLS, gRPC server registration,
  listener startup, stale-runner reaping, and routing-sync workers.
- `services/dispatcher/internal/service` owns the dispatcher gRPC scheduling
  service: runner registration, queueing and assignment, runner dispatch
  controls, route synchronization, NopsAI callback forwarding, run-status
  checks, runner lifecycle cleanup, and service-token method authorization.
- `services/dispatcher/internal/service/nopsai_client.go` owns the dispatcher
  side NopsAI callback client for log ingest, task status, finalization,
  pipeline fetch/trigger, run-status checks, and routing sync.
- `services/k8s-runner/cmd/k8s-runner/main.go` is the thin command entrypoint
  for the Kubernetes runner binary.
- `services/k8s-runner/internal/app` owns Kubernetes runner process bootstrap
  and runtime wiring: config loading, logging setup, runner identity/capacity
  defaults, dispatcher client auth/TLS setup, Kubernetes REST config discovery,
  Kubernetes client creation, and service handoff.
- `services/k8s-runner/internal/service` owns the Kubernetes runner runtime:
  dispatcher registration and heartbeat streaming, job acceptance, agent pod
  creation, workspace volume selection, runtime environment propagation, pod log
  forwarding, cancellation polling, cleanup, scheduling defaults, Kubernetes
  name/label normalization, and runner metadata.
- `services/k8s-runner/internal/service/workspace.go`,
  `services/k8s-runner/internal/service/pod.go`, and
  `services/k8s-runner/internal/service/scheduling.go` own workspace-volume
  planning, agent-pod construction/lifecycle, and runtime-pool scheduling
  defaults behind focused tests.

The goal of this slice is lower coupling without changing API behavior, UI
routes, permission decisions, or dispatcher semantics.

## Principles

- Prefer behavior-preserving extractions before package moves.
- Keep public route and API contracts stable unless a feature requires a change.
- Move pure helpers first, then services, then package boundaries.
- Add tests around extracted helpers when future work changes behavior.
- Keep Git provider, LLM, MCP, secrets, and runner concerns behind explicit
  interfaces before adding new providers or execution modes.

## Completion Status

The Batch 4 backend ownership items in this branch are complete and verified
with `SKIP_DOCKER_BUILDS=1 scripts/enterprise-gates.sh` on 2026-06-06. The
remaining items below are intentionally kept as Batch 5/product-scope backlog
because they change broader service hardening or UI/module contracts.

## Batch 5 Backlog

1. Finish the git-bot provider split by moving check-run create/update/list
   calls behind a dedicated GitHub checks provider.
2. Continue tightening the dispatcher service split by peeling pure scheduling
   and queue-assignment rules into smaller service files with focused tests.
3. Continue tightening the agent runner boundary by moving root-package concrete
   LLM/MCP, knowledge-context, dispatcher status, and NopsAI API callback wiring
   behind explicit internal constructors.
4. Apply startup-gate checks to dispatcher, agent, AAA, git-bot, runner, and
   Kubernetes runner configuration surfaces.
5. Split `services/ui/src/pages/System.tsx` into route-level system feature
   modules for config, setup, LLM profiles, MCP, dispatcher, access, and data
   management.
6. Generate or share API DTOs between Go and TypeScript once the run/system
   service contracts are stable.

## Review Guidance

Review this branch as a structural cleanup. The main things to check are:

- agent launch failure reasons and GitHub notification behavior still match the
  previous flow
- dispatcher assignment and queued-state handling remain unchanged
- manual run and rerun `pipeline_runs` fields, trigger event IDs, group
  resolution, Git check IDs, and authorization snapshots remain unchanged
- dispatcher client auth/TLS fallbacks and task/final-status reporting remain
  unchanged
- app navigation visibility and system subnavigation permission filtering remain
  unchanged
- run sidebar labels, status colors, group matching, and search behavior remain
  unchanged
