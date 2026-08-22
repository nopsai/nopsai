# NopsAI Documentation Map

This team now has a code-grounded documentation set for the current repository shape.

Start here when you want to understand the system from different angles:

- [architecture-overview.md](./architecture-overview.md): The big picture, main components, core data model, and deployment shape.
- [service-reference.md](./service-reference.md): What each service owns, how it talks to the others, and the main source files to read.
- [../examples/README.md](../examples/README.md): Runnable and copyable examples, including GitOps sample repos, sample pipelines, and SSO fixtures.
- [cli.md](./cli.md): Operator CLI contexts, credentials, interactive REST access, completion files, one-command installs, platform diagnostics, GitOps deployment, and binary separation.
- [release-bundles.md](./release-bundles.md): Shared build identity, versioned release assets, CLI-generated Compose/Kubernetes install files, Helm chart publication, and GitOps release locks.
- [runtime-flows.md](./runtime-flows.md): Step-by-step execution flows for request authorization, webhooks, manual runs, dispatching, agent execution, child pipelines, cancellation, and config sync.
- [team-resource-ownership-design.md](./team-resource-ownership-design.md): Target design for separating Teams from Pipeline Runs and adding team-scoped LLM, Agent, and MCP profiles.
- [kubernetes-runner.md](./kubernetes-runner.md): Kubernetes runner runtime, namespace deployment, agent-owned PVC workspace behavior, node affinity, GitOps settings, one-time install commands, and GitOps manifests.
- [runner-registry-auth.md](./runner-registry-auth.md): Private Docker registry credentials for runner bootstrap, Docker API image pulls, Kubernetes imagePullSecrets, GitOps assignments, and audit/metrics behavior.
- [feature-reference.md](./feature-reference.md): Functional capabilities exposed by the codebase and UI.
- [dashboards.md](./dashboards.md): Team dashboard model, dashboard final outputs, chart/series publication, publication history, source bindings, scheduled refresh orchestration, GitOps ownership, AAA, monitoring, and MCP.
- [assistant-capabilities.md](./assistant-capabilities.md): User-facing assistant capabilities and example chat prompts for each NopsAI feature area.
- [first-install-wizard.md](./first-install-wizard.md): UI bootstrap flow for an empty database, starter profiles, secret generation, GitOps seeding, repository teams, and production guardrails.
- [enterprise-gates.md](./enterprise-gates.md): Production startup gates, local/CI verification commands, Docker build checks, and integration coverage baseline.
- [performance-testing.md](./performance-testing.md): Backend load-test harness, request/auth/webhook/pipeline suites, concurrency ramps, per-service resource sampling, saturation analysis, and regression gating.
- [license-compliance.md](./license-compliance.md): Commercial dependency-license policy, current language dependency audit, container/service review boundaries, and MCP/plugin obligations.
- [enterprise-refactor-roadmap.md](./enterprise-refactor-roadmap.md): Enterprise feature ownership, SSO follow-up status, and remaining refactor targets.
- [credential-management.md](./credential-management.md): Implemented single credential interface, encrypted registry, bootstrap boundary, GitOps references and encrypted envelopes, AAA, rotation, and migration behavior.
- [package-ownership.md](./package-ownership.md): Handler, service, store, domain, DTO, provider-client, and bootstrap ownership rules.
- [services/ui/src/README.md](../services/ui/src/README.md): UI source ownership, feature contracts, accessibility primitives, and test placement rules.
- [decision-architecture.md](./decision-architecture.md): The main architectural decisions, why they exist, and the tradeoffs they introduce.
- [access-control.md](./access-control.md): Current AAA service, product roles, access grants, route authorization, and audit behavior.
- [jwt-authentication.md](./jwt-authentication.md): Local auth, Enterprise SSO/OIDC, User/API JWTs, refresh tokens, internal REST service JWTs, and dispatcher gRPC service JWTs.
- [local-keycloak-sso.md](./local-keycloak-sso.md): Local Keycloak fixture under `examples/sso/keycloak` with seeded users, teams, and OIDC settings for SSO testing.
- [knowledge-context.md](./knowledge-context.md): Project knowledge documents for LLM-backed pipeline steps, GitOps layout, runtime snapshots, and access checks.
- [agent-roles.md](./agent-roles.md): AI role/persona selection for pipelines and steps, GitOps management, AAA, and runtime prompt behavior.
- [final-output-rendering.md](./final-output-rendering.md): Final-output generation contracts, retry/audit behavior, current renderers, and the structured-document rendering roadmap.
- [system-logs.md](./system-logs.md): Live allow-listed platform logs, Docker and Kubernetes providers, SSE replay, AAA, redaction, limits, GitOps configuration, and monitoring.
- [browser-console-troubleshooting.md](./browser-console-troubleshooting.md): How to triage DevTools warnings from injected browser extension content scripts versus NopsAI UI code.
- [ui-modal-shell.md](./ui-modal-shell.md): The shared skin every create/edit dialog wears — the title pill, the form canvas, the action bar, the in-dialog control set, and the width and repaint rules a feature must follow.

Existing focused docs:

- [api.md](./api.md): REST API guide.
- [triggering.md](./triggering.md): Local GitHub and generic Git webhook simulation.
- [git-webhook-sources.md](./git-webhook-sources.md): GitLab, Bitbucket, Gitea, and generic source configuration, security, GitOps, payload normalization, path filters, and operations.
- [git-apps.md](./git-apps.md): GitHub App multi-installation management, GitOps schema, git-bot routing, AAA, monitoring, and MCP boundaries.
- [../examples/sso/README.md](../examples/sso/README.md): Runnable SSO example fixtures for local Keycloak and multi-provider IdP scenario testing.
- [../examples/gitops-quickstart/README.md](../examples/gitops-quickstart/README.md): The GitOps sample with a global config repository, a team config repository, and one pipeline per feature area.
- [llm-model-selection.md](./llm-model-selection.md): LLM provider and model notes.
- [mcp-pipeline-integration.md](./mcp-pipeline-integration.md): MCP integration details.
- [mcp-feature-coverage.md](./mcp-feature-coverage.md): Hosted MCP feature coverage, user-scoped permissions, and enterprise automation boundaries.
- [wiki](./wiki): Product-wiki source map for the in-app documentation page, including supported deployment models, GitOps authority, AI/MCP, operations, security boundaries, and confirmed implementation gaps.

Recommended reading order:

1. `architecture-overview.md`
2. `service-reference.md`
3. `access-control.md`
4. `jwt-authentication.md`
5. `runtime-flows.md`
6. `feature-reference.md`
7. `assistant-capabilities.md`
8. `decision-architecture.md`
