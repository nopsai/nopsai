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
- `credential_ref`: stable reference to an API key in System > Credentials.
  Hosted providers require it. LM Studio and Ollama can omit it when the
  endpoint does not require authentication.
- `allowed_scopes`: scopes where this profile can run. Empty means allowed everywhere.
- `reasoning`: optional LM Studio reasoning level: `off`, `low`, `medium`, `high`, or `on`.
- `thinking`: optional LM Studio shortcut. When `reasoning` is omitted, `thinking: true` maps to reasoning `on` and `thinking: false` maps to reasoning `off`.
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
| LM Studio | `max_output_tokens` | `0` to `1` | Yes, through the native chat API. |
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
    reasoning: off

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

The canonical GitOps path is `setting/system/llm_profile.yaml`. Group-scoped
config repositories cannot manage system LLM profiles.

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
`goal` tasks or step `condition` values, because those require an LLM.

## Resolution

At runtime Nopsai resolves the selected profile from most-specific to
least-specific:

1. Task `llm_profile`
2. Step `llm_profile`
3. Pipeline `llm_profile`
4. `llm_default_profile`

Step-level conditions use the resolved step profile. Task-level goals use the
resolved task profile. Script-only tasks can declare `llm_profile`, but it only
matters when an LLM-backed operation is performed.

## Validation

Runs are rejected before agent launch when:

- No LLM profiles are configured.
- The default profile does not exist.
- A referenced profile does not exist.
- A selected profile is not allowed for the run scope.
- The selected profile has invalid provider configuration.
- A required API key environment variable is missing.
- Timeout, token, or temperature limits are invalid.

These checks do not apply when the pipeline sets `llm_enabled: false`.

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

- The active default profile cannot be deleted.
- A profile referenced by pipelines or reusable steps cannot be deleted unless
  the deletion is forced with a migration target.

## Runtime Contract

The control plane loads the active profile registry from the database, validates
it, and packages the full registry into the agent runtime as
`NOPSAI_LLM_PROFILES`. The agent reads that registry and caches LLM clients by
profile name.

The Agent Profile catalog is packaged separately as `NOPSAI_AGENT_PROFILES`.
The agent uses it only to build persona text for prompts.

There is no fallback to provider-specific environment variables. NopsAI
resolves `credential_ref` through the encrypted registry and delivers only the
selected run credentials to the agent.
