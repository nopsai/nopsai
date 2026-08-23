#!/usr/bin/env bash
# Generates the release-specific third-party notice bundle that
# THIRD_PARTY_NOTICES.md requires before external distribution.
#
# scripts/license-check.sh decides which licences are *allowed*. This script
# produces the actual attribution document: per component, its name, version,
# licence identifier, and the full licence text as shipped by that component.
#
# Usage:
#   scripts/generate-notices.sh [--output FILE] [--version VERSION] [--image REF]
#
# --image may be repeated to sweep the OS packages of built images.
# Exit codes: 0 written, 1 a component is missing a licence text, 2 usage error.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT=""
VERSION=""
IMAGES=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output) OUTPUT="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --image) IMAGES+=("${2:-}"); shift 2 ;;
    -h|--help) sed -n '2,12p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$VERSION" ]]; then
  VERSION="$(tr -d ' \n\r' <"$ROOT_DIR/release/version.txt" 2>/dev/null || echo dev)"
fi
if [[ -z "$OUTPUT" ]]; then
  OUTPUT="$ROOT_DIR/dist/license-report/THIRD-PARTY-NOTICES-$VERSION.md"
fi

mkdir -p "$(dirname "$OUTPUT")"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

missing=0
component_count=0

find_license_file() {
  find "$1" -maxdepth 2 -type f \( \
    -iname 'LICENSE' -o -iname 'LICENSE.*' -o \
    -iname 'LICENCE' -o -iname 'LICENCE.*' -o \
    -iname 'COPYING' -o -iname 'COPYING.*' -o \
    -iname 'COPYRIGHT' -o \
    -iname 'NOTICE' -o -iname 'NOTICE.*' \
  \) 2>/dev/null | sort | head -1
}

emit_component() {
  local ecosystem="$1" name="$2" version="$3" license="$4" text_file="$5"
  component_count=$((component_count + 1))
  {
    printf '\n### %s\n\n' "$name"
    printf -- '- Ecosystem: %s\n' "$ecosystem"
    [[ -n "$version" ]] && printf -- '- Version: %s\n' "$version"
    printf -- '- Licence: %s\n\n' "${license:-UNKNOWN}"
    if [[ -n "$text_file" && -s "$text_file" ]]; then
      printf '```text\n'
      # Fenced blocks must survive a licence text that contains backticks.
      sed 's/```/`\u200b``/g' "$text_file"
      printf '\n```\n'
    else
      printf '_No licence text was shipped with this component._\n'
      printf 'MISSING LICENCE TEXT: %s %s %s\n' "$ecosystem" "$name" "$version" >&2
      missing=$((missing + 1))
    fi
  } >>"$OUTPUT"
}

collect_go() {
  command -v go >/dev/null || { echo "go is required" >&2; exit 2; }
  command -v jq >/dev/null || { echo "jq is required" >&2; exit 2; }

  # "go mod download all" resolves the full module graph, including test-only
  # dependencies of dependencies, and appends them to go.sum. Generating a
  # notice bundle must not leave the working tree dirty.
  if [[ -f "$ROOT_DIR/go.sum" ]]; then
    cp "$ROOT_DIR/go.sum" "$TEMP_DIR/go.sum.orig"
    restore_go_sum() {
      if ! cmp -s "$TEMP_DIR/go.sum.orig" "$ROOT_DIR/go.sum"; then
        cp "$TEMP_DIR/go.sum.orig" "$ROOT_DIR/go.sum"
      fi
    }
    trap 'restore_go_sum; rm -rf "$TEMP_DIR"' EXIT
  fi

  printf '\n## Go Modules\n' >>"$OUTPUT"
  # Process substitution, not a pipe: emit_component updates the component and
  # missing counters, and a pipeline would run it in a subshell and discard both.
  local path version dir license_file license
  while IFS=$'\t' read -r path version dir; do
    [[ -z "$dir" || ! -d "$dir" ]] && continue
    license_file="$(find_license_file "$dir")"
    license="$("$ROOT_DIR/scripts/license-check.sh" --classify "$license_file" 2>/dev/null || echo "")"
    emit_component go "$path" "$version" "$license" "$license_file"
  done < <(
    cd "$ROOT_DIR"
    GOFLAGS="${GOFLAGS:-} -mod=readonly" go mod download all >/dev/null
    GOFLAGS="${GOFLAGS:-} -mod=readonly" go list -m -json all |
      jq -rs '.[] | select(.Main != true) | [.Path, .Version, (.Dir // "")] | @tsv' | sort -u
  )
}

collect_ui() {
  local node_modules="$ROOT_DIR/services/ui/node_modules"
  printf '\n## UI Packages\n' >>"$OUTPUT"
  if [[ ! -d "$node_modules" ]]; then
    echo "services/ui/node_modules is not installed; run npm ci before generating notices" >&2
    exit 2
  fi

  # Attribution obligations attach to what is distributed. The UI image serves
  # built static assets, so build and test tooling never reaches a customer and
  # is deliberately excluded. INCLUDE_DEV_DEPENDENCIES=1 restores the full tree
  # for an internal audit.
  local -a package_dirs=()
  if [[ "${INCLUDE_DEV_DEPENDENCIES:-}" == "1" ]] || ! command -v npm >/dev/null; then
    if [[ "${INCLUDE_DEV_DEPENDENCIES:-}" != "1" ]]; then
      echo "npm is not available; falling back to the full node_modules tree" >&2
    fi
    local manifest
    while IFS= read -r -d '' manifest; do
      package_dirs+=("$(dirname "$manifest")")
    done < <(find "$node_modules" -maxdepth 4 -path '*/package.json' -type f -print0)
  else
    local entry
    while IFS= read -r entry; do
      [[ -z "$entry" || ! -d "$entry" ]] && continue
      # npm lists the workspace root itself first; it is NopsAI, not a third party.
      [[ "$entry" == "$ROOT_DIR/services/ui" ]] && continue
      package_dirs+=("$entry")
    done < <(cd "$ROOT_DIR/services/ui" && npm ls --omit=dev --all --parseable 2>/dev/null)
  fi

  local dir name version license license_file
  for dir in "${package_dirs[@]}"; do
    [[ -f "$dir/package.json" ]] || continue
    name="$(jq -r '.name // empty' "$dir/package.json" 2>/dev/null || true)"
    [[ -z "$name" ]] && continue
    version="$(jq -r '.version // ""' "$dir/package.json")"
    license="$(jq -r '
      if (.license | type) == "string" then .license
      elif (.license | type) == "object" then (.license.type // "UNKNOWN")
      elif (.licenses | type) == "array" then ((.licenses | map(if type == "string" then . else (.type // "UNKNOWN") end)) | join(" OR "))
      elif (.licenses | type) == "object" then (.licenses.type // "UNKNOWN")
      else "UNKNOWN" end' "$dir/package.json")"
    license_file="$(find_license_file "$dir")"
    emit_component ui "$name" "$version" "$license" "$license_file"
  done
}

collect_os_packages() {
  local image="$1"
  printf '\n## Operating System Packages — %s\n\n' "$image" >>"$OUTPUT"
  # Alpine records the licence identifier per installed package. The licence
  # texts themselves live in the distribution, not in the image, so the bundle
  # records the identifier and points at the upstream source offer.
  if ! docker run --rm --entrypoint /bin/sh "$image" -c \
      'apk info -v 2>/dev/null | while read -r pkg; do printf "%s\t%s\n" "$pkg" "$(apk info -L "$pkg" >/dev/null 2>&1 && apk info -d "$pkg" 2>/dev/null | head -1)"; done' \
      >"$TEMP_DIR/os-packages.tsv" 2>/dev/null; then
    printf '_Package inventory could not be read from this image._\n' >>"$OUTPUT"
    return
  fi
  printf '| Package | Description |\n| --- | --- |\n' >>"$OUTPUT"
  while IFS=$'\t' read -r pkg desc; do
    [[ -z "$pkg" ]] && continue
    printf '| %s | %s |\n' "$pkg" "${desc//|/\\|}" >>"$OUTPUT"
    component_count=$((component_count + 1))
  done <"$TEMP_DIR/os-packages.tsv"
}

cat >"$OUTPUT" <<HEADER
# NopsAI Third-Party Notices — $VERSION

NopsAI includes, links to, packages, or depends on the third-party software
listed below. Those components are not licensed under the NopsAI proprietary
notice in \`LICENSE\`; each remains subject to its own copyright notice and
licence terms, reproduced here in full.

No NopsAI notice removes or limits rights granted by a third-party licence.

Generated by \`scripts/generate-notices.sh\`. Do not edit by hand.
HEADER

collect_go
collect_ui
for image in "${IMAGES[@]:-}"; do
  [[ -z "$image" ]] && continue
  collect_os_packages "$image"
done

printf '\n---\n\nComponents recorded: %s\n' "$component_count" >>"$OUTPUT"

if [[ "$missing" -gt 0 ]]; then
  echo "" >&2
  echo "Notice bundle incomplete: $missing component(s) shipped no licence text." >&2
  echo "A component with no reproducible licence text cannot be distributed." >&2
  exit 1
fi

echo "Wrote $OUTPUT with $component_count components."
