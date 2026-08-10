# LLM Profiles

Nopsai uses named LLM profiles for all model selection. Pipelines do not set raw
provider credentials or model configuration directly; they reference approved
profile names.

Agent Profiles are separate from LLM Profiles. `agent_profile` selects the role
and instruction text inserted into prompts, while `llm_profile` selects the
provider/model client. See [agent-profiles.md](./agent-profiles.md).

## Configuration

Manage LLM profiles from **System -> LLM Profiles** or with GitOps from a
system config repository at `setting/system/llm_profile.yaml`. When the GitOps
file is present, config sync writes that file into the database and refreshes
the running profile registry. `config.yml` still accepts bootstrap profile
settings for non-GitOps deployments, but the checked-in config intentionally
does not define profiles so GitOps remains the source of truth.
In database-backed deployments, UI saves persist the registry to the database;
syncing those values back to `config.yml` is a best-effort bootstrap mirror.

Supported providers:

- `gemini`
- `lmstudio`
- `openai` (displayed as OpenAI / ChatGPT)
- `anthropic`
- `groq`
- `mistral`
- `ollama`
- `openrouter`
- `azure-openai`

`openai`, `groq`, `mistral`, `openrouter`, and `ollama` share the
OpenAI-compatible Chat Completions adapter. Anthropic uses its native Messages
API. Gemini and LM Studio keep their native adapters, including LM Studio model
discovery and serialized model loading.

Profile fields:

- `provider`: provider implementation to use.
- `model`: provider model name. LM Studio may omit this to auto-discover the first loaded model.
- `base_url`: required for LM Studio, Ollama, and Azure OpenAI. Hosted providers
  use provider defaults when it is omitted.
- `credential_ref`: stable reference to an API key in Credentials.
  Hosted providers require it. LM Studio and Ollama can omit it when the
  endpoint does not require authentication.
- `allowed_scopes`: scopes where this profile can run. Empty means allowed everywhere.
- `reasoning`: optional LM Studio reasoning level: `off`, `low`, `medium`,
  `high`, or `on`. Leave it omitted for LM Studio models that do not expose
  reasoning configuration. Nopsai also omits `off` on the wire so existing
  profiles can run against non-reasoning LM Studio models.
- `thinking`: optional LM Studio shortcut. When `reasoning` is omitted,
  `thinking: true` maps to reasoning `on` and `thinking: false` maps to
  reasoning `off`; `off` is omitted from LM Studio requests.
- `timeout_seconds`: optional HTTP timeout for provider requests.
- `max_tokens`: optional completion token limit supported by every built-in
  provider. OpenAI-compatible, Azure OpenAI, and Anthropic adapters default to
  `2048`; Gemini and LM Studio use the provider/model default when omitted.
- `temperature`: optional sampling temperature. It is omitted from provider
  requests when unset so the selected model can apply its own default.
- `extra`: provider-specific string options without expanding the top-level
  profile schema.

### Generation option compatibility

The profile schema stays consistent for GitOps, but provider adapters translate
the fields to the correct wire format:

| Provider | `max_tokens` | `temperature` | Generic `reasoning` / `thinking` |
| --- | --- | --- | --- |
| Gemini | `generationConfig.maxOutputTokens` | `generationConfig.temperature`; effective limits are model-specific | No. Gemini thinking uses model-specific `thinkingBudget` or `thinkingLevel` controls. |
| LM Studio | `max_output_tokens` | `0` to `1` | Yes, through the native chat API when the selected model exposes reasoning configuration. `off` is accepted in profiles but omitted from requests for broad model compatibility. |
| OpenAI | `max_completion_tokens` | `0` to `2`; some reasoning models reject it | No. Reasoning settings are model-specific. |
| Anthropic | `max_tokens` | `0` to `1` | No. Extended thinking requires Anthropic's model-specific thinking configuration. |
| Groq | `max_completion_tokens` | `0` to `2`; model support varies | No. Reasoning settings are model-specific. |
| Mistral | `max_tokens` | Supported; conservative values such as `0` to `0.7` are recommended | No. Reasoning settings are model-specific. |
| OpenRouter | `max_completion_tokens` | `0` to `2`; downstream model support varies | No. Reasoning settings are routed-model-specific. |
| Ollama | `max_tokens` through the OpenAI-compatible API | Support depends on the Ollama version and local model | No generic Nopsai mapping. |
| Azure OpenAI | `max_completion_tokens` | Deployment-specific; reasoning deployments may reject it | No. Reasoning settings are deployment/model-specific. |

`timeout_seconds` is a client-side HTTP timeout and works for every provider.
The control plane and agent both reject generic `reasoning` or `thinking`
values outside LM Studio instead of silently ignoring stale API or GitOps
configuration. Provider- and model-specific reasoning controls can be added as
dedicated options without changing this generic contract.

Official API references:

- [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create/)
- [OpenAI reasoning models](https://developers.openai.com/api/docs/guides/reasoning)
- [Azure OpenAI reasoning models](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/reasoning)
- [Anthropic Messages API](https://docs.anthropic.com/en/api/messages)
- [Anthropic extended thinking](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking)
- [Gemini content generation](https://ai.google.dev/api/generate-content)
- [Gemini thinking](https://ai.google.dev/gemini-api/docs/thinking)
- [LM Studio native chat](https://lmstudio.ai/docs/developer/rest/chat)
- [Groq API reference](https://console.groq.com/docs/api-reference)
- [Mistral chat API](https://docs.mistral.ai/api/endpoint/chat)
- [OpenRouter parameters](https://openrouter.ai/docs/api/reference/parameters)
- [Ollama OpenAI compatibility](https://docs.ollama.com/api/openai-compatibility)

Recognized `extra` options:

- OpenAI: `organization`, `project`
- Anthropic: `anthropic_version`
- OpenRouter: `http_referer`, `x_title`
- Azure OpenAI: `deployment`, `api_version`

The previous single-provider configuration is no longer supported.

## GitOps Configuration

System/global config repositories can define the same registry in Git:

```yaml
default_profile: standard

profiles:
  - name: fast
    provider: gemini
    model: gemini-2.5-flash
    credential_ref: credential://system/llm/gemini-fast
    allowed_scopes: ["dev", "test"]

  - name: standard
    provider: lmstudio
    model: google/gemma-4-e4b
    base_url: http://lmstudio:1234

  - name: hosted
    provider: openai
    model: gpt-4.1-mini
    credential_ref: credential://system/llm/openai-hosted
    timeout_seconds: 60
    max_tokens: 4096

  - name: claude-review
    provider: anthropic
    model: claude-sonnet-4-6
    credential_ref: credential://system/llm/anthropic-review
    max_tokens: 4096

  - name: local-ollama
    provider: ollama
    model: qwen2.5-coder:14b
    base_url: http://ollama:11434/v1
    # Optional for authenticated self-hosted gateways:
    # credential_ref: credential://system/llm/ollama-gateway

  - name: azure
    provider: azure-openai
    model: nopsai-gpt41-mini
    base_url: https://my-resource.openai.azure.com/openai/v1
    credential_ref: credential://system/llm/azure-primary
```

Azure OpenAI uses the current `/openai/v1` API when the base URL points to that
path. Existing deployments can use the legacy route by setting
`extra.deployment` and `extra.api_version`; the adapter then calls
`/openai/deployments/{deployment}/chat/completions`.

The canonical GitOps path is `setting/system/llm_profile.yaml`. Team-scoped
config repositories cannot manage system LLM profiles.

Team-scoped LLM profile storage and REST APIs are available at
`GET|PUT /v1/teams/{teamID}/llm-profiles` and
`PUT /v1/teams/{teamID}/llm-profiles/default`,
`PUT|DELETE /v1/teams/{teamID}/llm-profiles/{profileName}` for callers with
`team.read` or `team.update` on the team resource. The **LLM Profiles** page
loads those team-owned rows when a concrete team is selected in the profile
tree or `?team=` query, and the top default selector updates that team's
LLM default. The selected-team view also includes system-catalog
profiles whose slash-scoped names belong to that team, such as
`platform/ml/chatgpt`; those profiles can be selected as the team default
without copying them into the team-local table. The Teams area summarizes these
profiles and its **Defaults** tab lets users with `team.update` select the team
LLM default alongside the Agent and knowledge-kind defaults.

Config repositories manage team defaults in a separate team defaults file:

```yaml
# config-repositories/teams/platform/defaults.yaml
llm_profile: release-review
knowledge_context:
  guardrail: runtime-output-safety
  runbook: release-checklist
```

`llm_profile`, `agent_profile`, and `knowledge_context` are the team GitOps
shape for runtime defaults in `defaults.yaml`. If a team has no enabled team
config repo, the nearest parent/global config repo exports that team's
`config-repositories/teams/<team>/defaults.yaml`. Team-owned LLM profile
definitions are managed through the team UI/API.

Run preparation and agent launch merge team profiles over the system catalog
when the run belongs to that team.

LLM profiles accept optional `prompt_cache` and `provider_state` feature
preferences. `mode: auto` lets NopsAI use a supported provider optimization,
`mode: disabled` keeps the request stateless, and `mode: required` fails closed
when the selected provider adapter cannot satisfy the feature. NopsAI still
owns the logical session, scoped context, transcript, governance contract, and
cache identity; provider caches and continuation state are treated as
replaceable transport optimizations.

## Pipeline Usage

Pipelines, steps, and tasks can select a profile with `llm_profile`.

```yaml
name: repo-review
llm_profile: reasoning

steps:
  - name: quick
    llm_profile: fast
    goal: Summarize the change.

  - name: deep
    goal: Review carefully.
    tasks:
      - name: inspect
        goal: Inspect risky files.

      - name: summary
        llm_profile: fast
        goal: Summarize the findings.
```

Script-only pipelines can opt out of AI entirely:

```yaml
name: shell-check
llm_enabled: false
steps:
  - name: test
    script: go test ./...
```

When `llm_enabled: false` is set, LLM and MCP profile validation is skipped and
no LLM profile registry is required at runtime. Agent Profile references are
still schema-validated as pipeline/step metadata. The pipeline cannot define
`goal` tasks, step `condition` values, final outputs, or direct scripts with
blocking guardrail/policy Knowledge Context, because those require an LLM.

## Resolution

At runtime Nopsai resolves the selected profile from most-specific to
least-specific:

1. Task `llm_profile`
2. Step `llm_profile`
3. Pipeline `llm_profile`
4. Owning team LLM default
5. System/global default profile

For team-owned pipeline runs, the owning-team default is configured through the
Teams **Defaults** tab or `llm_profile` in the team's `defaults.yaml` GitOps
file.
If the team has no default, runtime validation and launch inherit the
system/global default profile; they never borrow a viewer preference or another
team's default. Team overview shows the configured default and links to the
team-scoped profile page, where users with
`team.update` on that team can change the default from the top selector. The
Teams workspace also has a **Defaults** tab where the same users can select the
team LLM default directly.

Step-level conditions use the resolved step profile. Task-level goals use the
resolved task profile. Script-only tasks can declare `llm_profile`, but it only
matters when an LLM-backed operation is performed.

The Nopsai AI Assistant uses the profile selected on the conversation. If no
conversation profile is selected, it uses `llm_default_profile`. The same
`allowed_scopes`, credential refs, provider options, timeout, token, and
temperature validation apply. The assistant UI reads profile choices from
`GET /v1/assistant/llm-profiles`, which returns only picker-safe metadata and
does not require system LLM profile administration permission. Assistant
replies are synthesized from the user request, conversation memory, and hosted
MCP tool outputs. Assistant conversation turns do not use static
normal-language routing. If the selected profile cannot be used for planning,
no hosted MCP tools run and the assistant reports that no changes were applied.
When a plan is validated, the assistant stores a user-visible execution-plan
activity before running MCP evidence calls. That activity labels steps as MCP,
docs, knowledge context, GitOps proposal, or LLM analysis, with phase and
confidence metadata, without exposing hidden model chain-of-thought.
For follow-up calculations or estimates, the assistant LLM may answer from
previous same-chat MCP evidence without another tool call, but the answer must
label its data source and confidence so exact MCP-backed facts stay separate
from LLM-derived assumptions.
If final synthesis fails after a validated plan already produced MCP evidence,
the assistant records the fallback reason and returns the deterministic
permission-bound tool summary. Transient upstream gateway responses from LLM
providers are retried once. If the provider still returns an HTML gateway page,
Nopsai strips the markup before storing or showing the fallback reason.

Assistant usage accounting keeps visible text estimates separate from provider
usage. User messages and deterministic replies without an LLM call add to
`content_tokens` only. Assistant planner and final synthesis calls add
provider-reported or estimated prompt, completion, and total tokens to the
selected LLM profile for monitoring and cost analysis.

## Validation

Runs are rejected before agent launch when:

- No LLM profiles are configured.
- The default profile does not exist.
- A referenced profile does not exist.
- A selected profile is not allowed for the run scope.
- The selected profile has invalid provider configuration.
- A required API key environment variable is missing.
- Timeout, token, or temperature limits are invalid.

These checks do not apply when the pipeline sets `llm_enabled: false`; direct
scripts with blocking guardrail/policy Knowledge Context must keep LLM enabled
so NopsAI can validate the exact script before execution.

Example scope error:

```text
LLM profile "reasoning" is not allowed in scope "prod"
```

## UI Management

The **System -> LLM Profiles** page shows:

- Default profile
- Name
- Provider
- Model
- Base URL
- API key secret
- Allowed scopes
- Thinking / reasoning status
- Validation status
- Actions

Admins can create, edit, delete, and test profiles. The **Test Profile** action
sends a tiny prompt (`reply ok`) to catch bad credentials or unreachable
providers. Hover over generation option labels for a one-line explanation of
each setting.

Deletion rules:

- The active system default profile cannot be deleted from the global view, and
  the active team default profile cannot be deleted from that team's scoped
  view.
- A profile referenced by pipelines or reusable steps cannot be deleted unless
  the deletion is forced with a migration target.

## Runtime Contract

The control plane loads the active profile registry from the database, validates
the profiles selected by the resolved pipeline, and packages only those selected
profiles into the agent runtime as `NOPSAI_LLM_PROFILES`. The packaged runtime
default is the profile the pipeline inherits when it does not set a more
specific override. The agent reads that run-scoped registry and caches LLM
clients by profile name.

The Agent Profile catalog is packaged separately as `NOPSAI_AGENT_PROFILES`.
The agent uses it only to build persona text for prompts.

There is no fallback to provider-specific environment variables. NopsAI
resolves `credential_ref` through the encrypted registry and delivers only the
selected run credentials to the agent. GitOps profile files store only
credential references; encrypted credential versions can be managed in
`setting/system/credentials.yaml`.
