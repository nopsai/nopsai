# Assistant Capabilities And Chat Examples

The NopsAI Assistant is the conversational layer over NopsAI's first-party
hosted MCP JSON-RPC tool path. It can inspect product state, explain
operational context, draft GitOps changes, and execute confirmed runtime/admin
operations as the current authenticated user.

## Operating Rules

- The assistant is user-scoped. It uses the same AAA subject, grants, scoped
  visibility, and audit model as the user who is chatting.
- The selected/default assistant LLM acts as a feature-wide planner. It creates
  a structured turn plan with goal, intent, tool steps, and success criteria
  before execution from the live permission-filtered hosted MCP tool list and a
  compact catalog of tool descriptions and input schemas.
- Validated plans are persisted as a user-visible execution-plan activity before
  MCP evidence tools run. Each plan step labels whether the work is MCP-backed,
  docs-backed, knowledge-context-backed, GitOps-proposal-backed, or LLM-derived,
  and includes phase and confidence metadata for audit/review surfaces.
- Assistant conversation turns do not use static normal-language routing. If
  the LLM planner returns malformed JSON, NopsAI retries once with a repair
  prompt that reuses the same live tool catalog and conversation context. If the
  planner is unavailable or still cannot produce a validated plan, no hosted MCP
  tools run and the assistant replies that no changes were applied.
- The planner routes clear normal-language requests across the full NopsAI MCP
  surface, including setup, config repositories, notifications, monitoring
  extras, credentials, runners, access/admin, backups/cleanup, webhook sources,
  external triggers, reusable steps, and explicit `nopsai.*` tool names.
- NopsAI validates planned tools against the current user's available hosted
  MCP tools, tool input schemas, bounded tool-call and argument limits, and
  mutation confirmation requirements before running the plan.
- Final answers from the planner require successful hosted MCP evidence from
  the current turn. The deterministic validator does not reinterpret natural
  language intent with static routing rules.
- Deterministic fallback rendering is evidence-driven. If the planner uses a
  broad or novel intent label but returns valid hosted MCP evidence, the
  assistant summarizes the returned data, proposals, monitoring rows, and object
  lists from the tool output instead of falling back to a docs-only answer.
- The assistant may answer follow-up calculations, estimates, comparisons, and
  explanations without another MCP call when previous same-chat MCP evidence is
  sufficient. These answers must label their data source and confidence,
  separating exact MCP-backed facts from LLM-derived calculations and
  assumptions.
- Pipeline-specific answers are additionally grounded in successful NopsAI
  pipeline evidence. Generated or edited pipeline YAML must go through
  `nopsai.validate_pipeline` or a `nopsai.propose_pipeline_*` tool before the
  assistant can present it as a valid NopsAI pipeline or GitOps plan.
- Some high-level evidence tools are terminal for the planner loop. For
  example, a successful `nopsai.analyze_pipeline_run_failure` call already
  includes run metadata, bounded logs, a root-cause hint, and next steps, so the
  assistant can render the deterministic run-analysis answer without asking a
  slower local model to re-process the same evidence.
- Tool lists, resources, and tool calls are permission-filtered. If a user
  cannot use a route or resource in NopsAI, the assistant cannot bypass that.
- Enterprise feature flags under `assistant.features` decide which broad
  capability families are globally available. AAA still decides the exact
  resources and actions available to the current user.
- Assistant model configuration is separate from pipeline execution profiles
  when `assistant.provider` is set. The safe config endpoint reports provider,
  model, docs defaults, limits, and feature flags without exposing credential
  refs, API key env names, base URLs, or provider extras.
- GitOps write tools return plans with `applies:false` and commit-ready file
  payloads. The assistant does not silently save pipeline, schedule, knowledge,
  webhook-source, external-trigger, notification, secret, variable, credential,
  or reusable-step changes to product state.
- Runtime, admin, cleanup, SMTP, trigger invocation, and other side-effecting
  actions require explicit confirmation in chat before the MCP call is made
  with `confirm:true`.
- Final LLM synthesis is quality-gated against the validated plan and hosted
  MCP evidence. If the model claims unapplied changes or omits required
  proposal-safe wording, NopsAI falls back to the deterministic tool summary.
- Hosted MCP API responses are interpreted from their JSON response envelope
  when deterministic assistant summaries and quality checks read analytics
  evidence. For example, AI usage answers rank pipelines, steps, models, and
  runs from the live `/v1/monitoring/ai-usage` payload instead of treating the
  wrapper metadata as an empty result.
- AI usage analytics expose provider, model, profile, feature, step, task,
  pipeline, schedule, subject, and run breakdowns. Assistant chat token usage is
  merged into the same global AI usage view and is attributed back to provider
  and model through the selected model when that profile is stored in
  NopsAI.
- Successful AI usage evidence is terminal for the assistant planner loop. The
  assistant renders the deterministic token analytics summary directly instead
  of asking a final LLM pass to reinterpret the numbers. These summaries use a
  reusable analytics-dimension catalog over the live payload, so provider,
  pipeline, step, task, run, model, profile, feature, and schedule questions all
  use the same selection path instead of static question templates.
- Pipeline proposal synthesis also requires review/GitOps-safe language. If
  the model omits the review path or implies an unverified pipeline, NopsAI
  falls back to the deterministic validation/proposal summary.
- The packaged UI nginx proxy keeps API connections open long enough for local
  LLM-backed assistant turns, while the assistant still avoids unnecessary
  planner/synthesis passes when a validated MCP tool result is sufficient.
- Assistant UI transport errors sanitize HTML proxy/gateway responses before
  rendering them in the chat error banner. For transient gateway, timeout, or
  network-like send failures, the UI briefly refetches the conversation and
  reconciles the saved assistant reply when the backend completed the turn even
  though the original browser request failed.
- Secret and credential workflows are metadata-, reference-, encrypted-payload-,
  or explicit-write-oriented. Plaintext secret reads are not part of ordinary
  assistant context.
- Direct MCP writes/deletes that require confirmation become pending
  confirmations. The assistant asks the user to confirm the exact action and
  applies it only after an explicit confirmation such as `confirm`; detail-only
  follow-ups update the pending action but do not execute it.
- Internal runner callbacks, public webhook delivery ingress, and UI rendering
  are intentionally not assistant mutation surfaces. The assistant can explain
  those boundaries and point to safe alternatives.

## How To Ask

Good assistant requests usually include four things:

- The intent: inspect, explain, draft a GitOps plan, validate, execute, or
  confirm an action.
- The target: pipeline path/name, run ID, schedule ID, scope, team path,
  config repo, credential ID, runner ID, or resource type/resource ID.
- The mode: read-only, proposal only, GitOps-ready, or confirmed execution.
- The constraints: environment, repository, branch, approval comment, retention
  window, variables, or notification recipient.

When the request already has enough context, the assistant should answer through
the matching permission-bound MCP tools. When a normal-language request is too
broad to choose the right tool safely, it should ask a short clarifying question
instead of guessing. For example, "show usage" should ask whether the user means
AI/LLM tokens, runner cost, pipeline runs, variables, or a specific target.

Examples:

- "Search pipelines that deploy `acme/api` and show their knowledge context."
- "Give me a pipeline that has an approval step."
- "List pipeline runs."
- "Validate this pipeline YAML and return a GitOps create plan for
  `platform/deploy-api`."
- "Analyze run `run_123` and explain the most likely failure from logs."
- "Show the final outputs for run `run_123`."
- "Cancel run `run_123`; I confirm."
- "Create a GitOps plan to rotate the GitHub deploy credential. Do not expose
  the secret value."
- "Check config repo drift for team `platform`."
- "Generate a Kubernetes runner manifest for runner `runner-prod`."
- "List Git webhook deliveries for source `gitlab-main`."
- "Create a data backup; I confirm."
- "Do we have any policy to prevent showing envs?"
- "What NopsAI features can I use from this account, and which ones are blocked
  by permissions?"

## Prompt Patterns

| Intent | Ask like this | Assistant behavior |
| --- | --- | --- |
| Discover access | "What assistant capabilities do I have in this team?" | Lists feature coverage, tools, resources, and missing AAA actions. |
| Investigate | "Why did run `run_123` fail?" | Reads run status, final outputs, and logs, then summarizes likely cause and next checks. |
| Draft GitOps | "Create a GitOps plan for a nightly schedule on `platform/deploy`." | Returns proposed files and commit message; does not apply. |
| Validate | "Validate this pipeline YAML before I commit it." | Parses and reports schema/semantic issues. |
| Confirm action | "Rerun `run_123`; I confirm." | Executes the dedicated mutation tool as the current user. |
| Feature action | "Show notification mail settings" or "List credentials metadata." | Routes to the matching first-party MCP tool and summarizes the result safely. |
| Explain policy | "Why can't the assistant ingest runner logs directly?" | Explains the safe boundary and supported alternatives. |
| Use API bridge | "Call the existing API to list my auth sessions." | Uses guarded `/v1` routes when no dedicated tool is needed. |

## Chat UI Controls

The assistant UI keeps conversation content first while making audit/evidence
metadata available on demand.

- Empty conversations open on a welcoming workspace: "Hi, I'm NopsAI. What are
  we solving today?" with starter prompts for failed-run analysis, pipeline
  improvement, system health, and rollout planning. Starters prefill the
  composer so users can edit before sending.
- The composer is a rounded ask bar with a goal-oriented placeholder, visible
  Send label, `Cmd/Ctrl + Enter` shortcut hint, and a draggable top border for
  vertical resizing.
- The visible session line stays human-readable: "Ready · changes always need
  your review." MCP, memory, confirmation policy, and chat-level model
  selection are available in a compact Session details disclosure. New
  conversations default to the configured assistant/default model unless
  the user chooses another chat profile.
- Message bubbles show only user/assistant content plus compact actions such as
  copy and retry. Raw internal planner and synthesis calls are not rendered
  under each message.
- Assistant replies can show a compact execution plan above the answer. This is
  the safe, user-facing plan summary with source, phase, and confidence labels;
  it is not hidden model chain-of-thought.
- While a turn is running, the assistant shows a compact time-aware staged
  progress list such as planning, reading permission-bound evidence, comparing
  relevant records, synthesizing the answer, and reconciling the saved result.
  These are safe operational stages, not hidden chain-of-thought.
- Optimistic user messages are reconciled with refetched server messages, so a
  user prompt is not shown twice when someone switches conversations and
  returns while the assistant turn is still in flight.
- Assistant messages render a safe markdown subset for headings, bullets,
  inline code, fenced code blocks, and HTTPS citations. Operational LLM
  synthesis is instructed to prefer Summary, Evidence, and Recommended next
  step sections when supported by the current turn's tool evidence.
- Each message records visible content-token estimates separately from model
  usage. Assistant replies record provider-reported or estimated
  planner/synthesis prompt, completion, and total token counts, LLM call count,
  and response duration; user messages do not add visible text estimates to
  model `total_tokens`, and deterministic replies without an LLM call stay
  visible-text-only as well. Conversation detail panels show both views for
  monitoring and troubleshooting. Prometheus exports assistant token, duration,
  and LLM-call counters from the same stored message usage fields.
- Planner and final synthesis prompts include a compact recent same-chat
  transcript plus conversation memory, so follow-up requests such as "generic
  pipeline", "the dashboard definition", or "give me both" can resolve against
  prior turns in that conversation instead of being treated as isolated prompts.
- Docked assistant turns also include bounded current-page context from the UI:
  route, page area, tab, team/scope, selected resource IDs, and allow-listed
  filters. This helps prompts like "explain this" on a run, pipeline, scope, or
  dashboard page resolve the visible target without copying page text into the
  chat. The composer renders this context as a removable chip; when removed,
  the context is omitted from profile scoping and message sends. Explicit user
  targets override page context, and page context overrides older conversation
  memory.
- Sample, template, schema, and definition-shape requests are routed through
  current NopsAI docs or capability evidence before the assistant answers. The
  chat path must not use hardcoded example payloads as a fallback.
- Planner prompts keep the live hosted MCP tool list visible, but include
  compact input schemas only for tools that look relevant to the request,
  extracted context, or previous evidence. This keeps planning grounded in
  current AAA/tool availability without sending every tool schema on every
  turn. Schema selection is mode-specific: read/check requests receive the
  domain read or validation schema, direct confirmed changes receive runtime
  write/delete schemas, and GitOps/proposal schemas are included only when the
  user asks for a GitOps/proposal/file-plan workflow. For example, "add
  encrypted secret" is treated as a direct secret-domain MCP write request;
  it is not routed to GitOps unless the user explicitly asks for GitOps or a
  proposal. If the LLM still tries to pick a tool outside the selected schema
  subset, NopsAI blocks the tool before execution and returns user-facing
  guidance instead of exposing planner internals.
- The full assistant view keeps NopsAI evidence in the context panel, filtering
  out internal `nopsai.llm.plan` and `nopsai.llm.complete` entries so product
  evidence remains scannable. Evidence is progressive disclosure: the default
  row shows tool, status, and resources, while "View details" reveals bounded
  input/output JSON for deeper investigation.
- Conversation memory, NopsAI evidence, usage, and proposed changes live in
  collapsible detail sections. The dock stays lightweight and leaves deep
  evidence/configuration to the full assistant page.
- The left rail focuses on conversations and conversation deletion. Delete
  actions stay visible for scannability; only the conversation currently
  running an assistant turn is locked from deletion, so users can clean up other
  chats while a long model request is still in flight. Chat-level actions stay
  close to the relevant message or session detail, avoiding duplicate new/copy
  controls in the conversation header.
- Conversation deletion uses the authenticated user's assistant subject and
  removes the conversation, messages, and memory through the existing database
  cascade. It does not modify GitOps-managed product configuration.

## Capability Catalog

### Feature Discovery, Permissions, And Policy

The assistant can show the current user's MCP coverage, available resources,
required AAA actions, and access state. It can also explain policy boundaries
for env/secret exposure, internal run operations, webhook ingress, and UI
rendering.

Example prompts, not static routing templates:

- "What features can I use with the assistant right now?"
- "Show the tools/resources available to my user."
- "Why can I read this pipeline but not update it?"
- "Do we have any policy to prevent showing envs or secrets?"
- "Explain why internal run finalization is not an assistant tool."
- "Can this user execute `pipeline.update` on `team:platform`?"

Main MCP coverage: `nopsai.get_feature_capabilities`,
`nopsai.get_effective_permissions`, `nopsai.check_resource_use`,
`nopsai.batch_check_resource_use`, `nopsai.explain_internal_run_operations`,
`nopsai.explain_webhook_ingress_policy`, and `nopsai.get_ui_context`.

### Documentation And Knowledge Search

The assistant can search NopsAI product docs, read documentation resources,
list managed knowledge contexts, read knowledge documents, and traverse the
knowledge refs attached to a pipeline.

Ask:

- "Search the docs for external trigger configuration."
- "I need a working pipeline example that sends data to a dashboard."
- "Read the knowledge context named `release-policy`."
- "Do we have any env exposure policy in knowledge context?"
- "Show all knowledge docs available to this pipeline."
- "Walk the knowledge refs in `platform/release` and tell me which docs are
  managed by NopsAI versus repo-local."

Main MCP coverage: `nopsai.search_docs`, `nopsai.read_doc`,
`nopsai.list_knowledge_contexts`, `nopsai.get_knowledge_context`, and
`nopsai.get_pipeline_knowledge_context`.

### First-Install Setup

The assistant can guide empty-environment bootstrap: setup status, preflight
health, starter templates, setup planning, and confirmed bootstrap execution.

Ask:

- "Check first-install setup status and preflight."
- "Generate setup templates for the team profile and repositories
  `acme/api`, `acme/web`."
- "Plan a first-install setup with generated secrets, starter database, and a
  config repository at `https://github.com/acme/nopsai-config.git`."
- "Bootstrap the planned first-install setup; I confirm."

Main MCP coverage: `nopsai.get_setup_status`, `nopsai.get_setup_preflight`,
`nopsai.get_setup_templates`, `nopsai.plan_first_install_setup`, and
`nopsai.bootstrap_first_install_setup`.

### Pipelines And YAML Authoring

The assistant can list/search pipelines, read pipeline definitions, validate
pasted or LLM-drafted YAML, prepare GitOps create/update plans, and prepare
reusable-step create/update/delete plans. Pipeline YAML drafting happens in the
LLM planner/synthesis layer and must be checked through the hosted MCP
validation or proposal tools before it is presented as evidence-backed output.
When a pipeline create proposal includes an explicit target name, the proposal
tool can use that target as the YAML top-level `name` if the drafted YAML omitted
it, while still rejecting mismatches between the target and YAML names.
Every drafted pipeline step must include exactly one execution method:
`include`, `tasks`, `goal`, `script`, or `approval`. For the common generated
Docker workflow of clone repository, build Docker image, and push image to a
registry, the proposal tool completes name-only generated steps with concrete
shell scripts, workspace defaults, required runtime variables, and step
dependencies before validation. Other name-only steps remain invalid and are
reported by validation instead of being guessed.
There is no static pipeline generator or template selector in the assistant
tool path.

Ask:

- "Search pipelines that mention Kubernetes runners."
- "Give me a pipeline that has an approval step."
- "Open pipeline `platform/deploy-api` and summarize its steps."
- "Draft and validate a pipeline for build, test, approval, and deploy to
  staging."
- "Prepare a GitOps create plan for a Golang service deploy pipeline."
- "Give me a pipeline that has 4 steps and the last one is approval; the goal
  is to build and publish a Docker image based on DDD standards."
- "Validate this YAML and identify unsafe task settings."
- "Create a GitOps update plan for `platform/deploy-api` to add a manual
  approval before production."
- "Create a reusable step named `shared/docker-login` as a GitOps plan."
- "Delete reusable step `shared/old-login` through GitOps."

Main MCP coverage: `nopsai.list_pipelines`, `nopsai.search_pipelines`,
`nopsai.get_pipeline`, `nopsai.validate_pipeline`, `nopsai.propose_pipeline_create`,
`nopsai.propose_pipeline_update`, `nopsai.propose_reusable_step_create`,
`nopsai.propose_reusable_step_update`, and
`nopsai.propose_reusable_step_delete`.

### Runs, Logs, Approvals, And Run Mutations

The assistant can investigate pipeline runs, inspect final outputs and logs,
analyze failures, start runs, list approvals, approve/reject gates, rerun,
cancel, and delete runs. Run investigation uses a hosted MCP chain: run
metadata, final output summaries when available, bounded recent logs, then
failure analysis. `nopsai.get_pipeline_run_logs` accepts `include_children` so
parent-run investigations can include logs from included child pipelines.
Mutations require confirmation.

Ask:

- "List recent failed runs for `platform/deploy-api`."
- "List of pipelineruns."
- "Analyze failure for run `run_123` and include relevant log excerpts."
- "Read the `Executive summary` final output for run `run_123`."
- "Start `platform/deploy-api` with variable `env=staging`; I confirm."
- "List pending approvals for run `run_123`."
- "Approve approval `approval_456` on run `run_123` with comment
  `Change reviewed`; I confirm."
- "Reject approval `approval_456` on run `run_123`; I confirm."
- "Rerun `run_123`; I confirm."
- "Cancel run `run_123`; I confirm."
- "Delete run `run_123`; I confirm."

Main MCP coverage: `nopsai.list_pipeline_runs`, `nopsai.get_pipeline_run`,
`nopsai.get_pipeline_run_output`, `nopsai.get_pipeline_run_logs`,
`nopsai.analyze_pipeline_run_failure`, `nopsai.run_pipeline`,
`nopsai.list_run_approvals`,
`nopsai.approve_run_approval`, `nopsai.reject_run_approval`,
`nopsai.rerun_pipeline_run`, `nopsai.cancel_pipeline_run`, and
`nopsai.delete_pipeline_run`.

### Triggers And Schedules

The assistant can list/read triggers and schedules, propose trigger or schedule
changes, enable/disable schedules through GitOps plans, and run a schedule now
with confirmation.

Ask:

- "List triggers for `platform/deploy-api`."
- "Show schedule `sched_123` and explain when it runs."
- "Create a GitOps plan for a weekday 08:00 UTC schedule on
  `platform/deploy-api`."
- "Disable schedule `sched_123` through GitOps."
- "Enable schedule `sched_123` through GitOps."
- "Run schedule `sched_123` now; I confirm."

Main MCP coverage: `nopsai.list_triggers`, `nopsai.get_trigger`,
`nopsai.propose_trigger_change`, `nopsai.list_schedules`,
`nopsai.get_schedule`, `nopsai.propose_schedule_create`,
`nopsai.propose_schedule_update`, `nopsai.propose_schedule_delete`,
`nopsai.propose_schedule_enable`, `nopsai.propose_schedule_disable`,
`nopsai.propose_schedule_change`, and `nopsai.run_schedule_now`.

### Knowledge Context Management

The assistant can create, update, and delete managed knowledge contexts as
GitOps-ready plans.

Ask:

- "Create a knowledge context named `release-policy` for team `platform` with
  this content, as a GitOps plan."
- "Update knowledge context `kc_123` to add the new rollback policy."
- "Delete knowledge context `kc_123` through GitOps."

Main MCP coverage: `nopsai.propose_knowledge_context_create`,
`nopsai.propose_knowledge_context_update`, and
`nopsai.propose_knowledge_context_delete`.

### Webhook Sources And External Triggers

The assistant can inspect Git webhook sources, review webhook deliveries,
prepare webhook-source GitOps plans, inspect external triggers, review
invocations, prepare external-trigger GitOps plans, and invoke an external
trigger with confirmation.

Ask:

- "List Git webhook sources and recent deliveries for the GitLab source."
- "Create a GitOps plan for a GitLab webhook source using credential ref
  `credential:gitlab-webhook`."
- "Update webhook source `gitlab-main` to restrict repositories to
  `acme/api`."
- "List external triggers for `platform/deploy-api`."
- "Show recent invocations for external trigger `ext_123`."
- "Create an external trigger for `platform/deploy-api` as a GitOps plan."
- "Invoke external trigger `ext_123` with payload `{ \"env\": \"staging\" }`;
  I confirm."
- "Explain why public webhook delivery ingress is not an assistant mutation
  tool."

Main MCP coverage: `nopsai.list_git_webhook_sources`,
`nopsai.get_git_webhook_source`, `nopsai.list_git_webhook_deliveries`,
`nopsai.propose_git_webhook_source_create`,
`nopsai.propose_git_webhook_source_update`,
`nopsai.propose_git_webhook_source_delete`,
`nopsai.list_external_triggers`, `nopsai.get_external_trigger`,
`nopsai.list_external_trigger_invocations`,
`nopsai.propose_external_trigger_create`,
`nopsai.propose_external_trigger_update`,
`nopsai.propose_external_trigger_delete`, and
`nopsai.invoke_external_trigger`.

### Config Repositories And GitOps Workflows

The assistant can inspect config repository bindings, check drift, sync one or
all config repos, and write files through the existing GitOps workflow with
confirmation. Config repository bindings include the selected Git provider and
credential reference when they use GitLab, Bitbucket, Gitea, or token-backed
GitHub access.

Ask:

- "Show global config sync status."
- "Check drift for the `platform` config repository."
- "Sync the global config repository; I confirm."
- "Sync all config repositories; I confirm."
- "Cancel the stuck config repository sync; I confirm."
- "Write these proposed files to the platform config repo with commit message
  `Add release pipeline`; I confirm."
- "List configured config repositories and their enabled state."

Main MCP coverage: `nopsai.get_config_sync_status`,
`nopsai.sync_system_config`, `nopsai.cancel_system_config_sync`,
`nopsai.get_config_repo`, `nopsai.get_config_repo_drift`, `nopsai.sync_config_repo`,
`nopsai.cancel_config_repo_sync`,
`nopsai.write_config_repo`, `nopsai.list_config_repos`, and
`nopsai.sync_all_config_repos`, `nopsai.cancel_all_config_repos_sync`.

### Scopes, Secrets, And Variables

The assistant can list scopes, explain scope permissions, inspect secret and
variable metadata, analyze repeated variable names across visible scopes without
reading values, encrypt secret values for GitOps, write/delete values with
confirmation, and prepare scoped GitOps value plans.

Ask:

- "List scopes available to me and explain which one should own deploy
  secrets."
- "How many scopes do we have, and how many secrets are in each scope?"
- "Show secret metadata for the `platform/prod` scope."
- "Encrypt this secret value for GitOps under key `GITHUB_TOKEN`."
- "Write secret `GITHUB_TOKEN` in scope `platform/prod`; I confirm."
- "Delete secret `OLD_TOKEN` from scope `platform/dev`; I confirm."
- "Create a GitOps plan to add variable `DEPLOY_REGION=eu-west-1` to
  `platform/prod`."
- "How many repetitive variables are used across all scopes?"
- "Read variable `DEPLOY_REGION` from `platform/prod`."
- "Delete variable `OLD_REGION` from `platform/prod`; I confirm."

Main MCP coverage: `nopsai.list_scopes`, `nopsai.get_scope`,
`nopsai.explain_scope_permissions`, `nopsai.list_secret_scopes`,
`nopsai.list_secrets_metadata`, `nopsai.encrypt_secret_for_gitops`,
`nopsai.write_secret_value`, `nopsai.delete_secret_value`,
`nopsai.propose_secret_gitops_write`,
`nopsai.propose_secret_gitops_delete`, `nopsai.list_variable_scopes`,
`nopsai.list_variables_metadata`, `nopsai.analyze_variable_usage`,
`nopsai.get_variable_value`, `nopsai.write_variable_value`,
`nopsai.delete_variable_value`, `nopsai.propose_variable_gitops_write`, and
`nopsai.propose_variable_gitops_delete`.

### Notifications

The assistant can inspect mail settings, prepare GitOps mail settings, send a
test SMTP notification with confirmation, inspect team notification routes,
and prepare route update/delete plans.

Ask:

- "Show current notification mail settings."
- "Create a GitOps plan to enable SMTP from `nopsai@example.com`."
- "Send a test notification to `ops@example.com`; I confirm."
- "Show the notification route for team `platform`."
- "Create a GitOps plan to route failed production runs to
  `ops@example.com`."
- "Delete the notification route for `platform/legacy` through GitOps."

Main MCP coverage: `nopsai.get_notification_mail_settings`,
`nopsai.propose_notification_mail_settings`,
`nopsai.test_notification_mail_settings`,
`nopsai.get_notification_route`,
`nopsai.propose_notification_route_update`, and
`nopsai.propose_notification_route_delete`.

### Monitoring, Analytics, Cost, And Recommendations

The assistant can summarize system health, run analytics, pipeline/step/task
performance, trigger analytics, AI usage, reliability, efficiency, security,
runner history, statistics, cost, design suggestions, cost suggestions, saved
views, alert rules, alert events, and recommendations. AI/LLM usage prompts
inspect before answering: empty default-window results are retried against
broader windows, combined with summary/efficiency context, and explained with a
recording/permission diagnosis when no events are visible.
Global AI usage includes assistant chat model tokens as the `assistant_chat`
feature, so monitoring can explain how much provider/model usage chat consumed
as well as pipeline/run LLM work. Visible user and assistant text estimates stay
separate from model token totals. Pipeline final output generation is recorded
as the `pipeline_final_output` feature. Run-level token questions are answered
from `nopsai.get_monitoring_ai_usage` with the run ID as a filter; those
run-scoped answers stay limited to run AI usage events, and run status, log, and
failure-analysis tools do not provide token counts.

Ask:

- "Summarize monitoring health for the last 24 hours."
- "Show pipeline performance for `platform/deploy-api` this week."
- "Which steps are slowest across production deploy pipelines?"
- "Find bottlenecks in deploy steps and suggest what to improve."
- "Analyze AI usage cost by provider."
- "Estimate the AI usage cost from the token data you just found."
- "Give me LLM usage."
- "Give me LLM usage for qwen model."
- "Show tokens for the openai profile last week."
- "How many tokens were used by pipeline run `e3850cec-550f-456a-bec8-e67777d71d24`?"
- "Which step used the most tokens?"
- "Which pipeline uses the highest LLM tokens?"
- "Which schedule runs a pipeline with the lowest LLM token usage?"
- "Compare schedule AI usage for this month."
- "Explain pipeline health for `platform/deploy-api`."
- "Find optimization opportunities across production pipelines."
- "Show reliability and security monitoring signals."
- "Create a monitoring saved view for failed production deploys; I confirm."
- "Create an alert rule for failure rate above 10 percent; I confirm."
- "Evaluate alert rule `alert_123` now; I confirm."
- "List alert events and open recommendations."
- "Acknowledge recommendation `rec_123`; I confirm."
- "Resolve recommendation `rec_123`; I confirm."
- "Suggest cost improvements for runner usage."
- "Suggest design improvements for pipeline reliability."

Main MCP coverage: `nopsai.get_statistics`, `nopsai.get_cost_summary`,
`nopsai.suggest_cost_improvements`, `nopsai.suggest_design_improvements`,
`nopsai.get_monitoring_summary`, `nopsai.get_monitoring_run_analytics`,
`nopsai.get_monitoring_pipeline_performance`,
`nopsai.get_monitoring_step_performance`,
`nopsai.get_monitoring_task_performance`,
`nopsai.get_monitoring_trigger_analytics`,
`nopsai.get_monitoring_external_trigger_analytics`,
`nopsai.get_monitoring_ai_usage`, `nopsai.get_monitoring_reliability`,
`nopsai.get_monitoring_efficiency`, `nopsai.get_monitoring_security`,
`nopsai.get_monitoring_runner_history`,
`nopsai.get_monitoring_schedule_ai_usage`,
`nopsai.get_monitoring_schedule_performance`,
`nopsai.get_monitoring_trigger_performance`,
`nopsai.get_pipeline_efficiency`, `nopsai.compare_pipelines`,
`nopsai.compare_schedules`, `nopsai.explain_pipeline_health`,
`nopsai.find_optimization_opportunities`, `nopsai.list_monitoring_views`,
`nopsai.create_monitoring_view`, `nopsai.update_monitoring_view`,
`nopsai.delete_monitoring_view`, `nopsai.list_monitoring_alert_rules`,
`nopsai.create_monitoring_alert_rule`,
`nopsai.update_monitoring_alert_rule`,
`nopsai.delete_monitoring_alert_rule`,
`nopsai.evaluate_monitoring_alert_rule`,
`nopsai.list_monitoring_alert_events`,
`nopsai.list_monitoring_recommendations`,
`nopsai.acknowledge_monitoring_recommendation`, and
`nopsai.resolve_monitoring_recommendation`.

### Credentials

The assistant can list credential metadata, inspect a credential, create
credentials, rotate values, activate versions, enable/disable credentials,
delete versions, delete credentials, and prepare GitOps credential plans.
Credential access grants use the `credential` resource type and reference-path
IDs, while sensitive values stay out of ordinary assistant context.

Ask:

- "List credential metadata for deployment integrations."
- "Show metadata for credential `cred_123`."
- "Create a credential named `github-deploy-token`; I confirm."
- "Rotate credential `cred_123`; I confirm."
- "Activate credential version `ver_456`; I confirm."
- "Disable credential `cred_123`; I confirm."
- "Enable credential `cred_123`; I confirm."
- "Delete credential version `ver_456`; I confirm."
- "Delete credential `cred_123`; I confirm."
- "Create a GitOps plan for the `github-deploy-token` credential reference."

Main MCP coverage: `nopsai.list_credentials_metadata`,
`nopsai.get_credential_metadata`, `nopsai.create_credential`,
`nopsai.rotate_credential_value`, `nopsai.activate_credential_version`,
`nopsai.disable_credential`, `nopsai.enable_credential`,
`nopsai.delete_credential_version`, `nopsai.delete_credential`, and
`nopsai.propose_credential_gitops`.

### Runners And Dispatcher

The assistant can inspect dispatcher/system status, generate local runner
Docker Compose, generate Kubernetes runner manifests, generate runner bootstrap
commands, inspect runner monitoring history, pause/resume dispatch for a
runner, and remove a runner registration with confirmation. Removal disconnects
live streams, clears dispatcher status, requeues in-flight work, and persists
the runner ID in `ejected_runner_ids`; ordinary network disconnects still allow
the same runner ID to reconnect.
Dispatcher status includes previously registered runners and marks those without
a live connection as unreachable rather than hiding them.

Ask:

- "Check dispatcher and runner health."
- "Generate Docker Compose for runner `runner-prod-1`."
- "Generate a Kubernetes runner manifest in namespace `nopsai-runners`."
- "Generate a bootstrap command for runner `runner-prod-1`."
- "Show runner history for the last seven days."
- "Pause dispatch to runner `runner-prod-1`; I confirm."
- "Resume dispatch to runner `runner-prod-1`; I confirm."
- "Remove runner `runner-prod-5`; I confirm."

Main MCP coverage: `nopsai.get_system_status`,
`nopsai.get_dispatcher_status`, `nopsai.generate_runner_compose`,
`nopsai.generate_kubernetes_runner_manifest`,
`nopsai.generate_runner_bootstrap_command`,
`nopsai.generate_kubernetes_runner_bootstrap_command`,
`nopsai.get_monitoring_runner_history`, and
`nopsai.update_runner_dispatch` and `nopsai.eject_runner`.

### AAA, Access, Audit, And Admin

The assistant can inspect and manage access grants, resource access, resource
use grants, effective permissions, audit logs, users, service accounts, roles,
and identity provider settings. Mutating admin operations require confirmation
and the user's existing admin permissions.

Ask:

- "List access grants for team `platform`."
- "Grant `maintainer` on team `platform` to user `user_123`; I confirm."
- "Remove access grant `grant_123`; I confirm."
- "Show resource access settings for pipeline `platform/deploy-api`."
- "Update resource access visibility to `private`; I confirm."
- "Create a resource-use grant for service account `sa_123`; I confirm."
- "List recent audit logs."
- "List admin users and service accounts."
- "Create service account `ci-bot`; I confirm."
- "Update identity provider `oidc` settings; I confirm."

Main MCP coverage: `nopsai.list_access_grants`,
`nopsai.create_access_grant`, `nopsai.delete_access_grant`,
`nopsai.get_resource_access`, `nopsai.update_resource_access`,
`nopsai.create_resource_use_grant`,
`nopsai.delete_resource_access_grant`, `nopsai.list_audit_logs`,
`nopsai.list_admin_users`, `nopsai.create_admin_user`,
`nopsai.update_admin_user`, `nopsai.delete_admin_user`,
`nopsai.list_admin_service_accounts`,
`nopsai.create_admin_service_account`,
`nopsai.update_admin_service_account`,
`nopsai.delete_admin_service_account`, `nopsai.list_admin_roles`,
`nopsai.create_admin_role`, `nopsai.delete_admin_role`,
`nopsai.list_admin_identity_providers`, and
`nopsai.update_admin_identity_provider`.

### Data Backup And Cleanup

The assistant can list backups, create/delete backups with confirmation,
preview cleanup impact, run cleanup with confirmation, and manage cleanup
schedules. Cleanup actions are high-impact and should be previewed first.

Ask:

- "List data backups."
- "Create a full data backup; I confirm."
- "Preview cleanup of runs older than 90 days and keep the last 20."
- "Run that cleanup with backup before cleanup; I confirm."
- "List data cleanup jobs and schedules."
- "Create a weekly cleanup schedule for old run logs; I confirm."
- "Disable cleanup schedule `cleanup_123`; I confirm."
- "Run cleanup schedule `cleanup_123` now; I confirm."
- "Delete backup `backup_123`; I confirm."

Main MCP coverage: `nopsai.list_data_backups`,
`nopsai.create_data_backup`, `nopsai.delete_data_backup`,
`nopsai.preview_data_cleanup`, `nopsai.run_data_cleanup`,
`nopsai.list_data_cleanup_jobs`, `nopsai.list_data_cleanup_schedules`,
`nopsai.create_data_cleanup_schedule`,
`nopsai.update_data_cleanup_schedule`,
`nopsai.enable_data_cleanup_schedule`,
`nopsai.disable_data_cleanup_schedule`,
`nopsai.run_data_cleanup_schedule`, and
`nopsai.delete_data_cleanup_schedule`.

### Models, MCP Profiles, System Status, And Lab Items

The assistant can inspect safe model metadata, MCP profile metadata,
system status, lab items, and lab results. It does not expose credential refs
or unsafe provider details in ordinary chat context.
When an assistant-specific provider is configured, the picker includes the
dedicated `assistant` profile. Otherwise it falls back to existing selectable
models for backward compatibility.

Ask:

- "List assistant models I can choose from."
- "Show MCP profiles configured for the system."
- "Check NopsAI system status."
- "List lab items and explain result `lab_123`."

Main MCP coverage: `nopsai.get_llm_profiles`, `nopsai.get_mcp_profiles`,
`nopsai.get_system_status`, `nopsai.list_lab_items`,
`nopsai.get_lab_item`, `nopsai.explain_lab_result`, and
`nopsai.call_api` for guarded compatibility routes.

### UI Context

The assistant can explain which UI area owns rendering and what data or plans
the assistant can provide. It does not render product UI through MCP.

Ask:

- "Which UI page owns pipeline rendering?"
- "What data should the monitoring UI request from MCP versus render itself?"
- "Explain the UI ownership for schedules."

Main MCP coverage: `nopsai.get_ui_context`.

## Compatibility API Bridge

When a feature has an existing guarded `/v1` API route but no richer
first-class assistant workflow is needed, the assistant can use
`nopsai.call_api`. The API bridge:

- runs as the current authenticated user
- allows only guarded `/v1` product routes
- rejects public/provider ingress and internal service routes
- blocks default plaintext secret reads
- requires `confirm:true` for mutating routes
- preserves the existing route authorization and audit behavior

Ask:

- "Use the existing API to list my auth sessions."
- "Call the API to inspect this settings route, read-only."
- "Use the API bridge to update this admin field; I confirm."

## Intentionally Unsupported As Chat Mutations

- Log ingestion, task status updates, and run finalization are runner/internal
  service operations. Ask the assistant to analyze runs and logs instead.
- Public webhook delivery ingress is an external provider endpoint. Ask the
  assistant to manage webhook sources, review deliveries, or invoke external
  triggers instead.
- UI rendering stays in the UI. Ask the assistant for data, plans, ownership
  hints, and implementation context instead.
- Plaintext secret and credential reads are not ordinary assistant context.
  Ask for metadata, encrypted GitOps payloads, references, or explicit
  write/rotation flows instead.
