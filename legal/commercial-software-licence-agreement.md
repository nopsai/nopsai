# NopsAI Commercial Software Licence Agreement

> **Draft for legal review.** This is a working first draft, written to be
> internally consistent with how NopsAI is actually built and delivered — the
> self-hosted deployment model, the absence of usage telemetry, and the fact
> that the Licensor is currently a natural person rather than a registered
> company. It has not been reviewed by a qualified lawyer. Have a Dutch
> technology lawyer review it before it is put in front of a customer, and in
> particular before relying on clauses 13 (Liability), 15 (Indemnity) and 20
> (Governing law).
>
> Bracketed values in `[BRACKETS]` are completed per deal in the Order Form.
> The Order Form template is Schedule 1.

---

## Parties

This Agreement is entered into between:

**(1) Hossein Yousefi**, a natural person trading under the name **NopsAI**, of
Emmakade 10, 2411 JA Bodegraven, the Netherlands (the **"Licensor"**); and

**(2) `[CUSTOMER LEGAL NAME]`**, a `[LEGAL FORM]` registered in
`[JURISDICTION]` under number `[REGISTRATION NUMBER]`, of `[REGISTERED
ADDRESS]` (the **"Customer"**).

Each a "Party" and together the "Parties".

The Licensor is not presently registered with the Dutch Chamber of Commerce.
Clause 18.2 governs what happens to this Agreement when the NopsAI business is
transferred to a registered legal entity, and the Customer's consent to that
transfer is given in advance under that clause.

## 1. Definitions

1.1 **"Software"** means the NopsAI platform in object-code form, comprising the
control plane, authorization service, dispatcher, runners, agent, operator CLI,
user interface, Helm charts, container images, configuration schemas and
accompanying Documentation, together with any Updates supplied under this
Agreement.

1.2 **"Documentation"** means the technical documentation the Licensor makes
available for the licensed Release.

1.3 **"Release"** means a version of the Software identified by the semantic
version applied consistently across its container images, Helm chart and CLI
archives.

1.4 **"Update"** means a new Release made generally available to licensed
customers during the Subscription Term, including corrections, security fixes
and new functionality, but excluding any product the Licensor designates and
prices as a separate offering.

1.5 **"Licensed Organization"** means the Customer legal entity named above,
and no other entity. Affiliates are included only if listed in the Order Form.

1.6 **"Licensed Scope"** means the number of production deployments or
environments, and any other scope metrics, stated in the Order Form.

1.7 **"Customer Environment"** means infrastructure owned, rented or otherwise
controlled by the Customer, in which the Customer installs and operates the
Software.

1.8 **"Customer Data"** means all data processed by the Software in the Customer
Environment, including pipeline definitions, credentials, run logs, approval
records, knowledge documents, model prompts and completions, and generated
outputs.

1.9 **"Third-Party Providers"** means the model providers, MCP servers,
container orchestrators, source control systems and other external services the
Customer connects to the Software.

1.10 **"Support Material"** means anything the Customer voluntarily sends to the
Licensor in connection with a support request, such as log excerpts,
configuration files, screenshots or the contents of a screen share.

1.11 **"Subscription Term"** means the initial term stated in the Order Form and
each renewal term under clause 8.

1.12 **"Fees"** means the amounts stated in the Order Form.

## 2. Licence grant

2.1 Subject to the Customer's compliance with this Agreement and payment of the
Fees, the Licensor grants the Customer, for the Subscription Term, a licence
that is:

  (a) **annual and subscription-based**, coterminous with the Subscription Term
      and conferring no perpetual right;

  (b) **non-exclusive**;

  (c) **non-transferable**, except as permitted by clause 18;

  (d) **non-sublicensable**;

  (e) **for the Customer's internal business operations only**;

  (f) **limited to the Licensed Organization**;

  (g) **limited to the Licensed Scope**, being the production deployments or
      environments recorded in the Order Form;

  (h) **for self-hosted operation in the Customer Environment**; and

  (i) **conditional on payment of the Fees and on compliance with this
      Agreement**.

2.2 The licence is to install, configure, execute and operate the Software, and
to make a reasonable number of copies for backup, disaster recovery, staging and
testing purposes supporting the Licensed Scope.

2.3 The Software is licensed, not sold. All rights not expressly granted are
reserved. Possession of, or access to, any Software artifact confers no right to
use it.

2.4 Non-production environments used solely for development, testing, staging or
disaster-recovery standby do not consume Licensed Scope unless the Order Form
states otherwise.

## 3. Restrictions

3.1 The Customer shall not, and shall not permit any third party to:

  (a) provide the Software to any third party, or operate it as a service
      bureau, hosted service or managed offering for the benefit of anyone other
      than the Licensed Organization;

  (b) sublicense, sell, lease, rent, lend or otherwise transfer the Software;

  (c) reverse engineer, decompile or disassemble the Software, except to the
      extent that such acts cannot lawfully be prohibited under article 45m of
      the Dutch Copyright Act or other mandatory law, and then only after
      requesting the necessary interoperability information from the Licensor
      and allowing a reasonable period for a response;

  (d) remove, obscure or alter any proprietary notice, licence identifier or
      third-party notice in or accompanying the Software;

  (e) exceed the Licensed Scope, or circumvent any technical or contractual
      limit on it;

  (f) use the Software to build, train or materially assist a competing product;
      or

  (g) publish benchmark results for the Software without the Licensor's prior
      written consent, which will not be unreasonably withheld where the
      methodology is disclosed.

3.2 The Customer is responsible for use of the Software by its personnel and
contractors as if it were its own.

## 4. Delivery, Releases and Updates

4.1 The Licensor delivers the Software electronically: container images, the
Helm chart, CLI archives and the corresponding checksums and notice files.

4.2 During the Subscription Term the Customer is entitled to all Updates the
Licensor makes generally available to licensed customers, at no additional
charge.

4.3 The Customer decides when to apply an Update, because the Customer operates
the deployment. The Licensor supports the current Release and the immediately
preceding minor series; a Customer running an older Release may be asked to
upgrade before a defect is investigated.

4.4 The Licensor may change the Software's functionality between Releases, but
will not remove a materially relied-upon capability during a Subscription Term
without a reasonable transition path.

## 5. Support

5.1 Support is provided at the level stated in the Order Form, through the
channels stated there, during the Licensor's business hours in the Netherlands.

5.2 The Software transmits nothing to the Licensor and the Licensor has no
access to the Customer Environment. It follows that the Licensor cannot observe
a problem: the Customer must report it and supply enough information to
reproduce or diagnose it.

5.3 Support Material is handled under clause 11.3 and clause 12.

5.4 Support excludes work on the Customer's own infrastructure, on Third-Party
Providers, on Customer-authored pipelines, and on modifications not supplied by
the Licensor.

## 6. Customer responsibilities

6.1 The Customer provides, operates and pays for everything the Software runs on
and connects to, including compute, storage, networking, container
orchestration, Kubernetes clusters and Docker hosts.

6.2 The Customer provides its own accounts, credentials and contracts with all
Third-Party Providers, including model providers and MCP servers. The Licensor
supplies no models, no inference endpoints, no MCP servers, no agents and no
runner infrastructure. Availability, terms, output quality and charges of
Third-Party Providers are matters between the Customer and those providers, and
the Licensor is not liable for them.

6.3 The Customer is responsible for securing the Customer Environment, including
identity configuration, network exposure, credential custody, backup and
recovery, and for configuring the Software's authorization, approval and
governance controls appropriately for its own risk profile.

6.4 The Customer is responsible for the lawfulness of the workflows it automates
and of the actions the Software is configured to take on its behalf.

## 7. Fees and payment

7.1 The Customer pays the Fees stated in the Order Form, annually in advance
unless stated otherwise, within `[30]` days of the invoice date.

7.2 Fees are exclusive of VAT and any other applicable taxes, which the Customer
pays in addition where due.

7.3 Late payment accrues statutory commercial interest under article 6:119a of
the Dutch Civil Code, without notice of default being required, together with
reasonable costs of collection.

7.4 If Fees remain unpaid more than `[30]` days after written notice of
non-payment, the Licensor may suspend support and Updates until payment is
made. Suspension does not extend the Subscription Term.

7.5 The Licensor may increase the Fees for a renewal term by giving at least
`[60]` days' written notice before the renewal date.

7.6 Fees are non-refundable except where this Agreement expressly says
otherwise.

## 8. Term, renewal and termination

8.1 This Agreement starts on the effective date in the Order Form and continues
for the initial Subscription Term.

8.2 It renews automatically for successive twelve-month terms unless either
Party gives written notice of non-renewal at least `[60]` days before the end of
the then-current term.

8.3 Either Party may terminate for material breach by written notice if the
breach is not remedied within 30 days of written notice describing it. Breach of
clause 3 (Restrictions) is material by definition, and the Licensor may
terminate immediately where the breach is not capable of remedy.

8.4 Either Party may terminate immediately if the other is declared bankrupt,
granted suspension of payments, or ceases to carry on business.

8.5 On expiry or termination the licence ends. The Customer shall within 30 days
cease all use of the Software, remove it from the Customer Environment, and
confirm in writing that it has done so.

8.6 Termination does not affect Customer Data, which is and remains in the
Customer Environment. The Customer is solely responsible for extracting anything
it wishes to retain before removing the Software.

8.7 Clauses 3, 9, 10, 12, 13, 14, 15, 16, 18 and 20 survive termination.

## 9. Intellectual property

9.1 The Licensor and its licensors own all intellectual property rights in the
Software, the Documentation, and the NopsAI name and brand.

9.2 The Customer owns all Customer Data and all pipelines, configuration and
content it authors. Nothing in this Agreement grants the Licensor any right to
Customer Data.

9.3 If the Customer provides feedback, suggestions or feature requests, the
Licensor may use them without restriction or obligation. This does not transfer
any Customer Data or confidential information.

## 10. Third-party components

10.1 The Software includes third-party components licensed under their own
terms, identified in the third-party notices supplied with each Release. Those
terms govern those components, and nothing in this Agreement restricts rights
the Customer has under them.

10.2 The Customer shall preserve all third-party notices in any copy it makes.

## 11. Data protection

11.1 The Software is self-hosted. Customer Data is created, processed and stored
entirely within the Customer Environment. The Licensor operates no database, log
store, artifact store or model endpoint holding Customer Data, has no access to
the Customer Environment, and cannot retrieve Customer Data from it. In respect
of Customer Data processed by the Software, the Licensor is neither controller
nor processor, because it processes none of it.

11.2 The Customer is the controller for personal data processed by the Software
and is responsible for the lawful basis, the information given to data subjects,
and the handling of data-subject requests.

11.3 Support Material is the one exception. Where Support Material contains
personal data, the Licensor acts as processor on the Customer's documented
instructions, uses it solely to answer the support request, does not transfer it
to any other system, applies appropriate technical and organisational measures,
and deletes it when the request is closed. The Data Processing Agreement at
Schedule 2 applies to Support Material and to nothing else. The Customer shall
minimise and redact Support Material so far as the request permits.

11.4 Where the Customer's use of the Software makes it a provider or deployer of
an AI system under Regulation (EU) 2024/1689, those obligations rest with the
Customer. The Licensor supplies orchestration software containing no model; the
AI system placed into service is the combination the Customer assembles from its
own models, tools and pipelines. The Licensor will provide reasonable technical
information within its knowledge to support the Customer's own assessment.

## 12. Confidentiality

12.1 Each Party shall keep the other's confidential information confidential,
use it only for the purposes of this Agreement, and disclose it only to
personnel and advisers who need it and are bound by equivalent obligations.

12.2 The obligation does not apply to information that is or becomes public
without breach, was lawfully known before disclosure, is independently
developed, or must be disclosed by law — in which case the disclosing Party
gives prior notice where lawful.

12.3 This obligation continues for five years after termination, and
indefinitely for anything qualifying as a trade secret.

## 13. Warranties and disclaimers

13.1 The Licensor warrants that it has the right to grant the licence in
clause 2, and that the Software will perform substantially in accordance with
the Documentation for the Release supplied.

13.2 If the Software does not meet the warranty in clause 13.1 and the Customer
reports it in writing, the Licensor's obligation, and the Customer's exclusive
remedy, is to correct it within a reasonable period or, failing that, to
terminate the affected licence and refund the Fees for the unexpired part of the
Subscription Term.

13.3 Except as stated in clause 13.1, the Software is provided "as is". The
Licensor does not warrant that operation will be uninterrupted or error-free,
that it will meet the Customer's requirements, or that defects will all be
corrected.

13.4 The Software orchestrates AI-assisted work. The Licensor gives no warranty
as to the accuracy, completeness, suitability or lawfulness of any model output.
The governance controls the Software provides — authorization, approval gates,
tool allowlisting and evidence capture — are effective only to the extent the
Customer configures and uses them, and the Customer remains responsible for
deciding which automated actions are permitted to take effect.

## 14. Verification of scope

14.1 The Software transmits no usage telemetry to the Licensor. Compliance with
the Licensed Scope is therefore self-certified.

14.2 On the Licensor's written request, and no more than once in any twelve-month
period, the Customer shall provide a written statement signed by an authorised
officer confirming the number of production deployments or environments in
which the Software is operated.

14.3 If a statement, or any other information, shows use beyond the Licensed
Scope, the Customer shall pay the additional Fees for that use from the date it
began, at the rates then applying.

14.4 The Licensor has no right of physical or remote access to the Customer
Environment for verification, and shall not require one.

## 15. Indemnity

15.1 The Licensor shall defend the Customer against any third-party claim that
the Software, used in accordance with this Agreement, infringes that party's
intellectual property rights in the European Union, and shall pay the damages
and costs finally awarded or agreed in settlement, provided the Customer
promptly notifies the Licensor, gives the Licensor sole control of the defence,
and provides reasonable assistance.

15.2 If such a claim is made or appears likely, the Licensor may at its own cost
procure the right to continue using the Software, modify or replace it so it
becomes non-infringing, or, if neither is reasonably available, terminate the
affected licence and refund the Fees for the unexpired part of the Subscription
Term.

15.3 The indemnity does not apply to claims arising from modification of the
Software by anyone other than the Licensor, combination with anything not
supplied by the Licensor where the claim would not have arisen without that
combination, use outside this Agreement, or use of a superseded Release after
the Licensor has made a non-infringing Release available at no additional cost.

15.4 Clause 15 states the Licensor's entire liability for intellectual property
infringement.

## 16. Limitation of liability

16.1 Neither Party excludes or limits liability for intent (*opzet*), wilful
recklessness (*bewuste roekeloosheid*), death or personal injury, or any other
liability that cannot lawfully be excluded.

16.2 Subject to clause 16.1, neither Party is liable for indirect or
consequential loss, loss of profit, loss of revenue, loss of anticipated
savings, loss of goodwill, business interruption, or loss or corruption of data,
however arising.

16.3 Subject to clauses 16.1 and 16.4, each Party's total aggregate liability
arising out of or in connection with this Agreement in any twelve-month period
is limited to the Fees paid or payable by the Customer for that period.

16.4 Clause 16.3 does not limit the Customer's obligation to pay the Fees, or
the Licensor's obligations under clause 15.

16.5 The Parties acknowledge that the limitations in this clause are a
reasonable allocation of risk reflected in the level of the Fees.

## 17. Compliance and sanctions

17.1 Each Party shall comply with applicable export control, sanctions and
anti-bribery laws.

17.2 The Customer shall not make the Software available to any person or in any
territory in breach of European Union or applicable national restrictions.

## 18. Assignment and change of Licensor

18.1 Neither Party may assign or transfer this Agreement without the other's
prior written consent, which shall not be unreasonably withheld, except that
either Party may assign it in full to a successor in connection with a merger,
reorganisation, or sale of substantially all of its assets or of the business to
which this Agreement relates.

18.2 The Licensor intends to transfer the NopsAI business to a Netherlands
private limited company (*besloten vennootschap*) on its incorporation. The
Customer consents in advance to the transfer of this Agreement, including all
rights and obligations under it, to that entity by contract takeover
(*contractsoverneming*) under article 6:159 of the Dutch Civil Code. The
Licensor shall give the Customer written notice of the transfer, including the
entity's registered name, address and Chamber of Commerce number. The transfer
does not alter the Fees, the Licensed Scope or any other term.

18.3 The Licensor may use subcontractors to perform its obligations but remains
responsible for their performance.

## 19. Publicity

19.1 Neither Party may use the other's name, logo or marks publicly without
prior written consent. Consent given for one use is not consent for another.

19.2 The Customer's identity, its workflows and any metrics relating to its use
of the Software are confidential and will not be published by the Licensor
without written permission.

## 20. General

20.1 **Notices.** Notices must be in writing and sent to the addresses in the
Order Form, or to contact@nopsai.com for the Licensor. Email is sufficient for
all notices except termination, which additionally requires postal delivery.

20.2 **Entire agreement.** This Agreement, its Schedules and the Order Form are
the entire agreement between the Parties on this subject and supersede all prior
discussions, proposals and marketing material, including any statement on
nopsai.com. Neither Party has relied on any representation not set out here,
save that nothing excludes liability for fraudulent misrepresentation.

20.3 **Order of precedence.** In case of conflict: the Order Form, then this
Agreement, then the Schedules.

20.4 **No general terms.** The applicability of either Party's general terms and
conditions is expressly excluded, including under article 6:225(3) of the Dutch
Civil Code.

20.5 **Amendment.** Amendments must be in writing and signed by both Parties.

20.6 **No dissolution.** To the extent permitted by law, the Parties exclude the
right to dissolve (*ontbinden*) this Agreement under article 6:265 of the Dutch
Civil Code and the right to invoke error (*dwaling*) under article 6:228, other
than as expressly provided in clause 8.

20.7 **Severability.** If any provision is held invalid, the remainder stays in
force and the invalid provision is replaced by a valid one approximating its
commercial intent as closely as possible.

20.8 **No waiver.** Failure to enforce a provision is not a waiver of it.

20.9 **Force majeure.** Neither Party is liable for failure to perform caused by
an event beyond its reasonable control, excluding payment obligations.

20.10 **Governing law.** This Agreement is governed by the laws of the
Netherlands. The United Nations Convention on Contracts for the International
Sale of Goods does not apply.

20.11 **Jurisdiction.** Disputes are submitted to the competent court in the
Netherlands, unless mandatory law requires otherwise.

---

## Schedule 1 — Order Form template

| Item | Value |
| --- | --- |
| Customer legal name | `[…]` |
| Registration number and jurisdiction | `[…]` |
| Affiliates included in the Licensed Organization | `[none / list]` |
| Effective date | `[…]` |
| Initial Subscription Term | `[12 months]` |
| Licensed Scope — production deployments or environments | `[…]` |
| Other scope metrics, if any | `[teams / repositories / runner pools]` |
| Annual Fee | `[…] EUR, excluding VAT` |
| Payment terms | `[annually in advance, 30 days]` |
| Support level and channels | `[email / scheduled call, business hours CET]` |
| Support response targets | `[…]` |
| Implementation or services scope | `[separate SOW / none]` |
| Notice addresses | `[…]` |
| Signatories | `[…]` |

## Schedule 2 — Data Processing Agreement for Support Material

Applies only to Support Material, as defined in clause 1.10, and only where it
contains personal data. It does not apply to Customer Data, which the Licensor
never processes.

1. **Roles.** The Customer is controller; the Licensor is processor.
2. **Subject matter and duration.** Diagnosis and resolution of a support
   request, for the duration of that request.
3. **Nature and purpose.** Receipt, reading, and technical analysis of material
   voluntarily supplied by the Customer, solely to answer the request.
4. **Categories of data subject.** Whatever the Customer's own systems contain
   and the Customer chooses to send — typically the Customer's personnel, such
   as pipeline authors, approvers and operators.
5. **Categories of personal data.** Typically names, work email addresses,
   usernames, IP addresses and free text appearing in logs, configuration or
   screenshots. The Customer shall not send special categories of personal data.
6. **Instructions.** The Licensor processes Support Material only on the
   Customer's documented instructions, being the support request itself and this
   Schedule, unless required otherwise by law, in which case it notifies the
   Customer in advance where lawful.
7. **Confidentiality.** Personnel with access are bound by confidentiality.
8. **Security.** Support Material is held in the Licensor's business email and,
   where necessary, on encrypted end-user devices with access control. It is not
   copied into any other system, and not used for training, product development
   or any other purpose.
9. **Sub-processors.** The Licensor's business email provider processes Support
   Material as a sub-processor by necessity. The Licensor shall notify the
   Customer of any intended change and give the Customer a reasonable
   opportunity to object.
10. **Transfers.** Any transfer outside the European Economic Area relies on the
    European Commission's standard contractual clauses.
11. **Assistance.** The Licensor shall assist the Customer, at the Customer's
    cost where the effort is more than trivial, with data-subject requests,
    security incidents, data protection impact assessments and consultations
    with a supervisory authority.
12. **Breach notification.** The Licensor shall notify the Customer without
    undue delay, and in any event within 48 hours, of becoming aware of a
    personal data breach affecting Support Material.
13. **Deletion.** The Licensor shall delete Support Material when the support
    request is closed, and in any event on termination, and shall confirm
    deletion in writing on request.
14. **Audit.** The Licensor shall make available the information necessary to
    demonstrate compliance with this Schedule and shall allow and contribute to
    audits by the Customer or its mandated auditor, no more than once a year
    unless a breach has occurred.

---

**Signed for and on behalf of the Licensor**

Name: Hossein Yousefi
Date: `[…]`

**Signed for and on behalf of the Customer**

Name: `[…]`
Title: `[…]`
Date: `[…]`
