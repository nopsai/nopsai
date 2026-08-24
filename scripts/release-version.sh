#!/usr/bin/env bash
set -euo pipefail

# The version is whatever version.txt says. It is not computed, not offset, and
# not derived from history, so rewriting history cannot move it and two builds
# of the same commit cannot disagree about it.
#
# Forgetting to bump it is caught at publication: the release pipeline refuses
# to publish when the tag for this version already exists on a different commit.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ref="HEAD"
format="version"

while (($# > 0)); do
  case "$1" in
    --ref)
      ref="${2:?missing --ref value}"
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

version="$(tr -d '[:space:]' <"$ROOT_DIR/version.txt")"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'version.txt must contain an exact major.minor.patch version\n' >&2
  exit 1
fi

source_commit="$(git -C "$ROOT_DIR" rev-parse "$ref^{commit}")"
short_commit="${source_commit:0:12}"
major="${version%%.*}"
minor="${version#*.}"
minor="${minor%%.*}"

case "$format" in
  version)
    printf '%s\n' "$version"
    ;;
  env)
    printf 'VERSION=%s\n' "$version"
    printf 'IMAGE_TAG=%s\n' "$version"
    printf 'SOURCE_COMMIT=%s\n' "$source_commit"
    printf 'SHORT_COMMIT=%s\n' "$short_commit"
    printf 'MAJOR_TAG=%s\n' "$major"
    printf 'MAJOR_MINOR_TAG=%s.%s\n' "$major" "$minor"
    ;;
  github)
    printf 'version=%s\n' "$version"
    printf 'image_tag=%s\n' "$version"
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
