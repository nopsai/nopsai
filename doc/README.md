# NopsAI Documentation Map

This folder now has a code-grounded documentation set for the current repository shape.

Start here when you want to understand the system from different angles:

- [architecture-overview.md](./architecture-overview.md): The big picture, main components, core data model, and deployment shape.
- [service-reference.md](./service-reference.md): What each service owns, how it talks to the others, and the main source files to read.
- [runtime-flows.md](./runtime-flows.md): Step-by-step execution flows for request authorization, webhooks, manual runs, dispatching, agent execution, child pipelines, cancellation, and config sync.
- [feature-reference.md](./feature-reference.md): Functional capabilities exposed by the codebase and UI.
- [decision-architecture.md](./decision-architecture.md): The main architectural decisions, why they exist, and the tradeoffs they introduce.
- [access-control.md](./access-control.md): Current AAA service, product roles, access grants, route authorization, and audit behavior.
- [jwt-authentication.md](./jwt-authentication.md): User/API JWTs, refresh tokens, dispatcher internal REST JWTs, and dispatcher gRPC service JWTs.
- [knowledge-context.md](./knowledge-context.md): Project knowledge documents for LLM-backed pipeline steps, GitOps layout, runtime snapshots, and access checks.

Existing focused docs:

- [api.md](./api.md): REST API guide.
- [triggering.md](./triggering.md): Local GitHub webhook simulation.
- [llm-model-selection.md](./llm-model-selection.md): LLM provider and model notes.
- [mcp-pipeline-integration.md](./mcp-pipeline-integration.md): MCP integration details.
- [wiki](./wiki): Earlier broad control-plane/data-plane overview.

Recommended reading order:

1. `architecture-overview.md`
2. `service-reference.md`
3. `access-control.md`
4. `jwt-authentication.md`
5. `runtime-flows.md`
6. `feature-reference.md`
7. `decision-architecture.md`
