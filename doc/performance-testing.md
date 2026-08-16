# Backend Performance Testing

This document describes the load-test harness that measures how the NopsAI
backend behaves under concurrency.

The load-bearing services are **nopsai** (the API), **aaa** (auth and
authorization), the **dispatcher**, **Postgres**, and the **UI**. The harness
puts all of them under the same pressure at once and reports, per service, how
much load each carried, how far its latency stretched, and which one gave out
first. That comparison is the point: it turns "the platform is slow" into "this
service is the constraint, scale it first".

The harness lives in three places:

| Path | Owns |
| --- | --- |
| [`internal/perf`](../internal/perf) | Measurement logic: scenarios, ramp execution, statistics, resource sampling, analysis, reporting. |
| [`cmd/nopsai-perf`](../cmd/nopsai-perf) | The `nopsai-perf` binary entry point. |
| [`test/perf/run-perf-test.sh`](../test/perf/run-perf-test.sh) | Orchestration: prerequisite checks, stack health gate, presets. |

## Quick start

The stack must be running before any load test:

```bash
docker compose up --build -d
```

Export credentials for the harness, then run a preset:

```bash
export NOPSAI_PERF_IDENTIFIER='admin@example.com'
export NOPSAI_PERF_PASSWORD='<the admin password>'
# Only for the webhook and pipeline suites. This must be the GitHub App webhook
# secret configured in the platform, not an arbitrary value. See "Webhook secret".
export GITHUB_WEBHOOK_SECRET='<the configured GitHub App webhook secret>'

test/perf/run-perf-test.sh --preset standard
```

Reports are printed to stdout and written to `test/perf/results/` as a
timestamped `.txt` (for reading) and `.json` (for trend charts or gates).

## Presets

| Preset | Suites | Ramp | Roughly |
| --- | --- | --- | --- |
| `quick` | api-read | 1, 5, 20 | 1 minute |
| `standard` | api-read, auth, runtime, ui | 1 → 50 | 3 minutes |
| `stress` | api-read, auth, runtime, ui | 10 → 400 | 5 minutes |
| `full` | api-read, auth, pipeline (self-contained) | 1 → 50 | 30+ minutes |
| `full-webhook` | as `full`, plus the git webhook path | 1 → 50 | 30+ minutes |

Any `nopsai-perf` flag can follow a preset and overrides it:

```bash
test/perf/run-perf-test.sh --preset stress --concurrency 50,100,250,500
```

## Suites

### `api-read`

Weighted GETs across the read surface: `/v1/runs`, `/v1/monitoring/summary`,
`/v1/monitoring/runs/analytics`, `/v1/monitoring/pipelines/performance`,
`/v1/monitoring/dispatcher`, `/v1/pipelines`, `/v1/teams`, `/metrics` and
`/healthz`. Run listings and monitoring aggregates carry the highest weight
because they are the expensive queries; `/healthz` is included unauthenticated
at low weight as a control signal that stays flat unless the process itself is
starved.

This suite isolates API serialization plus Postgres query cost.

### `auth`

`POST /v1/auth/login`, `GET /v1/auth/me`, `GET /v1/access/grants` and
`POST /v1/authz/resource-use/check`. This covers the credential path and the
AAA service that sits in front of every other request.

`/v1/access/grants` is used rather than `/v1/access/effective-permissions`
because the latter requires `action`, `resource_type` and `resource_id` query
parameters that must resolve to a resource which actually exists in the target
environment. A load scenario cannot assume any particular resource, so it would
return HTTP 400 everywhere. Listing grants exercises the same AAA decision path
without that coupling.

Login carries a small weight relative to token-bearing calls: password hashing
is expensive by design, and a realistic mix has far more authenticated requests
than logins. A denied authorization decision still counts as a successful
request, because the suite measures how fast the authorization path answers,
not what it answers.

### `webhook`

Signed `POST` deliveries to the git-bot webhook endpoint, exercising HMAC
verification and dispatch enqueue without waiting for pipelines to finish.
Every delivery carries a fresh UUIDv4 delivery ID so upstream deduplication
cannot silently discard load. The signature scheme matches
`services/git-bot`'s `X-Hub-Signature-256` verification.

### `runtime`

The traffic a pipeline generates against the platform **while it executes**: log
batches streaming in, run status being polled, and logs being read back. Every
one of these calls is authorized, so aaa is exercised at the same rate, and log
ingest queues one database insert per line, so it is the heaviest write path the
platform has.

This is deliberately measured **without executing pipelines**. Running a real
pipeline mostly exercises the runner and the agent, which are not the services
under test; what matters here is the load those runs place on nopsai, aaa and
Postgres. Driving that traffic directly makes it repeatable, free, and
independent of runner capacity.

The suite needs runs to write against. It discovers recent ones automatically
(`--runtime-runs`, default 5, spread evenly so writes do not concentrate on one
row), or takes `--runtime-run-ids` explicitly. It appends log lines marked
`nopsai-perf` to those runs, so point it at a test environment and expect the
log table to grow.

### `ui`

Loads the UI container, which serves static assets through nginx. It is the
control in the comparison: if the UI degrades at the same point as the API, the
constraint is the host rather than either service.

### `pipeline`

Whole pipelines end to end, measured in runs and minutes rather than requests
and milliseconds. Use this to measure orchestration overhead; use `runtime` to
measure the load a run places on the services. Concurrency here means "families in flight", not "requests per
second".

Runs are started through a **purpose-built fixture**, not by replaying a git
event. The fixture lives in
[`test/perf/fixtures`](../test/perf/fixtures/README.md) and must be installed
into your config repositories once before the suite can run:

- `perf-load-probe.yaml` — a pipeline with `llm_enabled: false` and `script`
  steps, so runs cost nothing and take the same time every execution.
- an external trigger bound to it, which the harness invokes.

This keeps the measurement about the platform's own orchestration cost — queue
time, dispatch, runner startup, step execution, completion — rather than about
whatever a real pipeline happens to call. The invoke response names the run it
created, so the harness follows exactly the run it started.

`--pipeline-work-seconds` shapes the run: `0` measures pure orchestration
overhead, a larger value models a realistic pipeline while keeping the work
constant so levels stay comparable.

It reports three distinct latencies:

- **Ingest**: the trigger call round trip.
- **Queue**: the delay from an accepted trigger to the run becoming visible
  through the API.
- **Family**: trigger through the last run in the family completing.

This suite creates real runner containers.

If no run appears within `--pipeline-first-run-timeout` (default 60s), the
family fails immediately with the reason rather than holding the full
`--pipeline-timeout` open.

#### Driving it through a git webhook instead

`--pipeline-trigger webhook` replays a signed git event, which measures the real
ingestion path. It only works when the payload describes a **commit that
exists** in a repository the platform can read, because the pipeline definition
is resolved from that repository at that SHA. It also generates real
github.com traffic. Prefer the default unless you specifically need to measure
git ingestion.

## Requirements for the `webhook` suite

The `webhook` suite, and `--pipeline-trigger webhook`, are the only paths with
external prerequisites. There are three, they fail in this order, and the
harness names each one during preflight:

1. **The signing secret** must match the platform's configured GitHub App
   webhook secret (below).
2. **The payload's installation** must be registered in the platform. The
   shipped `doc/sample-git-event.json` carries a placeholder
   (`installation.id: 987654`), so a real environment needs
   `--webhook-installation-id <id>`. List valid ids with
   `GET /v1/git-apps/github/installations`.
3. **git-bot must have GitHub App credentials.** Without them it answers 503
   and neither suite can run at all.

### These suites generate real GitHub API traffic

This is the important one. When the stack is backed by a real GitHub App, an
accepted delivery is forwarded and the platform **creates check runs on
github.com** for the repository named in the payload. A load test therefore:

- makes real GitHub API calls at whatever rate the ramp drives,
- will hit GitHub's rate limits well before it finds your backend's limit,
- pollutes a real repository with check runs, and
- measures GitHub's latency rather than the platform's once throttled.

Run these suites only against a stack wired to a disposable test GitHub App and
a throwaway repository. Against a production-linked or personal-account App,
use the request suites instead:

```bash
test/perf/run-perf-test.sh --suites api-read,auth
```

The `api-read` and `auth` suites have no external dependencies and produce the
throughput, latency and per-service resource numbers that describe the backend
itself.

## Webhook secret

git-bot verifies `X-Hub-Signature-256` against the **GitHub App webhook secret
configured in the platform**, which it loads from the credential broker at
startup. This is a common source of confusion, so to be explicit:

- It is **not** read from the compose environment or from `.env`.
- It is **not** a value you can choose for the test.
- Exporting an arbitrary `GITHUB_WEBHOOK_SECRET` produces HTTP 401 on every
  webhook request.

Find the configured secret in either:

- the UI, under the GitHub App settings, or
- the global config repository at `setting/git-apps/github.yaml`, where the
  `webhook_secret` credential reference is declared.

The harness sends one signed delivery during preflight and fails immediately
with this explanation if the signature is rejected, rather than spending the
whole ramp on requests that cannot succeed.

If you do not have the secret, run the read and auth paths on their own:

```bash
test/perf/run-perf-test.sh --suites api-read,auth
```

## How the measurement works

The harness is **closed-loop**. Each stage runs N workers that issue requests
back to back; the reported request rate is what the system completed, not a rate
forced onto it. This is why the throughput column answers "what can it do at
this concurrency" rather than "did it keep up with an arbitrary target".

Each stage runs for `--stage-duration`, of which the leading `--warmup` is
excluded from the measurements so that connection setup and cache warming do not
pollute the numbers. Throughput is derived from the measured window only.

Latency percentiles use **nearest-rank** over the complete sample set, so every
reported number is a latency the system actually produced. Stage-level
percentiles are computed from a combined sample set rather than from the
per-scenario percentiles, because a percentile of a mixed workload cannot be
derived by averaging the percentiles of its parts.

While the load runs, container CPU and memory are sampled every
`--sample-interval` via `docker stats` and attributed to whichever stage was
active at that moment. A container that is not running is skipped rather than
failing the round, and a sampling failure degrades the report instead of
aborting the test.

## Reading the report

### Load ramp

One row per concurrency level with request count, achieved RPS, mean and
p50/p90/p95/p99/max latency, error rate, and a saturation marker. A stage is
marked `SATURATED` when it breached the latency SLO or the error budget.

Request count, RPS and error rate exclude any broken scenarios (see below), so
the ramp describes the workload that actually ran.

### Per-endpoint breakdown

The heaviest stage broken down by endpoint. This is what identifies the specific
query that degrades first, rather than only showing that the system as a whole
got slower.

### Service resource usage

Per-container CPU and memory for each stage. CPU uses Docker's scale where 100%
is one fully saturated core, so a service on a 4-core host can legitimately
reach 400%.

### Service capacity

The comparison table: one row per service, showing how many requests it
completed, its peak throughput and at which concurrency, how far p95 stretched
from the lightest stage to the heaviest, its worst error rate, its container CPU
at peak, and whether it held or broke.

Two summary lines follow, and they answer different questions:

- **Carried most** — the service that completed the most work. Receiving the
  most traffic is not the same as handling it well.
- **Degraded least** — the service whose p95 grew least across the ramp without
  breaching. This is the "better capacity" answer: absorbing concurrency without
  turning it into latency is what headroom actually looks like.
- **Gave out first** — when a service breached, the one that broke at the lowest
  concurrency. Scale that one before any other.

Per-service p95 is the worst of that service's endpoints rather than a blend, so
the table never claims a service was faster than one of its calls actually was.

Postgres has no row because every request reaches it; its cost appears as
`nopsai-db` in the resource table.

### Broken scenarios

A scenario that fails at essentially every request in **every** stage is a
configuration or request-shape defect, not a capacity limit. The harness detects
these, lists them with their dominant failure status and a likely cause, and
**excludes them from the ramp columns and the verdict**.

This matters because the alternative is badly misleading. A single misconfigured
endpoint at 9% of the request mix puts a constant 9% error floor under every
stage; measured against a 1% error budget, that reports a perfectly healthy
system as saturated from the very first concurrency level.

The detection is deliberately strict, requiring near-total failure at every
level. A scenario that only collapses under load is exactly what the ramp exists
to find and is never excused this way.

One caveat, stated plainly: request counts, throughput and error rates exclude
broken scenarios, but the **latency percentiles do not**. A rejected request
typically returns in well under a millisecond, so its presence pulls the
reported percentiles down. When the report lists broken scenarios, treat its
latency numbers as optimistic and re-run once they are fixed.

### Verdict

The section that answers "what are the efficient numbers":

- **Safe operating point** — the highest concurrency that met both the latency
  SLO and the error budget. Levels past the first breach are never recommended,
  even if a later stage happens to pass.
- **Peak throughput** — the highest sustained rate observed, and where.
- **Saturation knee** — where added concurrency stopped producing added
  throughput. Past this point, extra load becomes queue time rather than
  completed work. Detected when throughput gains less than 10% while p95 grows
  more than 50%.
- **First threshold breach** — the first level that violated a threshold, with
  the reason.
- **Busiest service** — the container working hardest at the highest level
  reached, plus whether the constraint looks like CPU, memory, or something
  else (I/O, database, lock contention).
- **Findings** — plain statements with their supporting numbers, including
  latency amplification across the ramp and any error class that occurred.

A wall of HTTP 401/403 responses is explicitly flagged as a credentials problem
rather than being reported as a capacity result.

## Thresholds and gating

```bash
nopsai-perf --latency-slo 500ms --error-budget 0.005 --fail-on-breach
```

`--fail-on-breach` exits non-zero when no concurrency level met the thresholds,
which makes the harness usable as a regression gate. The report is still printed
on failure so the cause is visible.

The ramp stops early once a stage's error rate reaches `--abort-error-rate`
(default 50%), because levels past total collapse cost time and add no
information. Pass `--abort-error-rate 0` to disable that.

## Configuration reference

Run `nopsai-perf --help` for the full flag list. Credentials resolve from
environment variables so secrets stay off the command line:

| Variable | Purpose |
| --- | --- |
| `NOPSAI_PERF_IDENTIFIER` | Login identifier. |
| `NOPSAI_PERF_PASSWORD` | Login password. Takes precedence over the bootstrap variable. |
| `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD` | Fallback password, shared with the compose stack. |
| `GITHUB_WEBHOOK_SECRET` | GitHub App webhook secret, for the webhook and pipeline suites. See below. |
| `NOPSAI_PERF_WEBHOOK_SECRET` | Harness-specific override for the same secret. |
| `NOPSAI_API_URL` | Target API base URL. |

## Interpreting results responsibly

- **Compare like with like.** Absolute numbers depend on host hardware, what
  else is running, and how much history is in the database. Treat a run as a
  baseline for the machine it ran on, and compare later runs against it rather
  than against numbers from another environment.
- **The load generator shares the host.** When the harness runs on the same
  machine as the stack, both compete for CPU. At high concurrency this
  understates the backend's real capacity.
- **Database state matters.** `/v1/runs` and the monitoring aggregates get
  slower as run history grows, so a ramp against a fresh database and one
  against a long-lived database are not comparable.
- **"Never found a limit" means the ramp was too gentle**, not that the system
  is unbounded. The report says so explicitly and suggests a wider ramp.

## Relationship to `test/performance-test.sh`

The older [`test/performance-test.sh`](../test/performance-test.sh) is a
shell-based end-to-end pipeline timing script. The `pipeline` suite covers the
same ground with percentile statistics, per-service resource attribution and
machine-readable output. The older script remains for anyone who wants a
dependency-free shell version of the end-to-end check.

## Testing the harness itself

The harness has its own unit and integration tests, which run as part of the
backend suite:

```bash
scripts/test-backend.sh ./internal/perf/...
go test ./internal/perf/... -race -cover
```

Tests that need Docker skip themselves when no daemon is available.
