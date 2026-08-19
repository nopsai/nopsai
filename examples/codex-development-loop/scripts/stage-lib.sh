#!/usr/bin/env bash
# Shared preamble for the runner and reviewer stages.
#
# Sourced by the stage scripts that run after a task has been selected.

# dev_loop_stage_idle
#
# True when this run has no task to work on. Runner stages check it first and
# exit cleanly, so an empty queue ends the run successfully rather than failing
# every remaining stage.
dev_loop_stage_idle() {
  [[ -f .dev-loop/all-tasks-done ]]
}

# dev_loop_stage_load_task
#
# Loads the selected task and restores the Git credentials and identity. Only
# the workspace survives between steps, so every stage re-establishes both.
dev_loop_stage_load_task() {
  local toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"

  if [[ ! -f .dev-loop/task.env ]]; then
    echo "dev-loop: .dev-loop/task.env is missing; the task selection stage did not run" >&2
    return 1
  fi

  # shellcheck source=/dev/null
  source .dev-loop/task.env
  # shellcheck source=/dev/null
  source "$toolkit/scripts/git-env.sh"
}
