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
export GITHUB_WEBHOOK_SECRET="vsfdverguhuyi3287467324ujfbsaihufb"
```

> Replace the value with the secret configured in your GitHub App. The git-bot will reject payloads whose signature does not match this secret.

---

## 2. Compute the Signature

```bash
PAYLOAD_FILE="doc/sample-git-event.json"
SIGNATURE=$(openssl dgst -sha256 -hmac "$GITHUB_WEBHOOK_SECRET" "$PAYLOAD_FILE" | awk '{print $2}')
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

- **Signature mismatch**: Confirm both the git-bot OS variable (`GITHUB_WEBHOOK_SECRET`) and your shell export use the same value.
- **Run not created**: Check the git-bot logs for repository access issues or trigger mismatches. The API logs (container `nopsai`) will also record trigger evaluations.
- **No UI updates**: Refresh the Pipeline runs page and verify authenticated `/v1/runs` requests are succeeding.

When integrating with GitHub cloud, expose the git-bot via a tunnelling service (ngrok, Cloudflare Tunnel, etc.) and point the GitHub App webhook URL to that public address.
