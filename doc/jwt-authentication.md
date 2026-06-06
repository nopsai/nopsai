# JWT Authentication

Nopsai uses JWTs in two trust boundaries:

- user/API REST authentication in `services/nopsai`
- internal REST service authentication into `services/nopsai`
- dispatcher gRPC service authentication between `nopsai`, `runner`, `agent`, and `dispatcher`

These are intentionally separate. User/API JWTs protect the REST control plane. Service JWTs protect internal REST callbacks and dispatcher gRPC calls.

Related source files:

- `config/config.go`
- `services/nopsai/cmd/nopsai/main.go`
- `services/nopsai/internal/app`
- `services/nopsai/pkg/auth/*.go`
- `services/nopsai/app.go`
- `services/nopsai/bootstrap.go`
- `services/nopsai/aaa_helpers.go`
- `services/dispatcher/internal/app`
- `services/dispatcher/internal/service`
- `pkg/serviceauth/serviceauth.go`
- `pkg/servicetls/servicetls.go`

---

## Token Families

| Token family | Used by | Transport | Signing key | Main purpose |
| --- | --- | --- | --- | --- |
| User/API access JWT | browser UI, API clients | HTTP `Authorization: Bearer ...` | `JWT_SIGNING_KEY` | Authenticate REST API callers |
| Personal access token | API clients and automation | HTTP `Authorization: Bearer nopat_...` | Not signed; stored by hash in Postgres | Long-lived user-owned API credential |
| Service account token | integrations and automation | HTTP `Authorization: Bearer nopsat_...` | Not signed; stored by hash in Postgres | Long-lived token-only integration credential |
| Internal REST service JWT | dispatcher, agent | HTTP `Authorization: Bearer ...` | `SERVICE_JWT_SIGNING_KEY`, falling back to `JWT_SIGNING_KEY` | Let trusted services call selected Nopsai REST endpoints |
| Dispatcher gRPC service JWT | nopsai, runner, agent | gRPC metadata `authorization: Bearer ...` | `SERVICE_JWT_SIGNING_KEY`, falling back to `JWT_SIGNING_KEY` | Authenticate and authorize dispatcher gRPC clients |

Refresh tokens, personal access tokens, and service account tokens are not JWTs. They are opaque random strings stored only as hashes in Postgres.

---

## Config

Main API JWT settings:

| YAML | Env | Meaning |
| --- | --- | --- |
| `jwt_signing_key` | `JWT_SIGNING_KEY` | HS256 HMAC key for API access tokens |
| `jwt_issuer` | `JWT_ISSUER` | Issuer required for API JWTs |
| `jwt_audience` | `JWT_AUDIENCE` | Audience required for API JWTs when configured |
| `jwt_expiry_minutes` | `JWT_EXPIRY_MINUTES` | Access-token TTL, defaulted to `60` in `services/nopsai/internal/app` |
| `refresh_token_ttl_minutes` | `REFRESH_TOKEN_TTL_MINUTES` | Refresh-token lifetime; if `0`, login does not issue refresh tokens |
| `idle_timeout_minutes` | `IDLE_TIMEOUT_MINUTES` | Optional in-memory idle timeout for presented access tokens |
| `auth_provider_local_enabled` | `AUTH_PROVIDER_LOCAL_ENABLED` | Enables local username/password login |
| `rate_limit_login_per_minute` | `RATE_LIMIT_LOGIN_PER_MINUTE` | Per-identifier login rate limit |
| `login_lockout_threshold` | `LOGIN_LOCKOUT_THRESHOLD` | Failed password attempts before lockout |
| `login_lockout_window_minutes` | `LOGIN_LOCKOUT_WINDOW_MINUTES` | Lockout window |

Service JWT settings for internal REST and dispatcher gRPC:

| YAML | Env | Meaning |
| --- | --- | --- |
| `service_jwt_signing_key` | `SERVICE_JWT_SIGNING_KEY` | HS256 HMAC key for service tokens |
| `service_jwt_issuer` | `SERVICE_JWT_ISSUER` | Issuer required by service-token auth; defaults to `nopsai.internal` |
| `service_jwt_audience` | `SERVICE_JWT_AUDIENCE` | Audience required by service-token auth; defaults to `nopsai-dispatcher` |
| `nopsai_service_id` | `NOPSAI_SERVICE_ID` | Expected `sub` for Nopsai dispatcher gRPC calls; defaults to `nopsai` |
| `runner_service_id` | `RUNNER_SERVICE_ID` | Expected `sub` for runner dispatcher gRPC calls; defaults to `runner` |
| `agent_service_id` | `AGENT_SERVICE_ID` | Expected `sub` for agent dispatcher gRPC calls; defaults to `agent` |

If `SERVICE_JWT_SIGNING_KEY` is blank, the code reuses `JWT_SIGNING_KEY`. This keeps local development simple. In production, prefer a separate strong `SERVICE_JWT_SIGNING_KEY`.

Dispatcher gRPC transport security settings:

| YAML | Env | Meaning |
| --- | --- | --- |
| `dispatcher_tls_mode` | `DISPATCHER_TLS_MODE` | `mtls` by default; also supports `tls` or `disabled` |
| `dispatcher_tls_secret` | `DISPATCHER_TLS_SECRET` | Optional private CA seed; falls back to the effective service JWT signing key |
| `dispatcher_tls_server_name` | `DISPATCHER_TLS_SERVER_NAME` | Logical dispatcher TLS identity; defaults to `dispatcher` |

With the default `mtls` mode, the dispatcher and all dispatcher gRPC clients generate certificates in memory at startup. No certificate files need to be created or mounted.

`jwt_rsa_key_path` / `JWT_RSA_KEY_PATH` exists in config but the current local JWT implementation uses HS256 HMAC signing, not RSA signing.

---

## User/API JWT Flow

The API auth service lives in `services/nopsai/pkg/auth`.

Login:

1. Client calls `POST /v1/auth/login` with `identifier` and `password`.
2. `LoginLocal` checks that local auth is enabled and that JWT signing is configured.
3. The user is loaded by `sub` or email.
4. The user must be `active` and use provider `local`.
5. Password is checked with the stored bcrypt hash.
6. Login rate limit and lockout state are updated.
7. Roles are fetched from `user_roles`.
8. An HS256 access JWT is minted.
9. If refresh TTL is configured, an opaque refresh token is generated and its hash is stored in `refresh_tokens`.

Access-token claims include:

```json
{
  "sub": "user-subject",
  "email": "user@example.com",
  "provider": "local",
  "roles": ["role-name"],
  "iss": "nopsai.local",
  "aud": ["nopsai-api"],
  "iat": 1710000000,
  "exp": 1710003600
}
```

The custom `sub` field and the registered JWT `sub` are both populated with the same user subject.

Refresh:

1. Client calls `POST /v1/auth/refresh` with `refresh_token`.
2. Nopsai hashes the presented refresh token and looks it up in Postgres.
3. The token must exist, not be revoked, and not be expired.
4. The user must still be active.
5. Nopsai mints a new access JWT.
6. Nopsai creates a new refresh token and revokes the old refresh token.

Logout:

1. Client calls `POST /v1/auth/logout` with `refresh_token`.
2. Nopsai hashes the refresh token and sets `revoked_at`.
3. Already-minted access JWTs are not individually revoked; they expire by TTL or idle timeout.

Personal access tokens:

1. A signed-in user creates a token with `POST /v1/auth/personal-tokens`.
2. The request must use an interactive session JWT; existing personal tokens cannot mint or revoke personal tokens.
3. Nopsai generates a `nopat_` opaque token with cryptographic randomness.
4. Only the token hash, suffix, owner, name, timestamps, and expiry are stored in `personal_access_tokens`.
5. The raw token is returned once in the create response and is never returned by list APIs.
6. The token can use `expires_in_days`, an exact `expires_at` date/timestamp within 365 days, or explicit `never_expires: true`.
7. Users can list their token metadata with `GET /v1/auth/personal-tokens` and revoke tokens with `DELETE /v1/auth/personal-tokens/{tokenID}`.

Service account tokens:

1. An IAM administrator creates a service account with `POST /v1/admin/service-accounts`,
   or a system/global config repository declares it with the `service_accounts`
   key in `access/*.yaml`.
2. Service accounts are stored as user-like identities with `provider = service-account`, no password hash, and no local login path.
3. GitOps may declare the service-account identity and grants, but it never stores the raw bearer token.
4. Nopsai generates a `nopsat_` opaque token for the service account through the System Access page or token API. Only the hash, suffix, owner, name, timestamps, and expiry are stored in `service_account_tokens`.
5. The raw token is returned once when a token is created and is never returned by list APIs.
6. Service account tokens authenticate as AAA `service_account` subjects, so integrations can receive roles and resource grants without borrowing a human user's personal token.
7. IAM administrators can list, rotate, and revoke service account tokens from the System Access page or the `/v1/admin/service-accounts/{serviceAccountID}/tokens` endpoints.

Request authentication:

1. Most REST endpoints require `Authorization: Bearer <access-token>`.
2. Public paths are `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout`, and `/v1/git/events`.
3. `authMiddleware` first attempts service-token validation. If that succeeds, the request is normalized as `provider = internal-service` with the service `role`.
4. If service-token validation fails, Nopsai validates the bearer token as a user/API HS256 JWT with signature, expiration, issuer, and audience checks.
5. If JWT validation fails and the token starts with `nopat_`, Nopsai hashes the value and looks for a non-revoked, non-expired personal access token.
6. If the token starts with `nopsat_`, Nopsai hashes the value and looks for a non-revoked, non-expired service account token.
7. Personal tokens authenticate as the owning active user with `provider = personal-token`; service account tokens authenticate with `provider = service-account-token`.
8. If idle timeout is enabled, Nopsai tracks last-seen session access tokens in process memory and rejects idle session tokens. Personal and service account tokens rely on expiry and revocation instead.
9. `authzMiddleware` maps claims to an AAA subject and performs route authorization.

---

## UI Behavior

The UI stores session data in browser `localStorage`:

- `nopsai.auth.token`
- `nopsai.auth.refresh`
- `nopsai.auth.roles`
- `nopsai.auth.sub`

The fetch interceptor in `services/ui/src/lib/api.ts` attaches:

```http
Authorization: Bearer <access-token>
```

When an authenticated request gets `401`, the UI calls `/v1/auth/refresh` once, persists the new tokens, and retries the original request. If refresh fails, it clears the local session.

The Profile page manages personal access tokens. It lists token metadata, creates tokens with 30-day, 90-day, or 1-year expiry, displays the raw token only immediately after creation, and can revoke existing tokens.

The System Access page manages service accounts. IAM administrators can create token-only integration identities, assign access roles and basic scoped roles, rotate tokens, and revoke tokens without involving a human user's profile.

---

## Authorization After JWT Authentication

JWT authentication only proves who the caller is. Authorization is separate.

Normal API callers become AAA `user` subjects based on token claims:

```text
subject.type = user
subject.sub = claims.sub
subject.email = claims.email
```

Dispatcher internal REST callers become an AAA internal service subject:

```text
subject.type = internal_service
subject.id = dispatcher
subject.sub = dispatcher
```

The AAA layer then checks actions and resources such as `pipeline.execute`, `pipeline_run.read`, or `system.update`.

---

## Internal REST Service JWT

The dispatcher sometimes calls Nopsai REST endpoints after receiving gRPC updates from agents or runners. Agents also call selected internal Nopsai endpoints directly for approval checkpoints. These calls use service-auth JWTs, not user/API JWTs.

The dispatcher mints short-lived service JWTs with:

```json
{
  "sub": "dispatcher",
  "provider": "internal-service",
  "role": "dispatcher",
  "iss": "<SERVICE_JWT_ISSUER>",
  "aud": ["<SERVICE_JWT_AUDIENCE>"],
  "iat": 1710000000,
  "exp": 1710000300
}
```

Agent approval checkpoint calls use the same token family with `sub = agent` and `role = agent`. These tokens are signed with the effective service JWT signing key and sent to Nopsai REST endpoints as:

```http
Authorization: Bearer <service-jwt>
```

The current dispatcher internal REST paths include:

- `POST /v1/runs/{runID}/logs/ingest`
- `POST /v1/runs/{runID}/finalize`
- `GET /v1/runs/{runID}/status`
- `POST /v1/runs/{runID}/steps/{stepName}/tasks/{taskName}`
- `GET /v1/pipelines/{pipelineName}`
- `POST /v1/run`

Nopsai recognizes these claims as `internal_service:dispatcher` for AAA subject construction.

The current agent-only internal REST paths include:

- `POST /v1/internal/runs/{runID}/approvals/pause`
- `GET /v1/internal/runs/{runID}/checkpoints/{checkpointID}`

Those endpoints require the service role `agent`.

---

## Dispatcher gRPC Service JWT

Dispatcher gRPC auth lives in `pkg/serviceauth` and `services/dispatcher/internal/service`, with process wiring in `services/dispatcher/internal/app`. It uses the same service-token format as internal REST service calls.

Each client attaches gRPC metadata:

```text
authorization: Bearer <service-jwt>
```

Service JWTs are HS256 tokens with:

```json
{
  "sub": "agent",
  "provider": "internal-service",
  "role": "agent",
  "iss": "nopsai.internal",
  "aud": ["nopsai-dispatcher"],
  "iat": 1710000000,
  "exp": 1710000300
}
```

The default service token TTL is 5 minutes. Clients mint a fresh token through gRPC per-RPC credentials as calls are made.

Dispatcher validation requires:

- `Authorization` metadata with a bearer token
- HS256 signature with the configured service signing key
- non-expired token
- expected issuer
- expected audience
- `provider = internal-service`
- non-empty service `role`
- non-empty service identity in `sub`
- service identity matching the configured service ID for that role
- role allowed for the requested RPC method

Allowed dispatcher gRPC roles:

| RPC | Allowed role |
| --- | --- |
| `SubmitJob` | `nopsai` |
| `GetStatus` | `nopsai` |
| `UpdateRunnerDispatch` | `nopsai` |
| `Register` | `runner` |
| `IngestLogs` | `runner`, `agent` |
| `GetRunStatus` | `runner`, `agent` |
| `ReportTaskStatus` | `agent` |
| `FinalizeRun` | `agent` |
| `FetchPipeline` | `agent` |
| `TriggerPipeline` | `agent` |

Client wiring:

- `services/nopsai` dials dispatcher with role `nopsai`.
- `services/runner` dials dispatcher with role `runner`.
- `services/nopsai` injects service JWT config into launched agent containers.
- `services/agent` dials dispatcher with role `agent`.

The gRPC transport uses automatic TLS/mTLS from `pkg/servicetls`. By default both sides derive an in-memory private CA from `DISPATCHER_TLS_SECRET`, falling back to the service JWT signing key. The dispatcher presents a certificate for the logical server name `dispatcher`, and in `mtls` mode clients also present automatically generated client certificates. JWT auth is still required after the TLS handshake.

---

## Key Generation

Use stable, high-entropy signing keys. Good local/prod examples:

```bash
openssl rand -base64 32
```

Do not rely on automatic random generation at service startup. These signing keys must be shared by multiple services and remain stable across restarts. If each service generates a different key, authentication breaks. If a service generates a new key on restart, existing tokens stop validating.

Recommended setup:

- Development: set `JWT_SIGNING_KEY`; leave `SERVICE_JWT_SIGNING_KEY` blank if you want service JWTs to reuse it.
- Production: set both `JWT_SIGNING_KEY` and a separate `SERVICE_JWT_SIGNING_KEY`.
- Store keys in your deployment secret manager or `.env` outside source control.
- Keep `SERVICE_JWT_ISSUER` and `SERVICE_JWT_AUDIENCE` identical for Nopsai, dispatcher, runners, and agents.
- Keep `DISPATCHER_TLS_SECRET` identical for all dispatcher gRPC clients and the dispatcher if you set it explicitly.
- Rotate keys with a coordinated deployment. The current implementation does not support multiple active signing keys or `kid`-based key selection.

If `JWT_SIGNING_KEY` is missing:

- local login cannot mint access tokens

If both `SERVICE_JWT_SIGNING_KEY` and `JWT_SIGNING_KEY` are missing:

- internal REST service authentication and dispatcher gRPC authentication cannot start or clients cannot create credentials

---

## Security Notes

- JWTs are bearer credentials. Anyone with the token can use it until it expires or, for refresh tokens, until it is revoked.
- Personal access tokens are bearer credentials too. Store them like passwords, prefer short expirations, and revoke unused or suspected-compromised tokens from Profile.
- Access JWTs are not stored server-side and are not individually revocable in the current implementation.
- Refresh tokens are opaque, hashed in Postgres, rotated on refresh, and revocable on logout.
- Personal access tokens are opaque, hashed in Postgres, owner-scoped, revocable, and never displayed after creation. Never-expiring tokens are supported for stable integrations, but they should be rare and manually revoked when no longer needed.
- The UI stores tokens in `localStorage`, which is convenient but means XSS would be serious. Avoid injecting untrusted script into the UI.
- Dispatcher gRPC uses automatic mTLS by default. Only set `DISPATCHER_TLS_MODE=disabled` for isolated local debugging.
- Use different keys for user/API JWTs and service gRPC JWTs in production to reduce blast radius.
- Never commit real signing keys or refresh tokens.
- `AAA_SHARED_INTERNAL_TOKEN` and `X-Internal-Token` are not JWT. They protect Nopsai-to-AAA HTTP calls separately from the JWT systems above.

---

## Troubleshooting

`missing bearer token`

- The REST request did not include `Authorization: Bearer <token>`.

`invalid token`

- The REST token is malformed, expired, signed with the wrong key, has the wrong issuer/audience, or the service lacks local/service JWT auth configuration.

`session expired due to inactivity`

- `IDLE_TIMEOUT_MINUTES` is enabled and this access token was idle too long in this Nopsai process.

`failed to configure dispatcher client authentication`

- A dispatcher gRPC client could not build service credentials. Check `SERVICE_JWT_SIGNING_KEY` or fallback `JWT_SIGNING_KEY`.

`invalid service token`

- Dispatcher rejected the gRPC bearer token. Check signing key, issuer, audience, expiry, and metadata.

`service role is not allowed to call dispatcher method`

- The token is valid, but the `role` claim is not allowed for that RPC.

`service identity is not allowed to call dispatcher method`

- The token role is allowed, but `sub` does not match the configured service ID for that role.
