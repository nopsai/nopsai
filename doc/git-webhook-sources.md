# Git Webhook Sources

Git Webhook Sources connect GitLab, Bitbucket Cloud, Gitea, and normalized
generic Git events to NopsAI trigger manifests. They are repository-event
entrypoints, not pipeline-bound endpoints.

Use External Triggers for systems such as ServiceNow or Jenkins jobs that
directly invoke one configured pipeline. Use Git Webhook Sources when the
repository event should be evaluated against trigger rules and may start one or
more pipelines.

## Runtime Flow

1. A provider posts to `POST /v1/git/webhooks/{sourceID}`.
2. NopsAI loads the enabled source and resolves its credential reference.
3. The raw request body is authenticated before JSON normalization.
4. The provider payload is normalized to repository, event, ref, target ref,
   commit, actor, changed files, URLs, and delivery ID.
5. The normalized repository must match the source allowlist.
6. NopsAI records an idempotent delivery audit row and applies the source rate
   limit.
7. The assigned NopsAI repository trigger is loaded. Missing or unassigned
   triggers finish as `no_match`.
8. Matched pipeline definitions are loaded from the NopsAI database and started
   through the normal run path with the repository as runtime caller.
9. The delivery is finalized as `processed`, `partial`, `no_match`, or `failed`
   with its run IDs and error summary.

Generic Git runs use `pipeline_source=git_webhook` and
`trigger_source=git_webhook_<provider>`. They do not create or update GitHub
checks.

If an exact allowlist entry has no NopsAI trigger, the source detail UI shows:

```text
Repository allowed, but no NopsAI trigger is configured.
Webhook events will not start pipelines.
```

## V1 Configuration Boundary

GitHub can fall back to `.nopsai/triggers.yaml` and repository pipeline files
because `git-bot` provides GitHub repository reads. Generic providers do not
yet have an equivalent repository-content client.

For Git Webhook Sources:

- synchronize trigger overrides into NopsAI through `triggers/` GitOps files or
  the trigger override API;
- synchronize every referenced pipeline through `pipelines/`;
- use pipeline identifiers, not remote HTTP URLs;
- treat the config repository as the reviewed source of truth.

This keeps v1 provider-neutral without embedding provider API credentials or
repository read behavior in webhook adapters.

## Source Configuration

Create sources from **Git Webhook Sources** in the UI, the REST API, or a config
repository under `git-webhook-sources/*.yaml`.

```yaml
id: gitlab-platform
name: GitLab Platform
description: Primary GitLab repository event source
provider: gitlab
enabled: true
team_path: platform
visibility: workspace
auth_mode: static_token
credential_ref: credential://system/webhooks/gitlab-platform
repository_allowlist:
  - platform/api
  - platform/*
rate_limit:
  per_minute: 120
```

`team_path` is the owner path. `visibility: team` limits assignment to triggers
in the same team boundary. `visibility: workspace` makes the source
workspace-shared so any team can assign a compatible repository trigger.

Supported providers:

- `generic`
- `gitlab`
- `bitbucket`
- `gitea`

Supported authentication modes:

- `hmac`: HMAC-SHA256 over the exact raw request body.
- `static_token`: GitLab uses `X-Gitlab-Token`; generic sources use
  `X-Nopsai-Token` or `Authorization: Bearer`.
- `none`: no application-layer authentication. Use only behind a trusted,
  network-isolated ingress.

Provider HMAC headers:

- GitLab Standard Webhooks:
  `webhook-id`, `webhook-timestamp`, and `webhook-signature`. Signing secrets
  use the `whsec_` format and timestamps have a five-minute replay window.
- Bitbucket Cloud: `X-Hub-Signature`.
- Gitea: `X-Gitea-Signature`, with `X-Hub-Signature-256` accepted as a
  compatibility fallback.
- Generic: `X-Nopsai-Signature-256`, with `X-Hub-Signature-256` accepted as a
  compatibility fallback.

Credential plaintext remains write-only in **Credentials**. Source
GitOps stores the credential reference, and `setting/system/credentials.yaml`
can store the encrypted credential envelope for the same reference.

## Repository Allowlist

Every source requires at least one `owner/repository` pattern. Matching is
case-insensitive and supports:

- `platform/api`
- `platform/*`
- `platform/**`

The allowlist is evaluated after signature verification and provider
normalization, before trigger lookup.

## Repository Triggers

Repository trigger manifests still use the existing `triggers:` rules, with
optional top-level metadata:

```yaml
provider: gitlab
team: team-1
webhook_source: gitlab-platform
management: nopsai
triggers:
  - on: push
    branches:
      - main
    pipelines:
      - team-1/build
      - shared/security-scan
    scope: team-1/dev
```

Rules:

- `repository_name` remains unique: one trigger configuration owns one
  repository key.
- The trigger can contain multiple event, branch, tag, and path rules.
- A matched rule can start multiple pipelines.
- `team` associates the repository/app with that team for runtime resource
  authorization.
- Non-GitHub providers require `webhook_source`.
- GitHub App triggers must not set `webhook_source`; ingress is automatic.
- NopsAI-managed triggers take precedence over a repository
  `.nopsai/triggers.yaml` file.

The trigger UI edits the same top-level YAML metadata for provider, team,
management mode, and webhook source through create/edit workflow dialogs. The
explorer groups triggers by Git owner and then by NopsAI team, but save/delete
and webhook matching still use only the provider repository key (`owner/repo`).
Team assignment is selected from known NopsAI teams. For non-GitHub providers,
the UI requires choosing an existing compatible Git webhook source; GitHub uses
automatic GitHub App ingress. UI-created, edited, and cloned trigger overrides
are stored with team resource visibility so they remain compatible with the
same access and GitOps sync model as repository trigger manifests.

For GitHub repositories without a NopsAI-managed trigger, git-bot reads
`.nopsai/triggers.yaml` from the repository and treats that file as read-only
repository-managed configuration in NopsAI.

## Trigger Path Filters

Trigger manifests support `include_paths` and `exclude_paths`:

```yaml
triggers:
  - on: push
    branches:
      - main
      - release/*
    include_paths:
      - services/api/**
      - pkg/**
    exclude_paths:
      - docs/**
      - "**/*.md"
    pipelines:
      - platform/api-ci
    scope: production
```

Semantics:

- Empty `include_paths` matches any changed file.
- Excluded files are removed before include matching.
- If only excluded files remain, the trigger does not match.
- If changed-file data is unavailable, matching fails open and the pipeline
  runs. Skipping CI because a provider omitted file data is unsafe.
- Pull-request branch filters apply to the target branch.

## Generic Payload

The generic adapter accepts `push`, `pull_request`, or `merge_request`
(`merge_request` normalizes to `pull_request`):

```json
{
  "event_type": "push",
  "repository": {
    "full_name": "platform/api",
    "owner": "platform",
    "name": "api",
    "clone_url": "https://git.example/platform/api.git",
    "ssh_url": "git@git.example:platform/api.git"
  },
  "ref": "refs/heads/main",
  "target_ref": "",
  "commit_sha": "0123456789abcdef",
  "before_sha": "fedcba9876543210",
  "changed_files": ["services/api/main.go"],
  "delivery_id": "delivery-123",
  "commit": {
    "url": "https://git.example/platform/api/commit/0123456",
    "message": "Update API",
    "author": "Ada",
    "email": "ada@example.com",
    "username": "ada"
  },
  "actor": {
    "name": "Ada",
    "email": "ada@example.com"
  }
}
```

When `delivery_id` is absent, NopsAI derives one from the request body. Repeated
deliveries for the same source and delivery ID are acknowledged without
starting duplicate runs.

## Authorization And GitOps

Management actions are:

- `git_webhook_source.read`
- `git_webhook_source.create`
- `git_webhook_source.update`
- `git_webhook_source.delete`
- `git_webhook_source.manage_acl`

Webhook delivery is public at the bearer-token layer because providers cannot
use NopsAI sessions. Source authentication, the repository allowlist, rate
limits, payload limits, delivery idempotency, trigger assignment, and runtime
resource checks form the ingress boundary.

Runtime authorization checks the repository independently against each resource:
`pipeline.use`, `scope.use`, reusable `step.use`, and child `pipeline.use`.
Same-team resources are normally available. Cross-team resources require
explicit sharing or workspace visibility.

Direct update and delete APIs are allowed when AAA permits. Updating a
GitOps-managed source stores a database override, and deleting one removes the
database row; the next GitOps sync can replace or recreate it unless the change
is pushed to the owning config repository. Config sync imports, updates, and
prunes source manifests, while config drift/write exports database-created
sources into the global config repository.

## Operations

Use `GET /v1/git-webhook-sources/{sourceID}/deliveries` or the UI detail panel
to inspect the latest 100 deliveries.

Operational checks:

- rotate webhook credentials in the registry without changing the stable
  source reference;
- configure provider retries with stable delivery IDs;
- keep unauthenticated sources on private ingress only;
- review `partial` and `failed` deliveries for missing pipelines, runtime AAA
  denials, or invalid trigger configuration;
- review `no_match` deliveries for missing triggers, unassigned triggers, or
  source/trigger provider mismatches;
- use source-level rate limits in addition to ingress rate limiting;
- keep trigger overrides and pipelines synchronized before enabling a source.
