#!/usr/bin/env bash
# Runner stage 3: write the plan for the task.
#
# Codex runs with write access because it has to create the plan file, so the
# check afterwards is what actually constrains it: if anything but the plan
# changed, the run stops. The rule is enforced by Git, not by the prompt.
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

if [[ ! -f "$agents_file" ]]; then
  echo "dev-loop: engineering rules file '$agents_file' is missing; refusing to plan against unknown rules" >&2
  exit 1
fi

mkdir -p "$(dirname "$DEV_LOOP_TASK_PLAN_PATH")"

"$toolkit/scripts/render-prompt.sh" \
  "$toolkit/prompts/planning.md" \
  .dev-loop/planning-prompt.md \
  "TASK_ID=$DEV_LOOP_TASK_ID" \
  "TASK_TITLE=$DEV_LOOP_TASK_TITLE" \
  "TASK_FILE=$task_file" \
  "AGENTS_FILE=$agents_file" \
  "PLAN_PATH=$DEV_LOOP_TASK_PLAN_PATH"

"$toolkit/scripts/run-codex.sh" \
  .dev-loop/planning-prompt.md \
  .dev-loop/planning-result.md \
  workspace-write

if [[ ! -s "$DEV_LOOP_TASK_PLAN_PATH" ]]; then
  echo "dev-loop: the planning stage did not write '$DEV_LOOP_TASK_PLAN_PATH'" >&2
  exit 1
fi

unexpected="$(git status --porcelain -- . ":(exclude)$DEV_LOOP_TASK_PLAN_PATH")"
if [[ -n "$unexpected" ]]; then
  echo "dev-loop: the planning stage modified files other than the plan:" >&2
  echo "$unexpected" >&2
  exit 1
fi

git add -- "$DEV_LOOP_TASK_PLAN_PATH"
git commit --quiet -m "Plan task ${DEV_LOOP_TASK_ID}: ${DEV_LOOP_TASK_TITLE}"
git push --quiet origin "$DEV_LOOP_TASK_BRANCH"

echo "plan=$DEV_LOOP_TASK_PLAN_PATH"
