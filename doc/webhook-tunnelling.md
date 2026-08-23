# Webhook Tunnelling for Local Development

GitHub, GitLab, Bitbucket, and Gitea deliver webhooks over the public internet.
When you develop NopsAI on a laptop or inside a private network, the provider
cannot reach your machine, so no pipeline run is created. A tunnelling service
such as ngrok or Cloudflare Tunnel gives that one endpoint a temporary public
address.

## What Has To Be Reachable

Only the **git-bot webhook endpoint** needs to be reachable from the internet.

NopsAI itself never requires a public URL. The API, UI, AAA, dispatcher, agent,
and runners all stay private, and no NopsAI feature is designed to depend on the
platform being publicly addressable. Do not tunnel the API or the UI to make a
feature work; if something appears to need that, it is a configuration problem
rather than a networking one.

- GitHub App webhooks reach git-bot at `/webhook` on port `8081`.
- Generic Git webhook sources reach the API at
  `/v1/git/webhooks/{sourceID}`. In a local setup, route these through git-bot
  as well rather than exposing the API directly.

## ngrok With Docker Compose

The tunnel definition is intentionally **not tracked in this repository**,
because it carries a personal ngrok authtoken and a reserved personal domain.
`.gitignore` excludes `ngrok-docker-compose.yaml` so a local copy can never be
committed by accident.

Create the file locally from this template:

```yaml
networks:
  nopsai-net:
    driver: bridge
    name: nopsai-net

services:
  ngrok:
    image: ngrok/ngrok:latest
    restart: unless-stopped
    ports:
      - "4040:4040"
    environment:
      - NGROK_AUTHTOKEN=${NGROK_AUTHTOKEN:?set NGROK_AUTHTOKEN in your shell}
    command: http nopsai-git-bot:8081 --domain=<your-reserved-domain>.ngrok-free.app
    networks:
      - nopsai-net
```

Export the authtoken from your shell or secret manager instead of writing it
into the file, so the token never lands on disk in the repository:

```bash
export NGROK_AUTHTOKEN=...
docker compose -f docker-compose.yaml -f ngrok-docker-compose.yaml up -d
```

The tunnel joins the shared `nopsai-net` network and forwards to the
`nopsai-git-bot` service by container name. The ngrok inspection UI is available
at `http://localhost:4040` and shows every delivered webhook, which is the
fastest way to confirm whether a missing run is a delivery problem or a trigger
problem.

A reserved ngrok domain is worth the setup: without one the public address
changes on every restart, and the GitHub App webhook URL has to be updated each
time.

## Cloudflare Tunnel

Cloudflare Tunnel is the better choice for a shared or long-lived development
environment, because the hostname is stable and tied to a domain you control:

```bash
cloudflared tunnel --url http://localhost:8081
```

For a named tunnel, point the ingress rule at `http://nopsai-git-bot:8081` and
use the resulting hostname as the webhook address.

## Pointing NopsAI At The Tunnel

Set the public address in `setting/git-apps/github.yaml` in your global config
repository. `webhook_url` is where the provider delivers events; when it is
empty, NopsAI falls back to `public_url` with `/webhook` appended. See
[git-apps.md](./git-apps.md) for the full schema and
[git-webhook-sources.md](./git-webhook-sources.md) for non-GitHub providers.

## Security Notes

A tunnel publishes an endpoint on your machine to the whole internet, so treat
it as a development-only tool.

- Never use a tunnel for a production or customer installation. Terminate
  webhooks on a real ingress with TLS you control.
- Keep webhook secret verification enabled. The tunnel address is guessable and
  signature checking is what actually authenticates a delivery.
- Stop the tunnel when you are not using it, and rotate the authtoken if it was
  ever written into a file that left your machine.
- An ngrok authtoken is a credential for your ngrok account. If one is
  committed, revoke it in the ngrok dashboard; removing the file does not
  invalidate the token.

## Troubleshooting

- **No delivery at all**: check the provider's webhook delivery log first, then
  the ngrok inspector at `http://localhost:4040`.
- **Delivered but no run**: the tunnel is fine. Check git-bot logs for
  repository access or trigger mismatches, as described in
  [triggering.md](./triggering.md).
- **401 or signature failure**: the webhook secret in the provider does not
  match the configured credential reference.
