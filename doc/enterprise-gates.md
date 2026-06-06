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
- If a GitHub App is configured, `GITHUB_WEBHOOK_SECRET` is production-grade.
- The built-in `admin@example.com` account is not using the default password.
  In production gate mode, a missing default admin is not auto-seeded.

Unmet production gates are returned through `/v1/setup/preflight`. If the full
API cannot safely start, the process stays in setup preflight mode so operators
can inspect readiness before login.

Additional service binaries also call the shared startup gates directly:

- `dispatcher` requires production-grade service JWT isolation, dispatcher
  TLS, and a configured NopsAI callback URL.
- `agent` requires dispatcher address, service identity, production-grade
  service JWT signing, and dispatcher TLS when production gates are enabled.
- `aaa` requires a database URL and production-grade shared internal token.
- `git-bot` requires GitHub App identifiers, private key path, webhook secret,
  and NopsAI callback URL.
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

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `golangci-lint run ./...`
- `gosec ./...`
- `govulncheck ./...`
- Docker build checks for the base image, all Go service images, the pipeline
  helper image, and the UI image

Set `SKIP_DOCKER_BUILDS=1` when validating Go/lint/security gates without
local Docker builds.

`golangci-lint` must be built with the same Go major/minor version as the
module target in `go.mod` or newer. If the local binary was built with an older
Go toolchain, upgrade it before running the gates:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
```

## CI Gates

`.github/workflows/enterprise-gates.yml` runs the same categories in CI:

- Go test, race test, vet, lint, gosec, and govulncheck
- Docker build checks for service images and the UI image

Service Dockerfiles that depend on the base image accept `BASE_IMAGE`, so CI can
build from the local `nopsai-base:ci` image instead of pulling a published base.
CI uses `golangci/golangci-lint-action@v7` and pins the `golangci-lint`
binary to `v2.12.2`, which is built with a Go 1.26 toolchain and is compatible
with the repository's `go 1.26.4` module target.

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
  local default admin/dev-token values guarded by production startup gates.
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
