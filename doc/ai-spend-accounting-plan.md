# AI Spend Accounting Plan

This document is the migration plan for making NopsAI report AI usage as one
trustworthy number: **money spent, in USD**.

It exists because two things are broken today. Token recording for pipeline runs
has been dead since `18c0d183` (2026-08-14), and the cost columns that every
dashboard reads have never been populated by anything. The result is a monitoring
surface that shows a dozen token counters, all of which are stale, next to a cost
figure that is structurally always `$0.00`.

## Decision

The user sees **one** number for AI usage: cost in USD.

Tokens remain the input to that number and stay queryable for engineers through
Prometheus and the API, but they stop being a product surface. A person looking
at a run, a pipeline, a team, or an assistant conversation should be able to
answer "what did this cost me" without reading a token counter, and without
knowing what a prompt token is.

For that number to be worth showing, three properties must hold:

1. **It is recorded.** Every LLM call on every path writes a usage event.
2. **The token counts behind it are exact.** Provider-reported counts, with every
   billable component included — not a `len(text)/4` estimate, and not a subset
   of the tokens the provider actually charges for.
3. **It is complete or it says so.** A model with no price cannot silently
   contribute `$0.00`. Unable-to-price is not the same as free.

## Current State

### Recording is broken

Commit `18c0d183` renamed the GitOps concept "LLM profile" to "model". A global
search-and-replace of `llm_profile` to `model` reached SQL identifiers:

| Site | Damage |
| --- | --- |
| `services/nopsai/ai_usage_handlers.go:124` | `INSERT` names `model` twice. PostgreSQL rejects with `42701 column "model" specified more than once`. Every usage event and the run summary update roll back together. |
| `services/nopsai/monitoring_analytics_schema.go:21` | `CREATE TABLE` declares `model` twice. Dormant where the table already exists; fatal on a fresh install that does not run `db/init.sql`. |
| `services/nopsai/metrics.go:1439` | `SELECT feature, provider, model, model` is legal SQL, so no error. The Prometheus `model` label silently carries the provider model instead of the profile. |

Both write paths discard the error — `pipeline_final_outputs.go:1529` uses
`_ = a.recordAIUsage(...)` and the agent reporter logs at warn level — so the
failure produced no signal for six days.

Confirmed against the live database: 498 rows, newest `2026-08-06`.

### Token counts are incomplete

| Provider | Issue |
| --- | --- |
| Gemini | `candidatesTokenCount` is mapped to completion tokens. It excludes `thoughtsTokenCount`, which is billed at the output rate. On thinking models the output figure is short by the entire reasoning budget, and `prompt + completion != total`. |
| Anthropic | Only `input_tokens` and `output_tokens` are read. `cache_creation_input_tokens` and `cache_read_input_tokens` are reported separately and are *not* included in `input_tokens`. Harmless today because no request sets `cache_control`, but the runtime already carries prompt-cache plumbing that would report near-zero input the moment caching is enabled. |
| OpenAI-compatible | Correct. `completion_tokens` already includes reasoning tokens; `cached_tokens` is a subset of `prompt_tokens`. |

The `len(runes)/4` fallback estimator exists in three copies. It only fires when a
provider omits usage, and it is honestly flagged through to
`metadata->>'estimated_tokens'`, so it stays — but it must never feed a dollar
figure.

### Nothing computes cost

`InputCostUSD`, `OutputCostUSD` and `TotalCostUSD` are plumbed through the agent,
the API, the database and the UI. No code assigns them. There is no pricing table
anywhere in the repository. `pipeline_run_usage_summary.ai_cost_usd` and
`runner_cost_usd` are both permanently zero, which means `total_cost_usd` is too.

### The UI shows tokens, not money

`MonitoringDashboard.tsx` alone renders token figures in fourteen places — "LLM
tokens", "Trigger tokens", "Total tokens", "Exact tokens", "Estimated tokens",
"Token trend", plus seven token breakdown panels and a "Top Token Runs" list.
Run detail, the run list and the assistant panel each surface their own token
counters.

## Target Design

### Pricing is a property of a model, declared in Git

Pricing goes on the existing `models/<name>.yaml` GitOps resource, next to the
provider and credential it belongs to:

```yaml
provider: gemini
model: gemini-2.5-flash
credential_ref: credential://system/llm/gemini
pricing:
  input_per_million_usd: 0.30
  output_per_million_usd: 2.50
  cached_input_per_million_usd: 0.075
  cache_write_per_million_usd: 0.3750
```

This keeps every configuration resource Git-owned end to end, and it gives a
model's price exactly one place to change. Two model resources may point at the
same provider model with different prices; that is intentional and covers
different contracts, regions and negotiated rates.

`cached_input_per_million_usd` and `cache_write_per_million_usd` are optional and
default to the input rate, which is the correct behaviour for a provider that
does not discount cache reads.

### Pricing is required where a model is defined, not where one is used

`parseGitOpsModels` rejects a model with no `pricing` block, as does the profile
API. A local provider declares its rates as `0`, which states "this is free" — a
claim the operator makes deliberately, distinct from "nobody told us".

The check deliberately does not sit in `validateLLMProfileDefinition`, which
also runs when a profile is resolved in order to *use* it. Refusing to run a
pipeline because a rate card is missing would hold delivery hostage to an
accounting gap. A model that predates this requirement still runs; its usage
records with a NULL cost and the dashboards report it as unpriced.

This is a breaking change for existing configuration repositories. NopsAI is
pre-validation, so it ships as a hard requirement rather than a warned-on
default; a silently unpriced model would reintroduce exactly the `$0.00` problem
this plan exists to remove.

### Cost is frozen at record time

`recordAIUsage` resolves the model's pricing and computes
`input_cost_usd` / `output_cost_usd` / `total_cost_usd` before the insert. Cost is
stored, never recomputed on read.

A price change is not retroactive. Last month's spend is what it cost last month,
and a dashboard must not rewrite history because a rate card moved.

### Assistant turns are priced per call

A turn can mix models — the planner and the synthesis step need not share a
profile — so each LLM call is priced on its own. Summing a turn's tokens before
pricing them would charge every call at one model's rate.

### Estimated tokens never produce a dollar figure

An event whose tokens came from the `len/4` estimator records tokens as it does
now and cost as `NULL` — not `0`. The API reports those separately as
"unpriced calls", and the UI shows the money number with an explicit
incompleteness marker rather than folding a guess into the total.

### The UI shows one number

Every AI usage surface reduces to spend in USD:

| Surface | Before | After |
| --- | --- | --- |
| Monitoring overview | "LLM tokens" | "AI spend" |
| Monitoring AI usage tab | 6 token cards, 7 token breakdowns, top-token runs | 1 spend card, the same breakdowns valued in USD, most-expensive runs |
| Trigger analytics | "Trigger tokens" | "Trigger spend" |
| Efficiency tab | "LLM tokens", tokens by pipeline/team/step | "AI spend", spend by pipeline/team/step |
| Run detail / run list | token count + `$0.00` | spend |
| Assistant panel | per-message tokens | per-conversation spend |

Token fields are removed from the API response bodies rather than kept as
deprecated aliases. Engineers keep token-level detail through
`nopsai_ai_tokens_total` in Prometheus and through `ai_usage_events` directly.

## Work Plan

### Phase 1 — Restore recording

1. Fix the duplicate `model` column in the `INSERT` (`ai_usage_handlers.go`).
2. Fix the duplicate `model` column in the `CREATE TABLE` (`monitoring_analytics_schema.go`).
3. Fix the `SELECT`/`GROUP BY`/`ORDER BY` in `metrics.go` so the `model` label carries the profile.
4. Stop discarding the error from both `recordAIUsage` call sites; failure to record spend is an error, not a debug detail.
5. Add a test that parses every SQL literal in the tree and fails on a repeated column name. A live-PostgreSQL test would have needed a container dependency for one assertion; the static check needs none, and it covers every table rather than the one that broke. String-matching tests did not catch this and will not catch the next one.

### Phase 2 — Make the token counts exact

6. Gemini: parse `thoughtsTokenCount`, `cachedContentTokenCount` and `toolUsePromptTokenCount`; completion becomes `candidatesTokenCount + thoughtsTokenCount`.
7. Anthropic: parse `cache_creation_input_tokens` and `cache_read_input_tokens`; prompt becomes `input_tokens + cache_read + cache_creation`, with the cache components retained separately for pricing.
8. Apply both to the second implementation in `pkg/llmclient/client.go`.

### Phase 3 — Compute money

9. Add `LLMPricing` to `config.LLMProfile` and to `llmProfileForm`.
10. Require it in `validateLLMProfileDefinition`; update the quickstart examples, the setup wizard seed and the docs.
11. Compute cost in `recordAIUsage`, pricing cache reads and cache writes at their own rates.
12. Record `analysis_evaluation` usage, which currently returns tokens to the caller and persists nothing.
13. Price assistant conversations from the per-message usage already stored on `assistant_messages`.

### Phase 4 — One number

14. Reduce `monitoringAIUsageResponse` to spend plus the unpriced-call count; value every breakdown in USD.
15. Rework the monitoring dashboard, run detail, run list and assistant panel to the table above.
16. Update `doc/dashboards.md`, `doc/api.md` and `doc/llm-model-selection.md`.

## Out Of Scope

`runner_cost_usd` is uncomputed for the same reason `ai_cost_usd` was, and
`total_cost_usd` therefore reports AI spend alone. Runner pricing needs a rate
model for self-hosted, Kubernetes and Docker runners and is its own piece of
work. Until it lands, the user-facing number is labelled as AI spend, not total
spend, so it never overstates its own coverage.

Replacing the `len/4` estimator with a real tokenizer is also deferred. Once
Phase 3 records unpriced calls explicitly, an estimated call is visible rather
than silently folded into a total, which removes the urgency.
