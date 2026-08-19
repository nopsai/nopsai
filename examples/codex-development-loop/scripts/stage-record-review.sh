#!/usr/bin/env bash
# Reviewer stage 4: decide the verdict of record and write it down.
#
# The verdict is the conjunction of two independent signals - the repository's
# own build and tests, and the Codex review. PASS needs both; anything else is a
# FAIL. The review is committed either way, so a failed attempt leaves its
# reasons next to the code that caused them.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
# shellcheck source=/dev/null
source "$toolkit/scripts/git-env.sh"

review_dir="${DEV_LOOP_REVIEW_DIR:-.nopsai/reviews}"
review_path="${review_dir}/${DEV_LOOP_TASK_ID}-${DEV_LOOP_TASK_SLUG}.md"
validation_status="$(cat .dev-loop/validation-status)"
codex_verdict="$(cat .dev-loop/codex-verdict)"

if [[ "$validation_status" == "0" && "$codex_verdict" == "PASS" ]]; then
  verdict="PASS"
else
  verdict="FAIL"
fi
echo "$verdict" >.dev-loop/verdict

mkdir -p "$review_dir"
{
  echo "# Review of task ${DEV_LOOP_TASK_ID}: ${DEV_LOOP_TASK_TITLE}"
  echo
  echo "$verdict"
  echo
  echo "- Branch: \`${DEV_LOOP_TASK_BRANCH}\`"
  echo "- Commit: \`$(git rev-parse HEAD)\`"
  echo "- Plan: \`${DEV_LOOP_TASK_PLAN_PATH}\`"
  if [[ "$validation_status" == "0" ]]; then
    echo "- Repository validation: passed"
  else
    echo "- Repository validation: failed (exit ${validation_status})"
  fi
  echo "- Codex verdict: ${codex_verdict}"
  echo
  echo "## Changes"
  echo
  echo '```'
  cat .dev-loop/task.diffstat
  echo '```'
  echo
  echo "## Review"
  echo
  cat .dev-loop/review-result.md
  echo
  echo "## Repository validation"
  echo
  cat .dev-loop/validation.md
} >"$review_path"

git add -- "$review_path"
git commit --quiet -m "Record ${verdict} review for task ${DEV_LOOP_TASK_ID}"
git push --quiet origin "$DEV_LOOP_TASK_BRANCH"

# Kept outside the tree so they survive the switch to the base branch.
cp "$review_path" .dev-loop/review-record.md
cp "$DEV_LOOP_TASK_PLAN_PATH" .dev-loop/plan-record.md
echo "$review_path" >.dev-loop/review-path

if [[ "$verdict" != "PASS" ]]; then
  echo "dev-loop: review FAILED for task ${DEV_LOOP_TASK_ID}." >&2
  echo "dev-loop: the review is recorded on '${DEV_LOOP_TASK_BRANCH}' at ${review_path}." >&2
  echo "dev-loop: the task stays unchecked and the loop stops here." >&2
  exit 1
fi

echo "Review PASSED for task ${DEV_LOOP_TASK_ID}"
