#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: sign-notarize-macos-cli.sh --binary PATH --archive PATH

Sign a macOS CLI binary with Developer ID, archive it, and submit the archive
to Apple's notarization service. The required credentials are read from:

  APPLE_DEVELOPER_ID_P12_BASE64
  APPLE_DEVELOPER_ID_P12_PASSWORD
  APPLE_DEVELOPER_ID_IDENTITY
  APPLE_NOTARY_KEY_P8_BASE64
  APPLE_NOTARY_KEY_ID
  APPLE_NOTARY_ISSUER_ID
EOF
}

binary=""
archive=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --binary)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--binary requires a path' >&2
        exit 2
      fi
      binary="${2:-}"
      shift 2
      ;;
    --archive)
      if [[ $# -lt 2 ]]; then
        printf '%s\n' '--archive requires a path' >&2
        exit 2
      fi
      archive="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "$binary" || -z "$archive" ]]; then
  printf '%s\n' '--binary and --archive are required' >&2
  usage >&2
  exit 2
fi
if [[ ! -f "$binary" ]]; then
  printf 'CLI binary does not exist: %s\n' "$binary" >&2
  exit 1
fi
if [[ "$(uname -s)" != "Darwin" ]]; then
  printf '%s\n' 'macOS CLI signing must run on a macOS runner' >&2
  exit 1
fi

required_variables=(
  APPLE_DEVELOPER_ID_P12_BASE64
  APPLE_DEVELOPER_ID_P12_PASSWORD
  APPLE_DEVELOPER_ID_IDENTITY
  APPLE_NOTARY_KEY_P8_BASE64
  APPLE_NOTARY_KEY_ID
  APPLE_NOTARY_ISSUER_ID
)
for variable in "${required_variables[@]}"; do
  if [[ -z "${!variable:-}" ]]; then
    printf 'required signing variable is empty: %s\n' "$variable" >&2
    exit 1
  fi
done

for command in security codesign ditto xcrun openssl spctl; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'required signing command is unavailable: %s\n' "$command" >&2
    exit 1
  fi
done

work_dir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/nopsai-signing.XXXXXX")"
certificate="$work_dir/developer-id.p12"
notary_key="$work_dir/notary-key.p8"
keychain="$work_dir/signing.keychain-db"
keychain_password="$(openssl rand -hex 24)"

cleanup() {
  security delete-keychain "$keychain" >/dev/null 2>&1 || true
  rm -rf "$work_dir"
}
trap cleanup EXIT

printf '%s' "$APPLE_DEVELOPER_ID_P12_BASE64" | /usr/bin/base64 -D >"$certificate"
printf '%s' "$APPLE_NOTARY_KEY_P8_BASE64" | /usr/bin/base64 -D >"$notary_key"
chmod 600 "$certificate" "$notary_key"

security create-keychain -p "$keychain_password" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$keychain_password" "$keychain"
security import "$certificate" \
  -k "$keychain" \
  -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" \
  -T /usr/bin/codesign
security set-key-partition-list \
  -S apple-tool:,apple: \
  -s \
  -k "$keychain_password" \
  "$keychain"

codesign \
  --force \
  --options runtime \
  --timestamp \
  --keychain "$keychain" \
  --sign "$APPLE_DEVELOPER_ID_IDENTITY" \
  "$binary"
codesign --verify --strict --verbose=2 "$binary"

mkdir -p "$(dirname "$archive")"
ditto -c -k --keepParent "$binary" "$archive"
xcrun notarytool submit "$archive" \
  --key "$notary_key" \
  --key-id "$APPLE_NOTARY_KEY_ID" \
  --issuer "$APPLE_NOTARY_ISSUER_ID" \
  --wait

# Gatekeeper assessment verifies that the notarized executable is accepted.
spctl --assess --type execute --verbose=2 "$binary"
