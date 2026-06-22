#!/usr/bin/env bash
set -eo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${1:?usage: generate-changelog.sh VERSION OUTPUT [REF]}"
output="${2:?usage: generate-changelog.sh VERSION OUTPUT [REF]}"
ref="${3:-HEAD}"

previous_tag=""
while IFS= read -r candidate; do
  if [[ "$candidate" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ && "$candidate" != "$version" && "$candidate" != "v$version" ]]; then
    previous_tag="$candidate"
    break
  fi
done < <(git -C "$ROOT_DIR" tag --merged "$ref" --sort=-version:refname)

range="$ref"
comparison="Initial release history"
if [[ -n "$previous_tag" ]]; then
  range="$previous_tag..$ref"
  comparison="Changes since $previous_tag"
fi

declare -a added=()
declare -a fixed=()
declare -a changed=()
declare -a breaking=()

while IFS=$'\t' read -r short_hash subject; do
  [[ -n "$short_hash" ]] || continue
  subject="${subject//</&lt;}"
  subject="${subject//>/&gt;}"
  entry="- ${subject} (\`${short_hash}\`)"
  lowered="$(printf '%s' "$subject" | tr '[:upper:]' '[:lower:]')"
  if [[ "$subject" == *"!:"* || "$lowered" == *"breaking change"* ]]; then
    breaking+=("$entry")
  elif [[ "$lowered" =~ ^(feat|add|added|implement|implemented|init)(\(|:|[[:space:]]) ]]; then
    added+=("$entry")
  elif [[ "$lowered" =~ ^(fix|fixed|bugfix|hotfix)(\(|:|[[:space:]]) ]]; then
    fixed+=("$entry")
  else
    changed+=("$entry")
  fi
done < <(git -C "$ROOT_DIR" log "$range" --format=$'%h\t%s')

mkdir -p "$(dirname "$output")"
{
  printf '# NopsAI %s\n\n' "$version"
  printf '%s. Generated from repository history at `%s`.\n' "$comparison" "$(git -C "$ROOT_DIR" rev-parse --short=12 "$ref")"
  for category in Breaking Added Fixed Changed; do
    case "$category" in
      Breaking) entries=("${breaking[@]}") ;;
      Added) entries=("${added[@]}") ;;
      Fixed) entries=("${fixed[@]}") ;;
      Changed) entries=("${changed[@]}") ;;
    esac
    if ((${#entries[@]} > 0)); then
      printf '\n## %s\n\n' "$category"
      printf '%s\n' "${entries[@]}"
    fi
  done
  if ((${#breaking[@]} + ${#added[@]} + ${#fixed[@]} + ${#changed[@]} == 0)); then
    printf '\nNo repository changes were found for this release.\n'
  fi
} >"$output"
