# Assistant, Analysis, and MCP Coverage Plan

This document is the plan for making the NopsAI assistant do the operator's work
instead of handing them raw data.

It covers three surfaces that fail the same way today: **hosted MCP**, **team
and pipeline analysis**, and the **assistant** that is supposed to tie them
together. Each one returns material the user must interpret themselves, which is
the opposite of the job.

## Decision

When someone asks the assistant "how is my team doing?", "why is this pipeline
slow?", or "what should I fix first?", the answer is a **ranked set of findings
with evidence and a next action** — not a data dump, and not a paragraph of
model prose invented from a JSON blob.

For that to hold, three properties must be true:

1. **The evidence exists as a tool.** If a question is about teams, there is a
   team tool. The assistant never has to guess from adjacent data.
2. **Analysis happens where the data is.** Findings, severities, and scores are
   computed deterministically on the server from permission-filtered evidence,
   and the LLM explains them. The model never derives a health score from raw
   metrics.
3. **A wrong guess is recoverable.** A planner that picks a tool whose schema it
   was not given asks for the schema. It does not end the turn.

## Current State

### MCP: broad, but blind in the middle

220 tools are registered across `hosted_mcp_tools.go`, `hosted_mcp_final_tools.go`,
and `hosted_mcp_dedicated_tools.go`. Coverage of pipelines, runs, secrets,
schedules, triggers, and admin is genuinely good. Two holes matter:

| Hole | Evidence |
| --- | --- |
| **No team tools at all.** Team is the ownership unit of the entire product — nearly every other tool takes `team_path` — yet no tool lists or reads a team. `GET /v1/teams` and `GET /v1/teams/{teamID}` exist (`routes.go:99`, `routes.go:101`) and are reachable only through the generic `nopsai.call_api` escape hatch. | `grep '"nopsai\.[a-z_]*team' hosted_mcp*.go` returns nothing. `hostedMCPFeatureCapabilityCatalog()` has 15 areas and none of them is teams. |
| **"Analysis" tools return raw fan-out.** `nopsai.explain_pipeline_health`, `nopsai.find_optimization_opportunities`, `nopsai.get_pipeline_efficiency`, and `nopsai.compare_pipelines` each issue 3–4 monitoring GETs and return the concatenated payloads under one key per source. | `hostedMCPMonitoringInsightTool()` at `hosted_mcp_final_tools.go:896` — the result map is `{key: raw_api_response}` plus `source_paths`. No finding, no severity, no ranking, no recommendation. |

The second hole is expensive twice over: the model pays for thousands of tokens
of JSON, and the user pays for whatever the model concluded from it.

### Analysis: real, but trapped in the browser

The product already has a serious analysis engine — 2,413 lines in
`services/ui/src/features/analysis/model.ts` — with a defensible contract:
findings carrying category, severity, evidence, affected resources,
recommendations, and confidence; per-category scores; a health score with a
published formula (baseline 100, `critical 25 / high 15 / medium 8 / low 3 /
opportunity 1`, clamped 0–100).

It runs in the browser, over whatever the current page happens to have loaded,
and only inside the Analysis modal. Consequences:

- **The assistant cannot see it.** A user analyses a team, then has to re-type
  the conclusions into the chat for the assistant to work with them.
- **`POST /v1/analysis/evaluate` is a prompt passthrough.** The handler
  (`analysis_evaluation_handlers.go:53`) takes a client-built `prompt` string,
  forwards it to the LLM, records spend, and returns the text. No tools, no MCP
  evidence, no grounding. The server contributes nothing but a model call.
- **Nothing is reusable.** Another surface that wants the same analysis — the
  assistant, a scheduled report, the CLI — has to reimplement it.

### Assistant: one keyword miss from a dead end

The planner is a sound agentic loop (`assistant_llm_planner.go:56`): plan, call
tools, observe, decide whether more evidence is needed, up to 4 iterations and 8
tool calls. Two things undercut it.

**Tool routing is a hand-written keyword table.**
`assistantPlannerSchemaToolNames()` (`assistant_llm_planner.go:477`) is ~270
lines of `if assistantTextHasAny(lower, "schedule", "cron") { add(...) }`. It
decides which of 220 tools get their input schema into the prompt, capped at 18.
Every new tool is invisible to the planner until somebody remembers to edit this
function.

**A routing miss is fatal.** If the planner names a tool whose schema was not
included, `assistantValidatePlannerDecisionUsesSchemaTools()`
(`assistant_llm_planner.go:771`) rejects the decision, the turn ends, and the
user gets *"I could not create a validated NopsAI tool plan for that request."*
The planner was right about the tool and got killed for it. This is the single
most common way a good question fails.

## Phase 1 — Teams and real analysis in MCP

- [x] `nopsai.list_teams` and `nopsai.get_team`, backed by the existing routes
      and the current subject's visibility.
- [x] `nopsai.analyze_team` — deterministic findings over reliability,
      efficiency, cost, security, and delivery evidence for a team window.
- [x] `nopsai.analyze_pipeline` — the same contract for a single pipeline,
      including its recent-run and step-level evidence.
- [x] Shared analysis contract, scored with the **same constants as the UI
      engine** so the modal and the assistant never disagree about a number.
- [x] A "Teams, applications, and delivery analysis" area in the feature-capability catalog.

### The contract

Both tools return one shape:

```
{
  "analysis": "team" | "pipeline",
  "subject":   {type, id, label, path},
  "window":    {from, to, days},
  "health_score": 0-100,
  "score_basis":  {baseline, formula, severity_weights, severity_counts, ...},
  "scores":       [{category, score, finding_count, deduction, basis}],
  "findings":     [{id, category, severity, title, summary,
                    evidence[{label, value, kind}],
                    recommendations[{title, detail}],
                    confidence}],
  "next_actions": [{label, tool, args}],
  "limitations":  [...],
  "data_sources": [...],
  "ok": bool
}
```

`next_actions` is the part that makes the user's job smaller: every analysis
ends by naming the tool call that investigates the top finding, so the assistant
can offer — or take — the next step without another round of guessing.

**Evidence that cannot be read is a limitation, never a clean score.** If a
monitoring source is denied or errors, the analysis records it in `limitations`,
marks `ok:false`, and does not award points for the categories it could not
evaluate. Being unable to see a problem is not the same as not having one.

## Phase 2 — Planner recovery and routing

- [x] **Schema repair.** When a planner decision names a **read** tool outside
      `schema_tools`, re-prompt once with those schemas added instead of ending
      the turn. Bounded to one repair per decision.

      Repair covers reads only. A missing write or proposal schema is usually not
      an omission: mode selection withholds it on purpose, so that "add a secret"
      is not quietly turned into a GitOps proposal and an unconfirmed mutation is
      not handed the tool it needs. Those keep the hard rejection.
- [x] Route team, analysis, and review vocabulary to the new tools.
- [x] Teach the planner prompt to prefer a first-party analysis tool over
      stitching monitoring endpoints together by hand.
- [x] Deterministic reply composer for analysis output, so the answer is a
      professional findings summary even when LLM synthesis is unavailable.
- [x] Surface `next_actions` in the chat as one-click follow-ups, so acting on
      the recommendation does not mean retyping it.
- [x] Trim the planner tool catalogue: descriptions shortened for tools whose
      schema is not shipped, and the `schema_included`/`mutating`/`proposal`
      flags emitted only when true. Across 220+ tools the false flags were most
      of the catalogue's size and none of its information, which paid for the new
      tools and guidance without growing the prompt.

## Phase 3 — One analysis engine

The target is: the server owns analysis; the modal and the assistant both read
it. Most of the way there.

- [x] **Definition rules on the server.** Ported from `model.ts`: unparseable
      YAML, missing description, no steps, embedded secret literals (evidence is
      the line, redacted, never the value), unpinned images, privileged
      execution, risky shell scripts, missing timeout, production without an
      approval gate, self-approval on a production gate, broken and cyclic
      dependencies, no operator-facing output, and over-sequential check chains.
      `nopsai.analyze_pipeline` now reads the stored definition alongside the run
      evidence, so a pipeline nobody has run yet is still reviewable and still
      scores.

      The sequential-checks rule is deliberately stricter than the browser one:
      it stays quiet when the chain contains an approval or a deploy/release
      step, because that ordering is intentional rather than accidental.
- [x] **`POST /v1/analysis/team` and `POST /v1/analysis/pipeline`**, returning
      the same contract the MCP tools return. Concrete paths rather than a
      `{subjectType}` wildcard: a mutating-method route is exempt from route
      authorization only when it is named, and a wildcard would hand that
      exemption to every future `/v1/analysis/*` route.

      The route needs no LLM and is not gated by assistant feature flags. Every
      evidence source is read as the caller through the permission-checked API
      bridge, so the route grants exactly what the caller could read themselves.
- [x] **`POST /v1/analysis/evaluate` is grounded.** The handler now appends the
      deterministic server analysis for the same subject to the client's prompt,
      labelled authoritative where the two disagree, and reports
      `server_grounded` plus `data_sources` in the response so the modal can say
      which it was. The prompt is appended to, never replaced: the strict JSON
      contract the modal parses is unchanged, and evidence that would exceed the
      input limit is dropped whole rather than truncated into invalid JSON.
- [x] **Analysis hands off to chat.** "Ask NopsAI" in the analysis header opens
      the assistant with the analysis page context and an opener naming the score
      and the blocking findings, so going from a finding to a conversation costs
      no retyping.
- [x] **Run analysis on the server.** `nopsai.analyze_run` and
      `POST /v1/analysis/run` classify a failure into a domain — application
      code, application tests, pipeline definition, credential or authorization,
      runner infrastructure, timeout or capacity, approval or policy, AI
      provider, trigger or input — with a confidence, name the first failure
      point (step, task, exit code), and list what changed since the last
      successful run of the same pipeline. Ported from the browser classifier so
      chat and the modal name the same domain for the same run.

      Two boundaries this holds: run logs are read through the same permission
      checks as the run, so a caller without log access loses that one signal and
      keeps everything else; and the runtime-input comparison reports override
      **names only**, because an override value can carry whatever the trigger
      sent.
- [x] **Team inventory on the server.** `nopsai.analyze_team` now reads what the
      team owns as well as what it ran: duplicate resources (a duplicated
      credential is a security finding, a duplicated pipeline an efficiency one),
      near-identical reusable definitions, resources used by one team but owned
      globally, a split between GitOps-managed and database-managed resources,
      disabled or deprecated resources, and hierarchy deeper than it needs to be.
      Inventory alone is enough to score a team that has never run anything.

      Two boundaries: another team's resources never enter this team's analysis,
      and the inherited-ownership rule only fires when there is a team scope to
      inherit into. Inventory costs three extra reads, so `include_inventory`
      can turn it off.
- [ ] **Repoint the Analysis modal at the routes and delete the browser rule
      engine.** The remaining structural debt, now a UI migration rather than a
      port. Two things still stand in the way:

      1. **Per-resource analysis has no server equivalent.** The Teams surface
         analyses a single credential, MCP profile, knowledge context, schedule,
         or trigger, and those rules (`buildIndividualResourceFindings`) are
         still browser-only. Repointing team analysis without them would lose a
         working feature.
      2. **The modal's plumbing reads more than findings.** Tabs, the reuse and
         unused views, and the reviewed-score cache key off the browser result
         shape; a mapping layer from the server contract to `AnalysisResult` has
         to come first.

      Repointing one subject and leaving the other on the browser engine would
      leave two engines answering the same question, which is worse than the
      duplication that exists now.

## Phase 4 — Tool metadata instead of a keyword table

- [x] **Routing is derived, not enumerated.** `hostedMCPToolRoutingFor()` reads a
      tool's domain from its AAA resource type (already curated per tool, with
      name-based overrides where a tool answers a different question than the
      resource it reads — monitoring tools read runs, but nobody asking about
      them is asking about a run), its capability from the action and schema, and
      its terms from its own name and description.
- [x] **Schema inclusion is scored from that metadata**, so a tool registered
      today is routable today. A test registers a pipeline tool that exists
      nowhere in the planner and asserts it is selected.
- [x] **`nopsai.find_tools`** searches the caller's own permission-filtered
      catalogue by query or domain and returns input schemas, so a planner that
      needs a tool it was not given can ask instead of guessing arguments.
- [x] **The hand-maintained branches are gone.** What replaced them is 6 lines of
      scoring plus ~25 lines of *intent rules* — the cases where relevance is not
      the question: an exposure question must not be answered by reading a value,
      a "what should this look like" must be answered from current docs, a "which
      is slowest" needs step granularity, and a policy question needs capability
      evidence.

### What this changed about correctness

Ranking cannot express mode selection. A request to set a value must not be
offered a GitOps proposal, a GitOps request must not be offered a runtime write,
and a read must be offered neither — so `assistantPlannerModePolicy` **excludes**
those classes rather than down-ranking them.

Two latent bugs surfaced while making that true:

- `addLexicalSchemaToolMatches()`, the old fallback for when the keyword table
  found nothing, matched on tool names with no notion of mode. "add encrypted
  secret" reached `nopsai.encrypt_secret_for_gitops` through it, straight past
  the GitOps-versus-runtime rule. It is deleted; the metadata scorer is the
  fallback it was trying to be, and it respects the policy.
- `assistantPlannerWantsWriteSchema()` knew `add/set/create/update/enable/write`
  and nothing else, so "pause runner X" did not read as a change request. That
  was invisible while the fallback re-admitted the tool anyway. The operational
  verbs (pause, resume, drain, eject, cancel, rotate, approve, sync, …) are now
  part of the decision.

## Non-goals

- No new public HTTP surface for MCP tools; they call existing routes through
  the permission-checked API bridge.
- No analysis that requires NopsAI to reach the internet.
- No score that mixes evidence the current subject is not allowed to see.
