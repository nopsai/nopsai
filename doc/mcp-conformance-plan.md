# Hosted MCP Conformance Review and Plan

An audit of NopsAI's MCP surface against the Model Context Protocol spec
(`2025-06-18`, the version the server already declares) and against how MCP
clients behave in practice.

The original verdict was: **the tool catalogue is strong, the protocol
implementation is thin.** 224 permission-filtered tools with real AAA checks is
more coverage than most MCP servers ship; what was missing was the protocol
scaffolding that lets a general client use them well and safely.

Phases A, B, and C below are now implemented. What remains is one open decision:
whether the transport stays request/response or becomes Streamable HTTP.

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

## Gaps found by the audit

Ordered by what a real client notices first. Items 1–7 are addressed in the plan
below; item 8 remains an open decision. They are kept here in full because the
reasoning is what makes the fixes reviewable.

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

### Phase A — Publish what we already know (done)

- [x] `title` and `annotations` on every tool, derived from the routing metadata
      rather than a second hand-maintained list: reads and proposals are
      `readOnlyHint: true` (a proposal applies nothing, however alarming its
      name); deletes, ejects, revokes, cancels, and disables are
      `destructiveHint: true`; `idempotentHint` marks the calls a client may
      safely retry; `openWorldHint` marks the tools that reach a Git provider,
      mail server, or external trigger.

      The hints are advisory and nothing enforces on them. The AAA check at call
      time is what actually stops a tool, and it does not read them.
- [x] `outputSchema` on the analysis tools. It requires only `analysis` and `ok`,
      the two fields every path sets including the failure path, because a schema
      a real response can fail is worse than no schema — clients validate it.
- [x] `listChanged` is no longer advertised. On a request/response transport
      there is no channel to send the notification on, so claiming it told a
      client to wait for something that could never arrive.
- [x] `ping`.
- [x] `initialize` echoes the client's `protocolVersion` when this server speaks
      it (`2025-06-18`, `2025-03-26`, `2024-11-05`) and answers with its own when
      it does not. The `MCP-Protocol-Version` request header is validated and an
      unsupported value is refused with `400` — before the availability checks,
      because a version mismatch is a fact about the request, and answering
      "service unavailable" would send the client debugging the wrong thing.
- [x] `initialize` also returns `instructions` and a real `serverInfo.version`.

### Phase B — Make the catalogue navigable (done)

- [x] Cursor pagination on `tools/list` and `resources/list`, 100 per page. The
      cursor carries the last name of the previous page rather than an index, so
      a catalogue that changes between pages cannot silently skip an entry.
- [x] `resources/templates/list` with templated reads for pipelines, runs,
      schedules, triggers, teams, and analysis
      (`nopsai://analysis/{subject_type}/{subject_id}`). Each templated read runs
      through the same tool path as the equivalent `tools/call`, so the concrete
      resource is authorized and audited identically — the template is not the
      thing being permitted.
- [x] `completion/complete` for both prompt arguments and resource template
      arguments. Candidates come from the same permission-filtered tools a call
      would use, so a completion never reveals that something exists which the
      caller could not read anyway. Matches are prefix-first, then substring,
      de-duplicated and bounded at 100 with `hasMore`.

      A completion is a convenience, so a source it cannot read yields no values
      rather than an error: failing a keystroke is worse than offering nothing to
      pick from.
- [x] Templates for dashboards and knowledge contexts. Collection URIs keep their
      own meaning — `nopsai://dashboards` is still the inventory, and only
      `nopsai://dashboards/{id}` resolves through a template.

### Phase C — Prompts (done)

- [x] `prompts/list` and `prompts/get` for the five flows NopsAI answers well:
      review a team, explain a run failure, review a pipeline, explain platform
      spend, prepare a GitOps change. Each prompt names the tool that does the
      work and states how the answer should be shaped — which is the part a
      general model improvises badly.
- [x] Prompts are filtered by permission: a prompt whose tool the caller may not
      call is not listed, because offering a workflow that will be refused is
      worse than not offering it. A missing required argument is refused with the
      argument named rather than rendered with an empty subject.

### Phase D — Decide the transport (open, needs a product decision)

The server is request/response over `POST /v1/mcp`: no session id, no
server-to-client stream, no resumability. `GET /v1/mcp` already answers `405` with
`Allow: POST`, which is what the spec asks of a server that offers no stream, and
no capability claims otherwise. So today's behaviour is conformant — the question
is whether to add a stream, not whether we are wrong without one.

**What a stream would buy.** Two things, and only two:

1. **Progress on slow calls.** `notifications/progress` needs a stream. A team
   analysis with inventory makes eight internal reads; `run_pipeline` can take
   longer. Today a client waits with no signal until the result lands.
2. **`listChanged` becoming true.** Tool lists change when AAA grants change or
   the MCP registry is edited. With a stream we could tell clients instead of
   asking them to re-list.

**What it would cost.** Session state (`Mcp-Session-Id`) in a service that is
currently horizontally scalable with no affinity; SSE connection lifecycle and
resumability (`Last-Event-ID`); and a second code path through the same
authorization checks, which is the part most likely to grow a bug.

**Recommendation: stay request/response** until a client asks for progress. The
calls that would benefit are the analysis tools, and the right fix for those is
to make them faster or narrower — `include_inventory: false` already exists — not
to stream a spinner. Revisit if long-running mutating flows (`run_pipeline` with
wait semantics) become a common external-client use case.

- [ ] Decide: stay stateless, or implement Streamable HTTP with sessions and SSE.
- [ ] If streaming: emit `notifications/tools/list_changed` on AAA and registry
      changes, and re-advertise `listChanged`.

## Non-goals

- Sampling (`sampling/createMessage`): NopsAI calls models through its own
  profiles and spend accounting. Borrowing the client's model would move spend
  off the books we report.
- Elicitation: the assistant already asks clarifying questions in its own
  surface, and confirmation for mutations is enforced server-side rather than
  delegated to a client prompt.
- Roots: NopsAI has no filesystem workspace concept to bound.
