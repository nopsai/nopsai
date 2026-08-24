# Legal and commercial documents

The customer-facing paperwork for NopsAI. Technical documentation lives in
[`../doc/`](../doc/); this directory holds the documents that decide what
someone is allowed to do with the software.

## The licence model in one paragraph

NopsAI is free for any non-commercial purpose, under the PolyForm Noncommercial
License 1.0.0 shipped as [`../LICENSE`](../LICENSE). That grant is unconditional
and uncapped: no key, no registration, no contact with us, and no limit on
users, teams or runs. Commercial use — running NopsAI in or for a business, or
for any other commercial purpose — is not granted by that licence and requires a
separate written agreement. That agreement is the Commercial Software Licence
Agreement in this directory, and it starts with an email to contact@nopsai.com.

## What is here

| Document | Purpose |
| --- | --- |
| [commercial-software-licence-agreement.md](./commercial-software-licence-agreement.md) | The agreement a commercial customer signs. Schedule 1 is the per-deal Order Form; Schedule 2 is a data processing agreement scoped to support material only. |
| [../LICENSE](../LICENSE) | The licence shipped in every artifact: PolyForm Noncommercial 1.0.0 under a NopsAI header. This is the whole grant for non-commercial use. |
| [../THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) | Third-party component obligations, and the per-release notice bundle that carries their licence texts. |
| [../SECURITY.md](../SECURITY.md) | Vulnerability disclosure policy. |
| [../doc/licensing-and-distribution.md](../doc/licensing-and-distribution.md) | How the licence model is carried through artifacts and release review. |
| [../doc/licensing-entitlements.md](../doc/licensing-entitlements.md) | How a commercial licence key records an entitlement, and where that is consulted. |
| [../doc/license-compliance.md](../doc/license-compliance.md) | Dependency licence policy and the gate that enforces it. |

## Why the commercial boundary is self-certified

Nothing in the software detects commercial use, and nothing in it ever will.
That is a consequence of three properties of how NopsAI is built, each of which
is load-bearing in the licence agreement and on the website, and none of which
is a policy that could quietly change:

1. **No customer data reaches the licensor.** Control plane, database, runners,
   credentials and evidence all live in the customer's environment.
2. **The software sends nothing home.** There is no update check, no licence
   activation call and no usage reporting. Every telemetry and analytics path in
   this repository terminates inside the customer's own deployment.
3. **The licensor supplies no models, MCP servers, agents, clusters or runner
   hosts.** Those are the customer's own accounts and contracts, attached
   through LLM profiles, MCP profiles and runner registration.

Point 2 is why clause 14 of the licence agreement makes licensed scope
self-certified, and why the production startup check states the licence position
rather than blocking on it: a business that declines to declare itself would be
unaffected by a gate, while a gate would block the non-commercial use the
licence expressly grants. If a future release ever adds an outbound call to
licensor-controlled infrastructure, clause 14, `SECURITY.md`, and the privacy
and security pages on the website all have to change with it.

## How this stays consistent with the website

The published legal pages — privacy notice, terms of use and legal notice at
nopsai.com — are generated from `src/content/simplePages.ts` and
`src/content/site.ts` in the `web` repository, and the pricing page from
`src/content/pricing.ts`. Anything said here about identity, data handling, the
licence grant or the commercial boundary has a counterpart on those pages, and
the two must not drift.
