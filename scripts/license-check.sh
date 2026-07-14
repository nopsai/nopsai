#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPORT_DIR="${LICENSE_REPORT_DIR:-}"
TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

GO_INVENTORY="$TEMP_DIR/go-licenses.tsv"
UI_INVENTORY="$TEMP_DIR/ui-licenses.tsv"
SUMMARY="$TEMP_DIR/license-summary.txt"

require_tool() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf 'missing required tool: %s\n' "$tool" >&2
    return 1
  fi
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

normalize_license() {
  local value
  value="$(trim "$1")"
  value="${value#(}"
  value="${value%)}"
  printf '%s' "$value"
}

is_allowed_license() {
  local license
  license="$(normalize_license "$1")"
  case "$license" in
    Apache-2.0 | BSD-2-Clause | BSD-3-Clause | BlueOak-1.0.0 | CC-BY-4.0 | CC0-1.0 | ISC | MIT | MIT-0 | MPL-2.0 | Python-2.0)
      return 0
      ;;
  esac
  return 1
}

is_review_license() {
  local license
  license="$(normalize_license "$1")"
  case "$license" in
    BlueOak-1.0.0 | CC-BY-4.0 | MPL-2.0 | Python-2.0)
      return 0
      ;;
  esac
  return 1
}

is_forbidden_license() {
  local license lower
  license="$(normalize_license "$1")"
  lower="$(printf '%s' "$license" | tr '[:upper:]' '[:lower:]')"
  case "$lower" in
    *agpl* | *gpl* | *lgpl* | *sspl* | *commons-clause* | *commons\ clause* | *business-source* | *busl* | *polyform-noncommercial* | *noncommercial*)
      return 0
      ;;
  esac
  return 1
}

classify_go_license_file() {
  local file="$1"
  local text
  text="$(tr '[:upper:]' '[:lower:]' <"$file" | tr '\n' ' ' | sed 's/[[:space:]]\+/ /g')"

  case "$text" in
    *"mozilla public license version 2.0"*)
      printf 'MPL-2.0'
      return
      ;;
    *"gnu affero general public license"*)
      printf 'AGPL'
      return
      ;;
    *"gnu lesser general public license"*)
      printf 'LGPL'
      return
      ;;
    *"gnu general public license"*)
      printf 'GPL'
      return
      ;;
    *"server side public license"*)
      printf 'SSPL'
      return
      ;;
    *"apache license"*"version 2.0"*)
      printf 'Apache-2.0'
      return
      ;;
    *"mit license"* | *"permission is hereby granted, free of charge"*)
      printf 'MIT'
      return
      ;;
    *"isc license"* | *"permission to use, copy, modify, and/or distribute"*)
      printf 'ISC'
      return
      ;;
    *"redistribution and use in source and binary forms"*"neither the name"*)
      printf 'BSD-3-Clause'
      return
      ;;
    *"redistribution and use in source and binary forms"*)
      printf 'BSD-2-Clause'
      return
      ;;
  esac

  printf 'UNKNOWN'
}

find_go_license_file() {
  local dir="$1"
  find "$dir" -maxdepth 2 -type f \( \
    -iname 'LICENSE' -o -iname 'LICENSE.*' -o \
    -iname 'LICENCE' -o -iname 'LICENCE.*' -o \
    -iname 'COPYING' -o -iname 'COPYING.*' -o \
    -iname 'COPYRIGHT' -o \
    -iname 'NOTICE' -o -iname 'NOTICE.*' \
  \) | sort | head -1
}

collect_go_licenses() {
  require_tool go
  require_tool jq

  local readonly_go_flags
  readonly_go_flags="${GOFLAGS:-}"
  if [[ -n "$readonly_go_flags" ]]; then
    readonly_go_flags="$readonly_go_flags -mod=readonly"
  else
    readonly_go_flags="-mod=readonly"
  fi

  (
    cd "$ROOT_DIR"
    GOFLAGS="$readonly_go_flags" go mod download all >/dev/null
    GOFLAGS="$readonly_go_flags" go list -m -json all
  ) | jq -rs '.[] | select(.Main != true) | [.Path, .Version, .Dir] | @tsv' |
    while IFS=$'\t' read -r path version dir; do
      local license_file license relative_license_file
      license_file="$(find_go_license_file "$dir")"
      if [[ -z "$license_file" ]]; then
        license="UNKNOWN"
        relative_license_file=""
      else
        license="$(classify_go_license_file "$license_file")"
        relative_license_file="${license_file#"$dir"/}"
      fi
      printf 'go\t%s\t%s\t%s\t%s\n' "$path" "$version" "$license" "$relative_license_file"
    done >"$GO_INVENTORY"
}

read_package_license_jq='
  select(.name != null) |
  [
    "ui",
    .name,
    (.version // ""),
    (
      if (.license | type) == "string" then
        .license
      elif (.license | type) == "object" then
        (.license.type // "UNKNOWN")
      elif (.licenses | type) == "array" then
        ((.licenses | map(if type == "string" then . else (.type // "UNKNOWN") end)) | join(" OR "))
      elif (.licenses | type) == "object" then
        (.licenses.type // "UNKNOWN")
      else
        "UNKNOWN"
      end
    ),
    "package.json"
  ] |
  @tsv
'

collect_ui_licenses_from_node_modules() {
  local node_modules="$1"
  find "$node_modules" -maxdepth 4 -path '*/package.json' -type f -print0 |
    xargs -0 jq -r "$read_package_license_jq" |
    sort -u >"$UI_INVENTORY"
}

collect_ui_licenses_with_docker() {
  require_tool docker
  docker run --rm \
    -v "$ROOT_DIR/services/ui/package.json:/src/package.json:ro" \
    -v "$ROOT_DIR/services/ui/package-lock.json:/src/package-lock.json:ro" \
    node:22-alpine sh -s <<'SH' >"$UI_INVENTORY"
set -euo pipefail
apk add --no-cache jq >/dev/null
work_dir="$(mktemp -d)"
cp /src/package.json /src/package-lock.json "$work_dir"/
cd "$work_dir"
npm ci --ignore-scripts --no-audit --no-fund >/dev/null
find node_modules -maxdepth 4 -path '*/package.json' -type f -print0 |
  xargs -0 jq -r '
    select(.name != null) |
    [
      "ui",
      .name,
      (.version // ""),
      (
        if (.license | type) == "string" then
          .license
        elif (.license | type) == "object" then
          (.license.type // "UNKNOWN")
        elif (.licenses | type) == "array" then
          ((.licenses | map(if type == "string" then . else (.type // "UNKNOWN") end)) | join(" OR "))
        elif (.licenses | type) == "object" then
          (.licenses.type // "UNKNOWN")
        else
          "UNKNOWN"
        end
      ),
      "package.json"
    ] |
    @tsv
  ' |
  sort -u
SH
}

collect_ui_licenses() {
  require_tool jq

  local node_modules="$ROOT_DIR/services/ui/node_modules"
  if [[ -d "$node_modules" ]] && [[ -n "$(find "$node_modules" -maxdepth 2 -path '*/package.json' -type f -print -quit)" ]]; then
    collect_ui_licenses_from_node_modules "$node_modules"
    return
  fi

  if command -v npm >/dev/null 2>&1; then
    (
      cd "$ROOT_DIR/services/ui"
      npm ci --ignore-scripts --no-audit --no-fund >/dev/null
    )
    collect_ui_licenses_from_node_modules "$node_modules"
    return
  fi

  collect_ui_licenses_with_docker
}

validate_inventory() {
  local inventory="$1"
  local failures=0
  while IFS=$'\t' read -r ecosystem name version license source; do
    license="$(normalize_license "$license")"
    if [[ -z "$license" || "$license" == "UNKNOWN" ]]; then
      printf 'unknown license: %s %s %s (%s)\n' "$ecosystem" "$name" "$version" "$source" >&2
      failures=1
      continue
    fi
    if is_forbidden_license "$license"; then
      printf 'forbidden license: %s %s %s is %s (%s)\n' "$ecosystem" "$name" "$version" "$license" "$source" >&2
      failures=1
      continue
    fi
    if ! is_allowed_license "$license"; then
      printf 'license requires policy review before release: %s %s %s is %s (%s)\n' "$ecosystem" "$name" "$version" "$license" "$source" >&2
      failures=1
    fi
  done <"$inventory"
  return "$failures"
}

print_summary() {
  {
    printf 'Go licenses:\n'
    awk -F '\t' '{ count[$4]++ } END { for (license in count) printf "  %s %s\n", count[license], license }' "$GO_INVENTORY" | sort -k2
    printf '\nUI licenses:\n'
    awk -F '\t' '{ count[$4]++ } END { for (license in count) printf "  %s %s\n", count[license], license }' "$UI_INVENTORY" | sort -k2
    printf '\nReview-notice licenses:\n'
    awk -F '\t' '{ print }' "$GO_INVENTORY" "$UI_INVENTORY" |
      while IFS=$'\t' read -r ecosystem name version license source; do
        if is_review_license "$license"; then
          printf '  %s %s %s %s (%s)\n' "$ecosystem" "$name" "$version" "$license" "$source"
        fi
      done | sort -u
  } | tee "$SUMMARY"
}

copy_reports() {
  if [[ -z "$REPORT_DIR" ]]; then
    return
  fi
  mkdir -p "$REPORT_DIR"
  cp "$GO_INVENTORY" "$REPORT_DIR/go-licenses.tsv"
  cp "$UI_INVENTORY" "$REPORT_DIR/ui-licenses.tsv"
  cp "$SUMMARY" "$REPORT_DIR/license-summary.txt"
}

collect_go_licenses
collect_ui_licenses

validate_inventory "$GO_INVENTORY"
validate_inventory "$UI_INVENTORY"
print_summary
copy_reports

printf '\nLicense compatibility gate passed.\n'
