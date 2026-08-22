# Hosted MCP Conformance Review and Plan

An audit of NopsAI's MCP surface against the Model Context Protocol spec
(`2025-06-18`, the version the server already declares) and against how MCP
clients behave in practice.

The verdict: **the tool catalogue is strong, the protocol implementation is
thin.** 224 permission-filtered tools with real AAA checks is more coverage than
most MCP servers ship. What is missing is the protocol scaffolding that lets a
general client use them well and safely — annotations, pagination, prompts,
resource templates, and honest capability advertisement.

## What is already right

- **JSON-RPC 2.0 shape.** Version checked, `-32700 / -32600 / -32601 / -32602`
  used, notifications answered with `202 Accepted` and no body, which is what the
  spec requires and what most servers get wrong.
- **Tool errors are results, not transport errors.** A failing tool returns
  `isError: true` with content, so the model can read and react to the failure
  instead of the client aborting the turn.
- **`structuredContent` alongside text.** Both are emitted, which is the
  `2025-06-18` shape.
- **Authorization is per call, per resource.** Tools are filtered by the caller's
  AAA subject at `tools/list`, and re-checked with the concrete resource id at
  `tools/call`. A tool the caller cannot use is never listed.
- **The outbound client is well behaved.** `pkg/mcpclient` performs the
  handshake, sends `notifications/initialized`, follows `nextCursor` pagination,
  carries `Mcp-Session-Id` and `MCP-Protocol-Version`, and parses SSE replies.

## Gaps

Ordered by what a real client notices first.

### 1. Tools carry no annotations, title, or output schema — high

Every tool ships `name`, `description`, `inputSchema` only. The spec's optional
fields are the ones clients use to protect the user:

| Field | What a client does with it |
| --- | --- |
| `annotations.readOnlyHint` | Runs it without prompting |
| `annotations.destructiveHint` | Warns, or requires explicit approval |
| `annotations.idempotentHint` | Decides whether a retry is safe |
| `annotations.openWorldHint` | Signals the tool reaches outside the system |
| `title` | Shows a human label instead of `nopsai.propose_secret_gitops_write` |
| `outputSchema` | Validates and renders `structuredContent` |

Without them, a general client treats `nopsai.delete_admin_user` exactly like
`nopsai.get_platform_version`. NopsAI already knows the difference — Phase 4 of
the analysis plan derives capability (read / proposal / mutation) for every tool
— so this is publishing what we compute, not new classification work.

### 2. Capabilities are advertised that are not implemented — high

`initialize` returns `tools.listChanged: true` and `resources.listChanged: true`.
Nothing ever sends `notifications/tools/list_changed`, and the transport is a
single `POST /v1/mcp` with no server→client channel to send it on. A client that
trusts the advertisement will cache a tool list forever and never be told it
changed.

Either implement the notification channel or stop advertising the capability.
Advertising it is the worse half of the two.

### 3. No pagination on `tools/list` — medium

`tools/list` ignores `cursor` and returns all 224 tools in one response, roughly
80 KB. The spec makes pagination optional, but our own client implements the
cursor loop, and a client with a small context budget has no way to ask for less.

### 4. No prompts — medium

`prompts/list` and `prompts/get` are unimplemented and unadvertised. This is the
most valuable missing feature for external clients: the work NopsAI does well —
"analyse this team", "explain why this run failed", "review this pipeline before
I merge it" — is exactly what a prompt entry is for. Today an external client has
to know to call `nopsai.analyze_run` with the right argument; a prompt would hand
it the whole flow.

### 5. No resource templates — medium

Resources are 17 fixed URIs (`nopsai://pipelines`, `nopsai://features`, …).
There is no `resources/templates/list`, so a client cannot read
`nopsai://pipelines/{pipeline_id}` or `nopsai://runs/{run_id}`. Every specific
read has to go through a tool call instead, which is the difference between a
client browsing NopsAI and a client interrogating it.

### 6. Protocol version is not negotiated — low

`initialize` returns a constant and ignores the client's requested
`protocolVersion`. The spec says the server should reply with the requested
version when it supports it, or its own when it does not, and the client decides
whether to continue. The request header `MCP-Protocol-Version` is not read
either.

### 7. No `ping` — low

Utility method used by clients for liveness. Cheap and expected.

### 8. Session and transport shape — low, decide deliberately

The server is stateless: no `Mcp-Session-Id`, no `GET /v1/mcp` SSE stream, no
resumability. For a remote, authenticated, request/response server this is a
defensible choice — it is simpler and horizontally scalable — but it should be a
recorded decision rather than an omission, and it is what makes gap 2 a decision
rather than a bug.

## Plan

### Phase A — Publish what we already know (small, high value)

- [ ] Add `title` and `annotations` to every tool, derived from the routing
      metadata that already exists (`hostedMCPToolRoutingFor`): read tools get
      `readOnlyHint: true`; mutations get `destructiveHint: true` unless the
      action is additive; proposals are read-only because they apply nothing;
      `openWorldHint: false` for everything except the tools that reach a
      configured external MCP server or Git provider.
- [ ] Add `outputSchema` to the analysis tools first, since their result shape is
      already a published contract, then to the list/read tools.
- [ ] Stop advertising `listChanged` until it is emitted.
- [ ] Implement `ping`.
- [ ] Echo the client's `protocolVersion` when supported, and read the
      `MCP-Protocol-Version` request header.

### Phase B — Make the catalogue navigable (medium)

- [ ] Cursor pagination for `tools/list` and `resources/list`, with a default
      page size around 50 and a stable ordering.
- [ ] `resources/templates/list` plus templated reads for the resources that have
      a natural identifier: pipelines, runs, schedules, triggers, dashboards,
      teams, knowledge contexts.
- [ ] `completion/complete` for template arguments, so a client can offer
      pipeline ids rather than asking the user to type one.

### Phase C — Prompts (medium)

- [ ] `prompts/list` and `prompts/get` covering the flows NopsAI is good at:
      review a team, explain a run failure, review a pipeline before merge,
      explain platform spend, prepare a GitOps change. Each prompt names the
      tools it expects to use, so the client's model starts with the right plan.

### Phase D — Decide the transport (needs a decision first)

- [ ] Record whether NopsAI stays request/response or implements Streamable HTTP
      with an SSE channel and `Mcp-Session-Id`.
- [ ] If streaming: emit `notifications/tools/list_changed` when AAA grants or
      the MCP registry change, and re-advertise `listChanged`.
- [ ] If not: document the stateless choice in `mcp-feature-coverage.md` so
      integrators know not to wait for notifications.

## Non-goals

- Sampling (`sampling/createMessage`): NopsAI calls models through its own
  profiles and spend accounting. Borrowing the client's model would move spend
  off the books we report.
- Elicitation: the assistant already asks clarifying questions in its own
  surface, and confirmation for mutations is enforced server-side rather than
  delegated to a client prompt.
- Roots: NopsAI has no filesystem workspace concept to bound.
