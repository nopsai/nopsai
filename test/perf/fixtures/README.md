# Performance harness fixtures

The pipeline suite drives runs through a purpose-built pipeline rather than by
replaying a git event. These two files are that fixture.

| File | What it is |
| --- | --- |
| `pipelines/perf-load-probe.yaml` | A deterministic, no-LLM pipeline whose only job is to be measured. |
| `external-triggers/perf-load-probe.yaml` | The trigger the harness invokes to start it. |

## Why a fixture instead of a git webhook

Replaying a git event makes the platform resolve the pipeline definition from
the repository *at the payload's commit SHA*. That ties every measurement to a
real commit in a real repository, puts third-party API latency inside the
numbers, and creates real check runs as a side effect of running a test.

The fixture removes all of that. `llm_enabled: false` and `script` steps mean no
model calls, so runs are free, deterministic, and repeatable. The invoke
response names the run it created, so the harness watches exactly the run it
started instead of guessing which one is its own.

What the suite then measures is the platform's own orchestration cost: queue
time, dispatch, runner startup, step execution, and completion tracking.

## Installing

These are configuration resources, so they belong in Git like everything else in
NopsAI. They are not created at runtime by the harness.

1. Copy the two files into your config repositories:
   - `pipelines/perf-load-probe.yaml` → your **team** config repository, under
     its pipelines directory.
   - `external-triggers/perf-load-probe.yaml` → the config repository that owns
     external triggers for that team.

2. Edit the environment-specific fields in the trigger. The shipped values
   assume a team named `team-1` and the default local administrator:

   ```yaml
   pipeline: team-1/perf-load-probe   # must match where you installed the pipeline
   scope: team-1/dev
   run_team_path: team-1
   allowed_callers:
     - type: user
       id: admin@example.com          # the identity running the load test
   ```

3. Commit, push, and let config sync apply them. Confirm both landed:

   ```bash
   curl -s -H "Authorization: Bearer $TOKEN" \
     http://127.0.0.1:8080/v1/external-triggers | jq -r '.[].id'
   ```

4. Run the suite:

   ```bash
   test/perf/run-perf-test.sh --preset full
   ```

If the trigger is missing, the harness fails immediately and points back here
rather than waiting on runs that will never appear.

## Tuning the shape of a run

`--pipeline-work-seconds` sets how long the probe's work step sleeps:

- `0` measures pure orchestration overhead — how much the platform costs to
  start and finish a run that does nothing.
- A larger value models a realistic pipeline while keeping the work identical
  across runs, so concurrency levels stay comparable.

The default is 2 seconds. Because the step sleeps rather than spins, it does not
compete with the services under test for CPU.
