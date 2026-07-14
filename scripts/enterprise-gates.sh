#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

run() {
  printf '\n==> %s\n' "$*"
  "$@"
}

require_tool() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf 'missing required tool: %s\n' "$tool" >&2
    return 1
  fi
}

require_golangci_lint_compatible() {
  require_tool golangci-lint

  local target_go lint_version lint_go target_major target_minor lint_major lint_minor
  target_go="$(awk '$1 == "go" { print $2; exit }' go.mod)"
  lint_version="$(golangci-lint version 2>/dev/null || true)"
  lint_go="$(printf '%s\n' "$lint_version" | sed -nE 's/.*built with go([0-9]+)\.([0-9]+).*/\1.\2/p')"

  if [[ -z "$target_go" || -z "$lint_go" ]]; then
    return 0
  fi

  target_major="${target_go%%.*}"
  target_minor="${target_go#*.}"
  target_minor="${target_minor%%.*}"
  lint_major="${lint_go%%.*}"
  lint_minor="${lint_go#*.}"

  if (( lint_major < target_major || (lint_major == target_major && lint_minor < target_minor) )); then
    printf 'golangci-lint is too old for this repository.\n' >&2
    printf '  target Go version: %s\n' "$target_go" >&2
    printf '  %s\n' "$lint_version" >&2
    printf 'Install a compatible linter, for example:\n' >&2
    printf '  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2\n' >&2
    printf 'or upgrade via your package manager, then ensure the new binary is first on PATH.\n' >&2
    return 1
  fi
}

run scripts/test-backend.sh
run scripts/test-backend.sh -race
run scripts/release-tooling-test.sh
run scripts/license-check.sh
run go vet ./...

require_golangci_lint_compatible
run golangci-lint run ./...

require_tool gosec
run gosec -exclude=G101,G103,G104,G110,G115,G118,G122,G301,G304,G306,G703,G704 ./...

require_tool govulncheck
run govulncheck ./...

if [[ "${SKIP_DOCKER_BUILDS:-}" == "1" ]]; then
  printf '\n==> skipping Docker build checks because SKIP_DOCKER_BUILDS=1\n'
  exit 0
fi

require_tool docker

run docker build -t nopsai-base:ci -f Dockerfile .
run docker build -t nopsai-docker-socket-proxy:ci -f container/Dockerfile.socket-proxy .
run docker build -t nopsai-pipeline:ci -f container/Dockerfile.pipeline .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-agent:ci -f container/Dockerfile.agent .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-aaa:ci -f container/Dockerfile.aaa .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-api:ci -f container/Dockerfile.nopsai .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-dispatcher:ci -f container/Dockerfile.dispatcher .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-git-bot:ci -f container/Dockerfile.git-bot .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-runner:ci -f container/Dockerfile.docker-runner .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-k8s-runner:ci -f container/Dockerfile.k8s-runner .
run docker build -t nopsai-ui:ci -f services/ui/Dockerfile services/ui
