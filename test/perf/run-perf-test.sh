#!/usr/bin/env bash
#
# Orchestrates a backend performance run.
#
# The measurement itself lives in the nopsai-perf tool (cmd/nopsai-perf, backed
# by internal/perf). This script only does the things a shell is better at:
# checking prerequisites, confirming the stack is actually up, translating a
# preset into flags, and handing over.
#
# Usage:
#   test/perf/run-perf-test.sh [--preset quick|standard|stress|full|full-webhook] [flags...]
#
# Examples:
#   test/perf/run-perf-test.sh                       # standard read+auth ramp
#   test/perf/run-perf-test.sh --preset quick        # 2 minute smoke ramp
#   test/perf/run-perf-test.sh --preset stress       # find the ceiling
#   test/perf/run-perf-test.sh --preset full         # everything, incl. pipelines
#   test/perf/run-perf-test.sh --suites api-read --concurrency 1,10,100
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

API_URL="${NOPSAI_API_URL:-http://127.0.0.1:8080}"
PRESET=""
PASSTHROUGH=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --preset)
      [[ $# -ge 2 ]] || { echo "error: --preset needs a value" >&2; exit 2; }
      PRESET="$2"
      shift 2
      ;;
    --preset=*)
      PRESET="${1#*=}"
      shift
      ;;
    --api-url)
      [[ $# -ge 2 ]] || { echo "error: --api-url needs a value" >&2; exit 2; }
      API_URL="$2"
      PASSTHROUGH+=("$1" "$2")
      shift 2
      ;;
    --api-url=*)
      API_URL="${1#*=}"
      PASSTHROUGH+=("$1")
      shift
      ;;
    -h|--help)
      exec go run ./cmd/nopsai-perf --help
      ;;
    *)
      PASSTHROUGH+=("$1")
      shift
      ;;
  esac
done

# Presets are ordinary flag sets, placed before the passthrough arguments so an
# explicit flag always overrides the preset.
PRESET_ARGS=()
case "$PRESET" in
  "")
    ;;
  quick)
    # A smoke ramp: enough to catch a gross regression, short enough to run on
    # every branch.
    PRESET_ARGS=(
      --suites api-read
      --concurrency 1,5,20
      --stage-duration 20s
      --warmup 3s
    )
    ;;
  standard)
    # Every load-bearing service at once: the API and its queries, aaa on the
    # auth path, the dispatcher through its status surface, the UI container,
    # and the telemetry a running pipeline emits. This is the preset that
    # answers which service carries load best.
    PRESET_ARGS=(
      --suites api-read,auth,runtime,ui
      --concurrency 1,2,5,10,25,50
      --stage-duration 30s
      --warmup 5s
    )
    ;;
  stress)
    # Pushes the same service mix until something breaks, so the ceiling is a
    # measured number rather than an estimate.
    PRESET_ARGS=(
      --suites api-read,auth,runtime,ui
      --concurrency 10,25,50,100,200,400
      --stage-duration 45s
      --warmup 10s
    )
    ;;
  full)
    # Everything except the git webhook path, which needs a real GitHub App and
    # generates third-party API traffic. Pipeline runs go through the
    # purpose-built external trigger, so this is self-contained. Expect runner
    # containers to be created.
    PRESET_ARGS=(
      --suites api-read,auth,runtime,ui,pipeline
      --concurrency 1,5,10,25,50
      --stage-duration 30s
      --warmup 5s
      --pipeline-concurrency 1,3,5
      --pipeline-timeout 15m
    )
    ;;
  full-webhook)
    # As above, plus the git webhook ingestion path. Needs the platform's
    # GitHub App webhook secret, a registered installation, and a payload for a
    # commit that exists. Generates real github.com traffic.
    PRESET_ARGS=(
      --suites api-read,auth,runtime,ui,webhook,pipeline
      --concurrency 1,5,10,25,50
      --stage-duration 30s
      --warmup 5s
      --pipeline-concurrency 1,3,5
      --pipeline-timeout 15m
    )
    ;;
  *)
    echo "error: unknown preset '$PRESET' (want quick, standard, stress, full or full-webhook)" >&2
    exit 2
    ;;
esac

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: '$1' is required but not installed" >&2
    exit 1
  }
}

require_command go
require_command curl

# A load test against a stack that is not up produces a page of connection
# refusals that looks like a catastrophic performance result. Fail early with a
# message that says what to do instead.
if ! curl --silent --fail --max-time 5 "$API_URL/healthz" >/dev/null 2>&1; then
  cat >&2 <<EOF
error: $API_URL/healthz is not responding.

Start the stack first:
  docker compose up --build -d

Then wait for it to become healthy:
  docker compose ps
EOF
  exit 1
fi

# Resource sampling needs the Docker CLI. Without it the run still produces
# latency and throughput numbers, just no per-service attribution.
SAMPLING_ARGS=()
if ! command -v docker >/dev/null 2>&1; then
  echo "warning: docker is not available; per-service CPU and memory will not be sampled" >&2
  SAMPLING_ARGS=(--no-resources)
fi

# Only the git webhook path needs a signing secret. The pipeline suite drives
# runs through the purpose-built external trigger by default, which needs none,
# so this guard must not fire merely because "pipeline" appears in the arguments.
JOINED_ARGS="${PRESET_ARGS[*]-} ${PASSTHROUGH[*]-}"
NEEDS_WEBHOOK_SECRET=0
case "$JOINED_ARGS" in
  *--suites*webhook*) NEEDS_WEBHOOK_SECRET=1 ;;
esac
case "$JOINED_ARGS" in
  *"--pipeline-trigger webhook"*|*--pipeline-trigger=webhook*) NEEDS_WEBHOOK_SECRET=1 ;;
esac

if [[ "$NEEDS_WEBHOOK_SECRET" -eq 1 && -z "${GITHUB_WEBHOOK_SECRET:-}${NOPSAI_PERF_WEBHOOK_SECRET:-}" ]]; then
  cat >&2 <<'EOF'
error: the git webhook path needs the HMAC secret that git-bot verifies.

This must be the GitHub App webhook secret configured in the platform. It is not
a free-form value, and it is not read from the compose environment. Find it in:
  - the UI, under the GitHub App settings, or
  - the global config repository, at setting/git-apps/github.yaml, where the
    webhook_secret credential reference is declared.

Then export it:
  export GITHUB_WEBHOOK_SECRET='<that secret>'

The pipeline suite does not need this: it drives runs through the external
trigger fixture in test/perf/fixtures. Use --preset full instead.
EOF
  exit 1
fi

echo "==> target:  $API_URL"
echo "==> preset:  ${PRESET:-none}"
echo "==> started: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo

exec go run ./cmd/nopsai-perf \
  ${SAMPLING_ARGS[@]+"${SAMPLING_ARGS[@]}"} \
  ${PRESET_ARGS[@]+"${PRESET_ARGS[@]}"} \
  ${PASSTHROUGH[@]+"${PASSTHROUGH[@]}"}
