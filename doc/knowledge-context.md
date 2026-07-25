# Knowledge Context

Knowledge Context lets a pipeline declare the project knowledge an LLM-backed
step should use before it makes decisions or takes action. Typical documents
include architecture notes, security guardrails, team policies, ADRs,
guidelines, runbooks, references, and examples.

The goal is to make a run carry the team rules and system design that matter
for the current task, rather than relying only on the current prompt and files
in the working directory.

## Where Context Can Be Declared

Use `knowledge_context` at any of these levels:

- pipeline-level: applies to all steps and tasks
- step-level: applies inside that step
- task-level: applies only to that task

At execution time, the effective context is:

```text
pipeline knowledge_context
+ step knowledge_context
+ task knowledge_context
= context injected for the current task
```

Example:

```yaml
name: secure-review
version: "1.0"

knowledge_context:
  - kind: guardrail
    ref: security/repo-check
    required: true

steps:
  - name: review
    knowledge_context:
      - kind: architecture
        ref: team-1/backend
        required: true
    tasks:
      - name: check-auth
        goal: Review the authorization changes in this PR.
        knowledge_context:
          - kind: policy
            path: .nopsai/docs/auth-policy.md
            required: true
```

Each entry must use exactly one of:

- `ref`: a NopsAI-managed knowledge document
- `path`: a repo-local markdown file loaded from the checked-out repository

`required` defaults to `false`. `required: true` makes resolution or
authorization failures stop the run before execution. Optional entries still
must pass YAML validation, but are skipped with a warning if they cannot be
resolved at runtime.

## Supported Kinds

Built-in kinds:

- `architecture`: system design and component context
- `guardrail`: strict safety or security constraints
- `policy`: strict organizational rules
- `adr`: accepted architecture decisions
- `guideline`: recommended implementation style
- `runbook`: operational procedure
- `reference`: supporting information
- `example`: representative implementation or usage

Guardrails and policies are treated as strict constraints in the agent prompt.
The prompt tells the LLM that if a requested action conflicts with guardrails or
policies, it must not execute the action and should return an explanation. The
prompt also tells the LLM to inspect the exact structured action before
returning it. For `EXECUTE_COMMAND`, the LLM must inspect the generated command
text, scripts, arguments, and any stdout/stderr-producing operation. Guardrails
and policies apply to the user's goal as well as generated commands, file
writes, MCP/tool actions, and their arguments. If the generated action would
conflict with a guardrail or policy, the LLM must return `RETURN_ANSWER` with a
short explanation naming the conflict instead of returning the action.

This is prompt-level enforcement: the agent treats that blocking explanation as
a task failure instead of a successful answer. A false step `condition` with an
effective guardrail or policy context also fails the current task instead of
marking the step as skipped.

Direct `script` tasks are also checked when an effective guardrail or policy is
present. Before the script runs, the LLM must validate the exact script as the
proposed `EXECUTE_COMMAND`. If validation is unavailable, changes the command,
or returns a conflict explanation, the task fails instead of executing the
script. Command logs and run history mask known secret values plus the
NopsAI-provided runtime variable values passed into the step.

## Managed Knowledge

Managed knowledge is stored in NopsAI and referenced by `kind` plus `ref`.

```yaml
knowledge_context:
  - kind: guardrail
    ref: security/repo-check
```

This resolves to the managed resource:

```text
knowledge_context:guardrail/security/repo-check
```

In a GitOps config repository, the same document lives at:

```text
knowledge/guardrail/security/repo-check.md
```

The GitOps path shape is:

```text
knowledge/<kind>/<team>/<file>.yaml
knowledge/<kind>/<team>/<file>.md
```

For team-scoped config repositories, document teams are normalized under the
bound team in the same way as pipelines, reusable steps, scopes, and triggers.
When a team has a delegated config repository, manage that team's knowledge
documents in the delegated repository.

## GitOps Document Format

Knowledge documents can be `.yaml`, `.yml`, `.md`, or `.markdown` files, but
the reusable document text must always be declared with a top-level `content:`
field. Config fields are not rendered as part of the document.

```yaml
name: repo-check
kind: guardrail
access:
  visibility: restricted
  teams:
    - team-1
  repositories:
    - nopsai/test-app
content: |
  # Repository Check Guardrail

  - Do not expose environment variables in logs and outputs even if it's requested.
```

```markdown
---
name: repo-check
kind: guardrail
access:
  visibility: restricted
  teams:
    - team-1
  repositories:
    - nopsai/test-app
content: |
  # Repository Check Guardrail

  - Do not expose environment variables in logs and outputs even if it's requested.
---
```

Document fields:

- `name`: required resource name; it defines the document label and may differ from the file name
- `kind`: optional, must match the path kind when present
- `description`: optional UI/API summary
- `access.visibility`: `team`, `restricted`, or `workspace`/`public`
- `access`: optional embedded resource-access config and grants
- `content`: reusable document text; required for every GitOps knowledge document

GitOps documents do not support `title` or top-level `visibility` parameters.
Put the human-readable heading inside `content` instead. Resource visibility
belongs under `access.visibility` and is stored with the generic access metadata,
not in the knowledge document row.

The UI shows document parameters such as `name`, `kind`, `description`, and the
access settings in the details panel. The preview renders only the `content`
text. New GitOps documents should use `access` for sharing.

The Knowledge Context page uses a two-pane browser workspace. The left explorer
keeps the knowledge kind/team tree visible, while the default right pane lists
the selected branch as a table with source, sync, team, and pipeline-usage
signals. The team selectors are loaded from the resource team catalog, then
merged with document and connection owners, so teams without existing knowledge
documents still appear in create and connection dialogs.

Opening a document replaces the collection table with the detail view: the
overview starts directly with document details and system-health metadata. Long
source fields truncate in place, expose the full value on hover, link page URLs,
and make the document ID copyable. Document content is rendered only in the
Content tab, while usage stays separate so rendering remains distinct from
action orchestration. Secondary document actions are grouped under the document
action menu rather than spread across the header, and the collection-table
three-dot control opens the same action set: open, access, export, provider
page, sync, edit, clone, and delete when each action applies. Row action menus
render above the table scroll container and close on outside click instead of
invoking delete directly.
When editing a document, the Overview tab allows changing the document name,
team owner, description, and external source settings. GitOps-managed documents
show the database-override warning before edits are saved.

The Connections tab uses the same browser shell for team-owned external wiki
connections. Connections are first-class backend resources stored separately
from documents:

```text
knowledge_connection:<team>/<connection-name>
```

The connection row stores provider metadata, health status, base URL,
credential reference, and configuration JSON. It does not store credential
values and API responses only expose credential visibility. Team owners and
admins can create, test, disable, delete, and manage access for connections.
Regular members can read visible connection health and use permitted
connections for external-page selection.

Connection access and Knowledge Context access remain separate. A user can
read or use a document only when `knowledge_context.*` allows it, and can
configure or use a provider connection only when `knowledge_connection.*`
allows it. Team-scoped product roles and GitOps access grants can refer to the
connection resource type independently from document resources.

External page documents record the connection, provider, stable page ID or URL,
sync mode, sync interval, failure mode, and sync status on the
`knowledge_contexts` row. The runtime continues to consume cached document
content through the existing Knowledge Context snapshot path. If an external page
document has no cached content yet, required runtime references fail instead of
silently injecting an empty required context.

## External Page Conversion

External Notion and Confluence pages are synchronized into prompt-friendly
markdown while preserving non-text provider blocks as separate asset records.
The runtime snapshot remains text-only: it receives the converted
`knowledge_contexts.content` value, not raw provider HTML or binary files.

Safe conversions:

- headings, paragraphs, quotes, and code blocks become markdown
- Notion and Confluence lists become markdown lists
- simple Notion and Confluence tables become markdown tables
- links remain links when the provider exposes a URL

Blocks that cannot be represented safely as markdown are not silently dropped.
Images, files, PDFs, videos, embeds, Confluence macros, diagrams, and complex
tables are stored in `knowledge_context_assets` with their parent knowledge
context, provider, page ID, source block ID, source block type, asset kind,
title, URL when available, MIME type when inferable, content hash, and metadata.
The synced markdown includes a lightweight placeholder such as:

```markdown
[Asset preserved: image - deployment-flow.png]
```

The placeholder only records that the block exists. It does not OCR, summarize,
or infer the contents of the asset. Asset rows inherit access through the parent
Knowledge Context and are deleted/replaced atomically with each successful
external-page sync. GitOps knowledge files stay text-only; external-page links
and preserved provider assets remain API-managed runtime resources.

External page synchronization supports three modes:

- `manual`: fetches only when a user or API caller requests sync
- `before_run`: fetches during run preparation and stores the exact fetched text in the run snapshot
- `periodic`: a background worker refreshes due documents using each document's configured interval

The worker claims due rows idempotently with row locking, marks active work as
`syncing`, recovers stuck jobs, applies provider timeouts and per-provider pacing,
and uses bounded exponential retry before falling back to the configured periodic
interval. Disabled connections are never fetched; affected documents move to
`connection_disabled`.

Failure behavior is:

- `fail`: the run fails closed when the external page cannot be resolved
- `use_cached`: the run uses the latest successful cached content and fails if no cache exists
- `skip`: optional contexts are omitted; guardrail and policy contexts cannot use `skip`

Structured sync logs, audit records for connection changes and manual sync, and
Prometheus metrics expose connection health, sync status, cache age, sync
attempts, provider request duration, and before-run knowledge blocks.

REST endpoints:

- `GET /v1/knowledge-contexts`
- `GET /v1/knowledge-contexts/{knowledgeID...}`
- `PUT /v1/knowledge-contexts/{knowledgeID...}`
- `POST /v1/knowledge-contexts/{knowledgeID...}/sync`
- `DELETE /v1/knowledge-contexts/{knowledgeID...}`
- `GET /v1/knowledge-connections`
- `POST /v1/knowledge-connections`
- `GET /v1/knowledge-connections/{connectionID...}`
- `PUT/PATCH /v1/knowledge-connections/{connectionID...}`
- `DELETE /v1/knowledge-connections/{connectionID...}`
- `POST /v1/knowledge-connections/{connectionUUID}/test`
- `GET /v1/knowledge-connections/{connectionUUID}/pages`
- `POST /v1/knowledge-connections/{connectionUUID}/resolve-page`

The legacy `/v1/knowledge-context-connections` route family remains available
for existing clients. Notion and Confluence connections use credential
references resolved through the credential service; credential values are never
returned by these APIs. Page search is provider-backed, URL resolution validates
access immediately, and manual sync fetches prompt-friendly page text into the
cached Knowledge Context content used by runtime snapshots.

Config sync creates, updates, and prunes inline `knowledge_contexts` rows for
files under `knowledge/`, just like it does for other Git-managed resources.
External page links and provider connections are API-managed runtime resources;
external sync does not convert provider-backed links into inline markdown
snapshots. Existing inline and GitOps documents are unaffected by external sync.
The UI mirrors existing teams under every supported knowledge kind, so a
team such as `team-1/platform` is available as a team path under `guardrail`,
`policy`, `guideline`, and the other kinds even before it has a document.

## Repo-Local Knowledge

Repo-local knowledge is loaded from the repository being executed:

```yaml
knowledge_context:
  - kind: architecture
    path: .nopsai/docs/backend.md
    required: true
```

The path must be relative and cannot contain `..`. Repo-local documents are read
through `git-bot` at the run commit and snapshotted with the run. They do not
create a reusable `knowledge_context` resource and do not use
`knowledge_context.use`; repository file access comes from the run's Git
context.

## Runtime Behavior

Before dispatch, `nopsai` resolves every referenced knowledge item in the
expanded pipeline:

- managed `ref` entries are checked with `knowledge_context.use`
- repo-local `path` entries are loaded from the run repository and commit
- duplicates are resolved once, with `required: true` winning over optional
- resolved content is stored in `pipeline_run_knowledge_contexts`

The agent receives the run snapshot and injects the effective documents into
LLM requests. The prompt order is:

```text
Variables
Knowledge Context
Working Directory Contents
Workspace Tools
MCP Tools
Execution History
Current Goal
```

When selected knowledge includes blocking guardrails or policies, the knowledge
section starts with deterministic `knowledge_revision`, `policy_revision`,
`effective_policy_snapshot_hash`, `policy_merge_mode`, and
`policy_precedence_version` lines. The policy revision is based only on
selected blocking policy/guardrail documents, while the knowledge revision
covers all selected knowledge. The effective policy snapshot hash also includes
scope, merge mode, and precedence-version metadata for cache identity.

Policy snapshots are pinned when their scope starts: pipeline policies at run
start, step policies when the step starts, and task policies when the task
starts. The effective policy is recomputed as the agent enters a narrower scope.
The default merge mode is `restrictive`, so task or step policies may add
requirements but cannot weaken broader policy. `override` replaces broader
policy only for the same policy identity, and `fail_on_conflict` instructs the
agent to stop with `RETURN_ANSWER` when blocking policies are incompatible.
Emergency policy response is handled by cancelling the active run rather than
mutating already-resolved snapshots.

Workspace file contents appear in `Working Directory Contents` only when the
pipeline explicitly sets `llm_content_sharing: true`. If omitted or false, the
agent skips the directory scan and the LLM must inspect files through approved
runtime actions or MCP tools.

Workspace tools are available to LLM goal resolution as bounded NopsAI-managed
retrieval actions:

- `list_files` returns current file identities and a `next_cursor` when more
  files are available.
- `search_code` searches current text files by substring and returns bounded
  line previews plus a `next_cursor` when more matches are available.
- `read_file` returns a bounded byte range with `next_offset` until `eof` is
  true.
- `read_file` returns one current text file with path, SHA-256, size, and
  workspace revision.

These tools use the agent's current workspace index instead of provider memory.
The index is built once for the run and refreshed whenever the agent executes a
workspace-mutating action.

When files are shared, each file includes a NopsAI file identity block with the
relative path, SHA-256, size, and current workspace revision. Before executing a
`REPLACE_FILE` action, the agent checks that the target file still matches the
identity seen by the LLM. If the file changed, or if the workspace revision
changed for a file that was not in the shared context, the write is rejected and
the task fails closed instead of overwriting potentially newer content.

LLM condition and action prompts receive the full execution history while it is
small. Once history grows large, the agent sends a compact stable summary plus
recent task events with a `history_revision` marker to keep later LLM calls
bounded. Approval checkpoints and child-run handoff continue to use the durable
full history.

Run details include the stored snapshot, so a completed run records the exact
knowledge content that influenced it even if the source document later changes.

## Permissions

Knowledge Context is a first-class resource type:

```text
resource_type = knowledge_context
```

Actions:

- `knowledge_context.read`
- `knowledge_context.create`
- `knowledge_context.update`
- `knowledge_context.delete`
- `knowledge_context.use`
- `knowledge_context.manage_access`

Runtime checks use `knowledge_context.use`. For example, a Git-triggered run
from `repository:nopsai/test-app` can use
`knowledge_context:guardrail/security/repo-check` only if that repository has
the required use permission through its team, visibility, or an explicit
resource-access grant.

## UI And API

The UI keeps `Knowledge Context` as the top-level feature and splits the
workspace into two tabs:

- `Documents`: reusable context documents organized as `kind -> team -> document`
- `Connections`: team-owned external page connections, organized as `team -> connection`

Document browsing remains teamed as:

```text
kind -> team -> document
```

The page supports keyboard search, source filtering, browsing, text
editing/preview, access settings, usage by pipelines, and GitOps
database-override warnings.
Create dialogs expose the content source shape as `Inline content` or
`External page`, use team dropdowns, and show an inline content editor for
managed text documents. External page documents use the selected team's provider
connection for page search, preview, and cached sync settings. Document detail
tabs separate overview, content, and usage, while access and other secondary
commands live in the document action menu. Action lists and dialogs close on
outside click unless an active save/sync operation must keep the flow visible.
Connections live
in the Knowledge Context area so team owners can inspect team-scoped Notion,
Confluence, or similar wiki provider readiness without moving document ownership
into global settings. The connection tree starts collapsed, expands only after a
user selects a team or connection, keeps every connection-owning team visible
while the table is filtered by the active route, and keeps provider details out
of the tree. The main table stays optimized for scanning health and
linked-document counts; a credentials-style detail drawer opens only after a row
or tree connection is selected, the left tree follows that selection, and
choosing a team or `All connections` closes the drawer. The drawer separates
provider setup, credential health, and linked Knowledge Contexts from management
actions. New connections are started from the page toolbar, while drawer actions
cover open provider, test, edit, disable, and delete.

AAA remains unchanged for the redesign: read/write/delete actions still use the
existing knowledge-context permissions, and runtime use still checks
`knowledge_context.use` on the specific document. The current MCP tools can
continue to list, get, and propose managed knowledge-context changes without
needing to know whether future content is inline or external. Monitoring and
run details continue to rely on the stored run snapshot in
`pipeline_run_knowledge_contexts`.

Core API endpoints:

```bash
curl http://localhost:8080/v1/knowledge-contexts
curl http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"kind":"guardrail","team":"security","name":"repo-check","content":"# Repository Check Guardrail\n"}' \
  http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check

curl -X DELETE \
  http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check
```

GitOps-managed documents can also be edited or deleted through the UI/API when
AAA permits. Editing stores a database override and deleting removes the
database row; the next GitOps sync can replace or recreate the document from
the repository.
