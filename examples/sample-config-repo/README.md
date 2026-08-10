# Config repository samples

These examples show the Git layout consumed by Nopsai config sync.

This sample lives under `examples/sample-config-repo` so it can be copied,
tested, and referenced independently from the explanatory docs in `doc/`.

The first-install wizard can preview a smaller starter version of this layout
from **System > Setup**. Production installs should commit the generated starter
files to the global config repository and import them through config sync instead
of seeding starter resources directly into the database.

There are two repositories represented here:

- `global-repo`: a system/global config repository, bound as `scope_type=system`, `scope_id=global`, and `base_path=""`.
- `team-1-repo`: a team-owned config repository, bound to team `team-1` by the global repo.

The global repo can sync every shared config type and can define team config
repo bindings. Team repos then become authoritative for everything under their
team path.

`team-1-repo` includes an intent-driven dashboard publication sample:
`pipelines/team-1/dashboard-sample.yaml` publishes the `Service metrics` final
output to `dashboards/team-1/ops-dashboard.yaml` in the `service-metrics` section. The
pipeline prompt asks for service metrics from JSON log evidence and lets NopsAI
choose the best dashboard structure when the prompt does not name a table,
text, bar chart, or another visualization.
`pipelines/team-1/dashboard-multi-output-sample.yaml` declares the same
`dashboard-sample` pipeline with several final outputs to exercise all dashboard
presets and every publication mode: `replace`, `append`, `snapshot`, and
`series`. It also includes circular `donut`/`pie` chart prompts and a boolean
readiness matrix so dashboard rendering can be checked against graph and status
chip views. The `report` preset example is narrative-first and keeps any table
as supporting evidence after the executive summary, changes, blockers, and next
action sections.
The repo also includes five purpose-specific dashboard pipelines that can run
immediately with `alpine:3.20` and no required variables, secrets, approvals, or
external MCP profiles. Two are technical operational checks
(`technical-api-readiness` and `technical-slo-burn-rate`), and three are
business-facing workflows (`customer-onboarding-pulse`,
`finance-close-snapshot`, and `people-capacity-plan`). Each emits structured
`dashboard_evidence=<json>` and publishes one dashboard final output to
`team-1/ops-dashboard`.

`pipelines/team-1/prompt-injection-guardrail-probe.yaml` is a governed LLM
pipeline for red-team testing. Its first step prepares a report directory and
then presents an admin-tagged prompt-injection payload as untrusted data; a
later script step verifies the generated probe artifact before normal
release-readiness LLM work continues. The pipeline attaches the managed
`guardrail/team-1/prompt-injection-safety` and
`policy/team-1/llm-task-integrity` Knowledge Context documents and uses no
external MCP profile, so it stays GitOps-compatible with the standard LLM and
Agent Profile registry.

## Global repo binding

```json
{
  "provider": "github",
  "repo_url": "https://github.com/acme/nopsai-global-config",
  "branch": "main",
  "base_path": "",
  "enabled": true,
  "write_enabled": true,
  "write_branch": "nopsai/ui-changes"
}
```

Create or update it through:

```bash
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"provider":"github","repo_url":"https://github.com/acme/nopsai-global-config","branch":"main","base_path":"","enabled":true,"write_enabled":true,"write_branch":"nopsai/ui-changes"}' \
  http://localhost:8080/v1/system/config-repo
```

`branch` is the GitOps sync source. `write_enabled` and `write_branch` let
Nopsai push generated GitOps changes to a review branch instead of writing
directly to the sync branch. `provider` can be `github`, `gitlab`, `bitbucket`,
or `gitea`. GitHub can use the existing GitHub App/git-bot path when
`credential_ref` is omitted; GitHub with `credential_ref`, GitLab, Bitbucket,
Cloud-compatible repositories, and Gitea use a `bearer_token` credential
reference for read and write access.

Team bindings use the same provider fields, either as standalone
`config-repositories/teams/<team>.yaml` files or inline `config:` blocks in a
team `structure.yaml`:

```yaml
config:
  provider: gitlab
  repo_url: https://gitlab.com/acme/platform/team-1-config.git
  credential_ref: credential://system/gitops/gitlab-platform
  branch: main
  base_path: ""
  enabled: true
  write_enabled: true
  write_branch: nopsai/team-1-ui
```

The drift endpoint compares Nopsai's current config with the sync branch before
you push:

```bash
curl http://localhost:8080/v1/system/config-repo/drift
```

Drift is bidirectional for syncable resources: Git-only changes appear as files
to import or delete, and UI-side changes appear as generated GitOps updates. The
check covers pipelines, reusable steps, schedules, trigger manifests, Git
webhook sources, scopes,
knowledge contexts, run team/config-repository structure, notification routes,
access manifests, Agent Profiles, LLM profiles, MCP registry files, auth
settings, mail settings, data cleanup schedules, runtime settings, team
defaults files, and encrypted credential envelopes.
Pipeline run records themselves are runtime audit state, so they are not
exported as Git-owned objects. For pipeline, reusable step, scope, and knowledge context Access dialog
changes, the generated diff updates the embedded `access:` block in that
resource file so the change can be pushed to the configured review branch.
After the pushed files are merged into the sync branch, config sync may adopt
matching database-owned resources inside the repo scope and flip them to GitOps
ownership. This keeps the handoff from UI-created config to Git-owned config
validated by the repository contents first.

The write endpoint accepts GitOps file paths relative to `base_path`:

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"message":"Add deploy pipeline","files":[{"path":"pipelines/services/api/deploy.yaml","content":"name: deploy\nsteps:\n  - name: deploy\n    script: echo deploy\n"}]}' \
  http://localhost:8080/v1/system/config-repo/write
```

## Syncable directories

Under the configured `base_path`, Nopsai scans:

```text
pipelines/             Pipeline definitions
dashboards/            Team dashboard definitions, sections, sources, and refresh schedules
dashboard-templates/   Reusable dashboard templates
steps/                 Reusable step definitions
schedules/             One-time and recurring pipeline schedules
triggers/              Trigger override manifests
external-triggers/     Authenticated external trigger endpoints
git-webhook-sources/   GitLab, Bitbucket, Gitea, and generic Git event sources
scopes/                Scope variable and secret key files
knowledge/             Managed knowledge context markdown documents
config-repositories/   Team config repo bindings, team structure, colocated notifications, and defaults
access/                Users, service accounts, advanced roles, policies, and basic role grants
setting/               System settings such as auth, mail, data cleanup, Agent Profiles, LLM, MCP, runtime settings, and encrypted credentials
```

Scope files use separate `variables:` and `secrets:` sections. Variables must be
declared under `variables:` as plain strings; flat top-level variable entries are
not supported. Variable and secret names must use runtime-safe names matching
`^[A-Za-z0-9_.-]+$`; repository-scoped entries use `owner/repo/NAME`, and team
repositories also accept canonical nested keys such as
`team-1/owner/repo/NAME`.
Secret entries may be `null` placeholders, or encrypted strings generated by
this Nopsai instance through the scope page GitOps encryption dialog or
`POST /v1/secrets/encrypt`. Invalid encrypted secret values are imported as keys
with no value.

Pipeline, reusable step, scope, and knowledge context files may also include an
`access:` block. That block maps to the same resource Access UI controls:
`visibility` controls Only this team / selected subjects /
Public, and `use_access` lists teams, repositories, or service accounts
that can use a restricted resource. When Access is changed in the UI, config
repository drift exports the current Access state back into these same embedded
blocks.

## Global repo file map

```text
global-repo/pipelines/platform/prod/platform-maintenance.yaml
  -> pipeline platform/prod/platform-maintenance, public use access

.nopsai/nopsai-platform-release.yaml
  -> pipeline platform/prod/nopsai-platform-release, repository-owned self-hosted release publication workflow for NopsAI images, image package metadata, Helm chart, CLI archives, changelog, and checksums

global-repo/pipelines/knowledge-kind-comparison.yaml
  -> pipeline knowledge-kind-comparison, comparing guardrail, policy, and guideline prompt behavior

team-1-repo/pipelines/team-1/prompt-injection-guardrail-probe.yaml
  -> pipeline team-1/prompt-injection-guardrail-probe, running a controlled prompt-injection guardrail probe before LLM release-readiness work

team-1-repo/pipelines/team-1/dashboard-sample.yaml
  -> pipeline team-1/dashboard-sample, publishing prompt-generated service metrics into team-1/ops-dashboard

team-1-repo/pipelines/team-1/dashboard-multi-output-sample.yaml
  -> pipeline team-1/dashboard-sample, publishing multiple dashboard outputs into team-1/ops-dashboard to exercise replace, append, snapshot, series, circular charts, boolean status chips, a narrative-first release report, and all dashboard presets

team-1-repo/pipelines/team-1/technical-api-readiness.yaml
  -> pipeline team-1/technical-api-readiness, publishing executable API release-readiness status into team-1/ops-dashboard

team-1-repo/pipelines/team-1/technical-slo-burn-rate.yaml
  -> pipeline team-1/technical-slo-burn-rate, publishing executable SLO burn-rate monitoring metrics into team-1/ops-dashboard

team-1-repo/pipelines/team-1/customer-onboarding-pulse.yaml
  -> pipeline team-1/customer-onboarding-pulse, publishing customer onboarding progress into team-1/ops-dashboard

team-1-repo/pipelines/team-1/finance-close-snapshot.yaml
  -> pipeline team-1/finance-close-snapshot, publishing a month-end close report into team-1/ops-dashboard

team-1-repo/pipelines/team-1/people-capacity-plan.yaml
  -> pipeline team-1/people-capacity-plan, publishing staffing capacity comparison into team-1/ops-dashboard

team-1-repo/dashboards/team-1/ops-dashboard.yaml
  -> dashboard team-1/ops-dashboard, with service metrics, technical readiness, image-build, release-readiness, customer success, finance, and people operations sources bound to executable sample pipelines

global-repo/steps/shared/announce.yaml
  -> reusable step shared/announce

global-repo/triggers/acme/service-api.yaml
  -> GitLab repository trigger acme/service-api, assigned to team-1 and gitlab-platform

global-repo/triggers/acme/deploy-webhook.yaml
  -> trigger override acme/deploy-webhook for the webhook-deployer service account sample

global-repo/triggers/nopsai/nopsai.yaml
  -> GitHub App repository trigger for self-hosted NopsAI release on push to main, assigned to platform/prod with scope prod

global-repo/external-triggers/deploy-prod.yaml
  -> authenticated external trigger for ServiceNow-style production deploy approvals,
     with invoked runs teamed under platform/prod

global-repo/git-webhook-sources/gitlab-platform.yaml
  -> workspace-shared GitLab source with owner path, credential reference,
     repository allowlist, connected trigger counts, and rate limit

global-repo/scopes/dev/scope.yaml
  -> variables and secret key placeholders in scope dev

global-repo/scopes/default/scope.yaml
  -> default-scope variables used by cross-scope pipeline variable examples

global-repo/scopes/prod/scope.yaml
  -> production variables, NopsAI release publisher variables/secrets, secret key placeholders, and restricted scope use access for servicenow-prod and nopsai-release-bot

global-repo/knowledge/guardrail/security/repo-check.md
  -> knowledge context guardrail/security/repo-check

global-repo/knowledge/guardrail/team-1/runtime-output-safety.md
  -> knowledge context guardrail/team-1/runtime-output-safety

global-repo/knowledge/guardrail/team-1/prompt-injection-safety.md
  -> knowledge context guardrail/team-1/prompt-injection-safety

global-repo/knowledge/policy/team-1/release-evidence.md
  -> knowledge context policy/team-1/release-evidence

global-repo/knowledge/policy/team-1/llm-task-integrity.md
  -> knowledge context policy/team-1/llm-task-integrity

global-repo/knowledge/guideline/team-1/pipeline-report-style.md
  -> knowledge context guideline/team-1/pipeline-report-style

global-repo/knowledge/architecture/team-1/backend.md
  -> knowledge context architecture/team-1/backend

global-repo/config-repositories/teams/team-2/platform.yaml
  -> config repo binding and team shell for team team-2/platform

global-repo/config-repositories/teams/team-1/structure.yaml
  -> Pipeline Runs team structure, apps with repository URLs under the team-1 team shell, and inline team config repo binding

global-repo/config-repositories/teams/data-team/structure.yaml
  -> Pipeline Runs team structure and inline team config repo binding for data-team

global-repo/config-repositories/teams/team-2/structure.yaml
  -> Pipeline Runs team structure and inline team config repo binding for the team-2 subtree

global-repo/config-repositories/teams/platform/structure.yaml
  -> Pipeline Runs team structure for platform automation

global-repo/access/*.yaml
  -> global users, service accounts, advanced roles, policies, advanced role bindings, and basic role grants

global-repo/access/service-accounts.yaml
  -> service account identities webhook-deployer, servicenow-prod, and nopsai-release-bot, plus scoped webhook grants and a least-privilege release pipeline runner role

global-repo/config-repositories/teams/team-2/notifications.yaml
  -> team notification policy with named routes for team-2 pipeline events

global-repo/setting/system/llm_profile.yaml
  -> system LLM profile registry

global-repo/setting/system/agent-profiles.yaml
  -> system Agent Profile persona registry and default profile setting

global-repo/setting/system/mcp.yaml
  -> system MCP server and profile registry

global-repo/setting/system/auth.yaml
  -> local-login and OIDC SSO settings

global-repo/setting/git-apps/github.yaml
  -> GitHub App ID, credential references, and installation records

global-repo/setting/system/runner.yaml
  -> runner install defaults, dispatcher runtime routing, and assistant settings

global-repo/setting/system/data-management.yaml
  -> scheduled cleanup definitions for system data management

global-repo/setting/system/mail.yaml
  -> SMTP mail notification settings with a password credential reference

global-repo/setting/system/credentials.example.yaml
  -> documentation-only example of encrypted credential envelopes; drift export writes the active credentials.yaml
```

The `webhook-deployer`, `servicenow-prod`, and `nopsai-release-bot` service
accounts are intentionally tokenless in Git. After sync, create or rotate their `nopsat_` tokens from System Access or
`POST /v1/admin/service-accounts/{id}/tokens`, then use that token for
integration API calls such as starting `platform/prod/platform-maintenance` in scope `dev`.
The paired `triggers/acme/deploy-webhook.yaml` file shows the GitHub webhook
trigger that maps `acme/deploy-webhook` events to the same pipeline and scope.
The paired `external-triggers/deploy-prod.yaml` file shows the enterprise path:
`servicenow-prod` has an advanced role that can invoke `deploy-prod`, execute
and use `platform/prod/platform-maintenance`; `scopes/prod/scope.yaml` shares restricted
`scope.use` access with that service account.
The `triggers/nopsai/nopsai.yaml` file is the native GitHub App
self-release trigger. It starts `platform/prod/nopsai-platform-release` when the
GitHub App receives a `push` event for `main`; it intentionally sets
`provider: github`, `team_path: platform/prod`, and `management: nopsai`, with
no `webhook_source` because GitHub App ingress is automatic. The release
pipeline in `.nopsai/nopsai-platform-release.yaml` and `prod` scope both grant
use access to `repository:nopsai/nopsai` so the GitHub-triggered
run can pass runtime authorization. Release inputs come
from `scopes/prod/scope.yaml`, including the repository URL, source ref, shared
GHCR package root, image platforms, and Docker host. GitHub and GHCR token values
stay as secret placeholders.
If a service account is first created in the UI or API, config repository drift
can export the identity and service-account product grants back to
`access/service-accounts.yaml` for review-branch push. Token values remain local
runtime secrets and are not exported.

The GitLab source references
`credential://system/webhooks/gitlab-platform`. Config sync creates pending
credential metadata when needed. Store the encrypted token envelope through
**Credentials** or by committing a real
`setting/system/credentials.yaml` exported from this NopsAI instance before
enabling provider delivery.

## SSO settings

A system/global config repo can define local-login and OIDC SSO settings in
`setting/system/auth.yaml`. The same auth fields are also accepted under a
wrapped `auth:` key for migration from `config.yml`, but the canonical GitOps
shape is top-level:

```yaml
local_enabled: true
oidc:
  enabled: true
  auto_create_users: true
  default_role: ""
  domain_mapping:
    example.com: nopsai
  providers:
    nopsai:
      type: oidc
      display_name: Enterprise SSO
      issuer: https://sso.example.com/realms/nopsai
      client_id: nopsai
      client_credential_ref: credential://system/oidc/nopsai/client-secret
      scopes: ["openid", "email", "profile"]
      allowed_email_domains: ["example.com"]
```

GitOps feature files store credential references, not provider plaintext.
Create referenced values in **Credentials** or sync encrypted versions
from `setting/system/credentials.yaml`; sync creates missing metadata in
`pending` state when no encrypted value is present.

SSO-managed users are intentionally excluded from access GitOps. Export and
drift skip linked OIDC users, raw `oidc:*` user subjects, and provider-managed
basic role grants because those identities and entitlements belong to the
identity provider. Use access manifests for local users, service accounts, and
local grants; use `setting/system/auth.yaml` for the SSO provider settings.

## Runtime settings

A system/global config repo can define runner and dispatcher runtime settings in
one file. The canonical path is `setting/system/runner.yaml`; `runner.yaml` is
the only accepted settings file name for this runner/dispatcher GitOps surface.

```yaml
dispatcher_grpc_address: dispatcher:9090

# Defaults used when generating a new runner install command
runner_id: runner-general
runner_scopes: dev,prod
runner_capacity: 2
runtime: docker
runtime_output_max_bytes: 65536

assistant:
  enabled: true
  provider: gemini
  model: gemini-2.5-pro
  credential_ref: credential://system/assistant/api-key
  timeout: 60s
  default_docs_version: auto
  conversation_retention_days: 30
  max_input_logs_bytes: 120000
  max_conversation_turns: 30
  docs_enabled: true
  docs_version_aware: true
  mcp:
    enabled: true
    server_url: ""
  features:
    docs: true
    pipeline_debugging: true
    config_generation: true
    statistics_insights: true
    maintenance_recommendations: true
    cost_recommendations: true
    action_execution: false
  actions:
    require_confirmation: true
  memory:
    enabled: true
    scope: conversation

# Optional hard routing by scope to runner IDs
dispatcher_routing:
  prod:
    - runner-prod-1
    - runner-prod-2
  dev:
    - runner-dev-1
  "*":
    - runner-general

# Optional runner-to-registry assignments. Values stay in Credentials.
runner_registry_credentials:
  runner-prod-1:
    - credential://system/registry/production-ghcr
  k8s-runner-prod:
    - credential://system/registry/production-ghcr
```

`runner_id`, `runner_scopes`, and `runner_capacity` are defaults used by the UI
when it generates a new runner install command. Individual runner deployments
can still use their own environment values. An empty `runner_scopes` value means
the runner can accept every scope.

`dispatcher_routing` is an extra allow-list by scope. A run can be assigned to a
runner only when the runner's declared scopes include the run scope and, when a
matching routing entry exists, that runner ID is listed. The `*` entry applies
alongside every scope-specific route and also covers scopes without an explicit
entry. Changes to `dispatcher_routing` are written to runtime config and exposed
through the protected internal control-plane endpoint that the dispatcher polls,
so new scheduling decisions can use the updated table without a restart.
`runner_registry_credentials` assigns runner IDs to `docker_config_json`
credential references for private registry pulls. GitOps stores only references;
encrypted Docker config values remain in `setting/system/credentials.yaml` or
the database-managed Credentials page. Docker bootstrap installs a local
env-carried Docker config for runner and agent Docker API pulls, while
Kubernetes runners use ServiceAccount `imagePullSecrets` created by bootstrap
or by cluster infrastructure.
Runtime settings are persisted in the database when synced. `config.yml`,
`.env`, Docker Compose, and deployment secrets are bootstrap inputs only. On
NopsAI restart, the persisted database snapshot is loaded before connecting to
the dispatcher, so a GitOps-synced runner file remains effective without
manually syncing again. Services can read versioned snapshots from
`/internal/v1/runtime-config/{service}` and long-poll
`/internal/v1/runtime-config/{service}/watch?version=<n>`.
`runtime_output_max_bytes` applies to each declared task runtime output file
written under `/nopsai/outputs` and is passed to new agent runs through
`NOPSAI_RUNTIME_OUTPUT_MAX_BYTES`.

## GitHub App settings

A system/global config repo can define GitHub App settings in
`setting/git-apps/github.yaml`:

```yaml
provider: github
app_id: "123456"
private_key_credential_ref: credential://system/github/app-private-key
webhook_credential_ref: credential://system/github/webhook-secret
installations:
  - installation_id: "987654"
    account_login: nopsai
    account_type: organization
    enabled: true
```

The GitHub file stores only app-scoped IDs and credential references. Store
encrypted private-key and webhook-secret versions in
`setting/system/credentials.yaml` or through **Credentials**. Keep
`git_bot_api_url` in System Config or service configuration. The legacy
`setting/system/github.yaml` path is read for migration compatibility for one
release, but exports write `setting/git-apps/github.yaml`.

Keep bootstrap values out of GitOps. Database URLs, master keys, and service JWT
signing keys stay in deployment secrets. Operational integration credentials
use the encrypted registry, are referenced from feature GitOps files, and can be
stored as encrypted envelopes in `setting/system/credentials.yaml`.

System mail notification settings live in `setting/system/mail.yaml`. The SMTP
password plaintext is not stored in the mail file; `smtp.password_credential_ref`
names an encrypted registry entry that can be managed through
`setting/system/credentials.yaml`.

```yaml
enabled: true
from: nopsai@example.com
smtp:
  host: smtp.example.com
  port: 587
  start_tls: true
  username: nopsai@example.com
  password_credential_ref: credential://system/mail/smtp-primary
```

When the global repo defines team bindings under `config-repositories/teams`,
those bindings create the team shells. Put app placement next to those bindings
in scoped files such as `config-repositories/teams/team-1/structure.yaml`.
A team node can include `apps:` entries with `name` and `repo_url`, plus
`config:` with the same fields as a standalone binding file.

## Team Repo File Map

When `team-1-repo` is bound to team `team-1`, NopsAI prefixes synced
resources with `team-1`. The closest enabled team config repository is the
authority for resources under its team path, including generated drift for
GitOps-labeled resources that no longer have active config-repository ownership.
Drift also rewrites legacy managed source paths to the canonical explicit-team
layout, for example from legacy shorthand `knowledge/guideline/go-style.md` to
explicit `knowledge/guideline/team-1/go-style.md` inside the `team-1` repo:

```text
team-1-repo/pipelines/team-1/build-and-test.yaml
  -> pipeline team-1/build-and-test with restricted use access

team-1-repo/pipelines/team-1/variable-feature-exercise.yaml
  -> pipeline team-1/variable-feature-exercise exercising bare scoped variables,
     explicit default/team refs, step/task overrides, reusable-step variable
     merge, child-pipeline include variables, and file-based runtime outputs

team-1-repo/pipelines/team-1/variable-feature-child.yaml
  -> child pipeline used by team-1/variable-feature-exercise

team-1-repo/pipelines/team-1/services/api/deploy.yaml
  -> pipeline team-1/services/api/deploy

team-1-repo/schedules/team-1/prod/scheduled/nightly-api-deploy.yaml
  -> schedule team-1/prod/scheduled/nightly-api-deploy, targeting team-1/services/api/deploy,
     with runs teamed under team-1 for notification routing

team-1-repo/schedules/team-1/prod/scheduled/release-window.yaml
  -> one-time schedule team-1/prod/scheduled/release-window, targeting team-1/services/api/deploy,
     with runs routed under team-1 for notification routing

team-1-repo/steps/team-1/shared/checkout.yaml
  -> reusable step team-1/shared/checkout

team-1-repo/steps/team-1/shared/variable-defaults.yaml
  -> reusable step team-1/shared/variable-defaults with default variables
     overridden by team-1/variable-feature-exercise

team-1-repo/triggers/team-1/service-api.yaml
  -> trigger override team-1/service-api

team-1-repo/external-triggers/team-1-deploy-prod.yaml
  -> external trigger team-1-deploy-prod, with repo-relative pipeline/scope/run team references normalized under team-1

team-1-repo/git-webhook-sources/team-1-gitlab-main.yaml
  -> Git webhook source team-1-gitlab-main, defaulting team_path to team-1

team-1-repo/config-repositories/teams/team-1/notifications.yaml
  -> notification policy with named routes for team team-1, owned by the delegated team repo

team-1-repo/scopes/team-1/dev/scope.yaml
  -> variables and secret key placeholders in scope team-1/dev

team-1-repo/scopes/team-1/prod/scope.yaml
  -> variables and secret key placeholders in scope team-1/prod with restricted scope use access

team-1-repo/knowledge/runbook/team-1/deploy/api.md
  -> knowledge context runbook/team-1/deploy/api

team-1-repo/knowledge/guideline/team-1/go-style.md
  -> knowledge context guideline/team-1/go-style

team-1-repo/access/*.yaml
  -> basic role grants scoped to team-1
```

Pipeline and step file names must match their `name` fields. Team repo trigger
manifests, schedules, and includes should reference the final team-prefixed IDs
or repo-relative IDs that sync can normalize under the bound team. A
`prod/scheduled` schedule team path is useful for presentation, while
`run_team_path` controls where scheduled runs appear and which notification
routes receive their events. Use `run_team_path: root` to keep runs at the
Pipeline Runs root without assigning them to a team. Pipeline schedules are
created with team-only visibility by default; GitOps schedule files do not need a
`visibility` field.
Repository-triggered runs that do not set an explicit run team are assigned to
an existing matching repository/application owner when one exists. Runtime
ingestion does not create or rewrite team/application records; unmatched
repository runs stay unassigned until a team/application owner is configured.

Team notification policies control who receives pipeline event notifications for
a run team. A system/global repo can define policies at
`config-repositories/teams/<team>/notifications.yaml` beside that team's
structure file. A delegated team repo uses the same explicit
`config-repositories/teams/<team>/notifications.yaml` path for the team it owns.
Each file can contain one or more named `routes`, so teams can
split failure, approval, and success notifications without creating competing
GitOps files for the same team. Recipients can include direct users, teams, and
the reserved `same_team` team. Exclusions are applied after includes. Event keys
support failure, success, pending, running, waiting_approval, approval_requested,
approval_approved, approval_rejected, cancelled, and skipped. Branch, pipeline,
and repository filters use glob-style patterns, and delivery currently supports
the `mail` channel. Policies apply to their team subtree, with the nearest
policy in the run team's ancestry taking precedence. Schedules and external
triggers can set `run_team_path` from the Teams hierarchy when their run events
should be routed to a notification team that differs from the target pipeline's
team. The reserved `root` value always means the Pipeline Runs root, not a
team named `root`.

Nopsai reads every `.yaml` and `.yml` file under `access/`; file names such as
`all.yaml` or `grants.yaml` are only examples, so teams can split manifests by
owner, environment, or workflow.

Global config repos may manage basic role grants even when the target team has
its own delegated config repo. Team-owned config repos may manage basic role
grants for their own team subtree. User, advanced-role, policy, and direct
advanced-role-binding management remain global-repo only. In team repos, grant
resource IDs are prefixed with the bound team automatically, so
`resource: team:dev` in the `team-1` repo targets `team:team-1/dev`,
`resource: knowledge_context:guideline/go-style` targets
`knowledge_context:guideline/team-1/go-style`, and
`resource: knowledge_connection:notion-main` targets
`knowledge_connection:team-1/notion-main`.

User `advanced_roles` are global access-role assignments and may reference
custom roles or protected built-in bundles such as `viewer`, `developer`,
`owner`, and `admin`. Use `basic_roles` when those same product role names
should be scoped to a team target. Prefer `user: alice` and
`service_account: webhook-deployer` in `basic_roles`; drift exports those
shorthands and sync resolves them to the canonical runtime subject IDs. GitOps
basic roles may point at pipelines, triggers, or scopes that are created later
by a delegated team repo during the same sync-all run. Global drift still
exports product `basic_roles` for delegated teams from `access/all.yaml` or
`access/service-accounts.yaml`; embedded resource access is exported with the
repo that owns the resource file.

## Embedded resource access

Use embedded `access:` for the per-object Access dialog settings:

```yaml
name: deploy
access:
  visibility: restricted # team, restricted, or public/workspace
  use_access:
    grants:
      - subject_type: repository
        subject_id: nopsai/test-app
      - subject_type: team
        subject_id: data-team
steps:
  - name: deploy
    script: echo deploy
```

For the common UI subjects, the shorter form is also accepted:

```yaml
access:
  teams: [data-team]
  repositories: [nopsai/test-app]
```

When grants are present and `visibility` is omitted, Nopsai treats the resource
as `restricted`. Scopes are sensitive, so they support `team` and `restricted`
visibility only; `public` is accepted for pipelines, reusable steps, and
knowledge contexts.

## Knowledge context documents

Managed knowledge context files live under `knowledge/<kind>/<team>/<name>.md`
or `.yaml`/`.yml`.
Supported kinds are `architecture`, `guardrail`, `policy`, `adr`,
`guideline`, `runbook`, `reference`, and `example`.

```markdown
---
name: repo-check
kind: guardrail
access:
  visibility: restricted
  repositories:
    - nopsai/test-app
content: |
  # Repository Check Guardrail

  - Do not expose secrets in logs.
---
```

Pipeline YAML references this document with:

```yaml
knowledge_context:
  - kind: guardrail
    ref: security/repo-check
    required: true
```

In a team-scoped config repository, the document team is normalized under the
bound team, but the canonical path still keeps that team explicit.
`knowledge/guideline/team-1/go-style.md` in the `team-1` repo becomes
`guideline/team-1/go-style`, and `knowledge/runbook/team-1/deploy/api.md`
becomes `runbook/team-1/deploy/api`. Legacy shorthand such as
`knowledge/guideline/go-style.md` is still imported for compatibility, and drift
will propose moving it to `knowledge/guideline/team-1/go-style.md`.
