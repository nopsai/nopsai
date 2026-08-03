#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

require_text() {
  local pattern="$1"
  local file="$2"
  local description="$3"
  if ! grep -F "$pattern" "$file" >/dev/null; then
    printf '%s is missing %s\n' "$file" "$description" >&2
    return 1
  fi
}

commit_count="$(git -C "$ROOT_DIR" rev-list --count HEAD)"
base_version="$(tr -d '[:space:]' <"$ROOT_DIR/release/version.txt")"
expected="$base_version.$((commit_count + 2))"
actual="$("$ROOT_DIR/scripts/release-version.sh" --offset 2)"
if [[ "$actual" != "$expected" ]]; then
  printf 'version = %s, want %s\n' "$actual" "$expected" >&2
  exit 1
fi
release_env="$("$ROOT_DIR/scripts/release-version.sh" --offset 2 --format env)"
require_text "MAJOR_TAG=${base_version%%.*}" <(printf '%s\n' "$release_env") "the major release tag"
require_text "MAJOR_MINOR_TAG=$base_version" <(printf '%s\n' "$release_env") "the major.minor release tag"
if "$ROOT_DIR/scripts/release-version.sh" --offset invalid >/dev/null 2>&1; then
  printf 'invalid version offset succeeded\n' >&2
  exit 1
fi
release_tags=()
while IFS= read -r release_tag; do
  release_tags+=("$release_tag")
done < <("$ROOT_DIR/scripts/release-tags.sh" "$actual")
expected_release_tags=("$actual" "latest" "${base_version%%.*}" "$base_version")
if [[ "${release_tags[*]}" != "${expected_release_tags[*]}" ]]; then
  printf 'release tags = %s, want %s\n' "${release_tags[*]}" "${expected_release_tags[*]}" >&2
  exit 1
fi
if "$ROOT_DIR/scripts/release-tags.sh" "$actual+build" >/dev/null 2>&1; then
  printf 'release tags accepted build metadata\n' >&2
  exit 1
fi

"$ROOT_DIR/scripts/sign-notarize-macos-cli.sh" --help >"$temp_dir/macos-signing-help.txt"
require_text \
  "APPLE_DEVELOPER_ID_P12_BASE64" \
  "$temp_dir/macos-signing-help.txt" \
  "the Developer ID credential contract"
if "$ROOT_DIR/scripts/sign-notarize-macos-cli.sh" --binary >/dev/null 2>&1; then
  printf 'macOS signing accepted an incomplete binary argument\n' >&2
  exit 1
fi

"$ROOT_DIR/scripts/generate-changelog.sh" "$actual" "$temp_dir/CHANGELOG.md"
require_text "# NopsAI $actual" "$temp_dir/CHANGELOG.md" "the generated release heading"

mkdir -p "$temp_dir/chart"
helm package "$ROOT_DIR/deploy/helm/nopsai" \
  --version "$actual" \
  --app-version "$actual" \
  --destination "$temp_dir/chart"
chart_file="$temp_dir/chart/nopsai-$actual.tgz"
helm show chart "$chart_file" >"$temp_dir/chart-metadata.yaml"
require_text "version: $actual" "$temp_dir/chart-metadata.yaml" "the chart version"
require_text "appVersion: $actual" "$temp_dir/chart-metadata.yaml" "the chart application version"
require_text "nopsai.com/license: LicenseRef-NopsAI-Proprietary" "$temp_dir/chart-metadata.yaml" "the proprietary licence annotation"
tar -tzf "$chart_file" >"$temp_dir/chart-contents.txt"
require_text "nopsai/LICENSE" "$temp_dir/chart-contents.txt" "the packaged proprietary notice"
require_text "nopsai/THIRD_PARTY_NOTICES.md" "$temp_dir/chart-contents.txt" "the packaged third-party notice index"
helm show values "$chart_file" >"$temp_dir/chart-values.yaml"
require_text "repository: ghcr.io/nopsai/nopsai-api" "$temp_dir/chart-values.yaml" "the API image repository"

image_names=(
  nopsai-api
  nopsai-aaa
  nopsai-agent
  nopsai-dispatcher
  nopsai-git-bot
  nopsai-docker-runner
  nopsai-k8s-runner
  nopsai-ui
)
for image_name in "${image_names[@]}"; do
  require_text \
    "repository: ghcr.io/nopsai/$image_name" \
    "$temp_dir/chart-values.yaml" \
    "the $image_name repository"
done
require_text "repository: postgres" "$temp_dir/chart-values.yaml" "the PostgreSQL image repository"

container_dockerfiles=(
  Dockerfile
  container/Dockerfile.aaa
  container/Dockerfile.agent
  container/Dockerfile.dispatcher
  container/Dockerfile.docker-runner
  container/Dockerfile.git-bot
  container/Dockerfile.k8s-runner
  container/Dockerfile.nopsai
  container/Dockerfile.pipeline
  container/Dockerfile.socket-proxy
  services/ui/Dockerfile
)
for dockerfile in "${container_dockerfiles[@]}"; do
  require_text \
    "org.opencontainers.image.licenses=\"LicenseRef-NopsAI-Proprietary\"" \
    "$ROOT_DIR/$dockerfile" \
    "the proprietary OCI licence label"
  require_text \
    "/usr/share/licenses/nopsai" \
    "$ROOT_DIR/$dockerfile" \
    "the packaged licence directory"
done

(cd "$ROOT_DIR" && go run ./cmd/nopsai-cli license >"$temp_dir/cli-license.txt")
require_text "Hossein Yousefi" "$temp_dir/cli-license.txt" "the copyright owner"
require_text "written agreement" "$temp_dir/cli-license.txt" "the commercial licence requirement"

(cd "$ROOT_DIR" && go run ./cmd/nopsai-cli install docker-compose --version "$actual" --output-dir "$temp_dir/compose-install" --force >/dev/null)
require_text "NOPSAI_VERSION=$actual" "$temp_dir/compose-install/.env" "the generated Compose release version"
require_text "ghcr.io/nopsai/nopsai-api:$actual" "$temp_dir/compose-install/.env" "the generated Compose API image"
test -s "$temp_dir/compose-install/docker-compose.yaml"
test -s "$temp_dir/compose-install/db/init.sql"
test ! -e "$temp_dir/compose-install/release-manifest.json"

(cd "$ROOT_DIR" && go run ./cmd/nopsai-cli install kubernetes --version "$actual" --output-dir "$temp_dir/kubernetes-install" --force >/dev/null)
require_text "releaseVersion: \"$actual\"" "$temp_dir/kubernetes-install/values.yaml" "the generated values release version"
require_text "tag: \"$actual\"" "$temp_dir/kubernetes-install/values.yaml" "the generated values image tag"
require_text "postgres:" "$temp_dir/kubernetes-install/values.yaml" "the generated PostgreSQL values"
require_text "kind: Secret" "$temp_dir/kubernetes-install/nopsai-secrets.yaml" "the generated Kubernetes Secret manifest"
require_text "NopsAI Kubernetes Installation" "$temp_dir/kubernetes-install/installation.md" "the generated Kubernetes installation guide"
require_text "oci://ghcr.io/nopsai/charts/nopsai" "$temp_dir/kubernetes-install/.nopsai/install.lock" "the generated chart reference"
test ! -e "$temp_dir/kubernetes-install/release-manifest.json"

helm template nopsai "$chart_file" --namespace nopsai -f "$temp_dir/kubernetes-install/values.yaml" >"$temp_dir/chart-manifests.yaml"
require_text \
  "image: \"ghcr.io/nopsai/nopsai-api:$actual\"" \
  "$temp_dir/chart-manifests.yaml" \
  "the versioned API workload image"
require_text \
  "name: AGENT_IMAGE, value: \"ghcr.io/nopsai/nopsai-agent:$actual\"" \
  "$temp_dir/chart-manifests.yaml" \
  "the versioned dynamic agent image"
require_text \
  "kind: StatefulSet" \
  "$temp_dir/chart-manifests.yaml" \
  "the bundled PostgreSQL StatefulSet"
