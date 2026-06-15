# Triggering Pipelines Locally

Use this guide to simulate GitHub webhook traffic against the git-bot and verify that the end-to-end pipeline orchestration works as expected.

---

## Prerequisites

- Git-bot running locally and reachable on `http://localhost:8081/webhook` (default from `docker-compose`).
- The webhook secret used by your GitHub App. For local testing you can generate a new random string.
- A sample payload. `doc/sample-git-event.json` mirrors a GitHub `push` event and references the repository/branch used throughout the examples.

---

## 1. Export the Webhook Secret

```bash
export TEST_GITHUB_WEBHOOK_SECRET="<the-value-written-to-the-webhook credential>"
```

> Replace the value with the secret configured in your GitHub App. The git-bot will reject payloads whose signature does not match this secret.

---

## 2. Compute the Signature

```bash
PAYLOAD_FILE="doc/sample-git-event.json"
SIGNATURE=$(openssl dgst -sha256 -hmac "$TEST_GITHUB_WEBHOOK_SECRET" "$PAYLOAD_FILE" | awk '{print $2}')
echo "sha256=$SIGNATURE"
```

- The calculated signature must be sent in the `X-Hub-Signature-256` header.
- Recompute the signature whenever you modify the payload file.

---

## 3. Send the Webhook

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: push" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  --data-binary "@${PAYLOAD_FILE}" \
  http://localhost:8081/webhook
```

- A `202 Accepted` response confirms the git-bot has queued the event for processing.
- The core API should create a new pipeline run; refresh the UI or inspect `/v1/runs`.
- Git-bot logs (container `nopsai-git-bot`) will show signature validation, repository lookup, and check-run coordination.

---

## Customising the Payload

- Update `doc/sample-git-event.json` to reflect the repository, commit, and branch you want to test.
- To mimic pull requests, change `"event": "push"` to `"pull_request"` and provide the corresponding PR payload structure.
- Ensure that the referenced repository is installed on your GitHub App and that triggers exist for the event/branch combination.

---

## Troubleshooting

- **Signature mismatch**: Confirm the GitHub App, your local test variable, and
  the active webhook credential version use the same value.
- **Run not created**: Check the git-bot logs for repository access issues or trigger mismatches. The API logs (container `nopsai`) will also record trigger evaluations.
- **No UI updates**: Refresh the Pipeline runs page and verify authenticated `/v1/runs` requests are succeeding.

When integrating with GitHub cloud, expose the git-bot via a tunnelling service (ngrok, Cloudflare Tunnel, etc.) and point the GitHub App webhook URL to that public address.

---

## Authenticated External Triggers

For enterprise integrations that are not GitHub webhooks, create an External
Trigger and invoke it with a user token or, preferably, a service-account token.

```bash
curl -X POST \
  -H "Authorization: Bearer nopsat_<service-account-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "event_type":"servicenow.change.approved",
    "idempotency_key":"servicenow.change.approved:<SOURCE_EVENT_ID>",
    "variables":{"VERSION":"1.2.3"},
    "payload":{"version":"1.2.3"}
  }' \
  http://localhost:8080/v1/external-triggers/deploy-prod/invoke
```

External triggers are not anonymous webhooks. The caller must be listed in the
trigger's allowed callers, have `external_trigger.invoke` on the trigger, and
must still be allowed to execute the selected pipeline and use the selected
scope/resources.

These are separate checks. A caller in `allowed_callers` still needs an AAA
policy such as `external_trigger.invoke` on `external_trigger:deploy-prod`
or, for intentionally broad integrations, `external_trigger:*`. Folder-scoped
pipeline access does not automatically grant permission to invoke a standalone
external trigger.

The selected run scope is also checked through runtime resource access. For a
service account that deploys to a restricted scope, share the scope with that
account in the scope file or the Scope Access dialog:

```yaml
access:
  visibility: restricted
  use_access:
    grants:
      - service_account: servicenow-prod
```

`idempotency_key` is optional, but recommended for production integrations.
Use a stable source-system event ID: reuse the same key for retries of the same
event, and use a new key for a new change, ticket, deployment, or approval.
Manual tests can omit it or use a fresh test key.

GitOps-managed external triggers live in `external-triggers/*.yaml`. Config sync
imports them, direct UI/API edits are blocked while GitOps owns them, and config
repository drift can push database-created triggers back into that directory.
