#!/usr/bin/env bash
# End-to-end test of the development loop's stage scripts.
#
#   examples/codex-development-loop/tests/run-loop-integration-test.sh
#
# Drives the real stage scripts against a real Git repository, with a fake Codex
# CLI standing in for the model. Everything else - the repository, the origin,
# the toolkit, the task file - is built here, so the test depends on no external
# repository, service, or credential.
#
# The two stages that call the NopsAI API are not exercised; they need a running
# platform. Everything that decides what happens is.
set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
example_dir="$(cd "$script_dir/.." && pwd)"

work_root="$(mktemp -d)"
trap 'rm -rf "$work_root"' EXIT

passed=0
failed=0

pass() { passed=$((passed + 1)); echo "ok   - $1"; }
fail() { failed=$((failed + 1)); echo "FAIL - $1"; [[ -n "${2:-}" ]] && echo "       $2"; }

assert_equals() {
  if [[ "$2" == "$3" ]]; then pass "$1"; else fail "$1" "expected [$2], got [$3]"; fi
}

assert_contains() {
  if [[ "$2" == *"$3"* ]]; then pass "$1"; else fail "$1" "expected [$3] in [$2]"; fi
}

assert_not_contains() {
  if [[ "$2" != *"$3"* ]]; then pass "$1"; else fail "$1" "did not expect [$3] in [$2]"; fi
}

# The checkout step runs before the repository exists in the workspace, so its
# script lives inline in the step manifest rather than in the toolkit. Extract
# and run the shipped one instead of reimplementing it here, so the test covers
# what actually runs.
extract_checkout_script() {
  # The block scalar under 'script:' is indented by two spaces and ends at the
  # next top-level key, so awk can lift it out without a YAML library.
  awk '
    /^script: \|/ { in_block = 1; next }
    in_block {
      if ($0 ~ /^[^[:space:]]/) { in_block = 0; next }
      sub(/^  /, "")
      print
    }
  ' "$example_dir/steps/platform/shared/dev-loop-checkout.yaml" >"$1"

  if [[ ! -s "$1" ]]; then
    echo "could not extract the checkout script from the step manifest" >&2
    return 1
  fi
}

# --------------------------------------------------------------------------
# Fixture: a bare origin, a repository with the toolkit installed, three tasks.
# --------------------------------------------------------------------------

origin="$work_root/origin.git"
git init -q --bare --initial-branch=main "$origin"

seed="$work_root/seed"
git init -q --initial-branch=main "$seed"
(
  cd "$seed"
  git config user.email seed@example.test
  git config user.name Seed

  mkdir -p .nopsai/dev-loop .nopsai/plans .nopsai/reviews
  cp -R "$example_dir/scripts" .nopsai/dev-loop/scripts
  cp -R "$example_dir/prompts" .nopsai/dev-loop/prompts

  cat >AGENTS.md <<'RULES'
Write tests for new behavior.
Keep changes scoped to the task.
RULES

  cat >development-task.md <<'TASKS'
# Development tasks

1- [ ] Add a greeting helper
2- [ ] Add a farewell helper
TASKS

  printf 'initial\n' >src.txt

  git add -A
  git commit -q -m "Seed repository"
  git remote add origin "$origin"
  git push -q origin main
) || { echo "fixture setup failed" >&2; exit 1; }

checkout_script="$work_root/dev-loop-checkout.sh"
extract_checkout_script "$checkout_script"
chmod +x "$checkout_script"

# run-codex.sh looks for a binary named 'codex', so the stand-in is exposed
# under that name on a PATH entry of our own.
fake_bin="$work_root/bin"
mkdir -p "$fake_bin"
ln -s "$script_dir/fake-codex" "$fake_bin/codex"
export PATH="$fake_bin:$PATH"
export DEV_LOOP_REPOSITORY_URL="$origin"
export DEV_LOOP_BASE_BRANCH="main"
export DEV_LOOP_TASK_FILE="development-task.md"
export DEV_LOOP_AGENTS_FILE="AGENTS.md"
export DEV_LOOP_TOOLKIT_DIR=".nopsai/dev-loop"
export DEV_LOOP_PLAN_DIR=".nopsai/plans"
export DEV_LOOP_REVIEW_DIR=".nopsai/reviews"
export DEV_LOOP_BRANCH_PREFIX="nopsai/task"
export DEV_LOOP_GIT_AUTHOR_NAME="Dev Loop Test"
export DEV_LOOP_GIT_AUTHOR_EMAIL="dev-loop@example.test"
export DEV_LOOP_GIT_TOKEN="unused-for-a-local-remote"
export DEV_LOOP_CODEX_API_KEY="unused-by-the-fake-cli"
export DEV_LOOP_VALIDATE_COMMAND="true"
export FAKE_CODEX_SOURCE_FILE="src.txt"

toolkit=".nopsai/dev-loop/scripts"

# new_workspace <name> [checkout-ref]
new_workspace() {
  local name="$1" ref="${2:-main}" dir
  dir="$work_root/$name"
  mkdir -p "$dir"
  (
    cd "$dir"
    DEV_LOOP_CHECKOUT_REF="$ref" "$checkout_script"
  ) >/dev/null 2>&1 || {
    echo "checkout of '$ref' into '$name' failed" >&2
    return 1
  }
  printf '%s' "$dir"
}

origin_file() {
  git -C "$origin" show "$1:$2" 2>/dev/null
}

# --------------------------------------------------------------------------
# Round one: task 1 planned, implemented, reviewed, and recorded.
# --------------------------------------------------------------------------

runner_one="$(new_workspace runner-one)"
(
  cd "$runner_one"
  set -e
  "$toolkit/stage-select-task.sh"
  "$toolkit/stage-create-branch.sh"
  # shellcheck source=/dev/null
  source .dev-loop/task.env
  FAKE_CODEX_MODE=plan FAKE_CODEX_PLAN_PATH="$DEV_LOOP_TASK_PLAN_PATH" "$toolkit/stage-plan.sh"
  FAKE_CODEX_MODE=implement FAKE_CODEX_TASK_ID="$DEV_LOOP_TASK_ID" "$toolkit/stage-implement.sh"
) >"$work_root/runner-one.log" 2>&1
runner_one_status=$?
assert_equals "runner completes a task" "0" "$runner_one_status"
if [[ $runner_one_status -ne 0 ]]; then
  sed 's/^/       /' "$work_root/runner-one.log"
fi

branch="nopsai/task/001-add-a-greeting-helper"
assert_contains "runner pushes the task branch" \
  "$(git -C "$origin" for-each-ref --format='%(refname:short)' refs/heads)" "$branch"
assert_contains "the plan is committed to the task branch" \
  "$(origin_file "$branch" .nopsai/plans/001-add-a-greeting-helper.md)" "# Plan"
assert_contains "the implementation is committed to the task branch" \
  "$(origin_file "$branch" src.txt)" "feature for 001"
assert_not_contains "the base branch does not yet have the work" \
  "$(origin_file main src.txt)" "feature for 001"
assert_contains "the task is still unchecked on the base branch" \
  "$(origin_file main development-task.md)" "1- [ ] Add a greeting helper"

reviewer_one="$(new_workspace reviewer-one "$branch")"
(
  cd "$reviewer_one"
  set -e
  export DEV_LOOP_TASK_BRANCH="$branch"
  export DEV_LOOP_TASK_ID="001"
  export DEV_LOOP_TASK_NUMBER="1"
  export DEV_LOOP_TASK_TITLE="Add a greeting helper"
  export DEV_LOOP_TASK_SLUG="add-a-greeting-helper"
  export DEV_LOOP_TASK_PLAN_PATH=".nopsai/plans/001-add-a-greeting-helper.md"
  export DEV_LOOP_COMMIT_SHA="$(git rev-parse HEAD)"
  "$toolkit/stage-collect-evidence.sh"
  "$toolkit/stage-validate.sh"
  FAKE_CODEX_MODE=review-pass "$toolkit/stage-review.sh"
  "$toolkit/stage-record-review.sh"
  "$toolkit/stage-promote-state.sh"
) >"$work_root/reviewer-one.log" 2>&1
reviewer_one_status=$?
assert_equals "reviewer passes and promotes the task" "0" "$reviewer_one_status"
if [[ $reviewer_one_status -ne 0 ]]; then
  sed 's/^/       /' "$work_root/reviewer-one.log"
fi

assert_contains "the task is checked off on the base branch" \
  "$(origin_file main development-task.md)" "1- [x] Add a greeting helper"
assert_contains "the next task stays unchecked" \
  "$(origin_file main development-task.md)" "2- [ ] Add a farewell helper"
assert_contains "the plan is promoted to the base branch" \
  "$(origin_file main .nopsai/plans/001-add-a-greeting-helper.md)" "# Plan"
assert_contains "the review is promoted to the base branch" \
  "$(origin_file main .nopsai/reviews/001-add-a-greeting-helper.md)" "VERDICT: PASS"
assert_contains "the review records the passing verdict" \
  "$(origin_file main .nopsai/reviews/001-add-a-greeting-helper.md)" "# Review of task 001"
# state-only is the default: the record moves, the code does not.
assert_not_contains "the code stays on its branch under state-only" \
  "$(origin_file main src.txt)" "feature for 001"

# --------------------------------------------------------------------------
# Round two: the loop moves on to the next unchecked task.
# --------------------------------------------------------------------------

runner_two="$(new_workspace runner-two)"
(
  cd "$runner_two"
  set -e
  "$toolkit/stage-select-task.sh"
  "$toolkit/stage-create-branch.sh"
  # shellcheck source=/dev/null
  source .dev-loop/task.env
  FAKE_CODEX_MODE=plan FAKE_CODEX_PLAN_PATH="$DEV_LOOP_TASK_PLAN_PATH" "$toolkit/stage-plan.sh"
  FAKE_CODEX_MODE=implement FAKE_CODEX_TASK_ID="$DEV_LOOP_TASK_ID" "$toolkit/stage-implement.sh"
) >"$work_root/runner-two.log" 2>&1
assert_equals "runner picks up the next round" "0" "$?"

assert_contains "the second task gets its own branch" \
  "$(git -C "$origin" for-each-ref --format='%(refname:short)' refs/heads)" \
  "nopsai/task/002-add-a-farewell-helper"
assert_contains "the second branch carries its own plan" \
  "$(origin_file nopsai/task/002-add-a-farewell-helper .nopsai/plans/002-add-a-farewell-helper.md)" \
  "# Plan"

# --------------------------------------------------------------------------
# The guards.
# --------------------------------------------------------------------------

# A failing review must not touch the base branch, and must stop the run.
reviewer_fail="$(new_workspace reviewer-fail nopsai/task/002-add-a-farewell-helper)"
(
  cd "$reviewer_fail"
  set -e
  export DEV_LOOP_TASK_BRANCH="nopsai/task/002-add-a-farewell-helper"
  export DEV_LOOP_TASK_ID="002"
  export DEV_LOOP_TASK_NUMBER="2"
  export DEV_LOOP_TASK_TITLE="Add a farewell helper"
  export DEV_LOOP_TASK_SLUG="add-a-farewell-helper"
  export DEV_LOOP_TASK_PLAN_PATH=".nopsai/plans/002-add-a-farewell-helper.md"
  export DEV_LOOP_COMMIT_SHA="$(git rev-parse HEAD)"
  "$toolkit/stage-collect-evidence.sh"
  "$toolkit/stage-validate.sh"
  FAKE_CODEX_MODE=review-fail "$toolkit/stage-review.sh"
  "$toolkit/stage-record-review.sh"
) >"$work_root/reviewer-fail.log" 2>&1
assert_equals "a failing review stops the run" "1" "$?"
assert_contains "the failing review is recorded on the task branch" \
  "$(origin_file nopsai/task/002-add-a-farewell-helper .nopsai/reviews/002-add-a-farewell-helper.md)" \
  "VERDICT: FAIL"
assert_contains "a failed task stays unchecked on the base branch" \
  "$(origin_file main development-task.md)" "2- [ ] Add a farewell helper"
assert_equals "the failing review is not promoted to the base branch" "" \
  "$(origin_file main .nopsai/reviews/002-add-a-farewell-helper.md)"

# A review with no parseable verdict is a failure, not a pass.
reviewer_silent="$(new_workspace reviewer-silent nopsai/task/002-add-a-farewell-helper)"
(
  cd "$reviewer_silent"
  set -e
  export DEV_LOOP_TASK_BRANCH="nopsai/task/002-add-a-farewell-helper"
  export DEV_LOOP_TASK_ID="002"
  export DEV_LOOP_TASK_NUMBER="2"
  export DEV_LOOP_TASK_TITLE="Add a farewell helper"
  export DEV_LOOP_TASK_SLUG="add-a-farewell-helper"
  export DEV_LOOP_TASK_PLAN_PATH=".nopsai/plans/002-add-a-farewell-helper.md"
  "$toolkit/stage-collect-evidence.sh"
  "$toolkit/stage-validate.sh"
  FAKE_CODEX_MODE=review-no-verdict "$toolkit/stage-review.sh"
  test "$(cat .dev-loop/codex-verdict)" = "FAIL"
) >"$work_root/reviewer-silent.log" 2>&1
assert_equals "a review with no verdict resolves to FAIL" "0" "$?"

# A review that edits the branch is a review that stopped reviewing.
reviewer_writes="$(new_workspace reviewer-writes nopsai/task/002-add-a-farewell-helper)"
(
  cd "$reviewer_writes"
  set -e
  export DEV_LOOP_TASK_BRANCH="nopsai/task/002-add-a-farewell-helper"
  export DEV_LOOP_TASK_ID="002"
  export DEV_LOOP_TASK_TITLE="Add a farewell helper"
  export DEV_LOOP_TASK_SLUG="add-a-farewell-helper"
  export DEV_LOOP_TASK_PLAN_PATH=".nopsai/plans/002-add-a-farewell-helper.md"
  "$toolkit/stage-collect-evidence.sh"
  "$toolkit/stage-validate.sh"
  FAKE_CODEX_MODE=review-writes-code "$toolkit/stage-review.sh"
) >"$work_root/reviewer-writes.log" 2>&1
assert_equals "a review that modifies the tree fails" "1" "$?"

# Validation that fails cannot be overridden by an approving review.
reviewer_broken="$(new_workspace reviewer-broken nopsai/task/002-add-a-farewell-helper)"
(
  cd "$reviewer_broken"
  set -e
  export DEV_LOOP_TASK_BRANCH="nopsai/task/002-add-a-farewell-helper"
  export DEV_LOOP_TASK_ID="002"
  export DEV_LOOP_TASK_NUMBER="2"
  export DEV_LOOP_TASK_TITLE="Add a farewell helper"
  export DEV_LOOP_TASK_SLUG="add-a-farewell-helper"
  export DEV_LOOP_TASK_PLAN_PATH=".nopsai/plans/002-add-a-farewell-helper.md"
  "$toolkit/stage-collect-evidence.sh"
  DEV_LOOP_VALIDATE_COMMAND="exit 1" "$toolkit/stage-validate.sh"
  FAKE_CODEX_MODE=review-pass "$toolkit/stage-review.sh"
  "$toolkit/stage-record-review.sh"
) >"$work_root/reviewer-broken.log" 2>&1
assert_equals "a passing review cannot override failing tests" "1" "$?"

# The planner may write the plan and nothing else.
runner_overreach="$(new_workspace runner-overreach)"
(
  cd "$runner_overreach"
  set -e
  "$toolkit/stage-select-task.sh"
  DEV_LOOP_ALLOW_EXISTING_BRANCH=true "$toolkit/stage-create-branch.sh"
  # shellcheck source=/dev/null
  source .dev-loop/task.env
  FAKE_CODEX_MODE=plan-overreach FAKE_CODEX_PLAN_PATH="$DEV_LOOP_TASK_PLAN_PATH" "$toolkit/stage-plan.sh"
) >"$work_root/runner-overreach.log" 2>&1
assert_equals "a planner that touches source fails" "1" "$?"
assert_contains "and says which file it touched" \
  "$(cat "$work_root/runner-overreach.log")" "unrelated-source.go"

# The implementation may not mark its own task complete.
runner_self_marks="$(new_workspace runner-self-marks)"
(
  cd "$runner_self_marks"
  set -e
  "$toolkit/stage-select-task.sh"
  DEV_LOOP_ALLOW_EXISTING_BRANCH=true "$toolkit/stage-create-branch.sh"
  # shellcheck source=/dev/null
  source .dev-loop/task.env
  FAKE_CODEX_MODE=plan FAKE_CODEX_PLAN_PATH="$DEV_LOOP_TASK_PLAN_PATH" "$toolkit/stage-plan.sh"
  FAKE_CODEX_MODE=implement-touches-task-file "$toolkit/stage-implement.sh"
) >"$work_root/runner-self-marks.log" 2>&1
assert_equals "an implementation that edits the task file fails" "1" "$?"

# Nothing to review is not something to review.
runner_noop="$(new_workspace runner-noop)"
(
  cd "$runner_noop"
  set -e
  "$toolkit/stage-select-task.sh"
  DEV_LOOP_ALLOW_EXISTING_BRANCH=true "$toolkit/stage-create-branch.sh"
  # shellcheck source=/dev/null
  source .dev-loop/task.env
  FAKE_CODEX_MODE=plan FAKE_CODEX_PLAN_PATH="$DEV_LOOP_TASK_PLAN_PATH" "$toolkit/stage-plan.sh"
  FAKE_CODEX_MODE=implement-noop "$toolkit/stage-implement.sh"
) >"$work_root/runner-noop.log" 2>&1
assert_equals "an implementation with no changes fails" "1" "$?"

# Reusing a task branch takes an explicit opt-in.
runner_existing="$(new_workspace runner-existing)"
(
  cd "$runner_existing"
  set -e
  "$toolkit/stage-select-task.sh"
  "$toolkit/stage-create-branch.sh"
) >"$work_root/runner-existing.log" 2>&1
assert_equals "an existing task branch stops the run" "1" "$?"
assert_contains "and explains the opt-in" \
  "$(cat "$work_root/runner-existing.log")" "DEV_LOOP_ALLOW_EXISTING_BRANCH"

# Reviewing a branch that moved since submission reviews the wrong code.
reviewer_moved="$(new_workspace reviewer-moved nopsai/task/002-add-a-farewell-helper)"
(
  cd "$reviewer_moved"
  set -e
  export DEV_LOOP_TASK_BRANCH="nopsai/task/002-add-a-farewell-helper"
  export DEV_LOOP_TASK_PLAN_PATH=".nopsai/plans/002-add-a-farewell-helper.md"
  export DEV_LOOP_COMMIT_SHA="0000000000000000000000000000000000000000"
  "$toolkit/stage-collect-evidence.sh"
) >"$work_root/reviewer-moved.log" 2>&1
assert_equals "reviewing a moved branch is refused" "1" "$?"

# --------------------------------------------------------------------------
# Finishing the queue ends the loop.
# --------------------------------------------------------------------------

drain="$(new_workspace drain)"
(
  cd "$drain"
  set -e
  "$toolkit/mark-task-done.sh" development-task.md 2
  git add -- development-task.md
  git -c user.email=t@t -c user.name=T commit -q -m "Complete the queue"
  git push -q origin main
) >/dev/null 2>&1

idle="$(new_workspace idle)"
idle_output="$(cd "$idle" && "$toolkit/stage-select-task.sh" 2>&1)"
assert_contains "an empty queue reports ALL_TASKS_DONE" "$idle_output" "ALL_TASKS_DONE"

idle_branch_output="$(cd "$idle" && "$toolkit/stage-create-branch.sh" 2>&1)"
assert_equals "later stages skip cleanly when there is no task" "0" "$?"
assert_contains "and say so" "$idle_branch_output" "no pending task"

echo
echo "$passed passed, $failed failed"
[[ $failed -eq 0 ]]
