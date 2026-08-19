#!/usr/bin/env bash
# Reviewer stage 1: gather everything the review needs, without a model.
#
# The diff, the commit log, and the tree state are facts. Collecting them here
# means the review stage reasons about a fixed, recorded snapshot rather than
# whatever it happens to find in the workspace.
set -euo pipefail

base_branch="${DEV_LOOP_BASE_BRANCH:-main}"
task_branch="${DEV_LOOP_TASK_BRANCH:?DEV_LOOP_TASK_BRANCH is required}"
plan_path="${DEV_LOOP_TASK_PLAN_PATH:?DEV_LOOP_TASK_PLAN_PATH is required}"

head_sha="$(git rev-parse HEAD)"
if [[ -n "${DEV_LOOP_COMMIT_SHA:-}" && "$DEV_LOOP_COMMIT_SHA" != "$head_sha" ]]; then
  echo "dev-loop: branch '$task_branch' is at $head_sha but the review was requested for ${DEV_LOOP_COMMIT_SHA}." >&2
  echo "dev-loop: refusing to review code other than the commit that was submitted." >&2
  exit 1
fi

if [[ ! -s "$plan_path" ]]; then
  echo "dev-loop: plan '$plan_path' is missing from the task branch; there is nothing to review against" >&2
  exit 1
fi

mkdir -p .dev-loop

# Three dots: the work this branch added since it left the base branch, not the
# unrelated commits the base branch has picked up since.
git diff "origin/${base_branch}...HEAD" >.dev-loop/task.diff
git diff --stat "origin/${base_branch}...HEAD" >.dev-loop/task.diffstat
git log --oneline "origin/${base_branch}..HEAD" >.dev-loop/task.log
git status --porcelain >.dev-loop/task.status

if [[ ! -s .dev-loop/task.diff ]]; then
  echo "dev-loop: '$task_branch' contains no changes against '$base_branch'" >&2
  exit 1
fi

if [[ -s .dev-loop/task.status ]]; then
  echo "dev-loop: the task branch has uncommitted changes:" >&2
  cat .dev-loop/task.status >&2
  exit 1
fi

echo "Reviewing $task_branch at $head_sha"
cat .dev-loop/task.diffstat
