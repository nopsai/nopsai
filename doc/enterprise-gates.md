# Enterprise Gates

This document captures the Batch 4 production-hardening and CI gate baseline.
The goal is to keep enterprise readiness explicit and repeatable instead of
leaving it as tribal knowledge.

## Startup Gates

Set `NOPSAI_ENVIRONMENT=production` or
`NOPSAI_REQUIRE_PRODUCTION_GATES=true` to make service processes fail closed
when production hardening is incomplete. Shared gate logic lives in
`pkg/startupgates`.

The NopsAI API production gates currently check:

- `NOPSAI_MASTER_KEY` is present, long enough, and not a known placeholder.
- `JWT_SIGNING_KEY` is present, long enough, and not a known placeholder.
- `SERVICE_JWT_SIGNING_KEY` is present, production-grade, and different from
  `JWT_SIGNING_KEY`.
- `AAA_SHARED_INTERNAL_TOKEN` is production-grade and not the local development
  default.
- Dispatcher transport security is not disabled.
- If a GitHub App is configured, private-key and webhook credential references
  are configured.
- The bootstrap administrator is not using the development `admin` password.
  In production gate mode, a missing or insecure bootstrap admin is created or
  rotated only from `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD` or
  `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE`.

Unmet production gates are returned through `/v1/setup/preflight`. If the full
API cannot safely start, the process stays in setup preflight mode so operators
can inspect readiness before login.

Additional service binaries also call the shared startup gates directly:

- `dispatcher` requires production-grade service JWT isolation, dispatcher
  TLS, and a configured NopsAI callback URL.
- `agent` requires dispatcher address, service identity, production-grade
  service JWT signing, and dispatcher TLS when production gates are enabled.
- `aaa` requires a database URL and production-grade shared internal token.
- `git-bot` requires service identity and the NopsAI callback URL; it obtains
  GitHub App credentials from the authenticated broker during startup.
- `runner` and `k8s-runner` require dispatcher address, production-grade
  service JWT isolation, and dispatcher TLS.

## HTTP Server Hardening

NopsAI, setup preflight mode, AAA, and git-bot HTTP servers use shared
production timeout defaults from `pkg/httpapi`:

- read-header timeout: 5 seconds
- read timeout: 15 seconds
- write timeout: 60 seconds
- idle timeout: 120 seconds

## Local Gate Runner

Run the enterprise gate suite locally:

```bash
scripts/enterprise-gates.sh
```

The script runs:

- `scripts/test-backend.sh`
- `scripts/test-backend.sh -race`
- `scripts/release-tooling-test.sh`
- `scripts/license-check.sh`
- `go vet ./...`
- `golangci-lint run ./...`
- `gosec ./...`
- `govulncheck ./...`
- Docker build checks for the base image, all Go service images including the
  restricted Docker socket proxy, the pipeline helper image, and the UI image
- CLI package and command-contract tests, including authenticated `httptest`
  requests, context/credential permissions, diagnostics, exact-byte streaming
  and downloads, and separate `nopsai`/`nopsai-api` build outputs
- Generated API catalog parity against every Go HTTP route registration, so a
  new server API cannot land without CLI discovery and template-call support
- Release compatibility, strict manifest, digest pinning, chart verification,
  deterministic Helm overrides, post-success lockfile, `/version`, and binary
  build-metadata tests
- Commit-count version calculation, PR `+2` forecasts, changelog generation,
  deployment-only Compose rendering, Helm lint/package/render validation, and
  checksums
- Go and UI dependency license compatibility, including blocked-license and
  unknown-license failures plus review/notice reporting for MPL-2.0,
  CC-BY-4.0, BlueOak-1.0.0, Python-2.0, and similar obligations

`scripts/test-backend.sh` tests repository-level packaging contracts, command
entrypoints, internal CLI packages, `config`, shared Go packages, and every
service except `services/ui`. This keeps frontend dependencies under
`node_modules` outside Go package discovery. Docker Compose is reserved for the
install/runtime stack and build-only service images.

The gate itself has read-only repository permissions and never publishes PR
artifacts to a registry. It uploads a release preview with the forecast version.
Main-branch publication is owned by `.github/workflows/platform-release.yml`,
which runs as a separate privileged workflow with job-scoped package and content
permissions. See [release-bundles.md](./release-bundles.md).

Run the UI boundary gate from `services/ui` whenever a frontend change touches
route pages, feature modules, hooks, or shared UI helpers:

```bash
npm run check:ui-boundaries
```

The boundary gate fails on raw `fetch`, new TypeScript suppression comments,
React Hooks/Fast Refresh lint suppressions, and route-local transport growth
beyond the current baseline. It also prints a report-only summary of large route
and feature-shell files plus browser `window.*` usage so release reviews can
track extraction debt before a branch is cut.

The release pipeline `ui-gates` step prints UTC start, finish, and duration
lines for `npm ci`, lint, UI boundary checks, tests, and build. Platform log
timestamps may be rendered in the operator's local timezone while Vitest reports
the Node container timezone, so compare the explicit `duration_seconds` fields
when checking whether the UI gate is slow or looping.

For workflow UI changes that introduce dialogs, empty states, alerts, icon-only
commands, toast/live-region feedback, editor autocomplete, graph controls, or
log dialogs, keep the shared primitives in `services/ui/src/components` as the
default starting point. Component tests should cover accessible names,
descriptions, validation announcements, focus trap behavior, Escape close, Tab
order, outside-click dismissal for transient dialogs/lists, and focus
restoration. The mocked Playwright suite is the release gate for
serious/critical axe violations across login, authenticated workspace, workflow
dialogs, editor autocomplete, graph interaction, and populated logs.

Set `SKIP_DOCKER_BUILDS=1` when validating Go/lint/security gates without
local Docker builds.

Helm 3.17 or newer is required for release chart linting, packaging, and
template validation. CI pins Helm 3.17.3 through `azure/setup-helm`.

`golangci-lint` must be built with the same Go major/minor version as the
module target in `go.mod` or newer. If the local binary was built with an older
Go toolchain, upgrade it before running the gates:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

## CI Gates

`.github/workflows/enterprise-gates.yml` runs the same categories in CI:

- Go test, race test, vet, lint, gosec, and govulncheck
- Go/UI license compatibility
- Docker build checks for service images and the UI image
- UI lint, boundary checks, unit/component tests, and production build
- release-contract validation and a downloadable predicted/actual release
  preview

`.github/workflows/platform-release.yml` runs on pushes to `main` and manual
dispatches for a selected `source_ref`. It validates release package contracts,
publishes version-only GHCR tags and standalone CLI archives, publishes the
versioned OCI Helm chart, and creates or recovers the changelog-backed GitHub
Release only after every image, CLI target, and chart stage succeeds.

`.github/workflows/ui-live-smoke.yml` provides the post-deployment UI gate. It
can be called by a deployment workflow or dispatched manually against a
protected GitHub environment. Configure that environment with:

- variable `NOPS_UI_LIVE_BASE_URL`
- secrets `NOPS_UI_LIVE_USERNAME` and `NOPS_UI_LIVE_PASSWORD`
- secret `NOPS_UI_LIVE_PIPELINE_ID` for the optional mutation job

The authentication job validates login, authorization-controlled navigation,
and setup status. The mutation job only runs when explicitly enabled and
round-trips a dedicated pipeline before starting a smoke run. Live CI fails
closed for missing configuration, retains Playwright diagnostics on failure,
and serializes runs per deployment environment.

Service Dockerfiles that depend on the base image accept `BASE_IMAGE`, so CI can
build from the local `nopsai-base:ci` image instead of pulling a published base.
AAA and agent images copy their binaries from that shared artifact path. The
Docker socket proxy intentionally remains a separate scratch-based image because
it exposes only the minimal read-only Docker API surface for System Logs.
CI uses `golangci/golangci-lint-action@v7` and pins the `golangci-lint`
binary to `v2.12.2`, which is built with a Go 1.26 toolchain and is compatible
with the repository's `go 1.26.5` module target.

## Current Baseline Decision

The gate policy is strict for newly surfaced categories: failures should either
be fixed or documented with a specific, reviewed exclusion. Batch 4 baselines the
known repo-wide lint/security backlog so CI can run consistently while the
backlog is paid down in focused follow-ups.

Current `golangci-lint` baseline in `.golangci.yml`:

- `errcheck`: unchecked close/write/copy/rollback errors across legacy code.
- `staticcheck`: deprecated gRPC/string helpers, style cleanups, and small
  simplifications across moved packages.
- `unused`: legacy helpers left behind by structural extraction.
- `govet`: disabled inside golangci only because `go vet ./...` is a separate
  strict gate.

Current `gosec` baseline in `scripts/enterprise-gates.sh` and CI:

- `G101`: false-positive credential names, setup suggestions, and the tracked
  local development admin/dev-token values guarded by production startup gates.
- `G103`: generated protobuf unsafe calls.
- `G104`: unchecked write/close/copy errors already tracked by the lint
  backlog.
- `G110`: approval-checkpoint decompression warning; checkpoint size limits
  exist but the restore path still needs a scanner-friendly bounded copy.
- `G115`: integer conversion checks around proto `int32` fields and tar header
  modes.
- `G118`: background goroutine context-lifetime warnings in async notification,
  config-sync, dispatcher, and runner flows.
- `G122`: workspace walk/read symlink race warnings that need a root-scoped
  filesystem pass.
- `G301`/`G306`: file and directory permission backlog.
- `G304`: dynamic config/checkpoint path reads that need a broader file-access
  policy pass.
- `G703`: taint-analysis path traversal warnings for config/private-key writes.
- `G704`: agent API URL SSRF taint warnings; agent callback base URLs are
  deployment-controlled but still need a tighter URL policy.

Anything outside these baseline categories should be treated as a failing
enterprise gate.

## Integration Coverage Baseline

The Batch 4 integration baseline is covered by focused component tests rather
than one heavyweight end-to-end fixture:

- Dispatcher queue/assignment and NopsAI callback handoff:
  `services/dispatcher/internal/service/server_test.go`
- Git-bot webhook forwarding and repository provider handoff:
  `services/git-bot/internal/service/server_test.go`
- Run launch handoff through the `RunLauncher` boundary:
  `services/nopsai/run_launcher_test.go`
- GitOps config sync fetch/parse/apply/prune behavior:
  `services/nopsai/config_sync_fetch_test.go`,
  `services/nopsai/config_sync_parse_test.go`, and
  `services/nopsai/config_sync_test.go`
- Agent Profile GitOps parsing, validation, route authorization, prompt
  selection, and UI normalization:
  `services/nopsai/agent_profiles_test.go`,
  `services/nopsai/pkg/validation/pipeline_test.go`,
  `services/agent/internal/llm/agent_profiles_test.go`, and
  `services/ui/src/features/system/agent-profiles/model.test.ts`
- Authorization decisions and inheritance:
  `services/nopsai/aaa_integration_test.go` and
  `services/nopsai/access_grants_test.go`
- Agent lifecycle with direct execution, approvals, includes, retries, and
  status/final-status callbacks:
  `services/agent/internal/app/pipeline_test.go`,
  `services/agent/internal/approval`, `services/agent/internal/include`, and
  `services/agent/internal/resolver`
- Kubernetes runner workspace, pod, and scheduling boundaries:
  `services/k8s-runner/internal/service/runner_test.go`
