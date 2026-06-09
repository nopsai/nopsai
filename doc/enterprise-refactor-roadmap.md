# Enterprise UI Refactor Roadmap

This document tracks the UI clean-code work needed to keep NopsAI maintainable as an enterprise control plane.

## Executive Summary

The current UI refactor is in a healthy state. Phase 1 route-shell slimming is complete: the largest workflow routes now delegate substantial presentation, validation, catalog, graph, token, and workflow-dialog responsibilities to feature-owned modules. Auth and authorization remain centralized, API access goes through `apiClient`, Git-managed resources preserve read-only/clone workflows, and the configured UI quality gates pass locally.

The refactor should not be called enterprise-complete yet. The main remaining blocker is to record the first protected-environment live execution against a deployed stack. The next phase should focus on reducing large route modules, locking architectural boundaries, improving coverage in high-risk flows, and making clean-code rules enforceable instead of tribal knowledge.

## Current Status

- **Completed local refactor pass:** App bootstrap, auth, system modules, editor workflows, Phase 1 route-shell slimming, pipeline/step/scope/trigger/lab/run-log boundaries, accessibility primitives, component tests, mocked Playwright coverage, and local Docker Compose UI verification are in place.
- **Still open for enterprise completion:** First protected-environment live execution has not been recorded because live credentials and mutation fixtures were not available in the workspace.
- **Main maintainability risk:** The route shells are materially smaller but remain state-heavy. Future changes should continue extracting orchestration only when a tested feature boundary is clear, with `AccessPanel.tsx` and the Pipeline Runs feature modules as the largest remaining surfaces.
- **Quality baseline:** Local verification on June 9, 2026 passed lint, 88 unit tests, 115 component tests, all 9 mocked Playwright workflows, and the production build. Component coverage measured 22.19% statements, 19.31% branches, 23.89% functions, and 23.01% lines against enforced floors of 20%, 17%, 21%, and 21%.

## What Is Already Cleanly Separated

The refactor has established these boundaries and they should be preserved:

- `src/App.tsx` remains a small router/bootstrap wrapper.
- `src/app/*` owns shell composition, lazy routes, sidebar state, resource trees, initial setup redirect, and run-sidebar orchestration.
- `src/auth/*` owns session loading, current-user state, capability normalization, access decisions, auth context, and route guards.
- `src/lib/api.ts` owns API URL resolution, session persistence, bearer-token attachment, and refresh retry behavior through `apiClient`.
- `src/features/**/model.ts` owns pure domain rules, normalization, validation, identifiers, payload shaping, and display helpers.
- `src/features/**/api.ts` owns transport and endpoint-specific request/response mapping.
- `src/features/**/use*.ts` owns async orchestration, polling, local mutation state, permission checks, and toast-facing failure handling.
- Feature-owned `*.tsx` components own rendering, accessibility semantics, keyboard behavior, and local visual interaction only.
- Shared workflow UI lives in reusable editor, dialog, toast, autocomplete, and accessibility helpers rather than being duplicated across pages.

## Clean-Code Principles For The Next Phase

Every new or touched UI change should follow these rules:

1. **Feature ownership first.** Domain logic must live under `src/features/<area>/` or `src/app/` when it is app-shell behavior. Avoid adding new business logic to route pages.
2. **Pure models before hooks.** Put parsing, normalization, permission key derivation, payload mapping, and validation in pure functions with unit tests before wiring them into React hooks.
3. **Transport isolation.** All HTTP work must go through `apiClient` or feature API helpers. Do not call `fetch` directly from components.
4. **Fail-closed authorization.** Permission and readiness state must be keyed to the active resource/folder/selection and must never reuse stale grants across navigation.
5. **Accessible by default.** Dialogs, editors, graphs, forms, and notifications must expose semantic roles, labels, validation relationships, Escape/Tab behavior, and focus restoration.
6. **Small rendering units.** Route modules should compose feature components and hooks. They should not own API calls, normalization, modal lifecycles, or large presentation branches.
7. **Tests follow the boundary.** Pure model/API behavior gets unit tests; hooks and components get component tests; end-to-end tests cover critical user journeys, accessibility, and deployed smoke paths.
8. **No hidden regressions.** GitOps read-only behavior, clone workflows, local drafts, route synchronization, action-time AAA checks, and backward-compatible notification/log routes must be preserved during extraction.

## Updated Enterprise Refactor Plan

### Phase 0 — Enterprise Live Gate

Goal: prove the refactored UI works against a protected deployed stack.

- Configure the protected GitHub environment with `NOPS_UI_LIVE_BASE_URL`, `NOPS_UI_LIVE_USERNAME`, and `NOPS_UI_LIVE_PASSWORD`.
- Add a dedicated non-production mutation fixture through `NOPS_UI_LIVE_PIPELINE_ID` before enabling the mutation job.
- Run `.github/workflows/ui-live-smoke.yml` with authentication first, then opt-in mutation once the fixture is confirmed disposable.
- Record the workflow run link, target environment, executed jobs, skipped jobs, and any artifact links in this document.
- Do not mark the enterprise UI refactor complete until this run is recorded.

Acceptance criteria:

- Protected live auth smoke passes against the deployed stack.
- Mutation smoke either passes against a dedicated test pipeline or is explicitly deferred with the missing fixture documented.
- Fail-closed missing-secret behavior remains intact.

### Phase 1 — Route Shell Slimming

Status: **Complete on June 9, 2026.**

Goal: reduce the largest route modules into readable composition shells without changing behavior.

Priority order:

1. `PipelineRuns.tsx`: extract run list filters, selected-run summary, notification-route form shell, graph/log orchestration panels, and empty/loading/error presentations.
2. `AccessPanel.tsx`: extract user/service-account sections, role-policy panels, token workflows, destructive confirmations, and resource-catalog presentation.
3. `Scopes.tsx` and `Triggers.tsx`: continue moving workflow-specific presentation and selection state into feature components while preserving GitOps read-only behavior and action-time AAA checks.
4. `Pipelines.tsx` and `Steps.tsx`: keep editors as route shells around shared collection, draft, mutation, autocomplete, validation, and permission hooks.
5. `Lab.tsx`: isolate pipeline selection, scope selection, override preview, run feedback, and denied-resource presentation.

Acceptance criteria:

- [x] Route modules primarily compose hooks and feature components.
- [x] New extracted files have focused names and one reason to change.
- [x] No route, draft, permission, GitOps, or URL synchronization behavior changes were introduced without tests.

Completion record:

- `PipelineRuns.tsx` was reduced from 4,786 to 1,553 lines by extracting dashboard/run-card presentation, selected-run detail, graph dialogs, folder/configuration dialogs, and notification-route UI under `src/features/pipeline-runs/`.
- `AccessPanel.tsx` was reduced from 2,397 to 2,010 lines by extracting user, service-account, role, and policy catalogs plus service-account token and confirmation workflows under `src/features/system/access/`.
- `Scopes.tsx` and `Triggers.tsx` now delegate selection, grouping, route identifiers, source presentation, manifest validation, and usage metadata to feature models and components. Existing Git-managed read-only behavior and action-time authorization hooks remain unchanged.
- `Pipelines.tsx` and `Steps.tsx` now delegate activity, usage, validation, and presentation concerns while retaining the existing draft, mutation, autocomplete, permission, and URL synchronization hooks.
- `Lab.tsx` now delegates dependency preview/model behavior and shares the extracted YAML validation presentation without changing authorization or run-submission contracts.
- Focused unit and component tests cover the new pure models and critical extracted interactions. The existing mocked Playwright suite continues to cover AAA restrictions, GitOps-compatible pipeline mutation contracts, dialogs, graph/log interaction, and accessibility.

### Phase 2 — Boundary Enforcement

Goal: make the intended architecture enforceable.

- Add a lightweight architectural checklist to PR reviews for UI changes.
- Add lint or script checks for direct component-level `fetch`, broad `window` API usage, and new `// @ts-ignore` directives.
- Track large-file counts for `services/ui/src/pages` and feature shells so route growth is visible in CI or review notes.
- Prefer explicit exported types from `model.ts` and `api.ts`; avoid anonymous payload shapes crossing module boundaries.
- Keep `react-hooks`, TypeScript, and Fast Refresh rules clean with no local exceptions.

Acceptance criteria:

- PRs that add route-local transport or duplicated model logic are blocked or sent back for extraction.
- A simple size/boundary report is available before each release branch.
- No new TypeScript ignore comments or hook-rule suppressions are introduced.

### Phase 3 — Coverage And Regression Hardening

Goal: raise confidence where enterprise regressions would be expensive.

- Increase component coverage floors only after measured headroom exists; avoid arbitrary threshold jumps.
- Add focused tests for each extraction before moving behavior out of a large route file.
- Expand component coverage around permission denial, stale selection cleanup, failed API responses, Git-managed read-only states, and route synchronization.
- Keep mocked Playwright focused on critical workflows: login, password change, setup, access navigation, editor save, workflow dialogs, graph/log interaction, and accessibility gates.
- Keep live Playwright small and reliable: deployed auth smoke by default, mutation smoke only for dedicated disposable resources.

Acceptance criteria:

- Each high-risk extraction has pure/unit or component tests before merge.
- Coverage floors are raised only when the latest measured result leaves safe buffer.
- Mocked and live suites remain deterministic and fail with useful diagnostics.

### Phase 4 — UX Consistency And Accessibility Finish

Goal: remove remaining UI inconsistency while keeping domain-specific visuals intentional.

- Standardize modal, drawer, form, empty-state, alert, toolbar, and toast patterns through shared primitives.
- Keep common command actions on `lucide-react`; allow custom SVG only for domain-specific status, graph, resource, or chart visuals.
- Ensure all new controls have label-driven automation, keyboard paths, visible focus, and screen-reader-friendly feedback.
- Add accessibility checks when introducing new dialogs, complex editors, graph controls, or live regions.

Acceptance criteria:

- New workflow surfaces use shared primitives unless a documented product reason exists.
- Axe serious/critical gates stay green for login, authenticated workspace, dialogs, editor autocomplete, graph interaction, and populated logs.
- Manual keyboard review is performed for changed complex interactions before release.

### Phase 5 — Documentation And Ownership

Goal: make the refactor maintainable for future contributors.

- Keep this roadmap as the source of truth for enterprise UI refactor status.
- Add or update feature-level README notes when a module has non-obvious contracts such as GitOps read-only behavior, permission keys, or legacy route compatibility.
- Document ownership for app shell, auth, editor, system, workflow pages, pipeline runs, lab, monitoring, and accessibility primitives.
- Record verification commands, latest measured coverage, live-smoke results, and known skips after every major refactor pass.

Acceptance criteria:

- A new contributor can identify where to put model, API, hook, rendering, and test changes without reading large route files first.
- Roadmap status matches the actual repo state and is updated in the same PR as major boundary changes.

## Enterprise Completion Checklist

The enterprise UI refactor can be marked complete only when all of the following are true:

- [ ] Protected-environment live auth smoke has passed and is recorded here.
- [ ] Optional mutation smoke has passed against a dedicated disposable fixture, or an explicit enterprise decision documents why auth-only live smoke is sufficient.
- [ ] Large route modules have active extraction follow-ups or are below the agreed maintainability threshold.
- [x] No direct component-level transport, hook-rule suppressions, or TypeScript ignore directives were introduced in the final pass.
- [x] Local gates pass from `services/ui`: `npm run lint`, `npm run test`, `npm run build`, and `npm run test:e2e`.
- [x] Docker Compose UI gates pass from the repository root when host Node/npm are unavailable.
- [x] Accessibility serious/critical gates remain green for the covered browser workflows.
- [x] Coverage floors match the latest stable measured baseline with safe headroom.

## Verification Commands

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

Latest local verification on June 9, 2026 used the documented Docker Compose UI gates because the host shell did not expose `node` or `npm`: lint passed, 88 unit tests passed, 115 component tests passed with 22.19% statement coverage, 19.31% branch coverage, 23.89% function coverage, and 23.01% line coverage against enforced floors of 20%, 17%, 21%, and 21%; all 9 mocked Playwright workflows passed; and the production build completed successfully. The rebuilt `nopsai-ui` service was healthy at `http://localhost`, and a desktop Playwright render confirmed the served login surface had no clipping or overlap. The route extraction preserved the existing `apiClient`, request-keyed permission hooks, action-time AAA checks, Git-managed read-only/clone paths, and GitOps-compatible deployment workflow. The protected live suite remains credential-gated. `actionlint` was not available on the host shell during this pass, so workflow lint was not re-run.

## Change Log

- **June 9, 2026:** Completed Phase 1 route-shell slimming across Pipeline Runs, Access, Scopes, Triggers, Pipelines, Steps, and Lab; added focused model/component coverage; and recorded the updated local quality baseline.
- **June 9, 2026:** Reframed the roadmap into an enterprise clean-code execution plan, preserved the completed refactor summary, made the protected live execution the explicit completion blocker, and added phased guardrails for route slimming, boundary enforcement, coverage hardening, accessibility consistency, and documentation ownership.
