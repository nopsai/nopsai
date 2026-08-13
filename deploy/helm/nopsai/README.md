# NopsAI Helm Chart

This chart deploys PostgreSQL, the NopsAI API, AAA, dispatcher, git-bot, UI,
Gotenberg, and Kubernetes runner. The release workflow replaces every NopsAI
image with the digest produced for the same source commit before packaging the
chart.

Deployment secrets are intentionally external. Before install, create the Secret
named by `secrets.existingSecret` with these keys:

- `database-url`
- `postgres-password`
- `master-key`
- `jwt-signing-key`
- `service-jwt-signing-key`
- `aaa-shared-internal-token`
- `dispatcher-tls-secret`
- `bootstrap-admin-password`

The key names can be changed under `secrets.keys`. Use External Secrets,
Sealed Secrets, SOPS, or the cluster's secret manager rather than committing
secret values to a Helm values file.

The bundled PostgreSQL StatefulSet is enabled by default and stores data in a
PVC. The `database-url` key should point at the internal `postgres` Service, for
example `postgres://nopsai_user:<postgres-password>@postgres:5432/nopsai_db?sslmode=disable`.
For managed PostgreSQL, set `postgres.enabled=false` and point `database-url` at
the external database instead.

For a new installation, let the CLI generate non-secret values, an applyable
Secret manifest, and the installation guide:

```bash
nopsai install kubernetes \
  --version <version> \
  --output-dir ./nopsai-prod \
  --values-file values.yaml \
  --secret-file nopsai-secrets.yaml
```

Review `./nopsai-prod/installation.md`, keep `values.yaml` and lock metadata
GitOps-tracked, and keep `nopsai-secrets.yaml` private or encrypt/seal it before
GitOps. Apply the generated Secret before Helm renders workloads:

```bash
cd ./nopsai-prod
kubectl create namespace nopsai --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f nopsai-secrets.yaml
helm upgrade --install nopsai ./nopsai-<version>.tgz \
  --namespace nopsai \
  --create-namespace \
  --set secrets.existingSecret=nopsai-secrets
```

The chart version is the only version to change. Every NopsAI image tag
resolves from `global.releaseVersion`, and when that is empty it falls back to
the chart `appVersion`, which release packaging stamps with the release version.
Deploying `--version <release>` therefore moves every NopsAI image without any
values edit. Set `global.releaseVersion` only to run images from a version other
than the chart's own; set `api.image.tag`, `agent.image.tag`, or another
per-image tag only for an intentional component-specific override, and `digest`
to pin one image by digest. Third-party images such as PostgreSQL and Gotenberg
keep explicit tags in values.

Private GHCR installations should create a registry pull Secret in the target
namespace and attach it through `global.imagePullSecrets`. The Kubernetes
runner ServiceAccount inherits those credentials so the runner Deployment and
dynamically created agent pods can pull the same release images. Workload image
pull secrets are also passed to step pods.

```bash
kubectl -n nopsai create secret docker-registry nopsai-registry \
  --docker-server=ghcr.io \
  --docker-username=<registry-user> \
  --docker-password=<registry-token>
```

```yaml
global:
  imagePullSecrets:
    - name: nopsai-registry
```

`api.metricsRequireAuth` defaults to `true` because production startup gates
require authenticated `/metrics`. Keep it enabled unless the chart is used only
for isolated local evaluation.

When `api.serviceAccount.create=true` and
`k8sRunner.serviceAccount.create=true`, the chart writes the same
`global.imagePullSecrets` list onto the `nopsai-api` and `nopsai-runner`
ServiceAccounts and onto workload pod specs. This lets the API, runner, and
runner-created agent/step pods pull private images without putting registry
credentials in values.

If you bring your own ServiceAccounts by setting either `*.serviceAccount.create`
to `false`, create or patch those ServiceAccounts yourself before deploy:

```bash
kubectl -n nopsai patch serviceaccount nopsai-api \
  -p '{"imagePullSecrets":[{"name":"nopsai-registry"}]}'
kubectl -n nopsai patch serviceaccount nopsai-runner \
  -p '{"imagePullSecrets":[{"name":"nopsai-registry"}]}'
```

These chart-level pull secrets are infrastructure-provided release credentials.
They remain separate from runner registry credentials managed inside NopsAI.
For additional runners created from **System > Dispatcher > Runner Installs**,
administrators can select active `docker_config_json` credentials. Kubernetes
bootstrap commands turn those selected configs into a temporary
`kubernetes.io/dockerconfigjson` Secret for the runner and workload
ServiceAccounts. Agent pods use the RBAC-bearing runner ServiceAccount because
they create step pods and PVCs; step pods use
`k8sRunner.workload.serviceAccount` without runner RBAC. Docker bootstrap
commands use a temporary Docker CLI config for
the initial runner image pull, then pass the selected config to the Docker runner as
`NOPSAI_REGISTRY_DOCKER_CONFIG_B64`. Docker runner and agent image pulls match
registry hosts locally and pass per-image `RegistryAuth` to the Docker Engine
API without calling NopsAI for every pull. Do not put registry passwords in Helm
values.

`topology.dispatcherGRPCAddress` controls the internal dispatcher gRPC endpoint
in the API and Kubernetes runner Deployments. It defaults to `dispatcher:9090`
for chart-managed pods in the same namespace and can be overridden when the
dispatcher Service name, namespace, or port is customized. Cross-namespace
runners should use a fully-qualified Service DNS name such as
`dispatcher.<platform-namespace>.svc.cluster.local:9090`.
`topology.nopsaiAPIURL`, `topology.aaaAPIURL`,
`topology.gitBotAPIURL`, and `topology.gotenbergURL` expose the matching
service URLs for split-service or custom-DNS deployments.

`bootstrapAdmin.email` sets the initial local administrator email. The password
is read from the `secrets.keys.bootstrapAdminPassword` key in the existing
Secret so GitOps values do not contain plaintext credentials.

System Logs defaults to the `kubernetes,docker` provider list in this chart. The API
Deployment runs as the `api.serviceAccount.name` service account and receives
read-only namespace Role permissions for `pods` and `pods/log` when
`systemLogs.enabled=true`, the provider list includes `kubernetes`, and
`systemLogs.kubernetes.rbac.create=true`. Set
`systemLogs.kubernetes.labelSelector` to override the default release-scoped
selector. The API and bundled k8s runner share `NOPSAI_PLATFORM_ID` from
`global.platformID`, and runner installs generated from **System > Dispatcher**
are labeled with `nopsai.io/runner-id` and `nopsai.io/platform-id`; when the API
service account can list pods and read logs in that namespace, the UI exposes
owned pods as `runner:<runner-id>` system log sources from the runner detail
view. Set `global.platformID` only when an external GitOps convention needs a
stable value different from the Helm release name. Hybrid deployments can keep
the default `systemLogs.provider` value when both Kubernetes RBAC and a Docker
API endpoint are intentionally available to the API. Set
`systemLogs.dockerHost` to the Docker API or socket-proxy endpoint when the
provider list includes Docker.

When installing a Docker runner against a Helm-installed control plane reached
through local port-forwarding, forward the dispatcher Service as well as the UI:

```bash
kubectl -n nopsai port-forward service/dispatcher 9090:9090
```

Docker runner install commands generated from a local UI URL rewrite the
dispatcher endpoint to `host.docker.internal:9090` and add a Docker
`host-gateway` mapping. For production runners, prefer private DNS, a VPN, or a
load-balanced gRPC endpoint over a long-lived local port-forward.
