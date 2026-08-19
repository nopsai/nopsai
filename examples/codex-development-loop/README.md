# Codex Development Loop

Two pipelines that work through a task list one task at a time: the first plans
and implements a task on its own branch, the second reviews it and, if the work
holds up, records the task complete and starts the next round.

```
              MANUAL START
                   │
                   ▼
      development-task-runner ──────────── no unchecked tasks ──▶ stop
                   │
        first '[ ]' task in development-task.md
                   │
             Codex writes the plan  ──▶ commit on nopsai/task/001-<slug>
                   │
             Codex writes the code  ──▶ commit on nopsai/task/001-<slug>
                   │
                   ▼
      development-task-reviewer
                   │
        build + tests, then Codex reviews the diff
                   │
        ┌──────────┴──────────┐
      FAIL                   PASS
        │                      │
  review committed        task marked [x] on main,
  to the task branch,     plan + review copied to main
  loop stops                   │
                        more '[ ]' tasks?
                          ┌────┴────┐
                        yes         no
                          │          │
                          ▼          ▼
              runner via trigger    stop
```

The division of labour is the point: **Codex writes plans, code, and reviews.
Scripts make every decision.** Which task is next, whether the planner stayed in
its lane, whether the build passed, whether the verdict counts, whether the loop
continues - each of those is a shell script with a deterministic answer. Both
pipelines set `llm_enabled: false`, so a model cannot reach the control flow.

## What is here

| Path | Purpose |
| --- | --- |
| `pipelines/platform/development-task-runner.yaml` | Selects a task, plans it, implements it on a branch, calls the reviewer. |
| `pipelines/platform/development-task-reviewer.yaml` | Validates and reviews the branch, records the verdict, promotes task state, continues the loop. |
| `steps/platform/shared/dev-loop-checkout.yaml` | Reusable checkout shared by both pipelines. |
| `external-triggers/platform/*.yaml` | The authenticated API entry points the two pipelines use to call each other. |
| `scopes/platform/dev-loop/scope.yaml` | Every setting and secret key the loop uses, in one place. |
| `access/grants.yaml` | The service account the loop runs as, and its permissions. |
| `runner-image/Dockerfile` | Image carrying Git, the Codex CLI, and the target repository's toolchain. |
| `prompts/` | The planning, implementation, and review prompts. |
| `scripts/stage-*.sh` | One script per pipeline step. This is where the loop's logic lives, which is what makes it testable without a running platform. |
| `scripts/` (rest) | The deterministic primitives: task selection, task completion, validation, verdict parsing, prompt rendering, trigger invocation. |
| `repo-template/` | What to add to the repository the loop develops. |
| `tests/run-script-tests.sh` | Unit tests for the primitives. |
| `tests/run-loop-integration-test.sh` | End-to-end test: the real stage scripts against a real Git repository, with a fake Codex CLI. |

## Task lifecycle

```
PENDING ─▶ PLANNING ─▶ IMPLEMENTING ─▶ REVIEWING ─▶ PASSED ─▶ DONE
```

For task 1, "Add retry to the checkout API":

| Artifact | Where |
| --- | --- |
| Branch | `nopsai/task/001-add-retry-to-the-checkout-api` |
| Plan | `.nopsai/plans/001-add-retry-to-the-checkout-api.md` |
| Review | `.nopsai/reviews/001-add-retry-to-the-checkout-api.md` |
| Task state | `1- [x] Add retry to the checkout API` in `development-task.md` |

The leading number is a permanent identifier. Tasks are never renumbered, so
these four artifacts stay tied together for the life of the repository.

## Where state lives

Task branches hold the work: the plan, the implementation, and the review of
that attempt, pass or fail. The base branch holds the record: which tasks are
done, and the plan and review for each one.

That split matters because every task branches from the base branch. Task state
is promoted to the base branch the moment a task passes, so the next round sees
an accurate picture of what is left. A failed review is recorded on its own
branch and goes no further - the base branch never claims work that did not pass.

By default the code itself stays on its branch for a human to merge
(`DEV_LOOP_ON_PASS: state-only`). Set `DEV_LOOP_ON_PASS: merge` when tasks build
on each other and each one needs the previous task's code to be present.

## Installing it

### 1. Prepare the repository the loop develops

Copy `repo-template/` into it and fill in the task list:

```
development-task.md          the queue
AGENTS.md                    the engineering rules every stage is held to
.nopsai/plans/               plan records
.nopsai/reviews/             review records
.nopsai/dev-loop/scripts/    copied from scripts/
.nopsai/dev-loop/prompts/    copied from prompts/
```

The scripts and prompts live in the target repository so the loop's behaviour is
versioned alongside the code it changes. `DEV_LOOP_TOOLKIT_DIR` names that
location; when the loop develops NopsAI itself, point it at
`examples/codex-development-loop` instead of copying anything.

Task file format:

```
1- [ ] Add retry to the checkout API
2- [ ] Raise the request timeout to 30s
3- [x] Already finished
```

### 2. Build the runner image

```bash
docker build -t ghcr.io/acme/nopsai-codex-runner:1.0.0 examples/codex-development-loop/runner-image
docker push ghcr.io/acme/nopsai-codex-runner:1.0.0
```

Then set that tag as `container_image` in both pipelines and in the shared
checkout step. Pin by digest for production, as the platform release pipeline
does. The image ships no credentials.

### 3. Write the credentials

Three secrets, declared by key in the scope and resolved from the credential
store at run time:

| Secret | Used for |
| --- | --- |
| `DEV_LOOP_GIT_TOKEN` | Pushing branches, plans, reviews, and task state. |
| `DEV_LOOP_CODEX_API_KEY` | The Codex CLI. `run-codex.sh` maps it onto the CLI's own variable. |
| `DEV_LOOP_NOPSAI_TOKEN` | The service-account token each pipeline uses to call the other. |

### 4. Sync the configuration

Copy `pipelines/`, `steps/`, `external-triggers/`, `scopes/`, and `access/` into
your config repository, adjust the `platform` team prefix and the example hosts,
and sync. Everything the loop needs is Git-owned, including the trigger
definitions that let the two halves call each other.

### 5. Start it

```bash
curl -X POST \
  -H "Authorization: Bearer nopsat_<service-account-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type":"dev-loop.manual.start",
    "idempotency_key":"dev-loop.manual:2026-08-19T09:00:00Z",
    "payload":{
      "repository_url":"https://github.com/acme/nopsai.git",
      "base_branch":"main"
    }
  }' \
  http://localhost:8080/v1/external-triggers/development-task-runner/invoke
```

From there the loop runs itself until the tasks run out or a review fails.

## Where it refuses to continue

A loop that runs unattended has to stop for the right reasons, so each of these
is a hard stop rather than a warning:

- **The task file is malformed.** A line that looks like a task but does not
  parse stops the run. Reading it as "nothing left to do" would silently end the
  loop with work outstanding.
- **The task branch already exists.** A previous attempt is in flight or failed.
  Set `DEV_LOOP_ALLOW_EXISTING_BRANCH: "true"` to deliberately continue on it.
- **The planning stage touched anything but the plan.** Checked with
  `git status`, not trusted to the prompt.
- **The implementation stage edited the task file.** Task state belongs to the
  review stage; a stage that marks its own work complete is not reviewed.
- **The implementation produced no changes.** There is nothing to review.
- **The branch moved since it was submitted.** The reviewer refuses to review a
  commit other than the one it was called for.
- **Validation cannot run.** No configured command and no detected toolchain
  means the repository's health is unknown, and unknown is not passing.
- **The verdict is anything but one clean `VERDICT: PASS` line.** A missing
  verdict, a qualified one, contradictory ones, an empty review, prose approval -
  all resolve to FAIL. A review that cannot be evaluated has not passed.
- **The merge conflicts** under `DEV_LOOP_ON_PASS: merge`. The task is not
  marked done.

The verdict of record is the conjunction of two independent signals: the
repository's own build and tests, and the Codex review. Both must pass.

## When a review fails

The first version stops. The review is committed to the task branch with its
reasons and required changes, the task stays unchecked, and the loop waits for a
person. Fix the branch, then start the runner again with
`DEV_LOOP_ALLOW_EXISTING_BRANCH: "true"` to resume that task.

An automatic fix-and-re-review cycle with a retry cap is the natural next step,
but it is worth running the simple version first: the failures it surfaces tell
you whether the prompts or the task descriptions are the thing that needs work.

## How the pieces fit

Each pipeline step is one line: it calls a stage script from the toolkit. The
step manifests own orchestration - dependencies, per-step secrets, the container
image - and the stage scripts own what actually happens. That split is what lets
the whole loop be tested end to end without a running platform.

The one exception is the checkout step, whose script is inline in
`steps/platform/shared/dev-loop-checkout.yaml`. It runs before the repository
exists in the workspace, so it cannot call a script from inside it.

## Running the tests

```bash
examples/codex-development-loop/tests/run-script-tests.sh
```

```bash
examples/codex-development-loop/tests/run-loop-integration-test.sh
```

```bash
go test -run CodexDevelopmentLoop ./...
```

The first suite unit-tests the primitives. The second builds a Git repository
and a bare origin in a temporary directory, installs the toolkit into it, and
runs the real stage scripts through two complete rounds with a fake Codex CLI -
including the shipped checkout script, lifted straight out of its step manifest.
It then drives each guard: a planner that touches source, an implementation that
checks off its own task, an implementation that changes nothing, a review with
no verdict, a review that edits the branch, a passing review over failing tests,
a branch that moved since submission, and a task branch that already exists.

The Go test validates both pipelines against the platform's pipeline schema and
checks that variables, secrets, trigger mappings, and stage-script names line up
across the files.
