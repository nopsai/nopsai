#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "usage: publish-release-image.sh <image-name> <context-path> <dockerfile-path>" >&2
  exit 2
fi

image_name="$1"
context_path="$2"
dockerfile_path="$3"

. dist/release/env
if [[ "$SHOULD_RELEASE" != "true" ]]; then
  echo "Release v$VERSION already exists; skipping $image_name publication"
  exit 0
fi
if [[ -z "${NOPSAI_RELEASE_GHCR_TOKEN:-}" ]]; then
  echo "NOPSAI_RELEASE_GHCR_TOKEN is required to publish container images" >&2
  exit 1
fi
if [[ ! -s dist/digests/nopsai-base.digest ]]; then
  echo "Base image digest is required before publishing $image_name" >&2
  exit 1
fi

base_digest="$(tr -d '\r\n' <dist/digests/nopsai-base.digest)"
if [[ ! "$base_digest" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo "Invalid base image digest for $image_name: $base_digest" >&2
  exit 1
fi

printf '%s' "$NOPSAI_RELEASE_GHCR_TOKEN" | docker login ghcr.io --username "$GHCR_USERNAME" --password-stdin
builder_suffix="$(printf '%s-%s-%s' "$VERSION" "$image_name" "$$" | tr -c 'A-Za-z0-9_.-' '-')"
builder="${NOPSAI_RELEASE_BUILDX_NAME:-nopsai-release-builder}-${builder_suffix}"
if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  docker buildx create --name "$builder" --driver docker-container --use
else
  docker buildx use "$builder"
fi
trap 'docker buildx rm "$builder" >/dev/null 2>&1 || true' EXIT
docker buildx inspect --bootstrap

mkdir -p dist/digests dist/docker-metadata
metadata_file="dist/docker-metadata/${image_name}.json"
release_tag_args=()
while IFS= read -r release_tag; do
  release_tag_args+=(--tag "$REGISTRY/$image_name:$release_tag")
done < <(scripts/release-tags.sh "$VERSION")
oci_annotation_args=(
  --annotation "index,manifest:org.opencontainers.image.source=$SOURCE_URL"
  --annotation "index,manifest:org.opencontainers.image.version=$VERSION"
  --annotation "index,manifest:org.opencontainers.image.revision=$SOURCE_COMMIT"
  --annotation "index,manifest:org.opencontainers.image.created=$BUILD_DATE"
  --annotation "index,manifest:org.opencontainers.image.licenses=LicenseRef-NopsAI-Proprietary"
  --annotation "index,manifest:org.opencontainers.image.vendor=NopsAI"
  --annotation "index,manifest:org.opencontainers.image.title=$image_name"
)
echo "Building and publishing $image_name"
docker buildx build \
  --file "$dockerfile_path" \
  --platform "$PLATFORMS" \
  --push \
  "${release_tag_args[@]}" \
  "${oci_annotation_args[@]}" \
  --build-arg "BASE_IMAGE=$REGISTRY/nopsai-base@$base_digest" \
  --build-arg "VERSION=$VERSION" \
  --build-arg "COMMIT=$SOURCE_COMMIT" \
  --build-arg "BUILD_DATE=$BUILD_DATE" \
  --build-arg "SOURCE_URL=$SOURCE_URL" \
  --build-arg "API_VERSION=$API_VERSION" \
  --build-arg "RUNNER_PROTOCOL_VERSION=$RUNNER_PROTOCOL_VERSION" \
  --build-arg "CLI_COMPATIBILITY=$CLI_COMPATIBILITY" \
  --build-arg "RUNNER_COMPATIBILITY=$RUNNER_COMPATIBILITY" \
  --build-arg "PLATFORM_COMPATIBILITY=$PLATFORM_COMPATIBILITY" \
  --build-arg "CAPABILITIES=$CAPABILITIES" \
  --provenance=mode=max \
  --sbom=true \
  --metadata-file "$metadata_file" \
  "$context_path"
digest="$(jq -r '."containerimage.digest"' "$metadata_file")"
if [[ ! "$digest" =~ ^sha256:[a-f0-9]{64}$ ]]; then
  echo "Docker buildx did not return a valid $image_name digest" >&2
  exit 1
fi
printf '%s\n' "$digest" >"dist/digests/${image_name}.digest"
echo "published_image=$REGISTRY/$image_name@$digest"
