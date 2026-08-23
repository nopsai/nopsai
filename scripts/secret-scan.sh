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
  'Generic long secret assignment|(SECRET|PASSWORD|TOKEN|API_KEY|AUTHTOKEN|ACCESS_KEY)[A-Z_]*[[:space:]]*=[[:space:]]*[\"'"'"']?[A-Za-z0-9/+_-]{24,}'
)

# Paths that must never appear inside a published image layer.
FORBIDDEN_IMAGE_PATHS=(
  '/.git'
  '/.env'
  '/root/.docker/config.json'
  '/root/.kube/config'
  '/root/.ssh'
)

allowlisted() {
  local line="$1"
  [[ -f "$ALLOWLIST" ]] || return 1
  local pattern
  while IFS= read -r pattern; do
    [[ -z "$pattern" || "$pattern" == \#* ]] && continue
    if printf '%s' "$line" | grep -qE -- "$pattern"; then
      return 0
    fi
  done <"$ALLOWLIST"
  return 1
}

findings=0

scan_stream() {
  # Reads NUL-separated file paths on stdin and greps each for every pattern.
  local base="$1"
  local entry label regex hit
  while IFS= read -r -d '' entry; do
    for spec in "${PATTERNS[@]}"; do
      label="${spec%%|*}"
      regex="${spec#*|}"
      while IFS= read -r hit; do
        [[ -z "$hit" ]] && continue
        if allowlisted "$base/$entry:$hit"; then
          continue
        fi
        printf 'FINDING [%s] %s:%s\n' "$label" "$entry" "$hit" >&2
        findings=$((findings + 1))
      done < <(grep -nEI -- "$regex" "$base/$entry" 2>/dev/null || true)
    done
  done
}

scan_worktree() {
  echo "Scanning git-tracked files in $ROOT_DIR"
  scan_stream "$ROOT_DIR" < <( cd "$ROOT_DIR" && git ls-files -z )
}

scan_directory() {
  local dir="$1"
  echo "Scanning directory $dir"
  scan_stream "$dir" < <( cd "$dir" && find . -type f -size -2M -print0 )

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
  workdir="$(mktemp -d)"
  trap 'rm -rf "$workdir"' RETURN
  echo "Exporting image $image"
  container="$(docker create "$image")"
  docker export "$container" | tar -C "$workdir" -xf - 2>/dev/null || true
  docker rm -f "$container" >/dev/null
  scan_directory "$workdir"
}

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
