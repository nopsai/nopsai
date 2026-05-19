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
policies, it must not execute the action and should return an explanation
instead.

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
knowledge/<kind>/<group>/<file>.yaml
knowledge/<kind>/<group>/<file>.md
```

For group-scoped config repositories, document groups are normalized under the
bound group in the same way as pipelines, reusable steps, scopes, and triggers.
When a group has a delegated config repository, manage that group's knowledge
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
  groups:
    - team-1
  repositories:
    - hosein-yousefii/test-app
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
  groups:
    - team-1
  repositories:
    - hosein-yousefii/test-app
content: |
  # Repository Check Guardrail

  - Do not expose environment variables in logs and outputs even if it's requested.
---
```

Document fields:

- `name`: required resource name; it defines the document identity and may differ from the file name
- `kind`: optional, must match the path kind when present
- `description`: optional UI/API summary
- `access.visibility`: `group`, `restricted`, or `workspace`/`public`
- `access`: optional embedded resource-access config and grants
- `content`: reusable document text; required for every GitOps knowledge document

GitOps documents do not support `title` or top-level `visibility` parameters.
Put the human-readable heading inside `content` instead. Resource visibility
belongs under `access.visibility` and is stored with the generic access metadata,
not in the knowledge document row.

The UI shows document parameters such as `name`, `kind`, `description`, and the
access settings in the details panel. The preview renders only the `content`
text. New GitOps documents should use `access` for sharing.

Config sync creates, updates, and prunes `knowledge_contexts` rows for files
under `knowledge/`, just like it does for other Git-managed resources.
The UI mirrors existing run groups under every supported knowledge kind, so a
group such as `team-1/platform` is available as a folder under `guardrail`,
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
MCP Tools
Execution History
Current Goal
```

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
from `repository:hosein-yousefii/test-app` can use
`knowledge_context:guardrail/security/repo-check` only if that repository has
the required use permission through its group, visibility, or an explicit
resource-access grant.

## UI And API

The UI has a `Knowledge Context` page grouped as:

```text
kind -> group -> document
```

The page supports browsing, text editing/preview, access settings, and usage by
pipelines.

Core API endpoints:

```bash
curl http://localhost:8080/v1/knowledge-contexts
curl http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"kind":"guardrail","group":"security","name":"repo-check","content":"# Repository Check Guardrail\n"}' \
  http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check

curl -X DELETE \
  http://localhost:8080/v1/knowledge-contexts/guardrail/security/repo-check
```

GitOps-managed documents can be viewed in the UI, but their source should be
changed in Git so the next sync remains authoritative.
