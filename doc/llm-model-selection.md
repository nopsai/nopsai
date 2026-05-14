# LLM Model Selection

This document describes a recommended model for letting Nopsai pipelines choose
different LLM models for different execution scopes.

## Recommendation

Use profile-based model selection:

- Define allowed LLM profiles centrally.
- Let pipelines, steps, and tasks reference those profiles by name.
- Resolve the active profile using inheritance.
- Keep provider credentials and raw model IDs under operator control.

This gives pipeline authors useful control without letting every pipeline pick
arbitrary providers, models, costs, or data-handling behavior.

## Why This Helps

Different pipeline work has different model requirements:

- Fast triage, classification, and summarization can use a cheaper model.
- Code review, security checks, and migration analysis may need a stronger model.
- Repository-wide inspection may need a larger-context model.
- Sensitive or production-scoped pipelines may need a restricted approved model.

Per-scope model selection can reduce cost and latency while reserving stronger
models for steps where they materially improve results.

## Configuration Shape

Recommended pipeline YAML:

```yaml
name: repo-review
container_image: ubuntu:latest

llm_profile: standard

steps:
  - name: quick-triage
    llm_profile: fast
    goal: Classify the change risk.

  - name: deep-review
    llm_profile: reasoning
    tasks:
      - name: inspect-code
        goal: Review risky changed files carefully.

      - name: summarize
        llm_profile: fast
        goal: Summarize the findings for the pull request.
```

Suggested model fields:

```go
type Pipeline struct {
    // existing fields...
    LlmProfile string `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
}

type BaseStep struct {
    // existing fields...
    LlmProfile string `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
}

type Task struct {
    // existing fields...
    LlmProfile string `yaml:"llm_profile,omitempty" json:"llm_profile,omitempty"`
}
```

The first implementation can map profile names to Gemini model IDs in service
configuration. A future implementation can expand the profile to include
provider, model, timeout, temperature, context limits, and safety settings.

Example operator configuration:

```yaml
gemini_model: gemini-default

llm_profiles:
  fast:
    provider: gemini
    model: gemini-fast

  standard:
    provider: gemini
    model: gemini-standard

  reasoning:
    provider: gemini
    model: gemini-reasoning
```

## Resolution Rules

Resolve the active profile from most-specific to least-specific:

1. Task `llm_profile`
2. Step `llm_profile`
3. Pipeline `llm_profile`
4. Server default model

For step-level `condition` evaluation, use the step profile. Task profiles should
only apply to the task's own LLM goal execution.

For legacy goal steps, the synthetic task should inherit from the step profile.

For script-only tasks, the profile can be accepted but it has no effect unless
the step condition needs LLM evaluation.

## Runtime Behavior

Recommended execution flow:

1. Load and validate the pipeline.
2. Resolve the default LLM profile from service configuration.
3. For each step, resolve the step LLM profile.
4. Evaluate the step condition, if present, using the step profile.
5. For each task, resolve the task LLM profile.
6. Use the task profile when calling the LLM for goal-to-action selection.
7. Record the resolved profile and model in task logs or run metadata.
8. Preserve existing `llm_content_sharing`, `llm_output_sharing`,
   `llm_content_include`, and `llm_content_ignore` semantics independently from
   model selection.

## Validation

Pipeline validation should reject unknown profile names before the run starts.

Recommended validation rules:

- Pipeline `llm_profile`, if set, must exist in the configured profile registry.
- Step `llm_profile`, if set, must exist in the configured profile registry.
- Task `llm_profile`, if set, must exist in the configured profile registry.
- Empty `llm_profile` means inherit.
- Raw model IDs should not be accepted in pipeline YAML unless explicitly enabled
  by an operator-controlled compatibility mode.

This keeps cost, provider choice, and sensitive data routing auditable.

## Raw Model Field Alternative

A simpler design would add `llm_model` directly to pipelines, steps, and tasks:

```yaml
steps:
  - name: review
    llm_model: gemini-expensive-model
    goal: Review this change.
```

This is easy to implement but weaker operationally:

- Pipeline authors can bypass cost controls.
- Provider migration becomes harder because YAML contains vendor model IDs.
- Auditing and policy enforcement are less clear.
- Future provider-specific settings become scattered across pipeline files.

Use profiles as the default design. Consider raw model IDs only for local
development or explicitly trusted internal pipelines.

## Implementation Plan

Recommended MVP:

1. Add `LlmProfile` to `models.Pipeline`, `models.BaseStep`, and `models.Task`.
2. Add profile name validation to pipeline validation.
3. Add profile resolution helpers in the agent.
4. Change `LLMClient` to accept a model per call or cache clients by model.
5. Pass the resolved model into condition and action-selection LLM calls.
6. Log the resolved profile and provider model for each LLM-backed task.
7. Keep the existing `GEMINI_MODEL` as the default fallback.

The current runtime already passes the full pipeline definition to the agent, so
the agent can resolve per-step and per-task profiles without changing the
dispatcher protocol.

## Open Questions

- Should profile definitions live only in service configuration, or should
  pipelines be allowed to define local aliases from an approved allowlist?
- Should task status metadata store `llm_profile` and `llm_model` alongside
  `llm_duration_ms`?
- Should profile selection support provider-specific timeouts at the same time
  as model IDs?
- Should a child pipeline inherit the parent pipeline's default profile, or use
  only its own pipeline definition and service default?
