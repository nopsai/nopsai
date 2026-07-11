# UI Source Ownership

Use this map when adding or moving UI code. The roadmap in
`doc/enterprise-refactor-roadmap.md` remains the enterprise status source of
truth; this file is the source-adjacent placement guide.

## Placement Rules

- `app/` owns shell composition, route wiring, sidebar state, navigation trees,
  setup redirect behavior, and run-sidebar orchestration.
- `auth/` owns session state, current-user loading, capability normalization,
  route guards, and redirect safety.
- `lib/api.ts` owns API base URL resolution, token persistence, bearer-token
  attachment, refresh retry behavior, and the `apiClient` transport boundary.
- `features/<area>/model.ts` owns pure rules: parsing, normalization,
  identifiers, permission keys, validation, payload shaping, and display
  labels.
- `features/<area>/api.ts` owns endpoint calls and response/request mapping.
- `features/<area>/use*.ts` owns async orchestration, polling, mutation state,
  permission checks, and toast-facing failure handling.
- `features/<area>/*.tsx` owns feature rendering, accessibility semantics,
  keyboard behavior, and local interaction state.
- `pages/` should compose feature hooks/components and handle URL-level
  selection. Avoid new route-local transport, model logic, modal lifecycles, or
  large presentation branches.
- `components/` owns shared UI primitives that cross feature boundaries.
  Workflow dialogs, inline alerts, empty states, icon-only commands, focus
  handling, compact resource collection cards, shared object icons, and
  toast/live-region feedback should start here.
- `tools/` owns local and CI guardrails such as boundary checks. Runtime code
  should not depend on tool-only modules.

## Feature Contracts

### Editor, Pipelines, And Steps

- Keep YAML drafts, collection navigation, validation, autocomplete, workflow
  create/clone/delete dialogs, and shared editor affordances under
  `features/editor`.
- Pipeline- and step-specific API, model, usage, activity, and permission logic
  stays under `features/pipelines` and `features/steps`.
- Permission checks must be keyed to the active resource path/name and must fail
  closed when navigation changes.
- Git-managed workflow resources can be edited in the UI when AAA permits; those
  saves become database overrides and must warn that the next GitOps sync can
  replace them unless pushed back to GitOps.

### Scopes And Triggers

- Scope and trigger route identifiers, teaming, source labels, usage indexes,
  manifest validation, and modal mutation state belong in their feature modules.
- GitOps-managed trigger manifests use the same database-override behavior as
  pipelines and steps. Clone paths remain available for draft workflows.
- GitOps-managed scoped variables and secrets follow the same database-override
  rule: direct edits become database overrides and direct deletes remove the
  database row until GitOps sync replaces or recreates it.
- Destructive actions require action-time AAA checks and alert-dialog semantics.
- GitOps secret encryption must remain compatible with config-repository
  workflows and avoid exposing plaintext after encryption.

### Teams

- `pages/Teams.tsx` owns URL-level selection, Teams API loading, create/delete
  mutation handlers, and modal composition for GitOps, notifications, AI
  profiles, and drift review.
- `features/teams/model.ts` owns pure hierarchy, subtree metric, filtering,
  timestamp, parent, and kind-label rules for the Teams workspace.
- `features/teams/workspaceModel.ts` owns UI-only tab metadata and table copy
  helpers for the Teams workspace.
- `features/teams/TeamsWorkspace.tsx` owns master-detail composition, toolbar,
  tree navigation, high-level summary cards, and responsive layout hooks.
- `features/teams/TeamsWorkspacePanels.tsx` owns detail-tab panels, scoped
  activity cards, resource tables, empty states, and table copy helpers.
- `features/teams/teams.css` owns the scoped Teams workspace styling.
- Team settings, notification routes, AI profile editors, and config repository
  orchestration remain in their existing `features/teams` modules so GitOps and
  AAA behavior stay compatible with the rest of the enterprise UI.

### Pipeline Runs

- Run list presentation, selected-run detail, graph rendering, graph dialogs,
  notification-route UI, and log dialogs belong under `features/pipeline-runs`.
- Legacy run-log hash routes are compatibility contracts. Preserve hydration and
  route synchronization when changing log filters, wrapping, structured view,
  agent/all-source view, or full/short display modes.
- Graph controls and log dialogs must keep keyboard paths, labelled controls,
  and serious/critical axe gates green.

### Lab

- Dependency preview, variable overrides, session state, run authorization, and
  run mutation behavior belong under `features/lab`.
- Run submission must re-check resource authorization for the selected pipeline
  and scope before launch.
- Autocomplete metadata should remain keyed to the active scope and editor
  context.

### System

- System tab panels own their domain UI under `features/system`.
- Access-specific catalogs, policy fields, grant editors, token panels,
  confirmation dialogs, resource catalogs, and presentation helpers belong under
  `features/system/access`.
- Runtime config, dispatcher, data management, credentials, setup, access, and
  logs stay under the System route.
- LLM profiles, agent profiles, and MCP are first-class workspace routes. Their
  model/API/hook/panel code can remain under `features/system` while the route
  wrappers live in `pages/`, because the backend capabilities still use the
  existing system profile permissions.
- `features/system/AIResourcePanel.tsx`, `features/system/aiResourcePanel.css`,
  and `features/system/aiResourcePresentation.ts` own the shared hero, stats,
  search, count, labeled resource rows, compact icon actions, split profile
  detail layouts, and responsive presentation for LLM, agent, and MCP resource
  pages; domain panels still own filtering inputs, mutations, and side-panel
  rendering.
- Individual LLM profiles, agent profiles, MCP servers, and MCP profiles share
  access through `ResourceAccessCard` with `llm_profile`, `agent_profile`,
  `mcp_server`, and `mcp_profile` resource types.
- System workflows that generate GitOps commands or deployment snippets must
  preserve copyable, deterministic output.

### Knowledge Context, Monitoring, And Schedules

- Knowledge Context model rules own document identifiers, team trees, draft
  handling, GitOps source labels, and route encoding.
- Monitoring model rules own metric normalization and display teaming.
- Schedules model/API files own cron mode normalization, schedule request
  shaping, metadata normalization, and schedule transport.
- New route-level growth in these areas should first look for a tested
  feature-owned model, API, hook, or presentation boundary.

## Accessibility And Test Ownership

- `AppShell` owns the single route-level `h1` for authenticated pages. Page
  content must not repeat the route title; distinct hero copy starts at `h2`.
  Standalone routes such as Login own their own `h1`.
- Create and edit forms should use `WorkflowFormDialog`, which owns the
  Pipeline modal card, header, scrollable body, footer, focus, and animation
  contract. Lower-level or destructive dialogs should use `WorkflowDialogFrame`
  unless a documented product reason requires a specialized shell.
- Feature create actions remain visible when AAA makes a collection read-only;
  disable them and explain the restriction instead of removing the control.
- Dense resource collections should use `CompactResourceCard` in a responsive
  tile grid and the same glass surface, icon treatment, hover state, spacing,
  and divider metadata used by Pipelines and Pipeline Runs. Shared icon-led
  identity, descriptions, facts, footer status tags, and action slots keep
  hierarchy consistent without repeating the collection's resource type on each
  card. Feature modules own essential facts, status labels, actions, and
  GitOps/AAA decisions. Resource identity glyphs should come from
  `components/ObjectIcon.tsx` and `components/objectIconRegistry.ts`; new
  object types must extend that registry and its focused component test instead
  of adding inline SVGs or feature-local icon switches.
- Collection routes should reuse `ResourceCollectionToolbar` for compact search,
  refresh, create, summaries, and feature-owned filters. Create controls remain
  visible but disabled when AAA grants read-only access. Secondary detail panes
  should mount only after a resource is selected or deep-linked.
- Form dialogs share the Pipeline themed surface and independently scrollable
  body; collection card effects such as `glass-card` are not dialog shells.
- New validation feedback should use `WorkflowInlineAlert` or an equivalent
  `role="alert"` relationship included in `aria-describedby`.
- New toast/live-region feedback should use `WorkflowToastRegion`.
- Icon-only common commands need accessible names and should use lucide-react.
- Pure model/API behavior gets unit tests; hooks and components get component
  tests; login, workspace, dialogs, autocomplete, graph, logs, and deployed
  smoke paths stay in Playwright.
