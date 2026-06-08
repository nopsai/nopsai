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
- `src/features/editor/useDraftCollection.ts` owns scoped local draft subscriptions, mutations, and debounced editor autosave for pipelines and reusable steps.
- `src/features/editor/useYamlResourceMutations.ts` owns the shared pipeline/step save, create, clone, and delete lifecycles, including action-time create authorization and draft-to-database transitions.
- `src/features/pipelines/model.ts` owns pipeline directive catalogs, source/identifier helpers, YAML validation, YAML-to-detail parsing, and pipeline dependency graph data normalization.
- `src/features/pipelines/api.ts` owns pipeline list/detail/save/delete requests, permission checks, recent-run loading, trigger discovery, and list payload normalization.
- `src/features/steps/model.ts` owns step directive catalogs, identifier/source helpers, timestamp formatting, and strict step YAML validation.
- `src/features/steps/api.ts` owns reusable-step list/detail/save/delete requests, permission checks, usage loading, and API payload normalization.
- `src/features/schedules/model.ts` and `api.ts` own schedule types, cron/form conversion, metadata normalization, validation, payload mapping, and schedule lifecycle API calls.
- `src/features/knowledge-context/model.ts` and `api.ts` own knowledge document identities, routes, draft storage, tree/content normalization, validation, and CRUD calls.
- `src/features/triggers/model.ts` and `api.ts` own trigger manifest normalization, pipeline/run helpers, editor metadata, permission checks, list/detail loading, and trigger lifecycle calls.
- `src/features/scopes/model.ts` and `api.ts` own scope routes, repository-scoped identities, tree/list normalization, usage-analysis transport, clone naming, permission checks, scoped value lifecycle calls, and secret encryption.
- `src/features/pipelines/usePipelinePermissions.ts`, `steps/useStepPermissions.ts`, `triggers/useTriggerPermissions.ts`, and `scopes/useScopePermissions.ts` own fail-closed, request-keyed permission orchestration for their workflow pages.
- `src/features/lab/useLabRunAuthorization.ts` owns Lab resource-use discovery, deduplication, debounced batch authorization, denied-resource state, and authorization failures.
- `src/features/lab/useLabSession.ts` and `useLabRunMutation.ts` own isolated Lab session persistence, protected pipeline switching, override editing, and authorized scoped-run submission.
- `src/features/pipeline-runs/api.ts` owns Pipeline Runs JSON, log, and folder config-repository transport behavior.
- `src/features/pipeline-runs/contracts.ts` owns shared run-list and graph layout contracts that were previously embedded in `pages/PipelineRuns.tsx`.
- `src/features/pipeline-runs/notificationRoutes.ts` owns legacy and current notification-route normalization, form state, editing, and API payload mapping.
- `src/features/pipeline-runs/graphLayout.ts` owns graph labels, run/task/edge statuses, deterministic layout, and cyclic dependency protection.
- `src/features/pipeline-runs/RunGraph.tsx` owns Pipeline Runs graph rendering and interaction, while `runGraphModel.ts` owns elapsed-time and step-duration formatting.
- `src/features/pipeline-runs/runLogs.ts` owns log contracts, structured/plain-text parsing, filtering, legacy hash compatibility, and download formatting.
- `src/features/pipeline-runs/useRunLogs.ts` owns incremental log polling, URL synchronization, filters, follow state, loading, and failure state.
- `src/features/pipeline-runs/RunLogsModal.tsx` owns log dialog rendering, filter controls, download interaction, and accessible dialog/log semantics.
- `src/features/system/access/AccessPolicyRuleFields.tsx` owns the visual AAA rule editor, while `policyRuleModel.ts` owns selector catalogs, parsing, normalization, and display summaries.
- `src/app/useInitialSetupRedirect.ts` owns first-install setup routing decisions.
- `src/app/AppRoutes.tsx` owns lazy authenticated route composition and permission guards.
- `src/app/runSidebarApi.ts` and `usePipelineRunsSidebar.ts` own recent/repository run loading, pagination, expansion, active-run synchronization, caching, and polling.
- `src/app/BaseSidebarNavigation.tsx` owns the stable application navigation list.
- `src/features/editor/EditorAutocompleteMenu.tsx` provides the shared autocomplete menu renderer used by workflow editors.
- `src/auth/AuthContext.ts` owns the auth context and `useAuth` hook so the provider component stays Fast Refresh friendly.
- The UI package has separate unit, component, mocked Playwright, and live-backend Playwright suites. The default `test` command runs unit and component coverage.
- Focused component tests cover access forms and policy rules, config repository drift, stable sidebar navigation, editor autocomplete, run graph interaction, run log polling/filtering/failures, and schedule save/delete modal flows.
- Mocked Playwright workflows cover login, required password-change redirect, pipeline save, first-install setup, access-controlled navigation, and serious/critical Axe accessibility audits for login, the authenticated workspace, workflow dialogs, editor autocomplete, graph interaction, and populated logs.
- `playwright.live.config.ts` provides credential-gated deployed-stack authentication and optional pipeline save/run smoke coverage without API mocks.
- `.github/workflows/ui-live-smoke.yml` provides a protected-environment, concurrency-controlled, reusable/manual deployment gate with separate authentication and explicit mutation jobs, fail-closed secret validation, and retained Playwright diagnostics.
- Vitest V8 coverage is enforced at 11% statements, 9% branches, 12% functions, and 12% lines. The June 8, 2026 baseline is 11.52% statements, 9.30% branches, 12.82% functions, and 12.14% lines.
- Login form labels are now programmatically associated with their inputs, preserving accessible label-driven automation and assistive-technology behavior.
- The login primary action now meets the automated color-contrast gate.
- The Pipeline Runs log modal now exposes dialog, search, log, status, and alert semantics, supports Escape-to-close, and has tested incremental polling and failure announcements.
- The sidebar resizer now exposes an adjustable separator contract and supports Arrow, Home, and End keyboard controls.
- Shared pipeline/step workflow dialogs, schedule editing, config-repository drift, Resource Access, and populated run logs now use a common focus trap with initial focus, Escape handling, and opener focus restoration.
- Pipeline, reusable-step, and trigger YAML editors now expose accessible names, validation relationships, shared listbox semantics, and Arrow/Enter/Escape autocomplete behavior.
- Pipeline run graph controls, step nodes, step details, and task-log actions are keyboard operable and expose status-aware accessible names.
- Mocked Playwright accessibility coverage now audits workflow dialogs, editor autocomplete, active navigation state, keyboard graph interaction, and fully populated log dialogs in addition to login and the base workspace.
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
- `features/system/access/AccessPolicyRuleFields.tsx`: Focused AAA policy-rule editor rendering.
- `features/system/access/policyRuleModel.ts`: AAA resource/action catalogs, selector parsing, normalization, and summaries.
- `features/system/access/BasicAccessGrantEditor.tsx`: Shared basic-role rendering and editing for user and service-account create/edit workflows.
- `features/system/access/basicGrantModel.ts`: Basic-grant staging, duplicate detection, dirty comparison, and API reconciliation planning.
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
- `features/editor/useDraftCollection.ts`: Scoped local draft subscriptions, mutations, and debounced pipeline/step editor autosave.
- `features/editor/useYamlResourceMutations.ts`: Shared pipeline/step persistence and create/clone/delete modal orchestration with action-time authorization.
- `features/pipelines/model.ts`: Pipeline editor directive catalogs, pure validation, identifier/source normalization, YAML detail parsing, and dependency graph data normalization.
- `features/pipelines/api.ts`: Pipeline list/detail/save/delete API calls, permission checks, recent-run loading, trigger discovery, and typed payload normalization.
- `features/pipelines/usePipelinePermissions.ts`: Pipeline folder-create, selected-update, and selected-execute permission orchestration.
- `features/steps/model.ts`: Step editor directive catalogs, pure validation, identifier/source normalization, and timestamp formatting.
- `features/steps/api.ts`: Step list/detail/save/delete API calls, permission checks, usage loading, and typed payload normalization.
- `features/steps/useStepPermissions.ts`: Reusable-step folder-create and selected-update permission orchestration.
- `features/schedules/model.ts`: Schedule types, cron parsing/building, form mapping, metadata normalization, and payload validation.
- `features/schedules/api.ts`: Schedule list, metadata, save, enable/disable, run, and delete API calls.
- `features/knowledge-context/model.ts`: Knowledge identities, route helpers, draft storage, tree normalization, content previews, and validation.
- `features/knowledge-context/api.ts`: Knowledge list, detail, save, and delete API calls.
- `features/triggers/model.ts`: Trigger manifest, list, source, pipeline, and run normalization helpers.
- `features/triggers/api.ts`: Trigger permissions, list/detail/runs, editor metadata, pipeline YAML, save, create, clone, and delete API calls.
- `features/triggers/useTriggerPermissions.ts`: Trigger folder-create and selected-update permission orchestration.
- `features/triggers/TriggerRecentRuns.tsx`: Trigger-linked run history rendering and run-navigation interaction.
- `features/scopes/model.ts`: Scope identity, route, tree, item metadata, and clone-name normalization.
- `features/scopes/api.ts`: Scope permissions, catalogs, usage-analysis resources, variables/secrets CRUD, and secret encryption API calls.
- `features/scopes/useScopePermissions.ts`: Scope creation and selected variable/secret write permission orchestration.
- `features/scopes/ScopeUsagePanel.tsx`: Selected variable/secret metadata and related pipeline/trigger impact rendering.
- `features/editor/ResourceWorkflowModals.tsx`: Shared controlled create/clone/delete dialogs for pipeline and reusable-step workflows.
- `features/lab/LabRunControls.tsx`: Lab pipeline/scope selection, run readiness, access checks, launch action, and feedback rendering.
- `features/lab/useLabRunAuthorization.ts`: Lab dependency resource-use checks, debounced batch authorization, and denied-resource state.
- `features/lab/useLabSession.ts`: Lab session restoration/persistence, protected pipeline switching, scope state, and variable overrides.
- `features/lab/useLabRunMutation.ts`: Authorized Lab run payload validation, scoped submission, and run feedback.
- `features/monitoring/model.ts`: Monitoring run, group, service, runner, duration, status, and trend normalization/aggregation.
- `features/monitoring/MonitoringDashboard.tsx`: Monitoring metric, runtime, trend, status, group, and pipeline presentation.
- `features/pipeline-runs/api.ts`: Shared Pipeline Runs JSON, incremental logs, and folder config-repository requests.
- `features/pipeline-runs/contracts.ts`: Run-list and graph layout contracts shared by Pipeline Runs rendering.
- `features/pipeline-runs/notificationRoutes.ts`: Notification route contracts, legacy normalization, form editing, and payload mapping.
- `features/pipeline-runs/graphLayout.ts`: Run graph status derivation and deterministic layout.
- `features/pipeline-runs/RunGraph.tsx`: Run and task graph rendering, pan/zoom, expansion, and previews.
- `features/pipeline-runs/runGraphModel.ts`: Run graph duration formatting.
- `features/pipeline-runs/runLogs.ts`: Run log contracts, parsing, filtering, URL hash compatibility, and download formatting.
- `features/pipeline-runs/useRunLogs.ts`: Incremental polling and log-view state orchestration.
- `features/pipeline-runs/RunLogsModal.tsx`: Accessible log dialog rendering and interaction.
- `app/AppRoutes.tsx`: Lazy route composition and capability guards for authenticated pages.
- `app/useInitialSetupRedirect.ts`: Initial setup redirect orchestration.
- `app/runSidebarApi.ts`: Typed run-sidebar transport.
- `app/usePipelineRunsSidebar.ts`: Run-sidebar loading, caching, expansion, pagination, synchronization, and polling.
- `app/BaseSidebarNavigation.tsx`: Stable capability-aware top-level navigation.
- `features/editor/EditorAutocompleteMenu.tsx`: Reusable simple and grouped autocomplete rendering.

## Completed Roadmap Targets

1. The remaining workflow pages now delegate repeated transport and payload shaping to feature-owned `model.ts` and `api.ts` modules.
2. Component coverage exists for access forms and policy rules, config repository drift, sidebar navigation, editor autocomplete, Pipeline Runs graph/log behavior, workflow dialogs, scope impact analysis, trigger run history, Lab run controls, Monitoring presentation, and modal save/delete flows.
3. Browser-level coverage exists for login, password-change redirect, pipeline save, system setup, access-controlled navigation, and automated login/authenticated-workspace accessibility checks.
4. Pipeline run graph logic and notification route ownership now live under `features/pipeline-runs/` instead of the page module.
5. Access resource catalogs and Dispatcher deployment-guide requests now have feature-owned APIs, models, and hooks.
6. Setup redirect and run-sidebar orchestration now live outside `AppShell.tsx`.
7. Coverage reporting, an initial enforced baseline, and a credential-gated live-backend smoke suite are available.
8. Pipeline Runs graph and log rendering, Access policy-rule/basic-grant editing, and authenticated route composition now live in focused components. The parent modules are approximately 5,143 lines for `PipelineRuns.tsx`, 2,565 for `AccessPanel.tsx`, and 1,373 for `AppShell.tsx`.
9. The remaining workflow pages now delegate cohesive product surfaces: scope impact analysis, trigger run history, shared pipeline/step lifecycle dialogs, Lab run readiness/feedback, and the complete Monitoring analytics/dashboard model. Parent modules are approximately 2,400 lines for `Scopes.tsx`, 2,082 for `Triggers.tsx`, 1,579 for `Pipelines.tsx`, 1,277 for `Steps.tsx`, 1,114 for `Lab.tsx`, and 299 for `Monitoring.tsx`.
10. Coverage now exercises Pipeline Runs graph controls and callbacks, Access basic-grant replacement/reconciliation, sidebar loading and pagination, transport failures, permission redirects, request-keyed workflow permissions, and Lab run authorization. The enforced floors increased to 11% statements, 9% branches, 12% functions, and 12% lines against a measured baseline of 11.52%, 9.30%, 12.82%, and 12.14%.
11. Accessibility coverage now includes serious/critical Axe gates and focus-management checks for shared workflow dialogs, YAML editors and autocomplete, navigation state, pipeline run graphs, and populated log views. Shared modal focus handling and keyboard graph/editor behavior are component tested.
12. The deployed live suite now has a GitOps-compatible GitHub Actions workflow with protected environment selection, environment-scoped credentials, separate opt-in mutation execution, per-environment serialization, fail-closed CI validation, and failure artifacts.
13. Permission and run-readiness orchestration now lives in feature hooks across `Scopes`, `Triggers`, `Pipelines`, `Steps`, and `Lab`. Permission results are keyed to the active folder or selection so stale grants cannot leak across navigation, action-time checks remain in place for mutations, and focused hook tests cover grants, rejection, deselection, resource identifiers, debounce, and denied dependencies.
14. Pipeline and reusable-step draft persistence and save/create/clone/delete mutation lifecycles now live in shared editor hooks, while Lab session persistence, protected pipeline switching, override editing, and run submission live in Lab hooks. Routes, API payloads, local draft behavior, action-time AAA checks, and GitOps read-only handling remain unchanged and are covered by focused component tests.

## Remaining Enterprise Follow-On Work

These are not blockers for the completed pass, but they remain before the broader enterprise UI refactor should be called complete:

1. **Perform the first protected-environment live execution.** The deployment workflow and CI credential contract are complete, but an environment-backed run still requires configuring `NOPS_UI_LIVE_BASE_URL`, `NOPS_UI_LIVE_USERNAME`, and `NOPS_UI_LIVE_PASSWORD` in GitHub. The optional mutation job additionally requires a dedicated `NOPS_UI_LIVE_PIPELINE_ID`. Those values were unavailable in this workspace, so no deployed run can be truthfully recorded yet.
2. **Extract the remaining domain-specific Scope and Trigger modal controllers only alongside those workflow changes.** Shared pipeline/step draft persistence and lifecycle mutations plus Lab session/run orchestration now live in feature hooks. `Scopes` and `Triggers` still retain specialized repository-scoped secret/variable encryption and trigger-manifest modal lifecycles; move those slices when the workflows next change, preserving routes, action-time AAA checks, GitOps read-only behavior, drafts, and API contracts.

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

Latest local verification on June 8, 2026: lint passed, 72 unit tests passed, 57 component tests passed with 13.35% statement coverage, 10.57% branch coverage, 14.31% function coverage, and 14.04% line coverage against the raised thresholds; all 9 mocked Playwright workflows passed, including the Axe/focus scenarios, and the production build completed successfully. The live suite remains credential-gated; its local run reported 2 explicit skips, the CI missing-credential guard failed closed as designed, and the workflow passed YAML and `actionlint` validation.
