# Versioned Release Artifacts

NopsAI releases are versioned by the commit-count series in
`release/version.txt`. A release publishes container images, a Helm chart, CLI
archives, a changelog, and checksums. It does not publish a release index,
release manifest, Docker Compose asset, or deployment bundle in the default
pipeline.

## Version Identity

`release/version.txt` owns the `major.minor` series. The patch number is the
repository commit count for the released main-branch commit, so a `2.10` series
with commit count `648` becomes `2.10.648`. That same semantic version is used
for:

- Git tag `v<version>`
- NopsAI container image tags
- Helm chart `version` and `appVersion`
- standalone CLI archive names
- generated Docker Compose and Helm values created by the CLI

Go binaries still expose shared build identity through `pkg/buildinfo`: product
version, commit, build date, API version, runner protocol, compatibility ranges,
and capabilities. The public `/version` endpoint and hosted MCP version tool
return non-secret identity metadata only. The authenticated UI reads that same
endpoint and renders a quiet version-only footer at the far right of the app
chrome so operators can confirm the deployed build without opening diagnostics.

## Published Assets

The GitOps-managed `.nopsai/nopsai-platform-release.yaml` pipeline publishes
`platform/prod/nopsai-platform-release` assets:

- multi-architecture GHCR images for the base, API, AAA, agent, dispatcher,
  git-bot, Docker runner, Kubernetes runner, socket proxy, UI, and pipeline
  helper
- the NopsAI Helm chart to `oci://ghcr.io/<owner>/charts/nopsai`
- GitHub Release asset `nopsai-helm-chart-<version>.tgz`
- standalone `nopsai-cli_<version>_<os>_<arch>` archives for Linux, macOS, and
  Windows
- `nopsai-changelog-<version>.md`
- `SHA256SUMS` for the uploaded GitHub Release assets

The release pipeline intentionally does not upload `release-index.json`,
`release-manifest.json`, `nopsai-docker-compose-<version>.yaml`, or
`nopsai-deployment-bundle-<version>.tar.gz`. Operators generate deployment files
from the CLI for the exact version they want to install.

## CLI-Generated Installs

The first-install entry point is:

```bash
nopsai install
```

The wizard lets the operator choose Docker Compose or Kubernetes, enter required
runtime choices, and edit internal service topology. Noninteractive automation
can call the exact target directly:

```bash
nopsai install docker-compose \
  --version 2.10.648 \
  --output-dir ./nopsai-install \
  --nopsai-api-url http://nopsai:8080 \
  --dispatcher-address dispatcher:9090 \
  --run

nopsai install kubernetes \
  --version 2.10.648 \
  --output-dir ./nopsai-prod \
  --values-file values.yaml \
  --existing-secret nopsai-secrets
```

Docker Compose generation writes `docker-compose.yaml`, `.env`, `db/init.sql`,
and `.nopsai/install.lock`. The `.env` file contains generated local secrets and
must stay out of Git. The install lock is non-secret and records the generated
version, image references, and file hashes.

Kubernetes generation writes editable Helm values, `README.md`, and
`.nopsai/install.lock`. The values reference the versioned OCI chart and image
tags, plus the Secret named by `secrets.existingSecret`. The generated README
records prerequisites, the expected Secret keys, `kubectl` Secret creation
examples, CLI deploy commands, direct Helm commands, and basic verification
commands. Create the Secret with External Secrets, SOPS, Sealed Secrets, or
another cluster secret manager before deploying.

After editing values, deploy from the generated directory:

```bash
cd ./nopsai-prod
nopsai install kubernetes --output-dir . --values-file values.yaml --deploy --wait
```

Stored-file deploys read `global.releaseVersion` from `values.yaml`, run
`helm upgrade --install` against the versioned OCI chart, and write a
GitOps-readable release lock after success.

## Advanced Manifest Deploys

`nopsai platform release kubernetes` remains available for advanced internal
workflows that already produce digest-pinned release manifests. It is not the
default publication path. Use it only when CI or an internal registry supplies
an explicit manifest:

```bash
nopsai platform release kubernetes \
  --version 2.10.648 \
  --manifest ./release-manifest.json \
  --values deploy/production.yaml \
  --deploy --wait
```

Without `--manifest`, standalone released CLIs use their embedded
`release-manifest.json` for the same release version. Set
`NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE` only when GitOps automation should
intentionally resolve manifests from a trusted internal HTTPS artifact
repository.

`nopsai install` is the shortest first-install path. It opens a wizard, lets the
operator choose Docker Compose or Kubernetes, resolves the same manifest from
the CLI release version, and generates the required files itself.
`nopsai install docker-compose` is the automation shortcut: it writes
`docker-compose.yaml`, `.env`, `db/init.sql`, `release-manifest.json`, and
`.nopsai/install.lock`, then starts the stack when `--run` is set. The `.env`
file is generated with local secrets and must stay out of Git; the install lock
is non-secret and can be kept with environment state.

`nopsai install kubernetes` generates editable Helm values for the selected
version and references a Secret through `secrets.existingSecret`. Operators can
edit those values, create the Secret with their cluster secret manager, then run
the printed `nopsai install kubernetes ... --deploy` command from the generated
directory. The installer reuses stored `release-manifest.json` and `values.yaml`
without overwriting edits. `nopsai platform release` remains available for CI
and advanced GitOps workflows that want direct render/deploy control.

Planning verifies compatibility, downloads the exact OCI chart with Helm,
verifies the chart package digest, injects the release version, manifest
digest, and every image repository/digest into generated Helm values, and runs
`helm template`. Deployment uses the same verified inputs with
`helm upgrade --install`; no floating tag is passed to Helm.

After a successful deployment, the CLI atomically writes
`.nopsai/release.lock`. The GitOps-readable file records the bundle version,
manifest and chart digests, exact images, deterministic values hash, namespace,
release name, and deployment time. It contains no credentials and is not
written after a failed deployment.

The lock is also the deployment transition guard. Before Helm changes the
cluster, the CLI rejects a lock for another release or namespace, any database
migration regression, and a version downgrade unless both the deployed and
target bundles declare `rollback-safe`. Locks written before rollback policy was
recorded are treated as `forward-only`. Preserve the lock in the environment's
GitOps repository so these checks remain effective across operators and CI
runners.

The generated deployment bundle still has a deployment-only `docker-compose.yaml`
and `.env` with digest-pinned NopsAI image references for operators who want the
release archive instead of CLI generation. The GitHub asset
`nopsai-helm-chart-<version>.tgz` is the deployable chart containing the same
digest-pinned images. Kubernetes installations must create the Secret named by
`secrets.existingSecret` before installing the chart; PostgreSQL stays
externally managed. Override `topology.dispatcherGRPCAddress` only when the
dispatcher Service DNS name or port differs from the chart default
`dispatcher:9090`.

```bash
helm upgrade --install nopsai \
  oci://ghcr.io/<owner>/charts/nopsai \
  --version <version> \
  --namespace nopsai \
  --create-namespace \
  --set secrets.existingSecret=nopsai-secrets
```

## Release Boundary

The repository now owns commit-count image, CLI, and Helm publication,
SBOM/provenance generation, deployment image locks, and changelog generation.
Release-manifest signing, release-candidate promotion, package-manager
distribution, upgrade/status/rollback commands, and Kind smoke deployment
remain separate work.
