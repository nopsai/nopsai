#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ref="HEAD"
offset="${VERSION_COMMIT_OFFSET:-0}"
format="version"

while (($# > 0)); do
  case "$1" in
    --ref)
      ref="${2:?missing --ref value}"
      shift 2
      ;;
    --offset)
      offset="${2:?missing --offset value}"
      shift 2
      ;;
    --format)
      format="${2:?missing --format value}"
      shift 2
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      exit 2
      ;;
  esac
done

if [[ ! "$offset" =~ ^[0-9]+$ ]]; then
  printf 'version commit offset must be a non-negative integer\n' >&2
  exit 2
fi

base_version="$(tr -d '[:space:]' <"$ROOT_DIR/release/version.txt")"
if [[ ! "$base_version" =~ ^[0-9]+\.[0-9]+$ ]]; then
  printf 'release/version.txt must contain major.minor\n' >&2
  exit 1
fi

commit_count="$(git -C "$ROOT_DIR" rev-list --count "$ref")"
source_commit="$(git -C "$ROOT_DIR" rev-parse "$ref^{commit}")"
version_patch=$((commit_count + offset))
version="${base_version}.${version_patch}"
short_commit="${source_commit:0:12}"
major="${base_version%%.*}"
minor="${base_version#*.}"

case "$format" in
  version)
    printf '%s\n' "$version"
    ;;
  env)
    printf 'VERSION=%s\n' "$version"
    printf 'IMAGE_TAG=%s\n' "$version"
    printf 'COMMIT_COUNT=%s\n' "$commit_count"
    printf 'VERSION_COMMIT_OFFSET=%s\n' "$offset"
    printf 'SOURCE_COMMIT=%s\n' "$source_commit"
    printf 'SHORT_COMMIT=%s\n' "$short_commit"
    printf 'MAJOR_TAG=%s\n' "$major"
    printf 'MAJOR_MINOR_TAG=%s.%s\n' "$major" "$minor"
    ;;
  github)
    printf 'version=%s\n' "$version"
    printf 'image_tag=%s\n' "$version"
    printf 'commit_count=%s\n' "$commit_count"
    printf 'version_commit_offset=%s\n' "$offset"
    printf 'source_commit=%s\n' "$source_commit"
    printf 'short_commit=%s\n' "$short_commit"
    printf 'major_tag=%s\n' "$major"
    printf 'major_minor_tag=%s.%s\n' "$major" "$minor"
    ;;
  *)
    printf 'unsupported output format: %s\n' "$format" >&2
    exit 2
    ;;
esac
