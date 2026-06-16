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
7. The database trigger override for the repository, including an applicable
   repository-group override, is matched.
8. Matched pipeline definitions are loaded from the NopsAI database and started
   through the normal run path with the repository as runtime caller.
9. The delivery is finalized as `processed`, `partial`, `no_match`, or `failed`
   with its run IDs and error summary.

Generic Git runs use `pipeline_source=git_webhook` and
`trigger_source=git_webhook_<provider>`. They do not create or update GitHub
checks.

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
auth_mode: static_token
credential_ref: credential://system/webhooks/gitlab-platform
repository_allowlist:
  - platform/api
  - platform/*
rate_limit:
  per_minute: 120
```

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

Credential values remain write-only in **System > Credentials**. GitOps creates
or references credential metadata but never stores the secret value.

## Repository Allowlist

Every source requires at least one `owner/repository` pattern. Matching is
case-insensitive and supports:

- `platform/api`
- `platform/*`
- `platform/**`

The allowlist is evaluated after signature verification and provider
normalization, before trigger lookup.

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
limits, payload limits, delivery idempotency, and runtime resource checks form
the ingress boundary.

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
- use source-level rate limits in addition to ingress rate limiting;
- keep trigger overrides and pipelines synchronized before enabling a source.
