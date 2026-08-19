# Reviews

One file per reviewed task, named `<task-id>-<slug>.md`, matching the task's
permanent number.

Every review is committed to the task branch, whether it passed or failed, so a
failed attempt leaves its reasons next to the code that caused them. Passing
reviews are additionally copied here on the base branch alongside the plan and
the checked-off task, so the base branch carries the complete record of what was
done and why it was accepted.
