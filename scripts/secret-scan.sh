#!/usr/bin/env bash
# Fail-closed credential scan for the working tree and for built image
# filesystems. Runs before any publish step in the release pipeline, because a
# credential inside a publicly pullable image is permanently public.
#
# Usage:
#   scripts/secret-scan.sh                  # scan git-tracked files
#   scripts/secret-scan.sh --path DIR       # scan an extracted image rootfs
#   scripts/secret-scan.sh --image REF      # export a container image and scan it
#
# Exit codes: 0 clean, 1 findings, 2 usage or environment error.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ALLOWLIST="$ROOT_DIR/scripts/secret-scan-allowlist.txt"

SCAN_PATH=""
SCAN_IMAGE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --path) SCAN_PATH="${2:-}"; shift 2 ;;
    --image) SCAN_IMAGE="${2:-}"; shift 2 ;;
    --allowlist) ALLOWLIST="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,10p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; exit 2 ;;
  esac
done

# Credential patterns. Each entry is "label|extended regex". Keep these anchored
# on provider-specific shapes where possible; generic entropy rules produce more
# noise than they catch.
PATTERNS=(
  'GitHub personal access token|ghp_[A-Za-z0-9]{36}'
  'GitHub fine-grained token|github_pat_[A-Za-z0-9_]{22,}'
  'GitHub OAuth/app/server token|gh[osu]_[A-Za-z0-9]{36}'
  'ngrok authtoken|NGROK_AUTHTOKEN[[:space:]]*[=:][[:space:]]*[\"'"'"']?[0-9A-Za-z_]{30,}'
  'OpenAI API key|sk-[A-Za-z0-9]{32,}'
  'Anthropic API key|sk-ant-[A-Za-z0-9_-]{32,}'
  'AWS access key id|AKIA[0-9A-Z]{16}'
  'Slack token|xox[baprs]-[A-Za-z0-9-]{10,}'
  'Google API key|AIza[0-9A-Za-z_-]{35}'
  'Private key block|-----BEGIN[[:space:]]+([A-Z]+[[:space:]]+)?PRIVATE KEY-----'
  'Generic long secret assignment|(SECRET|PASSWORD|TOKEN|API_KEY|AUTHTOKEN|ACCESS_KEY)[A-Z_]*[[:space:]]*=[[:space:]]*[\"'"'"']?[A-Za-z0-9+_-][A-Za-z0-9/+_-]{23,}'
)

# Paths that must never appear inside a published image layer.
FORBIDDEN_IMAGE_PATHS=(
  '/.git'
  '/.env'
  '/root/.docker/config.json'
  '/root/.kube/config'
  '/root/.ssh'
)

# The allowlist is read once into memory. Re-reading it per candidate line was
# measurably worse and, combined with the per-file scanning below, crashed the
# shell outright on a repository of this size.
ALLOWLIST_PATTERNS=()
load_allowlist() {
  [[ -f "$ALLOWLIST" ]] || return 0
  local pattern
  while IFS= read -r pattern || [[ -n "$pattern" ]]; do
    [[ -z "$pattern" || "$pattern" == \#* ]] && continue
    ALLOWLIST_PATTERNS+=("$pattern")
  done <"$ALLOWLIST"
}

allowlisted() {
  local line="$1" pattern
  for pattern in ${ALLOWLIST_PATTERNS+"${ALLOWLIST_PATTERNS[@]}"}; do
    if printf '%s' "$line" | grep -qE -- "$pattern"; then
      return 0
    fi
  done
  return 1
}

findings=0

# scan_file_list greps the whole file list once per pattern rather than once per
# file per pattern. The per-file form spawned a process substitution for every
# file and every pattern -- roughly nineteen thousand subshells here -- which
# exhausted the shell and died with a bus error partway through the scan.
#
# -H forces the filename prefix: xargs may hand grep a batch containing a single
# file, and grep omits the name in that case, which would silently produce a
# finding with no path.
scan_file_list() {
  local base="$1" list_file="$2"
  local spec label regex hits hit
  hits="$TEMP_DIR/hits"

  for spec in "${PATTERNS[@]}"; do
    label="${spec%%|*}"
    regex="${spec#*|}"
    : >"$hits"
    ( cd "$base" && xargs -0 grep -nEIH -e "$regex" -- <"$list_file" ) >"$hits" 2>/dev/null || true
    while IFS= read -r hit; do
      [[ -z "$hit" ]] && continue
      if allowlisted "$hit"; then
        continue
      fi
      printf 'FINDING [%s] %s\n' "$label" "$hit" >&2
      findings=$((findings + 1))
    done <"$hits"
  done
}

scan_worktree() {
  echo "Scanning git-tracked files in $ROOT_DIR"
  ( cd "$ROOT_DIR" && git ls-files -z ) >"$TEMP_DIR/paths"
  scan_file_list "$ROOT_DIR" "$TEMP_DIR/paths"
}

scan_directory() {
  local dir="$1"
  echo "Scanning directory $dir"
  ( cd "$dir" && find . -type f -size -2M -print0 ) >"$TEMP_DIR/paths"
  scan_file_list "$dir" "$TEMP_DIR/paths"

  local forbidden
  for forbidden in "${FORBIDDEN_IMAGE_PATHS[@]}"; do
    if [[ -e "$dir$forbidden" ]]; then
      printf 'FINDING [forbidden path] %s present in image filesystem\n' "$forbidden" >&2
      findings=$((findings + 1))
    fi
  done
}

scan_image() {
  local image="$1"
  local workdir container
  workdir="$TEMP_DIR/rootfs"
  mkdir -p "$workdir"
  echo "Exporting image $image"
  container="$(docker create "$image")"
  docker export "$container" | tar -C "$workdir" -xf - 2>/dev/null || true
  docker rm -f "$container" >/dev/null
  scan_directory "$workdir"
}

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT
load_allowlist

if [[ -n "$SCAN_IMAGE" ]]; then
  scan_image "$SCAN_IMAGE"
elif [[ -n "$SCAN_PATH" ]]; then
  [[ -d "$SCAN_PATH" ]] || { echo "Not a directory: $SCAN_PATH" >&2; exit 2; }
  scan_directory "$SCAN_PATH"
else
  scan_worktree
fi

if [[ "$findings" -gt 0 ]]; then
  echo "" >&2
  echo "Secret scan failed with $findings finding(s)." >&2
  echo "Remove the credential, rotate it at the provider, and purge it from git history." >&2
  echo "Documentation examples can be allowlisted in ${ALLOWLIST#"$ROOT_DIR/"}." >&2
  exit 1
fi

echo "Secret scan clean."
