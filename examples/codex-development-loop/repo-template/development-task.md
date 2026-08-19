# Development tasks

The queue the development loop works through. One line per task, in the order
you want them done. The loop picks the first unchecked line, and the review
stage checks it off only after the work passes review.

Format:

```
1- [ ] Task description
2- [x] A completed task
```

Rules that keep the record trustworthy:

- **The number is permanent.** It appears in the branch name, the plan file, and
  the review file for that task. Never renumber, never reuse a number, and never
  reorder existing lines - append new tasks at the end.
- **Only the review stage checks a box.** A task is marked `[x]` on the base
  branch after its implementation passed review, and never by hand mid-run.
- **One task, one outcome.** Write tasks that a single branch can deliver. "Add
  retry to the checkout API" works; "improve reliability" does not, because
  nothing can decide when it is finished.
- **The description is the instruction.** It is the entire brief the planner
  gets, so say what the task must achieve rather than gesturing at an area.

Tasks:

1- [ ] Replace the description on this line with your first real task
