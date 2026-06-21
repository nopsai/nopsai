# NopsAI CLI

The repository builds one operator CLI, `nopsai`, and a separate control-plane
server binary, `nopsai-api`. The CLI keeps platform and API operations under one
context and authentication model while their implementation packages remain
separate.

## Build

```bash
go build -o nopsai ./cmd/nopsai-cli
go build -o nopsai-api ./services/nopsai/cmd/nopsai
```

The base container image also publishes `/nopsai` and `/nopsai-api`. The API
service image copies and starts only `nopsai-api`, as a non-root user.

## Contexts And Authentication

```bash
nopsai context add prod --api https://api.nopsai.example
nopsai context use prod
nopsai context list
nopsai login --token
```

`login --token` accepts an access JWT, `nopat_` personal access token, or
`nopsat_` service-account token. It reads `NOPSAI_TOKEN` first and otherwise
reads stdin, hiding terminal input. The token is verified against
`GET /v1/auth/me` before it is stored.

By default files live under the operating system user config directory in a
`nopsai` folder. Set `NOPSAI_CONFIG_DIR` or pass `--config-dir` to select a
different directory.

- `config.yaml` contains context names, API URLs, and the current context.
- `credentials.yaml` contains opaque tokens separately.
- Both are atomically written with `0600` permissions in a `0700` directory.
- Credential files with group or world permissions are rejected.
- `nopsai logout` deletes the local token; it does not revoke the server-side
  personal or service-account token.

For CI and GitOps automation, commit only declarative context configuration and
inject `NOPSAI_TOKEN` from the platform secret store. Never commit
`credentials.yaml`. Environment credentials override locally stored tokens,
which makes the same commands deterministic in automation.

## Complete API Access

The CLI contains a generated catalog for every route registered by the API. The
catalog currently covers 268 method/path combinations across operator, public,
and internal service domains. A parity test compares it to the Go route
composition and fails when the server changes without regenerating the catalog.

```bash
# Discover and inspect the compiled API contract without connecting to a server
nopsai api routes
nopsai api routes --domain monitoring --method GET
nopsai api routes --audience public --output json
nopsai api describe GET '/v1/pipelines/{pipelineName...}'

# Safely expand a registered route template and encode query values
nopsai api call GET '/v1/pipelines/{pipelineName...}' \
  --path pipelineName=delivery/release \
  --query include_source=true
```

`api call` rejects unregistered templates, unknown or missing path parameters,
and slashes in single-segment parameters. Catch-all parameters such as
`pipelineName...` preserve path segments. Repeated `--query NAME=VALUE` flags
preserve repeated query keys.

Regenerate the catalog after route-composition changes:

```bash
go generate ./internal/cli/apicatalog
```

The generic concrete-path command remains available for scripts and endpoints
whose path is already known:

```bash
nopsai api request GET /v1/monitoring/summary
nopsai api request POST /v1/system/config/sync --data sync.json
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  nopsai api request POST /v1/mcp --data -
```

All registered API transport shapes are supported:

```bash
# YAML or other non-JSON request bodies
nopsai api request PUT /v1/pipelines/delivery/release \
  --data release.yaml --content-type application/x-yaml

# Public endpoint without loading or attaching local credentials
nopsai api request GET /v1/auth/providers --no-auth

# Exact-byte ZIP, PDF, spreadsheet, or backup download
nopsai api request GET /v1/setup/templates.zip \
  --output-file nopsai-gitops-starter.zip --show-headers

# Long-lived SSE stream; zero disables the client timeout explicitly
nopsai --timeout 0 api call GET \
  '/v1/system/logs/sources/{sourceID}/stream' \
  --path sourceID=dispatcher --accept text/event-stream
```

`--data-raw` sends a literal body, while `--data -` streams stdin. Request paths
must be host-free absolute paths. Response bytes are never reformatted, so
binary and streaming responses remain intact. Successful `--output-file`
writes are atomic and use `0600` permissions; error bodies stay on stdout and do
not replace the requested file. The client refuses parent traversal and
cross-origin redirects so bearer credentials cannot be redirected to another
origin. `--api` and `--context` provide non-persistent overrides;
`--timeout` controls request duration.

Both API commands preserve the response body and exit non-zero for non-2xx
responses. Internal routes remain catalogued for service operators, but they
still require a valid internal service token and are not elevated by the CLI.
Typed workflow commands can layer on this complete transport without creating a
second API implementation.

Released CLI builds validate `GET /version` before mutating API requests. The
request is stopped locally when the platform API version or reciprocal semantic
version range is incompatible. Development builds retain a deliberate bypass
until release metadata is injected by the build.

## Platform Bundles

```bash
nopsai platform plan kubernetes --version 2.7.0 \
  --manifest release-manifest.json --values deploy/production.yaml
nopsai platform deploy kubernetes --version 2.7.0 \
  --manifest release-manifest.json --values deploy/production.yaml --wait
```

Both commands require an exact semantic version. They validate the manifest
and CLI compatibility, verify the downloaded OCI Helm chart package digest,
and render digest-pinned values for every platform image. `plan` runs `helm
template` and can emit text, JSON, or YAML. `deploy` runs `helm upgrade
--install` and writes `.nopsai/release.lock` atomically only after success.

Use `--manifest-digest sha256:...` to pin the manifest bytes as well as its
contents. Without `--manifest`, the CLI uses the release URL template; set
`NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE` for a trusted internal HTTPS registry.
See [release-bundles.md](./release-bundles.md) for the full contract.

## Platform Doctor

```bash
nopsai platform doctor
nopsai platform doctor --output json
nopsai platform doctor --output yaml
```

Doctor checks:

- `helm`, `kubectl`, and `docker` availability as advisory runtime checks
- bounded Kubernetes API and Docker daemon connectivity when their clients exist
- public API setup preflight readiness
- Prometheus metrics endpoint reachability
- token acceptance through `/v1/auth/me`
- dispatcher monitoring and registered runner count when AAA permits it

Missing optional local tools, unavailable local platform runtimes, and missing
dispatcher-read permission are warnings. NopsAI API readiness, metrics,
connectivity, malformed responses, and token rejection are errors and cause a
non-zero exit. JSON/YAML output is intended for CI and operational monitoring
ingestion.

## Code Ownership

- `cmd/nopsai-cli`: process composition and exit status only
- `internal/cli/config`: context model, validation, atomic persistence, tokens
- `internal/cli/client`: authenticated HTTP transport and origin controls
- `internal/cli/apicatalog`: generated route model and path-template rules
- `internal/cli/apicatalog/internal/discovery`: generator-only Go AST discovery
- `internal/cli/platform`: platform diagnostics, release resolution, compatibility, Helm execution, and release lock models
- `internal/cli/command`: Cobra routing, hook orchestration, and rendering

The CLI sends ordinary bearer credentials and does not bypass API middleware.
AAA decisions, audit behavior, MCP route rules, and resource visibility remain
owned by the server.
