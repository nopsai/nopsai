#!/usr/bin/env bash
# Mark one task complete in the task file.
#
# Usage:
#   mark-task-done.sh <task-file> <task-number>
#
# Flips '<n>- [ ]' to '<n>- [x]' for exactly one permanent task number and
# leaves every other character of the file untouched. Numbers are never
# reassigned, so identifiers stay stable across the life of the repository.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=task-lib.sh
source "$script_dir/task-lib.sh"

task_file="${1:-}"
task_number="${2:-}"

[[ -n "$task_file" ]] || dev_loop_die "usage: mark-task-done.sh <task-file> <task-number>"
[[ -n "$task_number" ]] || dev_loop_die "usage: mark-task-done.sh <task-file> <task-number>"
[[ "$task_number" =~ ^[0-9]+$ ]] || dev_loop_die "task number '$task_number' must be numeric"

dev_loop_require_task_file "$task_file"

target="$((10#$task_number))"
tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT

matched=0
already_done=0

while IFS= read -r line || [[ -n "$line" ]]; do
  if [[ "$line" =~ $DEV_LOOP_TASK_PATTERN ]]; then
    number="$((10#${BASH_REMATCH[1]}))"
    state="${BASH_REMATCH[2]}"
    if [[ "$number" -eq "$target" ]]; then
      matched=$((matched + 1))
      if [[ "$state" == " " ]]; then
        # Replace only the first checkbox on the line so descriptions that
        # themselves contain brackets are preserved verbatim.
        line="${line/\[ \]/[x]}"
      else
        already_done=1
      fi
    fi
  fi
  printf '%s\n' "$line" >>"$tmp_file"
done <"$task_file"

[[ $matched -gt 0 ]] || dev_loop_die "task $task_number was not found in '$task_file'"
[[ $matched -eq 1 ]] || dev_loop_die "task $task_number appears $matched times in '$task_file'; task numbers must be unique"

cat "$tmp_file" >"$task_file"

if [[ $already_done -eq 1 ]]; then
  echo "TASK_ALREADY_DONE $task_number"
else
  echo "TASK_MARKED_DONE $task_number"
fi
