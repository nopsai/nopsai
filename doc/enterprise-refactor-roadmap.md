# Enterprise Refactor Roadmap

This roadmap tracks clean-code separation for enterprise features. Keep new
work small, owned, testable, and compatible with config/GitOps deployment.

## Current Ownership Baseline

- Model/config logic: `config/config.go`, feature-local `model.ts`, and backend
  DTO/model files near handlers.
- API logic: backend handlers under `services/nopsai/*_handlers.go`; UI API
  calls under feature `api.ts` files or `services/ui/src/lib/api.ts` for shared
  auth/session behavior.
- Hook orchestration: React hooks named `use*` own loading, saving, draft state,
  and toast orchestration.
- Rendering: page and feature components own layout and interaction controls,
  not persistence or provider workflows.
- Route composition: backend route registration stays in
  `services/nopsai/routes.go`; frontend route composition stays in
  `services/ui/src/app/AppRoutes.tsx` and navigation files.

## Enterprise SSO Status

Enterprise SSO is separated into:

- Config/GitOps shape in `config/config.go` and `config.yml`.
- Auth schema in `services/nopsai/auth_schema.go`.
- OIDC persistence in `services/nopsai/auth_oidc_store.go`.
- OIDC provider flow/security in `services/nopsai/auth_oidc_flow.go`.
- Transport DTOs and handlers in `services/nopsai/auth_oidc_models.go` and
  `services/nopsai/auth_oidc_handlers.go`.
- Session issuance in the existing `services/nopsai/pkg/auth` package.
- Login UI API helpers in `services/ui/src/lib/api.ts`.
- System Access Identity Provider model/hook/rendering in
  `services/ui/src/features/system/access`.

## Completed Follow-Up Targets

- Added backend integration coverage with an in-process OIDC provider that
  serves discovery, token exchange, and rotating JWKS.
- Added mocked browser coverage for direct provider login, email discovery
  fallback, and session-code exchange into a Nopsai session.
- Added external ownership enforcement for OIDC users: Keycloak-sourced global
  roles are resynced on login, local role/basic-role writes are rejected, and
  System Access renders friendly external identity labels instead of raw OIDC
  subjects.
- Added Keycloak-to-NopsAI auth-team mapping so identity-provider teams can
  drive NopsAI team-scoped/basic grants while membership remains externally
  owned.
- Added Keycloak entitlement sync so direct client roles become global NopsAI
  access roles and team client roles become scoped Basic roles on matching
  team targets.
- Moved System Access provider admin API calls into
  `services/ui/src/features/system/access/api.ts` while shared login/session
  behavior remains in `services/ui/src/lib/api.ts`.
- Added a Compose Keycloak fixture with a seeded realm, users, teams, role
  mappings, and auth-team mappings for local end-to-end SSO testing.
- Implemented the single encrypted credential registry, management API/UI,
  AAA actions, versioning, audit/access records, and deletion safeguards.
- Migrated OIDC, mail, LLM, MCP, and GitHub integrations to stable credential
  references with no runtime environment-value fallback.
- Added authenticated encrypted GitHub credential delivery from `nopsai` to
  `git-bot`.
- Added config-repository import/export for identity-provider, mail, LLM, and
  MCP policy using credential references. GitOps creates pending metadata but
  never stores credential values.
- Added provider-neutral Git webhook triggering with:
  - shared trigger matching in `pkg/gittrigger`
  - isolated provider normalization/authentication in
    `services/nopsai/internal/gitwebhook`
  - source model, schema, persistence, handlers, and run orchestration in
    focused `git_webhook_*` files
  - UI model, API, hook, form, rendering, and route composition under
    `services/ui/src/features/git-webhook-sources`
  - AAA, encrypted credential references, GitOps sync/export/drift, delivery
    audit/idempotency, and changed-file include/exclude filters
