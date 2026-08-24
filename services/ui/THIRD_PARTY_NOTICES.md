# Third-Party Software Notices

NopsAI includes, links to, packages, or depends on third-party software. Those
components are not licensed under the NopsAI licence and remain subject to their
own copyright notices and licence terms.

## Authoritative dependency sources

The release-specific third-party inventory is derived from:

- `go.mod` and `go.sum` for Go modules;
- `services/ui/package.json` and `services/ui/package-lock.json` for UI packages;
- base images and installed operating-system packages declared by Dockerfiles;
- Helm chart dependencies, when present; and
- SBOMs and provenance attestations generated for released container images.

## Distribution requirement

Before an external release is distributed, the release owner must generate,
review, and retain a complete release-specific notice bundle containing all
required copyright notices, licence texts, attribution, source-offer information,
and other obligations for the components actually shipped.

This repository file is the notice index and distribution control. It is not, by
itself, a complete dependency licence audit or a substitute for the
release-specific notice bundle.

No NopsAI notice removes or limits rights granted by a third-party licence.
