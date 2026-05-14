# JWT Authentication

Nopsai uses JWTs in two trust boundaries:

- API and internal REST authentication in `services/nopsai`
- dispatcher gRPC service authentication between `nopsai`, `runner`, `agent`, and `dispatcher`

These are intentionally separate. The first protects the REST control plane. The second protects internal dispatcher gRPC calls and streams.

Related source files:

- `config/config.go`
- `services/nopsai/pkg/auth/*.go`
- `services/nopsai/main.go`
- `services/dispatcher/main.go`
- `pkg/serviceauth/serviceauth.go`
- `pkg/servicetls/servicetls.go`

---

## Token Families

| Token family | Used by | Transport | Signing key | Main purpose |
| --- | --- | --- | --- | --- |
| User/API access JWT | browser UI, API clients | HTTP `Authorization: Bearer ...` | `JWT_SIGNING_KEY` | Authenticate REST API callers |
| Dispatcher internal REST JWT | dispatcher | HTTP `Authorization: Bearer ...` | `JWT_SIGNING_KEY` | Let dispatcher call selected internal Nopsai REST endpoints |
| Dispatcher gRPC service JWT | nopsai, runner, agent | gRPC metadata `authorization: Bearer ...` | `SERVICE_JWT_SIGNING_KEY`, falling back to `JWT_SIGNING_KEY` | Authenticate and authorize dispatcher gRPC clients |

Refresh tokens are not JWTs. They are opaque random strings stored only as hashes in Postgres.

---

## Config

Main API JWT settings:

| YAML | Env | Meaning |
| --- | --- | --- |
| `jwt_signing_key` | `JWT_SIGNING_KEY` | HS256 HMAC key for API access tokens and dispatcher internal REST JWTs |
| `jwt_issuer` | `JWT_ISSUER` | Issuer written into API JWTs |
| `jwt_audience` | `JWT_AUDIENCE` | Audience written into API JWTs when configured |
| `jwt_expiry_minutes` | `JWT_EXPIRY_MINUTES` | Access-token TTL, defaulted to `60` in `services/nopsai/main.go` |
| `refresh_token_ttl_minutes` | `REFRESH_TOKEN_TTL_MINUTES` | Refresh-token lifetime; if `0`, login does not issue refresh tokens |
| `idle_timeout_minutes` | `IDLE_TIMEOUT_MINUTES` | Optional in-memory idle timeout for presented access tokens |
| `auth_provider_local_enabled` | `AUTH_PROVIDER_LOCAL_ENABLED` | Enables local username/password login |
| `rate_limit_login_per_minute` | `RATE_LIMIT_LOGIN_PER_MINUTE` | Per-identifier login rate limit |
| `login_lockout_threshold` | `LOGIN_LOCKOUT_THRESHOLD` | Failed password attempts before lockout |
| `login_lockout_window_minutes` | `LOGIN_LOCKOUT_WINDOW_MINUTES` | Lockout window |

Dispatcher gRPC service JWT settings:

| YAML | Env | Meaning |
| --- | --- | --- |
| `service_jwt_signing_key` | `SERVICE_JWT_SIGNING_KEY` | HS256 HMAC key for dispatcher gRPC service tokens |
| `service_jwt_issuer` | `SERVICE_JWT_ISSUER` | Issuer required by dispatcher gRPC auth; defaults to `nopsai.internal` |
| `service_jwt_audience` | `SERVICE_JWT_AUDIENCE` | Audience required by dispatcher gRPC auth; defaults to `nopsai-dispatcher` |
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

Request authentication:

1. Most REST endpoints require `Authorization: Bearer <access-token>`.
2. Public paths are `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout`, and `/v1/git/events`.
3. `authMiddleware` parses the bearer token, validates the HS256 signature and standard registered claims, then stores claims in request context.
4. If idle timeout is enabled, Nopsai tracks last-seen access tokens in process memory and rejects idle tokens.
5. `authzMiddleware` maps claims to an AAA subject and performs route authorization.

Current API JWT validation verifies the HMAC signature and registered claims. The issuer and audience are written into minted tokens, but the local API parser does not currently require exact issuer/audience values as separate parser options. Dispatcher gRPC service JWT validation does require issuer and audience.

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

## Dispatcher Internal REST JWT

The dispatcher sometimes calls Nopsai REST endpoints after receiving gRPC updates from agents or runners. Those calls use the existing API JWT signer, not the dispatcher gRPC service signer.

The dispatcher mints short-lived internal REST JWTs with:

```json
{
  "sub": "dispatcher",
  "provider": "internal-service",
  "iss": "<JWT_ISSUER>",
  "aud": ["<JWT_AUDIENCE>"],
  "iat": 1710000000,
  "exp": 1710000300
}
```

These tokens are signed with `JWT_SIGNING_KEY` and sent to Nopsai REST endpoints as:

```http
Authorization: Bearer <dispatcher-internal-jwt>
```

The current dispatcher internal REST paths include:

- `POST /v1/runs/{runID}/logs/ingest`
- `POST /v1/runs/{runID}/finalize`
- `GET /v1/runs/{runID}/status`
- `POST /v1/runs/{runID}/steps/{stepName}/tasks/{taskName}`
- `GET /v1/pipelines/{pipelineName}`
- `POST /v1/run`

Nopsai recognizes these claims as `internal_service:dispatcher` for AAA subject construction.

---

## Dispatcher gRPC Service JWT

Dispatcher gRPC auth lives in `pkg/serviceauth` and `services/dispatcher/main.go`.

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
- Keep `SERVICE_JWT_ISSUER` and `SERVICE_JWT_AUDIENCE` identical for all dispatcher gRPC clients and the dispatcher.
- Keep `DISPATCHER_TLS_SECRET` identical for all dispatcher gRPC clients and the dispatcher if you set it explicitly.
- Rotate keys with a coordinated deployment. The current implementation does not support multiple active signing keys or `kid`-based key selection.

If `JWT_SIGNING_KEY` is missing:

- local login cannot mint access tokens
- dispatcher internal REST tokens cannot be minted correctly

If both `SERVICE_JWT_SIGNING_KEY` and `JWT_SIGNING_KEY` are missing:

- dispatcher gRPC authentication cannot start or clients cannot create credentials

---

## Security Notes

- JWTs are bearer credentials. Anyone with the token can use it until it expires or, for refresh tokens, until it is revoked.
- Access JWTs are not stored server-side and are not individually revocable in the current implementation.
- Refresh tokens are opaque, hashed in Postgres, rotated on refresh, and revocable on logout.
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

- The REST token is malformed, expired, signed with the wrong `JWT_SIGNING_KEY`, or the service lacks local JWT auth configuration.

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
