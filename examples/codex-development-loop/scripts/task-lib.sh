#!/usr/bin/env bash
# Shared parsing helpers for the development task loop.
#
# The task file is the single source of truth for what is left to do. Its
# format is intentionally rigid so that every decision in the loop is made by
# these deterministic helpers and never by a model:
#
#   1- [ ] Task description
#   2- [x] Task description
#
# The leading number is a permanent identifier. Tasks are never renumbered, so
# plans, branches, and reviews recorded for task 7 keep pointing at task 7 even
# after earlier tasks are completed or the wording changes.
#
# Source this file; it defines functions and does not run anything on its own.

# A task line: number, hyphen, checkbox, description.
DEV_LOOP_TASK_PATTERN='^([0-9]+)-[[:space:]]*\[([ xX])\][[:space:]]*(.*)$'

# A line that starts like a task but is not one. Detecting these lets the loop
# refuse to run against a malformed task file instead of silently skipping work.
DEV_LOOP_TASK_LIKE_PATTERN='^[0-9]+-'

dev_loop_die() {
  echo "dev-loop: $*" >&2
  exit 1
}

# dev_loop_slugify <text>
#
# Produces the branch- and filename-safe half of a task identifier. Lowercase,
# alphanumerics and hyphens only, collapsed, trimmed, and capped so that branch
# names stay readable.
dev_loop_slugify() {
  local text="$1"
  local max="${2:-48}"
  local slug
  # POSIX basic regular expressions only: '\+' is a GNU extension that BSD sed
  # reads as a literal plus, which would leave the text unslugified on macOS.
  slug="$(printf '%s' "$text" \
    | tr '[:upper:]' '[:lower:]' \
    | sed -e 's/[^a-z0-9][^a-z0-9]*/-/g' -e 's/--*/-/g' -e 's/^-//' -e 's/-$//')"
  if [[ ${#slug} -gt $max ]]; then
    slug="${slug:0:$max}"
    slug="${slug%-}"
  fi
  if [[ -z "$slug" ]]; then
    slug="task"
  fi
  printf '%s' "$slug"
}

# dev_loop_task_id <number>
#
# Zero-pads the permanent task number to the three-digit form used in branch
# names, plan files, and review files.
dev_loop_task_id() {
  printf '%03d' "$((10#$1))"
}

# dev_loop_require_task_file <path>
#
# Fails closed: a task file that is missing, unreadable, or contains a line that
# looks like a task but does not parse stops the loop rather than being treated
# as "nothing to do".
dev_loop_require_task_file() {
  local file="$1"
  [[ -n "$file" ]] || dev_loop_die "task file path is required"
  [[ -f "$file" ]] || dev_loop_die "task file '$file' does not exist"
  [[ -r "$file" ]] || dev_loop_die "task file '$file' is not readable"

  local line_number=0
  local line
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    if [[ "$line" =~ $DEV_LOOP_TASK_LIKE_PATTERN ]] && ! [[ "$line" =~ $DEV_LOOP_TASK_PATTERN ]]; then
      dev_loop_die "task file '$file' line $line_number is not a valid task line: $line"
    fi
  done <"$file"
}

# dev_loop_shell_quote <value>
#
# Single-quotes a value so task titles containing spaces, quotes, or shell
# metacharacters survive being written to and sourced from an env file.
dev_loop_shell_quote() {
  local value="$1"
  # Close the quote, emit an escaped quote, reopen: 'it'\''s'. Pattern and
  # replacement go through variables because Bash 3.2, still the default shell
  # on macOS, does not honour quotes written inline inside the substitution.
  local quote="'"
  local escaped_quote="'\\''"
  printf "'%s'" "${value//$quote/$escaped_quote}"
}
