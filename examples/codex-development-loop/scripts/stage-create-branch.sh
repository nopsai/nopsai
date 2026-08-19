#!/usr/bin/env bash
# Runner stage 2: put the task on its own branch.
#
# Every task gets a branch cut from the base branch, so tasks stay independent
# and the base branch only ever gains work that has passed review.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
# shellcheck source=stage-lib.sh
source "$toolkit/scripts/stage-lib.sh"

if dev_loop_stage_idle; then
  echo "dev-loop: no pending task; skipping"
  exit 0
fi
dev_loop_stage_load_task

base_branch="${DEV_LOOP_BASE_BRANCH:-main}"
branch="$DEV_LOOP_TASK_BRANCH"

# An existing branch means a run for this task is already in flight, or an
# earlier one failed. Continuing on it silently would build on work nobody has
# reviewed, so it takes an explicit opt-in.
if git ls-remote --exit-code --heads origin "$branch" >/dev/null 2>&1; then
  if [[ "${DEV_LOOP_ALLOW_EXISTING_BRANCH:-false}" != "true" ]]; then
    echo "dev-loop: branch '$branch' already exists on origin." >&2
    echo "dev-loop: delete it, or set DEV_LOOP_ALLOW_EXISTING_BRANCH=true to continue on it." >&2
    exit 1
  fi
  echo "dev-loop: continuing on existing branch '$branch'"
  git fetch --quiet origin "$branch"
  git checkout -q -B "$branch" "origin/$branch"
else
  git checkout -q -B "$branch" "origin/$base_branch"
  git push --quiet --set-upstream origin "$branch"
fi

echo "branch=$branch"
echo "base=$base_branch"
