# NopsAI Data Processing Addendum

> **DRAFT FOR DUTCH/EU PRIVACY COUNSEL REVIEW. NOT APPROVED FOR SIGNATURE.**
>
> This draft assumes NopsAI is deployed in Customer-controlled infrastructure. The factual data flow and access model for each Order Form must be recorded before signature.

This Data Processing Addendum (**DPA**) forms part of the agreement between **[NopsAI contracting party]** (**NopsAI**) and **[Customer legal name]** (**Customer**).

## 1. Scope and roles

1.1 This DPA applies only where NopsAI processes personal data on behalf of Customer in connection with support or customer-hosted managed services.

1.2 Customer is the controller, or a processor acting under another controller's instructions, for Customer Personal Data. NopsAI is the processor or subprocessor, as applicable, only for processing carried out on Customer's documented instructions.

1.3 A self-hosted installation does not by itself mean NopsAI processes Customer Personal Data. If NopsAI has no access to the environment or operational data, the processor obligations in this DPA apply only to any separate support data actually received by NopsAI.

1.4 NopsAI may act as an independent controller for its own business-contact, billing, contract-administration, security, and legal-compliance records. That separate processing must be described in NopsAI's privacy notice.

## 2. Customer-selected LLM providers

2.1 NopsAI does not host or resell an LLM. Customer or an authorised team owner selects the provider, establishes the provider account, accepts the provider terms, pays the provider, supplies the credentials, and assigns access.

2.2 The Software may transmit Customer-authorised content from Customer's environment to the selected provider. Customer is responsible for determining whether that provider, model, region, retention setting, and account configuration are suitable for the intended data.

2.3 The parties must document the role of the selected LLM provider for the relevant deployment. Where Customer contracts directly with and controls the provider account, the provider will generally be governed by Customer's direct data-processing terms. This must be confirmed against the actual provider contract and technical flow.

2.4 NopsAI will not add an LLM provider to Customer's configuration or use Customer's credentials without Customer authorisation.

2.5 NopsAI will implement reasonable controls to avoid transmitting credentials and obvious secrets, but Customer remains responsible for access control, data classification, and provider approval. Automated redaction is a defence-in-depth control and not a guarantee that all personal or confidential data will be removed.

## 3. Documented instructions

NopsAI will process Customer Personal Data only:

- to provide the Services and support described in the Agreement and Order Form;
- through configurations and actions authorised by Customer;
- as otherwise documented in writing by Customer; or
- where required by applicable law, in which case NopsAI will inform Customer before processing unless prohibited by law.

NopsAI will notify Customer if, in its reasonable opinion, an instruction infringes applicable data-protection law.

## 4. Confidentiality

NopsAI will ensure that personnel authorised to process Customer Personal Data are bound by confidentiality obligations and receive access only as necessary for their duties.

## 5. Security measures

NopsAI will maintain appropriate technical and organisational measures proportionate to the processing and risk. The security schedule should address, as applicable:

- identity and least-privilege access;
- multi-factor authentication for privileged access;
- customer-approved and logged support access;
- encryption in transit and at rest where supported by Customer infrastructure;
- credential and secret management;
- secure development and code review;
- vulnerability and dependency management;
- signed release evidence, SBOMs, and third-party notices;
- environment separation;
- backup and recovery responsibilities;
- audit logging and monitoring;
- prompt and log redaction before LLM transmission;
- retention and deletion; and
- incident response.

Customer is responsible for the controls within Customer's infrastructure and accounts unless the Order Form assigns a specific control to NopsAI.

## 6. Personal-data breach

NopsAI will notify Customer without undue delay after becoming aware of a personal-data breach affecting Customer Personal Data processed by NopsAI.

The notice will provide information reasonably available to NopsAI, including the nature of the incident, affected data and subjects where known, likely consequences, containment measures, and a contact for follow-up. NopsAI may provide information in phases as the investigation develops.

Notification does not constitute an admission of fault or liability.

The Order Form or security schedule must state the operational notification contact and any contractually agreed notification target.

## 7. Assistance

Taking into account the nature of processing and information available, NopsAI will reasonably assist Customer with:

- data-subject requests;
- security and breach obligations;
- data-protection impact assessments;
- prior consultation with supervisory authorities; and
- demonstrating compliance.

Additional work outside normal support may be chargeable where permitted by the Agreement.

## 8. Subprocessors

8.1 NopsAI may use subprocessors only for processing it performs on Customer's behalf. The current list, processing purpose, and location must be maintained at **[subprocessor-list location]**.

8.2 Customer authorises the subprocessors listed at the effective date. NopsAI will provide **[30]** days' prior notice of a new or replacement subprocessor where reasonably practicable.

8.3 Customer may object on reasonable data-protection grounds. The parties will work in good faith on a mitigation. If no reasonable mitigation exists, either party may terminate the affected service under the agreed procedure.

8.4 NopsAI will impose materially equivalent data-protection obligations on subprocessors and remains responsible for their performance to the extent required by applicable law.

8.5 A third-party LLM provider selected, contracted, funded, and administered directly by Customer is not automatically a NopsAI subprocessor. The classification must follow the actual contracts and decision-making roles.

## 9. International transfers

NopsAI will not transfer Customer Personal Data outside the European Economic Area except under a lawful transfer mechanism and with supplementary measures where required.

Customer is responsible for international transfers arising from Customer-selected accounts and providers, including the selected LLM provider, unless NopsAI makes the relevant provider decision or contract on Customer's behalf.

The parties will execute applicable Standard Contractual Clauses or another valid transfer mechanism where required.

## 10. Retention, return, and deletion

10.1 Customer controls retention settings available in the Software. The production default and configurable limits must be stated in the product documentation and Order Form.

10.2 NopsAI will not retain support copies of Customer Personal Data longer than necessary for the authorised support purpose, contractual recordkeeping, security, or legal obligations.

10.3 At the end of the Services, NopsAI will, at Customer's choice, return or delete Customer Personal Data in NopsAI's possession, unless applicable law requires retention.

10.4 Backups may remain until overwritten in the ordinary backup cycle, provided they remain protected and are not restored except for recovery or legal necessity.

10.5 The product must provide or document procedures for conversation deletion, data export, and automated retention enforcement before an enterprise release is represented as supporting those controls.

## 11. Audit and evidence

NopsAI will make available information reasonably necessary to demonstrate compliance, such as relevant security documentation, release evidence, penetration-test summaries, certifications, or audit reports when available.

Customer may request an audit no more than once annually unless a breach or material compliance concern justifies an additional audit. Audits must be scoped, confidential, non-disruptive, and avoid exposing other customers' data or NopsAI trade secrets.

The parties should use independent reports and remote evidence before requiring an onsite audit.

## 12. Liability

Liability under this DPA is subject to the liability provisions of the Agreement, except where applicable law prohibits such limitation. Counsel must confirm whether any negotiated privacy-specific cap or allocation is appropriate.

## 13. Conflict and term

If this DPA conflicts with the Agreement on personal-data processing, this DPA controls. It remains effective for as long as NopsAI processes Customer Personal Data.

---

# Annex 1 — Processing description

Complete for each Order Form.

## Subject matter

Provision of self-hosted software support and/or customer-hosted deployment, configuration, maintenance, monitoring, and troubleshooting.

## Duration

The Subscription Term plus the documented deletion and backup period.

## Nature and purpose

- Authentication and authorisation.
- CI/CD and GitOps orchestration.
- Operational diagnostics and support.
- User-requested assistant processing through a Customer-selected LLM.
- Security, audit, and incident investigation.

## Categories of data subjects

Select those that apply:

- Customer employees and contractors.
- Customer administrators and developers.
- Users represented in source-control, CI/CD, ticketing, or identity-system metadata.
- Individuals whose data appears incidentally in logs, code, prompts, or support material.

## Types of personal data

Select and narrow those that apply:

- Names, business email addresses, usernames, and identifiers.
- Roles, teams, permissions, and authentication metadata.
- Source-control and pipeline activity metadata.
- Audit and security-event records.
- Prompt and conversation content.
- Logs and diagnostics that may incidentally contain personal data.
- IP addresses and device or session metadata.

Special-category data is not intentionally required and Customer must not submit it unless expressly agreed and protected by additional controls.

## Processing operations

Collection, access, transmission, organisation, storage, retrieval, consultation, redaction, support analysis, export, deletion, and other operations necessary to provide the contracted Services.

# Annex 2 — Security responsibility matrix

For every control, mark `Customer`, `NopsAI`, `Shared`, or `Not applicable`:

- Infrastructure and physical security.
- Kubernetes/host security.
- Network segmentation and firewalling.
- Identity provider and MFA.
- Application RBAC and team permissions.
- Secrets management.
- LLM provider selection and account security.
- LLM data-region and retention settings.
- Backups and disaster recovery.
- Logging and monitoring.
- Vulnerability remediation.
- Release verification.
- Support access approval and revocation.
- Incident notification contacts.
- Data export, retention, and deletion.

# Annex 3 — Subprocessors

No subprocessor may be listed without recording:

- legal name;
- service and purpose;
- data categories;
- processing location;
- transfer mechanism where applicable;
- contract owner; and
- approval date.
