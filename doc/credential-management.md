# Credential Management Architecture

Status: Implemented.

## Decision

Use one credential model:

- Store all operational integration credentials in one encrypted, versioned
  database-backed credential registry.
- Expose one `CredentialResolver` interface to every credential consumer.
- Store credential references, never plaintext values, in feature-specific
  GitOps configuration.
- Store encrypted credential envelopes in `setting/system/credentials.yaml`
  when GitOps should be the source of truth for credential versions.
- Resolve credentials only for the service and purpose that need them.
- Do not support environment-variable credential references, automatic
  precedence rules, or per-integration storage choices.

This replaces long-lived integration values currently supplied as
container environment variables, including LLM API keys, MCP bearer tokens,
SMTP passwords, OIDC client/admin secrets, GitHub App credentials, and Git
webhook source signing secrets.

The database password and root encryption key are bootstrap inputs, not
credentials managed by the product. They cannot be stored in the database they
are required to unlock.

The operator-facing contract is always:

```text
credential_ref -> CredentialResolver -> encrypted credential registry
```

Consumers do not know how credentials are encrypted or stored, and integrations
do not implement their own credential lookup rules.

## Why A Separate Credential Registry

The existing `secrets` table is designed for pipeline runtime secrets:

- values are selected by repository and runtime scope
- pipeline callers are authorized through `secret.use`
- owners may receive `secret.read_value`
- GitOps can contain instance-encrypted values

System integration credentials have different ownership and lifecycle needs:

- they are bound to a service or integration configuration
- most should be write-only for humans after creation
- they need versioning, rotation, expiry, and consumer audit records
- GitHub credentials must be delivered across a service boundary

Reusing pipeline secrets with reserved names or a synthetic scope would mix
these authorization and lifecycle models. A dedicated registry keeps both
models explicit.

## Implemented Controls

- AES-256-GCM envelope encryption with per-version data keys, authenticated
  credential identity, key IDs, and format versions
- immutable credential versions with activation, rollback, disable, expiry,
  management audit, and consumer access logs
- one resolver contract for LLM, MCP, SMTP, OIDC, and GitHub credentials
- purpose-bound resolution for Git webhook source HMAC and static-token secrets
- authenticated and encrypted `nopsai` to `git-bot` credential bootstrap
- controlled one-time import and scrubbing of legacy database/config values
- GitOps references that create pending metadata when no encrypted value is
  present
- GitOps import/export of encrypted credential envelopes in
  `setting/system/credentials.yaml`

## Bootstrap Boundary

Keep this small set outside the credential interface:

- `DATABASE_URL` or equivalent database authentication
- the root key-encryption key needed to decrypt credential data-encryption keys
- service identity and transport trust needed before internal services can
  authenticate, such as mTLS roots or signing-key bootstrap material
- disaster-recovery material required to restore encrypted database backups

These values are deployment bootstrap configuration. They should come from
Kubernetes Secrets, Docker secrets, or the deployment platform's equivalent,
and must not be writable through GitOps application configuration, the setup
UI, or the credential API.

Non-secret addresses, IDs, routing, and operational defaults remain normal
GitOps configuration.

## Data Model

Use separate metadata and immutable version records.

### `credentials`

- `id UUID`
- `name TEXT`
- `namespace TEXT`, initially `system`
- `kind TEXT`, such as `api_key`, `password`, `bearer_token`, `private_key`,
  `webhook_secret`, `client_secret`, or `docker_config_json`
- `status TEXT`, such as `pending`, `active`, or `disabled`
- `metadata JSONB`, for non-secret derived facts such as Docker registry
  hostnames
- `active_version INTEGER`
- `expires_at`, `last_rotated_at`, `created_at`, and `updated_at`
- actor and GitOps ownership metadata

### `credential_versions`

- `credential_id UUID`
- `version INTEGER`
- `ciphertext BYTEA`
- `wrapped_data_key BYTEA`
- `encryption_key_id TEXT`
- `encryption_format_version INTEGER`
- `created_at`, `activated_at`, and `revoked_at`
- actor metadata

The unique public reference should be stable, for example:

```text
credential://system/llm/openai-primary
credential://system/mcp/github-readonly
credential://system/mail/smtp-primary
credential://system/oidc/keycloak/client
credential://system/github/default/private-key
credential://system/webhooks/gitlab-platform
```

Integration tables should store the stable reference, not copy encrypted
values into each feature table.

Expected credential kinds by consumer:

- models and knowledge provider connections: `api_key`
- MCP profiles and config repositories: `bearer_token`
- SMTP mail: `password`
- OIDC client/admin credentials: `client_secret`
- GitHub App private key: `private_key`
- GitHub App webhook secret and Git webhook source signing secrets:
  `webhook_secret`

## Encryption

Introduce a credential encryption interface owned by a focused package, for
example `services/nopsai/internal/credentials`.

The default implementation should use envelope encryption:

1. Generate a random data-encryption key for each credential version.
2. Encrypt the value with AES-256-GCM.
3. Bind the credential ID, namespace, kind, version, and intended purpose as
   authenticated additional data.
4. Wrap the data-encryption key with a versioned key-encryption key.
5. Store the ciphertext, wrapped key, key ID, and format version.

For local deployments, the existing master key can initially implement the
key-encryption-key interface. The storage and consumer contract remains the
same if the encryption implementation is hardened later.

Writing a replacement value creates and atomically activates a new immutable
version. Administrators can reactivate an earlier retained version for rollback.
Disabling a credential preserves its active version and blocks resolution;
enabling it restores that version without requiring another rotation.

Administrators can permanently delete a non-active version only when at least
two versions exist before the deletion. The active version cannot be deleted,
and version numbers remain monotonic rather than being reused.

Deleting a credential is blocked while an integration still references it.

## GitOps Contract

GitOps has two layers:

- Feature files own integration configuration and stable credential references.
- `setting/system/credentials.yaml` owns encrypted credential envelopes when
  credential values should be promoted through GitOps.

GitOps never stores or exports plaintext secret values. The credential envelope
is instance-encrypted; it can be restored directly only with the matching
encryption root or after being re-encrypted for the target environment.

Example model:

```yaml
profiles:
  - name: standard
    provider: openai
    model: gpt-5
    credential_ref: credential://system/llm/openai-primary
```

Example mail settings:

```yaml
enabled: true
from: nopsai@example.com
smtp:
  host: smtp.example.com
  port: 587
  start_tls: true
  username: nopsai@example.com
  password_credential_ref: credential://system/mail/smtp-primary
```

Sync behavior:

- create missing credential metadata as `pending` when a feature file references
  a credential but `credentials.yaml` does not provide an active encrypted
  version
- import encrypted versions from `setting/system/credentials.yaml` without
  decrypting or re-encrypting during sync
- export encrypted versions to `setting/system/credentials.yaml` during config
  repository drift/export
- report unresolved or disabled references as drift/status errors
- never export plaintext through API responses, runtime snapshots, support
  bundles, or GitOps feature files
- when `setting/system/credentials.yaml` is present, prune credentials managed
  by the same config repository that are removed from that file

Instance-encrypted values in scope files can remain supported for pipeline
secrets. System credentials use `setting/system/credentials.yaml` instead so
the registry, UI, drift, and config sync share one source-of-truth model.

## AAA And Audit

Add a first-class `credential` resource with default-deny actions:

- `credential.list_metadata`
- `credential.create`
- `credential.write_value`
- `credential.rotate`
- `credential.disable`
- `credential.enable`
- `credential.delete_version`
- `credential.delete`
- `credential.use`
- `credential.manage_acl`

Do not add a normal human-facing `credential.read_value` action for system
credentials. Values are write-only after submission. UI and API responses
return metadata such as `has_value`, active version, expiry, and last
rotation time.

The Credentials page is a first-class left-navigation page rather than a
System sub-tab. Users can see credentials they created themselves and
credentials whose metadata, use, lifecycle, or ACL actions are granted to them.
Per-credential access management uses the same product access-grant model as
pipelines: grant resources use `credential` with IDs formed from the reference
path, for example `system/llm/openai-primary`.

Service identities receive narrowly scoped `credential.use` grants. Every
resolution records:

- credential ID and version
- consuming service and purpose
- initiating user, repository, schedule, trigger, or run when applicable
- success or failure
- request and correlation IDs

Audit records, logs, metrics, API errors, and health checks must never include
credential values.

## Runtime Delivery

The registry resolves credentials through one service interface and delivers
them only to the consumer that needs them. It replaces deployment-wide
environment injection with purpose-bound runtime resolution.

Preferred behavior by integration:

- LLM: `nopsai` resolves only the selected profile credential when preparing a
  run and includes it in the protected ephemeral agent payload.
- MCP: resolve only credentials for servers selected by the run's MCP profiles.
- Mail: resolve inside `nopsai` immediately before creating the SMTP client.
- OIDC: resolve inside `nopsai` for token exchange or entitlement sync.
- GitHub: `git-bot` authenticates as an internal service and obtains the active
  GitHub App private key and webhook secret through a protected broker flow.
- Git webhook sources: `nopsai` resolves only the source's referenced
  `webhook_secret` while verifying a delivery. Source GitOps files contain the
  reference, repository allowlist, and policy, never the value.
- Runner private registries: `docker_config_json` credentials are assigned to
  specific runner IDs. NopsAI stores non-secret `registry_hosts` metadata,
  creates an env-carried Docker config for Docker runners or a Kubernetes
  imagePullSecret for Kubernetes runners, and lets Docker runners and agents use
  that local config for per-image `RegistryAuth`.

`git-bot` retrieves the required values during startup through its authenticated
broker request and keeps them only in memory. Restart `git-bot` after rotating
either GitHub credential so it fetches the newly active version.

For agents and runners, avoid making all system credentials part of their base
container environment. Deliver only values selected for that run, and preserve
the existing masking behavior.

## Feature Ownership

Keep the implementation separated:

- Model and validation:
  `services/nopsai/internal/credentials`
- Persistence:
  `services/nopsai/pkg/store/credentials.go`
- Workflow and resolution:
  `services/nopsai/credential_service.go`
- API DTOs and handlers:
  `services/nopsai/credential_models.go` and
  `services/nopsai/credential_handlers.go`
- Route composition:
  `services/nopsai/routes.go`
- AAA action/resource mapping:
  `services/nopsai/pkg/routeauthz` and `services/aaa/pkg/model`
- GitOps parsing and reconciliation:
  feature-local credential reference fields plus
  `services/nopsai/credentials_gitops.go`
- UI API/model/hook/rendering:
  `services/ui/src/pages/Credentials.tsx` for route composition and separate
  files under `services/ui/src/features/system/credentials`

Provider clients consume a narrow `CredentialResolver` interface. They should
not query credential tables or decrypt ciphertext directly.

The interface should be intentionally small:

```go
type CredentialResolver interface {
	Resolve(ctx context.Context, ref Reference, purpose Purpose) (Value, error)
}
```

Administrative create, rotate, disable, and delete operations belong to a
separate credential management service. They must not be exposed through the
runtime resolver.

## Legacy Migration

Startup imports legacy OIDC, SMTP, LLM, MCP, and GitHub values once, writes them
to the registry, replaces integration configuration with references, and
scrubs legacy database values. Runtime consumers never fall back to legacy
environment variables, database columns, or private-key files.

## Required Coverage

- codec tests for tampering, wrong keys, wrong authenticated data, and format
  versions
- store tests for version creation, activation, rollback, expiry, and pruning
- API tests proving values are never returned
- AAA tests for default deny, human administration, and service-only use
- audit tests proving no secret material is recorded
- GitOps tests for pending metadata, encrypted envelope import/export, value
  preservation, drift, and plaintext non-disclosure
- integration tests for OIDC, SMTP, LLM, MCP, and GitHub consumers
- migration tests for existing plaintext OIDC records and explicit failure when
  credential references are unresolved
- backup/restore tests that require the separately managed encryption root

## Operational Requirements

- Encrypted database backups and a separately protected encryption root
- documented key and credential rotation procedures
- metrics for unresolved, expired, failed, and recently rotated credentials
- startup and readiness status without exposing secret material
- rate limits and step-up authentication for credential administration
- no plaintext secret values in support bundles, API config exports, runtime
  snapshots, or GitOps diffs

## References

- [NIST SP 800-57 Part 1 Rev. 5](https://csrc.nist.gov/pubs/sp/800/57/pt1/r5/final)
- [OWASP Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html)
- [Kubernetes Secrets good practices](https://kubernetes.io/docs/concepts/security/secrets-good-practices/)
