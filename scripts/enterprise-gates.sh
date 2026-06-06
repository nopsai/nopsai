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

run go test ./...
run go test -race ./...
run go vet ./...

require_tool golangci-lint
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
run docker build -t nopsai-agent:ci -f container/Dockerfile.agent .
run docker build -t nopsai-aaa:ci -f container/Dockerfile.aaa .
run docker build -t nopsai-pipeline:ci -f container/Dockerfile.pipeline .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-api:ci -f container/Dockerfile.nopsai .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-dispatcher:ci -f container/Dockerfile.dispatcher .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-git-bot:ci -f container/Dockerfile.git-bot .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-runner:ci -f container/Dockerfile.runner .
run docker build --build-arg BASE_IMAGE=nopsai-base:ci -t nopsai-k8s-runner:ci -f container/Dockerfile.k8s-runner .
run docker build -t nopsai-ui:ci -f services/ui/Dockerfile services/ui
