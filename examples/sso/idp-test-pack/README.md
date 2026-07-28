# NopsAI IdP Integration Test Pack

This pack lives under `examples/sso/idp-test-pack` and provides runnable SSO
fixtures for exercising NopsAI identity-provider login and authorization
mapping behavior.

Use these files as test examples. Copy one scenario's
`setting/system/auth.yaml` into the global config repository when you want
GitOps-managed SSO settings, but keep provider users, group membership, and
IdP-managed grants as runtime identity state.

## What the implementation actually supports

NopsAI validates an OIDC ID token, reads one configurable string/list claim via
`team_claim`, and then applies three independent mapping lanes:

- `team_mapping`: external group -> NopsAI AAA **auth team** membership.
- `basic_role_mapping`: external group -> scoped `viewer`, `developer`, or
  `owner` access grant on a resource such as `team:team-1`.
- `role_mapping`: external group -> global access role. This pack intentionally
  leaves it empty because the requested tests concern team-scoped authorization.

The `auth-only` scenarios still read the `groups` claim, but define no mappings.
This proves that users can authenticate without accidentally receiving team
permissions.

**Important:** NopsAI does not implement a "must belong to login group" check.
The `nopsai-auth-only` group is intentionally unmapped; it is not a login
allowlist. Enforce app assignment at the provider or add an explicit allowed-
groups feature if group membership must gate authentication.

Only one external provider may be enabled at a time, so apply one scenario
configuration per test run.

## Six users and four groups

| Login | Groups | Auth-and-team-authz result |
|---|---|---|
| `alice` | none | Authentication only |
| `bob` | auth-only | Authentication only |
| `carol` | team-1-viewers | `viewer` on `team:team-1` |
| `dave` | team-1-viewers + team-1-developers | `developer` on `team:team-1` |
| `erin` | data-team-owners | `owner` on `team:data-team` |
| `frank` | team-1-viewers + data-team-owners | viewer on team-1; owner on data-team |

`team-1-developers` is the logical subgroup of `team-1-viewers`. The mock emits
both parent and child membership for `dave`. NopsAI does not calculate group
hierarchies itself; it maps the exact claim values it receives.

## Run the mock IdP

```bash
docker compose -f examples/sso/idp-test-pack/docker-compose.yml up -d mock-oauth2
```

The exact image defaults to `ghcr.io/navikt/mock-oauth2-server:5.0.2`. Override
it with `MOCK_OAUTH2_SERVER_VERSION` when intentionally testing another release.

Discovery URLs:

```text
http://host.docker.internal:8090/entra/.well-known/openid-configuration
http://host.docker.internal:8090/google/.well-known/openid-configuration
http://host.docker.internal:8090/okta/.well-known/openid-configuration
http://host.docker.internal:8090/keycloak/.well-known/openid-configuration
```

On Linux, ensure `host.docker.internal` resolves from both the browser host and
the NopsAI container. Alternatively replace it consistently with a LAN IP or a
local DNS name. The issuer must be identical in discovery, the ID token `iss`,
and NopsAI configuration.

The mock login page accepts either the short username or its full email address
without a password. Both forms are mapped explicitly so an unmatched login cannot
silently produce a default token without the required `email` claim:

```text
alice  or alice@example.com
bob    or bob@example.com
carol  or carol@example.com
dave   or dave@example.com
erin   or erin@example.com
frank  or frank@example.com
```

Validate the fixture mappings from the repository root:

```bash
python3 examples/sso/idp-test-pack/scripts/validate-fixtures.py
```

## Automatic user creation requires email

NopsAI rejects first-time SSO login when `auto_create_users: true` and the
verified identity has no `email` claim. Every normal user mapping in this pack
therefore includes a non-empty `email` and `email_verified: true`.

If you see `identity email is required for automatic user creation`, the most
likely cause is that the mock login value did not match any `requestMapping`,
so mock-oauth2-server issued its default token instead of the configured user
claims. Use one of the exact short-name or email values above. The included
validator also fails if a normal user mapping lacks an email.

## Apply a NopsAI scenario

Copy exactly one scenario's `setting/system/auth.yaml` into the global config
repository and sync it. No `access/all.yaml` is included: OIDC users and
IdP-managed grants are runtime identity state, and NopsAI auto-creates them.

Examples:

```text
examples/sso/idp-test-pack/nopsai-tests/entra-auth-only/setting/system/auth.yaml
examples/sso/idp-test-pack/nopsai-tests/entra-auth-and-team-authz/setting/system/auth.yaml
examples/sso/idp-test-pack/nopsai-tests/google-auth-only/setting/system/auth.yaml
examples/sso/idp-test-pack/nopsai-tests/google-auth-and-team-authz/setting/system/auth.yaml
examples/sso/idp-test-pack/nopsai-tests/okta-auth-only/setting/system/auth.yaml
examples/sso/idp-test-pack/nopsai-tests/okta-auth-and-team-authz/setting/system/auth.yaml
```

The mock does not validate a client secret, so the test configurations omit
`client_credential_ref`. Production provider configurations should use a
credential reference.

These examples exercise the same AAA ownership model as production SSO:
provider sync may create or prune only `source: idp` memberships and grants for
that provider. Local UI/API/GitOps grants must remain intact.

## Provider realism and known gaps

### Microsoft Entra ID

The mock uses UUID-valued `groups`, matching Entra's normal group-object-ID
claim. It emits both parent and child IDs for the subgroup user. Real Entra can
omit `groups` and emit an overage indication when membership exceeds the JWT
limit. Current NopsAI reads only the configured claim and does not implement
Microsoft Graph fallback, so overage requires an implementation change or a
different entitlement source.

### Okta

The mock uses group names. Configure the Okta authorization server or OIDC app
so the `groups` claim is included in the **ID token**. NopsAI does not retrieve
missing groups from UserInfo after validating the ID token.

### Google Workspace

The Google auth-only scenario is representative. The auth-and-team-authz
scenario is intentionally synthetic: the mock injects a `groups` claim so you
can test NopsAI's generic mapping code. Google's canonical ID-token claims do
not include Workspace groups, and current NopsAI has no Google Directory API
entitlement adapter. Real Google group-driven authorization therefore needs an
OIDC broker that injects groups, or a new Google Directory/Cloud Identity sync
implementation.

## Keycloak

The pack includes a fourth mock issuer, `keycloak`, with both auth-only and
claim-driven team-authorization configurations:

```text
examples/sso/idp-test-pack/nopsai-tests/keycloak-auth-only/setting/system/auth.yaml
examples/sso/idp-test-pack/nopsai-tests/keycloak-auth-and-team-authz/setting/system/auth.yaml
```

The Keycloak fixture emits full group paths in the ID-token `groups` claim.
NopsAI matches those strings exactly; it does not infer parent membership from a
subgroup. Dave therefore receives both the parent and child paths, and the
stronger `developer` mapping wins for `team:team-1`. Do not request a `groups`
OAuth scope unless your Keycloak realm actually defines it. Instead, configure a
Keycloak protocol mapper/client scope to include `groups` in the ID token.

A production-shaped claim-mapping example is also included at:

```text
examples/sso/idp-test-pack/nopsai-tests/keycloak-real-claim-mapping/setting/system/auth.yaml
```

For Keycloak client roles attached to groups, NopsAI additionally supports
`entitlement_sync.mode: keycloak_team_roles`. That mode calls the Keycloak Admin
API and cannot be simulated by mock-oauth2-server alone.

## GitHub

NopsAI supports GitHub through its dedicated OAuth2 flow, not through generic
OIDC. It loads the GitHub user, verified email, and team memberships from the
GitHub API; external team keys have the form `organisation/team-slug`. A real
GitHub configuration is included at:

```text
examples/sso/idp-test-pack/nopsai-tests/github-real-auth-and-team-authz/setting/system/auth.yaml
```

The callback path is `/v1/auth/oauth2/github/callback`, and the GitHub OAuth
app needs that exact callback URL. The pure NAV mock server is insufficient for
a complete GitHub team-authorization test because NopsAI calls GitHub's
`/user/teams` API directly. Use a real GitHub OAuth app, or add a configurable
GitHub API base URL and a small API stub for deterministic tests.

## Assertions

Use `test-matrix.yaml` for the expected users, auth-team memberships and scoped
roles. The highest role wins when two mapped groups target the same resource,
so `dave` must receive `developer`, not `viewer`, on `team:team-1`.

Also test removal: after a mapped group disappears and the user signs in again,
NopsAI should prune only rows owned by that IdP and preserve locally managed
grants.

## Explicit OIDC endpoints

Each NopsAI test configuration sets `authorization_endpoint`,
`token_endpoint`, `jwks_uri`, and `userinfo_endpoint` explicitly. NopsAI can
discover these from `{issuer}/.well-known/openid-configuration` when the first
three are omitted, but explicit values make Docker networking failures easier
to diagnose.

For each issuer `<provider>` the paths are:

```text
http://host.docker.internal:8090/<provider>/authorize
http://host.docker.internal:8090/<provider>/token
http://host.docker.internal:8090/<provider>/jwks
http://host.docker.internal:8090/<provider>/userinfo
```

The current NopsAI OIDC login implementation reads `sub`, `email`,
`email_verified`, and the configured team claim from the signed ID token. It
does not call UserInfo to recover a missing email or group claim, so those
claims must remain in the ID token fixtures.

## v6 Keycloak fixture correction

The Keycloak `requestMappings` email regular expressions now use the same
escaping as the working Entra, Google, and Okta mappings. Each user can log in
with either the short username (`alice`) or full email (`alice@example.com`).
The fixture validator checks both forms.
