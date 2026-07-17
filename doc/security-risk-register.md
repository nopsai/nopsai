# NopsAI Enterprise Security Risk Register

> Status: active working register. Dates are remediation targets, not certifications. Until additional roles are assigned, the accountable owner is Hossein Yousefi.

## Severity definitions

- **Critical** — credible risk of severe compromise or unlawful disclosure; blocks enterprise release.
- **High** — material confidentiality, integrity, availability, legal, or supply-chain risk; blocks an unqualified enterprise-readiness claim.
- **Medium** — meaningful weakness requiring scheduled remediation or explicit acceptance.
- **Low** — limited impact or defence-in-depth improvement.

## Open risks

| ID | Risk | Severity | Current control | Required remediation | Owner | Target | Release decision |
|---|---|---:|---|---|---|---|---|
| SEC-001 | Pipeline logs, tool outputs, conversation content, and deterministic summaries may be serialised to a customer-selected LLM without applying the existing secret redactor at the final provider boundary. | Critical | Team/scope permissions; system-log UI redaction; prompt truncation. | Apply recursive redaction to every string entering the LLM prompt, including user content, memory, history, evidence, tool input/output, resource URIs, and summaries. Add regression tests for credentials, authorisation headers, tokens, passwords, and database URLs. | Hossein Yousefi | 2026-07-20 | Block until fixed and tested. |
| SEC-002 | Assistant conversation retention is documented/configured but automatic expiry was not found, creating over-retention and policy mismatch. | High | Manual deletion of individual conversations; configured retention value. | Implement scheduled deletion, tenant-safe queries, metrics/audit events, failure alerting, tests, and documented backup behaviour. | Hossein Yousefi | 2026-07-31 | Block managed-service production use of assistant history until fixed or disabled. |
| SEC-003 | No complete customer export route was identified for assistant conversations and related evidence. | Medium | Direct product access and manual deletion. | Define export scope and format; add authorised export endpoint/job; log exports; test tenant isolation; document portability limits. | Hossein Yousefi | 2026-08-14 | Allowed only with documented manual process and customer disclosure. |
| SUP-001 | Release evidence requests OCI SBOM/provenance for NopsAI-built images but does not package a complete, signed SBOM set for binaries, chart, source, operating-system packages, and third-party runtime images. | High | BuildKit SBOM/provenance enabled for NopsAI images; image digests recorded. | Generate CycloneDX/SPDX evidence for all shipped artifacts and material runtime images; archive it with the release; sign release index and artifacts; publish verification instructions. | Hossein Yousefi | 2026-07-31 | Block unqualified supply-chain assurance claims. |
| SUP-002 | PostgreSQL, Gotenberg, Alpine, Nginx, Node, Docker CLI, and other material images/build inputs use mutable tags or are not all represented in the release index. | High | Version tags for several images; NopsAI image digests in release index. | Pin external images and base images by digest; record source/tag/digest/licence; define controlled update process; include external runtime images in release evidence. | Hossein Yousefi | 2026-07-31 | Block reproducibility claim. |
| LIC-001 | Licence allow/review decisions are not fully scoped by runtime/build/test context, and release bundles do not yet include generated third-party notices. | High | Go/npm licence scanner; forbidden/unknown licence failures; engineering policy. | Move policy to machine-readable rules with scoped and expiring exceptions; generate THIRD_PARTY_NOTICES and licence texts; archive scanner evidence; add image/OS-package scanning. | Hossein Yousefi | 2026-08-07 | Block licence-compliance certification. |
| CI-001 | Enterprise-gate tooling uses moving `@latest` installations and some GitHub Actions are referenced by version tags instead of immutable commits. | Medium | Dedicated enterprise workflow and release workflow; release actions partly SHA-pinned. | Pin tool versions and checksums; pin actions by commit SHA; add scheduled update automation and change review. | Hossein Yousefi | 2026-08-07 | Does not alone block development; blocks reproducibility claim. |
| APP-001 | Gosec exclusions include decompression, symlink race, file permissions, dynamic file paths, path traversal, and SSRF categories without a structured per-instance justification and expiry. | High | Baseline documented in enterprise-gates documentation; other tests and review controls may exist. | Inventory every suppressed finding with file/line, exploitability analysis, compensating controls, owner, expiry, and test evidence. Fix reachable findings; narrow suppressions to exact locations. | Hossein Yousefi | 2026-08-14 | Block release if a reachable SSRF, traversal, arbitrary-file, or unsafe-decompression path remains. |
| GOV-001 | LLM profiles record provider and scopes but not customer approval metadata, data class, region, retention constraints, or policy acknowledgement. | Medium | Team owner grants access; customer provides provider account; scope allow-list. | Add optional governance metadata and an acknowledgement workflow without making NopsAI the approver of the customer's provider. Display provider/account responsibility and selected profile before use. | Hossein Yousefi | 2026-08-14 | Allowed with customer disclosure after SEC-001 is fixed. |
| LEG-001 | Contracting identity, EULA, DPA, liability terms, and support scope are not finalised for customer signature. | High | Counsel-review drafts in `doc/legal`. | Register legal form/trade name; complete company details; obtain Dutch counsel and insurance review; approve order form and security schedule. | Hossein Yousefi | Before first paid contract | Blocks contract signature. |
| IP-001 | NopsAI trademark has not received professional clearance or registration. | High commercial | Preliminary founder search only. | Run documented KVK/BOIP/EU/internet similarity search; obtain trademark-professional review; file appropriate word mark; monitor opposition and similar filings. | Hossein Yousefi | Before broad public launch | Does not block engineering; may require rebrand if delayed. |
| IP-002 | No documented targeted patent freedom-to-operate review for distinctive enterprise features. | Medium commercial | Public prior-art and patent databases available; no known claim mapping. | Prepare confidential feature brief; identify territories; obtain initial Netherlands Patent Office guidance and, where warranted, a registered patent attorney's claim review. | Hossein Yousefi | Before major enterprise launch or investment | Risk acceptance requires written founder decision after professional advice. |
| IP-003 | Sole-founder authorship and future transfer to a separate legal entity are not recorded in a signed founder IP declaration. | Medium | Repository history and sole-founder statement. | Sign and securely store Founder IP Declaration; disclose third-party assets; assign/license IP to a future BV if formed. | Hossein Yousefi | Before first contract or investment | Blocks ownership warranty until documented. |

## Accepted-risk rules

A risk may be temporarily accepted only when all of the following are recorded:

1. affected versions and deployment models;
2. technical evidence and exploitability analysis;
3. compensating controls;
4. customer disclosure where relevant;
5. named owner and approver;
6. expiry date no later than 90 days unless counsel or an independent security professional approves a longer period; and
7. an issue or pull request tracking permanent remediation.

Critical risks may not be accepted for an enterprise production release by the same person who authored the affected change without independent review.

## Release sign-off

Before an enterprise release, the founder must record:

- release version and immutable commit;
- open Critical/High risks;
- evidence links;
- customer-specific exceptions;
- approval or rejection decision; and
- date and signature/verified electronic approval.
