# Agent Profiles

Agent Profiles select the AI persona used in LLM prompts. They do not select the
LLM provider/model, MCP tools, knowledge context, secrets, variables, or runtime
permissions. Those controls remain separate enterprise boundaries.

## Default Profile

NopsAI ships with enabled built-in profiles, including `devops-engineer`, `sre`,
and `security-engineer`. The system default starts as `devops-engineer`, but an
operator can change it in **System -> Agent Profiles** or through GitOps. If a
pipeline and step do not specify `agent_profile`, the runtime uses the configured
default.

Built-in profiles are read-only in the UI. Operators can duplicate them into a
custom profile when they need local instructions.

## Pipeline Usage

Pipelines and steps can select an Agent Profile with `agent_profile`.

```yaml
name: release-readiness
agent_profile: release-manager
llm_profile: reasoning

steps:
  - name: reliability-review
    agent_profile: sre
    goal: Review rollout reliability risks.

  - name: deploy
    script: ./deploy.sh
```

Current runtime resolution order:

1. Step `agent_profile`
2. Pipeline `agent_profile`
3. System `default_profile`

Tasks must not define `agent_profile`. Use step-level profiles when different
tasks inside a step should share the same persona, or split tasks into separate
steps when they need different personas.

## GitOps Configuration

System/global config repositories can manage custom profiles at
`setting/system/agent-profiles.yaml`:

```yaml
default_profile: release-manager

agent_profiles:
  - id: release-manager
    display_name: Release Manager
    description: Coordinates release readiness and rollout communication.
    enabled: true
    instructions: |
      Focus on release readiness, change risk, evidence, stakeholder
      communication, rollout sequencing, rollback plans, and concise release
      notes. Keep decisions traceable and operationally safe.
```

`role` is optional. When it is omitted, the agent prompt uses `display_name`; if
that is missing during runtime fallback, it uses the profile ID. Keep `role` only
when the prompt should say something more specific than the profile name.

To change only the default to a built-in profile, the file can be as small as:

```yaml
default_profile: sre
```

Only system/global config repositories may define system Agent Profiles.
Team-scoped Agent Profile storage and REST APIs are available for delegated
ownership, and run preparation/agent launch merge team profiles over the system
catalog when the run belongs to that team. Team config repositories manage
team-owned profiles in root `ai-profiles.yaml`:

```yaml
agent_default_profile: release-reviewer
agent_profiles:
  - id: release-reviewer
    display_name: Release Reviewer
    enabled: true
    instructions: Review release risk, rollback readiness, and audit evidence.
```

## UI And API

The UI manages system profiles under **System -> Agent Profiles** and team
profiles under **Teams -> Team Settings -> AI profiles**.

System routes:

- `GET /v1/system/agent-profiles`
- `POST /v1/system/agent-profiles`
- `POST /v1/system/agent-profiles/validate`
- `PUT /v1/system/agent-profiles/default`
- `GET /v1/system/agent-profiles/{profileID}`
- `PUT /v1/system/agent-profiles/{profileID}`
- `DELETE /v1/system/agent-profiles/{profileID}`
- `GET /v1/system/agent-profiles/{profileID}/usage`

Team-scoped routes:

- `GET /v1/teams/{teamID}/agent-profiles`
- `POST /v1/teams/{teamID}/agent-profiles`
- `PUT /v1/teams/{teamID}/agent-profiles/default`
- `GET /v1/teams/{teamID}/agent-profiles/{profileID}`
- `PUT /v1/teams/{teamID}/agent-profiles/{profileID}`
- `DELETE /v1/teams/{teamID}/agent-profiles/{profileID}`

AAA resources:

- Reads require `system.read` on `system:agent-profiles`.
- Mutations require `system.update` on `system:agent-profiles`.
- Team-scoped reads require `folder.read` on the resolved team folder.
- Team-scoped mutations require `folder.update` on the resolved team folder.

## Runtime Contract

Before launching an agent, NopsAI validates pipeline and step references against
the effective built-in, UI, and GitOps profile catalog. It then sends the
runtime catalog to the agent through `NOPSAI_AGENT_PROFILES`.

The agent resolves the active profile for each condition or goal and prefixes
the LLM prompt with:

```text
You are {role | display_name | id}.

{instructions}
```

LLM profile selection still controls the model client. MCP profiles still
control external tools. Knowledge context still controls additional retrieved
context. Agent Profiles only control the prompt persona and instruction text.
