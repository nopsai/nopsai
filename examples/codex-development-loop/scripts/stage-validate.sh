#!/usr/bin/env bash
# Reviewer stage 2: run the repository's own build and tests.
#
# The result is recorded rather than thrown, so the review record can say what
# failed. It still binds the outcome: a validation run that fails, or that
# cannot run at all, can never become a PASS.
set -euo pipefail

toolkit="${DEV_LOOP_TOOLKIT_DIR:-.nopsai/dev-loop}"

status=0
"$toolkit/scripts/validate-repo.sh" .dev-loop/validation.md || status=$?
echo "$status" >.dev-loop/validation-status

if [[ $status -eq 0 ]]; then
  echo "VALIDATION_PASSED"
else
  echo "VALIDATION_FAILED (exit $status); the review will record this and cannot pass"
fi
