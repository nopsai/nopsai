#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT

require_text() {
  local pattern="$1"
  local file="$2"
  local description="$3"
  if ! grep -F -e "$pattern" "$file" >/dev/null; then
    printf '%s is missing %s\n' "$file" "$description" >&2
    return 1
  fi
}

expected="$(tr -d '[:space:]' <"$ROOT_DIR/version.txt")"
base_version="${expected%.*}"
actual="$("$ROOT_DIR/scripts/release-version.sh")"
if [[ "$actual" != "$expected" ]]; then
  printf 'version = %s, want %s\n' "$actual" "$expected" >&2
  exit 1
fi
release_env="$("$ROOT_DIR/scripts/release-version.sh" --format env)"
require_text "VERSION=$expected" <(printf '%s\n' "$release_env") "the exact release version"
require_text "MAJOR_TAG=${expected%%.*}" <(printf '%s\n' "$release_env") "the major release tag"
require_text "MAJOR_MINOR_TAG=$base_version" <(printf '%s\n' "$release_env") "the major.minor release tag"
expected_range=">=$base_version.0,<$(( ${expected%%.*} + 1 )).0.0"
actual_range="$("$ROOT_DIR/scripts/release-version.sh" --format compatibility-range)"
if [[ "$actual_range" != "$expected_range" ]]; then
  printf 'compatibility range = %s, want %s\n' "$actual_range" "$expected_range" >&2
  exit 1
fi
# The version is read, never computed, so nothing about history may leak into it.
if printf '%s\n' "$release_env" | grep -qE 'COMMIT_COUNT|VERSION_COMMIT_OFFSET'; then
  printf 'release version output still carries a commit-derived patch number\n' >&2
  exit 1
fi
if "$ROOT_DIR/scripts/release-version.sh" --offset 2 >/dev/null 2>&1; then
  printf 'the removed --offset flag was accepted\n' >&2
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
require_text "nopsai.com/license: PolyForm-Noncommercial-1.0.0" "$temp_dir/chart-metadata.yaml" "the licence annotation"
tar -tzf "$chart_file" >"$temp_dir/chart-contents.txt"
require_text "nopsai/LICENSE" "$temp_dir/chart-contents.txt" "the packaged licence notice"
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

# The packaged chart version is the only version an operator must change: every
# NopsAI image tag has to follow the chart appVersion without extra values edits.
helm template nopsai "$chart_file" \
  --namespace nopsai \
  --set secrets.existingSecret=nopsai-secrets \
  >"$temp_dir/chart-rendered.yaml"
for image_name in "${image_names[@]}"; do
  case "$image_name" in
    nopsai-docker-runner) continue ;;
  esac
  require_text \
    "ghcr.io/nopsai/$image_name:$actual" \
    "$temp_dir/chart-rendered.yaml" \
    "the $image_name tag inherited from the chart appVersion"
done
if grep -F "ghcr.io/nopsai/nopsai-api:dev" "$temp_dir/chart-rendered.yaml" >/dev/null; then
  printf 'packaged chart rendered development image tags\n' >&2
  exit 1
fi

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
  container/Dockerfile.release-core
  container/Dockerfile.release-docker
  container/Dockerfile.release-go
  container/Dockerfile.release-node
  container/Dockerfile.socket-proxy
  services/ui/Dockerfile
)
for dockerfile in "${container_dockerfiles[@]}"; do
  require_text \
    "org.opencontainers.image.licenses=\"PolyForm-Noncommercial-1.0.0\"" \
    "$ROOT_DIR/$dockerfile" \
    "the OCI licence label"
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
require_text "tag: \"\"" "$temp_dir/kubernetes-install/values.yaml" "the generated defaulted NopsAI image tag"
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

# GitHub release publication has to be idempotent: the assets are already built
# by the time it runs, so a release that already exists must be updated rather
# than turned into a hard failure, and a lookup that cannot answer must not be
# read as "no release exists".
publish_helpers="$temp_dir/publish-release-helpers.sh"
awk '
  /^      delete_release_assets\(\) \{$/ { capture = 1 }
  /^      \. dist\/release\/env$/ { capture = 0 }
  capture { sub(/^      /, ""); print }
' "$ROOT_DIR/release/nopsai-platform-release.yaml" >"$publish_helpers"
require_text "release_exists()" "$publish_helpers" "the extracted release lookup helper"
require_text "create_release()" "$publish_helpers" "the extracted release create helper"
require_text "release_target_matches_source()" "$publish_helpers" "the extracted same-source release guard"
require_text "run_gh_with_retry()" "$publish_helpers" "the extracted GitHub CLI retry helper"
require_text "load_release_metadata()" "$publish_helpers" "the extracted release metadata lookup"

fake_gh_dir="$temp_dir/fake-bin"
mkdir -p "$fake_gh_dir" "$temp_dir/release-workdir/dist/assets" "$temp_dir/release-workdir/dist/release"
printf 'asset\n' >"$temp_dir/release-workdir/dist/assets/nopsai.tgz"
printf 'notes\n' >"$temp_dir/release-workdir/dist/release/CHANGELOG.md"
cat >"$fake_gh_dir/gh" <<'FAKE_GH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$GH_CALL_LOG"

count_attempt() {
  local attempt_file="$1"
  local attempts=0
  if [[ -f "$attempt_file" ]]; then
    attempts="$(cat "$attempt_file")"
  fi
  attempts=$((attempts + 1))
  printf '%s\n' "$attempts" >"$attempt_file"
  printf '%s\n' "$attempts"
}

case "${1:-} ${2:-}" in
  "release view")
    # Listing the assets on a release is a separate question from looking the
    # release up, so it answers without consuming a lookup attempt.
    if [[ "$*" == *"--json assets"* ]]; then
      printf '%s' "${FAKE_GH_ASSETS:-}"
      exit 0
    fi
    view_attempts="$(count_attempt "$GH_VIEW_ATTEMPT_FILE")"
    if (( view_attempts <= ${FAKE_GH_VIEW_FAILS:-0} )); then
      echo 'non-200 OK status code: 503 Service Unavailable body: {"message":"No server is currently available to service your request."}' >&2
      exit 1
    fi
    case "$FAKE_GH_VIEW" in
      found)
        printf '%s\t%s\n' "${FAKE_GH_TARGET:-abc123}" "${FAKE_GH_DRAFT:-false}"
        exit 0
        ;;
      missing) echo "release not found" >&2; exit 1 ;;
      *) echo "HTTP 502: Bad gateway" >&2; exit 1 ;;
    esac
    ;;
  "release create")
    if [[ "$FAKE_GH_CREATE" == "conflict" ]]; then
      echo "HTTP 422: Validation Failed: a release with the same tag name already exists: $3" >&2
      exit 1
    fi
    exit 0
    ;;
  "release upload")
    upload_attempts="$(count_attempt "$GH_UPLOAD_ATTEMPT_FILE")"
    if (( upload_attempts <= ${FAKE_GH_UPLOAD_FAILS:-0} )); then
      echo 'non-200 OK status code: 503 Service Unavailable body: {"message":"No server is currently available to service your request."}' >&2
      exit 1
    fi
    exit 0
    ;;
  *) exit 0 ;;
esac
FAKE_GH
chmod +x "$fake_gh_dir/gh"

publish_stderr="$temp_dir/publish-stderr.log"

run_publish_case() {
  local view="$1" create="$2" allow_existing="$3" expected_status="$4"
  local target_commitish="${5:-abc123}"
  local upload_fails="${6:-0}"
  local view_fails="${7:-0}"
  local is_draft="${8:-false}"
  local status=0
  rm -f "$temp_dir/gh-upload-attempts" "$temp_dir/gh-view-attempts"
  (
    cd "$temp_dir/release-workdir"
    export PATH="$fake_gh_dir:$PATH"
    export GH_CALL_LOG="$temp_dir/gh-calls.log"
    export GH_UPLOAD_ATTEMPT_FILE="$temp_dir/gh-upload-attempts"
    export GH_VIEW_ATTEMPT_FILE="$temp_dir/gh-view-attempts"
    export FAKE_GH_VIEW="$view" FAKE_GH_CREATE="$create"
    export FAKE_GH_UPLOAD_FAILS="$upload_fails"
    export FAKE_GH_VIEW_FAILS="$view_fails"
    export FAKE_GH_TARGET="$target_commitish"
    export FAKE_GH_DRAFT="$is_draft"
    export GITHUB_REPOSITORY="nopsai/nopsai" VERSION="0.22.813" SOURCE_COMMIT="abc123"
    export ALLOW_EXISTING_RELEASE="$allow_existing"
    export NOPSAI_RELEASE_GITHUB_RETRY_DELAYS="0 0 0"
    set -euo pipefail
    # shellcheck disable=SC1090
    . "$publish_helpers"
    if release_exists "v$VERSION"; then
      require_existing_release_recovery "v$VERSION"
      update_release "v$VERSION" --title "NopsAI $VERSION" --latest
    else
      create_release "v$VERSION" true --title "NopsAI $VERSION" --latest
    fi
  ) >/dev/null 2>"$publish_stderr" || status=$?
  if [[ "$status" != "$expected_status" ]]; then
    printf 'publish case view=%s create=%s allow_existing=%s exited %s, want %s\n' \
      "$view" "$create" "$allow_existing" "$status" "$expected_status" >&2
    cat "$publish_stderr" >&2
    exit 1
  fi
}

: >"$temp_dir/gh-calls.log"
run_publish_case missing ok false 0
require_text "release create" "$temp_dir/gh-calls.log" "a create for a release that does not exist"

: >"$temp_dir/gh-calls.log"
run_publish_case found ok false 0
require_text "release upload" "$temp_dir/gh-calls.log" "an asset upload when the release already exists for the same source"
if grep -q "release create" "$temp_dir/gh-calls.log"; then
  printf 'existing release was re-created instead of updated\n' >&2
  exit 1
fi

: >"$temp_dir/gh-calls.log"
run_publish_case found ok false 0 abc123 1
upload_call_count="$(grep -c '^release upload' "$temp_dir/gh-calls.log")"
if [[ "$upload_call_count" != "2" ]]; then
  printf 'transient upload failure attempted %s uploads, want 2\n' "$upload_call_count" >&2
  exit 1
fi

# A lookup is retried like any other GitHub call: a 503 on the way to the
# answer is not the answer, and reruns used to fail on one.
: >"$temp_dir/gh-calls.log"
run_publish_case found ok false 0 abc123 0 1
require_text "release upload" "$temp_dir/gh-calls.log" "an update after a transient lookup failure was retried"
if grep -q "release create" "$temp_dir/gh-calls.log"; then
  printf 'transient lookup failure fell through to create\n' >&2
  exit 1
fi
view_call_count="$(grep '^release view' "$temp_dir/gh-calls.log" | grep -vc 'json assets')"
if [[ "$view_call_count" != "2" ]]; then
  printf 'transient lookup failure attempted %s lookups, want 2\n' "$view_call_count" >&2
  exit 1
fi

# The draft that a failed create leaves behind carries no git tag, so it can
# only be finished by publishing it.
: >"$temp_dir/gh-calls.log"
run_publish_case found ok false 0 abc123 0 0 true
require_text "--draft=false" "$temp_dir/gh-calls.log" "the edit that publishes a leftover draft"

: >"$temp_dir/gh-calls.log"
run_publish_case found ok true 0 deadbeef
require_text "release upload" "$temp_dir/gh-calls.log" "an asset upload when recovery mode is enabled"

: >"$temp_dir/gh-calls.log"
run_publish_case found ok false 1 deadbeef
if grep -q "release upload" "$temp_dir/gh-calls.log"; then
  printf 'existing release for another source was overwritten with recovery mode disabled\n' >&2
  exit 1
fi

# A lookup that fails for any reason other than "not found" must abort before
# creating anything, and must not be reported as a release that belongs to
# another commit: that misreading turned a transient GitHub outage into a
# permanent "already exists for another source" failure.
: >"$temp_dir/gh-calls.log"
run_publish_case error ok true 1
if grep -q "release create" "$temp_dir/gh-calls.log"; then
  printf 'unreadable release lookup fell through to create\n' >&2
  exit 1
fi
if grep -q "already exists for another source" "$publish_stderr"; then
  printf 'unreadable release lookup was reported as a release for another source\n' >&2
  exit 1
fi

# The create/exists race: another run created the release after the lookup.
: >"$temp_dir/gh-calls.log"
run_publish_case missing conflict true 0
require_text "release upload" "$temp_dir/gh-calls.log" "an update after losing the create race"

: >"$temp_dir/gh-calls.log"
run_publish_case missing conflict false 1
