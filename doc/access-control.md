# NopsAI Access Control

This document describes the current AAA-backed authorization model.

## Overview

NopsAI now has a dedicated AAA service plus an in-process fallback evaluator inside `services/nopsai`.

- `services/aaa` owns the HTTP authorization service, the policy-store schema, and the table-specific mutation helpers for roles, bindings, permissions, and ACL rows.
- `services/nopsai` authenticates users, maps API routes to low-level actions, calls AAA, exposes product-facing access-grant APIs, and performs runtime resource-use checks before pipeline execution.
- Product roles are stored by `nopsai` as user-friendly `access_grants`, then expanded through AAA-owned policy writer helpers instead of direct product-service SQL against AAA ACL tables.
- The evaluator stays generic: it checks roles, direct ACLs, auth-team membership, resource inheritance, and deny-before-allow rules.

## Ownership Boundary

`nopsai` owns product intent and UX-facing records: users, service-account credentials, `access_grants`, `resource_ownership`, visibility settings, and GitOps manifests. `aaa` owns the low-level policy representation used by the evaluator: `auth_roles`, `auth_role_bindings`, `auth_role_permissions`, `resource_acl`, auth-team policy membership, and authorization decision logs. When product workflows need to seed roles, bind an admin role, or expand a product grant into ACL rows, they call `services/aaa/pkg/store` helpers so schema-specific writes stay in the AAA package.

## Request Authorization Flow

1. `services/nopsai/pkg/auth` validates the bearer token and places auth claims in the request context.
2. `services/nopsai/pkg/routeauthz` maps the HTTP route to an action/resource pair such as `pipeline.update` on `pipeline:team/build`.

System Logs uses the dedicated `system_log.read` action on `system_log:<sourceID>`. Source discovery is filtered per resource, so a role can grant one service without exposing the remaining platform source catalog. Stream audit events record actor, source, open/close, and result but never log content.
3. `nopsai` builds an AAA subject:
   - normal callers become `user` subjects based on JWT `sub` and `email`
   - dispatcher internal calls become `internal_service:dispatcher`
4. `nopsai` calls the AAA service through `services/nopsai/pkg/aaaclient`.
5. If the AAA service is temporarily unavailable, `nopsai` falls back for 10 seconds to a local evaluator backed by the same Postgres store.
6. Allowed requests continue to the handler; denied requests return `403`.

Routes that need response-level filtering, such as list views, are authorized inside handlers with AAA `Filter`.

## AAA Service Contract

The standalone AAA service listens on `AAA_LISTEN_ADDRESS`, defaulting to
`:8082`. Docker Compose exposes it only inside the compose network as
`aaa:8082`.

The `nopsai` CLI is an ordinary API client. It accepts user access JWTs,
personal access tokens, and service-account tokens, sends them as
`Authorization: Bearer`, and never calls AAA directly or bypasses route
authorization. `platform doctor` reports a rejected token as an error and a
missing dispatcher-read grant as a warning. CLI requests continue to produce
the same API and AAA audit decisions as browser or automation requests.

`nopsai api routes` includes public, operator, and explicitly internal route
metadata for complete discovery. Catalog visibility grants no access: `api
call` and `api request` still traverse bearer authentication, route AAA, and
resource authorization. `--no-auth` suppresses local token loading and is
intended for routes declared public by the API; it does not make protected
routes public. Internal routes require the same internal service identity as
direct HTTP calls.

Public endpoint:

- `GET /healthz`
- `GET /version` exposes only immutable build, API compatibility, capability,
  and optional legacy release-manifest identity. It is available during setup preflight and
  does not expose environment state or credentials.

Internal endpoints require the `X-Internal-Token` header, configured with `AAA_SHARED_INTERNAL_TOKEN`:

- `POST /v1/authn/introspect`
- `POST /v1/authz/check`
- `POST /v1/authz/batch-check`
- `POST /v1/authz/filter`
- `POST /v1/audit/record`

`nopsai` reaches this service through `AAA_API_URL`, defaulting to `http://aaa:8082`.

## Product Roles

The product roles are templates seeded by `nopsai` startup:

- `viewer`: read/list access for teams, pipelines, schedules, dashboards, runs, logs, triggers, Git webhook sources, repositories, steps, scopes, knowledge contexts, credential metadata, secret metadata, variable metadata, and config repository metadata.
- `developer`: includes all viewer access plus non-destructive creation, updates, pipeline and schedule execution, dashboard publication/source management, `*.use` runtime permissions, rerun/cancel, trigger and Git webhook source updates, credential creation/rotation/use, secret writes, variable writes, repository updates, scope updates, reusable step usage, knowledge context usage, runner usage, and config repository usage.
- `owner`: includes all developer and viewer access plus all scoped non-admin actions, deletes, credential ACL management, secret and variable value reads, ownership, and ACL management inside the owned scope.
- `admin`: platform-wide access through the normal AAA `Check` path.

The `admin` role can only be granted on the `platform` resource. Team grants must inherit.

## Subjects And Resources

Supported grant subjects:

- `user`
- `auth_team`
- `repository`
- `trigger`
- `external_trigger`
- `git_webhook_source`
- `service_account` for scheduled-run identities and administrator-created integration accounts
- `internal_service`

Supported grant resources:

- `team`
- `pipeline`
- `pipeline_schedule`
- `dashboard`
- `pipeline_run`
- `trigger`
- `secret`
- `variable`
- `scope`
- `repository`
- `step`
- `knowledge_context`
- `credential`
- `runner`
- `config_repo`
- `platform`

Team grant requests use the internal `team` resource type and may use paths with a leading slash, such as `/payments/backend`; the stored internal ID is normalized without the leading slash. Use `root` for the root team scope; new teams cannot be named `root`.

Named secret and variable resources use query-style internal IDs built from repository, scope, and name. The public grant API accepts the same logical IDs shown in the UI.

Runtime resource-sharing grants use a separate `team` subject type for existing team paths. That `team` subject is only used by the resource Access UI/API to share a resource with a team path; it is not the same as an AAA `auth_team`.

Pipeline schedules use `pipeline_schedule` as the resource type. `viewer`
grants include `pipeline_schedule.list` and `pipeline_schedule.read`;
`developer` grants add `pipeline_schedule.create`, `pipeline_schedule.update`,
and `pipeline_schedule.execute`; `owner` grants add
`pipeline_schedule.delete` and `pipeline_schedule.manage_acl`.

Dashboards use `dashboard` as the resource type. Viewer grants include
`dashboard.list` and `dashboard.read`; developer grants add
`dashboard.create`, `dashboard.update`, `dashboard.publish`,
`dashboard.refresh`, and `dashboard.manage_sources`; owner grants add
`dashboard.delete` and `dashboard.manage_acl`. Refresh routes use
`dashboard.refresh` for start, cancel, and retry; refresh history reads use
`dashboard.read`. Dashboard refresh schedule reads use `dashboard.read`,
schedule create/update/delete/enable/disable use `dashboard.update`, and
schedule run-now uses `dashboard.refresh`. The dashboard creation UI assigns
the dashboard to an existing team and leaves broader sharing to Access grants,
matching pipeline access-management behavior.

Git Webhook Sources use `git_webhook_source`. Viewer grants add
`git_webhook_source.read`; developer grants add create and update; owner grants
add delete and `git_webhook_source.manage_acl`. The unauthenticated delivery
route is protected by the source credential, repository allowlist, payload
limit, idempotency, and rate limit rather than a user bearer token.

Credentials use the first-class `credential` resource. Resource IDs are the
stable credential reference path without the `credential://` prefix, such as
`system/llm/openai`. Viewer grants can see metadata, developer grants can
create/rotate/use credentials, and owner grants can manage lifecycle and
`credential.manage_acl`. Credential values remain write-only; there is no
human-facing value-read action.

Personal access tokens belong to the signed-in user and inherit that user's
current authorization. Local and OIDC/SSO browser sessions can create, list, and
revoke their own personal tokens from Profile or `/v1/auth/personal-tokens`.
Long-lived bearer tokens such as personal tokens, service-account tokens, and
internal service tokens cannot create more personal tokens.

## Grant Lifecycle

Product grants are managed through `nopsai`:

- `POST /v1/access/grants`
- `GET /v1/access/grants`
- `DELETE /v1/access/grants/{grantID}`
- `GET /v1/access/effective-permissions`

When a grant is created:

1. `nopsai` resolves the target subject and resource.
2. It checks whether the caller can manage ACLs for that resource, or has `iam.admin` for platform/admin grants.
3. It inserts an `access_grants` row.
4. It asks the AAA policy writer to expand the product role into `resource_acl` rows.
5. Owner grants also write `resource_ownership`.
6. Admin grants ask the AAA policy writer to bind `auth_role_bindings` for the platform role instead of resource ACLs.

Deleting a grant removes its expanded ACL and ownership rows through the grant foreign key.

## Runtime Resource Use Authorization

Pipeline execution uses a second authorization pass for resources that a run wants to use. This pass is centralized in `AuthorizeResourceUse` and is intentionally caller-based:

- manual Lab/API runs use the authenticated `user`
- Git-triggered runs use `repository:<owner>/<repo>`
- scheduled runs use `service_account:schedule:<schedule-id>` with explicit runtime grants derived from the schedule target pipeline
- other automation runs should use the trigger identity or an administrator-created service account token
- dispatcher/internal calls only execute after the original caller has already been authorized

Pipeline runs inherit from their pipeline path, scope path, repository metadata, and, when present, the app/team selected by `pipeline_runs.team_id`. This lets an owner of `team:team-1` see runs placed under `team-1/test-app`, including runs whose Git metadata resolves through the app's `repository_full_name`.

Supported low-level use actions include:

- `pipeline.use`
- `scope.use`
- `step.use`
- `secret.use`
- `variable.use`
- `runner.use`
- `config_repo.use`
- `knowledge_context.use`
- `credential.use`

The main rule is that same-team resources remain naturally available according to visibility. Cross-team use requires an explicit resource-use grant or public visibility.

Approval visibility is intentionally narrower than normal run ownership. A pending approval run can appear for a user with `approval.approve` on at least one assigned approval team, subject to `allow_self_approval`, so approvers can make the decision without receiving unrelated pipeline or log permissions.

Resource visibility values:

- `team`: default; usable inside the same team boundary
- `restricted`: same team still works, and selected teams, repositories, or service accounts can also be granted use access
- `workspace`: shown in the UI as `Public`; usable by authorized callers outside the team, but still does not grant access to scopes, secrets, variables, or runners

Current resource Access UI behavior:

- Pipeline, dashboard, step, scope, and knowledge context pages show an `Access` button next to the normal action buttons.
- The Access dialog offers `Only this team`, `This team and selected subjects`, and, for non-sensitive resources, `Public`.
- Dashboard grants use `dashboard.read`; other resource-use grants use their resource-specific `*.use` action.
- Team sharing uses existing Teams entries from `GET /v1/access/teams`.
- Team dropdowns in Access, resource creation, monitoring, and resource
  configuration surfaces list team paths only; application/repository nodes are
  reserved for the Teams and run-navigation resource trees.
- Auth-team subject pickers use SSO/AAA entries from `GET /v1/access/auth-teams`.
- Repository sharing accepts canonical repository IDs such as `hosein-yousefii/test-app`.
- Sensitive resources such as scopes do not expose `Public`.
- The default scope is addressed as `scope:default` in the API/UI. Secret and variable rows store it only as `scope = 'default'`; runtime lookups do not fall back to `NULL` or empty scope values.

Resource Access API:

- `GET /v1/resources/{type}/{id}/access`
- `PUT /v1/resources/{type}/{id}/access`
- `POST /v1/resources/{type}/{id}/grants`
- `DELETE /v1/resources/{type}/{id}/grants/{grantID}`
- `POST /v1/authz/resource-use/check`
- `POST /v1/authz/resource-use/batch-check`

Resource-use grant requests accept `subject_type: "repository"` with `subject_id: "owner/repo"` or `subject_type: "team"` with a team path such as `team-1`. Team use grants are stored in `access_grants` for audit and UI display; they do not write broad ACL rows that would elevate every member of the team. At check time, the original caller must resolve into the granted team boundary.

`pipeline_runs` stores the run authorization context in `trigger_source`, `requested_by_type`, `requested_by_id`, `effective_subject_type`, `effective_subject_id`, and `authorization_snapshot`. The snapshot records the caller and the resource-use checks that were allowed for the created run.

## GitOps Access Manifests

Config repositories can manage access records from YAML under `access/`.
Every `*.yaml` or `*.yml` file under that directory is read; `all.yaml` is only
a sample name. Manifests may use top-level `users`, `service_accounts`,
`advanced_roles`, `policies`, `advanced_role_bindings`, and `basic_roles` keys.
GitOps creates service-account identities and grants, but it does not store raw
service-account tokens. IAM administrators create or rotate those runtime tokens
through the System Access page or service-account token API after sync.

Example global manifest:

```yaml
users:
  - sub: alice
    email: alice@example.com
    advanced_roles: [release-manager]
  - sub: bob
    email: bob@example.com
    advanced_roles: [viewer]

service_accounts:
  - sub: webhook-deployer
    email: webhook-deployer@example.com

advanced_roles:
  - name: release-manager
    policies:
      - resource: pipeline:team-1/*
        action: pipeline.execute

basic_roles:
  - user: alice
    role: owner
    resource: team:team-1
  - subject_type: service_account
    subject_id: webhook-deployer
    role: developer
    resource: pipeline:platform-maintenance
```

Global config repositories can manage users, service accounts, advanced role
definitions, policies, advanced role bindings, and basic role grants, including
grants that target delegated teams. Team-scoped config repositories
can manage `basic_roles` only for their own team subtree; user,
service-account, advanced-role, policy, and direct advanced-role-binding
management remains global.
In a team repo, grant resource IDs are normalized under the bound team, so a
grant with `resource_type: team` in the `team-1` repo targets `team:team-1`.

User- and service-account-level `advanced_roles` assignments can reference
custom roles from the manifest or protected built-in role bundles such as
`viewer`, `developer`, `owner`, and `admin`. These assignments are global
access-role bindings. In SSO, direct Keycloak client roles on the NopsAI client
map to this same global lane. Use `basic_roles` when the same product role name
should be scoped to a team target.

GitOps `basic_roles` use the same product roles as the API: `viewer`,
`developer`, `owner`, and `admin`. The team is the grant target, not a
separate subject type. Non-admin basic roles are expanded through AAA-owned
policy helpers into `resource_acl`; owner grants also write
`resource_ownership`. `admin` grants remain
platform-only and are rejected in team-scoped config repositories.
OIDC providers can also sync scoped basic roles directly from SSO teams with
`entitlement_sync.mode: keycloak_team_roles`. In that mode, direct Keycloak
client roles become global access roles, while Keycloak team client roles
become scoped Basic roles. For example, Keycloak team `/team-1` with client
role `owner` becomes a provider-managed `owner` grant on `team:team-1`.
NopsAI reconciles those provider-managed grants on OIDC login, entitlement
worker startup, and periodic worker runs. SSO-sourced team grants can be
stored before the team exists, which keeps Keycloak and GitOps rollout order
flexible.
Use the shorthand subject fields `user:`, `service_account:`, or `service:` for
editable GitOps manifests. The canonical `subject_type` plus `subject_id` form
is still accepted for compatibility. Sync resolves users and service accounts to
their canonical runtime IDs before writing grants; drift exports users by `sub`
and service accounts by `sub`.
GitOps access sync accepts resource IDs declared by the repository even when the
target pipeline, trigger, or scope is created later in the same sync-all run by
a delegated team repository. Direct UI/API grant creation still requires the
target resource to exist. Global drift keeps exporting product `basic_roles`
for users and service accounts even when the target team also has a delegated
config repository; embedded resource access remains owned by the repository that
owns the resource file.

Config repository drift can export existing users, service-account identities,
advanced roles, policies, advanced role bindings, and basic product grants back
into access manifests so they can be pushed to the review branch. Service
account identities and service-account basic grants are written to
`access/service-accounts.yaml`;
other global IAM records are written to `access/all.yaml`, and team-scoped
repos write scoped grants to `access/grants.yaml`. Exported files include
identity metadata and roles only; passwords and `nopsat_` token values are never
exported.

## GitOps Resource Access

Pipeline, dashboard, reusable step, scope, and knowledge context config files can also
declare resource use access inline with the object they protect. This is the
GitOps form of the resource Access dialog in the UI.

```yaml
name: deploy
access:
  visibility: restricted
  use_access:
    grants:
      - subject_type: repository
        subject_id: hosein-yousefii/test-app
      - subject_type: team
        subject_id: data-team
steps:
  - name: deploy
    script: echo deploy
```

Supported `visibility` values are `team`, `restricted`, and
`workspace`/`public`. `public` is only allowed for non-sensitive resources such
as pipelines, reusable steps, and knowledge contexts; scopes remain sensitive
and can only be `team` or `restricted`. If a resource declares grants without a
visibility, sync treats it as `restricted`. Dashboard use-access grants expand
to `dashboard.read`.

The grant subjects match the Access UI. Use `repository:` with a canonical
repository ID, `service_account:` with a service-account sub, or `team:` with a
team path. The canonical `subject_type` plus `subject_id` form is
also accepted.

```yaml
access:
  teams: [data-team]
  repositories: [hosein-yousefii/test-app]
  use_access:
    grants:
      - service_account: webhook-deployer
```

Inline access grants are stored in the same `access_grants` table as UI-created
resource-use grants and are reconciled on config sync. The `access/` directory
remains for IAM-like records: users, advanced roles, policies, advanced role
bindings, and scoped product role grants.

Config repository drift exports the current resource Access state for pipelines,
dashboards, reusable steps, scopes, and knowledge contexts. If access is
changed in the UI, the drift response marks the owning GitOps resource file as
modified and the write endpoint can push the updated embedded `access:` block
to the review branch.

## Inheritance

The evaluator resolves parent resources before checking ACLs. A team grant can authorize:

- child teams
- pipelines and runs under the team path
- pipeline schedules under the team path
- repositories assigned under the team path
- runs associated with repositories under the team path
- triggers for inherited repositories
- scoped secrets and variables
- reusable steps under the team path
- knowledge contexts under the team path

Specific deny policies still win before inherited allow policies.

## Audit Behavior

Authorization decisions are written to `authz_decision_logs` when:

- the decision is denied
- the decision is allowed for a sensitive action

Sensitive actions include ACL management, admin/system operations, pipeline execution or deletion, run rerun/cancel/delete, trigger writes, secret value access, variable writes/deletes, team moves, repository deletes, step deletes, and knowledge context use/delete/manage-access operations.

The normal request audit trail still writes to `audit_logs`.

## Bootstrap

Startup seeds:

- the local bootstrap admin user with role `nopsai-admin`; generated installs
  create it from `NOPSAI_BOOTSTRAP_ADMIN_EMAIL` and
  `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD(_FILE)`
- `nopsai-admin` with `*` permissions
- `dispatcher-internal` bound to `internal_service:dispatcher`
- dispatcher permissions for pipeline fetches, execution, run status, logs, finalization, and task updates
- product role templates for `viewer`, `developer`, `owner`, and `admin`

The bootstrap admin role assignment is protected from accidental removal or deactivation through the admin APIs.
