#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output=""
registry=""
version=""
commit=""
build_date=""
digest_dir=""

while (($# > 0)); do
  case "$1" in
    --output) output="${2:?missing --output value}"; shift 2 ;;
    --registry) registry="${2:?missing --registry value}"; shift 2 ;;
    --version) version="${2:?missing --version value}"; shift 2 ;;
    --commit) commit="${2:?missing --commit value}"; shift 2 ;;
    --build-date) build_date="${2:?missing --build-date value}"; shift 2 ;;
    --digest-dir) digest_dir="${2:?missing --digest-dir value}"; shift 2 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

for value_name in output registry version commit build_date; do
  if [[ -z "${!value_name}" ]]; then
    printf '%s is required\n' "$value_name" >&2
    exit 2
  fi
done
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'version must use major.minor.patch\n' >&2
  exit 2
fi
if ! command -v jq >/dev/null 2>&1; then
  printf 'jq is required to render a release bundle\n' >&2
  exit 1
fi

registry="${registry%/}"
mkdir -p "$output"
cp "$ROOT_DIR/deploy/docker-compose.release.yaml" "$output/docker-compose.yaml"
mkdir -p "$output/db"
cp "$ROOT_DIR/db/init.sql" "$output/db/init.sql"

image_specs=(
  "BASE:nopsai-base:base"
  "API:nopsai-api:api"
  "AAA:nopsai-aaa:aaa"
  "AGENT:nopsai-agent:agent"
  "DISPATCHER:nopsai-dispatcher:dispatcher"
  "GIT_BOT:nopsai-git-bot:gitBot"
  "RUNNER:nopsai-runner:runner"
  "K8S_RUNNER:nopsai-k8s-runner:k8sRunner"
  "DOCKER_SOCKET_PROXY:nopsai-docker-socket-proxy:dockerSocketProxy"
  "UI:nopsai-ui:ui"
  "PIPELINE:pipeline-image:pipeline"
)

index_file="$output/release-index.json"
jq -n \
  --arg version "$version" \
  --arg commit "$commit" \
  --arg build_date "$build_date" \
  --arg registry "$registry" \
  '{schemaVersion:"v1",version:$version,commit:$commit,buildDate:$build_date,registry:$registry,images:{}}' >"$index_file"

{
  printf 'NOPSAI_VERSION=%s\n' "$version"
  printf 'NOPSAI_COMMIT=%s\n' "$commit"
  printf 'NOPSAI_BUILD_DATE=%s\n' "$build_date"
  printf 'NOPSAI_IMAGE_REGISTRY=%s\n' "$registry"
  printf '\n# Supply these through the deployment secret manager before Compose starts.\n'
  printf '# POSTGRES_PASSWORD=\n'
  printf '# DATABASE_URL=postgres://nopsai_user:<password>@db:5432/nopsai_db\n'
  printf '# SERVICE_JWT_SIGNING_KEY=\n'
  printf '# JWT_SIGNING_KEY=\n'
  printf '# AAA_SHARED_INTERNAL_TOKEN=\n'
  printf '# NOPSAI_MASTER_KEY=\n'
} >"$output/.env"

sed_args=(
  -e "s|{{VERSION}}|$version|g"
  -e "s|{{COMMIT}}|$commit|g"
)

for spec in "${image_specs[@]}"; do
  IFS=: read -r env_name image_name values_key <<<"$spec"
  repository="$registry/$image_name"
  digest=""
  if [[ -n "$digest_dir" ]]; then
    digest_file="$digest_dir/$image_name.digest"
    if [[ ! -f "$digest_file" ]]; then
      printf 'missing image digest: %s\n' "$digest_file" >&2
      exit 1
    fi
    digest="$(tr -d '[:space:]' <"$digest_file")"
    if [[ ! "$digest" =~ ^sha256:[a-f0-9]{64}$ ]]; then
      printf 'invalid image digest in %s\n' "$digest_file" >&2
      exit 1
    fi
    image_ref="$repository@$digest"
    digest_hex="${digest#sha256:}"
  else
    image_ref="$repository:$version"
    digest_hex=""
  fi
  printf 'NOPSAI_%s_IMAGE=%s\n' "$env_name" "$image_ref" >>"$output/.env"
  temp_index="$index_file.tmp"
  jq --arg key "$values_key" --arg value "$image_ref" '.images[$key]=$value' "$index_file" >"$temp_index"
  mv "$temp_index" "$index_file"
  sed_args+=(
    -e "s|{{${env_name}_REPOSITORY}}|$repository|g"
    -e "s|{{${env_name}_TAG}}|$version|g"
    -e "s|{{${env_name}_DIGEST}}|$digest|g"
    -e "s|{{${env_name}_IMAGE_SHA256}}|$digest_hex|g"
  )
done

release_values="$output/.helm-release-values.yaml"
sed "${sed_args[@]}" "$ROOT_DIR/deploy/helm/release-images.yaml.tmpl" >"$release_values"
"$ROOT_DIR/scripts/package-helm-chart.sh" \
  --values "$release_values" \
  --output "$output" \
  --version "$version" >/dev/null
rm "$release_values"

chart_name="nopsai-$version.tgz"
chart_checksum="$(shasum -a 256 "$output/$chart_name" | awk '{print $1}')"
if [[ -n "$digest_dir" ]]; then
  sed \
    "${sed_args[@]}" \
    -e "s|{{CHART_REFERENCE}}|oci://$registry/charts/nopsai|g" \
    -e "s|{{CHART_SHA256}}|$chart_checksum|g" \
    "$ROOT_DIR/release/manifest.tmpl.json" >"$output/release-manifest.json"
  jq empty "$output/release-manifest.json" >/dev/null
  manifest_checksum="$(shasum -a 256 "$output/release-manifest.json" | awk '{print $1}')"
  temp_index="$index_file.tmp"
  jq \
    --arg file "release-manifest.json" \
    --arg checksum "sha256:$manifest_checksum" \
    '.manifest = {file:$file,sha256:$checksum}' \
    "$index_file" >"$temp_index"
  mv "$temp_index" "$index_file"
fi
temp_index="$index_file.tmp"
jq \
  --arg file "$chart_name" \
  --arg version "$version" \
  --arg checksum "sha256:$chart_checksum" \
  '.chart = {file:$file,version:$version,sha256:$checksum}' \
  "$index_file" >"$temp_index"
mv "$temp_index" "$index_file"
(
  cd "$output"
  checksum_files=(.env db/init.sql docker-compose.yaml "$chart_name" release-index.json)
  if [[ -f release-manifest.json ]]; then
    checksum_files+=(release-manifest.json)
  fi
  shasum -a 256 "${checksum_files[@]}" >checksums.txt
)
