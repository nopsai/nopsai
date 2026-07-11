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

Released CLI builds are standalone GitHub Release assets, not container-only
tools. Linux and macOS archives contain `nopsai`; Windows archives contain
`nopsai.exe`. Asset names include the exact platform version, operating system,
and architecture, and `SHA256SUMS` covers every published archive.

macOS CLI binaries are built on a macOS runner, signed with a Developer ID
Application certificate, and accepted by Apple's notarization service before
publication. Gatekeeper assessment is part of the release job. This applies to
new releases; an older unsigned archive is not retroactively notarized and
should be replaced with a current release.

## Shell Completion

```bash
nopsai completion bash
nopsai completion zsh --output-dir ./completion
nopsai completion fish --stdout
```

After a shell is selected, the completion command writes a ready-to-copy file in
the current directory by default:

- bash: `nopsai.bash`
- zsh: `_nopsai`
- fish: `nopsai.fish`
- PowerShell: `nopsai.ps1`

The command prints the exact `cp`/`Copy-Item` instructions for common shell
completion directories. It does not modify shell startup files automatically.
Use `--output-dir` to place the generated file somewhere else, or `--stdout`
when package scripts need the raw completion script.

## Interactive Selection

Interactive list prompts render inline below the command as an aligned table
with selection, number, option, and detail columns. They do not switch to an
alternate screen or clear the whole terminal window. The prompt shows the full
matching set through a 10-row viewport. Type to filter live, use Up/Down to move
through visible and off-screen matches, use PgUp/PgDn for larger jumps, and
press Enter to select the highlighted row. Clearing the search shows every
available option again. When stdin or stdout is not a terminal, the CLI uses the
same numbered table shape with an explicit search prompt so scripted tests and
piped input stay deterministic.

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
`nopsai` team. Set `NOPSAI_CONFIG_DIR` or pass `--config-dir` to select a
different directory.

- `config.yaml` contains context names, API URLs, and the current context.
- `credentials.yaml` contains opaque tokens separately.
- Both are atomically written with `0600` permissions in a `0700` directory.
- Credential files with team or world permissions are rejected.
- Stored credentials are removed when a context moves to a different API URL.
- An `--api` override reuses a stored context credential only when the override
  has the same scheme and host. Set `NOPSAI_TOKEN` explicitly for a different
  origin.
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

# Search the compiled catalog, select a route, fill parameters, and call it
nopsai api call
nopsai api call --interactive

# Safely expand a registered route template and encode query values
nopsai api call GET '/v1/pipelines/{pipelineName...}' \
  --path pipelineName=delivery/release \
  --query include_source=true
```

`api call` rejects unregistered templates, unknown or missing path parameters,
and slashes in single-segment parameters. Catch-all parameters such as
`pipelineName...` preserve path segments. Repeated `--query NAME=VALUE` flags
preserve repeated query keys.

Interactive `api call` searches the compiled catalog locally with the shared
10-row live selector, then prompts for required path parameters, optional query
values, request body source for POST/PUT/PATCH routes, response `Accept` header,
and whether to attach configured credentials. Public routes default to no bearer
token; private routes default to using the current context or `NOPSAI_TOKEN`.
The selected route still goes through the same path expansion, compatibility,
AAA, audit, and MCP/API middleware as the noninteractive command.

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

Flag behavior is intentionally explicit:

- `--path NAME=VALUE` fills registered path-template parameters only.
- `--query NAME=VALUE` appends URL query values and can be repeated for the same
  name.
- `--header 'Name: value'` sends additional HTTP headers and rejects newlines.
- `--content-type` and `--accept` override body and response media negotiation.
- `--no-auth` avoids loading or attaching local credentials for public calls.
- `--show-headers` writes status and response headers to stderr before the body.

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
# Plan only; renders digest-pinned Kubernetes YAML
nopsai platform release kubernetes --version 2.7.0 \
  --manifest release-manifest.json --values deploy/production.yaml

# Plan and deploy in one command; works fully noninteractively
nopsai platform release kubernetes --version 2.7.0 \
  --manifest release-manifest.json --values deploy/production.yaml \
  --deploy --wait

# Prompt for target, version, manifest, values, namespace, wait, lock, and apply
nopsai platform release --interactive
```

`platform release` is the deployment entry point. It always resolves and
verifies the selected version before deployment. Without `--deploy` it runs a
plan and prints the rendered manifests. With `--deploy` it runs the same
verified plan and then performs `helm upgrade --install`, waiting when `--wait`
is set. Interactive mode uses the shared 10-row live selector for deployment
targets, prompts every Kubernetes option, shows the plan, and asks for
deployment confirmation.

The platform release command requires an exact semantic version. It validates
the manifest and CLI compatibility, verifies the downloaded OCI Helm chart
package digest, and renders digest-pinned values for every platform image. Plan
mode runs `helm template` and can emit text, JSON, or YAML. Deploy mode runs
`helm upgrade --install` and writes `.nopsai/release.lock` atomically only after
success.
Before deployment, an existing lock is checked for release identity, migration
regressions, and forward-only downgrade restrictions. Keep the lock with the
environment's GitOps state; older locks without rollback metadata are treated as
forward-only.

Use `--manifest-digest sha256:...` to pin the manifest bytes as well as its
contents. Without `--manifest`, the CLI uses the release URL template; set
`NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE` for a trusted internal HTTPS registry.
See [release-bundles.md](./release-bundles.md) for the full contract.

Deployment flags:

- `--version` selects the exact release bundle; released CLIs default to their
  embedded build version.
- `--manifest` points to a local manifest or trusted HTTPS manifest source.
- `--manifest-digest` pins the manifest bytes before decoding the release.
- `--values/-f` repeats Helm values files in GitOps merge order.
- `--release` and `--namespace` select the Helm release identity.
- `--lock-file` writes the GitOps-tracked release lock after successful deploy.
- `--deploy` turns the verified plan into an apply operation.
- `--interactive` prompts for every deployment level while preserving the same
  noninteractive flags for automation.

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
- `internal/cli/interactive`: live 10-row selectors, stdin/stdout fallback prompts, confirmations, and defaults
- `internal/cli/platform`: platform diagnostics, release resolution, compatibility, Helm execution, and release lock models
- `internal/cli/command`: Cobra routing, hook orchestration, and rendering

The CLI sends ordinary bearer credentials and does not bypass API middleware.
AAA decisions, audit behavior, MCP route rules, and resource visibility remain
owned by the server.
