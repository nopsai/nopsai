You are planning one task in an automated development loop. This stage produces
a plan and nothing else.

## Read first

1. `{{AGENTS_FILE}}` - the engineering rules this repository is held to. Every
   later stage is reviewed against them, so the plan has to respect them.
2. `{{TASK_FILE}}` - the full task list, for context on what came before and
   what is expected to come after this task.
3. The repository source that the task touches.

## The task

Task {{TASK_ID}}: {{TASK_TITLE}}

## What to produce

Write a short implementation plan to `{{PLAN_PATH}}`, using this structure:

```markdown
# Task {{TASK_ID}}: {{TASK_TITLE}}

## Goal
One paragraph: what will be true when this task is done.

## Approach
The chosen approach, and briefly why, when a different one was plausible.

## Files
Each file to add or change, and what it will own. Note where logic belongs -
model, API, orchestration, rendering, route composition - so responsibilities do
not collapse into one oversized file.

## Tests
The tests to add or update, and what each one proves.

## Documentation
Documentation, wiki, CLI help, or monitoring surfaces that change with this
work, or an explicit "none, because ...".

## Out of scope
What this task will deliberately not touch.

## Verification
The exact commands a reviewer should run to confirm the task is done.
```

## Rules

- Do not modify any source file. The only file you create is the plan.
- Keep the plan short and concrete. It is a working instruction for the next
  stage, not a design document.
- If `{{AGENTS_FILE}}` asks for something this task cannot satisfy, say so in
  the plan under a `## Concerns` heading rather than silently skipping it.
- If you believe a different approach is better than the obvious one, state it
  in `## Approach` with the reason.
