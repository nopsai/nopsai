# First-install Wizard

The first-install wizard guides an administrator from an empty database to a
working NopsAI workspace without reading the full configuration reference first.
It lives in the UI at **System > Setup** and is backed by `/v1/setup/*` API
endpoints. After the default admin changes the first-login password, the UI
checks setup status once per browser session and automatically opens a guided
setup modal when setup is incomplete.

Before login, the UI also checks `/v1/setup/preflight`. If required
configuration is missing, the login page shows installation guidance instead of
leaving the operator at a broken sign-in form.

Use it for a new installation, a local evaluation environment, or a controlled
bootstrap of a team workspace. The wizard uses one setup path: required steps
must be completed, while optional integrations can be skipped and configured
later.

## Required Access

The authenticated setup page is a system administration surface:

- `GET /v1/setup/preflight` is public so the UI can explain missing initial
  configuration before authentication is available.
- `GET /v1/setup/*` requires `system.read` on `system:config`.
- `POST /v1/setup/*` requires `system.update` on `system:config`.

The default local admin can open the wizard after login, but the health checks
will warn if the default `admin` password is still in use or if the admin must
change password.

## Wizard Shape

There are no starter profiles in the UI. The wizard is a single guided flow:

- required readiness and runtime configuration steps
- optional GitOps repository connection and sync kickoff
- optional repository groups, limited to one or two starter groups for an
  introduction
- optional default AI setup with one API key field
- optional user creation with group, role, and temporary password assignment
- final generated output for runtime variables, service/container environment,
  and GitOps files

## Wizard Flow

1. Open the UI. The login page checks preflight readiness for the database,
   master encryption key, and JWT signing key.
2. Resolve any required preflight items in `docker-compose.yaml`, exported
   environment, or `config.yml`, then restart affected services.
3. Sign in as an administrator. On an incomplete installation the UI opens the
   setup modal automatically. If the bootstrap admin must change password, the
   Profile page is shown first and setup opens after the password requirement is
   cleared.
4. Review health checks for database connectivity, local secrets, admin
   bootstrap state, git-bot service configuration, access grants, LLM/MCP
   configuration, starter pipeline presence, and runner connectivity.
5. Review runtime values for service-to-service auth, git-bot forwarding, and
   service discovery. The final step prints variables that can be applied as
   container environment, secret-manager values, or an environment file.
6. Optionally connect a global GitOps config repository and start sync.
7. Create one or two repository groups and place selected repositories under
   them. These groups drive starter trigger generation, run navigation, and
   initial access assignments.
8. Optionally configure the default LLM profile. For local development, the
   default is LM Studio at `http://lmstudio:1234` with model `qwen3-coder`.
   The catalog also supports Gemini, OpenAI / ChatGPT, Anthropic Claude, Groq,
   Mistral, OpenRouter, Ollama, and Azure OpenAI. Hosted providers use one API
   key field, stored as a NopsAI secret.
9. Optionally seed disabled MCP examples for later activation.
10. Optionally create starter users, assign them to a group with owner,
    developer, or viewer role, and set or generate temporary passwords. Created
    local users must change password on first login.
11. Review generated runtime variables, GitOps folder/file layout, and
    post-setup instructions.
12. Apply setup, then run the starter `setup/first-run` pipeline to verify the
    runner, agent, LLM path, logs, and UI.

The setup modal is step-by-step. Optional steps such as GitOps, repository
groups, AI, MCP examples, and users can be skipped and completed later. The
review step summarises generated variables, GitOps files, repository groups,
selected repositories, AI settings, and user assignments before anything is
applied.

After setup is completed, **System > Setup** remains available as an operator
reference page. The page keeps the same step navigation and opens on the output
step by default so runtime env groups, GitOps zip download, and generated file
preview can be inspected again later.

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

The login page shows suggested environment entries for missing required values.
The wizard can generate later service secrets after login, but these hard
pre-authentication values must exist before an authenticated workspace can open.

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
- The built-in `admin@example.com` account must not use the default password.
  Production gate mode does not auto-seed default admin credentials.

## Generated Service Secrets

When **Generate missing service secrets** is enabled, the wizard fills missing
values for:

- `JWT_SIGNING_KEY`
- `SERVICE_JWT_SIGNING_KEY`
- `AAA_SHARED_INTERNAL_TOKEN`
- `DISPATCHER_TLS_SECRET`

The API response lists the names that were generated and sets
`requires_restart` when runtime values changed. Restart services after
generating secrets so every process reads the same values.

Do not commit these values to the config repository.

## GitHub App And git-bot

The wizard prepares the internal NopsAI-to-git-bot service URL. In Docker
Compose that URL is usually `http://git-bot:8081`. Manage GitHub App IDs and
credential references in **System > Config** or `setting/system/github.yaml`,
store private-key and webhook secret values in **System > Credentials**, and set
the public webhook URL on the GitHub App.

Install flow:

1. Start the `git-bot` service with `NOPSAI_API_URL` pointing at the NopsAI API
   URL reachable from git-bot, usually `http://nopsai:8080` in Docker Compose.
2. Create or open a GitHub App and set its webhook URL to the public git-bot
   endpoint exposed by your deployment, ending in `/webhook`.
3. Configure the App ID, installation ID, private-key credential reference, and
   webhook credential reference in **System > Config**, then install the App on
   the selected repositories.

Required GitHub App events:

- `push`
- `pull_request`
- `check_run`
- `check_suite`
- `ping`

Required GitHub App permissions:

- `contents`: read and write
- `metadata`: read
- `pull_requests`: read
- `checks`: read and write

Repository groups are entered manually as GitHub `owner/repo` names or GitHub
URLs. Starter GitOps structure stores each app with a `repo_url`, which NopsAI
normalizes for trigger-to-app matching. If a repository does not trigger later,
verify the GitHub App ID, installation ID, private key, webhook secret, public
git-bot webhook URL, internal git-bot service URL, and which repositories are
selected in the GitHub App installation.

## Starter Templates

The template preview returns a GitOps-ready file set for the starter workspace
and selected repositories:

```text
README.md
pipelines/setup/first-run.yaml
steps/setup/announce.yaml
triggers/<owner>/<repo>.yaml
scopes/dev/scope.yaml
scopes/prod/scope.yaml
knowledge/guideline/platform/setup-run.md
access/bootstrap.yaml
setting/system/llm_profile.yaml
setting/system/mcp.yaml
config-repositories/groups/<group>/structure.yaml
```

The wizard can seed equivalent starter resources directly into the database for
a fast introduction. When GitOps is enabled, commit these files to the global
config repository and sync them through config sync.

## Security Guardrails

Setup is deliberately conservative:

- Local secret values are generated outside Git and are never returned in the
  API response.
- The default `admin/admin` state is reported as an error until changed.
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
    "repository_groups": [
      {"name": "platform", "repositories": ["acme/service-api"]},
      {"name": "applications", "repositories": ["acme/web-app"]}
    ],
    "repositories": ["acme/service-api"],
    "llm_profile": {
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
        "group": "platform",
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
- `warnings`: non-blocking setup warnings, including when LLM profile setup is
  skipped and AI-enabled pipelines may not work yet.
- `status`: refreshed setup status and health checks after bootstrap.

## Troubleshooting

- **GitHub webhooks do not arrive**: Check the public webhook URL, webhook
  secret, git-bot service networking, and that git-bot can forward to
  `nopsai_api_url`.
- **Starter run cannot execute**: Check dispatcher/runner health, runner
  registration, Docker access, and the configured LLM profile.
