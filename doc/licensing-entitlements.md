# Licence Keys and Entitlements

NopsAI is free and uncapped for any non-commercial purpose, so most
installations have no key and nothing to enforce. A licence key exists for the
other case: it records a commercial entitlement and whatever scope was agreed
for it.

For the acceptance step that precedes all of this, see
[first-install-wizard.md](./first-install-wizard.md). For the ownership and
distribution model, see
[licensing-and-distribution.md](./licensing-and-distribution.md).

## What A Key Is And Is Not

A licence key is signed data stating what a commercially licensed installation
may run. It is not a credential. Leaking one grants nobody any right that the
licence agreement did not already grant, which is why the key lives in plain
GitOps configuration rather than in the encrypted credential registry.

The signing key is the secret. It never leaves the issuing environment and is
never shipped in an artifact.

A key is also not what makes NopsAI usable. An installation without one is a
complete product running under the public non-commercial licence, not a trial.

## Offline Verification

Verification is a local Ed25519 signature check. NopsAI never calls home to
validate a key, for the same reason it never needs a public URL: an air-gapped
installation must be able to prove its entitlement with nothing but the key and
the public key compiled into the binary.

There is no activation call, no usage reporting, and no network-dependent
validation. A key is verified identically on a laptop and in an isolated
network.

The signed claims are:

```
licensee, license_id, tier, issued_at, not_before, expires_at,
max_users, max_teams, max_concurrent_runs, features[]
```

A limit of zero means **unlimited**. An omitted limit must never read as a
ceiling of nothing, which would make a paid key more restrictive than no key at
all.

## Tiers

There are two, and they mirror the licence rather than a price list:

| Tier | How an installation gets it | Limits |
| --- | --- | --- |
| `noncommercial` | No key, or a key that cannot be trusted. | None. Every limit resolves to unlimited. |
| `commercial` | A key that verifies. | Whatever the Order Form recorded, which may also be unlimited. |

## Configuration

The key is a configuration resource, so it is Git-owned like every other
setting. Set `license_key` in `setting/system/runner.yaml` in the global config
repository:

```yaml
license_key: eyJsaWNlbnNlZSI6...
```

`NOPSAI_LICENSE_KEY` sets the same value for a bootstrap or local run. The
entitlement is resolved per request rather than cached, so a key changed through
config sync takes effect without a restart, and a key that lapses while the
process runs stops granting anything at the moment it expires.

## Issuing

```bash
# Once, to create the issuing identity
go run ./internal/tools/licenseissuer -generate-keypair

# Per commercial customer
NOPSAI_LICENSE_PRIVATE_KEY=... go run ./internal/tools/licenseissuer \
  -licensee "Acme BV" -license-id lic-001 -tier commercial \
  -days 365 -max-users 50 -features sso,kubernetes-runner
```

The public key is compiled into the product with
`-ldflags "-X nopsai/pkg/buildinfo.LicensePublicKey=<public key>"`.

The issuer lives in `internal/tools/`, not `cmd/`, so the release pipeline —
which builds `./cmd/...` — never ships it.

## Non-Commercial Mode

An installation with no key runs under the non-commercial licence: unlimited
users, unlimited teams, unlimited concurrent runs, every feature. There is
nothing to unlock, because the licence it ships under already grants the whole
product for any non-commercial purpose.

An untrustworthy key grants exactly what no key grants. Expired, malformed,
signed by another issuer, or unreadable all resolve to the non-commercial
licence with a reason naming what to fix. A bad key is never treated as a good
one, and never as worse than no key.

## Where Enforcement Applies

Enforcement runs only where two things are true: the build carries a
verification key, and the resolved entitlement actually records a limit.

The first condition is a build boundary. A build compiled without
`buildinfo.LicensePublicKey` cannot distinguish a commercially licensed
installation from any other, so applying ceilings there would penalise the build
configuration rather than the operator.

The second is the common case. Non-commercial installations carry no limits, so
enforcement returns before it reads anything at all.

Within a verifying build, where a commercial key records a limit:

| Point | Behaviour |
| --- | --- |
| `POST /v1/users` | A request that would add a seat beyond the limit is refused with `402 Payment Required`. An upsert of an existing user is not a new seat and is not checked. |
| `POST /v1/teams` | Refused with `402` beyond the team limit, after authorization so an unauthorized caller learns nothing about the entitlement. |

Every denial is written to the audit trail as `system.license.denied` with the
resource and reason.

### Nothing Blocks Startup

The production startup check reports the licence position and never blocks on
it, in production or anywhere else.

Whether a purpose is commercial is a fact about the operator, not about the
deployment, and the software has no way to observe it. Refusing to start without
a key would block the non-commercial production use the licence expressly
grants, while doing nothing at all to a business that simply declines to declare
itself. So the check states the position — including, in production mode, that
commercial use needs an agreement — and lets the installation run.

### Fail Closed Means Refuse, Not Crash

Where a limit exists, enforcement fails closed in the sense that matters: if
current usage cannot be read, the action is **refused**, because being unable to
evaluate a limit is not the same as being under it.

Where no limit exists there is nothing to evaluate and nothing to fail closed
on, so an unreachable count is not turned into a refusal against an installation
that has no ceiling in the first place.

## Reading The Entitlement

```bash
nopsai license status
nopsai license status --output json
```

The interactive console shows the same entitlement above the notice under
**Home → license**, so the console covers the same ground as the CLI.

`GET /v1/system/license` returns the entitlement, its limits, and current usage
against them. It reports no key material — only what the installation may run.
