#!/usr/bin/env bash
# Select the next task to work on.
#
# Usage:
#   find-next-task.sh <task-file> <output-env-file>
#
# Writes a shell-sourceable env file describing the first unchecked task, or
# DEV_LOOP_ALL_TASKS_DONE=true when every task is complete. No model is
# involved: which task runs next is a deterministic property of the task file.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=task-lib.sh
source "$script_dir/task-lib.sh"

task_file="${1:-}"
output_file="${2:-}"

[[ -n "$task_file" ]] || dev_loop_die "usage: find-next-task.sh <task-file> <output-env-file>"
[[ -n "$output_file" ]] || dev_loop_die "usage: find-next-task.sh <task-file> <output-env-file>"

dev_loop_require_task_file "$task_file"

total=0
remaining=0
found_number=""
found_title=""

while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" =~ $DEV_LOOP_TASK_PATTERN ]]; then
    number="${BASH_REMATCH[1]}"
    state="${BASH_REMATCH[2]}"
    title="${BASH_REMATCH[3]}"
    total=$((total + 1))
    if [[ "$state" == " " ]]; then
      remaining=$((remaining + 1))
      if [[ -z "$found_number" ]]; then
        found_number="$number"
        found_title="$title"
      fi
    fi
  fi
done <"$task_file"

[[ $total -gt 0 ]] || dev_loop_die "task file '$task_file' contains no task lines; expected lines like '1- [ ] Description'"

mkdir -p "$(dirname "$output_file")"

if [[ -z "$found_number" ]]; then
  {
    echo "DEV_LOOP_ALL_TASKS_DONE=true"
    echo "DEV_LOOP_TASK_TOTAL=$total"
    echo "DEV_LOOP_TASK_REMAINING=0"
  } >"$output_file"
  echo "ALL_TASKS_DONE"
  exit 0
fi

# A title is the instruction handed to the planner, so an empty one is a
# malformed task rather than an empty assignment.
title="$(printf '%s' "$found_title" | sed -e 's/[[:space:]]*$//')"
[[ -n "$title" ]] || dev_loop_die "task $found_number in '$task_file' has no description"

task_id="$(dev_loop_task_id "$found_number")"
slug="$(dev_loop_slugify "$title")"

{
  echo "DEV_LOOP_ALL_TASKS_DONE=false"
  echo "DEV_LOOP_TASK_NUMBER=$found_number"
  echo "DEV_LOOP_TASK_ID=$task_id"
  echo "DEV_LOOP_TASK_TITLE=$(dev_loop_shell_quote "$title")"
  echo "DEV_LOOP_TASK_SLUG=$(dev_loop_shell_quote "$slug")"
  echo "DEV_LOOP_TASK_TOTAL=$total"
  echo "DEV_LOOP_TASK_REMAINING=$remaining"
} >"$output_file"

echo "TASK_SELECTED $task_id $slug"
