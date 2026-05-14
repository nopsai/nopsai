# NopsAI Access Control

This document describes the current AAA-backed authorization model.

## Overview

NopsAI now has a dedicated AAA service plus an in-process fallback evaluator inside `services/nopsai`.

- `services/aaa` owns the HTTP authorization service and uses Postgres as its policy store.
- `services/nopsai` authenticates users, maps API routes to low-level actions, calls AAA, exposes product-facing access-grant APIs, and performs runtime resource-use checks before pipeline execution.
- Product roles are stored as user-friendly grants, then expanded into low-level AAA ACL rows at grant time.
- The evaluator stays generic: it checks roles, direct ACLs, auth-group membership, resource inheritance, and deny-before-allow rules.

## Request Authorization Flow

1. `services/nopsai/pkg/auth` validates the bearer token and places auth claims in the request context.
2. `services/nopsai/pkg/routeauthz` maps the HTTP route to an action/resource pair such as `pipeline.update` on `pipeline:team/build`.
3. `nopsai` builds an AAA subject:
   - normal callers become `user` subjects based on JWT `sub` and `email`
   - dispatcher internal calls become `internal_service:dispatcher`
4. `nopsai` calls the AAA service through `services/nopsai/pkg/aaaclient`.
5. If the AAA service is temporarily unavailable, `nopsai` falls back for 10 seconds to a local evaluator backed by the same Postgres store.
6. Allowed requests continue to the handler; denied requests return `403`.

Routes that need response-level filtering, such as list views, are authorized inside handlers with AAA `Filter`.

## AAA Service Contract

The standalone AAA service listens on `AAA_ADDR`, defaulting to `:8082`. Docker Compose exposes it only inside the compose network as `aaa:8082`.

Public endpoint:

- `GET /healthz`

Internal endpoints require the `X-Internal-Token` header, configured with `AAA_SHARED_INTERNAL_TOKEN`:

- `POST /v1/authn/introspect`
- `POST /v1/authz/check`
- `POST /v1/authz/batch-check`
- `POST /v1/authz/filter`
- `POST /v1/audit/record`

`nopsai` reaches this service through `AAA_API_URL`, defaulting to `http://aaa:8082`.

## Product Roles

The product roles are templates seeded by `nopsai` startup:

- `viewer`: read/list access for groups, pipelines, runs, logs, triggers, repositories, steps, scopes, secret metadata, variable metadata, and config repository metadata.
- `developer`: includes all viewer access plus non-destructive creation, updates, pipeline execution, `*.use` runtime permissions, rerun/cancel, trigger updates, secret writes, variable writes, repository updates, scope updates, reusable step usage, runner usage, and config repository usage.
- `owner`: includes all developer and viewer access plus all scoped non-admin actions, deletes, secret and variable value reads, ownership, and ACL management inside the owned scope.
- `admin`: platform-wide access through the normal AAA `Check` path.

The `admin` role can only be granted on the `platform` resource. Group grants must inherit.

## Subjects And Resources

Supported grant subjects:

- `user`
- `auth_group`
- `repository`
- `trigger`
- `service_account`
- `internal_service`

Supported grant resources:

- `folder` (the internal resource type for UI groups)
- `pipeline`
- `pipeline_run`
- `trigger`
- `secret`
- `variable`
- `scope`
- `repository`
- `step`
- `runner`
- `config_repo`
- `platform`

Group grant requests use the internal `folder` resource type and may use paths with a leading slash, such as `/payments/backend`; the stored internal ID is normalized without the leading slash. The special group `general` maps to the internal general group resource.

Named secret and variable resources use query-style internal IDs built from repository, scope, and name. The public grant API accepts the same logical IDs shown in the UI.

Runtime resource-sharing grants use a separate `group` subject type for existing resource groups/folders. That `group` subject is only used by the resource Access UI/API to share a resource with a group path; it is not the same as an AAA `auth_group`.

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
4. It expands the product role into `resource_acl` rows.
5. Owner grants also write `resource_ownership`.
6. Admin grants write `auth_role_bindings` for the platform role instead of resource ACLs.

Deleting a grant removes its expanded ACL and ownership rows through the grant foreign key.

## Runtime Resource Use Authorization

Pipeline execution uses a second authorization pass for resources that a run wants to use. This pass is centralized in `AuthorizeResourceUse` and is intentionally caller-based:

- manual Lab/API runs use the authenticated `user`
- Git-triggered runs use `repository:<owner>/<repo>`
- scheduled or automation runs should use the trigger or service-account identity
- dispatcher/internal calls only execute after the original caller has already been authorized

Supported low-level use actions include:

- `pipeline.use`
- `scope.use`
- `step.use`
- `secret.use`
- `variable.use`
- `runner.use`
- `config_repo.use`

The main rule is that same-group resources remain naturally available, but same-group is not permission elevation. The caller still needs the required action through its product role or ACL. Cross-group use requires an explicit resource-use grant or public visibility.

Resource visibility values:

- `group`: default; usable inside the same group boundary when the caller has the required role/action
- `restricted`: same group still works, and selected groups or repositories can also be granted use access
- `workspace`: shown in the UI as `Public`; usable by authorized callers outside the group, but still does not grant access to scopes, secrets, variables, or runners

Current resource Access UI behavior:

- Pipeline, step, and scope pages show an `Access` button next to the normal action buttons.
- The Access dialog offers `Only this group`, `This group and selected groups or repositories`, and, for non-sensitive resources, `Public`.
- Group sharing uses existing resource groups from `GET /v1/groups`.
- Repository sharing accepts canonical repository IDs such as `hosein-yousefii/test-app`.
- Sensitive resources such as scopes do not expose `Public`.
- The default scope is addressed as `scope:default` in the API/UI and stored internally as the empty/default scope.

Resource Access API:

- `GET /v1/resources/{type}/{id}/access`
- `PUT /v1/resources/{type}/{id}/access`
- `POST /v1/resources/{type}/{id}/grants`
- `DELETE /v1/resources/{type}/{id}/grants/{grantID}`
- `POST /v1/authz/resource-use/check`
- `POST /v1/authz/resource-use/batch-check`

Resource-use grant requests accept `subject_type: "repository"` with `subject_id: "owner/repo"` or `subject_type: "group"` with a group path such as `team-1`. Group use grants are stored in `access_grants` for audit and UI display; they do not write broad ACL rows that would elevate every member of the group. At check time, the original caller must still have the requested use action in its own group.

`pipeline_runs` stores the run authorization context in `trigger_source`, `requested_by_type`, `requested_by_id`, `effective_subject_type`, `effective_subject_id`, and `authorization_snapshot`. The snapshot records the caller and the resource-use checks that were allowed for the created run.

## GitOps Access Manifests

Config repositories can manage access records from YAML under `access/`.
Every `*.yaml` or `*.yml` file under that directory is read; `all.yaml` is only
a sample name. Manifests may use top-level `users`, `advanced_roles`,
`policies`, `advanced_role_bindings`, and `basic_roles` keys.

Example global manifest:

```yaml
users:
  - sub: alice
    email: alice@example.com
    advanced_roles: [release-manager]
  - sub: bob
    email: bob@example.com
    advanced_roles: [viewer]

advanced_roles:
  - name: release-manager
    policies:
      - resource: pipeline:team-1/*
        action: pipeline.execute

basic_roles:
  - user: alice
    role: owner
    resource: folder:team-1
```

Global config repositories can manage users, advanced role definitions,
policies, advanced role bindings, and basic role grants, including grants that
target delegated groups/folders. Group-scoped config repositories can manage
`basic_roles` only for their own group subtree; user, advanced-role, policy,
and direct advanced-role-binding management remains global.
In a group repo, grant resource IDs are normalized under the bound group, so a
grant with `resource_type: folder` in the `team-1` repo targets `folder:team-1`.

User-level `advanced_roles` assignments can reference custom roles from the
manifest or protected built-in role bundles such as `viewer`, `developer`,
`owner`, and `admin`. These assignments are global access-role bindings. Use
`basic_roles` when the same product role name should be scoped to a
folder/group target.

GitOps `basic_roles` use the same product roles as the API: `viewer`,
`developer`, `owner`, and `admin`. The group/folder is the grant target, not a
separate subject type. Non-admin basic roles are expanded into `resource_acl`;
owner grants also write `resource_ownership`. `admin` grants remain
platform-only and are rejected in group-scoped config repositories.

## Inheritance

The evaluator resolves parent resources before checking ACLs. A group grant can authorize:

- child groups
- pipelines and runs under the group path
- repositories assigned under the group path
- runs associated with repositories under the group path
- triggers for inherited repositories
- scoped secrets and variables
- reusable steps under the group path

Specific deny policies still win before inherited allow policies.

## Audit Behavior

Authorization decisions are written to `authz_decision_logs` when:

- the decision is denied
- the decision is allowed for a sensitive action

Sensitive actions include ACL management, admin/system operations, pipeline execution or deletion, run rerun/cancel/delete, trigger writes, secret value access, variable writes/deletes, group moves, repository deletes, and step deletes.

The normal request audit trail still writes to `audit_logs`.

## Bootstrap

Startup seeds:

- the default local admin user `admin@example.com` with role `nopsai-admin`
- `nopsai-admin` with `*` permissions
- `dispatcher-internal` bound to `internal_service:dispatcher`
- dispatcher permissions for pipeline fetches, execution, run status, logs, finalization, and task updates
- product role templates for `viewer`, `developer`, `owner`, and `admin`

The default admin role assignment is protected from accidental removal or deactivation through the admin APIs.
