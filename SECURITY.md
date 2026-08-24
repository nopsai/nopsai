# Security Policy

How to report a vulnerability in NopsAI, and what happens after you do. This
policy mirrors the published one at https://nopsai.com/security/ and the
machine-readable pointer at https://nopsai.com/.well-known/security.txt.

## Reporting

Email **contact@nopsai.com**. Please include:

- what the issue is and where it lives — service, endpoint, CLI command, chart
  or configuration surface;
- how to reproduce it, with the smallest sequence that demonstrates it;
- the NopsAI version, taken from `nopsai version` or the container image tag;
- the runtime it was reproduced on, Docker or Kubernetes; and
- what you assess the impact to be.

Do not open a public issue for a suspected vulnerability, and do not include
customer data, live credentials or production secrets in a report. If a
credential has been exposed, say that it has and rotate it — do not send it.

Preferred languages: English, Dutch.

## What we commit to

- We acknowledge reports and come back to you for reproduction details where we
  need them.
- We never ask a reporter to publish sensitive customer data.
- We prioritise issues affecting authorization, credential handling, execution
  isolation and audit integrity above everything else.
- We credit reporters who want credit, and stay quiet about those who do not.

There is no bug bounty. Reports are handled because they matter, not because
they pay.

## Safe harbour

We will not pursue or support legal action against anyone who reports in good
faith under this policy: who tests only against their own installation or an
environment they are authorised to test, avoids privacy violations and service
degradation, does not access or modify data that is not theirs, and gives us a
reasonable opportunity to respond before disclosing publicly.

## Scope

**In scope.** The NopsAI control plane, authorization service, dispatcher,
runners, agent, operator CLI, user interface, Helm charts, container images, the
release and bundle verification path, and the nopsai.com website.

**Out of scope.** A customer's own deployment configuration, infrastructure and
credential hygiene; third-party providers the customer connects — model
providers, MCP servers, Git hosts, container registries; findings that require
control of the host the platform runs on; and issues already documented as
accepted design trade-offs.

One trade-off is documented rather than treated as a finding: the Docker runner
binds the host Docker socket into the agent container so the agent can create
step containers, which means an agent on that host can do what the Docker daemon
can do. This is stated on the security page and in `doc/architecture-overview.md`.
Use the Kubernetes runner for production and treat a Docker runner host as a
dedicated job host. Reports of consequences that follow directly from this are
welcome as hardening suggestions, but they are known.

## Supported versions

Security fixes land in the current Release series and, where the fix is
material, in the immediately preceding minor series. See
[doc/release-bundles.md](doc/release-bundles.md) for how versions are formed and
[legal/commercial-software-licence-agreement.md](legal/commercial-software-licence-agreement.md)
clause 4 for the contractual position on updates.

## Why disclosure works differently for a self-hosted product

NopsAI runs inside customer infrastructure. We have no access to any deployment,
receive no telemetry from one, and cannot patch anyone's installation ourselves.
A fix is therefore not the end of the process — it is a Release, plus a notice
that operators should upgrade, plus whatever mitigation is available to those
who cannot upgrade immediately.

That shapes our disclosure timing. We aim to publish an available Release before
any public detail, and to give commercial customers advance notice where we have
a contact for them, because a public advisory against software the operator has
not yet patched helps an attacker more than it helps the operator.
