#!/usr/bin/env bash
# Runner stage 4: implement the task and push it.
#
# Codex writes the code; this stage owns Git and the two guards that keep the
# result reviewable - the task file must be untouched, and there must actually
# be a change to review.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"
# shellcheck source=stage-lib.sh
source "$toolkit/scripts/stage-lib.sh"

if dev_loop_stage_idle; then
  echo "dev-loop: no pending task; skipping"
  exit 0
fi
dev_loop_stage_load_task

task_file="${DEV_LOOP_TASK_FILE:-development-task.md}"
agents_file="${DEV_LOOP_AGENTS_FILE:-AGENTS.md}"
base_branch="${DEV_LOOP_BASE_BRANCH:-main}"

"$toolkit/scripts/render-prompt.sh" \
  "$toolkit/prompts/implementation.md" \
  .dev-loop/implementation-prompt.md \
  "TASK_ID=$DEV_LOOP_TASK_ID" \
  "TASK_TITLE=$DEV_LOOP_TASK_TITLE" \
  "TASK_FILE=$task_file" \
  "AGENTS_FILE=$agents_file" \
  "PLAN_PATH=$DEV_LOOP_TASK_PLAN_PATH" \
  "BRANCH=$DEV_LOOP_TASK_BRANCH" \
  "BASE_BRANCH=$base_branch"

"$toolkit/scripts/run-codex.sh" \
  .dev-loop/implementation-prompt.md \
  .dev-loop/implementation-result.md \
  workspace-write

# Task state belongs to the review stage on the base branch. An implementation
# that edits it would be marking its own homework. --porcelain rather than
# 'git diff' so a staged edit is caught too.
if [[ -n "$(git status --porcelain -- "$task_file")" ]]; then
  echo "dev-loop: the implementation stage modified '$task_file'; task state is owned by the review stage" >&2
  git --no-pager diff HEAD -- "$task_file" >&2
  exit 1
fi

if [[ -z "$(git status --porcelain)" ]]; then
  echo "dev-loop: the implementation stage produced no changes" >&2
  exit 1
fi

git add -A
git commit --quiet -m "Implement task ${DEV_LOOP_TASK_ID}: ${DEV_LOOP_TASK_TITLE}"
git push --quiet origin "$DEV_LOOP_TASK_BRANCH"

git rev-parse HEAD >.dev-loop/commit-sha
echo "commit=$(cat .dev-loop/commit-sha)"
