#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
values=""
output=""
version=""

while (($# > 0)); do
  case "$1" in
    --values) values="${2:?missing --values value}"; shift 2 ;;
    --output) output="${2:?missing --output value}"; shift 2 ;;
    --version) version="${2:?missing --version value}"; shift 2 ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
done

for value_name in values output version; do
  if [[ -z "${!value_name}" ]]; then
    printf '%s is required\n' "$value_name" >&2
    exit 2
  fi
done
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'version must use major.minor.patch\n' >&2
  exit 2
fi
for tool in helm ruby; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    printf '%s is required to package the Helm chart\n' "$tool" >&2
    exit 1
  fi
done
if [[ ! -f "$values" ]]; then
  printf 'release values file does not exist: %s\n' "$values" >&2
  exit 1
fi

mkdir -p "$output"
temp_dir="$(mktemp -d)"
trap 'rm -rf "$temp_dir"' EXIT
chart_dir="$temp_dir/nopsai"
cp -R "$ROOT_DIR/deploy/helm/nopsai" "$chart_dir"

ruby -ryaml - "$chart_dir/values.yaml" "$values" <<'RUBY'
def deep_merge(base, overlay)
  base.merge(overlay) do |_key, left, right|
    left.is_a?(Hash) && right.is_a?(Hash) ? deep_merge(left, right) : right
  end
end

chart_values_path, release_values_path = ARGV
chart_values = YAML.load_file(chart_values_path)
release_values = YAML.load_file(release_values_path)
File.write(chart_values_path, deep_merge(chart_values, release_values).to_yaml)
RUBY

helm lint "$chart_dir" >/dev/null
helm template nopsai "$chart_dir" \
  --namespace nopsai \
  --version "$version" \
  --set-string "global.releaseVersion=$version" >/dev/null
helm package "$chart_dir" \
  --version "$version" \
  --app-version "$version" \
  --destination "$output" >/dev/null

chart_file="$output/nopsai-$version.tgz"
if [[ ! -s "$chart_file" ]]; then
  printf 'Helm did not create expected package: %s\n' "$chart_file" >&2
  exit 1
fi
printf '%s\n' "$chart_file"
