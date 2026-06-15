# Package Ownership Rules

Use these rules when adding or moving code. They are intentionally simple so
reviews can spot ownership drift quickly.

## Handlers

HTTP and gRPC handlers own transport concerns only:

- route registration
- request decoding and validation of transport shape
- authentication/authorization checks at the API boundary
- response status, headers, and DTO serialization

Handlers should not own durable business workflows, database transaction plans,
provider-specific HTTP/gRPC plumbing, or long-running orchestration.

## Services

Service packages own business workflows:

- run creation and lifecycle transitions
- config sync coordination
- dispatcher scheduling and runner lifecycle
- git-bot webhook/check-run workflows
- agent pipeline execution orchestration

Services may depend on narrow consumer-owned interfaces. They should not depend
directly on concrete provider clients when an interface already exists.

## Stores And Repositories

Store/repository packages own persistence only:

- SQL statements and scanning
- transaction helpers
- persistence-oriented filters and pagination
- durable status updates

Stores should not make authorization decisions, call remote providers, launch
runs, or build HTTP responses.

## Domain And Internal Rule Packages

Domain/internal packages own pure rules and data shaping:

- path normalization
- scheduling decisions
- config sync ownership and drift rules
- action preparation and resolver decisions
- check-run rendering

These packages should stay deterministic and easy to unit test. Prefer passing
inputs explicitly over reading global process state.

## DTOs

DTOs live at API boundaries:

- REST request/response shapes live near the handlers or shared model package
  when reused across services.
- gRPC DTOs live in `pkg/proto`.
- Config-file/GitOps DTOs live near the parser that owns the file format.

Do not leak persistence-only records into public API responses unless that shape
is intentionally part of the contract.

## Provider Clients

Provider clients stay behind interfaces:

- GitHub/git-bot behavior behind `GitProvider`
- dispatcher gRPC behavior behind `DispatcherClient`
- AAA HTTP/local behavior behind `AAAClient`
- config sync persistence/apply behavior behind `ConfigSyncStore`
- agent run launch behavior behind `RunLauncher`
- secret encryption/decryption behind `SecretCodec`

Concrete HTTP/gRPC/Postgres clients are wired in command/bootstrap packages.

## Enterprise SSO Ownership

Enterprise authentication follows the same split:

- Config model logic lives in `config/config.go`.
- OIDC persistence, provider records, state, login-code, external identity, and
  group-membership logic live in `services/nopsai/auth_oidc_store.go`.
- OIDC provider HTTP behavior, metadata discovery, token exchange, JWKS
  verification, PKCE, nonce, and safe redirect rules live in
  `services/nopsai/auth_oidc_flow.go`.
- Provider-specific entitlement lookups, such as Keycloak Admin API role and
  group-role reads, live in `services/nopsai/auth_keycloak_entitlements.go`.
- REST request/response DTOs and transport handlers live in
  `services/nopsai/auth_oidc_models.go` and
  `services/nopsai/auth_oidc_handlers.go`.
- Route composition lives in `services/nopsai/routes.go`; auth/public-path
  middleware lives in `services/nopsai/http_middleware.go`.
- JWT and refresh-token session issuance remains owned by
  `services/nopsai/pkg/auth`.
- UI auth API helpers live in `services/ui/src/lib/api.ts`.
- UI System Access Identity Provider API calls live in
  `services/ui/src/features/system/access/api.ts`.
- UI System Access model parsing/payload shaping lives in
  `services/ui/src/features/system/access/model.ts`.
- UI hook orchestration lives in
  `services/ui/src/features/system/access/useSystemAccess.ts` and
  `useAccessPanelController.ts`.
- UI rendering for Identity Providers lives in
  `services/ui/src/features/system/access/IdentityProvidersWorkspace.tsx`.

## Git Webhook Source Ownership

Provider-neutral Git webhook triggering follows the same split:

- Trigger model fields live in `pkg/models`; pure event/branch/path matching
  lives in `pkg/gittrigger`.
- Provider authentication and payload normalization live in
  `services/nopsai/internal/gitwebhook`.
- Source DTOs and validation live in
  `services/nopsai/git_webhook_sources_model.go`.
- Persistence and schema ownership live in
  `git_webhook_sources_store.go` and `git_webhook_sources_schema.go`.
- REST transport and public delivery ingress live in
  `git_webhook_sources_handlers.go`; run selection and launch orchestration
  live in `git_webhook_orchestrator.go`.
- GitOps parse/apply/export/drift integration lives in
  `git_webhook_sources_gitops.go` and the existing config-sync ownership files.
- Backend route composition stays in `services/nopsai/routes.go`.
- UI model, API transport, hook orchestration, form rendering, and feature
  rendering are separated under
  `services/ui/src/features/git-webhook-sources`.
- Frontend route composition stays in `services/ui/src/app/AppRoutes.tsx`;
  `services/ui/src/pages/GitWebhookSources.tsx` remains a thin route adapter.

## Commands And Bootstrap

Command entrypoints should be thin:

- load config
- configure logging
- construct concrete dependencies
- hand off to an importable app/service package
- own process lifecycle and signal handling

When startup logic grows, move it behind an `internal/app` package before adding
more behavior to `cmd`.
