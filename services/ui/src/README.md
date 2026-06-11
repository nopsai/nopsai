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
  handling, and toast/live-region feedback should start here.
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
- Git-managed resources stay read-only in the editor; clone/customize flows are
  the supported write path.

### Scopes And Triggers

- Scope and trigger route identifiers, grouping, source labels, usage indexes,
  manifest validation, and modal mutation state belong in their feature modules.
- GitOps-managed scoped values and trigger manifests must keep read-only affordances
  and explicit clone/customize paths.
- Destructive actions require action-time AAA checks and alert-dialog semantics.
- GitOps secret encryption must remain compatible with config-repository
  workflows and avoid exposing plaintext after encryption.

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
- Runtime config, dispatcher, data management, setup, LLM profile, and MCP
  behavior should stay in their existing system subfeatures.
- System workflows that generate GitOps commands or deployment snippets must
  preserve copyable, deterministic output.

### Knowledge Context, Monitoring, And Schedules

- Knowledge Context model rules own document identifiers, folder trees, draft
  handling, GitOps source labels, and route encoding.
- Monitoring model rules own metric normalization and display grouping.
- Schedules model/API files own cron mode normalization, schedule request
  shaping, metadata normalization, and schedule transport.
- New route-level growth in these areas should first look for a tested
  feature-owned model, API, hook, or presentation boundary.

## Accessibility And Test Ownership

- New dialogs should use `WorkflowDialogFrame` unless a documented product
  reason requires a specialized shell.
- New validation feedback should use `WorkflowInlineAlert` or an equivalent
  `role="alert"` relationship included in `aria-describedby`.
- New toast/live-region feedback should use `WorkflowToastRegion`.
- Icon-only common commands need accessible names and should use lucide-react.
- Pure model/API behavior gets unit tests; hooks and components get component
  tests; login, workspace, dialogs, autocomplete, graph, logs, and deployed
  smoke paths stay in Playwright.
