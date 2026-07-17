# NopsAI IP, Brand, and Patent Readiness Checklist

> Status: founder working document. Legal conclusions and final filings require qualified counsel or a registered patent/trademark professional.

## 1. What “contributor assignment” means

A contributor assignment is a signed agreement confirming who owns work contributed to the product and transferring the necessary intellectual-property rights to the business that sells the product.

It matters because paying someone, accepting a pull request, or calling someone a co-founder does not always transfer every copyright, database right, design right, invention right, or confidential know-how right needed for commercial licensing.

### Current NopsAI position

- Hossein Yousefi is the founder and only known contributor.
- If all code, documentation, designs, prompts, and brand assets were created personally by Hossein without using a previous employer's confidential information, equipment contrary to policy, or assigned work time, there is no outside contributor assignment currently required.
- The founder should still sign and retain a dated **Founder IP Declaration** recording authorship, third-party materials, prior-employer boundaries, and the intended business owner.
- A Dutch sole proprietorship has no legal personality separate from its owner. If NopsAI is operated as an `eenmanszaak`, Hossein remains the legal owner and contracting person, using NopsAI as a trade name.
- If a BV is created later, the relevant IP must be assigned or exclusively licensed to the BV in writing. The transfer list should include source code, history, documentation, UI/brand assets, domains, trademarks and applications, customer materials, release systems, and associated goodwill.

### Future contributor rule

Before any employee, contractor, agency, co-founder, intern, or external contributor starts work:

1. use a written employment, contractor, or contributor agreement;
2. identify pre-existing materials that remain theirs;
3. assign or license deliverables and related IP to the contracting NopsAI party;
4. include confidentiality and secure-return/deletion duties;
5. require disclosure and approval of third-party and AI-generated material;
6. require compliance with the repository contribution policy; and
7. preserve signed copies outside the source repository.

## 2. Founder IP Declaration — signing checklist

Prepare a short signed declaration containing:

- full legal name and address of Hossein Yousefi;
- current NopsAI legal form and KVK details, when available;
- a statement that the signer created the listed NopsAI works or identifies exceptions;
- the approximate development period;
- a list of repositories, domains, logos, documentation, designs, and other product assets;
- confirmation that no former employer or client owns the work, or a list of items requiring clearance;
- disclosure of third-party code, templates, datasets, fonts, images, icons, and AI tools used;
- confirmation that applicable third-party licence obligations will be respected;
- an assignment clause to a future or current BV, if counsel recommends it;
- further-assurances language for later registrations or evidence; and
- date and signature.

Do not commit the signed declaration or identity documents to GitHub. Store them in a restricted legal-records location.

## 3. Trademark clearance and registration

A preliminary exact-name search is not a trademark clearance opinion. Similar-looking or similar-sounding marks may conflict, and risk depends on the goods/services, territory, and actual market use.

### Required search scope

1. Search the KVK Business Register and BOIP Trademarks Register using the official name checker.
2. Search `NopsAI`, `Nops AI`, phonetic variants, common misspellings, and visually similar marks.
3. Review Benelux, EU, and international marks valid in the Benelux.
4. Search web domains, app stores, software directories, GitHub, social platforms, and relevant AI/DevOps markets for unregistered use.
5. Record the owner, classes, goods/services, territory, status, and risk for every close result.
6. Have a Benelux trademark professional review close matches before filing.

### Likely filing scope for counsel review

Potential Nice classes may include software and downloadable software, hosted or technical services, and business/technical consultancy. The exact class numbers and descriptions must be selected based on what NopsAI actually sells. Avoid filing broad descriptions that cannot be supported by genuine intended use.

### Filing decision

- Start with a Benelux filing if the near-term market is the Netherlands, Belgium, and Luxembourg.
- Consider an EU trade mark if genuine EU-wide expansion justifies the broader cost and conflict exposure.
- File the word mark first unless the logo has independent commercial importance.
- Do not use the registered symbol until registration is complete.
- Set reminders for opposition, renewal, monitoring, and evidence of genuine use.

## 4. What a targeted patent freedom-to-operate review means

A patent **novelty search** asks whether your invention appears new enough to patent.

A patent **freedom-to-operate (FTO) review** asks a different question: whether making, selling, importing, hosting, or using specific product features in specific countries may fall within claims of patents that are currently in force.

A targeted review is not a search for “the whole software industry.” It focuses on a short list of technically distinctive, commercially important features and the countries where NopsAI will be sold or operated.

### Suggested NopsAI feature map for an initial review

Ask a patent professional to decide whether any of these merit searching:

- automated CI/CD pipeline generation from natural-language or repository context;
- LLM-assisted failure analysis using pipeline logs and structured tool evidence;
- policy-controlled LLM profiles and team/scope access;
- GitOps change planning, validation, and controlled application;
- secure execution or mediation of pipeline/container operations;
- any distinctive orchestration, isolation, approval, or audit mechanism not standard in existing tools.

Generic use of an LLM, Kubernetes, Git, CI/CD, or RBAC is not by itself a useful FTO description. The search needs a precise technical workflow, architecture, and claim-relevant features.

### Initial process

1. Write a two-to-five-page confidential technical feature brief.
2. Identify launch territories, initially at least the Netherlands and any other intended sales markets.
3. Search public patent databases for related families and competitors.
4. Review the legal status and claims of relevant patents in each territory.
5. Map potentially relevant independent claims to the implemented feature.
6. Record design-around options, licence needs, expiry dates, and residual risk.
7. Obtain a written opinion from a registered patent attorney if a material risk is found or before a major enterprise launch or investment round.

The Netherlands Patent Office offers guidance and patent-database search support, but a commercial FTO conclusion should be made by a qualified patent professional based on the actual implementation and target territory.

## 5. Copyright and provenance inventory

Create an asset register covering:

- source files and commit history;
- copied snippets and generated code;
- npm and Go dependencies;
- container images and operating-system packages;
- fonts, icons, illustrations, screenshots, and logos;
- documentation templates and text;
- datasets, examples, fixtures, and model prompts;
- third-party APIs, SDKs, and schemas; and
- AI-assisted outputs requiring human review.

For each item record origin, author/supplier, licence, version, modifications, runtime or development scope, required notice, approval, and evidence location.

## 6. Immediate founder actions

- Register the chosen legal form and NopsAI trade name with KVK before invoicing customers.
- Run and save the official KVK/BOIP name-check results.
- Book a Benelux trademark-professional review and prepare a word-mark application.
- Sign and securely retain the Founder IP Declaration.
- Prepare the targeted technical feature brief for an initial patent consultation.
- Preserve architecture diagrams and dated design records, but do not publicly disclose potentially patentable technical inventions before obtaining patent advice.
- Use written IP terms before accepting any new contributor's work.
