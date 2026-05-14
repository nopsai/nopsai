# Config repository samples

These examples show the Git layout consumed by Nopsai config sync.

There are two repositories represented here:

- `global-repo`: a system/global config repository, bound as `scope_type=system`, `scope_id=global`, and `base_path=""`.
- `team-1-repo`: a group-owned config repository, bound to group `team-1` by the global repo.

The global repo can sync every shared config type and can define group config
repo bindings. Group repos then become authoritative for everything under their
group path.

## Global repo binding

```json
{
  "repo_url": "https://github.com/acme/nopsai-global-config",
  "branch": "main",
  "base_path": "",
  "enabled": true
}
```

Create or update it through:

```bash
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"repo_url":"https://github.com/acme/nopsai-global-config","branch":"main","base_path":"","enabled":true}' \
  http://localhost:8080/v1/system/config-repo
```

## Syncable directories

Under the configured `base_path`, Nopsai scans:

```text
pipelines/             Pipeline definitions
steps/                 Reusable step definitions
triggers/              Trigger override manifests
scopes/                Scope variable files
pipelineruns/          Run group structure
config-repositories/   Group config repo bindings
access/                Users, advanced roles, policies, and basic role grants
```

Secrets are not imported from Git. Pipelines may declare required secrets, but
secret values stay database-managed.

Pipeline, reusable step, and scope files may also include an `access:` block.
That block maps to the same resource Access UI controls: `visibility` controls
Only this group / selected groups or repositories / Public, and `use_access`
lists the groups or repositories that can use a restricted resource.

## Global repo file map

```text
global-repo/pipelines/platform-maintenance.yaml
  -> pipeline platform-maintenance, public use access

global-repo/steps/shared/announce.yaml
  -> reusable step shared/announce

global-repo/triggers/acme/service-api.yaml
  -> trigger override acme/service-api

global-repo/scopes/dev/scope.yaml
  -> variables in scope dev

global-repo/config-repositories/groups/team-2/platform.yaml
  -> config repo binding and group shell for group team-2/platform

global-repo/config-repositories/groups/structure.yaml
  -> Pipeline Runs group structure, repositories under group shells, and inline group config repo binding for team-1

global-repo/access/*.yaml
  -> global users, advanced roles, policies, advanced role bindings, and basic role grants
```

When the global repo defines group bindings under `config-repositories/groups`,
those bindings create the group shells. Put repository placement next to those
bindings in `config-repositories/groups/structure.yaml` or in a scoped file such
as `config-repositories/groups/team-1/structure.yaml`. A group node can include
`config:` with the same fields as a standalone binding file.

## Group Repo File Map

When `team-1-repo` is bound to group `team-1`, Nopsai prefixes synced
resources with `team-1`:

```text
team-1-repo/pipelines/build-and-test.yaml
  -> pipeline team-1/build-and-test with restricted use access

team-1-repo/pipelines/services/api/deploy.yaml
  -> pipeline team-1/services/api/deploy

team-1-repo/steps/shared/checkout.yaml
  -> reusable step team-1/shared/checkout

team-1-repo/triggers/service-api.yaml
  -> trigger override team-1/service-api

team-1-repo/scopes/prod/scope.yaml
  -> variables in scope team-1/prod with restricted scope use access

team-1-repo/access/*.yaml
  -> basic role grants scoped to team-1
```

Pipeline and step file names must match their `name` fields. Group repo trigger
manifests and includes should reference the final group-prefixed IDs.

Nopsai reads every `.yaml` and `.yml` file under `access/`; file names such as
`all.yaml` or `grants.yaml` are only examples, so teams can split manifests by
owner, environment, or workflow.

Global config repos may manage basic role grants even when the target group has
its own delegated config repo. Group-owned config repos may manage basic role
grants for their own group subtree. User, advanced-role, policy, and direct
advanced-role-binding management remain global-repo only. In group repos, grant
resource IDs are prefixed with the bound group automatically, so
`resource: folder:dev` in the `team-1` repo targets `folder:team-1/dev`.

User `advanced_roles` are global access-role assignments and may reference
custom roles or protected built-in bundles such as `viewer`, `developer`,
`owner`, and `admin`. Use `basic_roles` when those same product role names
should be scoped to a folder/group target.

## Embedded resource access

Use embedded `access:` for the per-object Access dialog settings:

```yaml
name: deploy
access:
  visibility: restricted # group, restricted, or public/workspace
  use_access:
    grants:
      - subject_type: repository
        subject_id: hosein-yousefii/test-app
      - subject_type: group
        subject_id: data-team
steps:
  - name: deploy
    script: echo deploy
```

For the common UI subjects, the shorter form is also accepted:

```yaml
access:
  groups: [data-team]
  repositories: [hosein-yousefii/test-app]
```

When grants are present and `visibility` is omitted, Nopsai treats the resource
as `restricted`. Scopes are sensitive, so they support `group` and `restricted`
visibility only; `public` is accepted for pipelines and reusable steps.
