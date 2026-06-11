# Enterprise UI Refactor Roadmap

This document tracks the UI clean-code work needed to keep NopsAI maintainable as an enterprise control plane.

## Executive Summary

The current UI refactor is in a healthy state. Phase 1 route-shell slimming is complete: the largest workflow routes now delegate substantial presentation, validation, catalog, graph, token, and workflow-dialog responsibilities to feature-owned modules. Auth and authorization remain centralized, API access goes through `apiClient`, Git-managed resources preserve read-only/clone workflows, Phase 4 UX/accessibility consistency is complete for the active workflow surfaces, Phase 5 documentation and ownership guidance is recorded, and the configured UI quality gates pass locally.

The refactor should not be called enterprise-complete yet. The only remaining blocker is to record the first protected-environment live execution against a deployed stack. Route-shell slimming, boundary enforcement, high-risk regression hardening, workflow UX/accessibility consistency, documentation ownership, and the report-only route-shell debt closure are complete locally; the next work should focus on the protected live gate.

## Current Status

- **Completed local refactor pass:** App bootstrap, auth, system modules, editor workflows, Phase 1 route-shell slimming, Phase 2 boundary enforcement, Phase 3 regression hardening, Phase 4 UX/accessibility consistency, Phase 5 documentation ownership, pipeline/step/scope/trigger/lab/run-log boundaries, report-only large-route cleanup, accessibility primitives, component tests, mocked Playwright coverage, and local Dockerized UI verification are in place.
- **Still open for enterprise completion:** First protected-environment live execution has not been recorded because live credentials and mutation fixtures were not available in the workspace.
- **Closed route-shell extraction queue:** The June 11 maintainability-debt passes retired `AccessPanel.tsx`, Pipeline Runs feature-shell, `SetupWizard.tsx`, and the remaining report-only large route entries from the boundary report. The current report shows 0 large route files and 0 large feature-shell files, so no non-blocking route-shell extraction items remain open; future growth remains release-visible through `npm run check:ui-boundaries`.
- **Quality baseline:** Local verification on June 11, 2026 passed lint, UI boundary checks, 95 unit tests, 129 component tests, all 9 mocked Playwright workflows, live Playwright credential-gated skips, and the production build. Component coverage measured 23.07% statements, 20.58% branches, 24.92% functions, and 23.97% lines against enforced floors of 21%, 18%, 22%, and 22%.

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

## Clean-Code Principles For Future UI Changes

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

Status: **Complete on June 11, 2026.**

Goal: make the intended architecture enforceable.

- Add a lightweight architectural checklist to PR reviews for UI changes.
- Add lint or script checks for direct component-level `fetch`, broad `window` API usage, and new `// @ts-ignore` directives.
- Track large-file counts for `services/ui/src/pages` and feature shells so route growth is visible in CI or review notes.
- Prefer explicit exported types from `model.ts` and `api.ts`; avoid anonymous payload shapes crossing module boundaries.
- Keep `react-hooks`, TypeScript, and Fast Refresh rules clean with no local exceptions.

Acceptance criteria:

- [x] PRs that add route-local transport or duplicated model logic are blocked or sent back for extraction.
- [x] A simple size/boundary report is available before each release branch.
- [x] No new TypeScript ignore comments or hook-rule suppressions are introduced.

Completion record:

- Added `.github/pull_request_template.md` with an enterprise UI checklist covering feature boundaries, route-local transport, direct `fetch`, broad browser API usage, AAA action-time checks, GitOps read-only/clone behavior, tests, and docs.
- Added `npm run check:ui-boundaries` in `services/ui`, backed by a typed checker in `src/tools/uiBoundaryCheck.ts` and Node unit coverage in `src/tools/uiBoundaryCheck.test.ts`.
- The boundary check fails on raw `fetch`, TypeScript suppression comments, React Hooks/Fast Refresh suppressions, and route-local `apiClient.fetch` growth beyond the current baseline. It reports large route/feature-shell files and browser `window.*` usage for release review visibility.
- `.github/workflows/enterprise-gates.yml` now runs UI lint, boundary checks, unit/component tests, and production build as a dedicated CI job, making the guardrail GitOps-compatible and repeatable.

### Phase 3 — Coverage And Regression Hardening

Status: **Complete on June 11, 2026.**

Goal: raise confidence where enterprise regressions would be expensive.

- Increase component coverage floors only after measured headroom exists; avoid arbitrary threshold jumps.
- Add focused tests for each extraction before moving behavior out of a large route file.
- Expand component coverage around permission denial, stale selection cleanup, failed API responses, Git-managed read-only states, and route synchronization.
- Keep mocked Playwright focused on critical workflows: login, password change, setup, access navigation, editor save, workflow dialogs, graph/log interaction, and accessibility gates.
- Keep live Playwright small and reliable: deployed auth smoke by default, mutation smoke only for dedicated disposable resources.

Acceptance criteria:

- [x] Each high-risk extraction has pure/unit or component tests before merge.
- [x] Coverage floors are raised only when the latest measured result leaves safe buffer.
- [x] Mocked and live suites remain deterministic and fail with useful diagnostics.

Completion record:

- Added focused regression coverage for pipeline permission races so stale selected-resource AAA results cannot grant update/execute after navigation changes.
- Added run-log hook coverage for legacy route-hash hydration, route synchronization, selected-run cleanup, and incremental API polling state. This caught and fixed a short-view initialization bug that was clobbering full-log `wrap` settings from existing deep links.
- Added pipeline-runs API unit coverage for empty/text response normalization, encoded run-log paths, and failed response bodies.
- Added editor toolbar coverage for read-only create affordances and disabled back navigation without a folder context.
- Raised component coverage floors from 20/17/21/21 to 21/18/22/22 after the measured result reached 22.31% statements, 19.54% branches, 23.96% functions, and 23.12% lines.

### Phase 4 — UX Consistency And Accessibility Finish

Status: **Complete on June 11, 2026.**

Goal: remove remaining UI inconsistency while keeping domain-specific visuals intentional.

- Standardize modal, drawer, form, empty-state, alert, toolbar, and toast patterns through shared primitives.
- Keep common command actions on `lucide-react`; allow custom SVG only for domain-specific status, graph, resource, or chart visuals.
- Ensure all new controls have label-driven automation, keyboard paths, visible focus, and screen-reader-friendly feedback.
- Add accessibility checks when introducing new dialogs, complex editors, graph controls, or live regions.

Acceptance criteria:

- [x] New workflow surfaces use shared primitives unless a documented product reason exists.
- [x] Axe serious/critical gates stay green for login, authenticated workspace, dialogs, editor autocomplete, graph interaction, and populated logs.
- [x] Keyboard review is covered for changed complex interactions before release.

Completion record:

- Added `src/components/WorkflowPrimitives.tsx` with shared `WorkflowDialogFrame`, `WorkflowInlineAlert`, `WorkflowEmptyState`, and `WorkflowIconButton` primitives, plus focused component coverage for labels, modal semantics, focus trap behavior, Escape close, focus restoration, alerts, empty states, and icon-only accessible names.
- Migrated active workflow dialog surfaces to the shared frame and alert primitives: pipeline/step resource dialogs, trigger create/clone/delete dialogs, scope create/value/GitOps encryption/delete dialogs, System Access modals and empty states, and Knowledge Context create/delete dialogs.
- Reused `WorkflowToastRegion` for the remaining page-level custom toast stacks in System and Knowledge Context so notifications consistently expose live-region/status/alert semantics.
- Kept command visuals on `lucide-react` and preserved domain-specific graph/status/chart visuals as intentional exceptions.
- Mocked Playwright axe gates stayed green for login, authenticated workspace, workflow dialogs, editor autocomplete, graph interaction, and populated logs. Component and Playwright keyboard paths cover Tab wrapping, Escape close, focus return, editor autocomplete dismissal, graph expansion/log opening, and log-dialog focus order.

### Phase 5 — Documentation And Ownership

Status: **Complete on June 11, 2026.**

Goal: make the refactor maintainable for future contributors.

- Keep this roadmap as the source of truth for enterprise UI refactor status.
- Add or update feature-level README notes when a module has non-obvious contracts such as GitOps read-only behavior, permission keys, or legacy route compatibility.
- Document ownership for app shell, auth, editor, system, workflow pages, pipeline runs, lab, monitoring, and accessibility primitives.
- Record verification commands, latest measured coverage, live-smoke results, and known skips after every major refactor pass.

Acceptance criteria:

- [x] A new contributor can identify where to put model, API, hook, rendering, and test changes without reading large route files first.
- [x] Roadmap status matches the actual repo state and is updated in the same PR as major boundary changes.

Completion record:

- Added `services/ui/src/README.md` as the source-adjacent UI ownership map for app shell, auth, shared components, feature models, feature APIs, hooks, rendering modules, route pages, tools, accessibility primitives, and test placement.
- Documented non-obvious feature contracts for editor/pipeline/step workflows, scopes, triggers, Pipeline Runs, Lab, System Access, Knowledge Context, Monitoring, and Schedules, including GitOps read-only/clone behavior, action-time AAA checks, legacy run-log route compatibility, autocomplete scope/context behavior, and GitOps secret encryption.
- Updated `doc/README.md` so the UI ownership guide is discoverable from the documentation map.
- Reconciled this roadmap with the actual repo state after Phases 2-4 and kept the remaining enterprise blocker explicit: protected live execution has not yet been recorded.

## Enterprise Completion Checklist

The enterprise UI refactor can be marked complete only when all of the following are true:

- [ ] Protected-environment live auth smoke has passed and is recorded here.
- [ ] Optional mutation smoke has passed against a dedicated disposable fixture, or an explicit enterprise decision documents why auth-only live smoke is sufficient.
- [x] Large route and feature-shell reports are clean through `npm run check:ui-boundaries` with 0 large route files and 0 large feature-shell files; future growth stays release-visible through the UI ownership guide and boundary report.
- [x] No direct component-level transport, hook-rule suppressions, or TypeScript ignore directives were introduced in the final pass.
- [x] Local gates pass from `services/ui`: `npm run lint`, `npm run check:ui-boundaries`, `npm run test`, `npm run build`, and `npm run test:e2e`.
- [x] Docker Compose UI gates pass from the repository root when host Node/npm are unavailable.
- [x] Accessibility serious/critical gates remain green for the covered browser workflows.
- [x] Coverage floors match the latest stable measured baseline with safe headroom.

## Remaining Items

- **Enterprise-completion blocker:** Record the protected live auth smoke, then run or explicitly defer the optional mutation smoke with a documented disposable fixture decision.

## Ongoing Maintenance Guardrails

- **Route-shell size guardrail:** The continuing non-blocking route-shell extraction queue is closed as of June 11, 2026. Keep route state and feature shells small when product work naturally touches them, and let `npm run check:ui-boundaries` make any renewed growth visible before release.

## Verification Commands

From `services/ui`:

```bash
npm run lint
npm run check:ui-boundaries
npm run test
npm run build
npm run test:e2e
npm run test:e2e:live
```

`test:unit` compiles pure model/API tests into `dist-test` and runs Node's test runner. `test:component` uses Vitest, Testing Library, jsdom, and V8 coverage. `test:e2e` uses Playwright with deterministic API mocks and a managed Vite server. `test:e2e:live` targets a deployed stack and skips explicitly when the required credentials or mutation fixture are unavailable.

From the repository root, the UI checks can also be run in the Docker Compose test service:

```bash
docker compose run --rm ui-test sh -c "npm ci && npm run lint && npm run check:ui-boundaries && npm run test && npm run build"
docker compose run --rm ui-e2e
docker compose run --rm ui-e2e sh -c "npm ci && npm run test:e2e:live"
```

Latest local verification on June 9, 2026 used the documented Docker Compose UI gates because the host shell did not expose `node` or `npm`: lint passed, 88 unit tests passed, 115 component tests passed with 22.19% statement coverage, 19.31% branch coverage, 23.89% function coverage, and 23.01% line coverage against enforced floors of 20%, 17%, 21%, and 21%; all 9 mocked Playwright workflows passed; and the production build completed successfully. The rebuilt `nopsai-ui` service was healthy at `http://localhost`, and a desktop Playwright render confirmed the served login surface had no clipping or overlap. The route extraction preserved the existing `apiClient`, request-keyed permission hooks, action-time AAA checks, Git-managed read-only/clone paths, and GitOps-compatible deployment workflow. The protected live suite remains credential-gated. `actionlint` was not available on the host shell during this pass, so workflow lint was not re-run.

Latest Phase 2 boundary-enforcement verification on June 11, 2026 used the documented Docker Compose UI gates because the host shell did not expose `node` or `npm`: lint passed; `npm run check:ui-boundaries` passed with 0 boundary violations, 11/11 route-local `apiClient.fetch` calls against baseline, 8 large route files, 3 large feature-shell files, and 99 report-only browser `window.*` usages; 93 unit tests passed; 115 component tests passed with 22.19% statement coverage, 19.31% branch coverage, 23.89% function coverage, and 23.01% line coverage; the production build completed successfully; all 9 mocked Playwright workflows passed; and `npm run test:e2e:live` skipped both live tests because protected credentials and the mutation fixture are not present locally.

Latest Phase 3 regression-hardening verification on June 11, 2026 used the documented Docker Compose UI gates because the host shell did not expose `node` or `npm`: lint passed; `npm run check:ui-boundaries` passed with 0 boundary violations, 11/11 route-local `apiClient.fetch` calls against baseline, 8 large route files, 3 large feature-shell files, and 99 report-only browser `window.*` usages; 95 unit tests passed; 119 component tests passed with 22.31% statement coverage, 19.54% branch coverage, 23.96% function coverage, and 23.12% line coverage against enforced floors of 21%, 18%, 22%, and 22%; the production build completed successfully; all 9 mocked Playwright workflows passed; and `npm run test:e2e:live` skipped both live tests because protected credentials and the mutation fixture are not present locally.

Latest Phase 4 UX/accessibility verification on June 11, 2026 used the documented Docker Compose UI gates because the host shell did not expose `node` or `npm`: lint passed; `npm run check:ui-boundaries` passed with 0 boundary violations, 11/11 route-local `apiClient.fetch` calls against baseline, 8 large route files, 3 large feature-shell files, and 99 report-only browser `window.*` usages; 95 unit tests passed; 121 component tests passed with 22.29% statement coverage, 19.60% branch coverage, 24.04% function coverage, and 23.09% line coverage against enforced floors of 21%, 18%, 22%, and 22%; the production build completed successfully; all 9 mocked Playwright workflows passed; and `npm run test:e2e:live` skipped both live tests because protected credentials and the mutation fixture are not present locally. `actionlint` and the in-app browser control tool were not available in this session, so workflow lint and a separate in-app browser inspection were not re-run.

Latest Phase 5 documentation verification on June 11, 2026 was docs-only and did not change runtime code after the Phase 4 gate. `git diff --check` passed. The full roadmap audit, excluding the Phase 1 section by request, found the protected live execution items as the only enterprise-completion blockers and route-shell size/state as tracked non-blocking maintainability debt.

Latest maintainability-debt verification on June 11, 2026 used Dockerized Node 22 and Playwright runtimes because host `node` and `npm` were unavailable. `npm run check:ui-boundaries` passed with 0 boundary violations, 11/11 route-local `apiClient.fetch` calls against baseline, 8 large route files, 0 large feature-shell files, and 99 report-only browser `window.*` usages. `npm run build` passed. `npm run lint && npm run test` passed with 95 unit tests and 127 component tests; component coverage measured 22.51% statements, 19.88% branches, 24.60% functions, and 23.33% lines against enforced floors of 21%, 18%, 22%, and 22%. All 9 mocked Playwright workflows passed, and `npm run test:e2e:live` skipped both live tests because protected credentials and the mutation fixture are not present locally. This pass retired `AccessPanel.tsx`, Pipeline Runs feature modules, and `SetupWizard.tsx` from the large-file report while preserving AAA, token, role-policy, basic grant, run-card, run-detail, first-install setup, and GitOps starter-output behavior behind existing and focused new tests.

Latest report-only large-route closure verification on June 11, 2026 used Dockerized Node 22 and Playwright runtimes because host `node` and `npm` were unavailable. `npm run check:ui-boundaries` passed with 0 boundary violations, 11/11 route-local `apiClient.fetch` calls against baseline, 0 large route files, 0 large feature-shell files, and 99 report-only browser `window.*` usages. `npm run build` passed. `npm run lint && npm run test` passed with 95 unit tests and 129 component tests; component coverage measured 23.07% statements, 20.58% branches, 24.92% functions, and 23.97% lines against enforced floors of 21%, 18%, 22%, and 22%. All 9 mocked Playwright workflows passed, and `npm run test:e2e:live` skipped both live tests because protected credentials and the mutation fixture are not present locally. This pass split the remaining report-only route debt into feature-owned collection/detail/view/controller modules for Pipeline Runs, Pipelines, Steps, Triggers, Schedules, Scopes, Knowledge Context, and Lab while preserving AAA, GitOps read-only/clone behavior, route synchronization, run logs, setup output, and workflow editor behavior.

Latest route-shell queue closure check on June 11, 2026 used Dockerized Node 22 because host `npm` was unavailable. `npm ci` completed with 0 vulnerabilities, and `npm run check:ui-boundaries` passed with 0 boundary violations, 11/11 route-local `apiClient.fetch` calls against baseline, 0 large route files, 0 large feature-shell files, and 99 report-only browser `window.*` usages. This confirms the continuing non-blocking route-shell extraction queue is closed; only the Phase 0 protected live gate remains open for enterprise completion.

## Change Log

- **June 11, 2026:** Closed the continuing non-blocking route-shell extraction queue in the roadmap after a fresh Dockerized `npm run check:ui-boundaries` report confirmed 0 large route files and 0 large feature-shell files.
- **June 11, 2026:** Completed the final report-only large-route closure pass; `npm run check:ui-boundaries` now reports 0 large route files and 0 large feature-shell files, leaving only the Phase 0 protected live gate open for enterprise completion.
- **June 11, 2026:** Completed the targeted non-blocking maintainability pass for `SetupWizard.tsx`; extracted setup review output, status/result panels, starter-file preview, and shared wizard primitives so the large-file report has 0 feature-shell entries.
- **June 11, 2026:** Completed the targeted non-blocking maintainability pass for `AccessPanel.tsx` and Pipeline Runs feature-shell debt; split Access into controller/view/workspace modules and moved Pipeline Runs card/branch presentation into `PipelineRunCards.tsx`.
- **June 11, 2026:** Clarified the remaining roadmap state: protected live execution is the only enterprise-completion blocker, while state-heavy route shells remain tracked non-blocking maintainability debt.
- **June 11, 2026:** Completed Phase 5 documentation and ownership with a UI source ownership README, feature-contract notes, doc-map linkage, and a roadmap audit that leaves only the protected live gate open for enterprise completion.
- **June 11, 2026:** Completed Phase 4 UX/accessibility consistency with shared workflow dialog, alert, empty-state, icon-button, and toast primitives; migrated active workflow modal/toast surfaces; and kept axe plus keyboard gates green.
- **June 11, 2026:** Completed Phase 3 coverage and regression hardening with permission-race, route-sync, selected-run cleanup, API failure, and read-only affordance tests; fixed run-log deep-link hydration; and raised component coverage floors to 21/18/22/22.
- **June 11, 2026:** Completed Phase 2 boundary enforcement with PR checklist guardrails, `npm run check:ui-boundaries`, typed boundary-check tests, CI integration, and release-visible reports for route transport, large files, and browser API usage.
- **June 9, 2026:** Completed Phase 1 route-shell slimming across Pipeline Runs, Access, Scopes, Triggers, Pipelines, Steps, and Lab; added focused model/component coverage; and recorded the updated local quality baseline.
- **June 9, 2026:** Reframed the roadmap into an enterprise clean-code execution plan, preserved the completed refactor summary, made the protected live execution the explicit completion blocker, and added phased guardrails for route slimming, boundary enforcement, coverage hardening, accessibility consistency, and documentation ownership.
