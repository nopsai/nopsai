## Enterprise UI Checklist

- [ ] UI model, API, hook, and rendering changes stay in their feature-owned boundary.
- [ ] Route pages do not add transport; use `apiClient` only through `src/lib/api.ts`, app-shell helpers, or feature-owned `api.ts`.
- [ ] No direct `fetch`, new TypeScript suppression comments, or React Hooks/Fast Refresh lint suppressions were added.
- [ ] Browser APIs such as `window.confirm`, `window.location`, resize listeners, or storage events are isolated in hooks/helpers when new behavior needs them.
- [ ] AAA checks fail closed at action time and remain keyed to the active resource/folder/selection.
- [ ] GitOps read-only, clone, local draft, and route compatibility behavior are preserved.
- [ ] Tests and docs were updated with the same change; run `npm run check:ui-boundaries` from `services/ui` for UI changes.
