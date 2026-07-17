# NopsAI Release Signing Policy

## Decision

NopsAI will use **keyless Cosign signing backed by GitHub Actions OpenID Connect (OIDC)** for enterprise release artifacts and container images.

This is the default because it avoids maintaining a long-lived private signing key. The release workflow receives a short-lived identity token from GitHub, Cosign records the workflow identity in the signature/certificate, and customers can verify that the artifact was produced by the expected repository and release workflow.

## What signing proves

A valid signature and verification policy can provide evidence that:

- the signed digest was approved by the configured release workflow;
- the artifact has not changed since it was signed;
- the signing identity was tied to the expected GitHub repository/workflow context; and
- release evidence can be independently verified by a customer.

Signing does not prove that the software is vulnerability-free, legally compliant, or correctly configured. It must be combined with tests, review, SBOMs, licence evidence, and risk acceptance.

## Required workflow controls

1. Grant `id-token: write` only to the release-signing job.
2. Keep `contents: read` and other permissions at the minimum required level.
3. Trigger production signing only from protected version tags or a protected GitHub Environment.
4. Require manual approval for the production release environment until a second release approver exists.
5. Pin Cosign and all release actions by immutable version/commit and verify downloaded tooling checksums.
6. Sign immutable digests, never mutable tags alone.
7. Sign:
   - every NopsAI container image digest;
   - the release index/manifest;
   - CLI and other downloadable archives or checksums;
   - Helm chart package or checksum; and
   - SBOM and provenance evidence, directly or through a signed release manifest that binds their hashes.
8. Archive signatures, certificates/bundles, SBOMs, provenance, checksums, and verification instructions with each release.
9. Verify signatures in CI before publishing the final release entry.
10. Do not expose OIDC tokens, registry credentials, or signing outputs containing sensitive claims in logs.

## Verification policy

Customer verification instructions must identify:

- expected repository: `hosein-yousefii/pre-nopsai` until the production repository changes;
- expected release workflow file;
- expected protected tag pattern;
- expected OIDC issuer;
- image digest or artifact SHA-256;
- exact Cosign verification command or policy file; and
- location of the signature bundle and release evidence.

Verification must fail closed when the repository/workflow identity, digest, or certificate transparency evidence does not match.

## Repository transfer or rename

Repository ownership, name, workflow path, and OIDC subject claims may change when NopsAI moves to a company organisation. Before that change:

1. document the old and new identities;
2. update verification policies and customer instructions;
3. produce a transition release signed under the old identity where practicable;
4. preserve old verification evidence; and
5. obtain explicit release approval.

## Recovery and fallback

Keyless signing depends on GitHub Actions, the identity service, registry, and transparency services. A temporary outage must delay a production enterprise release rather than cause an unsigned release to be labelled equivalent.

A future KMS/HSM-backed signing key may be added for regulated or offline customers, but it requires a separate key-management policy, access review, rotation, recovery, and revocation process.

## Implementation status

Policy selected. Workflow implementation, signing verification tests, and customer verification documentation remain required before the control is considered complete.
