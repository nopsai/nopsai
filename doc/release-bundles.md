# Versioned Platform Bundles

NopsAI releases are one platform bundle. The CLI remains independently
installable, but the API, AAA, dispatcher, git-bot, UI, agent, and runner
artifacts deployed to an environment must come from one release manifest.
Production deployment must not combine floating tags or artifacts from
different product versions.

## Commit-Count Versions

`release/version.txt` owns the `major.minor` release series. The patch number is
the repository commit count for the released main-branch commit. For example,
commit count `537` in series `2.10` produces `2.10.537`, image tag `2.10.537`,
and GitHub tag `v2.10.537`.

Pull requests calculate a forecast with `HEAD commit count + 2`, reserving one
final PR commit and one merge commit. The forecast is used only for CI image
metadata and the downloadable release preview. Main is authoritative;
if other changes merge first, its actual commit count wins and no forecast tag
is published.

## Compatibility Baseline

The repository baseline is declared in `release/compatibility.yaml`. It owns
the compatibility floor, CLI and platform support ranges, API version, runner
protocol version, and required capability IDs. `release/version.txt` owns the
active major/minor series. Breaking API, CLI, runner protocol, or deployment
changes require a new major version.

Every Go binary supports `--version` and receives the following values through
linker flags:

- product version and source commit
- UTC build date
- release manifest digest
- API and runner protocol versions
- CLI, runner, and platform compatibility ranges
- capability IDs

Development builds use explicit `dev` metadata. Container images also expose
OCI version, revision, and creation labels.

## Version Endpoint

`GET /version` is public in both setup-preflight and normal routing modes. It
returns non-sensitive product, API, compatibility, capability, and manifest
digest metadata. It does not return environment configuration or credentials.

```json
{
  "productVersion": "2.7.0",
  "apiVersion": "v1",
  "cliCompatibility": ">=2.0.0 <3.0.0",
  "runnerCompatibility": ">=2.0.0 <3.0.0",
  "capabilities": ["platform.helm", "runner.kubernetes", "config-sync.v1"],
  "releaseManifestDigest": "sha256:..."
}
```

Prometheus exposes the same immutable identity through
`nopsai_build_info{version,commit,api_version,release_manifest_digest} 1`.
Hosted MCP exposes `nopsai.get_platform_version`; it requires an authenticated
caller and returns the same public metadata.

Released CLIs query `/version` before `POST`, `PUT`, `PATCH`, and `DELETE`
requests. An incompatible API version or reciprocal product range stops the
mutation before it is sent. Local `dev` builds skip this live compatibility
gate so contributors can work before release metadata is injected.

## Release Manifest

`release/manifest.tmpl.json` defines the signed release input format. A valid
manifest contains:

- an exact semantic product version
- an OCI Helm chart reference and `sha256` chart package digest
- digest-pinned images for API, AAA, dispatcher, git-bot, UI, agent, Docker
  runner, and Kubernetes runner
- CLI/API/runner compatibility metadata and capabilities
- a database migration version and explicit rollback policy

The parser rejects unknown fields, floating image tags, absent services,
invalid compatibility ranges, mismatched chart versions, and undeclared
migration policy. The optional manifest digest supplied to the CLI is checked
against the resolved bytes before parsing.

The container workflow also creates `release-index.json`. This is the image,
CLI, and Helm artifact lock for one commit-count release. It records the chart
package checksum plus the OCI chart reference, version, and registry digest.

## Automated Publication

`doc/sample-config-repo/global-repo/triggers/hosein-yousefii/pre-nopsai.yaml`
starts the GitOps-managed `platform/prod/nopsai-platform-release` NopsAI
pipeline on GitHub App `push` events to `main`. The pipeline validates the
release package contracts before publication, calculates the commit-count
version from `release/version.txt`, and skips an existing version tag unless
`NOPSAI_RELEASE_ALLOW_EXISTING=true` enables recovery mode.

Each successful release publishes:

- multi-architecture `linux/amd64` and `linux/arm64` images under
  `ghcr.io/<owner>` for the base, API, AAA, agent, dispatcher, git-bot, Docker
  runner, Kubernetes runner, socket proxy, UI, and pipeline helper
- standalone `nopsai-cli_<version>_<os>_<arch>` archives for Linux, macOS, and
  Windows
- SBOM and provenance output for published OCI images
- a digest-pinned release manifest, image index, `SHA256SUMS`,
  `nopsai-changelog-<version>.md`, `nopsai-docker-compose-<version>.yaml`, and
  `nopsai-helm-chart-<version>.tgz` Helm chart asset
- the same Helm chart published to `oci://ghcr.io/<owner>/charts/nopsai`
- one deployment bundle whose `.env`, Compose file, Helm package, and image
  index all identify the same version and source commit
- standalone CLI archives that embed install templates, the exact
  `release-manifest.json`, and the manifest digest for their own version

The release pipeline renders the digest-pinned manifest before compiling the
standalone CLI archives. The CLI build links the manifest bytes into the binary
and injects the same digest into build metadata. Default `nopsai install` and
`nopsai platform release` flows therefore generate Compose, env, values,
manifest, and lock files without downloading the manifest from GitHub. Explicit
`--manifest` values and `NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE` remain available
for controlled internal registries and recovery workflows.

The current publication pipeline cross-compiles standalone Darwin binaries as
release assets; macOS signing and notarization are available through
`scripts/sign-notarize-macos-cli.sh` but are not wired into the default package
publication path.

GitHub release asset names are optimized for operators scanning the release
page. `release-manifest.json`, `release-index.json`, and `SHA256SUMS` keep
stable conventional names for CLI and automation compatibility. The Helm chart
is uploaded as `nopsai-helm-chart-<version>.tgz`; inside the deployment bundle
and in the OCI registry it still uses Helm's standard chart package name,
`nopsai-<version>.tgz`.

If an earlier run published some GHCR packages but failed before the GitHub
Release asset upload, rerun `platform/prod/nopsai-platform-release` for the same
`NOPSAI_RELEASE_SOURCE_REF` with `NOPSAI_RELEASE_ALLOW_EXISTING=true`. The
pipeline verifies that the existing version tag points to the same source
commit, republishes the OCI images and Helm chart, and replaces GitHub Release
assets with `--clobber`.

`scripts/generate-changelog.sh` teams commits since the most recent semantic
version tag into breaking, added, fixed, and changed sections. The generated
file is used as the GitHub Release body and shipped as an asset; release
automation never commits generated version or changelog files back to main, so
the act of releasing cannot change its own version.

The GHCR packages must be readable by the deployment environment. Public
installations can expose the packages publicly; private installations should
authenticate Docker, containerd, and Kubernetes with a package read token.

## Plan And Deploy

```bash
# First-time wizard; choose Docker Compose or Kubernetes
nopsai install

# Automation shortcut for Docker Compose
nopsai install docker-compose --run

nopsai install kubernetes \
  --output-dir ./nopsai-prod \
  --values-file values.yaml

# After editing values and creating the referenced Secret
cd ./nopsai-prod
nopsai install kubernetes --output-dir . --values-file values.yaml --deploy --wait

# Advanced CI/GitOps primitive for already prepared manifests and values
nopsai platform release kubernetes \
  --version 2.7.0 \
  --manifest ./release-manifest.json \
  --values deploy/production.yaml

nopsai platform release kubernetes \
  --version 2.7.0 \
  --manifest ./release-manifest.json \
  --manifest-digest sha256:<digest> \
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
externally managed.

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
