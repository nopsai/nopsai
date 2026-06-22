# NopsAI Helm Chart

This chart deploys the NopsAI API, AAA, dispatcher, git-bot, UI, Gotenberg,
and Kubernetes runner. The release workflow replaces every NopsAI image with
the digest produced for the same source commit before packaging the chart.

PostgreSQL and deployment secrets are intentionally external. Before install,
create the Secret named by `secrets.existingSecret` with these keys:

- `database-url`
- `master-key`
- `jwt-signing-key`
- `service-jwt-signing-key`
- `aaa-shared-internal-token`

The key names can be changed under `secrets.keys`. Use External Secrets,
Sealed Secrets, SOPS, or the cluster's secret manager rather than committing
secret values to a Helm values file.

```bash
helm upgrade --install nopsai ./nopsai-<version>.tgz \
  --namespace nopsai \
  --create-namespace \
  --set secrets.existingSecret=nopsai-secrets
```

Private GHCR installations should attach a registry credential through
`global.imagePullSecrets`. The Kubernetes runner ServiceAccount inherits those
credentials so dynamically created agent and step pods can pull the same
release images.
