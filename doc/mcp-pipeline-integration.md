# MCP Pipeline Integration

This document describes the recommended model for integrating Model Context Protocol
(MCP) servers into Nopsai pipelines.

## Recommendation

Use a hybrid design:

- Define MCP servers once at the pipeline level.
- Reference the required MCP servers from each step or task.

The pipeline-level definition acts as a registry of available MCP servers for the
run. Step and task definitions decide which of those servers are exposed during
execution.

This keeps MCP configuration reusable while preserving clear execution scope,
validation, auditability, and least-privilege access.

## Configuration Shape

Example:

```yaml
name: repo-review
container_image: ubuntu:latest

mcp_servers:
  github:
    type: external
    transport: http
    url: ${GITHUB_MCP_URL}
    secrets:
      - github_token

  postgres:
    type: managed
    transport: stdio
    image: ghcr.io/example/postgres-mcp:latest
    env:
      DATABASE_URL: ${DATABASE_URL}

steps:
  - name: inspect-pr
    mcp:
      - github
    goal: Review the pull request and identify risky changes.

  - name: database-checks
    mcp:
      - github
      - postgres
    tasks:
      - name: inspect-migrations
        goal: Check whether this pull request changes database migrations safely.

      - name: summarize-impact
        mcp:
          - github
        goal: Summarize the application impact of the changed files.
```

## Goal Semantics

Adding MCP servers to a step or task does not replace the `goal`.

The `goal` remains the natural-language objective for the agent. The MCP server
list controls which external tools and resources are available while the agent is
trying to satisfy that goal.

For example:

```yaml
steps:
  - name: review-pr
    mcp:
      - github
    goal: Review the pull request and identify risky changes.
```

Runtime meaning:

1. The agent receives the goal: `Review the pull request and identify risky changes.`
2. The runtime resolves the `github` MCP server from `mcp_servers`.
3. The runtime connects to or starts the MCP server.
4. The agent is given access to the tools exposed by that server.
5. The agent decides whether and when to call those tools.
6. Tool results become part of the agent's working context.

MCP should be treated as available capability, not as an instruction to always
call the server.

## Pipeline-Level MCP Servers

Pipeline-level MCP server definitions should describe how to reach or launch each
server.

Suggested model fields:

```go
type Pipeline struct {
    // existing fields...
    MCPServers map[string]MCPServerConfig `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
}

type MCPServerConfig struct {
    Type      string            `yaml:"type" json:"type"`
    Transport string            `yaml:"transport" json:"transport"`
    URL       string            `yaml:"url,omitempty" json:"url,omitempty"`
    Image     string            `yaml:"image,omitempty" json:"image,omitempty"`
    Command   string            `yaml:"command,omitempty" json:"command,omitempty"`
    Args      []string          `yaml:"args,omitempty" json:"args,omitempty"`
    Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
    Secrets   []string          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
}
```

Recommended `type` values:

- `external`: connect to an already-running MCP server.
- `managed`: start a per-run or per-step sidecar container/process.
- `local`: start a command inside the execution runtime.

Recommended `transport` values:

- `http`
- `sse`
- `stdio`

## Step and Task References

Steps and tasks should reference MCP servers by name.

Suggested model fields:

```go
type BaseStep struct {
    // existing fields...
    MCP []string `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}

type Task struct {
    // existing fields...
    MCP []string `yaml:"mcp,omitempty" json:"mcp,omitempty"`
}
```

Task behavior can follow one of two policies:

- Inherit step MCP servers when the task has no `mcp` field.
- Use the task `mcp` list as an explicit override when present.

Start with inheritance plus explicit override. It is simple to explain and gives
operators precise control when a task needs a narrower tool set than the step.

Example:

```yaml
steps:
  - name: analyze
    mcp:
      - github
    tasks:
      - name: inspect-code
        goal: Find files changed in this pull request and summarize the impact.

      - name: inspect-database
        mcp:
          - github
          - postgres
        goal: Check whether the changed migrations are safe.
```

In this example:

- `inspect-code` inherits `github` from the step.
- `inspect-database` explicitly receives `github` and `postgres`.

## Runtime Behavior

The runtime should not assume every pipeline-level MCP server must be started.
Only servers referenced by the current step or task should be resolved.

Recommended execution flow:

1. Load and validate the pipeline.
2. Build an MCP registry from `mcp_servers`.
3. For each step, compute the step MCP scope.
4. For each task, compute the task MCP scope from task override or step inheritance.
5. Resolve each referenced MCP server.
6. Connect to external servers or start managed/local servers.
7. Expose the resulting tools to the agent while it executes the `goal`.
8. Stop managed/local servers when their scope ends.
9. Record MCP server usage in run metadata or logs for auditability.

## Server Startup Options

### External Server

The pipeline points to a server that already exists.

```yaml
mcp_servers:
  github:
    type: external
    transport: http
    url: ${GITHUB_MCP_URL}
```

The runner only connects to this server. It does not start a container.

### Managed Container

The pipeline describes a server that the runtime should start.

```yaml
mcp_servers:
  postgres:
    type: managed
    transport: stdio
    image: ghcr.io/example/postgres-mcp:latest
    env:
      DATABASE_URL: ${DATABASE_URL}
```

The dispatcher or agent starts the container, connects to it, uses it for the
step or task, then stops it.

### Local Command

The pipeline describes a command to spawn inside the execution runtime.

```yaml
mcp_servers:
  filesystem:
    type: local
    transport: stdio
    command: npx
    args:
      - "@modelcontextprotocol/server-filesystem"
      - "/workspace"
```

The runner starts the process, communicates over stdio, and stops it after the
scope ends.

## Validation Rules

Pipeline validation should catch configuration mistakes before a run starts.

Recommended rules:

- MCP server names must be valid identifiers.
- Each `mcp` reference in a step must exist in `pipeline.mcp_servers`.
- Each `mcp` reference in a task must exist in `pipeline.mcp_servers`.
- `external` servers must define a compatible endpoint such as `url`.
- `managed` servers must define an `image` or another supported launch target.
- `local` servers must define a `command`.
- Server names should be unique by construction through the map key.
- Secret names referenced by MCP configs should be resolved through the existing
  secret mechanism.

## Required Versus Available MCP

The initial `mcp` field should mean "available to this step or task."

That is different from "must be called." The agent should decide whether the
available tools are needed to complete the goal.

If stricter behavior is needed later, add a separate field:

```yaml
steps:
  - name: review-pr
    mcp:
      - github
    required_mcp:
      - github
    goal: Review the pull request.
```

Suggested semantics:

- `mcp`: servers available to the agent.
- `required_mcp`: servers that must be reachable before execution starts.

If a required server cannot be reached, the step or task should fail before the
agent begins goal execution.

## Implementation Notes

Recommended first implementation:

1. Add `MCPServers` to `models.Pipeline`.
2. Add `MCP` to `models.BaseStep`.
3. Add `MCP` to `models.Task`.
4. Extend pipeline validation to check MCP references.
5. Store MCP config in pipeline metadata/API responses.
6. Start with external MCP servers only.
7. Add managed containers after the connection and scoping model is stable.

This keeps the first version small while preserving a clean path toward sidecar
containers and local stdio servers.
