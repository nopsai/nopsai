<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="public/brand/nopsai-logo-dark.png">
    <img src="public/brand/nopsai-logo-light.png" alt="NopsAI" width="360">
  </picture>
</p>

# NopsAI UI

Vite/React frontend for the NopsAI control plane.

```sh
npm install
npm run dev
npm run lint
npm run test
npm run build
npm run test:e2e
npm run test:e2e:live
```

## Browser Console Troubleshooting

Warnings that name `contentscript.js`, `ObjectMultiplex`,
`app-init-liveness`, or `background-liveness` usually come from injected browser
extension content scripts rather than the NopsAI UI bundle. Verify in a clean
browser profile or Playwright before changing UI listener code. See
[`doc/browser-console-troubleshooting.md`](../../doc/browser-console-troubleshooting.md).

## Architecture Notes

- `src/App.tsx` is only the router/bootstrap wrapper.
- `src/app/AppShell.tsx` composes the authenticated layout and navigation.
- `src/app/AppRoutes.tsx` owns lazy route composition and capability guards.
- `src/auth/*` owns current-user loading, capability normalization, derived permissions, and route guards.
- `src/lib/api.ts` exposes `apiClient`; UI code should call `apiClient.fetch(...)` instead of raw `fetch(...)` for NopsAI API requests.
- `src/app/useResourceTrees.ts` and `src/app/resourceTrees.ts` own sidebar resource loading and tree normalization.
- `src/app/useSidebarState.ts` owns sidebar resize and open/close state.
- `src/app/useInitialSetupRedirect.ts` owns first-install setup routing.
- `src/app/runSidebarApi.ts` and `src/app/usePipelineRunsSidebar.ts` own run-sidebar transport, caching, pagination, expansion, active-run synchronization, polling, and tested failure fallbacks.
- `src/features/system/api.ts` centralizes System-area JSON API behavior on top of `apiClient`.
- `src/features/system/config` owns the Settings Config workspace presentation,
  runtime config model/API/hook orchestration, notification mail settings, and
  global config repository sync/drift/write-back controls.
- `src/features/system/access` owns Access entity loading, user/service-account/role/policy catalogs, token workflows, destructive confirmations, resource catalogs, mutations, form state, the shared basic-grant editor/reconciliation model, focused policy-rule rendering/normalization, and role presentation. Basic grants keep the API value `root` but label that target as Global in the UI.
- `src/features/system/dispatcher` owns Dispatcher status, polling, runner actions, the compact tabbed dispatcher overview, runtime-filtered table-first runner fleet with detail below the fleet after selection, route edit/effective-routing tables, deployment scopes, install-template generation, private registry credential selection for runner installs, and guide state.
- `src/features/schedules`, `knowledge-context`, `triggers`, and `scopes` own their workflow models, metadata/usage loading, and API clients.
- `src/features/scopes/ScopeUsagePanel.tsx`, `scopes/ScopeWorkflowModals.tsx`, `triggers/TriggerRecentRuns.tsx`, `triggers/TriggerWorkflowModals.tsx`, `lab/LabRunControls.tsx`, `lab/LabVariableOverrides.tsx`, `lab/LabDependencyPanel.tsx`, and `editor/ResourceWorkflowModals.tsx` own focused workflow rendering delegated by the remaining editor pages.
- Feature hooks under `pipelines/`, `steps/`, `triggers/`, and `scopes/` own request-keyed permission orchestration; `lab/useLabRunAuthorization.ts` owns dependency discovery and debounced run authorization.
- `src/features/triggers/useTriggerManifestMutations.ts` owns trigger manifest create/clone/save/delete modal lifecycles with action-time authorization and Git-managed read-only handling.
- `src/features/scopes/useScopeModalMutations.ts` owns scope creation/seeding, repository-scoped variable/secret modal lifecycles, GitOps secret encryption/copy, and scoped value deletion.
- `src/features/editor/useDraftCollection.ts` and `useYamlResourceMutations.ts` own pipeline/step draft autosave and save/create/clone/delete lifecycles without changing routes or API contracts.
- `src/features/lab/useLabSession.ts` and `useLabRunMutation.ts` own Lab session persistence, protected pipeline switching, overrides, and authorized scoped-run submission.
- `src/features/monitoring/model.ts` and `MonitoringDashboard.tsx` own Monitoring normalization, aggregation, formatting, and dashboard presentation.
- `src/features/pipeline-runs` owns run transport/contracts, dashboard and run-card presentation, selected-run detail, notification and team dialogs, notification route normalization, graph rendering/layout/dialogs, and the incremental log dialog/polling/filter state.
- `src/features/pipelines/PipelineActivityPanels.tsx` and `src/features/steps/StepUsagePanel.tsx` own focused activity and usage presentation for their route shells.
- `src/features/editor/YamlValidationPanel.tsx` owns shared accessible YAML validation presentation across pipeline, step, trigger, and Lab editors.
- `src/features/editor/ResourceCollectionToolbar.tsx` and `src/components/WorkflowToastRegion.tsx` own shared pipeline/step collection controls and accessible workflow notifications.
- `src/features/event-automation/EventAutomationSwitch.tsx`, `EventAutomationToolbar.tsx`, `AutomationResourceTree.tsx`, and `resourceTreeModel.ts` own the rendering-only route switch/header and reusable event automation team tree. `src/features/triggers/model.ts`, `triggers/treeModel.ts`, `TriggerExplorerTree.tsx`, `TriggerDetailView.tsx`, `external-triggers/model.ts`, and `git-webhook-sources/model.ts` own collection metrics, resource-specific tree item shaping, persistent selected-resource navigation, and source/search filtering for the redesigned event automation workspaces.
- `src/components/ObjectIcon.tsx` and `src/components/objectIconRegistry.ts` own the shared lucide-backed object icon renderer and registry for navigation, resource cards, knowledge kinds, GitOps badges, and future first-class resources.
- `src/components/AppHelp.tsx` and `appHelpModel.ts` own accessible route-specific help rendering plus pure topic and documentation-link resolution.
- `src/app/navigationModel.ts` owns pure sidebar topic grouping rules, and
  `src/app/BaseSidebarNavigation.tsx` owns stable top-level navigation
  rendering.
- `src/features/editor/EditorAutocompleteMenu.tsx` owns reusable editor suggestion rendering.

## Testing

- `npm run test:unit`: TypeScript-compiled Node tests for pure models and API behavior.
- `npm run test:component`: Vitest, Testing Library, and V8 coverage for forms, policy rules, basic-grant editing, navigation, sidebar loading/failures, permission hooks and redirects, autocomplete, collection controls, notifications, keyboard run graphs, drift review, log polling/filtering, workflow-dialog focus management, scope impact analysis, Scope/Trigger/Access dialogs, trigger runs, Lab session/run/override controllers, pipeline/step draft persistence and lifecycle mutations, Monitoring presentation/model behavior, Knowledge Context model behavior, App Help, failure announcements, and modal mutations. Repository-wide coverage floors are 20% statements, 17% branches, 21% functions, and 21% lines.
- `npm run test`: unit and component suites.
- `npm run test:e2e`: Mocked Playwright coverage for authentication, password policy, pipeline save, setup, permission-controlled navigation, keyboard sidebar/graph/editor/dialog behavior, and serious/critical Axe audits for login, the authenticated workspace, workflow dialogs, editor autocomplete, run graphs, and populated logs.
- `npm run test:e2e:live`: Deployed-stack Playwright smoke coverage without API mocks. Set `NOPS_UI_LIVE_BASE_URL`, `NOPS_UI_LIVE_USERNAME`, and `NOPS_UI_LIVE_PASSWORD`. Pipeline mutation additionally requires `NOPS_UI_LIVE_MUTATION=true` and a dedicated `NOPS_UI_LIVE_PIPELINE_ID`.
- `npm run test:e2e:live:auth` and `npm run test:e2e:live:mutation`: focused commands used by the deployment workflow. CI fails closed when required credentials or the requested mutation fixture are missing.

Run UI gates directly from `services/ui`; Docker Compose is reserved for the install/runtime stack and build-only service images.

The deployed-stack smoke gate lives in `e2e-live` and is GitOps-compatible through the deployment pipeline that invokes it. Configure the selected protected environment with `NOPS_UI_LIVE_BASE_URL`, `NOPS_UI_LIVE_USERNAME`, `NOPS_UI_LIVE_PASSWORD`, and `NOPS_UI_LIVE_PIPELINE_ID` when mutation smoke is enabled. Run the focused auth or mutation commands after rollout; environment protection and deployment-pipeline concurrency should prevent unreviewed or overlapping mutation runs.
