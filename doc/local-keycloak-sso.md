# Local Keycloak SSO

This repo includes a development-only Keycloak realm so Enterprise SSO can be
tested with real OIDC redirects, users, and teams.

## Start Keycloak

```bash
docker compose up -d keycloak
```

This local fixture is configured for LAN testing at `http://192.168.1.143:8088`.
Use that browser-visible URL for login redirects so Keycloak's issuer matches
NopsAI's configured provider.

Admin console:

- URL: `http://192.168.1.143:8088/admin`
- Username: `admin`
- Password: `admin`

Seeded realm:

- Realm: `nopsai`
- Client ID: `nopsai`
- Client secret: `dev-nopsai-secret`
- Redirect URIs:
  - `http://192.168.1.143/` for post-logout return to the LAN UI
  - `http://192.168.1.143:8080/` for post-logout return through the direct backend LAN URL
  - `http://192.168.1.143/v1/auth/oidc/nopsai/callback` for LAN access with provider ID `nopsai`
  - `http://192.168.1.143:8080/v1/auth/oidc/nopsai/callback` for direct backend LAN access with provider ID `nopsai`
  - `http://192.168.1.143/v1/auth/oidc/keycloak/callback` for LAN access with provider ID `keycloak`
  - `http://192.168.1.143:8080/v1/auth/oidc/keycloak/callback` for direct backend LAN access with provider ID `keycloak`

Seeded users:

| Email | Password | Keycloak assignment | Expected NopsAI role |
| --- | --- | --- | --- |
| `sso-admin@example.com` | `qazwsx123456` | direct client role `nopsai-admin` | global `nopsai-admin` |
| `sso-operator@example.com` | `qazwsx123456` | direct client role `owner` | global `owner` |
| `sso-viewer@example.com` | `qazwsx123456` | direct client role `viewer` | global `viewer` |
| `alice@example.com` | `qazwsx123456` | team `/team-1` with client role `owner` | scoped `owner` on `team:team-1` |
| `jip@example.com` | `qazwsx123456` | team `/team-1/dev` with client role `owner` | scoped `owner` on `team:team-1/dev` |

Pipeline run visibility follows AAA inheritance. For the local fixture, `jip`
can see runs that inherit from `team:team-1/dev`, including repository
triggered runs from an app/repository registered under `team-1/dev`, even when
the triggered pipeline definition itself lives in the parent `team-1` team.
NopsAI may show parent team shells such as `/team-1` so scoped users can
navigate to their allowed child teams; those shells do not grant parent-team
operations or unrelated sibling visibility.

The realm fixture lives at `dev/keycloak/nopsai-realm.json`. It includes a
teams mapper that places Keycloak team names in the OIDC `teams` claim.
Keycloak's realm JSON field, protocol mapper implementation, and Admin API
paths use Keycloak's standard `groups` vocabulary while NopsAI exposes those
assignments as teams.
Users created or linked through OIDC are externally authenticated and can have
both Keycloak-owned and local grants. Keycloak-owned access-role and basic-role
assignments are resynced from Keycloak on login and by the OIDC entitlement
sync worker; local administrators can still add or remove local grants without
Keycloak sync pruning them.
When a user signs out of NopsAI, the UI revokes the NopsAI refresh token and
redirects through Keycloak's OIDC logout endpoint when available. Keycloak then
returns to the NopsAI origin, where the hash router sends the browser to login.
The local Keycloak client uses `post.logout.redirect.uris: "+"`, so Keycloak
validates post-logout returns against the normal client redirect URI list. The
next SSO login also sends `prompt=login` once, so switching from one Keycloak
test user to another does not silently reuse the previous Keycloak browser
session.

There are two SSO authorization lanes:

- Direct Keycloak client roles on the `nopsai` client become global NopsAI
  access roles. Use this for platform-wide `nopsai-admin`, `admin`, `owner`,
  `developer`, or `viewer` access.
- Keycloak team client roles on the `nopsai` client become scoped NopsAI
  Basic roles. The Keycloak team path is mapped to the NopsAI team target,
  so team `/team-1/dev` with client role `owner` becomes `owner` on
  `team:team-1/dev`.
- The local fixture does not set an OIDC default role. Users without direct
  client roles should not receive global `viewer` access automatically.

NopsAI uses `entitlement_sync.mode: keycloak_team_roles` for this fixture.
That sync calls the Keycloak Admin API after OIDC login and from a periodic
worker so it can keep the role-to-team pairing. A plain OIDC token claim such
as `teams: ["team-1"]` and `roles: ["owner"]` cannot reliably say which role
belongs to which team. Keycloak authorization is managed through client roles
and team role mappings in the Keycloak UI.

## Configure NopsAI

Use this provider when NopsAI runs in Docker Compose with Keycloak on the same
`nopsai-net` network. SSO settings are GitOps-managed; put the auth body in the
global config repository at `setting/system/auth.yaml`:

```yaml
local_enabled: true
oidc:
  enabled: true
  auto_create_users: true
  default_role: ""
  domain_mapping:
    example.com: nopsai
  providers:
    nopsai:
      type: oidc
      display_name: Local Keycloak
      issuer: http://192.168.1.143:8088/realms/nopsai
      authorization_endpoint: http://192.168.1.143:8088/realms/nopsai/protocol/openid-connect/auth
      token_endpoint: http://keycloak:8080/realms/nopsai/protocol/openid-connect/token
      jwks_uri: http://keycloak:8080/realms/nopsai/protocol/openid-connect/certs
      userinfo_endpoint: http://keycloak:8080/realms/nopsai/protocol/openid-connect/userinfo
      client_id: nopsai
      client_credential_ref: credential://system/oidc/nopsai/client-secret
      scopes: ["openid", "email", "profile"]
      allowed_email_domains: ["example.com"]
      allow_email_linking: true
      entitlement_sync:
        mode: keycloak_team_roles
        admin_base_url: http://keycloak:8080
        realm: nopsai
        admin_realm: master
        admin_client_id: admin-cli
        admin_username: admin
        admin_password_credential_ref: credential://system/oidc/nopsai/admin-password
        client_id: nopsai
        target_resource_type: team
```

`config.yml` still accepts the legacy nested `auth:` key for bootstrap-only or
non-GitOps deployments, but the checked-in local config intentionally leaves
OIDC out so the config repository remains the source of truth.

Use `setting/system/auth.yaml` for GitOps-managed auth settings. Create the
referenced client secret and admin password in **Credentials**, or sync
their encrypted envelopes from `setting/system/credentials.yaml`, before
testing login or entitlement sync.

The issuer is browser-visible because it must match the `iss` claim in
Keycloak ID tokens. The token, JWKS, and userinfo endpoints use the Docker
service name so the backend can reach Keycloak from inside the Compose network.
Keep the `public_url` runtime setting aligned with one of the client redirect
URIs above.
The callback path includes the NopsAI provider ID:
`/v1/auth/oidc/{provider-id}/callback`. If the provider ID is `nopsai`,
Keycloak must allow the `.../oidc/nopsai/callback` redirect URI.
Keep `teams` out of the requested scopes for this fixture. Keycloak emits the
`teams` claim through the client protocol mapper for visibility. Scoped Basic
roles come from the Keycloak Admin API sync above.
Leave `default_role` empty for this fixture so Keycloak remains the source of
truth for global access roles. Set it only when a provider should intentionally
grant every auto-created SSO user the same baseline global role.

The local fixture enables provider-scoped `allow_email_linking` so a verified
Keycloak email can attach to an existing NopsAI user. This keeps local testing
usable when the Keycloak realm is recreated and user subject IDs change. For
production, enable this only for trusted providers that assert verified email
ownership.

For production Keycloak, prefer a dedicated confidential admin client with
service-account permissions to read users, teams, clients, and role mappings.
Then use `admin_client_id` with `admin_client_credential_ref` instead of
`admin_username` with `admin_password_credential_ref`.

To manage roles in the Keycloak UI:

1. Open the `nopsai` client and create client roles `viewer`, `developer`,
   `owner`, and `nopsai-admin`.
2. Assign `nopsai-admin`, `owner`, `developer`, or `viewer` directly to a user
   when the role should be global in NopsAI.
3. Create or open a Keycloak team such as `/team-1` or `/team-1/dev`.
4. Assign `viewer`, `developer`, or `owner` from the `nopsai` client to that
   team when the role should be scoped to the matching NopsAI team.

Membership comes from Keycloak. NopsAI writes provider-managed `basic_roles`
for linked users during OIDC login, on entitlement worker startup, and every
five minutes after that. It prunes those grants when the Keycloak team role
mapping no longer applies. The scoped grant can be stored before the matching
NopsAI team exists, so Keycloak and GitOps changes do not have to be applied
in a strict order.
Each provider-managed membership or role grant stores `source: idp`, the
provider ID, and the external group or role ID. The worker prunes only those
IdP-owned rows and leaves local System Access or GitOps grants intact.

SSO-managed users and their provider-managed grants are runtime identity state,
not GitOps state. Config repository export and drift skip linked OIDC users,
their `oidc:*` subjects, and Keycloak-managed role grants; keep those users and
team mappings in Keycloak, while GitOps owns the provider settings in
`setting/system/auth.yaml` plus local users, local grants, and service accounts.

When NopsAI runs directly on the host instead of in Compose, use
`http://localhost:8088/...` for the token, JWKS, and userinfo endpoints too.

## Test Flow

1. Start Keycloak.
2. Start NopsAI.
3. Sign in locally as an admin and open System > Access > Identity Providers.
4. Enable OIDC, enable auto-create users, and add the `nopsai` provider above,
   or sync the global config repo containing `setting/system/auth.yaml`.
5. Sign out.
6. On the login page, either click `Continue with Local Keycloak` or enter one
   of the seeded `example.com` emails and continue through discovery.
7. Sign in to Keycloak with one of the seeded users.
8. Confirm the NopsAI session is created and the mapped role is visible in
   System Access. The user card should show a friendly email label with a
   `Authenticated by Local Keycloak` marker instead of the raw `oidc:nopsai:<subject>`
   value. For `sso-admin@example.com`, `sso-operator@example.com`, and
   `sso-viewer@example.com`, System Access should show the global access role.
   For `alice@example.com` and `jip@example.com`, System Access should show a
   provider-managed Basic role on the matching team target.

If Keycloak was already started before the realm file changed, recreate it:

```bash
docker compose rm -sf keycloak
docker compose up -d keycloak
```

## Troubleshooting

`invalid parameter: redirect_uri` means the Keycloak client does not allow the
exact callback URL in the browser request. The callback includes the NopsAI
provider ID, for example `.../oidc/nopsai/callback`, so add that exact URL to
the Keycloak client redirect URIs.

`invalid_scope` with `Invalid scopes: openid email profile teams` means the
NopsAI provider requested `teams` as an OAuth scope, but this Keycloak client
does not define a client scope named `teams`. Use only `openid`, `email`, and
`profile` as requested scopes for this fixture. Keep `team_claim: teams` and
the Keycloak protocol mapper so role mapping still receives team names.

`oidc id token validation failed` after a successful Keycloak login usually
means the stored provider issuer does not exactly match the Keycloak realm
issuer. For this fixture the issuer must include the realm:

```text
http://192.168.1.143:8088/realms/nopsai
```

`invalid oidc state` after refreshing or retrying a callback URL is expected
after an earlier failed callback consumed the one-time state. Start a fresh
login from the NopsAI login page after fixing the provider configuration.

If signing out and signing back in still returns the previous Keycloak user,
make sure the Keycloak client allows the NopsAI origin URL as a post-logout
redirect URI. The local fixture sets `post.logout.redirect.uris` to `+`, which
means Keycloak uses the normal redirect URI list for post-logout redirects;
recreate Keycloak after changing the realm import file.
