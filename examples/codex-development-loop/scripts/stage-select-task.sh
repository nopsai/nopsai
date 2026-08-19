#!/usr/bin/env bash
# Runner stage 1: choose the task this run will work on.
#
# Writes .dev-loop/task.env with the task's identity, branch, and plan path, or
# .dev-loop/all-tasks-done when the queue is empty. Later stages read that file
# rather than re-deriving anything, so one run works on exactly one task.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
task_file="${DEV_LOOP_TASK_FILE:-development-task.md}"

if [[ ! -d "$toolkit/scripts" ]]; then
  echo "dev-loop: toolkit directory '$toolkit' is missing from the repository" >&2
  exit 1
fi

"$toolkit/scripts/find-next-task.sh" "$task_file" .dev-loop/task.env

# shellcheck source=/dev/null
source .dev-loop/task.env

if [[ "$DEV_LOOP_ALL_TASKS_DONE" == "true" ]]; then
  touch .dev-loop/all-tasks-done
  echo "ALL_TASKS_DONE: every task in $task_file is complete"
  exit 0
fi

branch_prefix="${DEV_LOOP_BRANCH_PREFIX:-nopsai/task}"
plan_dir="${DEV_LOOP_PLAN_DIR:-.nopsai/plans}"

{
  echo "DEV_LOOP_TASK_BRANCH=${branch_prefix}/${DEV_LOOP_TASK_ID}-${DEV_LOOP_TASK_SLUG}"
  echo "DEV_LOOP_TASK_PLAN_PATH=${plan_dir}/${DEV_LOOP_TASK_ID}-${DEV_LOOP_TASK_SLUG}.md"
} >>.dev-loop/task.env

echo "Selected task ${DEV_LOOP_TASK_ID}: ${DEV_LOOP_TASK_TITLE}"
echo "${DEV_LOOP_TASK_REMAINING} of ${DEV_LOOP_TASK_TOTAL} tasks remain"
