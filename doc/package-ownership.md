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

## Operator CLI Ownership

- Local context and credential model logic and atomic storage live in
  `internal/cli/config`.
- Authenticated REST transport, URL construction, headers, and redirect policy
  live in `internal/cli/client`.
- Generated API route metadata and path-template expansion live in
  `internal/cli/apicatalog`; Go AST route discovery is generator/test-only in
  `internal/cli/apicatalog/internal/discovery`.
- Live 10-row selectors, searchable prompts, confirmations, and default-aware
  stdin/stdout fallback interaction primitives live in
  `internal/cli/interactive`.
- Platform diagnostic rules, release manifest resolution, compatibility checks,
  install-file planning, Helm process orchestration, and deployment lock models live in
  `internal/cli/platform`.
- Cobra command/hook orchestration, install-vs-deploy flow decisions for stored
  generated files, and text/JSON/YAML rendering live in `internal/cli/command`.
- Route composition and process exit behavior live in `cmd/nopsai-cli`.
- Embedded immutable installer assets such as the database bootstrap SQL are
  exposed from the asset-owning package (`db`) so released CLI binaries do not
  depend on a source checkout.

Typed API and platform features must extend these owners instead of placing
model, transport, subprocess orchestration, and rendering in one command file.
Server route additions must regenerate the API catalog; the parity test is the
contract preventing silent CLI coverage drift.

Shared immutable binary identity lives in `pkg/buildinfo`. Manifest parsing,
semantic-version ranges, and capability validation live in
`pkg/compatibility`. The API only renders shared build identity; it does not own
release model logic.

## Release Automation Ownership

- `release/version.txt` owns the release major/minor series; Git history owns
  the patch number.
- `scripts/release-version.sh` owns version calculation, including PR forecast
  offsets.
- `scripts/generate-changelog.sh` owns deterministic history-to-Markdown
  rendering.
- `scripts/render-release-bundle.sh` owns deployment artifact composition,
  release-manifest rendering, and image-lock rendering.
- `deploy/` owns deployment-only Compose, the NopsAI Helm chart, and the
  release image overlay used to create digest-pinned chart packages.
- `doc/sample-config-repo/global-repo/triggers/hosein-yousefii/pre-nopsai.yaml`
  owns the GitHub App main-branch release trigger.
- `doc/sample-config-repo/global-repo/pipelines/platform/prod/nopsai-platform-release.yaml`
  owns release package validation plus GHCR, OCI Helm, CLI, deployment bundle,
  and GitHub Release publication.

GitOps pipeline and trigger YAML should orchestrate these owners rather than
duplicate their model or rendering logic inline.

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
  team-membership logic live in `services/nopsai/auth_oidc_store.go`.
- OIDC provider HTTP behavior, metadata discovery, token exchange, JWKS
  verification, PKCE, nonce, and safe redirect rules live in
  `services/nopsai/auth_oidc_flow.go`.
- Provider-specific entitlement lookups, such as Keycloak Admin API role and
  team-role reads, live in `services/nopsai/auth_keycloak_entitlements.go`.
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

## System Logs Ownership

- Model, registry, signed cursor, redaction, ring buffer, fan-out, limits, and
  provider contracts live in `services/nopsai/internal/systemlogs`.
- Docker list/inspect/log transport and multiplexed stream decoding live in
  `services/nopsai/internal/systemlogs/docker`.
- Kubernetes pod discovery, label-to-source mapping, and `pods/log` streaming
  live in `services/nopsai/internal/systemlogs/kubernetes`.
- API, SSE heartbeat/reset composition, and content-free stream audit events
  live in `services/nopsai/system_logs_handlers.go`; route composition remains
  in `services/nopsai/routes.go` and route authorization in `pkg/routeauthz`.
- UI transport lives in `services/ui/src/features/system/logs/api.ts`, hook and
  reconnect orchestration in `useSystemLogs.ts`, data contracts in `types.ts`,
  and rendering in `SystemLogsPanel.tsx`.
- Docker proxy topology is deployment-owned in `docker-compose.yaml`; Kubernetes
  RBAC and provider env wiring are deployment-owned in the Helm chart. Metrics
  are exposed by the existing `services/nopsai/metrics.go` owner.

## Logging And Correlation Ownership

- Request ID and traceparent context helpers live in `pkg/correlation`.
- Shared stdout/stderr routing, HTTP access logging, and gRPC client/server
  logging interceptors live in `pkg/servicelog`.
- NopsAI request/audit middleware composition lives in
  `services/nopsai/http_middleware.go`; route composition remains in
  `services/nopsai/routes.go`.
- Durable pipeline log schema and existing-database convergence live in
  `db/init.sql` and `services/nopsai/run_dispatch_schema.go`. Log ingest and
  REST response shaping live in `services/nopsai/run_internal_handlers.go`.
- Dispatcher owns translating authenticated gRPC log batches into NopsAI HTTP
  ingest metadata in `services/dispatcher/internal/service`.

## Pipeline Final Output Ownership

- Provider-specific system instruction transport lives in `pkg/llmclient`.
- Versioned `DocumentSpec` and `SpreadsheetSpec` DTOs live in
  `pkg/models/final_output_specs.go`; strict schema and size rules live in
  `services/nopsai/pipeline_final_output_specs.go`.
- Final-output envelope extraction, schema dispatch, and retry rules live in
  `services/nopsai/pipeline_final_output_contract.go`.
- Generation orchestration, persistence transitions, and AI usage reporting
  live in `services/nopsai/pipeline_final_outputs.go`.
- Download dispatch lives in `pipeline_final_outputs_render.go`; server HTML,
  Gotenberg PDF transport, and Excelize workbook construction live in
  `pipeline_final_outputs_document.go`, `pipeline_final_outputs_pdf.go`, and
  `pipeline_final_outputs_spreadsheet.go`.
- Final-output schema evolution lives in
  `services/nopsai/pipeline_final_outputs_schema.go` and `db/init.sql`.
- Run-detail persistence reads live in `services/nopsai/internal/runs`.
- UI actions live in `RunFinalOutputs.tsx`; format parsing/rendering lives in
  `final-output-preview/`; route composition remains in `RunDetailPanel.tsx`.

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
