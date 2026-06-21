# System Logs

System Logs provides live operational visibility for persistent NopsAI control-plane containers. It is separate from pipeline run/task logs and never writes platform log lines to `pipeline_run_logs` or Postgres.

## Architecture and ownership

The browser opens one authenticated `fetch()` stream through `apiClient`. The NopsAI API authorizes the logical source, applies best-effort redaction and line limits, writes the sanitized entry to a bounded per-source ring buffer, and fans it out as Server-Sent Events (SSE). `services/nopsai/internal/systemlogs` owns registry, cursor, redaction, buffer, broker, and provider contracts. `system_logs_handlers.go` owns REST/SSE composition. `services/ui/src/features/system/logs` separates transport, hook orchestration, types, and rendering.

Docker deployments connect NopsAI to `docker-socket-proxy:2375`. Only the proxy mounts `/var/run/docker.sock`, read-only. The repo-owned proxy accepts only ping/version, container list, and allow-listed container inspect/log GET requests; it rejects all mutations, events, archive, stats, non-platform containers, and unknown query parameters. The Docker runner socket is not reused because runners may be remote and own job execution rather than control-plane observability.

Operators can consume the same authenticated SSE contract through the CLI
without response buffering or byte rewriting:

```bash
nopsai --timeout 0 api call GET \
  '/v1/system/logs/sources/{sourceID}/stream' \
  --path sourceID=dispatcher --accept text/event-stream
```

The generated route catalog marks this endpoint as streaming. Existing AAA
source visibility, cursor signing, redaction, audit, and rate limits remain
server-owned.

Platform identity is monitored separately from logs. Prometheus exports
`nopsai_build_info` with immutable version, commit, API version, and release
manifest digest labels so mixed control-plane bundles can be detected without
parsing log lines.

Allow-listed source IDs are `nopsai`, `aaa`, `dispatcher`, `git-bot`, `ui`, and optional `docker-runner`. Build-only `base`, `agent`, `pipeline`, and `k8s-runner` containers are not registered. Arbitrary container names and IDs are never accepted.

The UI stream label reflects the container's real stdout/stderr file descriptor.
NopsAI Go services route `trace`, `debug`, and `info` events to stdout and route
`warn`, `error`, `fatal`, and `panic` events to stderr. Stream and display
toggles use a selected background, accent ring, status dot, and `aria-pressed`
state so their current selection is visible and accessible.

## API and AAA

- `GET /v1/system/logs/sources` filters source discovery through AAA.
- `GET /v1/system/logs/sources/{sourceID}/tail?lines=500` returns a bounded snapshot.
- `GET /v1/system/logs/sources/{sourceID}/stream?tail=500&cursor=...` returns SSE with `status`, `log`, and `reset` events plus heartbeat comments.
- Action: `system_log.read`
- Resource: `system_log:<sourceID>`

Grant `system_log.read` on `system_log:*` for all platform sources or on an individual source. Stream open/close and source selection are audited; log content is never included in audit metadata. Hosted MCP exposes `nopsai.list_system_log_sources` and `nopsai.tail_system_logs`. Long-lived streams remain UI-only.

## Limits and redaction

Defaults are 10,000 entries or 15 minutes per source, a 2,000-line maximum tail, 16 KiB per line, 20 concurrent streams globally, and 10 per source. Stream opens are limited to 30 per actor per minute. HMAC-signed cursors prevent client tampering; an evicted cursor emits `reset` with `cursor_expired`.

Redaction masks common authorization headers, tokens, passwords, API keys, client secrets, and credential-bearing database URLs before buffering or returning data. Redaction is best effort, so operators must still avoid logging sensitive values and keep `system_log.read` narrowly assigned.

## GitOps and deployment configuration

Compose declares the socket proxy and sets `SYSTEM_LOGS_DOCKER_HOST=tcp://docker-socket-proxy:2375`. Other deployments can configure the feature declaratively in the mounted NopsAI YAML:

```yaml
system_logs:
  enabled: true
  docker_host: tcp://docker-socket-proxy:2375
  buffer_lines: 10000
  buffer_age_minutes: 15
  max_tail_lines: 2000
  max_line_bytes: 16384
  max_streams: 20
  max_streams_per_source: 10
```

`SYSTEM_LOGS_DOCKER_HOST` overrides the YAML host for deployment topology. Omitting a host disables collection while leaving source status visible as unavailable. Kubernetes collection should implement the same provider interface with label-selected pods and read-only `pods`/`pods/log` RBAC; it is not enabled in this release.

## Monitoring

`/metrics` exports active/opened stream counts, provider reconnects/errors, redacted lines, and slow-consumer drops under the `nopsai_system_log_*` namespace. Alert on sustained provider errors or dropped lines, and investigate frequent reconnects alongside container restart boundaries shown in the UI.
