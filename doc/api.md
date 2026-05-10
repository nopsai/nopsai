# Nopsai API Guide

The core service exposes its REST and WebSocket APIs on `http://localhost:8080`. This guide summarises the high-impact endpoints that power day-to-day automation. All examples assume local development defaults.

---

## Quick Start

```bash
# Refresh pipelines, reusable steps, environments, and triggers from the config repo
curl -X POST http://localhost:8080/v1/internal/config/sync
```

- Use this after updating the configuration repository or when bootstrapping a fresh database.
- The call is idempotent and can be triggered manually or by Git events.

---

## Access Control

NopsAI keeps the existing AAA policy engine as the decision point and layers product roles on top of it.

Predefined product roles:

- `viewer`: read-only access to folder metadata, pipelines, runs, logs, triggers, repository metadata, step metadata, secret metadata, and variable metadata
- `developer`: viewer permissions plus pipeline create/update/execute, rerun/cancel, trigger updates, secret value writes, variable writes, repository updates, scope updates, and reusable step usage
- `owner`: developer permissions plus delete operations, secret value reads, and permission management inside the owned scope
- `admin`: platform-wide access through the normal AAA `Check` path, with sensitive actions still audited

Important behavior:

- Product roles are expanded to low-level AAA permissions when the grant is created.
- Folder grants inherit to child folders, pipelines, runs, triggers, repositories, scoped secrets, scoped variables, and reusable steps under that folder path.
- `developer` can write secret values but cannot read them.
- `developer` and `viewer` cannot manage ACLs.
- `owner` cannot grant `admin`.
- `admin` is platform-scoped only.
- Deny rules still win before allows.
- Denied requests and sensitive allowed requests are still audit logged.

Supported access-grant subject types:

- `user`
- `auth_group`
- `internal_service`

Folder grants target folder paths, not numeric `group_id` values. Example: `/payments/backend`.

---

## Access Grants

Use these endpoints to assign product roles to subjects on resources.

```bash
# Grant developer to an auth group on a folder subtree
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "subject_type":"auth_group",
    "subject_id":"payments-devs",
    "role":"developer",
    "resource_type":"folder",
    "resource_id":"/payments",
    "inherit":true
  }' \
  http://localhost:8080/v1/access/grants

# List grants for a folder
curl "http://localhost:8080/v1/access/grants?resource_type=folder&resource_id=/payments"

# Delete a grant
curl -X DELETE http://localhost:8080/v1/access/grants/grant_123
```

Grant request fields:

- `subject_type`: `user`, `auth_group`, or `internal_service`
- `subject_id`: user subject, email, UUID, auth-group name/UUID, or service id
- `role`: `viewer`, `developer`, `owner`, or `admin`
- `resource_type`: `folder`, `pipeline`, `trigger`, `secret`, `variable`, `scope`, `repository`, `step`, or `platform`
- `resource_id`: folder path such as `/payments`, pipeline id such as `team-1/dev/build`, repository id such as `owner/repo`, or `platform`
- `inherit`: required for folder-style subtree grants; folder grants should normally use `true`

Example response:

```json
{
  "id": "grant_123",
  "subject_type": "auth_group",
  "subject_id": "payments-devs",
  "role": "developer",
  "resource_type": "folder",
  "resource_id": "/payments",
  "inherit": true,
  "granted_by": "admin"
}
```

Validation and guardrails:

- The target subject and resource must already exist.
- Every folder must retain at least one `owner`.
- Only `owner` or `admin` can manage grants.
- `admin` grants are only valid on `platform`.

---

## Effective Permissions

Use `GET /v1/access/effective-permissions` when you want to see why a request is allowed or denied.

```bash
curl "http://localhost:8080/v1/access/effective-permissions?action=pipeline.update&resource_type=pipeline&resource_id=payments/deploy-api"
```

Example response:

```json
{
  "allowed": true,
  "action": "pipeline.update",
  "resource": "pipeline:payments/deploy-api",
  "reason": "group payments-devs has developer on folder:/payments, inherited by pipeline:payments/deploy-api",
  "matched_role": "developer",
  "matched_subject": "group payments-devs",
  "matched_resource": "folder:/payments",
  "inherited": true,
  "source_parent_resource": "folder:/payments",
  "low_level_permission": "pipeline.update"
}
```

This endpoint is the product-facing explanation layer on top of the existing AAA `Check` and inheritance logic.

---

## Secrets

Secrets are encrypted at rest using the master key. They can be scoped globally, per environment, or per repository.

```bash
# List secrets (add ?env=scope and ?include_source=true for metadata)
curl "http://localhost:8080/v1/secrets?env=prod&include_source=true"

# Discover environments that currently hold secrets
curl http://localhost:8080/v1/secrets/scopes

# Upsert a global secret (optionally scoped to an environment)
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value":"General level secret prod env"}' \
  "http://localhost:8080/v1/secrets/TEST_SECRET?env=prod"

# Delete a global secret
curl -X DELETE http://localhost:8080/v1/secrets/TEST_SECRET

# Repository-scoped secret (owner/repo path segments)
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value":"repo level secret"}' \
  "http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET"
```

- Repository endpoints also accept `?env=` to target scoped environments.
- Repository-scoped entries returned by `GET /v1/secrets` are prefixed with `owner/repo/SECRET`, so the UI can group them under the same scope as global secrets.
- `GET /v1/secrets/scopes` reports only scopes (default, prod, etc.) to mirror the Scopes page.
- Secrets resolve in the following order: repo+env → repo → global+env → global.
- Predefined product roles expose secret metadata broadly, but secret value reads remain owner/admin-level by default.

---

## Scope Variables

Scope variables mirror the scoping rules used for secrets.

```bash
# Global scope variable
curl -X PUT -d '{"value":"general"}' \
  "http://localhost:8080/v1/variables/TEST_ENV"

# Repository scope variable
curl -X PUT -d '{"value":"repo"}' \
  "http://localhost:8080/v1/repositories/hosein-yousefii/test-app/variables/TEST_ENV"

# Fetch scoped variables
curl "http://localhost:8080/v1/variables?env=prod"
```

- The list endpoint now returns both global variables (e.g. `DATABASE_URL`) and repository-scoped entries in the form `owner/repo/NAME`.
- Duplicate keys inside the same scope are rejected during config sync.
- The config repo may define scope folders under `environments/<scope>/env.yaml`; the sync endpoint imports them automatically.
- Predefined product roles allow variable metadata reads and writes, but do not grant variable value reads by default.

---

## Pipelines

```bash
# List pipelines (global + scoped names)
curl http://localhost:8080/v1/pipelines

# Inspect a specific pipeline
curl http://localhost:8080/v1/pipelines/main-pipeline
curl http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline

# Upsert a pipeline definition from disk
curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/main-pipeline

# Delete a pipeline
curl -X DELETE http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline
```

- Paths containing slashes map to nested folders (e.g. `team-1/dev`).
- Pipeline responses include metadata such as version, description, steps, tasks, timeout, container image, and LLM controls.
- Folder-level product grants inherit to pipelines below that folder path.

---

## Pipeline Run Structure

- The GitOps config repository can now define the folder and repository hierarchy for the Pipeline Runs UI via `pipelineruns/structure.yaml`.
- Each top-level key is a folder. Nest folders by adding child keys, and assign repositories under a folder with a `repos:` list.
- Repository entries should use the same `owner/repo` strings that appear in triggers and run metadata.
- Example:

```yaml
general:
  description: General workflows
team-1:
  description: Description for team-1 folder
  repos:
    - hosein-yousefii/general-app
  dev:
    description: This is new
    repos:
      - hosein-yousefii/test-app
      - hosein-yousefii/t-app
team-2:
  bank:
    description: Handles bank-facing apps
    repos:
      - hosein-yousefii/all-app
```

- Running `POST /v1/internal/config/sync` now ingests this file, creating or updating the folders in the `groups` table and assigning repositories to their Git-defined parents. Existing manual folders not referenced in the file are left untouched.

---

## Reusable Steps

```bash
curl http://localhost:8080/v1/steps                     # list
curl http://localhost:8080/v1/steps?include_source=true # list with metadata
curl http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/shared/utilities/archive-step.yaml" \
  http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X DELETE http://localhost:8080/v1/steps/shared/utilities/archive-step
```

- Reusable steps can be referenced from pipelines through the `include:` directive.
- When `include_source=true` each item includes `identifier`, `path`, `name`, `source`, and `updated_at`, allowing the UI to distinguish Git-managed definitions from database overrides.
- Using a reusable step from a pipeline requires `step.use`. Managing step definitions is effectively admin-only in the predefined role set.

---

## Trigger Overrides

```bash
curl http://localhost:8080/v1/overrides                               # list
curl http://localhost:8080/v1/overrides/hosein-yousefii/test-app      # inspect

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X DELETE http://localhost:8080/v1/overrides/hosein-yousefii/test-app
```

- Overrides let you replace or augment the config-repo trigger manifest for a given repository.
- The payload mirrors the `.nopsai/triggers.yaml` schema (event, branches, skip branches, tags, pipelines, scope).

---

## Run Lifecycle

```bash
# Start a run (pipeline name optional in path)
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"pipeline":"main-pipeline"}' \
  http://localhost:8080/v1/run

curl -X POST http://localhost:8080/v1/run/team-1/dev/main-pipeline

# Fetch runs and details
curl http://localhost:8080/v1/runs
curl http://localhost:8080/v1/runs/<run-id>
curl http://localhost:8080/v1/runs/<run-id>/status
curl http://localhost:8080/v1/runs/<run-id>/logs
curl http://localhost:8080/v1/runs-by-check/<check-run-id>

# Rerun, cancel, or finalise
curl -X POST http://localhost:8080/v1/runs/<run-id>/rerun
curl -X POST http://localhost:8080/v1/runs/<run-id>/cancel
curl -X POST -H 'Content-Type: application/json' \
  -d '{"status":"success"}' \
  http://localhost:8080/v1/runs/<run-id>/finalize

# Cleanup
curl -X DELETE http://localhost:8080/v1/runs/<run-id>
curl -X DELETE \
  http://localhost:8080/v1/repositories/<owner>/<repo>/branches/<branch>
```

- Step/task status updates are posted by the agent to `/v1/runs/{runID}/steps/{step}/tasks/{task}` (payload includes status, exit code, and LLM timing).
- Run listings return summary metadata used by the UI cards and WebSocket broadcasts.
- Run log access is authorized separately from run-detail access in the low-level AAA layer.
- Branch cleanup removes historical runs for the specified branch while leaving the repository intact.

---

## Run Groups (UI Folders)

```bash
curl -X POST -H 'Content-Type: application/json' \
  -d '{"name":"team-1", "parent_id":null}' \
  http://localhost:8080/v1/groups

curl http://localhost:8080/v1/groups

curl -X PUT -H 'Content-Type: application/json' \
  -d '{"name":"team-1/platform"}' \
  http://localhost:8080/v1/groups/<group-id>

curl -X PUT -H 'Content-Type: application/json' \
  -d '{"parent_id":42}' \
  http://localhost:8080/v1/groups/<group-id>/move

curl -X DELETE http://localhost:8080/v1/groups/<group-id>
```

- Groups power the “Main” dashboard’s folder hierarchy. Each run card can be assigned to a group path.
- Access grants should target the resolved folder path, not the numeric `group_id`.
- `GET /v1/groups` is filtered by the caller’s folder visibility.

---

## Git & Repository Utilities

```bash
# Triggered by git-bot; useful for debugging
curl -X POST http://localhost:8080/v1/git/events \
  -H "Content-Type: application/json" \
  --data-binary "@doc/sample-git-event.json"

# List branches known to the API (for branch clean-up helpers)
curl http://localhost:8080/v1/repositories/hosein-yousefii/test-app/branches
```

- The git-bot exposes additional HTTP helpers (from `services/git-bot`) for file content, directory listings, repository access checks, and child check-run creation.

---

## WebSocket Stream

```bash
# Connect (browser or CLI using wscat or websocat)
ws://localhost:8080/v1/ws

# Subscribe to a specific run
{"type":"subscribe", "payload":{"runId":"<run-id>"}}
```

- Events include `new_run_started`, `run_update`, `run_summary_update`, and `log_line` payloads.
- The UI automatically replays subscriptions after reconnects; automation clients should mimic the same behaviour.

---

## Environment Reset (Optional)

For local testing, you can clear Docker state with:

```bash
docker-compose down -v
docker container prune -f -a
docker volume prune -f -a
docker image prune -f -a
```

> Warning: these commands remove **all** containers, volumes, and images on your machine. Use them only on disposable environments.
