# Licensing and Distribution

## The licence model

NopsAI is owned by **Hossein Yousefi** and published under the **PolyForm
Noncommercial License 1.0.0**, shipped as the root [`../LICENSE`](../LICENSE)
file. The model has exactly two states:

- **Non-commercial use is free.** Personal use, study, research,
  experimentation, hobby projects, and use by charitable organizations,
  educational institutions, public research organizations, public safety or
  health organizations, environmental protection organizations and government
  institutions are all permitted purposes. No key, no registration, no contact
  with anyone, and no ceiling on users, teams, runs or environments.
- **Commercial use requires a written agreement.** Running NopsAI in or for a
  business, or for any other commercial purpose, is not granted by the public
  licence. That agreement is
  [../legal/commercial-software-licence-agreement.md](../legal/commercial-software-licence-agreement.md),
  with the per-deal Order Form as Schedule 1 and a support-material data
  processing agreement as Schedule 2. It starts with an email to
  contact@nopsai.com.

The public licence also permits modification and redistribution for
noncommercial purposes, provided the licence and the `Required Notice:` line
travel with every copy. That obligation is why the notice files below have to
survive into every artifact.

## Distribution artifacts

Every independently distributed NopsAI artifact must carry the licence and the
third-party notice information:

- **Container images:** include `LICENSE` and `THIRD_PARTY_NOTICES.md` under
  `/usr/share/licenses/nopsai/` and publish the OCI licence identifier
  `PolyForm-Noncommercial-1.0.0`.
- **CLI:** provide the licence notice through `nopsai license`. Release archives
  should also carry the root `LICENSE` and `THIRD_PARTY_NOTICES.md`.
- **Helm chart:** package `LICENSE` and `THIRD_PARTY_NOTICES.md` in the chart
  root and declare the licence identifier in `Chart.yaml` annotations.
- **Deployment bundles and copied files:** include the same notice files. Add
  the SPDX identifier below to independently copied scripts, templates, and
  examples where the file format permits comments.

```text
SPDX-License-Identifier: PolyForm-Noncommercial-1.0.0
Copyright (c) 2026 Hossein Yousefi
```

## Third-party components

Third-party components retain their own licence terms. `THIRD_PARTY_NOTICES.md`
is the notice index; `scripts/generate-notices.sh` produces the release-specific
bundle carrying the actual licence texts, and the release pipeline cannot build
an artifact family without it.

Do not label third-party source, packages, images, fonts, icons, or generated
clients as owned by NopsAI. Preserve all required notices and source-offer
obligations.

## Release review

Before publishing a Release, confirm that:

1. every artifact contains or exposes the licence notice;
2. the release-specific third-party notice bundle is complete;
3. container SBOMs and provenance correspond to the released digests;
4. the Helm chart and CLI refer to the same versioned images; and
5. no customer data, credentials, confidential material, or unapproved
   third-party content is present.

Note what is *not* on that list: checking that a recipient has an agreement.
Artifacts are published for anyone to pull, because the licence grants
non-commercial use to anyone. The commercial boundary is a licence question
settled between the operator and the licensor, not a gate on distribution — see
[licensing-entitlements.md](./licensing-entitlements.md) for why the software
does not try to decide it at runtime.
