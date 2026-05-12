# NopsAI Decision Architecture

This document explains the main architectural decisions visible in the codebase and the tradeoffs behind them.

## 1. Split Control Plane From Data Plane

Decision:

- Keep `nopsai`, `git-bot`, and `dispatcher` separate from `runner` and `agent`.

Why:

- The API layer can stay durable and stateful.
- Execution can scale independently on Docker-capable hosts.
- Runner hosts can be placed closer to the environments they operate on.

Tradeoffs:

- More moving parts than a single-process system
- Need both HTTP and gRPC contracts
- More care required around internal auth and status propagation

## 2. Use Long-Lived Runners But Ephemeral Agents

Decision:

- Make runners persistent and agents per-run.

Why:

- Runners amortize the cost of registration and host-level setup.
- Agents isolate per-run state, secrets, history, and pipeline definition snapshots.
- Failed or cancelled runs can be cleaned up without restarting the runner.

Tradeoffs:

- Each run pays the cost of agent container startup.
- Debugging may span runner, agent, and step-container layers.

## 3. Reuse One Step Container Per Step

Decision:

- Reuse the same step container across tasks in a step instead of creating a new container per task.

Why:

- Tasks inside a step can share working-directory changes naturally.
- Reduces Docker startup overhead for sequential task chains inside one step.
- Fits the idea that a step is a session-like execution context.

Tradeoffs:

- Tasks in the same step are less isolated from each other.
- A task can affect the environment seen by later tasks in that step.

## 4. Resolve Reusable `step:` Includes Before Dispatch

Decision:

- Expand reusable steps inside `nopsai` before the run reaches the agent.

Why:

- Validation runs against the actual expanded pipeline.
- The agent only needs one execution model.
- Run snapshots stored in the DB are closer to what actually executed.

Tradeoffs:

- Reusable steps must exist in the DB at resolution time.
- Git-backed step changes do not affect a run unless they have already been synchronized.

## 5. Keep Child Pipeline Includes As Runtime Events

Decision:

- Treat `pipeline:` includes as child runs instead of static expansion.

Why:

- Child pipelines get their own run records, logs, statuses, and GitHub check handling.
- Allows async child execution and better operational visibility.
- Makes large workflows composable.

Tradeoffs:

- More coordination between agent, dispatcher, and `nopsai`
- Parent/child status handling becomes more subtle, especially with `sync: false`

## 6. Let The Agent Call The LLM Directly

Decision:

- Use an embedded `LLMClient` in the agent instead of a separate live LLM microservice.

Why:

- Fewer runtime services to deploy
- Simpler data flow for prompt building, secret masking, and retry logic
- Keeps LLM context close to the execution environment

Tradeoffs:

- LLM provider concerns live inside the agent binary
- Less centralized control over LLM usage than a dedicated mediation service

Note:

- `pkg/proto/agent.proto` still describes an `LLMService`, but current runtime code uses direct provider calls.

## 7. Use Git As The Main Config Source, But Allow Database Overrides

Decision:

- Favor Git-backed configuration while still allowing database-managed overrides.

Why:

- Git is a natural system of record for pipelines and triggers.
- Database overrides are useful for emergency changes, central policy, or multi-repo governance.
- The config sync path keeps Git-managed objects easy to refresh and prune.

Tradeoffs:

- Two sources of truth must be understood clearly
- Operators need to know whether a pipeline/trigger came from Git or the database

## 8. Make Scoped Resolution Strict

Decision:

- Do not let scoped runs fall back to unscoped variables or secrets.

Why:

- Prevents accidental leakage of default credentials/config into environments like `prod`.
- Makes missing environment-specific setup fail fast.

Tradeoffs:

- More configuration work up front
- A partially configured scope fails instead of silently using defaults

## 9. Preserve Runner Affinity For Related Runs

Decision:

- Use `trigger_event_id`, parent run, and `preferred_runner_id` to keep related work on the same runner when possible.

Why:

- Child pipelines often benefit from locality with their parent.
- Affinity reduces surprising cross-run placement changes during a single event tree.
- Helps when runners have environment-specific tooling or caches.

Tradeoffs:

- Affinity can delay a job when the preferred runner is full.
- Scheduler behavior is slightly more complex than simple load balancing.

## 10. Use Dispatcher As Both Scheduler And Bridge

Decision:

- The dispatcher is not only a scheduler; it also translates agent/runner gRPC activity into internal HTTP calls to `nopsai`.

Why:

- Keeps runners and agents decoupled from direct API routing details.
- Centralizes internal status forwarding and trusted-call behavior.
- Lets `nopsai` keep its public/internal APIs while runners stay gRPC-native.

Tradeoffs:

- Dispatcher becomes a critical path component
- Extra hop for logs and status updates

## 11. Use Internal JWTs For Trusted Dispatcher Calls

Decision:

- Dispatcher mints short-lived internal JWTs when calling protected internal `nopsai` endpoints.

Why:

- Reuses the auth machinery already present in the system
- Avoids leaving internal endpoints unauthenticated
- Makes internal trust explicit in code

Tradeoffs:

- Dispatcher must share JWT signing config with `nopsai`
- Internal auth is still only as strong as the current network and secret handling

## 12. Run AAA As A Service With Local Fallback

Decision:

- Put the primary authorization decision point in `services/aaa`, while keeping an in-process evaluator fallback in `nopsai`.

Why:

- Authorization logic has a clear service boundary and HTTP contract.
- The UI/API can use the same `Check`, `Filter`, and `Introspect` behavior everywhere.
- Short AAA service outages do not have to block all protected requests, because the fallback uses the same Postgres policy data.

Tradeoffs:

- There is another runtime service and shared internal token to configure.
- `nopsai` still imports AAA evaluator/store packages for fallback, so the boundary is operational rather than a fully independent deployment boundary.
- Operators must monitor both the AAA service health and authorization decision logs.

## 13. Expand Product Roles Into Low-Level ACLs

Decision:

- Represent `viewer`, `developer`, `owner`, and `admin` as product-level grants, then expand non-admin grants into `resource_acl` rows.

Why:

- The evaluator can stay generic and does not need product-role special cases.
- Product access management stays understandable for UI users.
- Folder inheritance works through the same resource inheritance path as other ACLs.

Tradeoffs:

- Grant changes write multiple rows and need reconciliation on startup.
- Explaining an effective permission requires mapping low-level ACL matches back to product-role language.

## 14. Keep Postgres As The Durable Source Of Operational Truth

Decision:

- Store runs, tasks, logs, config objects, user auth data, and audit data in one relational store.

Why:

- The UI and APIs can query consistent execution state
- Restarts do not lose run history
- Parent/child relationships and config metadata are easy to model relationally

Tradeoffs:

- High write volume from logs and status updates lands in the main DB
- Future scaling may require more specialized event/log storage

## 15. Favor Simple Local-First Infrastructure Defaults

Decision:

- Docker Compose, insecure gRPC, and direct Docker-socket access are the default operating model.

Why:

- Keeps local development and self-hosted experimentation easy
- Reduces initial infrastructure requirements
- Matches the current maturity level of the project

Tradeoffs:

- Production-hardening still requires extra work
- Security boundaries are weaker than in a hardened multi-tenant design

## 16. Design Intent Visible In The Code

Taken together, these decisions show a clear intent:

- keep the control plane understandable
- keep execution scalable and isolated enough for CI/CD work
- keep GitHub integration first-class
- allow LLM assistance without making the whole platform LLM-dependent
- prefer practical operability over premature infrastructure complexity

That makes the current architecture a good fit for a self-hosted, Docker-centric automation platform that is growing toward a more distributed model.
