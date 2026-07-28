# SSO Examples

This directory owns runnable identity-provider examples for local and
integration SSO testing. Keep these fixtures separate from production
deployment files and product runtime configuration.

| Path | Purpose |
| --- | --- |
| `keycloak/` | Local Keycloak realm with a confidential `nopsai` client, seeded users, client roles, team groups, and Keycloak entitlement-sync coverage. |
| `idp-test-pack/` | Mock OAuth2 scenarios for Entra ID, Okta, Google, and Keycloak claim mapping, plus a production-shaped GitHub OAuth2 config example. |

## Quick Commands

Start the real Keycloak fixture:

```bash
docker compose -f examples/sso/keycloak/docker-compose.yaml up -d keycloak
```

The Keycloak compose file uses the `nopsai` compose project name so it joins the
same `nopsai-net` network as the local platform stack.

Start the mock multi-provider IdP fixture:

```bash
docker compose -f examples/sso/idp-test-pack/docker-compose.yml up -d mock-oauth2
```

Validate the mock fixture mappings:

```bash
python3 examples/sso/idp-test-pack/scripts/validate-fixtures.py
```

## Enterprise Boundary

SSO settings are GitOps-compatible through the global config repository at
`setting/system/auth.yaml`. The example scenario files under
`idp-test-pack/nopsai-tests/**/setting/system/auth.yaml` can be copied into
that location for a test run.

Credential values should not be committed in feature config. Use stable
credential references such as `credential://system/oidc/nopsai/client-secret`
and store values through **Credentials** or encrypted
`setting/system/credentials.yaml` envelopes.

IdP-created users, external identities, team memberships, and provider-managed
role grants are runtime identity state. NopsAI records them as `source: idp` and
prunes only rows owned by the active provider; local UI/API/GitOps grants remain
editable. Only one external provider can be enabled per installation.

Monitor SSO tests through auth audit logs, System Access identity-provider
state, and `/metrics` identity-provider configuration/capability plus
authorization grant ownership series. Hosted MCP and the CLI use the same
authenticated NopsAI API and AAA decisions after SSO login, so these fixtures do
not require a separate MCP or CLI version change.

See [local-keycloak-sso.md](../../doc/local-keycloak-sso.md) and
[jwt-authentication.md](../../doc/jwt-authentication.md) for the detailed
operator flow.
