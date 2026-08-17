#!/usr/bin/env bash
# Builds the nopsai-base image in one of two modes.
#
#   cache  Runs the build without producing a release artifact and exports the
#          result to the registry build cache. Safe to run before the quality
#          gates pass, because it publishes a :buildcache tag rather than any
#          release tag.
#   push   Runs the same build with the release tags and pushes it. Every layer
#          is served from the cache written by the cache mode, so this is a
#          cache replay rather than a second compile.
#
# Both modes must produce identical cache keys, so the build flags below are
# shared and only the output flags differ.
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: build-release-base-image.sh <cache|push>" >&2
  exit 2
fi

mode="$1"
case "$mode" in
  cache|push) ;;
  *)
    echo "unsupported mode $mode; expected cache or push" >&2
    exit 2
    ;;
esac

. dist/release/env
if [[ "$SHOULD_RELEASE" != "true" ]]; then
  echo "Release v$VERSION already exists; skipping base image $mode"
  exit 0
fi
if [[ -z "${NOPSAI_RELEASE_GHCR_TOKEN:-}" ]]; then
  echo "NOPSAI_RELEASE_GHCR_TOKEN is required to build the base image" >&2
  exit 1
fi

printf '%s' "$NOPSAI_RELEASE_GHCR_TOKEN" | docker login ghcr.io --username "$GHCR_USERNAME" --password-stdin
if [[ "${NOPSAI_RELEASE_ENABLE_QEMU:-true}" == "true" && "$PLATFORMS" == *","* ]]; then
  docker run --privileged --rm tonistiigi/binfmt@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0 --install all
fi

builder_suffix="$(printf '%s-nopsai-base-%s-%s' "$VERSION" "$mode" "$$" | tr -c 'A-Za-z0-9_.-' '-')"
builder="${NOPSAI_RELEASE_BUILDX_NAME:-nopsai-release-builder}-${builder_suffix}"
if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  docker buildx create --name "$builder" --driver docker-container --use
else
  docker buildx use "$builder"
fi
trap 'docker buildx rm "$builder" >/dev/null 2>&1 || true' EXIT
docker buildx inspect --bootstrap

cache_ref="$REGISTRY/nopsai-base:buildcache"

build_args=(
  --file Dockerfile
  --platform "$PLATFORMS"
  --builder "$builder"
  --build-arg "VERSION=$VERSION"
  --build-arg "COMMIT=$SOURCE_COMMIT"
  --build-arg "BUILD_DATE=$BUILD_DATE"
  --build-arg "SOURCE_URL=$SOURCE_URL"
  --build-arg "API_VERSION=$API_VERSION"
  --build-arg "RUNNER_PROTOCOL_VERSION=$RUNNER_PROTOCOL_VERSION"
  --build-arg "CLI_COMPATIBILITY=$CLI_COMPATIBILITY"
  --build-arg "RUNNER_COMPATIBILITY=$RUNNER_COMPATIBILITY"
  --build-arg "PLATFORM_COMPATIBILITY=$PLATFORM_COMPATIBILITY"
  --build-arg "CAPABILITIES=$CAPABILITIES"
  --cache-from "type=registry,ref=$cache_ref"
)

if [[ "$mode" == "cache" ]]; then
  echo "Warming the nopsai-base build cache"
  docker buildx build \
    "${build_args[@]}" \
    --cache-to "type=registry,ref=$cache_ref,mode=max" \
    --output type=cacheonly \
    .
  echo "warmed_base_cache=$cache_ref"
  exit 0
fi

rm -rf dist/digests dist/docker-metadata
mkdir -p dist/digests dist/docker-metadata
release_tag_args=()
while IFS= read -r release_tag; do
  release_tag_args+=(--tag "$REGISTRY/nopsai-base:$release_tag")
done < <(scripts/release-tags.sh "$VERSION")
oci_annotation_args=(
  --annotation "index,manifest:org.opencontainers.image.source=$SOURCE_URL"
  --annotation "index,manifest:org.opencontainers.image.version=$VERSION"
  --annotation "index,manifest:org.opencontainers.image.revision=$SOURCE_COMMIT"
  --annotation "index,manifest:org.opencontainers.image.created=$BUILD_DATE"
  --annotation "index,manifest:org.opencontainers.image.licenses=LicenseRef-NopsAI-Proprietary"
  --annotation "index,manifest:org.opencontainers.image.vendor=NopsAI"
  --annotation "index,manifest:org.opencontainers.image.title=nopsai-base"
)

echo "Publishing nopsai-base"
docker buildx build \
  "${build_args[@]}" \
  --push \
  "${release_tag_args[@]}" \
  "${oci_annotation_args[@]}" \
  --provenance=mode=max \
  --sbom=true \
  --metadata-file dist/docker-metadata/nopsai-base.json \
  .
base_digest="$(jq -r '."containerimage.digest"' dist/docker-metadata/nopsai-base.json)"
if [[ ! "$base_digest" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo "Docker buildx did not return a valid nopsai-base digest" >&2
  exit 1
fi
printf '%s\n' "$base_digest" >dist/digests/nopsai-base.digest
echo "published_image=$REGISTRY/nopsai-base@$base_digest"
