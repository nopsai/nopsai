# Enterprise UI Refactor Roadmap

This document tracks the UI clean-code work needed to keep NopsAI maintainable as an enterprise control plane.

## Status

- The refactor pass described under **Completed Roadmap Targets** is complete and passes the documented UI gates.
- The broader enterprise UI refactor is not complete. The prioritized follow-on work below remains, but no release-blocking regression was found during the June 7, 2026 review.

## Completed In Current Pass

- `src/App.tsx` is now a small router/bootstrap wrapper.
- `src/app/AppShell.tsx` owns composition, not auth payload normalization or resource-tree construction.
- `src/auth/*` owns session loading, current-user state, capability normalization, derived access decisions, and route guard rendering.
- `src/lib/api.ts` now exposes an explicit `apiClient` with bearer-token attachment and refresh retry handling. UI API calls use this client instead of relying on a global `window.fetch` override.
- `src/app/useResourceTrees.ts` and `src/app/resourceTrees.ts` own contextual sidebar resource loading and tree normalization.
- `src/app/useSidebarState.ts` owns sidebar width, mobile open/close state, and resize behavior.
- `src/features/system/api.ts` provides the shared System-area JSON request helper on top of `apiClient`.
- `src/features/system/config/model.ts`, `api.ts`, and `useSystemConfig.ts` own runtime config, notification mail settings, global config repository loading/mutations, drift checks, and Git push orchestration.
- `src/features/system/access/useSystemAccess.ts` owns core Access entity loading, mutations, form state, token workflows, role-policy persistence, and toast-facing errors.
- `src/features/system/access/model.ts` owns Access role constants, policy/grant normalization, protected-role checks, and grant display helpers shared by the hook and panel.
- `src/features/system/access/resourceCatalog.ts` and `api.ts` own resource-option normalization and loading for Access policy editors, including malformed and cyclic group protection.
- `src/features/system/dispatcher/useSystemDispatcher.ts` owns Dispatcher tab status loading, polling, runner action state, runner dispatch toggles, and runner-guide anchor handling.
- `src/features/system/dispatcher/model.ts` owns dispatcher status normalization, runner metadata parsing, and runner action keys shared by the hook and panel.
- `src/features/system/dispatcher/useRunnerDeploymentGuide.ts` and `api.ts` own runner scope loading, deployment form state, request validation, and Docker/Kubernetes install-template generation.
- `src/features/system/llm-profiles/model.ts`, `api.ts`, and `useLLMProfiles.ts` own LLM profile normalization, form/payload mapping, profile CRUD, default-profile persistence, delete-conflict handling, and profile tests.
- `src/features/system/mcp/model.ts`, `api.ts`, and `useMCPRegistry.ts` own MCP registry normalization, server/profile form mapping, headers parsing, tool selection, registry CRUD, discovery, profile tests, and mutation state.
- `src/features/system/setup/model.ts` and `api.ts` own first-install wizard types, step metadata, repository/runtime normalization, setup status/template/bootstrap calls, and setup helper tests.
- `src/features/system/SetupWizard.tsx` is now feature-owned by the System area instead of living under `pages/`.
- `src/features/system/data-management/model.ts`, `api.ts`, and `useDataManagement.ts` own backup/cleanup/schedule types, cleanup payload mapping, data-management API calls, mutation state, and cleanup helper tests.
- `src/features/system/DataManagementPanel.tsx` is now feature-owned by the System area instead of living under `pages/`.
- `src/features/editor/autocomplete.ts` owns shared editor autocomplete metadata loading for secrets, variables, scopes, reusable steps, LLM profiles, and MCP profiles.
- `src/features/pipelines/model.ts` owns pipeline directive catalogs, source/identifier helpers, YAML validation, YAML-to-detail parsing, and pipeline dependency graph data normalization.
- `src/features/pipelines/api.ts` owns pipeline list/detail/save/delete requests, permission checks, recent-run loading, trigger discovery, and list payload normalization.
- `src/features/steps/model.ts` owns step directive catalogs, identifier/source helpers, timestamp formatting, and strict step YAML validation.
- `src/features/steps/api.ts` owns reusable-step list/detail/save/delete requests, permission checks, usage loading, and API payload normalization.
- `src/features/schedules/model.ts` and `api.ts` own schedule types, cron/form conversion, metadata normalization, validation, payload mapping, and schedule lifecycle API calls.
- `src/features/knowledge-context/model.ts` and `api.ts` own knowledge document identities, routes, draft storage, tree/content normalization, validation, and CRUD calls.
- `src/features/triggers/model.ts` and `api.ts` own trigger manifest normalization, pipeline/run helpers, editor metadata, permission checks, list/detail loading, and trigger lifecycle calls.
- `src/features/scopes/model.ts` and `api.ts` own scope routes, repository-scoped identities, tree/list normalization, usage-analysis transport, clone naming, permission checks, scoped value lifecycle calls, and secret encryption.
- `src/features/pipeline-runs/api.ts` owns Pipeline Runs JSON, log, and folder config-repository transport behavior.
- `src/features/pipeline-runs/contracts.ts` owns shared run-list and graph layout contracts that were previously embedded in `pages/PipelineRuns.tsx`.
- `src/features/pipeline-runs/notificationRoutes.ts` owns legacy and current notification-route normalization, form state, editing, and API payload mapping.
- `src/features/pipeline-runs/graphLayout.ts` owns graph labels, run/task/edge statuses, deterministic layout, and cyclic dependency protection.
- `src/app/useInitialSetupRedirect.ts` owns first-install setup routing decisions.
- `src/app/runSidebarApi.ts` and `usePipelineRunsSidebar.ts` own recent/repository run loading, pagination, expansion, active-run synchronization, caching, and polling.
- `src/app/BaseSidebarNavigation.tsx` owns the stable application navigation list.
- `src/features/editor/EditorAutocompleteMenu.tsx` provides the shared autocomplete menu renderer used by workflow editors.
- `src/auth/AuthContext.ts` owns the auth context and `useAuth` hook so the provider component stays Fast Refresh friendly.
- The UI package has separate unit, component, mocked Playwright, and live-backend Playwright suites. The default `test` command runs unit and component coverage.
- Focused component tests cover access forms, config repository drift, stable sidebar navigation, editor autocomplete, and schedule save/delete modal flows.
- Mocked Playwright workflows cover login, required password-change redirect, pipeline save, first-install setup, access-controlled navigation, and a serious/critical Axe accessibility audit.
- `playwright.live.config.ts` provides credential-gated deployed-stack authentication and optional pipeline save/run smoke coverage without API mocks.
- Vitest V8 coverage is enforced with an initial 1% repository-wide floor. The June 7, 2026 baseline is 2.76% statements, 2.81% branches, 3.29% functions, and 2.98% lines.
- Login form labels are now programmatically associated with their inputs, preserving accessible label-driven automation and assistive-technology behavior.
- The login primary action now meets the automated color-contrast gate.
- Browserslist compatibility metadata was refreshed on June 7, 2026.

## Current Boundaries

- `auth/capabilities.ts`: payload normalization, `can(user, permission)`, app-level access derivation, system-page permission contract.
- `auth/useCurrentUser.ts`: current session and `/v1/auth/me` loading.
- `auth/AuthProvider.tsx`: auth context provider for the app shell.
- `auth/AuthContext.ts`: auth context object and `useAuth()` hook.
- `auth/permissionGuards.tsx`: reusable access-controlled route rendering.
- `lib/api.ts`: API URL resolution, session persistence, explicit auth-aware API client.
- `app/resourceTrees.ts`: pure tree builders for pipelines, triggers, steps, scopes, and knowledge contexts.
- `app/useResourceTrees.ts`: sidebar resource API loading and draft merge behavior.
- `app/useSidebarState.ts`: sidebar UI state.
- `features/system/api.ts`: shared JSON fetch behavior for System feature modules.
- `features/system/config/model.ts`: System config, global config repository, and mail settings types, defaults, payload builders, and normalizers.
- `features/system/config/api.ts`: System config, mail, and global config repository API client functions.
- `features/system/config/useSystemConfig.ts`: Config tab state, mutations, polling, drift modal state, and toast-facing errors.
- `features/system/SystemConfig.tsx`: Config tab rendering and local routing-row interaction state.
- `features/system/access/model.ts`: Access constants, role/grant helpers, and access payload normalization.
- `features/system/access/resourceCatalog.ts`: Access resource-option contracts, normalization, deduplication, sorting, and group-path construction.
- `features/system/access/api.ts`: Access resource catalog transport.
- `features/system/access/useSystemAccess.ts`: Access entity and resource-catalog API orchestration and state.
- `features/system/AccessPanel.tsx`: Access tab rendering and local modal interaction state.
- `features/system/dispatcher/model.ts`: Dispatcher status, runner metadata, deployment template, and runtime scope normalization.
- `features/system/dispatcher/api.ts`: Dispatcher scope and Docker/Kubernetes install-template transport.
- `features/system/dispatcher/useSystemDispatcher.ts`: Dispatcher status/action orchestration, polling, runner pending state, and guide anchor behavior.
- `features/system/dispatcher/useRunnerDeploymentGuide.ts`: Runner deployment form, scope loading, validation, and template generation state.
- `features/system/DispatcherPanel.tsx`: Dispatcher tab rendering and clipboard interaction state.
- `features/system/llm-profiles/model.ts`: LLM profile types, defaults, payload normalization, and form-to-API mapping.
- `features/system/llm-profiles/api.ts`: LLM profile API client functions for list, save, default-profile update, delete, and test calls.
- `features/system/llm-profiles/useLLMProfiles.ts`: LLM Profiles tab loading, mutation state, delete-conflict panel state, default selection, and toast-facing errors.
- `features/system/LLMProfilesPanel.tsx`: LLM Profiles tab rendering and local focus/scroll behavior.
- `features/system/mcp/model.ts`: MCP server/profile types, defaults, registry normalization, form-to-API mapping, header parsing, and tool selection helpers.
- `features/system/mcp/api.ts`: MCP registry API client functions for server/profile list, save, delete, discovery, and profile test calls.
- `features/system/mcp/useMCPRegistry.ts`: MCP tab loading, mutation state, active panel state, selected-tool state, and toast-facing errors.
- `features/system/MCPPanel.tsx`: MCP tab rendering and local focus/scroll behavior.
- `features/system/setup/model.ts`: First-install setup types, wizard step metadata, repository/runtime helpers, status class mapping, and provider defaults.
- `features/system/setup/api.ts`: Setup status, template preview/download, and bootstrap API client functions.
- `features/system/SetupWizard.tsx`: Setup wizard rendering, local wizard interaction state, env-file download helpers, and first-install modal behavior.
- `features/system/data-management/model.ts`: Backup, cleanup, schedule, form, count, date, and size helpers.
- `features/system/data-management/api.ts`: Backup, cleanup preview/run, schedule CRUD, schedule enable/disable, and schedule run API client functions.
- `features/system/data-management/useDataManagement.ts`: Data Management loading, mutation state, preview state, schedule form orchestration, and toast-facing errors.
- `features/system/DataManagementPanel.tsx`: Data Management rendering for backups, manual cleanup, schedules, and cleanup jobs.
- `features/editor/autocomplete.ts`: Shared editor autocomplete metadata normalization and API loading for pipeline and reusable step editors.
- `features/pipelines/model.ts`: Pipeline editor directive catalogs, pure validation, identifier/source normalization, YAML detail parsing, and dependency graph data normalization.
- `features/pipelines/api.ts`: Pipeline list/detail/save/delete API calls, permission checks, recent-run loading, trigger discovery, and typed payload normalization.
- `features/steps/model.ts`: Step editor directive catalogs, pure validation, identifier/source normalization, and timestamp formatting.
- `features/steps/api.ts`: Step list/detail/save/delete API calls, permission checks, usage loading, and typed payload normalization.
- `features/schedules/model.ts`: Schedule types, cron parsing/building, form mapping, metadata normalization, and payload validation.
- `features/schedules/api.ts`: Schedule list, metadata, save, enable/disable, run, and delete API calls.
- `features/knowledge-context/model.ts`: Knowledge identities, route helpers, draft storage, tree normalization, content previews, and validation.
- `features/knowledge-context/api.ts`: Knowledge list, detail, save, and delete API calls.
- `features/triggers/model.ts`: Trigger manifest, list, source, pipeline, and run normalization helpers.
- `features/triggers/api.ts`: Trigger permissions, list/detail/runs, editor metadata, pipeline YAML, save, create, clone, and delete API calls.
- `features/scopes/model.ts`: Scope identity, route, tree, item metadata, and clone-name normalization.
- `features/scopes/api.ts`: Scope permissions, catalogs, usage-analysis resources, variables/secrets CRUD, and secret encryption API calls.
- `features/pipeline-runs/api.ts`: Shared Pipeline Runs JSON, incremental logs, and folder config-repository requests.
- `features/pipeline-runs/contracts.ts`: Run-list and graph layout contracts shared by Pipeline Runs rendering.
- `features/pipeline-runs/notificationRoutes.ts`: Notification route contracts, legacy normalization, form editing, and payload mapping.
- `features/pipeline-runs/graphLayout.ts`: Run graph status derivation and deterministic layout.
- `app/useInitialSetupRedirect.ts`: Initial setup redirect orchestration.
- `app/runSidebarApi.ts`: Typed run-sidebar transport.
- `app/usePipelineRunsSidebar.ts`: Run-sidebar loading, caching, expansion, pagination, synchronization, and polling.
- `app/BaseSidebarNavigation.tsx`: Stable capability-aware top-level navigation.
- `features/editor/EditorAutocompleteMenu.tsx`: Reusable simple and grouped autocomplete rendering.

## Completed Roadmap Targets

1. The remaining workflow pages now delegate repeated transport and payload shaping to feature-owned `model.ts` and `api.ts` modules.
2. Component coverage exists for access forms, config repository drift, sidebar navigation, editor autocomplete, and modal save/delete flows.
3. Browser-level coverage exists for login, password-change redirect, pipeline save, system setup, access-controlled navigation, and automated login accessibility checks.
4. Pipeline run graph logic and notification route ownership now live under `features/pipeline-runs/` instead of the page module.
5. Access resource catalogs and Dispatcher deployment-guide requests now have feature-owned APIs, models, and hooks.
6. Setup redirect and run-sidebar orchestration now live outside `AppShell.tsx`.
7. Coverage reporting, an initial enforced baseline, and a credential-gated live-backend smoke suite are available.

## Remaining Enterprise Follow-On Work

These are not blockers for the completed pass, but they remain before the broader enterprise UI refactor should be called complete:

1. **Continue visual decomposition by product responsibility.** `PipelineRuns.tsx` is about 6,734 lines, `AccessPanel.tsx` about 3,838, and `AppShell.tsx` about 1,465. Data ownership has moved out, but logs, graph rendering, policy-editor sections, route composition, and other visual surfaces should become focused components when their product areas change.
2. **Continue workflow-page extraction.** `Scopes`, `Triggers`, `Pipelines`, `Steps`, `Lab`, and `Monitoring` still contain large stateful surfaces. Extract cohesive product workflows when changes touch them, while keeping behavior and routing stable.
3. **Ratchet coverage upward.** The 1% repository-wide threshold prevents loss of the initial baseline, but it is intentionally modest. Prioritize Pipeline Runs polling/log/graph behavior, Access policy and grant editing, sidebar run loading, and failure/permission paths, then raise thresholds with each tested extraction.
4. **Run the live suite in a deployed CI environment.** Authentication/setup smoke requires `NOPS_UI_LIVE_USERNAME` and `NOPS_UI_LIVE_PASSWORD`. The mutation smoke additionally requires `NOPS_UI_LIVE_MUTATION=true` and a dedicated `NOPS_UI_LIVE_PIPELINE_ID`; it has not been executed in this local review because those values were unavailable.
5. **Broaden accessibility coverage.** The login page has a serious/critical Axe gate. Add authenticated-page audits plus keyboard, focus, dialog, editor, navigation, graph, and log-view checks.

## Verification

From `services/ui`:

```bash
npm run lint
npm run test
npm run build
npm run test:e2e
npm run test:e2e:live
```

`test:unit` compiles pure model/API tests into `dist-test` and runs Node's test runner. `test:component` uses Vitest, Testing Library, jsdom, and V8 coverage. `test:e2e` uses Playwright with deterministic API mocks and a managed Vite server. `test:e2e:live` targets a deployed stack and skips explicitly when the required credentials or mutation fixture are unavailable.

From the repository root, the UI checks can also be run in the Docker Compose test service:

```bash
docker compose run --rm ui-test sh -c "npm ci && npm run lint && npm run test && npm run build"
docker compose run --rm ui-e2e
docker compose run --rm ui-test sh -c "npm ci && npm run test:e2e:live"
```

Latest Docker Compose verification on June 7, 2026: lint passed, 61 unit tests passed, 7 component tests passed with the coverage threshold, all 6 mocked Playwright workflows passed, and the production build completed successfully. The live suite reported 2 explicit skips because live credentials and the dedicated mutation fixture were not available.
