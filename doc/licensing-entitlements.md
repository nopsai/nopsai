# Licence Keys and Entitlements

NopsAI container images are publicly pullable, so pulling an artifact is not
what decides whether an installation may use it. The licence key is. This
document describes the key format, how an installation is entitled, and exactly
where enforcement applies.

For the acceptance step that precedes all of this, see
[first-install-wizard.md](./first-install-wizard.md). For the ownership and
distribution model, see
[licensing-and-distribution.md](./licensing-and-distribution.md).

## The Key Is Not A Secret

A licence key is signed data stating what an installation may run. It is not a
credential. Leaking one grants nobody any right that the licence agreement did
not already grant, which is why the key lives in plain GitOps configuration
rather than in the encrypted credential registry.

The signing key is the secret. It never leaves the issuing environment and is
never shipped in an artifact.

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

# Per customer
NOPSAI_LICENSE_PRIVATE_KEY=... go run ./internal/tools/licenseissuer \
  -licensee "Acme BV" -license-id lic-001 -tier enterprise \
  -days 365 -max-users 50 -features sso,kubernetes-runner
```

The public key is compiled into the product with
`-ldflags "-X nopsai/pkg/buildinfo.LicensePublicKey=<public key>"`.

The issuer lives in `internal/tools/`, not `cmd/`, so the release pipeline —
which builds `./cmd/...` — never ships it.

## Evaluation Mode

An installation with no key runs under evaluation limits: 5 users, 2 teams, 2
concurrent runs. These are deliberately usable. The point of publishing images
openly is to let someone evaluate NopsAI without talking to sales, and limits
that make the product unevaluable would defeat that.

An untrustworthy key grants exactly what no key grants. Expired, malformed,
signed by another issuer, or unreadable all resolve to evaluation limits with a
reason naming what to fix. A bad key is never treated as a good one, and never
as worse than no key.

## Where Enforcement Applies

Enforcement only runs in builds that carry a verification key.

This is the important boundary. A build compiled without
`buildinfo.LicensePublicKey` cannot distinguish a licensed installation from an
unlicensed one. Applying evaluation ceilings there would penalise the build
configuration rather than the operator, and would silently cap every
installation that predates licensing. Such a build reports its situation and
enforces nothing.

Within a verifying build:

| Point | Behaviour |
| --- | --- |
| Production startup gate | An installation in production mode without a valid key fails its startup gate rather than running unlicensed by accident. |
| `POST /v1/users` | A request that would add a seat beyond the limit is refused with `402 Payment Required`. An upsert of an existing user is not a new seat and is not checked. |
| `POST /v1/teams` | Refused with `402` beyond the team limit, after authorization so an unauthorized caller learns nothing about the entitlement. |

Every denial is written to the audit trail as `system.license.denied` with the
resource and reason.

### Fail Closed Means Refuse, Not Crash

Enforcement fails closed in the sense that matters: if current usage cannot be
read, the action is **refused**, because being unable to evaluate a limit is not
the same as being under it.

It does not fail closed by refusing to boot. An installation that cannot read
its key must still start, so an operator can log in and fix it. Refusing to
start is reserved for the production startup gate, where an administrator has
explicitly declared the installation to be production.

## Reading The Entitlement

```bash
nopsai license status
nopsai license status --output json
```

The interactive console shows the same entitlement above the notice under
**Home → license**, so the console covers the same ground as the CLI.

`GET /v1/system/license` returns the entitlement, its limits, and current usage
against them. It reports no key material — only what the installation may run.
