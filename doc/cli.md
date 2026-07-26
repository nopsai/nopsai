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

macOS CLI binaries are built on a macOS runner and published as standalone
archives. The repository includes `scripts/sign-notarize-macos-cli.sh` for a
future signed/notarized publication path, but the default release workflow does
not currently require Apple Developer credentials.

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

Running `nopsai` with no subcommand opens the default full-screen operator
console. In a real terminal the CLI uses the alternate screen, clears the view,
and renders the Contextual Zen layout: a quiet home-style status header that
does not change between screens, a centered focused control, a fixed menu
viewport, a guide/details section pinned beneath that viewport, and the same
home-style keybind footer pinned to the bottom. The home screen shows
the CLI version, current context, API URL, token source, authenticated session
user when available, and
lightweight NopsAI health checks for `/healthz`, `/version`, setup preflight,
and `/v1/auth/me`.

From the home menu operators can switch into the full CLI surface: API catalog
calls, raw API requests, route listing, route descriptions, context management,
token login/logout, first-install flows, platform doctor, platform release,
completion generation, guide topics, help, or exit. Nested screens keep the
same visual model: route parameters, raw transport options, install options,
release options, completion output, help, doctor checks, guide text, and API
responses render as separated sections or scrollable result panels instead of
falling back to line-by-line prompts. Esc moves one level back, Enter selects
or accepts the current step, and Ctrl+C exits the interactive session.
Nested menus show the current location above the menu, for example
`Home > API >`, while the top chrome remains unchanged.
Terminals that support ANSI styling receive bold titles, highlighted
selections, colored status cues, and intentionally quiet separators instead of
nested boxes.
Form and wizard screens keep the same centered menu/detail geometry as feature
menus. The central block renders an inline `Parameters` progress list for the
entire form, with completed, active, and pending items shown in place. The
active item expands inline with required status and its current value while
guidance, examples, and validation remain pinned under the same separator.
Pinned detail keys such as `Guide:`, `Example:`, and `Validation:` render bold
on their own line, with content wrapped and indented underneath the key.
Screen-specific action guidance stays in the centered body or guide/details
area so the footer remains stable.

Interactive lists filter live as the operator types. Use Up/Down to move
through visible and off-screen matches, PgUp/PgDn for larger jumps, Home/End for
edges, and Enter to select the highlighted row. Clearing the search shows every
available option again. Live menus reserve the same screen rows whether a list
has two matches or hundreds; large lists show at most 20 rows at a time, then
scroll within that fixed viewport so the guide/details section stays in the
same place. Choice menus use a wider centered block so long route labels remain
visible on typical terminals. API, context, authentication, install, platform,
completion, guide, help, and result screens all reuse the same breadcrumb,
header, footer, menu viewport, and detail positioning. Result viewers render a
fixed `Result` section with scroll ranges in the breadcrumb only when output is
larger than the viewport. API request parameter lists follow the same order as
the wizard steps, including operational inputs such as additional query values,
response format, token attachment, and the final send gate. Empty active input
values render as a blinking cursor instead of a textual blank placeholder. When
stdin or stdout is not a terminal, the CLI uses the same numbered table shape
with explicit prompts so scripted tests and
piped input stay deterministic.

## Built-In Guides

```bash
nopsai guide --list
nopsai guide api
nopsai guide install
nopsai guide monitoring
```

The `guide` command provides CLI-native samples for config, API access,
first-install environments, GitOps, monitoring, AAA, and MCP. It is intended for
operators who know what they want to accomplish but do not remember the exact
subcommand or flag shape.

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
catalog currently covers 351 method/path combinations across operator, public,
and internal service domains. A parity test compares it to the Go route
composition and fails when the server changes without regenerating the catalog.

```bash
# Discover and inspect the compiled API contract without connecting to a server
nopsai api routes
nopsai api routes --domain monitoring --method GET
nopsai api routes --audience public --output json
nopsai api describe GET '/v1/pipelines/{pipelineName...}'
nopsai api describe POST /v1/run --output text

# Search the compiled catalog, select a route, fill parameters, and call it
nopsai api call
nopsai api call --interactive

# Safely expand a registered route template and encode query values
nopsai api call GET '/v1/pipelines/{pipelineName...}' \
  --path pipelineName=delivery/release \
  --query include_source=true
```

`api call` rejects unregistered templates, unknown or missing path parameters,
missing catalogued required query parameters, missing required request bodies,
and slashes in single-segment parameters. Errors name the exact `--path`,
`--query`, `--data`, or `--data-raw` shape to provide and point to
`nopsai api describe METHOD ROUTE_TEMPLATE` for samples. Catch-all parameters
such as `pipelineName...` preserve path segments. Repeated
`--query NAME=VALUE` flags preserve repeated query keys.

`api describe --output text` shows the route audience, required path parameters,
catalogued query parameters, body content type, example payloads when known, and
a noninteractive `api call` command. JSON/YAML output exposes the same metadata
for generated tooling.

Interactive `api call` searches the compiled catalog locally, shows route
guidance in the pinned guide/details section, then opens a step-by-step request
wizard.
Only relevant steps are shown: required path parameters, required query
parameters, additional query assignments when the route exposes them, payload
file or literal content when the route expects a body, bearer-token attachment,
and a final send gate. The advanced response-format step is labeled as
`Response format (HTTP Accept)` so it is clear that it controls response media
negotiation rather than approving the next action. Public routes default to no
bearer token; private routes default to using the current context or
`NOPSAI_TOKEN`. Additional query assignments and payload editor steps support
multiline input: Enter adds a new line, Tab advances to the next step, and
Ctrl+S sends from anywhere. JSON response bodies are pretty-printed in a
scrollable result panel with separate request/result sections. The selected
route still goes through the same path expansion, compatibility, AAA, audit,
and MCP/API middleware as the noninteractive command.

Regenerate the catalog after route-composition changes:

```bash
go generate ./internal/cli/apicatalog
```

The generic concrete-path command remains available for scripts and endpoints
whose path is already known:

```bash
nopsai api request GET /v1/monitoring/summary
nopsai api request GET /v1/git-apps/github
nopsai api request POST /v1/system/config/sync --data sync.json
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  nopsai api request POST /v1/mcp --data -
```

All registered API transport shapes are supported. Git Apps uses the same
generic `api request` path; no CLI version bump or dedicated command is needed
for `/v1/git-apps/github` as long as the API catalog is regenerated after route
composition changes:

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

Repository trigger provider/team/webhook-source metadata, config repository
provider/credential metadata, and Knowledge Context external-page sync fields are
additive to the v1 API and GitOps file format. They do not require a CLI protocol
or version-range bump; released CLIs continue to rely on `/version`
compatibility checks before mutating trigger, config-repository, or
knowledge-context routes.

## Install

```bash
# First-time wizard; choose Docker Compose or Kubernetes
nopsai install

# Automation shortcut for Docker Compose
nopsai install docker-compose \
  --bootstrap-admin-email platform-admin@example.com \
  --bootstrap-admin-password '<initial-password>' \
  --run

# Kubernetes values generation, then edit values and create the referenced Secret
nopsai install kubernetes \
  --output-dir ./nopsai-prod \
  --values-file values.yaml \
  --existing-secret nopsai-secrets \
  --bootstrap-admin-email platform-admin@example.com \
  --bootstrap-admin-password-secret-key bootstrap-admin-password \
  --nopsai-api-url http://nopsai-api.prod.svc:8080 \
  --dispatcher-address dispatcher.prod.svc:9090

# Deploy later from the stored, edited files
cd ./nopsai-prod
nopsai install kubernetes --output-dir . --values-file values.yaml --deploy --wait
```

Run `nopsai install` for the first-time wizard. The wizard uses a full-screen
target picker and editable form for required runtime choices and internal
service topology, then generates the files itself from the selected NopsAI
version. It does not ask for release-manifest files in the normal path. Esc from
an install form returns to the target picker; exact noninteractive subcommands
remain available for GitOps and CI. After the last install parameter is filled,
interactive install shows the equivalent `nopsai install ...` command and waits
for confirmation before writing files or running Docker/Helm.

Across the interactive CLI, actions show an equivalent `nopsai ...` command
preview before they execute. This includes API calls and raw requests, route
listing/description rendering, context changes, token login/logout, completion
generation, platform doctor/release, and install generation. Multiline fields
such as request bodies, headers, extra query assignments, and Helm values files
render their selected multiline content in the form instead of only showing a
line count. Blank optional parameters, including optional multiline parameters,
can be skipped with Enter.

`install docker-compose` is the noninteractive shortcut. It generates a
deployment-only Compose file, `.env` with generated local secrets including the
master key, browser JWT key, service JWT key, AAA shared token, dispatcher TLS
secret, and bootstrap admin password, embedded `db/init.sql`, and a non-secret
`.nopsai/install.lock` from `--version`. With `--run` it executes
`docker compose --env-file .env -f docker-compose.yaml up -d`. Re-run with
`--force` only when replacing generated files is intentional.
Service addresses are written to `.env` and can be set noninteractively with
`--nopsai-api-url`, `--dispatcher-address`, `--aaa-api-url`,
`--git-bot-api-url`, `--gotenberg-url`, and `--docker-network`. The bootstrap
admin email is set with `--bootstrap-admin-email`; omit
`--bootstrap-admin-password` to generate a strong first-login password into the
sensitive `.env` file. Bootstrap-created local admin credentials are treated as
temporary by default and must be changed on first login. The literal built-in
`admin` password is rejected for generated installs; use the repository
development Compose file for local development defaults.

`install kubernetes` generates editable Helm values, `README.md`, and a
non-secret install lock. The generated README records prerequisites, expected
Secret keys, registry pull Secret setup, Secret manifest and `kubectl create
secret` examples, CLI deploy commands, direct Helm commands, and verification
commands. The generated values reference `secrets.existingSecret`; create that
Secret through External Secrets, SOPS, Sealed Secrets, or `kubectl` before
deploying. Kubernetes values include a bundled PostgreSQL StatefulSet by
default; set `postgres.enabled=false` and replace the database URL when using
managed PostgreSQL. That Secret must include database URL, bundled PostgreSQL
password, master key, browser JWT key, service JWT key, AAA shared token,
dispatcher TLS secret, and the bootstrap admin password key named by
`--bootstrap-admin-password-secret-key` because the generated values do not
store plaintext secrets. If the release images are private, create the registry
pull Secret in the namespace and reference it from `global.imagePullSecrets`;
the CLI documents the command but does not own the registry credential. Add
`--deploy --wait` on the first command to deploy
immediately, or run
`nopsai install kubernetes --deploy` later from the stored output directory
after editing values. Stored-file deploys reuse `values.yaml`
without overwriting it, then write a GitOps-readable release lock after
success.
Kubernetes service topology is stored under `topology.nopsaiAPIURL`,
`topology.dispatcherGRPCAddress`, `topology.aaaAPIURL`,
`topology.gitBotAPIURL`, and `topology.gotenbergURL` so multi-cluster,
split-service, or custom-DNS environments can keep those changes reviewable in
Git.

Without `--version`, a released CLI defaults to its own semantic build version.
Development builds prompt for a version in the wizard and require `--version`
for exact noninteractive install commands.

## Advanced Platform Bundles

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

`install` is the first-install entry point. `platform release` is the lower-level
enterprise/GitOps primitive for CI jobs and advanced operators who already have
release manifests and values files and want direct plan/render/deploy control.
Without `--deploy` it runs a plan and prints the rendered manifests. With
`--deploy` it runs the same verified plan and then performs
`helm upgrade --install`, waiting when `--wait` is set.

The platform release command resolves an exact semantic version and requires an
explicit `--manifest` or trusted `NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE`. It
exists for advanced CI/GitOps workflows that already publish digest-pinned
manifests outside the default release pipeline. It validates the manifest and
CLI compatibility, verifies the downloaded OCI Helm chart package digest, and
renders digest-pinned values for every platform image. Plan mode runs
`helm template` and can emit text, JSON, or YAML. Deploy mode runs
`helm upgrade --install` and writes
`.nopsai/release.lock` atomically only after success.
Before deployment, an existing lock is checked for release identity, migration
regressions, and forward-only downgrade restrictions. Keep the lock with the
environment's GitOps state; older locks without rollback metadata are treated as
forward-only.

Use `--manifest-digest sha256:...` to pin the manifest bytes as well as its
contents. Without `--manifest`, released CLI archives use their embedded
manifest for the same version. Set `NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE` only
when intentionally resolving manifests from a trusted internal HTTPS registry.
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
- `internal/cli/apicatalog`: generated route model, path/query/body guidance, samples, and path-template rules
- `internal/cli/apicatalog/internal/discovery`: generator-only Go AST discovery
- `internal/cli/interactive`: alternate-screen selectors, editable forms, scrollable result panels, stdin/stdout fallback prompts, confirmations, and defaults
- `internal/cli/platform`: platform diagnostics, install topology, release resolution, compatibility, Helm execution, and release lock models
- `internal/cli/command`: Cobra routing, interactive home/menu orchestration, guide rendering, hook orchestration, and command rendering
- `internal/cli/command/interactive_*.go`: focused interactive workflow composition for API, auth, platform release, completion, shared screen helpers, and home routing; model/API/platform execution stays in the packages above

The CLI sends ordinary bearer credentials and does not bypass API middleware.
AAA decisions, audit behavior, MCP route rules, and resource visibility remain
owned by the server.
