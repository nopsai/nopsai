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
return non-secret identity metadata only.

## Published Assets

The GitOps-managed `platform/prod/nopsai-platform-release` pipeline publishes:

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

Kubernetes generation writes editable Helm values and `.nopsai/install.lock`.
The values reference the versioned OCI chart and image tags, plus the Secret
named by `secrets.existingSecret`. Create that Secret with External Secrets,
SOPS, Sealed Secrets, or another cluster secret manager before deploying.

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

That command validates manifest compatibility, verifies the OCI chart package
digest, renders digest-pinned Helm values, and writes `.nopsai/release.lock`
after a successful deploy.
