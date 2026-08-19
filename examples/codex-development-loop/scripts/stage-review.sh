#!/usr/bin/env bash
# Reviewer stage 3: ask Codex to judge the change.
#
# Runs read-only, and the tree is checked afterwards, so a review cannot become
# a second implementation attempt.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
task_file="${DEV_LOOP_TASK_FILE:-development-task.md}"
agents_file="${DEV_LOOP_AGENTS_FILE:-AGENTS.md}"
base_branch="${DEV_LOOP_BASE_BRANCH:-main}"

if [[ ! -f "$agents_file" ]]; then
  echo "dev-loop: engineering rules file '$agents_file' is missing; the review cannot check compliance" >&2
  exit 1
fi

"$toolkit/scripts/render-prompt.sh" \
  "$toolkit/prompts/review.md" \
  .dev-loop/review-prompt.md \
  "TASK_ID=$DEV_LOOP_TASK_ID" \
  "TASK_TITLE=$DEV_LOOP_TASK_TITLE" \
  "TASK_FILE=$task_file" \
  "AGENTS_FILE=$agents_file" \
  "PLAN_PATH=$DEV_LOOP_TASK_PLAN_PATH" \
  "BRANCH=$DEV_LOOP_TASK_BRANCH" \
  "BASE_BRANCH=$base_branch" \
  "DIFF_FILE=.dev-loop/task.diff" \
  "VALIDATION_REPORT=.dev-loop/validation.md"

"$toolkit/scripts/run-codex.sh" \
  .dev-loop/review-prompt.md \
  .dev-loop/review-result.md \
  read-only

"$toolkit/scripts/parse-verdict.sh" .dev-loop/review-result.md >.dev-loop/codex-verdict
echo "codex verdict: $(cat .dev-loop/codex-verdict)"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "dev-loop: the review stage modified the working tree; reviews must not change code" >&2
  git status --porcelain >&2
  exit 1
fi
