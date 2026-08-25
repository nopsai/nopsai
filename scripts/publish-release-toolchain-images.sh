#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

tag="${1:-${NOPSAI_RELEASE_TOOLCHAIN_TAG:-2026.08.25}}"
registry="${NOPSAI_RELEASE_TOOLCHAIN_REGISTRY:-ghcr.io/nopsai}"
platforms="${NOPSAI_RELEASE_PLATFORMS:-linux/amd64,linux/arm64}"
builder="${NOPSAI_RELEASE_BUILDX_NAME:-nopsai-release-toolchain-builder}"

require_tool() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "missing required tool: $tool" >&2
    exit 2
  fi
}

if [[ ! "$tag" =~ ^[A-Za-z0-9_.-]+$ ]]; then
  echo "release toolchain tag must contain only letters, numbers, dots, underscores, and hyphens" >&2
  exit 2
fi

require_tool docker
require_tool jq
if ! docker buildx version >/dev/null 2>&1; then
  echo "missing required Docker Buildx plugin" >&2
  exit 2
fi

registry="${registry%/}"
registry_host="${registry%%/*}"
source_commit="$(git rev-parse HEAD 2>/dev/null || printf unknown)"
build_date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
source_url="${NOPSAI_RELEASE_SOURCE_URL:-https://github.com/nopsai/nopsai}"

if [[ -n "${NOPSAI_RELEASE_GHCR_TOKEN:-}" ]]; then
  ghcr_username="${NOPSAI_RELEASE_GHCR_USERNAME:-nopsai}"
  printf '%s' "$NOPSAI_RELEASE_GHCR_TOKEN" | docker login "$registry_host" --username "$ghcr_username" --password-stdin
fi

if [[ "${NOPSAI_RELEASE_ENABLE_QEMU:-true}" == "true" && "$platforms" == *","* ]]; then
  docker run --privileged --rm tonistiigi/binfmt@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0 --install all
fi

docker buildx rm --force "$builder" >/dev/null 2>&1 || true
docker buildx create --name "$builder" --driver docker-container
trap 'docker buildx rm --force "$builder" >/dev/null 2>&1 || true' EXIT
docker buildx inspect --bootstrap --builder "$builder"

images=(
  "nopsai-release-core:container/Dockerfile.release-core"
  "nopsai-release-go:container/Dockerfile.release-go"
  "nopsai-release-node:container/Dockerfile.release-node"
  "nopsai-release-docker:container/Dockerfile.release-docker"
)

build_args=(
  --build-arg "VERSION=$tag"
  --build-arg "COMMIT=$source_commit"
  --build-arg "BUILD_DATE=$build_date"
  --build-arg "SOURCE_URL=$source_url"
)
for name in \
  NOPSAI_RELEASE_HELM_VERSION \
  NOPSAI_RELEASE_HELM_SHA256_AMD64 \
  NOPSAI_RELEASE_HELM_SHA256_ARM64 \
  NOPSAI_RELEASE_ORAS_VERSION \
  NOPSAI_RELEASE_ORAS_SHA256_AMD64 \
  NOPSAI_RELEASE_ORAS_SHA256_ARM64 \
  NOPSAI_RELEASE_GH_VERSION \
  NOPSAI_RELEASE_GH_SHA256_AMD64 \
  NOPSAI_RELEASE_GH_SHA256_ARM64; do
  if [[ -n "${!name:-}" ]]; then
    build_args+=(--build-arg "$name=${!name}")
  fi
done

for spec in "${images[@]}"; do
  image_name="${spec%%:*}"
  dockerfile="${spec#*:}"
  cache_ref="$registry/$image_name:buildcache"
  metadata_file="$(mktemp)"
  echo "publishing_toolchain_image=$registry/$image_name:$tag"
  docker buildx build \
    --builder "$builder" \
    --file "$dockerfile" \
    --platform "$platforms" \
    --tag "$registry/$image_name:$tag" \
    --tag "$registry/$image_name:latest" \
    "${build_args[@]}" \
    --cache-from "type=registry,ref=$cache_ref" \
    --cache-to "type=registry,ref=$cache_ref,mode=max" \
    --push \
    --provenance=mode=max \
    --sbom=true \
    --metadata-file "$metadata_file" \
    .
  digest="$(jq -r '."containerimage.digest"' "$metadata_file")"
  rm -f "$metadata_file"
  if [[ ! "$digest" =~ ^sha256:[a-f0-9]{64}$ ]]; then
    echo "Docker buildx did not return a valid $image_name digest" >&2
    exit 1
  fi
  echo "published_toolchain_image=$registry/$image_name@$digest"
done
