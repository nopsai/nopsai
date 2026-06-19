# Assistant Capabilities And Chat Examples

The NopsAI Assistant is the conversational layer over NopsAI's first-party
hosted MCP JSON-RPC tool path. It can inspect product state, explain
operational context, draft GitOps changes, and execute confirmed runtime/admin
operations as the current authenticated user.

## Operating Rules

- The assistant is user-scoped. It uses the same AAA subject, grants, scoped
  visibility, and audit model as the user who is chatting.
- Static workflows create a structured turn plan with goal, intent, tool steps,
  and success criteria before execution.
- A feature-wide planner routes clear normal-language requests across the full
  NopsAI MCP surface, including setup, config repositories, notifications,
  monitoring extras, credentials, runners, access/admin, backups/cleanup,
  webhook sources, external triggers, reusable steps, and explicit `nopsai.*`
  tool names.
- NopsAI validates planned tools against the current user's available hosted
  MCP tools, bounded tool-call and argument limits, and mutation confirmation
  requirements before running the plan.
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
- Secret and credential workflows are metadata-, reference-, encrypted-payload-,
  or explicit-write-oriented. Plaintext secret reads are not part of ordinary
  assistant context.
- Internal runner callbacks, public webhook delivery ingress, and UI rendering
  are intentionally not assistant mutation surfaces. The assistant can explain
  those boundaries and point to safe alternatives.

## How To Ask

Good assistant requests usually include four things:

- The intent: inspect, explain, draft a GitOps plan, validate, execute, or
  confirm an action.
- The target: pipeline path/name, run ID, schedule ID, scope, group path,
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
- "Cancel run `run_123`; I confirm."
- "Create a GitOps plan to rotate the GitHub deploy credential. Do not expose
  the secret value."
- "Check config repo drift for folder `platform`."
- "Generate a Kubernetes runner manifest for runner `runner-prod`."
- "List Git webhook deliveries for source `gitlab-main`."
- "Create a data backup; I confirm."
- "Do we have any policy to prevent showing envs?"
- "What NopsAI features can I use from this account, and which ones are blocked
  by permissions?"

## Prompt Patterns

| Intent | Ask like this | Assistant behavior |
| --- | --- | --- |
| Discover access | "What assistant capabilities do I have in this folder?" | Lists feature coverage, tools, resources, and missing AAA actions. |
| Investigate | "Why did run `run_123` fail?" | Reads run status/logs and summarizes likely cause and next checks. |
| Draft GitOps | "Create a GitOps plan for a nightly schedule on `platform/deploy`." | Returns proposed files and commit message; does not apply. |
| Validate | "Validate this pipeline YAML before I commit it." | Parses and reports schema/semantic issues. |
| Confirm action | "Rerun `run_123`; I confirm." | Executes the dedicated mutation tool as the current user. |
| Feature action | "Show notification mail settings" or "List credentials metadata." | Routes to the matching first-party MCP tool and summarizes the result safely. |
| Explain policy | "Why can't the assistant ingest runner logs directly?" | Explains the safe boundary and supported alternatives. |
| Use API bridge | "Call the existing API to list my auth sessions." | Uses guarded `/v1` routes when no dedicated tool is needed. |

## Capability Catalog

### Feature Discovery, Permissions, And Policy

The assistant can show the current user's MCP coverage, available resources,
required AAA actions, and access state. It can also explain policy boundaries
for internal run operations, webhook ingress, and UI rendering.

Ask:

- "What features can I use with the assistant right now?"
- "Show the tools/resources available to my user."
- "Why can I read this pipeline but not update it?"
- "Do we have any policy to prevent showing envs or secrets?"
- "Explain why internal run finalization is not an assistant tool."
- "Can this user execute `pipeline.update` on `folder:platform`?"

Main MCP coverage: `nopsai.get_feature_capabilities`,
`nopsai.get_effective_permissions`, `nopsai.check_resource_use`,
`nopsai.batch_check_resource_use`, `nopsai.explain_internal_run_operations`,
`nopsai.explain_webhook_ingress_policy`, and `nopsai.get_ui_context`.

### Documentation And Knowledge Search

The assistant can search NopsAI docs, read documentation resources, list managed
knowledge contexts, read knowledge documents, and traverse the knowledge refs
attached to a pipeline.

Ask:

- "Search the docs for external trigger configuration."
- "Read the knowledge context named `release-policy`."
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

The assistant can list/search pipelines, read pipeline definitions, generate
pipeline YAML, validate pasted YAML, prepare GitOps create/update plans, and
prepare reusable-step create/update/delete plans. Generated pipeline YAML is
template-aware for common deployment domains such as Go services deployed to
AWS ECS through ECR, and includes assumptions plus required variables/secrets
so the proposal can be reviewed through GitOps.

Ask:

- "Search pipelines that mention Kubernetes runners."
- "Give me a pipeline that has an approval step."
- "Open pipeline `platform/deploy-api` and summarize its steps."
- "Generate a pipeline for build, test, approval, and deploy to staging."
- "Create steps to build, test, and deploy a Golang app to AWS ECS."
- "Validate this YAML and identify unsafe task settings."
- "Create a GitOps update plan for `platform/deploy-api` to add a manual
  approval before production."
- "Create a reusable step named `shared/docker-login` as a GitOps plan."
- "Delete reusable step `shared/old-login` through GitOps."

Main MCP coverage: `nopsai.list_pipelines`, `nopsai.search_pipelines`,
`nopsai.get_pipeline`, `nopsai.generate_pipeline`,
`nopsai.validate_pipeline`, `nopsai.propose_pipeline_create`,
`nopsai.propose_pipeline_update`, `nopsai.propose_reusable_step_create`,
`nopsai.propose_reusable_step_update`, and
`nopsai.propose_reusable_step_delete`.

### Runs, Logs, Approvals, And Run Mutations

The assistant can investigate pipeline runs, inspect logs, analyze failures,
start runs, list approvals, approve/reject gates, rerun, cancel, and delete
runs. Run investigation uses a hosted MCP chain: run metadata, bounded recent
logs, then failure analysis. Mutations require confirmation.

Ask:

- "List recent failed runs for `platform/deploy-api`."
- "List of pipelineruns."
- "Analyze failure for run `run_123` and include relevant log excerpts."
- "Start `platform/deploy-api` with variable `env=staging`; I confirm."
- "List pending approvals for run `run_123`."
- "Approve approval `approval_456` on run `run_123` with comment
  `Change reviewed`; I confirm."
- "Reject approval `approval_456` on run `run_123`; I confirm."
- "Rerun `run_123`; I confirm."
- "Cancel run `run_123`; I confirm."
- "Delete run `run_123`; I confirm."

Main MCP coverage: `nopsai.list_pipeline_runs`, `nopsai.get_pipeline_run`,
`nopsai.get_pipeline_run_logs`, `nopsai.analyze_pipeline_run_failure`,
`nopsai.run_pipeline`, `nopsai.list_run_approvals`,
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

- "Create a knowledge context named `release-policy` for group `platform` with
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
confirmation.

Ask:

- "Show global config sync status."
- "Check drift for the `platform` config repository."
- "Sync the global config repository; I confirm."
- "Sync all config repositories; I confirm."
- "Write these proposed files to the platform config repo with commit message
  `Add release pipeline`; I confirm."
- "List configured config repositories and their enabled state."

Main MCP coverage: `nopsai.get_config_sync_status`,
`nopsai.sync_system_config`, `nopsai.get_config_repo`,
`nopsai.get_config_repo_drift`, `nopsai.sync_config_repo`,
`nopsai.write_config_repo`, `nopsai.list_config_repos`, and
`nopsai.sync_all_config_repos`.

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
test SMTP notification with confirmation, inspect folder notification routes,
and prepare route update/delete plans.

Ask:

- "Show current notification mail settings."
- "Create a GitOps plan to enable SMTP from `nopsai@example.com`."
- "Send a test notification to `ops@example.com`; I confirm."
- "Show the notification route for folder `platform`."
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

Ask:

- "Summarize monitoring health for the last 24 hours."
- "Show pipeline performance for `platform/deploy-api` this week."
- "Which steps are slowest across production deploy pipelines?"
- "Analyze AI usage cost by provider."
- "Give me LLM usage."
- "Give me LLM usage for qwen model."
- "Show tokens for the openai profile last week."
- "Which step used the most tokens?"
- "Which pipeline uses the highest LLM tokens?"
- "Which schedule runs a pipeline with the lowest LLM token usage?"
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
`nopsai.get_monitoring_runner_history`, `nopsai.list_monitoring_views`,
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
Sensitive values stay out of ordinary assistant context.

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
commands, inspect runner monitoring history, and pause/resume dispatch for a
runner with confirmation.

Ask:

- "Check dispatcher and runner health."
- "Generate Docker Compose for runner `runner-prod-1`."
- "Generate a Kubernetes runner manifest in namespace `nopsai-runners`."
- "Generate a bootstrap command for runner `runner-prod-1`."
- "Show runner history for the last seven days."
- "Pause dispatch to runner `runner-prod-1`; I confirm."
- "Resume dispatch to runner `runner-prod-1`; I confirm."

Main MCP coverage: `nopsai.get_system_status`,
`nopsai.get_dispatcher_status`, `nopsai.generate_runner_compose`,
`nopsai.generate_kubernetes_runner_manifest`,
`nopsai.generate_runner_bootstrap_command`,
`nopsai.generate_kubernetes_runner_bootstrap_command`,
`nopsai.get_monitoring_runner_history`, and
`nopsai.update_runner_dispatch`.

### AAA, Access, Audit, And Admin

The assistant can inspect and manage access grants, resource access, resource
use grants, effective permissions, audit logs, users, service accounts, roles,
and identity provider settings. Mutating admin operations require confirmation
and the user's existing admin permissions.

Ask:

- "List access grants for folder `platform`."
- "Grant `maintainer` on folder `platform` to user `user_123`; I confirm."
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

### LLM Profiles, MCP Profiles, System Status, And Lab Items

The assistant can inspect safe LLM profile metadata, MCP profile metadata,
system status, lab items, and lab results. It does not expose credential refs
or unsafe provider details in ordinary chat context.
When an assistant-specific provider is configured, the picker includes the
dedicated `assistant` profile. Otherwise it falls back to existing selectable
LLM profiles for backward compatibility.

Ask:

- "List assistant LLM profiles I can choose from."
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
