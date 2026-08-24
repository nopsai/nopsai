#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

package_patterns=(
  ./contract/...
  ./cmd/...
  ./config/...
  ./internal/...
  ./pkg/...
  ./release/...
)

while IFS= read -r service_dir; do
  package_patterns+=("./${service_dir}/...")
done < <(
  find services \
    -mindepth 1 \
    -maxdepth 1 \
    -type d \
    ! -name ui \
    -print |
    LC_ALL=C sort
)

exec go test "$@" "${package_patterns[@]}"
