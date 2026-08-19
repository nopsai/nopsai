#!/usr/bin/env bash
# Extract the review verdict from a Codex review message.
#
# Usage:
#   parse-verdict.sh <review-message-file>
#
# Prints exactly PASS or FAIL on stdout and always exits 0, so the caller
# records the verdict rather than losing it to a non-zero exit.
#
# This is the fail-closed gate of the whole loop. PASS is printed only when the
# message contains exactly one well-formed verdict line and that line says
# PASS. A missing verdict, a malformed verdict, contradictory verdicts, an
# empty file, or an unreadable file all resolve to FAIL, because a review that
# cannot be evaluated has not passed.
set -uo pipefail

review_file="${1:-}"

fail() {
  if [[ -n "${1:-}" ]]; then
    echo "dev-loop: $1" >&2
  fi
  echo "FAIL"
  exit 0
}

[[ -n "$review_file" ]] || fail "usage: parse-verdict.sh <review-message-file>"
[[ -f "$review_file" && -r "$review_file" ]] || fail "review message '$review_file' is missing or unreadable"
[[ -s "$review_file" ]] || fail "review message '$review_file' is empty"

verdict_pattern='^[[:space:]]*VERDICT:[[:space:]]*(PASS|FAIL)[[:space:]]*$'

verdicts=()
while IFS= read -r line || [[ -n "$line" ]]; do
  # Strip a carriage return so CRLF output does not break the exact match.
  line="${line%$'\r'}"
  if [[ "$line" =~ $verdict_pattern ]]; then
    verdicts+=("${BASH_REMATCH[1]}")
  fi
done <"$review_file"

if [[ ${#verdicts[@]} -eq 0 ]]; then
  fail "no 'VERDICT: PASS' or 'VERDICT: FAIL' line found in '$review_file'"
fi
if [[ ${#verdicts[@]} -gt 1 ]]; then
  fail "found ${#verdicts[@]} verdict lines in '$review_file'; a review must state exactly one"
fi

echo "${verdicts[0]}"
