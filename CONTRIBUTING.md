# Contributing to NopsAI

NopsAI is currently a proprietary product. Contributions are accepted only when the repository owner has expressly authorised the contributor and the contribution terms below are satisfied.

## Before contributing

Do not submit code, documentation, designs, prompts, examples, datasets, images, fonts, or other material unless:

- you created it or have documented permission to contribute it;
- it does not contain confidential information, personal data, credentials, customer data, or former-employer/client material;
- all third-party and AI-assisted material is disclosed;
- its licence is compatible with NopsAI's commercial distribution policy; and
- any required employment, contractor, contributor, confidentiality, and IP agreement has been signed.

External pull requests may be closed without review when these prerequisites have not been arranged.

## Developer Certificate of Origin sign-off

Every commit must include a `Signed-off-by` trailer:

```text
Signed-off-by: Full Legal Name <email@example.com>
```

Add it with:

```bash
git commit -s
```

By signing off, the contributor certifies that:

1. the contribution was created by the contributor, in whole or in part, and the contributor has the right to submit it under the repository's contribution terms; or
2. the contribution is based on prior work that the contributor reasonably believes is appropriately licensed and the contributor has the right to submit the modifications; or
3. the contribution was provided by another person who certified one of the preceding statements and the contributor has not knowingly modified the provenance information; and
4. the contribution and sign-off will be maintained as part of the project record and may be redistributed with the contribution.

A DCO sign-off records provenance. It does not replace an employment, contractor, founder, or contributor IP agreement when one is required.

## Third-party material

A contribution that adds or changes a dependency, container image, copied snippet, generated client, font, icon, image, template, dataset, model, prompt pack, or other third-party material must include:

- source and supplier;
- exact version or immutable digest where available;
- licence and copyright information;
- intended runtime, build, test, or documentation scope;
- required notices or source-offer obligations;
- security and maintenance owner; and
- approval in the pull request.

Do not add copyleft, source-available, non-commercial, field-of-use restricted, or unknown-licence material without written approval from the repository owner and legal review.

## AI-assisted contributions

AI tools may be used only as an assistant. The human contributor remains responsible for originality, correctness, security, licence compatibility, and disclosure.

The pull request must identify material AI assistance where generated output is more than trivial completion. Do not submit output copied from a model when its provenance or permitted use cannot be reasonably established.

Never submit customer code, secrets, non-public vulnerability information, or other restricted material to an unapproved model.

## Security

Do not open a public issue for a suspected vulnerability or exposed credential. Use the private security reporting process identified in `SECURITY.md` when available, or contact the repository owner directly through an agreed private channel.

## Pull request requirements

A pull request should:

- explain the problem, change, and user impact;
- contain focused commits with sign-offs;
- include tests for behaviour changes;
- update documentation and release notes where needed;
- identify migrations and rollback steps;
- pass licence, security, test, and release-integrity gates; and
- disclose known residual risks.

## Records

Signed employment, contractor, founder, contributor, or assignment agreements must be stored in a restricted legal-records system. Do not commit signed legal documents, identity documents, addresses, or private signatures to this repository.
