# Public Distribution Controls

NopsAI's source, container images and CLI archives are published openly, because
the licence grants non-commercial use to anyone. Publishing openly puts four
things on the critical path that a private repository never had to think about:
attribution, credential hygiene, an acceptance record, and a way to record a
commercial entitlement. This document describes how each is handled.

For the licence model itself, see
[licensing-and-distribution.md](./licensing-and-distribution.md). For the key
format and where entitlements are consulted, see
[licensing-entitlements.md](./licensing-entitlements.md).

## What Open Distribution Does And Does Not Grant

Pulling an artifact grants exactly what the `LICENSE` in it grants: use for any
noncommercial purpose, plus modification and redistribution for noncommercial
purposes, provided the licence and the `Required Notice:` line travel with every
copy.

Commercial use is not granted by it, and is not gated at pull time either.
Gating the pull would break the free non-commercial use the licence promises,
add `imagePullSecrets` support load, and still not bind anyone to terms.
Registry credentials are therefore deliberately not part of the model.

## 1. Release-Specific Third-Party Notice Bundle

`THIRD_PARTY_NOTICES.md` is a notice *index* and says so. Attribution
obligations attach to actual licence texts, which is what the bundle carries.

- `scripts/generate-notices.sh` emits
  `dist/notices/THIRD-PARTY-NOTICES-<version>.md` with, per component: name,
  version, licence identifier, and the full licence text as shipped by that
  component.
- `scripts/license-check.sh --classify FILE` lets the dependency gate and the
  bundle agree on what a licence file says, rather than each reimplementing the
  classification.
- Go modules are resolved through `go list -m -json all`; UI packages through
  `npm ls --omit=dev --all --parseable`.
- The generator fails closed: a distributed component with no reproducible
  licence text exits non-zero rather than emitting a partial bundle.
- A `release-notices` step in `release/nopsai-platform-release.yaml` generates
  the bundle. `publish-base-image`, `package-helm-chart`, `build-cli-archives`
  and `package-release-assets` all depend on it, so no artifact family can be
  built without it.
- `TestReleasePipelineGeneratesThirdPartyNoticeBundle` asserts the step, the
  four dependency edges, and the generator's fail-closed behaviour.

### Scope: Production Dependencies Only

The first full run recorded 615 components and failed on 17 with no licence
text. All 17 were build or test tooling — ESLint plugins, platform-specific
`@rollup`/`@esbuild` binaries, test utilities — none of which reach a recipient,
because the UI image serves built static assets.

Attribution obligations attach to what is *distributed*, so the UI sweep is
scoped to production dependencies. The bundle records 292 components, every one
with reproducible licence text. `INCLUDE_DEV_DEPENDENCIES=1` restores the full
tree for an internal audit.

## 2. Secret And Content Scan Gate

Public images make any committed credential permanently public, so the scan runs
before anything is published rather than after.

- `scripts/secret-scan.sh` scans git-tracked files, an extracted rootfs
  (`--path`), or a container image (`--image`). It uses a built-in
  provider-shaped pattern set rather than a downloaded scanner, so it adds no
  unpinned third-party binary to the release path and runs offline. Swapping in
  gitleaks later requires pinning it in the prepared release toolchain images
  alongside the other release tools.
- `scripts/secret-scan-allowlist.txt` holds documentation placeholders and test
  fixtures, matched against `<path>:<line>:<content>`.
- Image scanning also fails on forbidden paths in a published layer: `/.git`,
  `/.env`, `/root/.docker/config.json`, `/root/.kube/config`, and `/root/.ssh`.
- `scripts/enterprise-gates.sh` runs the worktree scan next to the licence gate
  and scans every locally built image, so the check gates local runs and the
  pipeline's `quality-gates` step alike.
- A `scan-published-images` step in `release/nopsai-platform-release.yaml` pulls
  each published digest and scans it. `publish-helm-chart` depends on it, so a
  finding fails the release before the chart and the GitHub release make the
  version discoverable.
- `TestReleasePipelineScansForCredentialsBeforePublication` in
  `release/release_test.go` asserts the pipeline wiring, the enterprise-gate
  wiring, and the fail-closed behaviour of the script itself.

The scan is fail-closed by construction: any finding exits non-zero. An early
revision counted findings inside a pipeline subshell and exited clean while
printing them, so the test explicitly forbids piping into `scan_stream`.

## 3. Licence Acceptance In The First-Install Wizard

Anonymous pull means an installation can exist without anyone having read the
licence. Acceptance is what turns a pull into an installation that has agreed to
terms — including the warranty disclaimer, the liability limitation, and the
statement that commercial use needs a separate agreement.

Acceptance is recorded in the existing `setup_state` key/value table, so no
schema migration was needed: `license_accepted_at`, `license_accepted_by`,
`license_document_version`, `license_document_sha256`.

- `pkg/licensenotice` holds the notice, its document version, and its SHA-256.
  `TestLicenseNoticeMatchesShippedLicenseFile` fails if it drifts from the root
  `LICENSE`, so the accepted notice is the shipped notice.
- `GET /v1/setup/license` returns the text, version, digest, and acceptance
  state. Public, like `/v1/setup/preflight` — the licence must be readable
  before it can be accepted, and it already ships publicly in every image.
- `POST /v1/setup/license/accept` requires `system.update` on `system:config`
  through the existing `/v1/setup/` route authorization, and writes a
  `system.license.accept` audit entry.
- Licence is the **first** required wizard step, ahead of readiness.
- `SetupLicenseStep.tsx` shows the full licence, an explicit checkbox, and a
  disabled continue button until it is ticked. Body text uses `--text-primary`
  at `text-sm leading-6` so the notice stays readable in dark mode rather than
  fading into secondary grey.
- Re-prompt on upgrade: acceptance is bound to the document digest, so a changed
  notice makes `accepted` false again and names the version previously agreed
  to.
- Documented in `doc/first-install-wizard.md` and `doc/api.md`.

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

## 4. Commercial Licence Keys

Full detail in [licensing-entitlements.md](./licensing-entitlements.md).

- `pkg/license` defines the claims, Ed25519 signing and verification, and
  entitlement resolution.
- Offline by construction: verification is a local signature check. No
  activation call, no usage reporting, no network-dependent validation.
- `internal/tools/licenseissuer` generates the issuing keypair and signs keys.
  It lives outside `cmd/`, so the release pipeline, which builds `./cmd/...`,
  never ships it.
- The key is Git-owned: `license_key` in `setting/system/runner.yaml`,
  round-tripping through config sync and the settings export.
  `NOPSAI_LICENSE_KEY` covers bootstrap. It is signed data rather than a
  credential, so it is not held in the encrypted credential registry.
- The entitlement resolves per request, so a key changed through config sync
  applies without a restart and a key that lapses stops granting anything.
- No key means the non-commercial licence: unlimited users, teams and concurrent
  runs. An untrustworthy key grants exactly what no key grants — never more,
  never less.
- Where a commercial key records a limit, it is enforced at `POST /v1/users` and
  `POST /v1/teams`, answering `402 Payment Required`, checked after
  authorization, and skipped for an upsert that adds no seat.
- Every denial audited as `system.license.denied`.
- `GET /v1/system/license`, `nopsai license status`, and the interactive console
  entry under **Home → license**, so CLI and console stay at parity.

### Why Nothing Blocks Startup

Enforcement applies only to builds carrying a verification key, and only where
the entitlement records a limit. Neither condition is about punishing an
operator: a build that cannot verify anything must not cap installations that
predate licensing, and a non-commercial installation has no ceiling to check.

The production startup check reports the licence position and never blocks. A
gate there would stop the non-commercial production use the licence grants,
while a business that declines to declare itself would sail past it. That is
why the commercial boundary is self-certified against the licence — see clause
14 of the commercial agreement — rather than decided at runtime.
