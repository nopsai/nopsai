#!/usr/bin/env bash
# Reviewer stage 5: record the task complete on the base branch.
#
# Reached only after a PASS. The next round branches from the base branch, so
# task state has to land there for the loop to see an accurate picture of what
# is left. The code itself stays on its branch unless DEV_LOOP_ON_PASS=merge.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
# shellcheck source=/dev/null
source "$toolkit/scripts/git-env.sh"

base_branch="${DEV_LOOP_BASE_BRANCH:-main}"
task_file="${DEV_LOOP_TASK_FILE:-development-task.md}"
review_path="$(cat .dev-loop/review-path)"
plan_path="$DEV_LOOP_TASK_PLAN_PATH"

git fetch --quiet origin "$base_branch"
git checkout -q -B "$base_branch" "origin/$base_branch"

on_pass="${DEV_LOOP_ON_PASS:-state-only}"
case "$on_pass" in
  state-only)
    ;;
  merge)
    if ! git merge --quiet --no-ff "origin/${DEV_LOOP_TASK_BRANCH}" \
        -m "Merge task ${DEV_LOOP_TASK_ID}: ${DEV_LOOP_TASK_TITLE}"; then
      git merge --abort || true
      echo "dev-loop: merging '${DEV_LOOP_TASK_BRANCH}' into '${base_branch}' conflicts." >&2
      echo "dev-loop: the task is not marked done; resolve the merge by hand." >&2
      exit 1
    fi
    ;;
  *)
    echo "dev-loop: DEV_LOOP_ON_PASS must be 'state-only' or 'merge', got '$on_pass'" >&2
    exit 1
    ;;
esac

mkdir -p "$(dirname "$plan_path")" "$(dirname "$review_path")"
cp .dev-loop/plan-record.md "$plan_path"
cp .dev-loop/review-record.md "$review_path"

"$toolkit/scripts/mark-task-done.sh" "$task_file" "$DEV_LOOP_TASK_NUMBER"

git add -- "$task_file" "$plan_path" "$review_path"
git commit --quiet -m "Mark task ${DEV_LOOP_TASK_ID} complete: ${DEV_LOOP_TASK_TITLE}"
git push --quiet origin "$base_branch"

"$toolkit/scripts/find-next-task.sh" "$task_file" .dev-loop/next-task.env
echo "Task ${DEV_LOOP_TASK_ID} is recorded complete on ${base_branch}"
