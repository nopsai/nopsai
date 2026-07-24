# UI Source Ownership

Use this map when adding or moving UI code. The roadmap in
`doc/enterprise-refactor-roadmap.md` remains the enterprise status source of
truth; this file is the source-adjacent placement guide.

## Placement Rules

- `app/` owns shell composition, route wiring, sidebar state, desktop
  collapse/resize behavior, top-level navigation topic grouping, navigation
  trees, setup redirect behavior, and run-sidebar orchestration.
  HashRouter URLs must be canonicalized by app-shell code so stale pre-hash
  paths such as `/system/config?...#/dashboards` are replaced with root hash
  routes.
  Primary sidebar categories remain collapsible app-shell state: Organization
  follows Build & Automate, System Settings owns config/setup/data-management
  links, and only System Settings starts collapsed when no system route is
  active.
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
  toast/live-region feedback should start here. Shared in-page tree resizing
  behavior belongs in `components/resizableTreeColumn.tsx`; feature modules own
  only their default/min/max widths and layout placement.
- `styles.css` owns global theme tokens, app-shell dark surfaces, sidebar
  selected states, and cross-route color aliases. Feature CSS should consume
  those tokens or define scoped aliases instead of hard-coding a competing dark
  palette. Dark mode uses the softer enterprise palette from `--page-bg`,
  `--sidebar-bg`, `--card-bg`, `--input-bg`, `--border`, and `--accent`;
  avoid reintroducing near-black page glows or pure-white primary text.
  App chrome and feature split panes should avoid permanent divider lines for
  rails, headers, and resizers; use spacing, surface contrast, elevation, or
  hover/focus-only resize handles while keeping functional form, table, card,
  alert, and status borders intact.
  The global `[data-page]` active/hidden contract must preserve route-root
  display classes such as `flex` and `grid`; feature pages should not add local
  workarounds for active page display.
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

- Scope and trigger route identifiers, repository-owner grouping, source labels,
  usage indexes, manifest validation, and modal mutation state belong in their
  feature modules.
- `features/event-automation/EventAutomationSwitch.tsx`,
  `EventAutomationToolbar.tsx`, `AutomationResourceTree.tsx`, and
  `resourceTreeModel.ts` own the rendering-only route switch, shared page
  header, and reusable team-tree browser between trigger, external API trigger,
  and Git webhook source pages. The reusable tree is collapsed by default and
  user-resizable while keeping selected-resource ancestry open. These components
  link to each owning page and must not duplicate API, model, or mutation state.
- `features/triggers/model.ts` owns trigger collection metrics, source/search
  filtering, repository-owner membership rules, and structured trigger metadata
  helpers that read/write GitOps-compatible root YAML fields. `features/triggers/treeModel.ts`
  owns trigger tree shaping, lookup, team-under-owner grouping, and nested counts.
  `TriggerCollectionToolbar.tsx`, `TriggerExplorerTree.tsx`,
  `TriggerCollectionList.tsx`, and `TriggerDetailView.tsx` own the demo-style trigger workspace rendering:
  compact event-automation switch/filter/create toolbar, collapsed-by-default and user-resizable explorer tree, subtree
  metrics/table list, and selected-trigger routes that keep the explorer visible
  while showing overview, definition, and recent runs together on one aligned page.
  `useTriggerManifestMutations.ts` owns trigger create/save/delete mutation state.
  `pages/Triggers.tsx` owns URL selection, hook orchestration, dialog composition,
  and mutation wiring only.
- `features/external-triggers/model.ts` owns external trigger metrics,
  run-team tree item shaping, team membership filtering, and search filtering.
  `ExternalTriggerWorkspace.tsx` owns the demo-style external API trigger tree,
  metrics, table list, full-page endpoint detail, caller policy, curl example,
  and invocation history rendering. `pages/ExternalTriggers.tsx` owns URL
  selection, reference-data loading, route-local legacy transport, form modal
  composition, team selection state, and mutation wiring.
- `features/git-webhook-sources/model.ts` owns Git webhook source metrics,
  team ownership tree item shaping, global fallback labeling, request shaping,
  and search filtering. `GitWebhookSourcesWorkspace.tsx` owns the demo-style
  source tree, metrics, table list, full-page routing/access detail, credential
  reference, and delivery history rendering. `useGitWebhookSources.ts` owns hook
  orchestration, team option loading, and mutation state.
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
  mutation handlers, selected-team summary loading, and modal composition for
  GitOps, notifications, and drift review.
- `features/teams/model.ts` owns pure hierarchy, subtree metric, filtering,
  timestamp, parent, and kind-label rules for the Teams workspace.
- `features/teams/workspaceModel.ts` owns UI-only tab metadata and table copy
  helpers for the Teams workspace.
- `features/teams/resourceCatalogModel.ts` owns pure linked-resource
  normalization, ownership filtering, and route building for resources listed
  from a Teams overview. Repository-backed resources such as triggers keep
  repository identity for matching and links, while `teamPath` remains the
  ownership scope used by team/application catalogs; trigger ownership must not
  be inferred from the Git `owner/repo` slug.
- `features/teams/TeamsWorkspace.tsx` owns master-detail composition, toolbar,
  collapsed-by-default and user-resizable tree navigation, high-level resource cards, shared
  `ObjectIcon` resource identity, and responsive layout hooks.
- `features/teams/TeamsWorkspacePanels.tsx` owns detail-tab panels,
  team/application overview rendering, GitOps/notification summaries, read-only
  access summaries, resource tables, empty states, and table copy helpers.
- `features/teams/hooks/useTeamOperationsSummary.ts` owns selected-team
  GitOps, notification, AI profile, and access-grant summary orchestration.
- `features/teams/hooks/useTeamResourceCatalog.ts` owns selected-scope catalog
  loading for pipelines, steps, trigger sources, schedules, knowledge context,
  scopes, and credentials. API responses remain permission-filtered by their
  owner pages, and Teams only links to visible resources. Public/root resources
  are included inside team scopes so teams can discover shared resources without
  duplicating ownership. The Teams overview shows those resource types as
  category boxes in the Resources section and lists individual items in
  profile-style summaries below the boxes. Applications and AI profile rows are composed in
  `TeamsWorkspace.tsx` from the selected team scope and operations summary so
  the top-level tab strip does not duplicate Applications or AI Profiles.
- Team overview merges the previous activity signal into the overview card:
  it shows Applications, owners, and latest application run context, without a
  separate range selector, repository count, or repeated activity card.
- `features/teams/teams.css` owns the scoped Teams workspace styling.
- Team and application create/edit forms live in `features/teams/TeamSettingsModals.tsx`;
  parent-option and hierarchy safety shaping lives in `features/teams/model.ts`;
  request routing for team creation, team updates, app creation, and app moves lives in
  `features/teams/api.ts`; `pages/Teams.tsx` only orchestrates the mutation,
  refresh, and route update.
- Team settings configure GitOps repositories and notification routes only; AI
  profiles and access are summarized from Teams and linked to their owning
  pages so GitOps, AAA, and profile ownership stay compatible with the rest of
  the enterprise UI.
- Navigation-only team nodes are still valid team scopes for GitOps and
  notifications; application nodes are the scopes that cannot own team config
  repositories.
- Application pages use a single Overview tab that combines ownership details,
  repository metadata, related-run navigation, and app-matched resources such
  as triggers while keeping activity dashboards, GitOps, notifications, and
  team access tabs out of the app-specific surface.
- Teams Access shows current-session access roles, matching scoped basic roles,
  and effective scope checks separately. Access role editing, basic role grants,
  advanced role definitions, and policy editing remain owned by System Access.
- The Teams root links GitOps to `/system/config`; it does not own a separate
  team-scoped repository from the global system config repository.
- The Teams root summarizes global LLM, agent, and MCP profiles plus
  platform-wide admin grants; team rows continue to summarize only their
  team-scoped profiles and access grants.
- Notification routes are team-scoped; the Teams root does not show a root
  notification policy editor.

### Pipeline Runs

- Run list presentation, selected-run detail, graph rendering, graph dialogs,
  notification-route UI, and log dialogs belong under `features/pipeline-runs`.
- `pages/PipelineRuns.tsx` owns route composition, polling, query
  synchronization, and run mutation orchestration only. Source/status/search
  filters remain URL-backed for shareable operations views.
- `features/pipeline-runs/overviewModel.ts` owns overview metrics, source and
  status filtering, team/application navigation shaping, and table row shaping.
  The repository-backed source kind remains `repository` for API/query
  compatibility, but user-facing Pipeline Runs filters and tables present it as
  Application.
- Pipeline Runs list rows stay one-line-per-run: status, run name, repository,
  branch, 8-character run ID, trigger, final-output status, LLM usage, latest
  activity, and action controls. Additional diagnostics belong in detail, graph,
  or feed surfaces.
- Run final outputs stay under `features/pipeline-runs`: `finalOutputs.ts`
  owns dashboard-target/link rules and run-list output-status presentation,
  `RunFinalOutputs.tsx` owns clickable output cards and actions, and
  `final-output-preview/` owns artifact previews, including dashboard-spec
  rendering through the dashboard block renderer.
- `features/pipeline-runs/runPresentation.ts` owns run timestamp parsing and
  relative-time labels. UI code must treat Go zero `time.Time` values such as
  `0001-01-01T00:00:00Z` as unset, show `—` for the Started field, and use
  valid activity timestamps only for sorting or latest-run summaries.
- The Pipeline Runs overview `All teams` selection loads the accessible recent
  run list without a `teamId` filter; selecting a specific team/application uses
  the branch-grouped team endpoint.
- `features/pipeline-runs/PipelineRunsOverview.tsx` owns the redesigned
  overview rendering: user-resizable team/application rail, metrics, and run feed. It must
  consume existing run/team API data rather than sample data.
- The overview rail should keep its top aligned with the metrics row and
  preserve the user-controlled team/application expand/collapse tree while
  switching teams or source/status filters. Metric drill-downs, including Needs
  attention, must remain URL-backed so operations views are shareable.
- Selected application scopes can add view-local branch filtering over the
  already loaded runs; branch option derivation belongs in the overview model.
- The app shell owns the main sidebar's pipeline-run contextual section. The
  overview tab should leave that section absent; Recent runs and Events keep the
  contextual run/team tree. Shared trigger IDs should remain on overview rows so
  related pipelines highlight consistently with All runs.
- The app sidebar is always rendered as a dark surface. Pipeline-run sidebar
  cards and team trees must use `pipeline-runs-sidebar-context` scoped colors
  instead of page light-mode tokens, and the Pipeline Runs route should keep one
  owned vertical scroll container under the sticky filter bar.
- All runs list mode should stay a compact one-line-per-run summary. Keep
  expanded diagnostics in grid/detail surfaces or row titles so the operations
  list remains scannable.
- Legacy run-log hash routes are compatibility contracts. Preserve hydration and
  route synchronization when changing log filters, wrapping, structured view,
  agent/all-source view, or full/short display modes.
- Task log opens must seed explicit step and task filters, clearing stale
  modal filters so pipeline-run task evidence is not hidden by an older hash
  state. The log hook/model owns those filter semantics.
- Graph controls and log dialogs must keep keyboard paths, labelled controls,
  and serious/critical axe gates green.
- Clicking a pipeline-run or pipeline-definition graph step should preserve the
  step expansion affordance while framing that step with its direct dependency
  and dependent neighbors when the step has tasks. Clicking the expanded step
  again must collapse it and clear the selected step. Clicking a step with no
  displayable tasks should open logs filtered to that step. Manual graph
  navigation after a click, including wheel zoom, pan, and zoom controls, must
  clear the transient selected-step focus so later run refreshes do not jump
  back to that step. Graph rendering should ignore single placeholder task rows
  that only repeat the step name when the pipeline definition has no matching
  task.

### Lab

- Dependency preview, variable overrides, session state, run authorization, and
  run mutation behavior belong under `features/lab`.
- Run submission must re-check resource authorization for the selected pipeline
  and scope before launch.
- Autocomplete metadata should remain keyed to the active scope and editor
  context.

### Assistant

- `features/assistant/pageContext.ts` owns route-derived assistant context:
  page label, route pattern, tab, team/scope, selected resource IDs, and
  allow-listed filters. It must stay metadata-only and must not scrape rendered
  page text, logs, secrets, credentials, or arbitrary query parameters.
- `features/assistant/api.ts` owns assistant request/response transport,
  including optional `page_context` payload shaping. `model.ts` owns persisted
  conversation/message normalization and display helpers.
- `features/assistant/useAssistantController.ts` owns chat orchestration,
  scoped LLM-profile loading, conversation creation, send/retry recovery, and
  page-context attachment. `AssistantPanel.tsx` owns rendering and local
  dismissal state for the composer context chip.
- `components/AssistantDock.tsx` composes the floating entry point and passes
  current route context. `pages/Assistant.tsx` composes the full-page route and
  accepts only sanitized route state from the dock.

### Analysis

- `features/analysis/model.ts` owns deterministic reviewer types, score
  provenance, resource checks, pipeline checks, run diagnosis, redaction, and
  baseline copy-report formatting. It also owns the canonical analysis scope
  path sent with AI Evaluation requests. Health and category scores must explain
  their baseline, severity weights, visible inputs, and deductions.
- `features/analysis/ai.ts` owns subject-specific AI Evaluation prompts, the
  redacted prompt snapshot, page context, and structured AI Evaluation parsing.
  It must not include raw secrets, credential values, or private logs, and the
  modal must render parsed sections and scored finding impacts rather than raw
  model output.
- `features/analysis/api.ts` owns usable LLM-profile selection plus calls to
  the authenticated `/v1/analysis/evaluate` endpoint. Analysis must not call
  model providers directly from the browser, and it must not create Assistant
  conversations just to evaluate the reviewer snapshot. Analysis must select the
  configured default from the unscoped Assistant profile picker, then pass the
  team/resource scope path to the evaluation endpoint for backend validation.
- `features/analysis/reviewedScore.ts` owns the AI-reviewed score overlay:
  converting structured scored findings into active health/category scores while
  preserving deterministic score provenance as the baseline.
- `features/analysis/evaluationCache.ts` owns browser-local AI review history
  keyed by subject type, subject ID, and exact snapshot revision.
- `features/analysis/useAnalysisAiEvaluation.ts` owns loading, retry, stale
  snapshot protection, cached-review hydration, and automatic run-analysis AI
  evaluation.
- `features/analysis/AnalysisModal.tsx` owns rendering only: left-rail health
  basis, hoverable metric scores, structured AI Evaluation state, focused
  findings, evidence expansion, recommendations, and safe navigation/copy
  actions.
- `features/pipelines/pipelineAnalysisEvidence.ts` owns pipeline-only prompt
  context: bounded redacted YAML, validation errors, parsed step/task graph,
  trigger bindings, dependencies, and recent-run summaries for AI Evaluation.
- `features/teams/teamAnalysisEvidence.ts` owns team/resource-only prompt
  context: visible resource rows, selected-resource peers, resource
  distribution, GitOps/notification/AI-profile/access metadata, and loader
  limitations for AI Evaluation.
- `features/pipeline-runs/runAnalysisEvidence.ts` owns run-only prompt context:
  bounded, tail-preserving redacted log excerpts, failed step/task command
  context, and YAML excerpts for AI Evaluation. Generic analysis modules should
  consume this as optional prompt context rather than fetching run logs
  themselves.

### Dashboards

- Dashboard route composition lives in `pages/Dashboards.tsx`. Keep dashboard
  model rules, response normalization, API transport, and block rendering under
  `features/dashboards`.
- `features/dashboards/model.ts` owns dashboard/source/publication/refresh
  schedule types, ref/slug helpers, form defaults, stale labels, and
  publication grouping.
  `api.ts` owns the `/v1/dashboards` transport. `blocks/` owns presentation for
  validated `DashboardSpec` blocks only, including chart and series rendering.
- Dashboard UI must not render arbitrary generated HTML. It renders structured
  `status`, `text`, `callout`, `list`, `properties`, `table`, `progress`, and
  `link` blocks plus validated chart/series blocks returned by the API and
  keeps run-output downloads out of the dashboard workflow.

### System

- System tab panels own their domain UI under `features/system`.
- Access-specific catalogs, policy fields, grant editors, token panels,
  confirmation dialogs, resource catalogs, and presentation helpers belong under
  `features/system/access`.
- Runtime config, dispatcher, data management, setup, access, and logs stay
  under the System route.
- Credentials are a first-class left-navigation route composed by
  `pages/Credentials.tsx`; model/API/hook/rendering code stays under
  `features/system/credentials`. `model.ts` owns credential reference parsing,
  dashboard grouping, filtering, and recent-update sorting; `api.ts` owns the
  `/v1/system/credentials` contract; `useCredentials.ts` owns load, create,
  rotate, enable, disable, delete, stale detail-request cancellation, and
  detail-selection orchestration; renderer
  files such as `CredentialDashboard.tsx`, `CredentialCatalog.tsx`,
  `CredentialCreateForm.tsx`, and `CredentialDetail.tsx` own presentation only.
  The create flow derives admin-only global credentials as
  `credential://system/...` and team credentials as
  `credential://team/<team path>/...` from the selected team scope. Non-admin
  users only render team-scoped credentials returned by the API, and their
  create form is anchored to available team paths. The catalog uses known team
  paths to present credentials in a scope-grouped registry table (`System`,
  `Global`, or the team path) with credential categories such as `LLM`, `Mail`,
  and `GitHub` surfaced as table metadata. The catalog renderer owns
  credentials-local filters, tabs, grouping controls, and selected-row
  presentation; `CredentialDetail.tsx` owns the
  slide-out detail drawer, rotation form, and version history; route composition
  owns URL selection. Team references that repeat the selected team path are
  normalized for display and create previews without changing the
  GitOps-compatible reference format.
- Credential catalog rows stay single-line registry entries. Descriptions,
  parent paths, category hints, and secret metadata belong in the detail drawer
  or explicit columns, not under the credential name.
- LLM profiles, agent profiles, and MCP are first-class workspace routes. Their
  model/API/hook/panel code can remain under `features/system` while the route
  wrappers live in `pages/`. Page visibility is topic-level: global system
  permissions and scoped team/product grants can show the route, while the
  backend list handlers filter individual LLM profiles, agent profiles, MCP
  servers, and MCP profiles by resource access before returning subjects.
- `features/system/AIResourcePanel.tsx`, `features/system/AIResourceWorkspace.tsx`,
  `features/system/aiResourcePanel.css`, `features/system/aiResourceTree.ts`,
  and `features/system/aiResourcePresentation.ts` own the shared trigger-style
  AI resource workspace shell, borderless outer workspace frame, resizable team
  tree, inline overview metrics, selected-resource detail mode, search, count,
  labeled resource rows, compact icon actions, team placement controls, and
  responsive presentation for LLM, agent, and MCP resource pages. Domain panels
  still own filtering inputs,
  mutations, and detail composition, while focused renderers such as
  `MCPViewSwitch.tsx`, `MCPResourceTables.tsx`, and detail helpers such as
  `MCPDetailSection.tsx` own domain-specific tabs, rows, and repeated detail
  structure.
- AI resource tables keep resource-name cells to primary names only. LLM,
  Agent, and MCP descriptions, base URLs, transports, and profile notes belong
  in explicit columns or detail panels; MCP server list scope coverage is shown
  as allowed scopes. Registry table typography, monospace metadata cells, and
  status text should stay aligned with the Triggers resource table.
- Individual LLM profiles, agent profiles, MCP servers, and MCP profiles share
  access through `ResourceAccessCard` with `llm_profile`, `agent_profile`,
  `mcp_server`, and `mcp_profile` resource types.
- New LLM profiles, agent profiles, MCP servers, and MCP profiles use the same
  slash path placement as pipelines: `team/subteam/name` belongs to
  `/team/subteam`, inherits parent team access, and remains global when no team
  prefix is present.
- Global default selectors stay tied to global system profile defaults. When
  the current default is outside the viewer's allowed resource set, the API
  returns an explicit empty `default_profile` and the UI renders the value as
  unavailable instead of guessing that a team-scoped subject is the default.
- System workflows that generate GitOps commands or deployment snippets must
  preserve copyable, deterministic output.

### Knowledge Context, Monitoring, And Schedules

- Knowledge Context model rules own document identifiers, team trees, draft
  handling, GitOps source labels, source-kind summaries, connection provider
  labels/status labels, connection form validation, Documents/Connections tab
  normalization, subtree document collection, source filter matching,
  external-page sync/failure labels, provider page result/preview types,
  persisted team-owned connection summaries, and route encoding.
- `features/knowledge-context/api.ts` owns Knowledge Context document transport
  and Knowledge Context connection transport, including provider page search,
  URL/page resolution, external-page save, and manual sync payloads. It must
  keep credential values out of UI response types; connection create/update
  payloads may send credential references only.
- `pages/KnowledgeContext.tsx` owns URL selection, loading, mutation
  orchestration, document/connection mutation state, and toast wiring only.
  `KnowledgeContextWorkspace.tsx` owns Documents/Connections composition,
  demo-style toolbar and browser shell, keyboard search focus, source filter
  controls, branch collection table, and browser navigation.
  `KnowledgeContextConnectionsView.tsx` owns provider/team connection
  rendering and connection action buttons, `KnowledgeContextDetailView.tsx`
  owns selected document rendering and local detail tabs, and
  `KnowledgeContextModals.tsx` owns document create/clone/delete, external page
  search/preview rendering, and connection create dialogs.
- Knowledge document collection rows stay single-line name entries; document
  path and source details remain in table columns or the selected detail panel.
- Monitoring model rules own metric normalization and display teaming.
- Schedules model/API files own cron mode normalization, schedule request
  shaping, metadata normalization, and schedule transport.
- New route-level growth in these areas should first look for a tested
  feature-owned model, API, hook, or presentation boundary.

### Product Docs

- `pages/ProductDocs.tsx` owns documentation route selection, query
  synchronization, and article scroll restoration. Article/topic changes should
  scroll the authenticated app content wrapper (`#page-content-wrapper`) when it
  exists, with `window.scrollTo` kept only as the standalone rendering fallback.
- Product docs content, search quality, article metadata, and evidence rules
  remain owned by `features/product-docs`.

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
