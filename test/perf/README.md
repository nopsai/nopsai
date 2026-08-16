# Backend performance tests

Load-tests the running stack and reports throughput, latency percentiles,
per-service resource usage, and the safe operating numbers.

Full documentation: [`doc/performance-testing.md`](../../doc/performance-testing.md).

## Run

```bash
docker compose up --build -d

export NOPSAI_PERF_IDENTIFIER='admin@example.com'
export NOPSAI_PERF_PASSWORD='<the admin password>'

test/perf/run-perf-test.sh --preset standard
```

Presets: `quick` (~1 min), `standard` (~3 min), `stress` (~5 min),
`full` (30+ min), `full-webhook` (30+ min).

`standard` puts every load-bearing service under the same pressure at once —
nopsai, aaa, the dispatcher, Postgres and the UI — and reports which one carried
the most load, which degraded least, and which gave out first. It needs nothing
beyond credentials.

`full` adds the pipeline suite, which drives real runs through a purpose-built
fixture that must be installed once — see
[`fixtures/README.md`](fixtures/README.md). It is self-contained: no GitHub App,
no signing secret, no third-party traffic.

`stress` and `full-webhook` exercise the git webhook path. That needs the
platform's configured GitHub App webhook secret plus a registered installation
id, and it generates **real github.com API traffic and check runs**. Use it only
against a disposable test App.

Any `nopsai-perf` flag can follow a preset and overrides it:

```bash
test/perf/run-perf-test.sh --preset stress --concurrency 50,100,250,500
test/perf/run-perf-test.sh --suites api-read --latency-slo 500ms --fail-on-breach
go run ./cmd/nopsai-perf --help
```

## Layout

| Path | Purpose |
| --- | --- |
| `run-perf-test.sh` | Prerequisite checks, stack health gate, presets, then hands over to the tool. |
| `results/` | Timestamped `.txt` and `.json` reports (gitignored). |
| `../../internal/perf` | Measurement logic and its tests. |
| `../../cmd/nopsai-perf` | Tool entry point. |

`../performance-test.sh` is the older shell-based end-to-end pipeline timing
script; the `pipeline` suite supersedes it with percentile statistics and
machine-readable output.
