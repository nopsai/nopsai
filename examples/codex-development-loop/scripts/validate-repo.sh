#!/usr/bin/env bash
# Run the repository's own build/test commands and record the evidence.
#
# Usage:
#   validate-repo.sh <report-file>
#
# Set DEV_LOOP_VALIDATE_COMMAND to the repository's real validation command
# (recommended). Without it, this script auto-detects a toolchain from the
# well-known marker files below.
#
# This check fails closed. If no validation command is configured and no
# toolchain can be detected, the run fails: "could not evaluate" is never
# treated as "passed".
set -uo pipefail

report_file="${1:-}"
if [[ -z "$report_file" ]]; then
  echo "dev-loop: usage: validate-repo.sh <report-file>" >&2
  exit 1
fi

mkdir -p "$(dirname "$report_file")"
: >"$report_file"

overall_status=0

run_check() {
  local description="$1"
  local command="$2"
  local output
  local status

  echo "### $description" >>"$report_file"
  echo '```' >>"$report_file"
  echo "\$ $command" >>"$report_file"

  output="$(bash -c "$command" 2>&1)"
  status=$?

  # Keep the report bounded so it stays usable as model context and as a log.
  printf '%s\n' "$output" | tail -n 200 >>"$report_file"
  echo '```' >>"$report_file"
  echo "exit_code: $status" >>"$report_file"
  echo >>"$report_file"

  if [[ $status -ne 0 ]]; then
    overall_status=1
  fi
  return $status
}

echo "# Repository validation" >>"$report_file"
echo >>"$report_file"

declare -a checks=()

if [[ -n "${DEV_LOOP_VALIDATE_COMMAND:-}" ]]; then
  checks+=("configured validation command::${DEV_LOOP_VALIDATE_COMMAND}")
elif [[ -f go.mod ]]; then
  checks+=("go build::go build ./...")
  checks+=("go vet::go vet ./...")
  checks+=("go test::go test ./...")
elif [[ -f package.json ]]; then
  checks+=("install dependencies::npm ci --no-audit --no-fund")
  checks+=("build::npm run build --if-present")
  checks+=("test::npm test")
elif [[ -f Cargo.toml ]]; then
  checks+=("cargo build::cargo build --all-targets")
  checks+=("cargo test::cargo test")
elif [[ -f pyproject.toml || -f requirements.txt ]]; then
  checks+=("pytest::python -m pytest")
else
  {
    echo "## Result"
    echo
    echo "VALIDATION_UNAVAILABLE"
    echo
    echo "No DEV_LOOP_VALIDATE_COMMAND was configured and no supported toolchain"
    echo "marker (go.mod, package.json, Cargo.toml, pyproject.toml, requirements.txt)"
    echo "was found in the workspace. The loop cannot confirm the repository still"
    echo "works, so this run is treated as a failure."
  } >>"$report_file"
  echo "dev-loop: no validation command configured and no toolchain detected" >&2
  exit 1
fi

for entry in "${checks[@]}"; do
  description="${entry%%::*}"
  command="${entry#*::}"
  run_check "$description" "$command"
done

{
  echo "## Result"
  echo
  if [[ $overall_status -eq 0 ]]; then
    echo "VALIDATION_PASSED"
  else
    echo "VALIDATION_FAILED"
  fi
} >>"$report_file"

exit $overall_status
