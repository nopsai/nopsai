# First-install Wizard

The first-install wizard guides an administrator from an empty database to a
working NopsAI workspace without reading the full configuration reference first.
It lives in the UI at **System > Setup** and is backed by `/v1/setup/*` API
endpoints. After the bootstrap admin clears any first-login password
requirement, the UI and API enforce setup completion before normal workspace
pages and APIs are available. Direct URL changes are redirected back to
**System > Setup** until setup is completed once. Generated installs create the
bootstrap admin from the operator-provided or generated first-install password
instead of seeding a production default.

Before login, the UI also checks `/v1/setup/preflight`. If required
configuration is missing, the login page shows installation guidance instead of
leaving the operator at a broken sign-in form.

Use it for a new installation, a local evaluation environment, or a controlled
bootstrap of a team workspace. The wizard uses one setup path: required steps
must be completed, while optional integrations can be skipped and configured
later. Licence acceptance is the first required step and cannot be skipped.

## Required Access

The authenticated setup page is a system administration surface:

- `GET /v1/setup/preflight` is public so the UI can explain missing initial
  configuration before authentication is available.
- `GET /v1/setup/license` is public for the same reason: an administrator has to
  be able to read the proprietary notice before being asked to accept it, and
  the notice is the same text already shipped in every artifact.
- `POST /v1/setup/license/accept` requires `system.update` on `system:config`,
  because recording acceptance is an administrator action even though reading
  the notice is not.
- `GET /v1/setup/*` requires `system.read` on `system:config`.
- `POST /v1/setup/*` requires `system.update` on `system:config`.
- Until `setup_state.completed_at` is written, authenticated API requests other
  than `/v1/auth/me`, `/v1/auth/password`, and `/v1/setup/*` fail closed with
  a first-install setup requirement.

The local bootstrap admin can open the wizard after login, but the health
checks will warn if the development `admin` password is still in use or if the
admin must change password.

## Wizard Shape

There are no starter profiles in the UI. The wizard is a single guided flow:

- required licence acceptance, before anything else
- required readiness and runtime configuration steps
- optional GitHub App connection and installation
- optional GitOps repository connection and sync kickoff
- optional repository teams, limited to one or two starter teams for an
  introduction
- optional default AI setup with one API key field
- optional user creation with team, role, and temporary password assignment
- final generated output for runtime variables and service/container
  environment

## Wizard Flow

1. Open the UI. The login page checks preflight readiness for the database,
   master encryption key, and JWT signing key. During Kubernetes or Compose
   cold starts, the API can serve preflight responses while it keeps retrying a
   database that is still starting; readiness becomes healthy after Postgres is
   reachable and normal startup resumes.
2. Resolve any required preflight items in `docker-compose.yaml`, exported
   environment, or `config.yml`, then restart affected services.
3. Sign in as an administrator. On an incomplete installation the UI locks
   normal navigation to **System > Setup**. If the bootstrap admin must change
   password, the Profile page is shown first and setup opens after the password
   requirement is cleared.
4. Review health checks for database connectivity, local secrets, admin
   bootstrap state, git-bot service configuration, access grants, LLM/MCP
   configuration, starter pipeline presence, and runner connectivity.
5. Review runtime values for service-to-service auth, git-bot forwarding, and
   service discovery. Docker Compose uses bridge-network DNS defaults;
   Kubernetes uses cluster DNS defaults. The final step prints variables that can
   be applied as container environment, secret-manager values, or an environment
   file.
6. Optionally connect GitHub with one button. NopsAI creates a GitHub App from
   a manifest, stores its credentials, and registers each account the App is
   installed on. The step needs an address GitHub can deliver webhooks to, and
   can be skipped and done later from **System > Git Apps**.
7. Optionally connect a global GitOps config repository and start sync.
8. Create one or two repository teams and place selected repositories under
   them. These teams drive starter trigger generation, run navigation, and
   initial access assignments.
9. Optionally configure the default model. For local development, the
   default is LM Studio at `http://lmstudio:1234` with model `qwen3-coder`.
   The catalog also supports Gemini, OpenAI / ChatGPT, Anthropic Claude, Groq,
   Mistral, OpenRouter, Ollama, and Azure OpenAI. Hosted providers use one API
   key field, stored as a NopsAI secret.
10. Optionally seed disabled MCP examples for later activation.
11. Optionally create starter users, assign them to a team with owner,
    developer, or viewer role, and set or generate temporary passwords. Created
    local users must change password on first login.
12. Review generated runtime variables and post-setup instructions.
13. Apply setup, then run the starter `setup/first-run` pipeline to verify the
    runner, agent, logs, and UI. When setup seeded an model, the same
    pipeline also verifies the LLM path with an AI smoke step.

The setup modal is step-by-step. Optional steps such as GitHub, GitOps,
repository teams, AI, MCP examples, and users can be skipped and completed
later. The review step summarises generated variables, repository teams,
selected repositories, AI settings, and user assignments before anything is
applied.

After setup is completed, **System > Setup** becomes a status page: health checks
and resource counts, and nothing else. The step navigation and the setup forms
belong to the one-time wizard and are not shown again, because re-walking a
finished bootstrap is not an operator task. Copy the generated runtime env blocks
from the output step before applying setup; afterwards the values themselves live
in the environment, secret manager, or env file they were applied to. Everything
else setup creates is a config resource that config sync carries to the global
config repository.

## Preflight Mode

If the API cannot safely start the authenticated control plane because a hard
prerequisite is missing, it starts in setup preflight mode and serves only
installation readiness endpoints. This keeps the UI useful even when normal
login cannot work yet.

Required preflight items:

- `DATABASE_URL`: NopsAI must reach Postgres before it can store users, runs, or
  setup state.
- `NOPSAI_MASTER_KEY`: required for encrypted secret storage.
- `JWT_SIGNING_KEY`: required for local login sessions.
- `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD` or
  `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE`: required in production gate mode when
  the first local administrator needs to be created or an insecure development
  password needs to be rotated.

The login page shows suggested environment entries for missing required values
and marks configured required checks with a green tick. The wizard can generate
later service secrets after login, but these hard pre-authentication values must
exist before an authenticated workspace can open.

Production startup gates:

- Set `NOPSAI_ENVIRONMENT=production` or
  `NOPSAI_REQUIRE_PRODUCTION_GATES=true` to make startup fail closed when
  production-hardening requirements are not met.
- Production gates require production-grade `NOPSAI_MASTER_KEY`,
  `JWT_SIGNING_KEY`, `SERVICE_JWT_SIGNING_KEY`, and
  `AAA_SHARED_INTERNAL_TOKEN` values.
- `SERVICE_JWT_SIGNING_KEY` must be separate from `JWT_SIGNING_KEY` so browser
  sessions and internal service tokens do not share one signing secret.
- Dispatcher TLS/mTLS must remain enabled.
- When a GitHub App is configured, private-key and webhook credential
  references must be configured.
- The bootstrap admin must not use the development `admin` password.
  Production gate mode creates or rotates the first local administrator only
  from `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD` or
  `NOPSAI_BOOTSTRAP_ADMIN_PASSWORD_FILE`.

## Generated Service Secrets

When **Generate missing service secrets** is enabled, the wizard fills genuinely
missing values for:

- `JWT_SIGNING_KEY`
- `SERVICE_JWT_SIGNING_KEY`
- `AAA_SHARED_INTERNAL_TOKEN`
- `DISPATCHER_TLS_SECRET`

Generated Docker Compose installs already provide these bootstrap-time values in
`.env`. If `DISPATCHER_TLS_SECRET` is not set but the effective service JWT key
already provides the dispatcher TLS trust seed, the wizard does not force an
env-file write. If values are missing and the API was not started with
`ENV_FILE_PATH`, setup returns a clear error instead of a generic apply failure.

The API response lists the names that were generated and sets `requires_restart`
when runtime values changed. Restart services after generating secrets so every
process reads the same values.

Do not commit these values to the config repository.

## GitHub App And git-bot

The wizard prepares the internal NopsAI-to-git-bot service URL. In Docker
Compose that URL is usually `http://git-bot:8081`. Manage GitHub App IDs and
credential references in **System > Git Apps** or
`setting/git-apps/github.yaml`, store private-key and webhook secret values in
**Credentials**, and keep the internal git-bot service URL in **System >
Config**.

Guided install flow (**GitHub** step, or **System > Git Apps**):

1. Start the `git-bot` service with `NOPSAI_API_URL` pointing at the NopsAI API
   URL reachable from git-bot, usually `http://nopsai:8080` in Docker Compose.
2. Select **Install GitHub App on GitHub**. The wizard step asks for nothing
   else when the deployment already knows the webhook address: App ID, private
   key, and webhook secret are issued by GitHub and stored automatically. Name a
   **GitHub organization** to create the App there instead of on your personal
   account, since a private App can only be installed on the account that owns
   it. Only a real organization belongs in that field: a personal username there
   sends the manifest to `github.com/organizations/<user>/settings/apps/new`,
   which GitHub answers with a 404.
3. If no webhook address is configured yet, the step asks for the one value
   NopsAI cannot infer: the public address of the tunnel or reverse proxy in
   front of git-bot. Give it either the bare address or the full endpoint — a
   value with no path gets git-bot's `/webhook` appended, and the step shows the
   resolved address before you leave for GitHub. NopsAI itself does not have to
   be reachable from the internet, because GitHub only sends the operator's
   browser back to the NopsAI address it is already on.
4. Approve the App on GitHub, then select the account and repositories there.
   The installation registers itself when GitHub returns, and so do later
   installs or uninstalls done directly on GitHub.

git-bot picks up the new credentials within a minute, so no container restart is
needed, including while the wizard is still open: the first-install gate that
locks the rest of the workspace exempts git-bot's internal bootstrap and
installations routes, which prove a git-bot service identity of their own.
Installing the App on further accounts later needs no NopsAI action at all.

Manual flow, for GitHub Enterprise Server, air-gapped installs, or an App that
already exists:

1. Create or open a GitHub App and set its webhook URL to the public git-bot
   endpoint exposed by your deployment, ending in `/webhook`.
2. Store the private key and webhook secret in **Credentials**.
3. Configure the App ID, credential references, and each installation account in
   **System > Git Apps** using **Add manually**.

Required GitHub App events:

- `push`
- `pull_request`
- `check_run`
- `check_suite`

Do not add `installation` or `installation_repositories`. GitHub sends App
lifecycle events to every App without a subscription and refuses a manifest that
lists them, so NopsAI receives them without asking.

Required GitHub App permissions:

- `contents`: read and write
- `metadata`: read
- `pull_requests`: read
- `checks`: read and write

Repository teams are entered manually as GitHub `owner/repo` names or GitHub
URLs. Starter GitOps structure stores each app with a `repo_url`, which NopsAI
normalizes for trigger-to-app matching. The direct database seed mirrors the
selected team names as top-level teams; it does not create an extra workspace
parent above them. If repository teams are disabled, setup does not create a
synthetic team root. If a repository does not trigger later, verify the GitHub
App ID, installation ID, private key, webhook secret, public git-bot webhook
URL, internal git-bot service URL, and which repositories are selected in the
GitHub App installation.

## Starter Templates

`/v1/setup/templates` returns a GitOps-ready file set for the starter workspace
and selected repositories. It is an API and CLI surface; the setup page does not
preview or download it, because config sync already carries setup's resources to
the config repository:

```text
README.md
pipelines/setup/first-run.yaml
steps/setup/announce.yaml
triggers/<owner>/<repo>.yaml
scopes/dev/scope.yaml
scopes/prod/scope.yaml
knowledge/guideline/<team>/setup-run.md
access/bootstrap.yaml
models/<name>.yaml
mcp/servers/<name>.yaml
config-repositories/teams/<team>/structure.yaml
```

The wizard seeds equivalent starter resources directly into the database. When
GitOps is enabled, config sync carries them to the global config repository.

## Security Guardrails

Setup is deliberately conservative:

- Local secret values are generated outside Git and are never returned in the
  API response.
- The local development `admin/admin` state is reported as an error until changed.
- Webhook delivery depends on `github_webhook_secret`; disabled or missing
  signature verification is not treated as production-ready.
- Optional steps can be skipped, but skipped integrations are reported in the
  generated output so operators know what remains.

## API Reference

```bash
# Public installation preflight, usable before login
curl http://localhost:8080/v1/setup/preflight

# Setup status and health checks
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  http://localhost:8080/v1/setup/status

# Preview starter GitOps templates
curl -H "Authorization: Bearer $NOPSAI_TOKEN" \
  "http://localhost:8080/v1/setup/templates?profile=team&repositories=acme/service-api"

# Apply setup
curl -X POST \
  -H "Authorization: Bearer $NOPSAI_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "profile": "team",
    "generate_secrets": true,
    "seed_starter_database": true,
    "seed_llm_profile": true,
    "sync_config_repository": false,
    "mcp_examples": false,
    "config_repository": {
      "repo_url": "https://github.com/acme/nopsai-config.git",
      "branch": "main",
      "base_path": "",
      "enabled": true
    },
    "repository_teams": [
      {"name": "platform", "repositories": ["acme/service-api"]},
      {"name": "applications", "repositories": ["acme/web-app"]}
    ],
    "repositories": ["acme/service-api"],
    "model": {
      "name": "standard",
      "provider": "lmstudio",
      "model": "qwen3-coder",
      "base_url": "http://lmstudio:1234",
      "allowed_scopes": ["dev", "prod"]
    },
    "users": [
      {
        "sub": "alice@example.com",
        "email": "alice@example.com",
        "role": "owner",
        "team": "platform",
        "password": "temporary-password"
      }
    ]
  }' \
  http://localhost:8080/v1/setup/bootstrap
```

Important response fields:

- `generated_secrets`: environment variable names that were generated.
- `requires_restart`: services should be restarted to pick up changed runtime
  values.
- `temporary_credentials`: one-time local user credentials for seeded users.
- `warnings`: non-blocking setup warnings, including when model setup is
  skipped and AI-enabled pipelines may not work yet.
- `status`: refreshed setup status and health checks after bootstrap.

## Troubleshooting

- **GitHub webhooks do not arrive**: Check the public webhook URL, webhook
  secret, git-bot service networking, and that git-bot can forward to
  `nopsai_api_url`.
- **The connect button spins and never reaches GitHub**: the browser blocked the
  manifest form post. NopsAI's own `Content-Security-Policy` allows it with
  `form-action 'self' https://github.com`; a reverse proxy in front of the UI
  that sets a stricter policy has to allow the same.
- **git-bot logs `credential broker returned 403`**: git-bot could not read the
  GitHub App credentials and is retrying in degraded mode. Expected before the
  GitHub step has connected an App; if it persists afterwards, check that
  git-bot's `NOPSAI_API_URL` reaches the API and that both services share the
  same `SERVICE_JWT_SIGNING_KEY`.
- **Starter run cannot execute**: Check dispatcher/runner health, runner
  registration, Docker access, and the configured model.

## Licence Acceptance

NopsAI is proprietary software, and possession of a binary, image, or chart
grants no right to use it. Container images are publicly pullable, so the
wizard is where an anonymous puller becomes an installation that has agreed to
terms.

The first wizard step shows the full notice — not a link to it — with an
explicit checkbox and a disabled continue button until it is ticked. Acceptance
records four keys in `setup_state`: `license_accepted_at`,
`license_accepted_by`, `license_document_version`, and
`license_document_sha256`, plus a `system.license.accept` audit entry naming
the accepting administrator and the exact document digest.

The gate fails closed in three places, so no single UI change can bypass it:

1. The accept endpoint refuses a digest that does not match the notice the
   server is serving, which stops a stale browser tab from accepting
   superseded wording.
2. `markSetupComplete` refuses to write `completed_at` while the current notice
   is unaccepted, and refuses equally when acceptance cannot be evaluated at
   all. `POST /v1/setup/bootstrap` answers `412 Precondition Failed`.
3. The wizard disables both Continue and Apply setup until acceptance is
   recorded.

Because the first-install gate already blocks the rest of the API until
`setup_state.completed_at` is written, an installation that never accepts the
notice never becomes usable.

### Re-acceptance After A Changed Notice

Acceptance is bound to a document digest rather than to a boolean. An upgrade
that changes the notice leaves `license_document_sha256` pointing at wording the
installation is no longer running, so `accepted` becomes false and the wizard
asks for acceptance again, naming the version previously agreed to. A release
that does not change the notice leaves acceptance untouched.

The notice text lives in `pkg/licensenotice` rather than being embedded from the
repository root, because `go:embed` cannot reach outside a package directory.
`TestLicenseNoticeMatchesShippedLicenseFile` fails if that copy ever drifts from
the `LICENSE` file shipped beside the binaries, so the accepted notice is always
the shipped notice.
