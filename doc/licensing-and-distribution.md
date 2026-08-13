# Licensing and Distribution

## Ownership and licence model

NopsAI is proprietary software owned by **Hossein Yousefi**. It is not released
under an open-source or source-available public licence. Access to the repository,
a binary, an image, a chart, or any other artifact does not itself grant a right
to use the software.

Customer use must be authorised by a signed NopsAI Commercial Software Licence
Agreement, evaluation agreement, partner agreement, or another written agreement
with the rights holder. Managed deployment or operation in customer-controlled
infrastructure is governed by a separate services schedule or statement of work.

The Commercial Software Licence Agreement is drafted in
[../legal/commercial-software-licence-agreement.md](../legal/commercial-software-licence-agreement.md),
with the per-deal Order Form as Schedule 1 and a support-material data
processing agreement as Schedule 2. It is a draft pending review by a Dutch
technology lawyer and must not be sent to a customer before that review.

The root `LICENSE` file is a proprietary notice. It is not a replacement for the
customer agreement, order form, support terms, data-processing terms, or managed
services schedule.

## Distribution artifacts

Every independently distributed NopsAI artifact must preserve the proprietary
notice and third-party notice information:

- **Container images:** include `LICENSE` and `THIRD_PARTY_NOTICES.md` under
  `/usr/share/licenses/nopsai/` and publish the OCI licence identifier
  `LicenseRef-NopsAI-Proprietary`.
- **CLI:** provide the proprietary notice through `nopsai license`. Release
  archives should also carry the root `LICENSE` and `THIRD_PARTY_NOTICES.md`.
- **Helm chart:** package `LICENSE` and `THIRD_PARTY_NOTICES.md` in the chart root
  and declare the proprietary licence reference in `Chart.yaml` annotations.
- **Deployment bundles and copied files:** include the same notice files. Add the
  SPDX identifier below to independently copied scripts, templates, and examples
  where the file format permits comments.

```text
SPDX-License-Identifier: LicenseRef-NopsAI-Proprietary
Copyright (c) 2026 Hossein Yousefi
```

## Third-party components

Third-party components retain their own licence terms. `THIRD_PARTY_NOTICES.md`
defines the dependency sources and release control, but a complete
release-specific notice bundle still needs to be generated and reviewed before
external distribution.

Do not label third-party source, packages, images, fonts, icons, or generated
clients as owned by NopsAI. Preserve all required notices and source-offer
obligations.

## Release review

Before external distribution, confirm that:

1. the intended customer or evaluator has a written agreement;
2. every artifact contains or exposes the proprietary notice;
3. the release-specific third-party notice bundle is complete;
4. container SBOMs and provenance correspond to the released digests;
5. the Helm chart and CLI refer to the same versioned images; and
6. no customer data, credentials, confidential material, or unapproved
   third-party content is present.
