#!/usr/bin/env bash
# Fill a prompt template with the concrete details of one task.
#
# Usage:
#   render-prompt.sh <template-file> <output-file> KEY=VALUE [KEY=VALUE ...]
#
# Replaces every {{KEY}} placeholder with its value using plain string
# substitution - no eval, no shell expansion of the task text, so a task
# description can contain any characters at all.
#
# Fails closed: if any {{PLACEHOLDER}} is still present after substitution the
# script errors instead of sending a half-written instruction to the model.
set -euo pipefail

template_file="${1:-}"
output_file="${2:-}"
shift 2 || true

die() {
  echo "dev-loop: $*" >&2
  exit 1
}

[[ -n "$template_file" ]] || die "usage: render-prompt.sh <template-file> <output-file> KEY=VALUE ..."
[[ -n "$output_file" ]] || die "usage: render-prompt.sh <template-file> <output-file> KEY=VALUE ..."
[[ -f "$template_file" ]] || die "prompt template '$template_file' does not exist"

content="$(cat "$template_file")"

for pair in "$@"; do
  [[ "$pair" == *=* ]] || die "substitution '$pair' must be in KEY=VALUE form"
  key="${pair%%=*}"
  value="${pair#*=}"
  [[ "$key" =~ ^[A-Z0-9_]+$ ]] || die "substitution key '$key' must match ^[A-Z0-9_]+$"
  content="${content//\{\{$key\}\}/$value}"
done

if [[ "$content" =~ \{\{[A-Z0-9_]+\}\} ]]; then
  die "prompt '$template_file' still contains the unfilled placeholder ${BASH_REMATCH[0]}"
fi

mkdir -p "$(dirname "$output_file")"
printf '%s\n' "$content" >"$output_file"
