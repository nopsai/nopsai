#!/usr/bin/env bash
# Tests for the development loop's deterministic scripts.
#
#   examples/codex-development-loop/tests/run-script-tests.sh
#
# Every fixture is built here in a temporary directory. Nothing reads a real
# repository, a real commit, or a network service, so the suite gives the same
# answer on any machine and can run in CI without setup.
set -uo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
scripts_dir="$(cd "$script_dir/../scripts" && pwd)"
prompts_dir="$(cd "$script_dir/../prompts" && pwd)"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

passed=0
failed=0

pass() {
  passed=$((passed + 1))
  echo "ok   - $1"
}

fail() {
  failed=$((failed + 1))
  echo "FAIL - $1"
  if [[ -n "${2:-}" ]]; then
    echo "       $2"
  fi
}

assert_equals() {
  local name="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    pass "$name"
  else
    fail "$name" "expected [$expected], got [$actual]"
  fi
}

assert_contains() {
  local name="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    pass "$name"
  else
    fail "$name" "expected to find [$needle] in [$haystack]"
  fi
}

assert_succeeds() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    pass "$name"
  else
    fail "$name" "command failed: $*"
  fi
}

assert_fails() {
  local name="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$name" "command unexpectedly succeeded: $*"
  else
    pass "$name"
  fi
}

new_case() {
  local dir
  dir="$(mktemp -d "$work_dir/case.XXXXXX")"
  printf '%s' "$dir"
}

# ---------------------------------------------------------------------------
# find-next-task.sh
# ---------------------------------------------------------------------------

case_dir="$(new_case)"
cat >"$case_dir/tasks.md" <<'TASKS'
# Development tasks

1- [x] Already finished
2- [ ] Add retry to the checkout API
3- [ ] Raise the request timeout
TASKS

"$scripts_dir/find-next-task.sh" "$case_dir/tasks.md" "$case_dir/task.env" >/dev/null 2>&1
# shellcheck source=/dev/null
source "$case_dir/task.env"
assert_equals "picks the first unchecked task" "2" "$DEV_LOOP_TASK_NUMBER"
assert_equals "zero-pads the task id" "002" "$DEV_LOOP_TASK_ID"
assert_equals "keeps the task title" "Add retry to the checkout API" "$DEV_LOOP_TASK_TITLE"
assert_equals "slugifies the title" "add-retry-to-the-checkout-api" "$DEV_LOOP_TASK_SLUG"
assert_equals "counts remaining tasks" "2" "$DEV_LOOP_TASK_REMAINING"
assert_equals "counts all tasks" "3" "$DEV_LOOP_TASK_TOTAL"
assert_equals "reports work to do" "false" "$DEV_LOOP_ALL_TASKS_DONE"

case_dir="$(new_case)"
cat >"$case_dir/tasks.md" <<'TASKS'
1- [x] Done
2- [X] Also done, capital X
TASKS
output="$("$scripts_dir/find-next-task.sh" "$case_dir/tasks.md" "$case_dir/task.env" 2>&1)"
assert_equals "reports when every task is done" "ALL_TASKS_DONE" "$output"
unset DEV_LOOP_TASK_NUMBER
# shellcheck source=/dev/null
source "$case_dir/task.env"
assert_equals "sets the all-done flag" "true" "$DEV_LOOP_ALL_TASKS_DONE"

# A title with quotes and shell metacharacters has to survive the round trip
# through the env file, because it is written by a human and read by a shell.
case_dir="$(new_case)"
cat >"$case_dir/tasks.md" <<'TASKS'
1- [ ] Fix the `run` command's $PATH handling & "quoting"
TASKS
"$scripts_dir/find-next-task.sh" "$case_dir/tasks.md" "$case_dir/task.env" >/dev/null 2>&1
unset DEV_LOOP_TASK_TITLE
# shellcheck source=/dev/null
source "$case_dir/task.env"
assert_equals "preserves shell metacharacters in a title" \
  'Fix the `run` command'"'"'s $PATH handling & "quoting"' "$DEV_LOOP_TASK_TITLE"

case_dir="$(new_case)"
cat >"$case_dir/tasks.md" <<'TASKS'
1- [ ] Valid task
2- broken line with no checkbox
TASKS
assert_fails "rejects a malformed task line" \
  "$scripts_dir/find-next-task.sh" "$case_dir/tasks.md" "$case_dir/task.env"

case_dir="$(new_case)"
printf '# No tasks here\n' >"$case_dir/tasks.md"
assert_fails "rejects a task file with no tasks" \
  "$scripts_dir/find-next-task.sh" "$case_dir/tasks.md" "$case_dir/task.env"

case_dir="$(new_case)"
assert_fails "rejects a missing task file" \
  "$scripts_dir/find-next-task.sh" "$case_dir/absent.md" "$case_dir/task.env"

case_dir="$(new_case)"
printf '1- [ ]\n' >"$case_dir/tasks.md"
assert_fails "rejects a task with no description" \
  "$scripts_dir/find-next-task.sh" "$case_dir/tasks.md" "$case_dir/task.env"

# ---------------------------------------------------------------------------
# mark-task-done.sh
# ---------------------------------------------------------------------------

case_dir="$(new_case)"
cat >"$case_dir/tasks.md" <<'TASKS'
# Development tasks

1- [x] Already finished
2- [ ] Add retry to the checkout API
3- [ ] Raise the request timeout
TASKS
"$scripts_dir/mark-task-done.sh" "$case_dir/tasks.md" 2 >/dev/null 2>&1
assert_contains "checks the requested task off" "$(cat "$case_dir/tasks.md")" \
  "2- [x] Add retry to the checkout API"
assert_contains "leaves later tasks unchecked" "$(cat "$case_dir/tasks.md")" \
  "3- [ ] Raise the request timeout"
assert_contains "leaves the file's other content alone" "$(cat "$case_dir/tasks.md")" \
  "# Development tasks"

# Task numbers are permanent identifiers, so completing one must not disturb
# the numbering of any other.
assert_equals "does not renumber tasks" \
  "1 2 3" \
  "$(grep -oE '^[0-9]+' "$case_dir/tasks.md" | tr '\n' ' ' | sed 's/ $//')"

output="$("$scripts_dir/mark-task-done.sh" "$case_dir/tasks.md" 2 2>&1)"
assert_contains "marking a done task again is not an error" "$output" "TASK_ALREADY_DONE"

case_dir="$(new_case)"
cat >"$case_dir/tasks.md" <<'TASKS'
1- [ ] Support [bracketed] descriptions
TASKS
"$scripts_dir/mark-task-done.sh" "$case_dir/tasks.md" 1 >/dev/null 2>&1
assert_equals "only the checkbox is replaced, not brackets in the text" \
  "1- [x] Support [bracketed] descriptions" "$(cat "$case_dir/tasks.md")"

case_dir="$(new_case)"
printf '1- [ ] Only task\n' >"$case_dir/tasks.md"
assert_fails "rejects an unknown task number" \
  "$scripts_dir/mark-task-done.sh" "$case_dir/tasks.md" 9

case_dir="$(new_case)"
printf '1- [ ] First\n1- [ ] Duplicate number\n' >"$case_dir/tasks.md"
assert_fails "rejects duplicate task numbers" \
  "$scripts_dir/mark-task-done.sh" "$case_dir/tasks.md" 1

# ---------------------------------------------------------------------------
# parse-verdict.sh - the fail-closed gate
# ---------------------------------------------------------------------------

case_dir="$(new_case)"

printf 'VERDICT: PASS\n\n## Reasons\nLooks right.\n' >"$case_dir/pass.md"
assert_equals "reads a clean PASS" "PASS" "$("$scripts_dir/parse-verdict.sh" "$case_dir/pass.md")"

printf 'VERDICT: FAIL\n\n## Reasons\nMissing tests.\n' >"$case_dir/fail.md"
assert_equals "reads a clean FAIL" "FAIL" "$("$scripts_dir/parse-verdict.sh" "$case_dir/fail.md")"

printf 'VERDICT: PASS\r\n' >"$case_dir/crlf.md"
assert_equals "tolerates CRLF line endings" "PASS" "$("$scripts_dir/parse-verdict.sh" "$case_dir/crlf.md")"

printf 'The change looks good to me, I would pass it.\n' >"$case_dir/prose.md"
assert_equals "prose approval is not a verdict" "FAIL" "$("$scripts_dir/parse-verdict.sh" "$case_dir/prose.md" 2>/dev/null)"

printf 'VERDICT: PASS\nVERDICT: FAIL\n' >"$case_dir/both.md"
assert_equals "contradictory verdicts fail" "FAIL" "$("$scripts_dir/parse-verdict.sh" "$case_dir/both.md" 2>/dev/null)"

printf 'VERDICT: PASS with reservations\n' >"$case_dir/qualified.md"
assert_equals "a qualified verdict line fails" "FAIL" "$("$scripts_dir/parse-verdict.sh" "$case_dir/qualified.md" 2>/dev/null)"

: >"$case_dir/empty.md"
assert_equals "an empty review fails" "FAIL" "$("$scripts_dir/parse-verdict.sh" "$case_dir/empty.md" 2>/dev/null)"

assert_equals "a missing review fails" "FAIL" "$("$scripts_dir/parse-verdict.sh" "$case_dir/absent.md" 2>/dev/null)"

assert_equals "no argument fails" "FAIL" "$("$scripts_dir/parse-verdict.sh" 2>/dev/null)"

# ---------------------------------------------------------------------------
# render-prompt.sh
# ---------------------------------------------------------------------------

case_dir="$(new_case)"
printf 'Task {{TASK_ID}}: {{TASK_TITLE}}\nPlan: {{PLAN_PATH}}\n' >"$case_dir/template.md"
"$scripts_dir/render-prompt.sh" "$case_dir/template.md" "$case_dir/out.md" \
  "TASK_ID=007" "TASK_TITLE=Add retry & \$backoff" "PLAN_PATH=.nopsai/plans/007-add-retry.md" >/dev/null 2>&1
assert_contains "substitutes placeholders" "$(cat "$case_dir/out.md")" "Task 007: Add retry & \$backoff"
assert_contains "substitutes every placeholder" "$(cat "$case_dir/out.md")" "Plan: .nopsai/plans/007-add-retry.md"

printf 'Task {{TASK_ID}} needs {{MISSING}}\n' >"$case_dir/partial.md"
assert_fails "refuses to render an unfilled placeholder" \
  "$scripts_dir/render-prompt.sh" "$case_dir/partial.md" "$case_dir/partial-out.md" "TASK_ID=001"

# Each shipped prompt must render with the values its pipeline actually passes;
# a typo in a placeholder name would otherwise surface only at run time.
"$scripts_dir/render-prompt.sh" "$prompts_dir/planning.md" "$case_dir/planning.md" \
  "TASK_ID=001" "TASK_TITLE=Example" "TASK_FILE=development-task.md" \
  "AGENTS_FILE=AGENTS.md" "PLAN_PATH=.nopsai/plans/001-example.md" >/dev/null 2>&1
assert_succeeds "planning prompt renders" test -s "$case_dir/planning.md"

"$scripts_dir/render-prompt.sh" "$prompts_dir/implementation.md" "$case_dir/implementation.md" \
  "TASK_ID=001" "TASK_TITLE=Example" "TASK_FILE=development-task.md" \
  "AGENTS_FILE=AGENTS.md" "PLAN_PATH=.nopsai/plans/001-example.md" \
  "BRANCH=nopsai/task/001-example" "BASE_BRANCH=main" >/dev/null 2>&1
assert_succeeds "implementation prompt renders" test -s "$case_dir/implementation.md"

"$scripts_dir/render-prompt.sh" "$prompts_dir/review.md" "$case_dir/review.md" \
  "TASK_ID=001" "TASK_TITLE=Example" "TASK_FILE=development-task.md" \
  "AGENTS_FILE=AGENTS.md" "PLAN_PATH=.nopsai/plans/001-example.md" \
  "BRANCH=nopsai/task/001-example" "BASE_BRANCH=main" \
  "DIFF_FILE=.dev-loop/task.diff" "VALIDATION_REPORT=.dev-loop/validation.md" >/dev/null 2>&1
assert_succeeds "review prompt renders" test -s "$case_dir/review.md"

# The review prompt has to teach the exact verdict line the parser accepts,
# or the two halves of the gate would drift apart.
rendered_review="$(cat "$case_dir/review.md")"
assert_contains "review prompt specifies the PASS line" "$rendered_review" "VERDICT: PASS"
assert_contains "review prompt specifies the FAIL line" "$rendered_review" "VERDICT: FAIL"

# ---------------------------------------------------------------------------
# validate-repo.sh
# ---------------------------------------------------------------------------

case_dir="$(new_case)"
(
  cd "$case_dir"
  DEV_LOOP_VALIDATE_COMMAND="true" "$scripts_dir/validate-repo.sh" report.md
) >/dev/null 2>&1
assert_contains "records a passing validation" "$(cat "$case_dir/report.md")" "VALIDATION_PASSED"

case_dir="$(new_case)"
(
  cd "$case_dir"
  DEV_LOOP_VALIDATE_COMMAND="exit 3" "$scripts_dir/validate-repo.sh" report.md
) >/dev/null 2>&1
status=$?
assert_contains "records a failing validation" "$(cat "$case_dir/report.md")" "VALIDATION_FAILED"
assert_equals "propagates validation failure" "1" "$status"

# The fail-closed rule: an unrecognised repository cannot be validated, and
# what cannot be validated must not pass.
case_dir="$(new_case)"
(
  cd "$case_dir"
  env -u DEV_LOOP_VALIDATE_COMMAND "$scripts_dir/validate-repo.sh" report.md
) >/dev/null 2>&1
status=$?
assert_equals "fails when validation cannot run at all" "1" "$status"
assert_contains "says why it could not validate" "$(cat "$case_dir/report.md")" "VALIDATION_UNAVAILABLE"

# ---------------------------------------------------------------------------

echo
echo "$passed passed, $failed failed"
[[ $failed -eq 0 ]]
