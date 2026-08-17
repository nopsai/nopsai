# Git Apps

Git Apps stores provider app configuration separately from trigger ownership.
For GitHub, NopsAI supports multiple GitHub App installations and routes
repository operations by repository owner.

A GitHub App can be connected from the UI without leaving the browser: NopsAI
generates the App manifest, GitHub asks the operator to approve it, and the App
ID, private key, and webhook secret come back automatically. Manual entry stays
available for GitHub Enterprise Server and air-gapped installations.

## Ownership

- Model logic: `config.Config.GitHubInstallations` and GitOps normalization in
  `config/config.go` and `services/nopsai/github_settings_gitops.go`.
- API logic: `services/nopsai/git_apps_handlers.go`.
- git-bot routing: `services/git-bot/internal/service/github_resolver.go`.
- Registration model logic: `services/nopsai/git_apps_registration_model.go`.
- Registration state store: `services/nopsai/git_apps_registration_store.go`.
- Registration API logic: `services/nopsai/git_apps_registration_handlers.go`.
- Installation lifecycle events: `services/nopsai/git_apps_installation_events.go`.
- git-bot credential refresh: `services/git-bot/internal/app/github_runtime.go`.
- Hook orchestration: `services/ui/src/features/system/git-apps/useGitHubApp.ts`.
- Rendering: `services/ui/src/features/system/git-apps/GitHubAppPanel.tsx` and
  `services/ui/src/features/system/git-apps/GitHubAppConnectCard.tsx`.
- Setup wizard step: `services/ui/src/features/system/setup/SetupGitHubStep.tsx`.
- Route composition: `services/nopsai/routes.go` and `services/ui/src/pages/System.tsx`.

## API

- `GET|PUT /v1/git-apps/github`
- `POST /v1/git-apps/github/register/start`
- `GET /v1/git-apps/github/register/callback`
- `POST /v1/git-apps/github/install/start`
- `GET /v1/git-apps/github/install/callback`
- `GET|POST /v1/git-apps/github/installations`
- `GET|DELETE /v1/git-apps/github/installations/{installationID}`
- `POST /v1/git-apps/github/installations/{installationID}/verify`
- `POST /v1/git-apps/github/installations/{installationID}/refresh`
- `GET /v1/git-apps/github/installations/{installationID}/repositories`

All public management routes map to `system.read` or `system.update` on
`system:config`. The two callbacks are the exception: GitHub redirects the
operator's browser to them without a bearer token, so they authorize themselves
with a single-use, short-lived state row created by an authorized start request.
git-bot reads enabled installations through
`GET /v1/internal/git-bot/installations` using the internal git-bot service
identity.

## Connect Flow

1. **Register.** `POST /v1/git-apps/github/register/start` builds a GitHub App
   manifest from `public_url` and returns it with the GitHub URL to post it to.
   GitHub only accepts manifests through a form submission from the operator's
   session, so the browser posts the manifest rather than the API.
2. **Convert.** GitHub redirects to `/v1/git-apps/github/register/callback` with
   a one-time code. NopsAI exchanges it for the App ID, slug, private key, and
   webhook secret, writes the two secrets to the credential store, and stores the
   App ID and slug in configuration. The OAuth client ID and secret GitHub also
   returns are discarded: NopsAI authenticates as the App, never as an OAuth
   client.
3. **Install.** The operator is sent to `https://github.com/apps/{slug}/installations/new`
   to choose the account and repositories. GitHub calls
   `/v1/git-apps/github/install/callback` with the `installation_id`, which
   NopsAI verifies with an App-authenticated GitHub call before storing it.

The generated App is private, requests no OAuth on install, and asks for exactly
the events and permissions listed below.

### Two Different URLs

A GitHub App uses two addresses, and only one of them has to be public:

- The **webhook URL** is fetched by GitHub's servers and must reach git-bot's
  `/webhook`. This is usually a tunnel or reverse proxy in front of git-bot. It
  is stored as `webhook_url` on the Git App and is the only value the connect
  flow requires.
- The **redirect and setup URLs** are only ever opened in the operator's own
  browser. NopsAI builds them from the address that browser is already using, so
  a workspace on `http://localhost:8080` behind a tunnel that exposes nothing but
  git-bot is a complete, supported setup. `public_url` is used when the request
  carries no browser origin, such as a CLI call.

The browser origin is accepted only when it matches the host serving the request,
a configured CORS origin, or `public_url`, so a foreign origin cannot redirect
GitHub's response somewhere else. When neither a browser origin nor `public_url`
is available, the flow refuses to start rather than registering an App whose
callbacks lead nowhere.

Required GitHub App events: `push`, `pull_request`, `check_run`, `check_suite`,
`installation`, `installation_repositories`.

Required GitHub App permissions: `contents` read and write, `metadata` read,
`pull_requests` read, `checks` read and write.

## GitOps

Canonical path:

```yaml
provider: github
app_id: "123456"
app_slug: nopsai-acme
webhook_url: https://nopsai-git-bot.example.com/webhook
private_key_credential_ref: credential://system/github/app-private-key
webhook_credential_ref: credential://system/github/webhook-secret
installations:
  - installation_id: "987654"
    account_login: nopsai
    account_type: organization
    enabled: true
```

`webhook_url` is where GitHub delivers events; when it is empty NopsAI falls back
to `public_url` + `/webhook`. A base address such as `https://example.ngrok.app`
is stored with `/webhook` appended.

`app_slug` is the App's GitHub URL name and is what the install action opens.
Editing `app_id` without a matching `app_slug` clears the slug, so an install
link can never point at a different App.

`setting/git-apps/github.yaml` must not include `git_bot_api_url`, `team_path`,
`visibility`, `repository_allowlist`, or runtime repository metadata. Internal
git-bot service URLs stay in System Config or deployment service configuration.
Trigger ownership remains in trigger manifests and Git webhook source records.

For one release, `setting/system/github.yaml` is read to migrate legacy
`github_installation_id` into the first installation record. Exports and UI
writes use `setting/git-apps/github.yaml` and stop writing the scalar field.

## Runtime Behavior

git-bot fetches the enabled installation catalog from NopsAI and caches
per-installation GitHub clients. Repository calls resolve by owner login.
Webhook deliveries must include `installation.id`; unknown or disabled
installations are rejected before the event is forwarded to NopsAI.

git-bot re-reads the App ID, private key, and webhook secret from NopsAI on an
interval and swaps them in place, dropping cached GitHub clients that used the
superseded key. Connecting or rotating a GitHub App therefore takes effect
without restarting the git-bot container, and a git-bot that started before any
App existed recovers on its own instead of staying degraded until a restart. A
failed refresh keeps the last working credentials.

`installation` and `installation_repositories` deliveries bypass the
installation registry check, because they describe installations NopsAI does not
know yet or has just lost; the verified webhook signature is what makes them
trustworthy. NopsAI registers new installations from them, refreshes account
metadata, disables suspended installations, and removes deleted ones. A
repository-selection change never re-enables an installation an operator
disabled.

Opening the Git Apps UI reads the app and installation records only; it does not
list repositories from GitHub. Repository details are loaded only by explicit
`Load` or `Refresh` actions. `Refresh` persists lightweight runtime metadata on
the installation record, including accessible repository count, last refresh
timestamp, and last error. Repository names and per-repository details are
returned only to the active response and are never written to GitOps config.

## Monitoring And MCP

GitHub App repository refresh and webhook rejection paths are logged through the
API and git-bot service logs. System Logs can be used to inspect `git-bot` and
`nopsai` events, while Monitoring continues to report run outcomes and webhook
triggered pipeline activity. Hosted MCP follows the same AAA permissions as the
UI/API surface; Git Apps management remains a system-config capability rather
than a trigger-scoped action.
