# Tests

Every fixture in both suites is built in a temporary directory, so neither
depends on a real repository, commit, credential, or network service.

## `run-script-tests.sh`

Unit tests for the deterministic primitives: task selection, task completion,
verdict parsing, prompt rendering, and repository validation. It also renders
each shipped prompt with the values its pipeline passes, so a typo in a
placeholder name fails here rather than mid-run.

```bash
examples/codex-development-loop/tests/run-script-tests.sh
```

## `run-loop-integration-test.sh`

End-to-end test of the loop itself. It creates a bare origin and a working
repository, installs the toolkit into it, and runs the real stage scripts
through two complete rounds - including the checkout script, extracted from
`steps/platform/shared/dev-loop-checkout.yaml` so the shipped version is what
gets tested.

`fake-codex` stands in for the model. It is the only fake: `FAKE_CODEX_MODE`
selects a behaviour, including the misbehaviours the loop's guards exist to
catch, so each guard is proven against the thing it is supposed to stop rather
than against a mock of it.

The two stages that call the NopsAI API are not covered; they need a running
platform. Everything that decides what happens is.

```bash
examples/codex-development-loop/tests/run-loop-integration-test.sh
```

## Go tests

`codex_development_loop_test.go` in the repository root validates both pipelines
against the platform's pipeline schema and checks that variables, secrets,
trigger mappings, stage-script names, and documentation links agree across the
files.

```bash
go test -run CodexDevelopmentLoop ./...
```
