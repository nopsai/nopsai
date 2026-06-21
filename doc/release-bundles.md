# Versioned Platform Bundles

NopsAI releases are one platform bundle. The CLI remains independently
installable, but the API, AAA, dispatcher, git-bot, UI, agent, and runner
artifacts deployed to an environment must come from one release manifest.
Production deployment must not combine floating tags or artifacts from
different product versions.

## Compatibility Baseline

The repository baseline is declared in `release/compatibility.yaml`. It owns
the product version, CLI and platform support ranges, API version, runner
protocol version, and required capability IDs. Breaking API, CLI, runner
protocol, or deployment changes require a new major version.

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

## Plan And Deploy

```bash
nopsai platform plan kubernetes \
  --version 2.7.0 \
  --manifest ./release-manifest.json \
  --values deploy/production.yaml

nopsai platform deploy kubernetes \
  --version 2.7.0 \
  --manifest ./release-manifest.json \
  --manifest-digest sha256:<digest> \
  --values deploy/production.yaml \
  --wait
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

## Release Boundary

This first slice establishes build identity, compatibility validation, the
manifest contract, and deterministic Helm plan/deploy behavior. Public OCI
chart publication, multi-architecture artifact publishing, SBOMs, provenance,
OIDC signing, release-candidate promotion, package-manager distribution,
upgrade/status/rollback commands, and Kind smoke deployment belong to the
dedicated release workflow slice. Until that workflow exists, operators must
supply a manifest whose chart and artifacts already exist in their trusted
registry.
