# LLM Profiles

Nopsai uses named LLM profiles for all model selection. Pipelines do not set raw
provider credentials or model configuration directly; they reference approved
profile names.

## Configuration

Define bootstrap profiles in `config.yml`, manage them from **System -> LLM
Profiles**, or manage them with GitOps from a system config repository at
`setting/system/llm_profile.yaml`. On startup, configured profiles seed the
database when no database profiles exist yet. After that, the database is
authoritative for UI/API changes unless the GitOps file is present; when it is
present, config sync writes that file into the database and refreshes the
running profile registry.

```yaml
llm_default_profile: standard

llm_profiles:
  fast:
    provider: gemini
    model: gemini-2.5-flash
    api_key_secret: GEMINI_API_KEY
    allowed_scopes: ["dev", "test", "prod"]

  reasoning:
    provider: lmstudio
    model: qwen3-coder
    base_url: http://lmstudio:1234
    api_key_secret: LMSTUDIO_API_KEY
    allowed_scopes: ["dev", "internal"]
    thinking: true

  standard:
    provider: gemini
    model: gemini-2.5-pro
    api_key_secret: GEMINI_API_KEY
    allowed_scopes: ["dev", "test", "prod"]
```

Supported providers:

- `gemini`
- `lmstudio`

Profile fields:

- `provider`: provider implementation to use.
- `model`: provider model name. LM Studio may omit this to auto-discover the first loaded model.
- `base_url`: required for LM Studio.
- `api_key_secret`: environment variable name that contains the API key. Gemini profiles require it. LM Studio can omit it when the server does not require auth.
- `allowed_scopes`: scopes where this profile can run. Empty means allowed everywhere.
- `reasoning`: optional LM Studio reasoning level: `off`, `low`, `medium`, `high`, or `on`.
- `thinking`: optional LM Studio shortcut. When `reasoning` is omitted, `thinking: true` maps to reasoning `on` and `thinking: false` maps to reasoning `off`.

The previous single-provider configuration is no longer supported.

## GitOps Configuration

System/global config repositories can define the same registry in Git:

```yaml
default_profile: standard

profiles:
  - name: fast
    provider: gemini
    model: gemini-2.5-flash
    api_key_secret: GEMINI_API_KEY
    allowed_scopes: ["dev", "test"]

  - name: standard
    provider: lmstudio
    model: google/gemma-4-e4b
    base_url: http://lmstudio:1234
    reasoning: off
```

The canonical path is `setting/system/llm_profile.yaml`. The sync path also
accepts `settings/system/llm_profile.yaml` and `.yml` variants. Group-scoped
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
providers.

Deletion rules:

- The active default profile cannot be deleted.
- A profile referenced by pipelines or reusable steps cannot be deleted unless
  the deletion is forced with a migration target.

## Runtime Contract

The control plane loads the active profile registry from the database, validates
it, and packages the full registry into the agent runtime as
`NOPSAI_LLM_PROFILES`. The agent reads that registry and caches LLM clients by
profile name.

There is no fallback to provider-specific environment variables for model
selection. Environment variables are only used when a profile's
`api_key_secret` points to one.
