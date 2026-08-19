#!/usr/bin/env bash
# Single entry point for every Codex CLI invocation in the loop.
#
# Usage:
#   run-codex.sh <prompt-file> <last-message-file> [sandbox-mode]
#
# sandbox-mode defaults to workspace-write. Planning and review pass read-only
# so those stages cannot touch the tree they are reasoning about.
#
# Keeping all three stages behind one script means the CLI contract - auth,
# flags, model selection, transcript capture - is defined once and can be
# adjusted for a new Codex release without editing the pipelines.
set -euo pipefail

prompt_file="${1:-}"
last_message_file="${2:-}"
sandbox_mode="${3:-workspace-write}"

die() {
  echo "dev-loop: $*" >&2
  exit 1
}

[[ -n "$prompt_file" ]] || die "usage: run-codex.sh <prompt-file> <last-message-file> [sandbox-mode]"
[[ -n "$last_message_file" ]] || die "usage: run-codex.sh <prompt-file> <last-message-file> [sandbox-mode]"
[[ -f "$prompt_file" ]] || die "prompt file '$prompt_file' does not exist"

command -v codex >/dev/null 2>&1 || die "the codex CLI is not installed in this image"

# NopsAI resolves the credential into DEV_LOOP_CODEX_API_KEY. The CLI reads
# OPENAI_API_KEY, so map it here rather than declaring the vendor's variable
# name as the pipeline secret.
if [[ -n "${DEV_LOOP_CODEX_API_KEY:-}" ]]; then
  export OPENAI_API_KEY="$DEV_LOOP_CODEX_API_KEY"
fi
[[ -n "${OPENAI_API_KEY:-}" ]] || die "no Codex credential is available; declare the DEV_LOOP_CODEX_API_KEY secret"

mkdir -p "$(dirname "$last_message_file")"
: >"$last_message_file"

transcript_file="${last_message_file}.transcript"
: >"$transcript_file"

declare -a codex_args=(exec)

if [[ -n "${DEV_LOOP_CODEX_MODEL:-}" ]]; then
  codex_args+=(--model "$DEV_LOOP_CODEX_MODEL")
fi

codex_args+=(--sandbox "$sandbox_mode")

# The container is already the isolation boundary, and there is no operator at
# a terminal to answer prompts, so approvals are disabled inside it.
if codex exec --help 2>&1 | grep -q -- '--ask-for-approval'; then
  codex_args+=(--ask-for-approval never)
fi

# Preferred way to capture the model's conclusion. Older CLI builds without the
# flag fall back to the transcript below.
supports_last_message=0
if codex exec --help 2>&1 | grep -q -- '--output-last-message'; then
  supports_last_message=1
  codex_args+=(--output-last-message "$last_message_file")
fi

if [[ -n "${DEV_LOOP_CODEX_EXEC_ARGS:-}" ]]; then
  # shellcheck disable=SC2206 # deliberate word splitting of operator-supplied flags
  extra_args=(${DEV_LOOP_CODEX_EXEC_ARGS})
  codex_args+=("${extra_args[@]}")
fi

echo "dev-loop: running codex ${codex_args[*]}" >&2

status=0
codex "${codex_args[@]}" - <"$prompt_file" 2>&1 | tee "$transcript_file" || status="${PIPESTATUS[0]}"

if [[ $status -ne 0 ]]; then
  die "codex exited with status $status; see $transcript_file"
fi

if [[ $supports_last_message -eq 0 || ! -s "$last_message_file" ]]; then
  cp "$transcript_file" "$last_message_file"
fi

[[ -s "$last_message_file" ]] || die "codex produced no output for '$prompt_file'"
