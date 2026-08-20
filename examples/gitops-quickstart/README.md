# GitOps Quickstart Sample

One small, complete NopsAI configuration, split the way real installations are:
a **global repository** owned by platform administrators and a **team repository**
owned by one team. Copy it, rename `platform` to your own team, replace the
example hosts and credential references, and sync it.

```
global-repo/   system settings, users and roles, knowledge, team registration
team-repo/     the platform team's pipelines, steps, scopes, schedules, dashboard
```

## What each file teaches

### global-repo (bind as `scope_type=system`, `scope_id=global`, `base_path=""`)

| File | Teaches |
| --- | --- |
| `setting/system/runner.yaml` | Runtime defaults, runner install defaults, and dispatcher routing per scope. |
| `setting/system/assistant.yaml` | Nopsai AI Assistant provider, model, features, and memory. Assistant settings have their own file; they are not part of `runner.yaml`. |
| `models/standard.yaml`, `models/reasoning.yaml` | One file per model, like a pipeline. `default: true` marks the workspace default. |
| `agent-roles/release-manager.yaml` | One file per agent role, with `default: true` for the workspace default. |
| `mcp/servers/github.yaml` | One file per MCP server. Servers are workspace-wide. |
| `mcp/profiles/github-readonly.yaml` | One file per MCP profile; profiles select tools from approved servers. |
| `setting/system/auth.yaml` | Local login and external identity providers. |
| `setting/system/mail.yaml` | SMTP settings for notification mail. |
| `setting/system/data-management.yaml` | Scheduled cleanup of run history. |
| `setting/system/credentials.example.yaml` | The shape of encrypted credential envelopes. Secret values never appear in Git. |
| `setting/git-apps/github.yaml` | GitHub App identity and installations for repository events. |
| `access/all.yaml` | Users, basic roles, and a fine-grained advanced role. |
| `config-repositories/teams/platform/structure.yaml` | Registers the `platform` team and binds it to `team-repo`. |
| `knowledge/architecture/platform/service-architecture.md` | Architecture knowledge attached to LLM work. |
| `knowledge/guardrail/platform/release-safety.md` | A guardrail that constrains LLM behavior. |
| `models/platform/team-review.yaml` | A team-scoped model. The team lives in the path, exactly like a pipeline, so this file defines the model named `platform/team-review`. |
| `knowledge/connections/platform/engineering-wiki.yaml` | A Notion, Confluence, or wiki connection. Git owns the definition; reachability status stays runtime state. |
| `knowledge/runbook/platform/service-onboarding.md` | A document whose body is mirrored from a connected page. Git owns which page is attached and how it syncs, not the page text. |
| `triggers/platform/service-api.yaml` | Repository events that start pipelines. |

### team-repo (bind to team `platform` from the global repo)

| File | Teaches |
| --- | --- |
| `pipelines/platform/hello-world.yaml` | The smallest working pipeline. Start here. |
| `pipelines/platform/build-and-test.yaml` | Variables, reusable steps, `depends_on`, and a markdown final output. |
| `pipelines/platform/deploy-service.yaml` | Scope secrets, restricted access, and a human approval checkpoint. |
| `pipelines/platform/release-notes.yaml` | An LLM goal task with an model, agent role, and Knowledge Context. |
| `pipelines/platform/service-health-dashboard.yaml` | A dashboard final output published from run evidence. |
| `pipelines/platform/release-service.yaml` | Every pipeline capability in one manifest: the finished example the wiki's Pipelines chapter builds page by page. |
| `steps/platform/shared/checkout.yaml`, `notify.yaml` | Reusable steps included by other pipelines. |
| `scopes/platform/dev/scope.yaml`, `prod/scope.yaml` | Per-scope variables and declared secret keys. |
| `schedules/platform/nightly-service-health.yaml` | A cron schedule bound to a scope. |
| `dashboards/platform/service-health.yaml` | The dashboard that receives the pipeline output. |
| `access/grants.yaml` | Team-owned role grants. |
| `config-repositories/teams/platform/notifications.yaml` | Team notification routing and throttling. |

## Try it

1. Create two Git repositories from `global-repo/` and `team-repo/`.
2. Bind the global repository:

   ```bash
   curl -X PUT -H "Content-Type: application/json" \
     -d '{"provider":"github","repo_url":"https://github.com/acme/nopsai-global-config","branch":"main","base_path":"","enabled":true,"write_enabled":true,"write_branch":"nopsai/ui-changes"}' \
     http://localhost:8080/v1/system/config-repo
   ```

   The team repository does not need a separate API call: syncing the global
   repository registers the `platform` team and its binding from
   `config-repositories/teams/platform/structure.yaml`.
3. Sync, then run `platform/hello-world` from the UI to confirm a runner picks
   up work.
4. Review pending changes before pushing back:

   ```bash
   curl http://localhost:8080/v1/system/config-repo/drift
   ```

## Rules worth knowing early

- Paths inside a team repository carry the team prefix (`pipelines/platform/...`);
  that is also the layout NopsAI writes back when it exports.
- Models, agent roles, and MCP profiles are one file per resource. A file
  directly under the directory is workspace-wide; a file under a team segment
  (`models/platform/…`) is owned by that team. Exactly one file per scope may
  declare `default: true`, so the default always travels with the resource.
- Secrets are declared by key in scopes and resolved from the credential store.
  Only encrypted envelopes belong in Git.
- `knowledge/connections/` is reserved for connection definitions, so it is never
  read as a knowledge kind. A document that declares `source.type: external_page`
  must not carry inline content: the connected page body is mirrored at run time
  and would otherwise show up as permanent drift.
- UI or API edits to a GitOps-managed resource create a database override. Push
  the change back to the owning repository to keep Git authoritative.
