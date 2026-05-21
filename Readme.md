<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="services/ui/public/brand/nopsai-logo-dark.png">
    <img src="services/ui/public/brand/nopsai-logo-light.png" alt="NopsAI" width="420">
  </picture>
</p>

# NopsAI

NopsAI is a self-hosted, Git-aware automation platform for running AI-assisted
pipelines with enterprise-grade control over configuration, access, runtime
isolation, and auditability.

It combines a CI/CD-style execution model with LLM-native pipeline steps:
operators define reusable pipelines, scripts, goals, scopes, secrets, access
rules, and project knowledge; NopsAI resolves the runtime context, dispatches the
work to Docker-backed runners, and records the full execution lifecycle in a
central control plane.

## Why NopsAI

Modern engineering teams need automation that is both flexible enough for
LLM-assisted work and governed enough for production operations. NopsAI is built
around that balance:

- GitHub events and manual UI/API runs enter a centralized control plane.
- Pipelines can mix deterministic scripts with LLM-backed goals.
- Secrets, variables, knowledge documents, scopes, and access rules are resolved
  before dispatch.
- Work executes inside per-run agents and step containers on registered runners.
- Product roles and low-level AAA policy checks protect both configuration and
  runtime resource usage.
- GitOps configuration lets teams keep operational definitions reviewable,
  versioned, and reproducible.

## Core Capabilities

| Area | What NopsAI Provides |
| --- | --- |
| AI-assisted pipelines | YAML pipelines with scripts, natural language goals, reusable steps, child pipelines, dependency ordering, conditions, timeouts, volumes, and failure tolerance. |
| GitHub automation | GitHub App webhooks, signed webhook validation, repository file access, trigger manifests, check-run creation, check-run updates, reruns, and stale-check cancellation. |
| GitOps configuration | Sync pipelines, reusable steps, triggers, scopes, access rules, knowledge documents, LLM profiles, MCP settings, and group config repository bindings from Git. |
| Enterprise access control | Local auth, JWTs, refresh tokens, personal access tokens, predefined product roles, inherited folder grants, AAA checks, deny-before-allow evaluation, and audit logs. |
| Secrets and scopes | Encrypted secrets, plaintext scoped variables, strict scope isolation, repository-specific overrides, cross-scope references, and runtime authorization checks. |
| Knowledge context | Managed or repo-local markdown context for architecture docs, guardrails, policies, ADRs, runbooks, references, examples, and guidelines injected into LLM tasks. |
| Runner-based execution | Dispatcher-managed runners, Docker-backed agent containers, per-step containers, affinity, scope routing, capacity controls, cancellation, and durable logs. |
| First-install bootstrap | UI wizard for empty databases, generated runtime configuration, GitHub App guidance, starter repository groups, starter templates, user bootstrap, and setup guardrails. |
| MCP integration | System-managed MCP server and profile registry with optional profile examples and scope-aware enablement. |

## Architecture

NopsAI uses a control-plane/data-plane architecture.

```text
GitHub / UI / API
    |
    v
git-bot or nopsai API
    |
    v
nopsai control plane
  - auth
  - authorization
  - config resolution
  - run records
  - secrets and variables
  - knowledge snapshots
    |
    +--> aaa policy service
    |
    v
dispatcher
    |
    v
runner
    |
    v
agent container
    |
    v
step containers + optional child pipelines
```

### Services

- `services/nopsai`: Main REST API, control-plane state, authentication,
  authorization integration, config sync, run creation, and system management.
- `services/aaa`: Internal policy decision service for access checks, filters,
  introspection, inheritance, and authorization audit records.
- `services/git-bot`: GitHub App edge service for webhook verification,
  repository content access, GitHub check runs, and GitHub status updates.
- `services/dispatcher`: Scheduling hub between the control plane and runners.
- `services/runner`: Long-running worker attached to Docker that starts agent
  containers for assigned jobs.
- `services/agent`: Per-run orchestrator that executes pipeline logic, talks to
  the configured LLM provider, runs step containers, and streams status/logs.
- `services/ui`: Operator UI for runs, pipelines, triggers, scopes, access,
  knowledge context, system settings, and first-install setup.
- `db/init.sql`: Postgres schema for durable runtime, configuration, auth,
  access, setup, and audit state.

See [doc/architecture-overview.md](doc/architecture-overview.md) and
[doc/service-reference.md](doc/service-reference.md) for a deeper system view.

## How A Run Works

1. A user starts a run from the UI/API, or GitHub sends an event to `git-bot`.
2. `nopsai` authenticates the request and maps the route to an authorization
   decision.
3. Pipeline definitions, reusable steps, trigger rules, variables, secrets, and
   knowledge context are resolved from the database or Git-backed sources.
4. Runtime access checks verify that the caller can use referenced pipelines,
   steps, scopes, variables, secrets, and knowledge documents.
5. A durable run record is created in Postgres.
6. The dispatcher selects an eligible runner by scope, capacity, affinity, and
   dispatch state.
7. The runner starts a per-run agent container.
8. The agent executes script tasks or asks the configured LLM profile to resolve
   goal-based tasks into executable work.
9. Logs, task status, step status, final status, and GitHub check updates flow
   back through the control plane.

## First Install

NopsAI includes a first-install wizard for turning a fresh database into a
working workspace. Before login, the UI checks `/v1/setup/preflight` and shows
required database, master-key, and JWT guidance when the authenticated API is
not ready yet. After the default admin changes the first-login password, the UI
opens **System > Setup** once when setup is incomplete. After completion,
**System > Setup** stays available for reviewing runtime env groups, GitOps
downloads, generated file previews, and setup guidance.

After starting the stack, open the UI and go to **System > Setup**. The wizard
uses one guided setup path: required readiness and runtime steps must be
completed, while GitOps, GitHub, repository groups, AI, MCP examples, and user
bootstrap can be skipped and configured later.

The wizard can:

- guide the operator through setup in a step-by-step modal
- check database, admin, local secret, GitHub App configuration, git-bot
  service configuration, access, LLM, MCP, demo pipeline, and runner readiness
- generate missing local keys and tokens
- create or connect the global GitOps config repository
- preview starter GitOps templates
- create one or two starter repository groups and generate trigger/config
  entries for selected repositories
- seed starter resources directly into the database for the introduction
- configure a default LLM profile with one API key field
- seed disabled MCP examples
- seed local users with group role assignment and forced password change
- produce final runtime variables and GitOps file guidance
- block insecure bootstrap defaults

Read the full operator guide in
[doc/first-install-wizard.md](doc/first-install-wizard.md).

## Quick Start With Docker Compose

Prerequisites:

- Docker Engine or Docker Desktop with Compose support
- A Docker runtime available to the runner
- Postgres is provided by `docker-compose.yaml`
- A GitHub App for webhook-driven automation
- An LLM provider supported by the configured LLM profile, such as LM Studio or
  Gemini

1. Review `config.yml` and `.env`.

   `config.yml` documents the available runtime settings. `.env` is loaded by
   the Docker Compose services. Do not commit real secrets.

2. Start the stack.

   ```bash
   docker compose -f docker-compose.yaml up --build
   ```

3. Open the UI.

   ```text
   UI:       http://localhost
   API:      http://localhost:8080
   git-bot:  http://localhost:8081
   Postgres: localhost:5432
   ```

4. Sign in with the local bootstrap administrator.

   ```text
   Email:    admin@example.com
   Password: admin
   ```

   This default is for bootstrap only. Change it immediately. The setup wizard
   reports `admin/admin` as an insecure state.

5. Run **System > Setup** after changing the first admin password.

6. Configure the GitHub App webhook URL shown in the wizard and verify the
   git-bot runtime settings.

7. Create one or two starter repository groups, apply setup, and run the starter
   `setup/first-run` pipeline to verify runner, agent, LLM, logs, and UI.

To stop and remove local state:

```bash
docker compose -f docker-compose.yaml down -v
```

## Configuration Model

NopsAI supports both database-managed and Git-managed configuration.

GitOps sync can import:

- `pipelines/`: pipeline definitions
- `steps/`: reusable step definitions
- `triggers/`: repository trigger overrides
- `scopes/`: scoped variables
- `knowledge/`: managed knowledge documents
- `access/`: users, roles, policies, bindings, and basic product role grants
- `config-repositories/`: global and group config repository bindings
- `setting/system/llm_profile.yaml`: system LLM profile registry
- `setting/system/mcp.yaml`: MCP server and profile registry
- `pipelineruns/structure.yaml`: legacy run group structure

Secrets are not imported from Git. Store secret values in NopsAI's encrypted
secret store and reference them by name from pipelines, LLM profiles, and MCP
profiles.

See [doc/sample-config-repo/README.md](doc/sample-config-repo/README.md) for a
working GitOps layout.

## Pipeline Example

Pipelines are declarative YAML definitions. A pipeline can combine reusable
steps, scripts, and LLM-backed goals:

```yaml
name: first-run
version: "1.0.0"
description: Verify that NopsAI can run a starter job and optional AI step.
container_image: alpine:3.20
timeout: 10m
llm_profile: standard
variables:
  - dev:NOPSAI_SETUP_PROFILE
steps:
  - name: announce
    include: step:setup/announce

  - name: runner-smoke
    image: alpine:3.20
    script: |
      #!/bin/sh
      set -e
      echo "NopsAI runner is executing"
    depends_on:
      - announce

  - name: ai-smoke
    goal: Return one short sentence confirming the setup smoke test reached the agent.
    ignore_failure: true
    depends_on:
      - runner-smoke
```

See [doc/feature-reference.md](doc/feature-reference.md) and
[doc/sample-pipeline](doc/sample-pipeline) for broader examples.

## Security And Governance

NopsAI is designed for controlled self-hosted operation:

- Local authentication with access tokens, refresh tokens, password changes,
  login rate limits, and personal access tokens.
- Product roles: `viewer`, `developer`, `owner`, and `admin`.
- AAA-backed authorization with deny-before-allow semantics.
- Folder-level access inheritance for child resources.
- Runtime resource-use checks for manual runs, Git-triggered runs, and child
  pipelines.
- Resource visibility modes for reusable pipelines, steps, scopes, and
  knowledge context.
- Encrypted secret values using the configured master key.
- Strict scope isolation for scoped secrets and variables.
- Secret masking in agent logs and execution history.
- Audit logging for denied requests and sensitive allowed operations.
- Internal service authentication for dispatcher and runner/agent callbacks.
- Production setup guardrails that reject unsafe direct bootstrap behavior.

For details, read:

- [doc/access-control.md](doc/access-control.md)
- [doc/jwt-authentication.md](doc/jwt-authentication.md)
- [doc/knowledge-context.md](doc/knowledge-context.md)

## GitHub App Requirements

NopsAI's GitHub automation is implemented through `git-bot`.

Required GitHub App events:

- `push`
- `pull_request`
- `check_run`
- `check_suite`
- `ping`

Required GitHub App permissions:

- `contents`: read
- `metadata`: read
- `pull_requests`: read
- `checks`: read and write

The first-install wizard shows the public webhook URL for GitHub and the
internal service URLs used between NopsAI and git-bot.

For local webhook simulation, see [doc/triggering.md](doc/triggering.md).

## LLM And MCP

LLM-driven work is configured through system LLM profiles. Profiles can be
managed in the UI/API or through the global config repository at
`setting/system/llm_profile.yaml`.

Supported profile concepts include:

- provider selection, currently including Gemini and LM Studio paths in the
  runtime configuration
- model name
- base URL for local/provider-compatible endpoints
- API key secret reference
- allowed scopes
- optional reasoning controls where supported by the provider path

MCP servers and MCP profiles can be managed through system configuration at
`setting/system/mcp.yaml`. The setup wizard can seed disabled MCP examples so
operators can review and enable them deliberately.

See:

- [doc/llm-model-selection.md](doc/llm-model-selection.md)
- [doc/mcp-pipeline-integration.md](doc/mcp-pipeline-integration.md)

## Production Checklist

Before production use:

- Replace bootstrap database credentials and local admin credentials.
- Generate strong values for all signing, webhook, dispatcher, AAA, and master
  key settings.
- Keep secrets out of Git and out of container images.
- Require GitHub webhook signature verification.
- Configure a GitHub App with the required events and permissions.
- Connect a global GitOps config repository and make it the source of truth for
  production resources.
- Use GitOps from **System > Setup** as the source of truth before onboarding
  production automation.
- Review product role grants and folder inheritance.
- Restrict runner scopes and capacity according to environment.
- Check dispatcher, runner, git-bot, LLM, and config sync health checks.
- Back up Postgres and protect access to the Docker socket on runner hosts.
- Review audit logs for sensitive operations.

## Repository Layout

```text
config/                 Runtime configuration loader and tests
container/              Service Dockerfiles
db/                     Postgres schema and seed data
doc/                    Architecture, API, feature, auth, GitOps, and setup docs
pkg/                    Shared models, protobuf contracts, service auth, TLS, proxy helpers
services/aaa            Authorization decision service
services/agent          Per-run pipeline orchestrator
services/dispatcher     Scheduler and runner bridge
services/git-bot        GitHub App integration
services/nopsai         Main API and control plane
services/runner         Docker-backed runner
services/ui             React operator UI
test/                   Local operational and performance scripts
```

## Development

Run backend tests:

```bash
go test ./...
```

Build the UI:

```bash
cd services/ui
npm install
npm run build
```

Useful local docs:

- [doc/README.md](doc/README.md): documentation map
- [doc/api.md](doc/api.md): REST API guide
- [doc/runtime-flows.md](doc/runtime-flows.md): runtime flow walkthroughs
- [doc/decision-architecture.md](doc/decision-architecture.md): architectural
  decisions and tradeoffs

## Documentation Map

Start here:

1. [Architecture overview](doc/architecture-overview.md)
2. [Service reference](doc/service-reference.md)
3. [First-install wizard](doc/first-install-wizard.md)
4. [Access control](doc/access-control.md)
5. [API guide](doc/api.md)
6. [Feature reference](doc/feature-reference.md)
7. [Runtime flows](doc/runtime-flows.md)

## Project Status

This repository contains the NopsAI product implementation and local deployment
shape. The documentation describes the current codebase, not an external
managed service. Treat the Docker Compose setup as the fastest path to local
evaluation, and use the production checklist plus GitOps model for hardened
deployments.
