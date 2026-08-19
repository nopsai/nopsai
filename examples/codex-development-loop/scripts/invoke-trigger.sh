#!/usr/bin/env bash
# Start the other half of the loop through the NopsAI external trigger API.
#
# Usage:
#   invoke-trigger.sh <trigger-id> <event-type> <idempotency-key> KEY=VALUE ...
#
# The KEY=VALUE pairs become the invocation payload. The trigger manifest maps
# payload fields onto the target pipeline's variables, so the set of variables a
# pipeline will accept from a caller stays declared in Git rather than decided
# at the call site.
#
# Requires DEV_LOOP_API_URL and the DEV_LOOP_NOPSAI_TOKEN secret, which should
# be a service-account token that is allowed to invoke this trigger and to run
# the pipeline behind it.
set -euo pipefail

die() {
  echo "dev-loop: $*" >&2
  exit 1
}

trigger_id="${1:-}"
event_type="${2:-}"
idempotency_key="${3:-}"
shift 3 || true

[[ -n "$trigger_id" ]] || die "usage: invoke-trigger.sh <trigger-id> <event-type> <idempotency-key> KEY=VALUE ..."
[[ -n "$event_type" ]] || die "an event type is required"
[[ -n "$idempotency_key" ]] || die "an idempotency key is required so a retried step cannot start a second run"

[[ -n "${DEV_LOOP_API_URL:-}" ]] || die "DEV_LOOP_API_URL is required"
[[ -n "${DEV_LOOP_NOPSAI_TOKEN:-}" ]] || die "the DEV_LOOP_NOPSAI_TOKEN secret is required"

command -v curl >/dev/null 2>&1 || die "curl is not installed in this image"
command -v jq >/dev/null 2>&1 || die "jq is not installed in this image"

# jq builds the payload so task titles and branch names are escaped correctly
# instead of being pasted into a hand-written JSON string.
payload='{}'
for pair in "$@"; do
  [[ "$pair" == *=* ]] || die "payload entry '$pair' must be in KEY=VALUE form"
  key="${pair%%=*}"
  value="${pair#*=}"
  payload="$(jq -n --argjson current "$payload" --arg k "$key" --arg v "$value" \
    '$current + {($k): $v}')"
done

body="$(jq -n \
  --arg event_type "$event_type" \
  --arg idempotency_key "$idempotency_key" \
  --argjson payload "$payload" \
  '{event_type: $event_type, idempotency_key: $idempotency_key, payload: $payload}')"

api_url="${DEV_LOOP_API_URL%/}"
response_file="$(mktemp)"
trap 'rm -f "$response_file"' EXIT

http_status="$(curl --silent --show-error --fail-with-body \
  --output "$response_file" --write-out '%{http_code}' \
  --request POST \
  --header "Authorization: Bearer ${DEV_LOOP_NOPSAI_TOKEN}" \
  --header 'Content-Type: application/json' \
  --data "$body" \
  "${api_url}/v1/external-triggers/${trigger_id}/invoke")" || {
  echo "dev-loop: trigger '$trigger_id' returned an error:" >&2
  cat "$response_file" >&2
  exit 1
}

case "$http_status" in
  2*) ;;
  *)
    echo "dev-loop: trigger '$trigger_id' returned HTTP $http_status:" >&2
    cat "$response_file" >&2
    exit 1
    ;;
esac

run_id="$(jq -r '.run_id // empty' <"$response_file")"
echo "TRIGGERED $trigger_id run_id=${run_id:-unknown}"
