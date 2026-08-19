#!/usr/bin/env bash
# Restore the Git push credentials and identity for a step.
#
#   source "$TOOLKIT/scripts/git-env.sh"
#
# Only the workspace is shared between steps; environment variables are not.
# The checkout step leaves the askpass helper on disk, so every later step
# re-exports it from there instead of re-deriving credentials.
#
# Sourced, not executed: it exports into the calling shell.

dev_loop_git_env() {
  local git_dir askpass_path
  git_dir="$(git rev-parse --absolute-git-dir 2>/dev/null)" || {
    echo "dev-loop: not inside a git repository" >&2
    return 1
  }

  askpass_path="$git_dir/nopsai-git-askpass"
  if [[ ! -x "$askpass_path" ]]; then
    echo "dev-loop: git askpass helper is missing; run the checkout step first" >&2
    return 1
  fi

  export GIT_ASKPASS="$askpass_path"
  export GIT_TERMINAL_PROMPT=0

  git config user.name "${DEV_LOOP_GIT_AUTHOR_NAME:-NopsAI Development Loop}"
  git config user.email "${DEV_LOOP_GIT_AUTHOR_EMAIL:-dev-loop@nopsai.local}"
}

dev_loop_git_env
