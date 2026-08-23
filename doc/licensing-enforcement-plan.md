# Licensing and Enforcement Implementation Plan

This plan takes NopsAI from "proprietary notice in a file" to "distributable
proprietary product with an enforceable acceptance record and a working
entitlement gate". It exists because the public-distribution decision (freely
pullable images, gated use) only holds up if the four items below are actually
built.

Status legend: `[ ]` not started, `[~]` in progress, `[x]` done.

## Distribution Model This Plan Assumes

Container images and CLI archives are **publicly pullable** from GHCR. Public
pull visibility is not a licence grant: `LICENSE` states that possession or
access grants no rights, and every artifact carries that notice plus the
`LicenseRef-NopsAI-Proprietary` OCI identifier.

Rights are therefore controlled at **use** time, not at **pull** time:

1. copyright alone makes unlicensed use infringement, with no acceptance needed;
2. the first-install licence acceptance creates the contract that adds warranty
   disclaimer, liability limits, and no-reverse-engineering terms; and
3. the licence key decides what an installation is entitled to run.

Registry credentials are deliberately **not** part of the enforcement story.
Gating the pull would break self-serve evaluation, add `imagePullSecrets`
support load, and still not bind anyone to terms.

## Stage 1 — Release-Specific Third-Party Notice Bundle — DONE

`THIRD_PARTY_NOTICES.md` is a notice *index* and says so. It requires a
release-specific bundle containing actual licence texts and attributions before
external distribution. `scripts/license-check.sh` gates *which* licences are
allowed; it did not produce the notice bundle.

- [x] `scripts/generate-notices.sh` emits
      `dist/notices/THIRD-PARTY-NOTICES-<version>.md` with, per component:
      name, version, licence identifier, and the full licence text as shipped
      by that component.
- [x] `scripts/license-check.sh --classify FILE` was added so the gate and the
      bundle agree on what a licence file says, instead of reimplementing the
      classification.
- [x] Go modules are resolved through `go list -m -json all`; UI packages
      through `npm ls --omit=dev --all --parseable`.
- [x] The generator fails closed: a distributed component with no reproducible
      licence text exits non-zero rather than emitting a partial bundle.
- [x] A `release-notices` step in `release/nopsai-platform-release.yaml`
      generates the bundle. `publish-base-image`, `package-helm-chart`,
      `build-cli-archives`, and `package-release-assets` all depend on it, so
      no artifact family can be built without it.
- [x] `TestReleasePipelineGeneratesThirdPartyNoticeBundle` asserts the step,
      the four dependency edges, and the generator's fail-closed behaviour.

### Scope Decision: Production Dependencies Only

The first full run recorded 615 components and failed on 17 with no licence
text. All 17 were build or test tooling — ESLint plugins, platform-specific
`@rollup`/`@esbuild` binaries, test utilities — none of which reach a customer,
because the UI image serves built static assets.

Attribution obligations attach to what is *distributed*, so the UI sweep is
scoped to production dependencies. The bundle now records 292 components, every
one with reproducible licence text, and exits clean. `INCLUDE_DEV_DEPENDENCIES=1`
restores the full tree for an internal audit.

### Remaining

- [ ] Copy the bundle into the images at
      `/usr/share/licenses/nopsai/THIRD-PARTY-NOTICES.md`, into the CLI
      archives, and into the Helm chart root. The step produces the file and
      the artifact steps depend on it; the per-Dockerfile and chart copies are
      still to be added.
- [ ] Attach the bundle to the GitHub Release next to `SHA256SUMS`.
- [ ] Sweep base-image OS packages. `collect_os_packages` exists behind
      `--image` but records identifiers and descriptions only, not licence
      texts, so Alpine's source-offer obligation is not yet satisfied.

## Stage 2 — Secret And Content Scan Gate — DONE

Public images make any committed credential permanently public. One real
credential was found in this repository's history (see "Known Credential
Exposure" below), which is exactly the failure this gate prevents.

- [x] `scripts/secret-scan.sh` scans git-tracked files, an extracted rootfs
      (`--path`), or a container image (`--image`). It uses a built-in
      provider-shaped pattern set rather than a downloaded scanner, so it adds
      no unpinned third-party binary to the release path and runs offline.
      Swapping in gitleaks later only requires pinning it in
      `scripts/install-release-tools.sh` alongside the other tools.
- [x] `scripts/secret-scan-allowlist.txt` holds documentation placeholders and
      test fixtures, matched against `<path>:<line>:<content>`.
- [x] Image scanning also fails on forbidden paths in a published layer:
      `/.git`, `/.env`, `/root/.docker/config.json`, `/root/.kube/config`,
      and `/root/.ssh`.
- [x] `scripts/enterprise-gates.sh` runs the worktree scan next to the licence
      gate and scans every locally built image, so the check gates local runs
      and the pipeline's `quality-gates` step alike.
- [x] A `scan-published-images` step in `release/nopsai-platform-release.yaml`
      pulls each published digest and scans it. `publish-helm-chart` now
      depends on it, so a finding fails the release before the chart and the
      GitHub release make the version discoverable.
- [x] `TestReleasePipelineScansForCredentialsBeforePublication` in
      `release/release_test.go` asserts the pipeline wiring, the enterprise-gate
      wiring, and the fail-closed behaviour of the script itself.

The scan is fail-closed by construction: any finding exits non-zero. An early
revision counted findings inside a pipeline subshell and exited clean while
printing them, so the test explicitly forbids piping into `scan_stream`.

## Stage 3 — Licence Acceptance In The First-Install Wizard — DONE

There was no acceptance record anywhere in the product. Without one, the
warranty disclaimer and liability limitation bind nobody who obtained the
software by anonymous pull.

Acceptance is recorded in the existing `setup_state` key/value table, so no
schema migration was needed: `license_accepted_at`, `license_accepted_by`,
`license_document_version`, `license_document_sha256`.

- [x] `pkg/licensenotice` holds the notice, its document version, and its
      SHA-256. `TestLicenseNoticeMatchesShippedLicenseFile` fails if it drifts
      from the root `LICENSE`, so the accepted notice is the shipped notice.
- [x] `GET /v1/setup/license` returns the text, version, digest, and acceptance
      state. Public, like `/v1/setup/preflight` — the notice must be readable
      before it can be accepted, and it already ships publicly in every image.
- [x] `POST /v1/setup/license/accept` requires `system.update` on
      `system:config` through the existing `/v1/setup/` route authorization,
      and writes a `system.license.accept` audit entry.
- [x] Licence is the **first** required wizard step, ahead of readiness.
- [x] `SetupLicenseStep.tsx` shows the full scrollable notice, an explicit
      checkbox, and a disabled continue button until it is ticked. Body text
      uses `--text-primary` at `text-sm leading-6` so the notice stays readable
      in dark mode rather than fading into secondary grey.
- [x] Re-prompt on upgrade: acceptance is bound to the document digest, so a
      changed notice makes `accepted` false again and names the version
      previously agreed to.
- [x] Documented in `doc/first-install-wizard.md` and `doc/api.md`.

### Fail-Closed In Three Places

The gate does not depend on the UI behaving:

1. The accept endpoint refuses a digest mismatch (`409`), so a stale browser tab
   cannot accept superseded wording.
2. `markSetupComplete` refuses to write `completed_at` while the current notice
   is unaccepted, **and refuses equally when acceptance cannot be evaluated**.
   Bootstrap answers `412 Precondition Failed`. Since the first-install gate
   already blocks the rest of the API until `completed_at` exists, an
   installation that never accepts never becomes usable.
3. The wizard disables Continue and Apply setup until acceptance is recorded.

### Still Open

- [ ] Surface the acceptance record in the About dialog and in
      `nopsai license`. Both currently quote the notice but do not show who
      accepted it or when. `contract/about_dialog_license_test.go` keeps the two notice
      copies identical, so any change has to move both together.

## Stage 4 — Licence Key And Entitlements — DONE

Full detail in [licensing-entitlements.md](./licensing-entitlements.md).

- [x] `pkg/license` defines the claims, Ed25519 signing and verification, and
      entitlement resolution. 27 tests, 90.9% statement coverage.
- [x] Offline by construction: verification is a local signature check. No
      activation call, no usage reporting, no network-dependent validation.
- [x] `internal/tools/licenseissuer` generates the issuing keypair and signs
      keys. It lives outside `cmd/`, so the release pipeline, which builds
      `./cmd/...`, never ships it.
- [x] The key is Git-owned: `license_key` in `setting/system/runner.yaml`,
      round-tripping through config sync and the settings export.
      `NOPSAI_LICENSE_KEY` covers bootstrap. It is signed data rather than a
      credential, so it is not held in the encrypted credential registry.
- [x] The entitlement resolves per request, so a key changed through config sync
      applies without a restart and a key that lapses stops granting anything.
- [x] Evaluation mode: 5 users, 2 teams, 2 concurrent runs, with a reason naming
      what to fix. An untrustworthy key grants exactly what no key grants —
      never more, never less.
- [x] Production startup gate through `enterpriseStartupGateChecks`.
- [x] Caps enforced at `POST /v1/users` and `POST /v1/teams`, answering
      `402 Payment Required`, checked after authorization, and skipped for an
      upsert that adds no seat.
- [x] Every denial audited as `system.license.denied`.
- [x] `GET /v1/system/license`, `nopsai license status`, and the interactive
      console entry under **Home → license**, so CLI and console stay at parity.
- [x] End-to-end verified: issued a real key, confirmed the claims resolve, and
      confirmed a key presented against a different issuer falls back to
      evaluation with a signature-failure reason.

### The Enforcement Boundary

Enforcement and the production gate apply **only to builds that carry a
verification key**.

A build compiled without `buildinfo.LicensePublicKey` cannot tell a licensed
installation from an unlicensed one. Applying evaluation ceilings there would
penalise the build configuration rather than the operator, and would cap every
installation that predates licensing — including every build that exists today,
since no issuing keypair has been created yet. Such a build reports its state
and enforces nothing.

This is what makes the change safe to merge before any key is issued.

### Fail Closed Means Refuse, Not Crash

If usage cannot be read, the action is refused: being unable to evaluate a limit
is not the same as being under it. But an installation that cannot read its key
still starts, so an operator can log in and fix it. Refusing to start is
reserved for the production gate, where production was explicitly declared.

### Still Open

- [ ] A UI banner naming the evaluation cap and remaining headroom.
      `GET /v1/system/license` already returns limits and usage for it.
- [ ] Concurrent-run enforcement. The limit is carried in the claims and
      reported, but the dispatcher does not consult it yet.
- [ ] Create the production issuing keypair and compile the public key into
      release builds. Until that happens, every build enforces nothing by
      design.

## Stage 5 — Repository And Release Hygiene

Completed as part of this effort:

- [x] `.nopsai/` untracked from the source repository. The platform release
      pipeline moved to `release/nopsai-platform-release.yaml` as the tested
      reference copy; the live pipeline and push trigger are owned by the
      GitOps config repositories.
- [x] `ngrok-docker-compose.yaml` untracked and ignored; local tunnelling
      documented in `doc/webhook-tunnelling.md`.
- [x] `AGENTS.md` untracked and ignored as local assistant instructions.

Remaining:

- [ ] Rotate the exposed ngrok authtoken and purge it from history.
- [ ] Rename the tracked `.env` to `.env.example` and ignore `.env`.
- [ ] Add `.github/CODEOWNERS`, a pull-request template, and a
      `.github/workflows/` CI entry point, or record deliberately that CI runs
      only through NopsAI's own pipelines.
- [ ] Add `.gitattributes` normalising line endings before external
      contributors or customers receive source.

## Known Credential Exposure

`ngrok-docker-compose.yaml` carried a real `NGROK_AUTHTOKEN` and a reserved
personal ngrok domain, committed across multiple commits in the history of this
repository. Untracking the file stops future exposure but does not remove the
token from existing history or from any clone.

Required actions, in order:

1. Revoke the token in the ngrok dashboard. This is the only step that actually
   invalidates it.
2. Issue a replacement token and keep it in the shell environment or a secret
   manager, never in a file inside the repository.
3. Purge the value from history with `git filter-repo` before the repository is
   ever made public or shared with a customer, then force-push and have every
   clone re-cloned.

Step 1 is not optional and does not depend on steps 2 and 3.

## Sequencing

Stages 1 and 2 gate the first public release. Stage 3 gates the first external
evaluator. Stage 4 gates the first paid deal. Stage 5 items are independent and
can land at any point, except the credential rotation, which is immediate.
