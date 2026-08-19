#!/usr/bin/env bash
# Runner stage 5: hand the branch to the review pipeline.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
# shellcheck source=stage-lib.sh
source "$toolkit/scripts/stage-lib.sh"

if dev_loop_stage_idle; then
  echo "dev-loop: no task was implemented; the loop ends here"
  exit 0
fi

# shellcheck source=/dev/null
source .dev-loop/task.env

commit_sha="$(cat .dev-loop/commit-sha)"
base_branch="${DEV_LOOP_BASE_BRANCH:-main}"

# Derived from the repository, task, and commit, so a retried step re-attaches
# to the review already running instead of starting a second one.
idempotency_key="dev-loop.review:${DEV_LOOP_REPOSITORY_URL}:${DEV_LOOP_TASK_ID}:${commit_sha}"

"$toolkit/scripts/invoke-trigger.sh" \
  "${DEV_LOOP_REVIEWER_TRIGGER_ID:-development-task-reviewer}" \
  "dev-loop.implementation.completed" \
  "$idempotency_key" \
  "repository_url=$DEV_LOOP_REPOSITORY_URL" \
  "base_branch=$base_branch" \
  "task_branch=$DEV_LOOP_TASK_BRANCH" \
  "task_id=$DEV_LOOP_TASK_ID" \
  "task_number=$DEV_LOOP_TASK_NUMBER" \
  "task_title=$DEV_LOOP_TASK_TITLE" \
  "task_slug=$DEV_LOOP_TASK_SLUG" \
  "plan_path=$DEV_LOOP_TASK_PLAN_PATH" \
  "commit_sha=$commit_sha"
