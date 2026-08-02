#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  printf 'usage: release-tags.sh <major.minor.patch>\n' >&2
  exit 2
fi

version="${1#v}"
if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  printf 'release version must use major.minor.patch without prerelease or build metadata\n' >&2
  exit 2
fi

major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"

printf '%s\n' "$version"
printf 'latest\n'
printf '%s\n' "$major"
printf '%s.%s\n' "$major" "$minor"
