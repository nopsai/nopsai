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

## Architecture Notes

- `src/App.tsx` is only the router/bootstrap wrapper.
- `src/app/AppShell.tsx` composes layout, navigation, and routes.
- `src/auth/*` owns current-user loading, capability normalization, derived permissions, and route guards.
- `src/lib/api.ts` exposes `apiClient`; UI code should call `apiClient.fetch(...)` instead of raw `fetch(...)` for NopsAI API requests.
- `src/app/useResourceTrees.ts` and `src/app/resourceTrees.ts` own sidebar resource loading and tree normalization.
- `src/app/useSidebarState.ts` owns sidebar resize and open/close state.
- `src/app/useInitialSetupRedirect.ts` owns first-install setup routing.
- `src/app/runSidebarApi.ts` and `src/app/usePipelineRunsSidebar.ts` own run-sidebar transport, caching, pagination, expansion, active-run synchronization, and polling.
- `src/features/system/api.ts` centralizes System-area JSON API behavior on top of `apiClient`.
- `src/features/system/access` owns Access entity loading, resource catalogs, mutations, and form state.
- `src/features/system/dispatcher` owns Dispatcher status, polling, runner actions, deployment scopes, install-template generation, and guide state.
- `src/features/schedules`, `knowledge-context`, `triggers`, and `scopes` own their workflow models, metadata/usage loading, and API clients.
- `src/features/pipeline-runs` owns run transport/contracts, notification route normalization, and graph status/layout logic.
- `src/app/BaseSidebarNavigation.tsx` owns stable top-level navigation.
- `src/features/editor/EditorAutocompleteMenu.tsx` owns reusable editor suggestion rendering.

## Testing

- `npm run test:unit`: TypeScript-compiled Node tests for pure models and API behavior.
- `npm run test:component`: Vitest, Testing Library, and V8 coverage for forms, navigation, autocomplete, drift review, and modal mutations. Repository-wide coverage has an initial enforced 1% floor.
- `npm run test`: unit and component suites.
- `npm run test:e2e`: Mocked Playwright coverage for authentication, password policy, pipeline save, setup, permission-controlled navigation, and a serious/critical Axe audit.
- `npm run test:e2e:live`: Deployed-stack Playwright smoke coverage without API mocks. Set `NOPS_UI_LIVE_BASE_URL`, `NOPS_UI_LIVE_USERNAME`, and `NOPS_UI_LIVE_PASSWORD`. Pipeline mutation additionally requires `NOPS_UI_LIVE_MUTATION=true` and a dedicated `NOPS_UI_LIVE_PIPELINE_ID`.

From the repository root, use `docker compose run --rm ui-test sh -c "npm ci && npm run lint && npm run test && npm run build"`, `docker compose run --rm ui-e2e`, and `docker compose run --rm ui-test sh -c "npm ci && npm run test:e2e:live"` for the containerized enterprise gates.
