# Team And Resource Ownership Design

This document is a target design for separating teams from Pipeline Runs and
for adding team-scoped AI profile ownership. It is intentionally written as a
migration plan because the current implementation still uses `teams` as a
mixed team/application tree and `pipeline_runs.team_id` as the run owner.

## Decision

Teams must not be created or administered from Pipeline Runs.

Pipeline Runs is runtime history. A run can reference a team, application,
repository, pipeline, scope, trigger, schedule, and ownership snapshot, but it
must not own the lifecycle of those objects.

The product structure should be:

```text
Team
  -> applications / repositories
  -> GitOps configuration
  -> notification policies
  -> access ownership
  -> team AI profiles
     -> LLM profiles
     -> Agent profiles
     -> MCP profiles
  -> resources
     -> pipelines
     -> reusable steps
     -> triggers
     -> schedules
     -> scopes
     -> knowledge contexts

PipelineRun
  -> references team + application + resource + runtime snapshot
```

## Implementation Status

The separation is now implemented as a team-only API layer on the current
storage table:

- `/teams` is the UI home for creating teams/applications and managing the
  existing team-scoped GitOps repository and notification policy.
- Pipeline Runs no longer exposes create, delete, or config/notification
  controls for legacy teams; selected run teams link to Teams for administration.
- Pipelines, Triggers, Steps, Scopes, Knowledge Context, and the global sidebar
  no longer inject empty team paths into their resource trees by default.
- Repository-triggered run resolution prefers an existing repository app under
  the trigger team. Trigger save and GitOps sync create missing supported
  provider apps idempotently, while unsupported providers still require an
  explicit application entry.
- `/v1/teams` exposes team/application CRUD, team config repositories, and
  notification routes as the only public team hierarchy API.
- Team-scoped LLM, Agent, and MCP profile tables and APIs are available under
  `/v1/teams/{teamID}/...` with team-scoped AAA checks.
- Team config repositories can import/export root `ai-profiles.yaml` files,
  and the Teams settings modal has an AI Profiles tab for permission-gated
  LLM, Agent, and MCP profile editing.
- The generated CLI API catalog includes the new team route metadata, so
  `nopsai api request`, `api describe`, and `api routes` stay compatible.

Still pending in the larger migration: dedicated `teams` and `applications`
tables and the canonical `teams/<team>/` GitOps layout.

## Original Problems Addressed In Phase 1

The repository had three related coupling points that drove the first
implementation slice.

1. `services/ui/src/pages/PipelineRuns.tsx` owned create/delete team behavior
   and passed `parent_id` from the selected Pipeline Runs team. Phase 1 moved
   this to `/teams`.
2. `services/ui/src/features/pipeline-runs` owned the team config repository
   and notification modal, even though GitOps and notifications are team
   administration responsibilities. Phase 1 moved those controls under
   `services/ui/src/features/teams`.
3. `services/nopsai/run_team_resolution.go` could create
   `teams.kind = 'app'` rows while resolving a repository-triggered run.
   Phase 1 changed runtime resolution to select an existing owner or leave the
   run unassigned.

The backend also stores two domain concepts in one table:

```text
teams.kind = 'team' -> team/team/product area
teams.kind = 'app'   -> repository/application node
```

`services/nopsai/team_schema.go` now replaces the old global `UNIQUE(name)`
constraint with sibling-scoped uniqueness, because two teams should be able to
own the same child label, for example `payments/backend` and
`identity/backend`.

The UI also projected the same team paths into unrelated resource trees
through `services/ui/src/app/resourceTrees.ts` and `useResourceTrees.ts`. Phase
1 stopped loading empty team paths into resource pages by default so teams do
not appear under pipelines, triggers, steps, scopes, and knowledge contexts
unless there is an actual resource in that branch.

## Target Domain Model

Use stable IDs for ownership. Paths and slugs are display and routing
attributes, not identity.

```text
teams
- id
- slug
- display_name
- description
- parent_team_id nullable
- status active|archived
- source database|gitops
- config_repo_id nullable
- config_source_path nullable
- created_at
- updated_at

applications
- id
- team_id
- slug
- display_name
- repo_url
- repository_full_name
- default_branch
- status active|archived
- source database|gitops
- config_repo_id nullable
- config_source_path nullable
- created_at
- updated_at
```

Resource tables should gain explicit ownership while preserving current string
identifiers during migration:

```text
pipelines.team_id
pipelines.application_id nullable
steps.team_id
triggers.team_id
external_triggers.team_id
pipeline_schedules.team_id
knowledge_contexts.team_id
variables.team_id nullable
secrets.team_id nullable
```

Pipeline runs should become historical references:

```text
pipeline_runs.team_id
pipeline_runs.application_id nullable
pipeline_runs.pipeline_id nullable
pipeline_runs.repository_full_name
pipeline_runs.ownership_snapshot jsonb
```

`ownership_snapshot` preserves the historical label if a team or application is
renamed, moved, or archived after a run completes.

## UI Structure

Add a first-class Teams area:

```text
Pipeline Runs
Monitoring
Teams
Pipelines
Schedules
Triggers
...
System
```

Teams should own these tabs:

```text
Overview
Applications
GitOps
Notifications
Access
AI Profiles
Resources
```

Pipeline Runs should only provide runtime workflows:

- filter by team, application, repository, branch, status, schedule, trigger,
  requester, and scope
- inspect run detail, logs, outputs, approvals, and authorization snapshot
- cancel, rerun, approve, reject, and delete runs when allowed
- link to `Teams -> selected team` for administration

Remove from Pipeline Runs:

- create team/team
- create application/repository node
- edit team GitOps config repository
- edit team notification policy
- move/delete organization nodes

Resource pages should build trees from actual resources only. Empty team shells
should not appear under pipelines, triggers, steps, scopes, or knowledge by
default. Administrative users can get an explicit "show empty teams" option if
needed.

## Team-Scoped AI Profiles

The current docs and code treat LLM, Agent, and MCP profiles as system-owned
catalogs. The target model should keep a system catalog but allow team-scoped
profiles when the caller has the correct permissions.

The enterprise boundary should be:

```text
System admin:
  owns provider/server catalogs, credential policies, global defaults,
  max limits, allowed model/provider/server templates, and deny rules.

Team admin:
  owns team defaults and team profile aliases inside approved boundaries.
```

### Profile Storage

Add ownership columns or replacement tables with scoped uniqueness:

```text
llm_profiles.scope_type system|team
llm_profiles.scope_id nullable
UNIQUE(scope_type, scope_id, lower(name))

agent_profiles.scope_type system|team
agent_profiles.scope_id nullable
UNIQUE(scope_type, scope_id, lower(id))

mcp_profiles.scope_type system|team
mcp_profiles.scope_id nullable
UNIQUE(scope_type, scope_id, lower(name))
```

Team-owned profiles should support `source`, `config_repo_id`, and
`config_source_path` exactly like team-owned resources.

### Resolution

Profile references in pipeline YAML stay simple for most authors:

```yaml
llm_profile: release-review
agent_profile: sre-reviewer
mcp_profiles:
  - github-pr-readonly
```

Resolution should use the run's owning team:

1. explicit fully-qualified reference, such as `system:standard` or
   `team:payments/release-review`
2. team-scoped profile with that name
3. system-scoped profile with that name
4. team default profile
5. system default profile

Validation must reject ambiguity and policy violations before agent launch.

### Policy Rules

Team LLM profiles can select only approved providers/models and credentials
allowed for that team. Credential references should support team namespaces:

```text
credential://team/<team-id-or-slug>/llm/openai-prod
credential://team/<team-id-or-slug>/mcp/github
```

Team Agent profiles may define local persona/instruction text, but system admins
can enforce maximum size, disallowed prompt fragments, and required enterprise
guardrails.

Team MCP profiles may compose approved MCP servers and tool allowlists. Defining
new MCP servers should remain system-owned unless an explicit policy enables a
team to register endpoints for its own namespace.

## GitOps Layout

Support a new canonical layout while importing legacy team files during
migration:

```text
config-repositories/
  teams/
    payments/
      structure.yaml
      notifications.yaml
ai-profiles.yaml
```

Example:

```yaml
apiVersion: nopsai.io/v1
kind: Team

metadata:
  name: payments
  displayName: Payments

spec:
  description: Owns payment services and release automation.

  gitops:
    repository: https://github.com/acme/nopsai-payments-config
    branch: main
    basePath: ""
    write:
      enabled: true
      branch: nopsai/payments-ui

  applications:
    - name: checkout-api
      repository: https://github.com/acme/checkout-api

  defaults:
    llmProfile: release-review
    agentProfile: payments-sre
    mcpProfiles:
      - github-pr-readonly
```

Team-scoped AI profiles live beside the team:

```yaml
llm_profiles:
  - name: release-review
    provider: openai
    model: gpt-4.1
    credential_ref: credential://team/payments/llm/openai
    allowed_scopes: ["dev", "staging", "prod"]

agent_profiles:
  - id: payments-sre
    display_name: Payments SRE
    enabled: true
    instructions: |
      Review rollout safety, payment blast radius, auditability, and rollback.

mcp_profiles:
  - name: github-pr-readonly
    description: Read-only GitHub PR context for payments repositories.
    enabled: true
    servers:
      - server: github
        tools: ["get_file", "list_pull_request_files"]
```

The global repository remains the place for:

- system profile defaults
- provider/model allowlists
- MCP server registry
- credential policy
- organization-wide deny rules
- system-managed teams

## API And AAA

Use team-oriented APIs only.

```text
GET    /v1/teams
POST   /v1/teams
GET    /v1/teams/{teamID}
PUT    /v1/teams/{teamID}
DELETE /v1/teams/{teamID}

GET    /v1/teams/{teamID}/applications
POST   /v1/teams/{teamID}/applications
PUT    /v1/teams/{teamID}/applications/{applicationID}
DELETE /v1/teams/{teamID}/applications/{applicationID}

GET    /v1/teams/{teamID}/config-repository
PUT    /v1/teams/{teamID}/config-repository
POST   /v1/teams/{teamID}/config-repository/sync
POST   /v1/teams/{teamID}/config-repository/sync/cancel
GET    /v1/teams/{teamID}/config-repository/drift
POST   /v1/teams/{teamID}/config-repository/write

GET    /v1/teams/{teamID}/notifications
PUT    /v1/teams/{teamID}/notifications

GET    /v1/teams/{teamID}/llm-profiles
PUT    /v1/teams/{teamID}/llm-profiles
PUT    /v1/teams/{teamID}/llm-profiles/default
PUT    /v1/teams/{teamID}/llm-profiles/{profileName}
DELETE /v1/teams/{teamID}/llm-profiles/{profileName}

GET    /v1/teams/{teamID}/agent-profiles
POST   /v1/teams/{teamID}/agent-profiles
PUT    /v1/teams/{teamID}/agent-profiles/default
GET    /v1/teams/{teamID}/agent-profiles/{profileID}
PUT    /v1/teams/{teamID}/agent-profiles/{profileID}
DELETE /v1/teams/{teamID}/agent-profiles/{profileID}

GET    /v1/teams/{teamID}/mcp-profiles
POST   /v1/teams/{teamID}/mcp-profiles
GET    /v1/teams/{teamID}/mcp-profiles/{profileName}
PUT    /v1/teams/{teamID}/mcp-profiles/{profileName}
DELETE /v1/teams/{teamID}/mcp-profiles/{profileName}

GET    /v1/teams/{teamID}/mcp/profiles
POST   /v1/teams/{teamID}/mcp/profiles
GET    /v1/teams/{teamID}/mcp/profiles/{profileName}
PUT    /v1/teams/{teamID}/mcp/profiles/{profileName}
DELETE /v1/teams/{teamID}/mcp/profiles/{profileName}
```

Team moves are expressed by `parent_id` or `parent_team_id` on
`PUT /v1/teams/{teamID}`. Application updates and moves use the target parent
in the application URL, for example
`PUT /v1/teams/{targetTeamID}/applications/{applicationID}`. Moving either
resource requires update permission on the resource plus create permission on
the destination parent scope, and parent updates reject hierarchy cycles.

New AAA actions should be separate from run actions:

```text
team.read
team.create
team.update
team.delete
team.gitops.manage
team.gitops.sync
team.notifications.manage
team.access.manage
team.ai_profiles.read
team.llm_profiles.manage
team.agent_profiles.manage
team.mcp_profiles.manage
application.create
application.update
application.delete
```

Profile resolution at runtime must check both resource permission and profile
use permission. A user who can run a pipeline does not automatically get to use
an expensive or sensitive team LLM profile unless the team/profile policy
allows it.

## Source Ownership

Keep the feature separated while it is developed:

- Model logic: `pkg/models` for public YAML/JSON contracts, and focused
  `services/nopsai/internal/team*` packages for normalization, resolution,
  import/export, and validation.
- API logic: new `services/nopsai/team_handlers.go`,
  `team_application_handlers.go`, and `team_ai_profile_handlers.go` instead of
  expanding `team_store_handlers.go`.
- Runtime resolution: replace new work in `run_team_resolution.go` with
  `run_ownership_resolution.go`; compatibility may read `teams`, but runtime
  should not create teams/apps.
- Hook orchestration: create `services/ui/src/features/teams/hooks` for team
  data, GitOps, notifications, access, and AI profile state.
- Rendering: create `services/ui/src/features/teams/components` for list,
  detail tabs, forms, and modals. Pipeline Runs should render only run
  workflows.
- Route composition: add `/teams` in `services/ui/src/app/AppRoutes.tsx` and
  navigation in `AppShell.tsx`; keep Pipeline Runs routes runtime-only.

## Monitoring, MCP, And CLI

Monitoring should add team/application dimensions while preserving existing
team filters during compatibility:

- AI usage by team, application, profile scope, provider, model, step, and task
- run reliability by team and application
- config sync health by team config repository
- notification delivery by team route
- profile usage and policy-denial counters

Hosted MCP should expose team-aware tools only when the current subject has the
matching AAA permissions:

- list/read teams and applications
- propose GitOps-ready team, application, notification, and profile changes
- read effective LLM/Agent/MCP profile resolution for a pipeline/run
- keep direct mutations behind existing `confirm:true` semantics

The CLI should expose team routes in the route catalog:

```text
nopsai teams list
nopsai teams get <team>
nopsai teams config-repo sync <team>
nopsai teams profiles llm list <team>
```

If the CLI surface is not expanded immediately, `nopsai api describe` and
`nopsai api request` must still include the new `/v1/teams` route metadata.

## Migration Plan

### Phase 1: UI Separation, Same Database

- Add `/teams` and move team/app creation, config repository settings, and
  notifications out of Pipeline Runs.
- Use `/v1/teams` directly in the UI.
- Remove team/app create and settings controls from Pipeline Runs.
- Stop injecting all team paths into resource trees by default.
- Add team filters to resource and monitoring pages.

### Phase 2: Team APIs And Compatibility Adapter

- Add `/v1/teams` and `/v1/teams/{teamID}/applications`.
- Implement team handlers over current storage rows.
- Add routeauthz and AAA tests for team actions.
- Use only team hierarchy routes in UI, CLI, MCP, and GitOps clients.

Status: implemented as a `teams`-backed adapter. Mutating team/application
routes perform handler-level AAA checks against the resolved team resource.

### Phase 3: Persistence Split

- Create `teams` and `applications`.
- Backfill from `teams.kind`.
- Keep team leaf-name uniqueness scoped to the same parent.
- Add `team_id` and `application_id` to resources and runs.
- Change run resolution so repository-triggered runs never auto-create teams.
- Add ownership snapshots to new runs.

### Phase 4: Team AI Profiles

- Add scoped profile storage and policy validation.
- Add team GitOps import/export for `ai-profiles.yaml`.
- Add team profile tabs and permission-gated forms.
- Update runtime profile resolution and hosted MCP profile reads.
- Add monitoring dimensions for profile scope and team.

Status: scoped storage, validation, REST APIs, routeauthz mappings, frontend
API helpers, runtime launch resolution, CLI catalog metadata, team GitOps
import/export, and profile editor tabs are implemented. Monitoring dimensions
remain future work.

### Phase 5: Canonical GitOps Rename

- Support `teams/<team>/team.yaml` as canonical.
- Reject non-team config repository hierarchy files.
- Export drift in the new canonical format.
- Deprecate legacy team layout after a documented transition window.

## Test Coverage

Required coverage before enabling the new path by default:

- unit tests for team/application normalization, profile resolution, and
  policy validation
- API handler tests for team CRUD, app CRUD, GitOps, notifications, and AI
  profiles
- routeauthz and AAA policy tests for every new action
- migration/backfill tests from `teams.kind = 'team'|'app'`
- run ownership resolution tests proving runs do not create teams/apps
- config sync and drift tests for `teams/<team>/team.yaml` and
  `ai-profiles.yaml`
- UI component tests for Teams tabs and Pipeline Runs removal of admin actions
- e2e coverage for create team, assign app, configure GitOps, configure
  profiles, run pipeline, and inspect monitoring
- hosted MCP tests for team visibility, profile reads, proposal tools, and
  mutation confirmation

## Acceptance Criteria

The redesign is done when:

1. Teams are created only from Teams, setup, API, or GitOps.
2. Pipeline Runs contains no organization administration controls.
3. Running a pipeline or ingesting a webhook never creates a team/application.
4. Empty teams no longer appear under every resource tree by default.
5. Applications are separate from teams and can reuse names under different
   teams.
6. Resources and runs have stable `team_id` ownership.
7. Team rename/move does not break run history, notifications, GitOps, access,
   or profile resolution.
8. Team admins can manage allowed LLM, Agent, and MCP profiles for their team.
9. System admins retain global policy, provider/server registry, defaults, and
   deny control.
10. GitOps drift/write can round-trip team structure and team AI profiles.
11. Monitoring, hosted MCP, and CLI route metadata are team-aware.
