#!/usr/bin/env bash
# Reviewer stage 6: start the next round, or stop.
#
# This is the only thing that keeps the loop going, and it is a plain check of
# the task file on the base branch. When nothing is left, nothing is called.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
# shellcheck source=/dev/null
source .dev-loop/next-task.env

if [[ "$DEV_LOOP_ALL_TASKS_DONE" == "true" ]]; then
  echo "ALL_TASKS_DONE: every task is complete. The loop ends here."
  exit 0
fi

base_branch="${DEV_LOOP_BASE_BRANCH:-main}"
state_sha="$(git rev-parse HEAD)"

# Keyed on the base-branch commit that closed the previous task, so a retry of
# this step re-attaches to the run it already started.
idempotency_key="dev-loop.next:${DEV_LOOP_REPOSITORY_URL}:${state_sha}"

echo "${DEV_LOOP_TASK_REMAINING} task(s) remain; starting the next round"

"$toolkit/scripts/invoke-trigger.sh" \
  "${DEV_LOOP_RUNNER_TRIGGER_ID:-development-task-runner}" \
  "dev-loop.task.completed" \
  "$idempotency_key" \
  "repository_url=$DEV_LOOP_REPOSITORY_URL" \
  "base_branch=$base_branch"
