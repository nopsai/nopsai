# NopsAI Enterprise Commercial Baseline

> Status: internal baseline for product, security, sales, and legal review. This document is not customer-facing legal advice.

## 1. Product owner and current contracting identity

- Product and trade name: **NopsAI**.
- Founder and current sole contributor: **Hossein Yousefi**.
- Country of establishment: **the Netherlands**.
- Before signing customer contracts, the order form and legal documents must identify the actual contracting party, legal form, registered address, KVK number, and VAT number.
- Until a separate legal entity exists, `NopsAI` must be described as a trade name of Hossein Yousefi or of the registered sole proprietorship, rather than as a separate company.
- If a Dutch BV is formed later, the software, brand assets, domains, documentation, customer contracts, and related intellectual-property rights must be transferred or licensed to the BV in writing.

## 2. Delivery models

NopsAI supports two enterprise delivery models.

### 2.1 Self-hosted software licence

The customer installs, operates, monitors, backs up, and secures NopsAI in infrastructure controlled by the customer. NopsAI provides software, updates, documentation, and contracted support.

### 2.2 Customer-hosted managed service

NopsAI personnel may deploy, configure, update, troubleshoot, or operate NopsAI inside infrastructure owned or controlled by the customer. This is not NopsAI-hosted SaaS. Access must be customer-authorised, time-limited where practical, logged, and removed when no longer required.

## 3. LLM responsibility model

- NopsAI does not host or resell an LLM service.
- The customer or its team owner chooses the LLM provider, opens and pays for the provider account, supplies the credentials, and grants access to teams.
- The customer is responsible for selecting providers and models suitable for its data, geography, retention requirements, contractual commitments, and internal policies.
- NopsAI must clearly show which provider/profile is selected before data is transmitted.
- NopsAI must enforce team and scope permissions configured by the customer.
- NopsAI must minimise and redact credentials and obvious secrets before prompt, conversation, pipeline-log, tool-output, or diagnostic content is sent to any configured LLM.
- Provider credentials must remain in the customer's environment and must not be included in prompts, logs, exports, support bundles, or telemetry.
- The customer must be able to disable hosted providers and operate with an approved local endpoint where supported.

Customer ownership of the LLM account does not remove NopsAI's obligation to implement secure defaults, accurate disclosures, access control, data minimisation, deletion, and auditable configuration.

## 4. Data-role baseline

The final GDPR role depends on the facts of each engagement.

- For product telemetry, billing, account administration, sales, and NopsAI's own support records, the contracting NopsAI party may act as an independent controller.
- For customer data accessed while delivering an authorised managed service or support engagement, NopsAI will normally act as a processor acting on documented customer instructions.
- Where NopsAI has no access to a self-hosted deployment, it may not process the customer's operational data at all.
- A customer-selected LLM provider using the customer's direct account is normally governed by the customer's agreement with that provider. Legal counsel must confirm the exact processor/subprocessor description used in each order form and DPA.

## 5. Enterprise release requirements

An enterprise release is not approved unless it includes or references:

1. Reproducible release identifiers and immutable image digests.
2. SBOMs for shipped binaries, images, charts, and material third-party runtime images.
3. Third-party licence notices and the applicable licence texts.
4. Signed release evidence and documented verification instructions.
5. Security scan results and an approved, dated exception register.
6. Data-retention, export, and deletion behaviour matching published documentation.
7. LLM-boundary redaction tests.
8. Customer-facing EULA/order form and, where applicable, a DPA reviewed by Dutch counsel.

## 6. Decisions still required before first contract

- Registered legal form and contracting name.
- Registered address, KVK number, VAT number, and support contact.
- Pricing, licence metric, subscription term, support levels, and service hours.
- Warranty period and liability cap approved by counsel and insurer.
- Whether managed-service personnel can access production data and the approval process for that access.
- Default assistant conversation retention period and customer-configurable limits.
- Trademark filing territory and goods/services classes.

## 7. Owner

Until roles are delegated, Hossein Yousefi is the accountable owner for legal readiness, security exceptions, release approval, and remediation tracking.
