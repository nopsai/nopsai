#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

commit_count="$(git -C "$ROOT_DIR" rev-list --count HEAD)"
expected="2.7.$((commit_count + 2))"
actual="$("$ROOT_DIR/scripts/release-version.sh" --offset 2)"
if [[ "$actual" != "$expected" ]]; then
  printf 'version = %s, want %s\n' "$actual" "$expected" >&2
  exit 1
fi
if "$ROOT_DIR/scripts/release-version.sh" --offset invalid >/dev/null 2>&1; then
  printf 'invalid version offset succeeded\n' >&2
  exit 1
fi

"$ROOT_DIR/scripts/generate-changelog.sh" "$actual" "$temp_dir/CHANGELOG.md"
grep -F "# NopsAI $actual" "$temp_dir/CHANGELOG.md" >/dev/null

"$ROOT_DIR/scripts/render-release-bundle.sh" \
  --output "$temp_dir/bundle" \
  --registry ghcr.io/hosein-yousefii \
  --version "$actual" \
  --commit "$(git -C "$ROOT_DIR" rev-parse HEAD)" \
  --build-date 2026-06-22T00:00:00Z

grep -F "NOPSAI_VERSION=$actual" "$temp_dir/bundle/.env" >/dev/null
grep -F "ghcr.io/hosein-yousefii/nopsai-api:$actual" "$temp_dir/bundle/.env" >/dev/null
chart_file="$temp_dir/bundle/nopsai-$actual.tgz"
test -s "$chart_file"
test ! -e "$temp_dir/bundle/kubernetes-values.yaml"
helm show chart "$chart_file" | grep -F "version: $actual" >/dev/null
helm show chart "$chart_file" | grep -F "appVersion: $actual" >/dev/null
helm show values "$chart_file" | grep -F "releaseVersion: $actual" >/dev/null
helm show values "$chart_file" | grep -F "repository: ghcr.io/hosein-yousefii/nopsai-api" >/dev/null
test -s "$temp_dir/bundle/db/init.sql"
(
  cd "$temp_dir/bundle"
  export SERVICE_JWT_SIGNING_KEY=test-service-signing-key-at-least-32-characters
  export POSTGRES_PASSWORD=test-postgres-password
  export DATABASE_URL=postgres://nopsai_user:test-postgres-password@db:5432/nopsai_db
  export AAA_SHARED_INTERNAL_TOKEN=test-aaa-shared-token-at-least-32-characters
  export NOPSAI_MASTER_KEY=dGVzdC1tYXN0ZXIta2V5LTMyaXRlcy1sb25nISE=
  export JWT_SIGNING_KEY=test-jwt-signing-key-at-least-32-characters
  docker compose config --quiet
  shasum -a 256 -c checksums.txt >/dev/null
)

image_names=(
  nopsai-base
  nopsai-api
  nopsai-aaa
  nopsai-agent
  nopsai-dispatcher
  nopsai-git-bot
  nopsai-runner
  nopsai-k8s-runner
  nopsai-docker-socket-proxy
  nopsai-ui
  pipeline-image
)
mkdir -p "$temp_dir/digests"
for image_name in "${image_names[@]}"; do
  printf 'sha256:%064d\n' 0 >"$temp_dir/digests/$image_name.digest"
done

"$ROOT_DIR/scripts/render-release-bundle.sh" \
  --output "$temp_dir/digest-bundle" \
  --registry ghcr.io/hosein-yousefii \
  --version "$actual" \
  --commit "$(git -C "$ROOT_DIR" rev-parse HEAD)" \
  --build-date 2026-06-22T00:00:00Z \
  --digest-dir "$temp_dir/digests"

for image_name in "${image_names[@]}"; do
  grep -F "ghcr.io/hosein-yousefii/$image_name@sha256:" \
    "$temp_dir/digest-bundle/.env" >/dev/null
done
if grep -F '{{' \
  "$temp_dir/digest-bundle/.env" \
  "$temp_dir/digest-bundle/docker-compose.yaml" \
  "$temp_dir/digest-bundle/release-index.json" >/dev/null; then
  printf 'digest release bundle contains an unresolved placeholder\n' >&2
  exit 1
fi
if jq -e '.images | length == 11 and all(.[]; contains("@sha256:"))' \
  "$temp_dir/digest-bundle/release-index.json" >/dev/null; then
  :
else
  printf 'release index does not contain all digest-pinned images\n' >&2
  exit 1
fi
digest_chart="$temp_dir/digest-bundle/nopsai-$actual.tgz"
if ! jq -e --arg file "nopsai-$actual.tgz" \
  '.chart.file == $file and (.chart.sha256 | test("^sha256:[a-f0-9]{64}$"))' \
  "$temp_dir/digest-bundle/release-index.json" >/dev/null; then
  printf 'release index does not contain the packaged Helm chart\n' >&2
  exit 1
fi
helm show values "$digest_chart" >"$temp_dir/digest-chart-values.yaml"
for image_name in "${image_names[@]}"; do
  grep -F "repository: ghcr.io/hosein-yousefii/$image_name" \
    "$temp_dir/digest-chart-values.yaml" >/dev/null
done
if [[ "$(grep -c 'digest: sha256:' "$temp_dir/digest-chart-values.yaml")" -ne 11 ]]; then
  printf 'packaged Helm chart does not contain all image digests\n' >&2
  exit 1
fi
helm template nopsai "$digest_chart" --namespace nopsai >"$temp_dir/digest-chart-manifests.yaml"
grep -F 'image: "ghcr.io/hosein-yousefii/nopsai-api@sha256:' \
  "$temp_dir/digest-chart-manifests.yaml" >/dev/null
grep -F 'image: "ghcr.io/hosein-yousefii/nopsai-agent@sha256:' \
  "$temp_dir/digest-chart-manifests.yaml" >/dev/null
(
  cd "$temp_dir/digest-bundle"
  export SERVICE_JWT_SIGNING_KEY=test-service-signing-key-at-least-32-characters
  export POSTGRES_PASSWORD=test-postgres-password
  export DATABASE_URL=postgres://nopsai_user:test-postgres-password@db:5432/nopsai_db
  export AAA_SHARED_INTERNAL_TOKEN=test-aaa-shared-token-at-least-32-characters
  export NOPSAI_MASTER_KEY=dGVzdC1tYXN0ZXIta2V5LTMyaXRlcy1sb25nISE=
  export JWT_SIGNING_KEY=test-jwt-signing-key-at-least-32-characters
  docker compose config --quiet
  shasum -a 256 -c checksums.txt >/dev/null
)
