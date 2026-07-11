# Versioned Platform Bundles

NopsAI releases are one platform bundle. The CLI remains independently
installable, but the API, AAA, dispatcher, git-bot, UI, agent, and runner
artifacts deployed to an environment must come from one release manifest.
Production deployment must not combine floating tags or artifacts from
different product versions.

## Commit-Count Versions

`release/version.txt` owns the `major.minor` release series. The patch number is
the repository commit count for the released main-branch commit. For example,
commit count `537` in series `2.7` produces `2.7.537`, image tag `2.7.537`, and
GitHub tag `v2.7.537`.

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
against the downloaded bytes before parsing.

The container workflow also creates `release-index.json`. This is the image,
CLI, and Helm artifact lock for one commit-count release. It records the chart
package checksum plus the OCI chart reference, version, and registry digest.

## Automated Publication

`.github/workflows/platform-release.yml` runs only after a successful
push-triggered `Enterprise Gates` run on the current main commit, or through an
explicit manual retry of the current main commit. It does not
consume PR gate runs, caches, or artifacts. A superseded gate run and an
existing version tag are skipped.

Each successful release publishes:

- multi-architecture `linux/amd64` and `linux/arm64` images under
  `ghcr.io/<owner>` for the base, API, AAA, agent, dispatcher, git-bot, Docker
  runner, Kubernetes runner, socket proxy, UI, and pipeline helper
- standalone CLI archives for Linux, macOS, and Windows
- SBOM and provenance output for published OCI images
- a digest-pinned image index, `SHA256SUMS`, generated changelog, deployment
  Compose file, and `nopsai-<version>.tgz` Helm package
- the same Helm chart published to `oci://ghcr.io/<owner>/charts/nopsai`
- one deployment bundle whose `.env`, Compose file, Helm package, and image
  index all identify the same version and source commit

Darwin CLI matrix entries run on `macos-14`. The release imports an Apple
Developer ID Application certificate into an ephemeral keychain, applies a
hardened-runtime timestamped signature, notarizes the ZIP with an App Store
Connect API key, and runs a Gatekeeper assessment before uploading the asset.
The keychain and decoded credentials are deleted before the job ends.

Configure these encrypted GitHub Actions secrets before enabling main-branch
publication:

- `APPLE_DEVELOPER_ID_P12_BASE64`: base64-encoded Developer ID Application P12
- `APPLE_DEVELOPER_ID_P12_PASSWORD`: password protecting the P12
- `APPLE_DEVELOPER_ID_IDENTITY`: full signing identity, including Team ID
- `APPLE_NOTARY_KEY_P8_BASE64`: base64-encoded App Store Connect API private key
- `APPLE_NOTARY_KEY_ID`: App Store Connect API key ID
- `APPLE_NOTARY_ISSUER_ID`: App Store Connect API issuer ID

Missing or rejected Apple credentials fail the Darwin matrix jobs and prevent
the unified release from being published. They never fall back to an unsigned
CLI artifact.

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

Without `--manifest`, the CLI resolves the manifest from the configured release
URL template. `NOPSAI_RELEASE_MANIFEST_URL_TEMPLATE` can point GitOps automation
at an internal HTTPS artifact repository.

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

The generated deployment bundle has a deployment-only `docker-compose.yaml`
and `.env` with digest-pinned NopsAI image references. Operators add production
secrets through their secret manager, then run `docker compose config` and
`docker compose up -d`. `nopsai-<version>.tgz` is a deployable chart containing
the same digest-pinned images. Kubernetes installations must create the Secret
named by `secrets.existingSecret` before installing the chart; PostgreSQL stays
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
