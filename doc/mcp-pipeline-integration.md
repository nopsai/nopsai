# MCP Pipeline Integration

Nopsai is an MCP client/host during goal-based pipeline execution. It does not
act as an MCP server.

Pipeline authors select approved MCP profiles:

```yaml
name: repo-review
container_image: ubuntu:latest

mcp_profiles:
  - github-readonly

steps:
  - name: review
    goal: Review this pull request
    mcp_profiles:
      - github-pr-review
```

MCP profiles only apply to goals. Explicit `mcp_profiles` on script steps,
script tasks, or include placeholders are rejected during validation.

## Registry Model

MCP servers are configured by admins under System > MCP. A server is an external
endpoint connection:

```yaml
mcp_servers:
  github:
    display_name: GitHub MCP
    enabled: true
    provider: github
    transport: streamable_http
    url: https://api.githubcopilot.com/mcp/x/all/readonly
    auth_type: bearer_token
    credential_ref: credential://system/mcp/github-readonly
    timeout: 30s
```

MCP profiles are approved bundles of server tools:

```yaml
mcp_profiles:
  github-pr-review:
    description: Read-only GitHub PR review tools
    enabled: true
    servers:
      - server: github
        tools:
          - "*"
```

Pipeline YAML must not define arbitrary MCP servers. It can only reference
configured profiles.

## GitOps

System config repositories can manage MCP the same way they manage LLM
profiles. Put the registry in `setting/system/mcp.yaml`:

```yaml
mcp_servers:
  github:
    display_name: GitHub MCP
    enabled: true
    provider: github
    transport: streamable_http
    url: https://api.githubcopilot.com/mcp/x/all/readonly
    auth_type: bearer_token
    credential_ref: credential://system/mcp/github-readonly
    timeout: 30s

mcp_profiles:
  github-pr-review:
    description: Read-only GitHub PR review tools
    enabled: true
    servers:
      - server: github
        tools:
          - "*"
```

Only system/global config repositories may define the MCP registry. Group config
repositories can reference approved `mcp_profiles` in their pipelines, but they
cannot define new MCP servers.

Create the referenced bearer token under **System > Credentials** or sync its
encrypted envelope from `setting/system/credentials.yaml`. GitOps owns the
binding; plaintext remains write-only in the API/UI.

## Inheritance

Inheritance is additive and de-duplicated for goal execution:

```text
pipeline mcp_profiles + step mcp_profiles + task mcp_profiles
```

Task-step `mcp_profiles` act as defaults for goal tasks inside that step.
Pipeline-level defaults and task-step defaults do not make script tasks MCP
enabled.

When MCP profiles resolve for a goal, including pipeline-level defaults, the
agent requires at least one successful MCP tool call before accepting a final
action.

## Runtime Flow

For a goal task, the agent:

1. Resolves the LLM profile.
2. Resolves allowed MCP profiles for the current scope.
3. Exposes selected MCP tools to the LLM as callable actions. A profile tool
   entry of `"*"` means all tools discovered from a configured read-only MCP
   server.
4. Executes requested MCP tool calls against external HTTP MCP servers.
5. Adds tool results back into the goal conversation.
6. Continues until the LLM returns a final Nopsai action.

The internal MCP action shape is:

```json
{
  "action": {
    "type": "CALL_MCP_TOOL",
    "mcp_tool_action": {
      "server": "github",
      "tool": "get_file",
      "arguments": { "path": "README.md" }
    }
  }
}
```

Final actions remain normal Nopsai actions such as `EXECUTE_COMMAND`,
`REPLACE_FILE`, or `RETURN_ANSWER`.

## Supported Scope

Current implementation:

- External HTTP/streamable HTTP MCP servers.
- Admin-managed MCP servers and profiles.
- Bearer-token or no-auth server connections.
- Extra MCP server headers for provider-specific configuration.
- Tool discovery via `initialize` and `tools/list`.
- Tool execution via `tools/call`.
- Read-only profile enforcement by write-like tool-name rejection.
- Scope checks for profiles and servers.
- Runtime logs for selected profiles and called tools.

Future extensions can add stdio servers, sidecars, write approvals, rate limits,
and richer per-tool audit records.
