# Config repository samples

These examples show the Git layout consumed by Nopsai config sync.

There are two repositories represented here:

- `global-repo/nopsai`: a system/global config repository, bound as `scope_type=system`, `scope_id=global`, and `base_path=nopsai`.
- `team-1-repo/nopsai`: a group-owned config repository, bound to group `team-1` by the global repo.

The global repo can sync every shared config type and can define group config
repo bindings. Group repos then become authoritative for everything under their
group path.

## Global repo binding

```json
{
  "repo_url": "https://github.com/acme/nopsai-global-config",
  "branch": "main",
  "base_path": "nopsai",
  "enabled": true
}
```

Create or update it through:

```bash
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"repo_url":"https://github.com/acme/nopsai-global-config","branch":"main","base_path":"nopsai","enabled":true}' \
  http://localhost:8080/v1/system/config-repo
```

## Syncable directories

Under the configured `base_path`, Nopsai scans:

```text
pipelines/             Pipeline definitions
steps/                 Reusable step definitions
triggers/              Trigger override manifests
environments/          Scoped variable files
pipelineruns/          Run group structure
config-repositories/   Group config repo bindings
```

Secrets are not imported from Git. Pipelines may declare required secrets, but
secret values stay database-managed.

## Global repo file map

```text
global-repo/nopsai/pipelines/platform-maintenance.yaml
  -> pipeline platform-maintenance

global-repo/nopsai/steps/shared/announce.yaml
  -> reusable step shared/announce

global-repo/nopsai/triggers/acme/service-api.yaml
  -> trigger override acme/service-api

global-repo/nopsai/environments/dev/env.yaml
  -> variables in scope dev

global-repo/nopsai/pipelineruns/structure.yaml
  -> Pipeline Runs group structure

global-repo/nopsai/config-repositories/groups/team-1.yaml
  -> config repo binding for group team-1

global-repo/nopsai/config-repositories/groups/team-2/platform.yaml
  -> config repo binding for group team-2/platform
```

## Group Repo File Map

When `team-1-repo/nopsai` is bound to group `team-1`, Nopsai prefixes synced
resources with `team-1`:

```text
team-1-repo/nopsai/pipelines/build-and-test.yaml
  -> pipeline team-1/build-and-test

team-1-repo/nopsai/pipelines/services/api/deploy.yaml
  -> pipeline team-1/services/api/deploy

team-1-repo/nopsai/steps/shared/checkout.yaml
  -> reusable step team-1/shared/checkout

team-1-repo/nopsai/triggers/service-api.yaml
  -> trigger override team-1/service-api

team-1-repo/nopsai/environments/prod/env.yaml
  -> variables in scope team-1/prod
```

Pipeline and step file names must match their `name` fields. Group repo trigger
manifests and includes should reference the final group-prefixed IDs.
