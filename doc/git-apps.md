# Git Apps

Git Apps stores provider app configuration separately from trigger ownership.
For GitHub, NopsAI supports multiple GitHub App installations and routes
repository operations by repository owner.

## Ownership

- Model logic: `config.Config.GitHubInstallations` and GitOps normalization in
  `config/config.go` and `services/nopsai/github_settings_gitops.go`.
- API logic: `services/nopsai/git_apps_handlers.go`.
- git-bot routing: `services/git-bot/internal/service/github_resolver.go`.
- Hook orchestration: `services/ui/src/features/system/git-apps/useGitHubApp.ts`.
- Rendering: `services/ui/src/features/system/git-apps/GitHubAppPanel.tsx`.
- Route composition: `services/nopsai/routes.go` and `services/ui/src/pages/System.tsx`.

## API

- `GET|PUT /v1/git-apps/github`
- `GET|POST /v1/git-apps/github/installations`
- `GET|DELETE /v1/git-apps/github/installations/{installationID}`
- `POST /v1/git-apps/github/installations/{installationID}/verify`
- `POST /v1/git-apps/github/installations/{installationID}/refresh`
- `GET /v1/git-apps/github/installations/{installationID}/repositories`

All public management routes map to `system.read` or `system.update` on
`system:config`. git-bot reads enabled installations through
`GET /v1/internal/git-bot/installations` using the internal git-bot service
identity.

## GitOps

Canonical path:

```yaml
provider: github
app_id: "123456"
private_key_credential_ref: credential://system/github/app-private-key
webhook_credential_ref: credential://system/github/webhook-secret
installations:
  - installation_id: "987654"
    account_login: nopsai
    account_type: organization
    enabled: true
```

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
