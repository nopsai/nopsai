# Legal and commercial documents

The customer-facing paperwork for NopsAI. Technical documentation lives in
[`../doc/`](../doc/); this directory holds the documents that decide what a
customer is allowed to do with the software.

Last reviewed: 13 August 2026.

## What is here

| Document | Purpose | Status |
| --- | --- | --- |
| [commercial-software-licence-agreement.md](./commercial-software-licence-agreement.md) | The licence a paying customer signs. Schedule 1 is the per-deal Order Form; Schedule 2 is a data processing agreement scoped to support material only. | Draft, pending review by a Dutch technology lawyer |
| [../LICENSE](../LICENSE) | Proprietary notice shipped in every artifact. Not a licence to use anything. | In force |
| [../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) | Third-party component obligations. | In force; the per-release notice bundle is still outstanding |
| [../SECURITY.md](../SECURITY.md) | Vulnerability disclosure policy. | In force |
| [../doc/licensing-and-distribution.md](../doc/licensing-and-distribution.md) | How the licence model is enforced in artifacts and at release review. | In force |
| [../doc/license-compliance.md](../doc/license-compliance.md) | Dependency licence policy and the gate that enforces it. | In force |

## What is missing

**A design-partner or evaluation agreement**, which is the gap that matters
today. The website describes a design-partner phase in which partners run real
workflows in their own environments for a sustained period; that is licensed use
of proprietary software, and the Commercial Software Licence Agreement — an
annual paid subscription — is the wrong instrument for it. Until this document
exists, software may be running in someone else's environment under nothing more
than an email.

Also missing: a mutual NDA, a support policy defining the response targets that
clause 5.1 of the licence agreement defers to the Order Form, and a services
schedule or statement-of-work template that
[../doc/licensing-and-distribution.md](../doc/licensing-and-distribution.md)
already refers to as though it exists.

## How this stays consistent with the website

The published legal pages — privacy notice, terms of use and legal notice at
nopsai.com — are generated from `src/content/simplePages.ts` and
`src/content/site.ts` in the `web` repository, and
`web/doc/legal-status.md` there records their status and open items. Anything
said here about identity, registration status, data handling or the licence
grant has a counterpart on those pages, and the two must not drift.

Three positions in particular are load-bearing in both places, and each is a
property of how the product is built rather than a policy that could quietly
change:

1. **No customer data reaches the licensor.** Control plane, database, runners,
   credentials and evidence all live in the customer's environment.
2. **The software sends nothing home.** There is no update check, no licence
   activation call and no usage reporting. Every telemetry and analytics path in
   this repository terminates inside the customer's own deployment.
3. **The licensor supplies no models, MCP servers, agents, clusters or runner
   hosts.** Those are the customer's own accounts and contracts, attached
   through LLM profiles, MCP profiles and runner registration.

Point 2 is why clause 14 of the licence agreement makes licensed scope
self-certified. If a future release ever adds an outbound call to
licensor-controlled infrastructure, clause 14, `SECURITY.md`, and the privacy and
security pages on the website all have to change with it.
