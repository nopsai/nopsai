import type { WikiSection } from '../types.js';

export const operationsSection: WikiSection = {
  id: 'operations',
  title: 'Operations',
  owner: 'Operations',
  description: 'Running the platform day to day: runs, logs, metrics, evaluation, dashboards, notifications, updates, hardening, and triage.',
  articles: [
    {
      id: 'run-lifecycle',
      title: 'How a run executes',
      docType: 'concept',
      audiences: ['automation-author', 'operator', 'developer'],
      summary: 'From trigger to final deliverable: validation, authorization, dispatch, agent execution, and finalization.',
      keywords: ['lifecycle', 'execution', 'flow', 'dispatch', 'sequence'],
      keyFacts: [
        'Validation happens before anything is queued: schema, structural limits, dependency graph, profile references, and scope access are all checked up front.',
        'AAA authorizes the original caller for every referenced resource, not just the pipeline.',
        'The dispatcher selects a runner by scope, capacity, and reachability, then assigns the run over gRPC.',
        'The agent resolves variables, secrets, knowledge, and profiles, then executes the graph respecting `depends_on`.',
        'Approval steps pause the run durably and release runner capacity instead of holding it.',
        'Final outputs are generated after execution finishes and stored separately from raw task logs.',
      ],
      details: [
        'Dependency order comes from the graph, not from list position. A step or task starts as soon as every dependency has completed, so independent branches run concurrently up to runner capacity.',
        'Runtime output references such as `$steps.<step>.<task>.outputs.<name>` are valid when the graph guarantees the producer runs first. A direct `depends_on` edge is not required if a transitive upstream path already provides that ordering.',
        'A failed step normally fails the run. With `ignore_failure`, the failure is recorded as `failure (ignored)`, the step renders as a warning, and an otherwise successful run finishes with status `warning`.',
        'Approval and explicit policy or guardrail enforcement failures always fail closed, regardless of `ignore_failure`.',
        'Blocking Knowledge Context (`policy` and `guardrail`) is pinned by scope at run start, then recomputed as pipeline, step, and task scopes begin. Emergency policy response cancels active runs rather than mutating already-resolved policy.',
      ],
      examples: [
        {
          title: 'Watch a run move through its states',
          language: 'bash',
          code: [
            '# lightweight: status only, safe to poll',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID/status" | jq',
            '',
            '# full detail once it settles',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID" | jq \'{status, started_at, finished_at}\'',
          ].join('\n'),
          expectedOutput:
            'A run is queued, dispatched to a runner, executed step by step, possibly paused at an approval, then finalised with its deliverables.',
        },
      ],
      related: ['pipeline-anatomy', 'script-steps', 'approvals', 'pipeline-runs'],
      sources: [
        { repositoryPath: 'doc/runtime-flows.md', purpose: 'Step-by-step execution flows for each entry point.' },
        { repositoryPath: 'services/nopsai/pkg/validation/pipeline.go', purpose: 'The validation performed before a run is accepted.' },
      ],
    },
    {
      id: 'pipeline-runs',
      title: 'Pipeline runs',
      docType: 'how-to',
      audiences: ['operator', 'automation-author'],
      summary: 'Reading a run: the execution graph, step and task detail, outputs, and what each status means.',
      keywords: ['run', 'graph', 'status', 'warning', 'timed out', 'execution', 'detail'],
      keyFacts: [
        'Run detail separates Graph and Outputs tabs. Selecting a multi-task step reveals its task DAG below the step overview.',
        'A run finishes `success`, `failure`, `warning`, `timed_out`, or `canceled`.',
        '`warning` means work failed but was ignored: the failing task stays auditable as `failure (ignored)`.',
        '`timed_out` is what an expired approval timeout or run timeout produces.',
        'Run list surfaces expose only aggregate `final_output_status`; generated content stays on authorized detail paths.',
        'Pipeline detail is organized as Flow, Definition, Trigger rules, Runs, Health, and Dependencies tabs.',
      ],
      details: [
        'The expanded graph dialog supports scroll-zoom traversal of larger graphs, centers bounds when a run opens or Fit is pressed, and keeps the task reveal during zoom and pan. Unmodified wheel input still scrolls the page outside expanded graph mode.',
        'The Health tab owns the full pipeline analysis result and the AI Evaluation surface. The Analyse Pipeline header action switches there rather than opening a separate modal.',
        'Copy actions across pipeline YAML, step YAML, trigger YAML, knowledge content, credential references, run definition YAML, and run graph links all use the shared clipboard helper, so the selection-copy fallback works when the browser Clipboard API is unavailable.',
      ],
      prerequisites: [
        { label: 'Access', value: 'Read access to runs in the team that owns the pipeline', verification: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/auth/me" | jq .capabilities' },
        { label: 'A run to read', value: 'A run ID, from the run list or from the pipeline detail Runs tab' },
      ],
      steps: [
        {
          title: 'Start from the run list',
          description: 'Pipeline Runs and `GET /v1/runs` return the same records. Filter by team with `teamId`, or by branch with `branch`.',
          commands: [
            {
              title: 'List recent runs',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs?limit=20" | jq \'.[] | {run_id, pipeline_name, status}\'',
              placeholders: ['`$NOPSAI_URL` is the API address, `http://localhost:8080` on a local install.'],
            },
          ],
          expectedOutput: 'Runs newest first, each with a status of `success`, `failure`, `warning`, `timed_out`, or `canceled`.',
        },
        {
          title: 'Open the run and read the graph',
          description: 'Run detail carries the execution graph and per-step state. Selecting a multi-task step reveals its task graph below the step overview.',
          commands: [
            { title: 'Fetch run detail', language: 'bash', code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID" | jq' },
          ],
          verification: 'The failing step is identifiable by status before you open any logs.',
        },
        {
          title: 'Separate ignored failures from real ones',
          description: 'A `warning` run contains at least one failure that was ignored. Find it before concluding the run was fine.',
          expectedOutput: 'An ignored failure is recorded as `failure (ignored)` and stays auditable.',
        },
        {
          title: 'Act on the run',
          description: 'Re-running starts a fresh run from the same definition; cancelling stops one in flight.',
          commands: [
            {
              title: 'Rerun or cancel',
              language: 'bash',
              code: 'curl -sX POST -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID/rerun" | jq -r .runId\ncurl -sX POST -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID/cancel" | jq',
            },
          ],
          verification: 'The rerun appears in history as its own record; the cancelled run finishes `canceled`.',
        },
      ],
      related: ['run-lifecycle', 'pipeline-logs', 'analysis-reviewers', 'final-deliverables'],
      sources: [
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'Run lifecycle, approval, and output routes.' },
        { repositoryPath: 'doc/runtime-flows.md', purpose: 'Cancellation, rerun, and child pipeline flows.' },
      ],
    },
    {
      id: 'pipeline-logs',
      title: 'Pipeline logs and redaction',
      docType: 'reference',
      audiences: ['operator', 'security'],
      summary: 'Durable run records, what is masked, and what is deliberately left visible.',
      keywords: ['logs', 'redaction', 'masking', 'secrets', 'sensitive', 'audit'],
      keyFacts: [
        'Pipeline logs are durable run records, distinct from live System Logs.',
        'Agent-side masking covers declared secrets, sensitive variable names, and outputs marked `sensitive`.',
        'Non-sensitive operational evidence — environment names, versions, image references, change IDs, declared non-sensitive output JSON — stays visible for troubleshooting and release review.',
        'Durable ingestion applies best-effort credential-pattern redaction, including escaped JSON inside agent log messages, before writing `pipeline_run_logs`.',
      ],
      details: [
        'The distinction matters when investigating a leak: agent-side masking is intentional and driven by your declarations, while ingestion-side redaction is a best-effort safety net. Declaring a value as a secret or `sensitive` output is the reliable control.',
        'If a value you expected to be masked appears in logs, check whether it was declared under `secrets` or marked `sensitive: true` on the producing output.',
      ],
      examples: [
        {
          title: 'Read run logs, including a child pipeline',
          language: 'bash',
          code: [
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID/logs" \\',
            '  | jq -r \'.[] | "\\(.timestamp) \\(.step_name // "-") \\(.line)"\' | tail -40',
          ].join('\n'),
          expectedOutput:
            'Log records carry the text under `line`, with the step and task that produced it. Declared secrets and sensitive outputs arrive already masked.',
        },
      ],
      related: ['system-logs', 'pipeline-variables', 'step-outputs'],
      sources: [
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'Run log read and ingest routes.' },
      ],
    },
    {
      id: 'system-logs',
      title: 'System logs',
      docType: 'how-to',
      audiences: ['operator', 'administrator'],
      summary: 'Live allow-listed platform container and pod logs, streamed over authenticated SSE.',
      keywords: ['system logs', 'sse', 'stream', 'tail', 'docker', 'kubernetes', 'runner logs'],
      keyFacts: [
        'Docker deployments read through the restricted socket proxy; Kubernetes deployments use read-only `pods` and `pods/log` RBAC.',
        'Installed runners appear as `runner:<runner-id>` sources while the runner remains in dispatcher status.',
        'A runner source is marked unavailable until the configured provider can reach the Docker host or an owned pod in the namespace the runner advertises.',
        'Discovery follows registered `docker_container_name`, `kubernetes_namespace`, `kubernetes_label_selector`, and `nopsai_platform_id` metadata.',
        'Removed runner registrations hide old runner logs even if their containers or pods still exist.',
        'Runner source labels overlay dispatcher connection health, so a ready pod can still show `dispatcher unreachable` or `recently reconnected`.',
      ],
      details: [
        'Hybrid deployments use comma-separated providers such as `docker,kubernetes`. Reading logs from the opposite runtime still requires the API to have matching Kubernetes RBAC or a restricted Docker endpoint.',
        '`NOPSAI_PLATFORM_ID` keeps bundled runners and generated runner installs on the same ownership boundary, which is what prevents one platform from listing another platform runner logs.',
        'Buffer size, age, tail limits, line size, and stream counts are all bounded by configuration so a log viewer cannot exhaust the API.',
      ],
      prerequisites: [
        { label: 'Access', value: 'System log access, which is an administrator capability' },
        { label: 'Provider', value: 'A Docker or Kubernetes log provider configured for the install' },
        { label: 'Source', value: 'A source ID from the source list', verification: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/logs/sources" | jq' },
      ],
      steps: [
        {
          title: 'List what can be read',
          description: 'Sources are allow-listed. A container that is not a platform service is not a source, which is the point of the socket proxy.',
          commands: [
            { title: 'List sources', language: 'bash', code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/logs/sources" | jq \'.[] | {id, name}\'' },
          ],
          expectedOutput: 'One entry per platform service the provider exposes.',
        },
        {
          title: 'Tail a source',
          description: 'Tailing returns the recent buffer in one response, which is usually enough to answer "did this service start?".',
          commands: [
            { title: 'Tail recent lines', language: 'bash', code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/logs/sources/$SOURCE_ID/tail?limit=200"' },
          ],
          verification: 'Lines come back redacted: secret values are masked before they leave the platform.',
        },
        {
          title: 'Stream while reproducing',
          description: 'The stream endpoint is server-sent events and replays recent lines before following live output.',
          commands: [
            { title: 'Follow a source', language: 'bash', code: 'curl -N -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/logs/sources/$SOURCE_ID/stream"' },
          ],
          verification: 'New lines appear as the action you are reproducing happens.',
        },
        {
          title: 'Decide which log you actually need',
          description: 'System logs are platform services; run logs are what a pipeline produced. A run that never started is a system-log question.',
        },
      ],
      related: ['pipeline-logs', 'runners-and-dispatcher', 'networking-and-exposure'],
      sources: [
        { repositoryPath: 'doc/system-logs.md', purpose: 'Providers, SSE replay, AAA, redaction, limits, and monitoring.' },
        { repositoryPath: 'config/config.go', purpose: 'System logs provider and limit settings.' },
      ],
    },
    {
      id: 'monitoring',
      title: 'Monitoring and metrics',
      docType: 'reference',
      audiences: ['operator', 'administrator'],
      summary: 'Authorization-filtered analytics across runs, triggers, runners, AI usage, reliability, efficiency, and security.',
      keywords: ['monitoring', 'metrics', 'prometheus', 'analytics', 'alerts', 'recommendations'],
      keyFacts: [
        'Monitoring is authorization-filtered: you see the runs, pipelines, and resources you are allowed to see.',
        'Coverage spans runs, pipelines, steps, tasks, triggers, external triggers, runners, LLM usage, reliability, efficiency, and security.',
        '`GET /metrics` exposes Prometheus metrics including identity-provider capability and authorization grant ownership series.',
        'Metrics are public by default for scraper compatibility and can require bearer auth with `metrics_require_auth`.',
        'Alert rules can be created, evaluated on demand, and produce alert events.',
        'Recommendations can be acknowledged or resolved so the same finding does not resurface indefinitely.',
      ],
      details: [
        'AI usage telemetry records hashes, revision markers, cache and session identity, provider-state IDs and support, cached input tokens, cache-write tokens where providers expose them, and bounded workspace retrieval sizes — without storing prompt bodies.',
        'Saved monitoring views let a team keep a filter set rather than rebuilding it each time.',
      ],
      examples: [
        {
          title: 'Pull the operational summary',
          language: 'bash',
          code: [
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/monitoring/summary" | jq',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/monitoring/reliability" | jq',
            '',
            '# Prometheus scrape target; set METRICS_REQUIRE_AUTH=true to require a token',
            'curl -s "$NOPSAI_URL/metrics" | head -20',
          ].join('\n'),
          expectedOutput:
            'The monitoring routes answer product questions — reliability, efficiency, AI usage — while `/metrics` is the raw scrape surface.',
        },
      ],
      related: ['pipeline-runs', 'runners-and-dispatcher', 'analysis-reviewers', 'api-index'],
      sources: [
        { repositoryPath: 'services/nopsai/monitoring_analytics_schema.go', purpose: 'Analytics schema and filters.' },
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'The monitoring route surface.' },
      ],
    },
    {
      id: 'analysis-reviewers',
      title: 'Analysis reviewers and AI Evaluation',
      docType: 'how-to',
      audiences: ['operator', 'automation-author'],
      summary: 'Read-only analysis of pipelines, runs, and resources, with an optional AI-reviewed health score.',
      keywords: ['analysis', 'health score', 'ai evaluation', 'analyse', 'review', 'findings'],
      keyFacts: [
        'The health score starts at 100 and subtracts weighted visible findings: critical ×25, high ×15, medium ×8, low ×3, opportunity ×1.',
        'Category scores use the same weights filtered to that category.',
        'A structured AI Evaluation can replace the displayed number with an AI-reviewed score.',
        'Analysis is read-only. Analyse Run never reruns a failed step; it opens existing logs and resources or copies the diagnosis until a user chooses a separate confirmed action.',
        'AI Evaluation uses the authenticated `POST /v1/analysis/evaluate` endpoint with a usable model, a redacted reviewer snapshot, and bounded page-owned evidence.',
        'It does not create an Assistant conversation and does not ask hosted MCP to inspect the pipeline, team, resource, or run.',
      ],
      details: [
        'Structured AI reviews are cached in browser-local review history by subject type, subject ID, and snapshot revision. Reopening the same analysis shows the reviewed score immediately.',
        'When only an older same-subject review exists, the modal labels it as a previous-snapshot score and asks you to regenerate for current evidence.',
        'Deterministic analysis still works when Assistant debugging or model access is unavailable — you lose the AI-reviewed score, not the findings.',
        'AI Evaluation selects the configured default from the unscoped Assistant profile picker and still sends the team and resource scope path to the evaluation endpoint for policy validation.',
      ],
      examples: [
        {
          title: 'Why the score moved',
          language: 'text',
          code: [
            'Base                      100',
            '2 critical findings       -50   (2 x 25)',
            '1 high finding            -15   (1 x 15)',
            '3 medium findings         -24   (3 x 8)',
            '                          ----',
            'Displayed health score      11',
          ].join('\n'),
        },
      ],
      prerequisites: [
        { label: 'Feature enabled', value: 'AI evaluation enabled for the install; the route refuses otherwise' },
        { label: 'Model access', value: 'A model the caller is allowed to use in the target scope' },
        { label: 'A pipeline to analyse', value: 'A saved pipeline, opened at its Health tab' },
      ],
      steps: [
        {
          title: 'Open the Health tab',
          description: 'Pipeline detail owns the analysis result on Health. The Analyse Pipeline header action switches there rather than opening a separate modal.',
          verification: 'The tab shows the current analysis for the pipeline.',
        },
        {
          title: 'Run an evaluation',
          description: 'Evaluation is a request against the analysis surface, not a pipeline run: it does not consume runner capacity.',
          commands: [
            {
              title: 'Request an evaluation',
              language: 'bash',
              code: 'curl -sX POST "$NOPSAI_URL/v1/analysis/evaluate" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  --data @evaluation.json | jq',
              placeholders: ['`evaluation.json` carries the analysis payload the Health tab submits.'],
            },
          ],
          expectedOutput: 'A structured evaluation the Health tab renders alongside the analysis.',
        },
        {
          title: 'Read the result as advice, not a gate',
          description: 'An evaluation is a reviewer opinion. Enforcement comes from governance level, guardrail knowledge, and approvals, which are separate mechanisms.',
        },
      ],
      related: ['pipeline-runs', 'models', 'assistant', 'monitoring'],
      sources: [
        { repositoryPath: 'services/nopsai/analysis_evaluation_handlers.go', purpose: 'Evaluation endpoint, snapshot redaction, and policy validation.' },
        { repositoryPath: 'doc/feature-reference.md', purpose: 'Analysis reviewer behavior and scoring.' },
      ],
    },
    {
      id: 'dashboards',
      title: 'Team dashboards',
      docType: 'how-to',
      audiences: ['operator', 'automation-author', 'administrator'],
      summary: 'Team-owned dashboards fed by pipeline dashboard outputs, with publication history and scheduled refresh.',
      keywords: ['dashboard', 'chart', 'series', 'publication', 'refresh', 'sections', 'sources'],
      keyFacts: [
        'A dashboard has sections; pipeline `output` items publish entries into a named section.',
        'Publication modes are `replace` (default), `append`, `snapshot`, and `series`.',
        '`entry_key` is what lets `replace` and `series` find the same entry across runs; it defaults to the output item name.',
        'Source bindings connect a dashboard to the pipelines that feed it, which is what makes refresh possible.',
        'Refreshes can be scheduled, run on demand, canceled, and retried for only the failed parts.',
        'Creating a dashboard from the UI requires selecting at least one matching dashboard-output pipeline.',
      ],
      details: [
        'GitOps applies dependency roots before dependents, so team-owned dashboards and notification routes resolve their team paths before pipeline dashboard outputs attach to dashboards created in the same sync.',
        'Final-output rows for dashboard items can link directly to the configured dashboard and section when `dashboard_target` metadata is available.',
        '`ttl` marks an entry stale after a duration, which is how a dashboard shows "this number is old" rather than quietly presenting outdated state as current.',
      ],
      prerequisites: [
        { label: 'Access', value: 'Dashboard write access in the owning team' },
        { label: 'A source of data', value: 'A pipeline that publishes a `type: dashboard` final output, or an existing dashboard to read' },
      ],
      steps: [
        {
          title: 'Create the dashboard',
          description: 'A dashboard is a named container with sections. Pipelines publish entries into those sections.',
          commands: [
            {
              title: 'Create a dashboard',
              language: 'bash',
              code: 'curl -sX POST "$NOPSAI_URL/v1/dashboards" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"name":"service-health","team_path":"platform","sections":[{"key":"releases","title":"Releases"}]}\'',
              placeholders: ['`team_path` decides ownership and who can see the dashboard.'],
            },
          ],
          verification: 'The dashboard is listed by `GET /v1/dashboards`.',
        },
        {
          title: 'Publish into it from a pipeline',
          description: 'The publication contract lives in the pipeline, not in the dashboard: a final output item of `type: dashboard` names the dashboard, section, and entry key.',
          commands: [
            {
              title: 'Dashboard final output',
              language: 'yaml',
              code: 'output:\n  items:\n    - name: Release health\n      type: dashboard\n      when: success\n      prompt: Publish the release health entry for this service.\n      dashboard:\n        ref: platform/service-health\n        section: releases\n        entry_key: payments\n        mode: series',
            },
          ],
          expectedOutput: '`series` accumulates one entry per run under the entry key; `replace` keeps only the latest.',
        },
        {
          title: 'Check the publication history',
          description: 'Publication history is where a missing tile is diagnosed: it records what was published, when, and by which run.',
          verification: 'The most recent run appears in the dashboard publication history.',
        },
      ],
      related: ['final-deliverables', 'gitops-and-config-repositories', 'teams-and-ownership'],
      sources: [
        { repositoryPath: 'doc/dashboards.md', purpose: 'Dashboard model, publication, history, source bindings, and scheduled refresh.' },
        { repositoryPath: 'services/nopsai/dashboard_schema.go', purpose: 'Dashboard, section, and publication schema.' },
      ],
    },
    {
      id: 'notifications',
      title: 'Notifications',
      docType: 'how-to',
      audiences: ['administrator', 'operator'],
      summary: 'Team-routed mail notifications for run outcomes, approvals, and alerts.',
      keywords: ['notification', 'email', 'smtp', 'mail', 'alert', 'routing'],
      keyFacts: [
        'SMTP settings live in `setting/system/mail.yaml` and are managed through `/v1/system/notifications/mail`.',
        '`POST /v1/system/notifications/mail/test` sends a test message so delivery can be verified before an incident depends on it.',
        'Notification routes are owned per team through `/v1/teams/{teamID}/notifications`.',
        'Run notification lineage follows `run_team_path`, which is why schedules and external triggers set it.',
        'Branding is controlled by `NOPSAI_MAIL_LOGO_URL`, `NOPSAI_MAIL_WEBSITE_URL`, `NOPSAI_MAIL_SUPPORT_URL`, and `NOPSAI_MAIL_FOOTER_ADDRESS`.',
      ],
      details: [
        'Approval checkpoints are the most valuable notification target: a paused run costs nothing but blocks delivery until someone decides.',
        'Monitoring alert rules are a separate mechanism from run notifications. Rules evaluate metric conditions and produce alert events.',
      ],
      prerequisites: [
        { label: 'Mail settings', value: 'SMTP settings configured for the platform', verification: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/notifications/mail" | jq' },
        { label: 'Team', value: 'The team ID that should receive notifications' },
      ],
      steps: [
        {
          title: 'Configure delivery once, at the platform level',
          description: 'Mail settings are a system concern. Teams choose what they are told about, not how it is delivered.',
          commands: [
            {
              title: 'Read and update mail settings',
              language: 'bash',
              code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/notifications/mail" | jq\ncurl -sX PUT "$NOPSAI_URL/v1/system/notifications/mail" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/json" \\\n  --data @mail-settings.json',
              placeholders: ['`mail-settings.json` carries the SMTP host, port, credentials reference, and sender address.'],
            },
          ],
        },
        {
          title: 'Send a test before trusting it',
          description: 'The test route exercises the real delivery path, which is the only way to know the credentials and sender are accepted.',
          commands: [
            { title: 'Send a test message', language: 'bash', code: 'curl -sX POST "$NOPSAI_URL/v1/system/notifications/mail/test" -H "Authorization: Bearer $NOPSAI_TOKEN" | jq' },
          ],
          verification: 'The test message arrives, or the response names the delivery failure.',
        },
        {
          title: 'Subscribe the team',
          description: 'Notification settings hang off the team, so ownership decides who hears about a run.',
          commands: [
            { title: 'Read team notifications', language: 'bash', code: 'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/teams/$TEAM_ID/notifications" | jq' },
          ],
          verification: 'The team lists the events it subscribes to.',
        },
      ],
      related: ['teams-and-ownership', 'approvals', 'monitoring'],
      sources: [
        { repositoryPath: 'services/nopsai/notification_schema.go', purpose: 'Notification route and mail settings schema.' },
      ],
    },
    {
      id: 'release-integrity',
      title: 'Release integrity and updates',
      docType: 'reference',
      audiences: ['administrator', 'operator'],
      summary: 'How releases are tagged and published, and how the CLI verifies what it downloads.',
      keywords: ['release', 'version', 'update', 'ghcr', 'checksum', 'compatibility', 'upgrade'],
      keyFacts: [
        '`scripts/release-tags.sh` publishes the stable tag set: exact version, `latest`, major, and major.minor.',
        'Container images and the Helm OCI chart publish all four aliases; installers keep exact versions in generated Compose, Helm values, and locks.',
        'CLI archives and `SHA256SUMS` publish to the `nopsai-cli` GHCR OCI package, which must be public so `nopsai update --version <x.y.z>` does not need repository release access.',
        '`nopsai update` downloads the exact OCI package archive and `SHA256SUMS`, verifies the checksum, then replaces the local binary.',
        '`nopsai platform upgrade` moves an installed platform forward: it reads the install or deployment lock, keeps generated secrets, and blocks a compatibility-series upgrade until the changelog is acknowledged with `--accept-series-upgrade`.',
        '`release/compatibility.yaml` is read into buildinfo linker flags and Docker build args so binaries advertise the current platform, runner, API, and capability contract.',
        'Enterprise mirrors can override the package, repository, or asset base URL without changing the archive naming contract.',
      ],
      details: [
        'Multi-arch container builds carry `org.opencontainers.image.source` as both Dockerfile labels and OCI index/manifest annotations so GHCR can associate packages with the source repository. Already-created unlinked packages may still need a one-time settings connection or a delete and republish.',
        'The exact GitHub Release is marked latest, and moving `v<major>` and `v<major>.<minor>` release aliases carry compatibility copies of the same CLI assets.',
        'Repository-owned platform release automation lives in `release/nopsai-platform-release.yaml`.',
      ],
      examples: [
        {
          title: 'Check what version is running and what it claims',
          language: 'bash',
          code: [
            'curl -s "$NOPSAI_URL/version" | jq',
          ].join('\n'),
          expectedOutput:
            'Product and API versions, supported CLI and runner ranges, capability IDs, and the release manifest digest when one is present. No deployment configuration and no credentials.',
        },
      ],
      related: ['deployment-models', 'cli', 'known-limits'],
      sources: [
        { repositoryPath: 'doc/release-bundles.md', purpose: 'Shared build identity, versioned assets, and GitOps release locks.' },
        { repositoryPath: 'release/compatibility.yaml', purpose: 'Platform, runner, API, and capability contract.' },
      ],
    },
    {
      id: 'production-hardening',
      title: 'Production hardening checklist',
      docType: 'how-to',
      audiences: ['administrator', 'security'],
      summary: 'What to change before a deployment stops being a local evaluation.',
      keywords: ['hardening', 'production', 'security', 'checklist', 'gates'],
      keyFacts: [
        'Set `NOPSAI_REQUIRE_PRODUCTION_GATES=true` so an unsafe configuration fails startup instead of running.',
        'Set `DISPATCHER_TLS_MODE` to `mtls` or `tls`. Never `disabled`.',
        'Generate distinct high-entropy values for `JWT_SIGNING_KEY` and `SERVICE_JWT_SIGNING_KEY`.',
        'Replace every Compose fallback secret, and leave `NOPSAI_BOOTSTRAP_ADMIN_ALLOW_DEFAULT_PASSWORD` false.',
        'Keep `NOPSAI_BOOTSTRAP_ADMIN_MUST_CHANGE_PASSWORD` true so the provisioned password rotates at first login.',
        'Point `SYSTEM_LOGS_DOCKER_HOST` at the restricted socket proxy, never the raw Docker socket.',
        'Set `METRICS_REQUIRE_AUTH=true` when `/metrics` is reachable outside the cluster network.',
        'Mount `DATA_BACKUP_DIR` as durable storage — the default Compose topology does not.',
      ],
      details: [
        'Rotate `SERVICE_JWT_SIGNING_KEY` and `DISPATCHER_TLS_SECRET` together before exposing a replacement dispatcher. Keeping the old values lets retired runner definitions authenticate again unless their IDs are carried in `ejected_runner_ids`.',
        'Keep step pods on the no-RBAC workload service account. Merging it with the runner service account gives workload code access to the Kubernetes API.',
        'Review egress: Docker step containers default to bridge networking. Use `DOCKER_NETWORK_NAME=none` or a dedicated network when workloads should not reach the internet.',
        'Prefer credential references over the legacy inline secret variables for GitHub App keys, webhook secrets, LLM keys, and MCP tokens.',
      ],
      prerequisites: [
        { label: 'Environment', value: 'A non-Compose deployment: a release bundle or the Helm chart' },
        { label: 'Secret store', value: 'A place to hold the platform secrets that is not a local `.env`' },
        { label: 'Administrator access', value: 'Enough access to change system settings and identity configuration' },
      ],
      steps: [
        {
          title: 'Replace every bootstrap value',
          description: 'The values that make a local install convenient are the ones that make a shared install dangerous. Rotate them all before anyone else connects.',
          commands: [
            { title: 'Values that must be install-specific', language: 'bash', code: 'NOPSAI_MASTER_KEY\nJWT_SIGNING_KEY\nSERVICE_JWT_SIGNING_KEY\nAAA_SHARED_INTERNAL_TOKEN\nPOSTGRES_PASSWORD\nNOPSAI_BOOTSTRAP_ADMIN_PASSWORD' },
          ],
          warning: '`JWT_SIGNING_KEY` and `SERVICE_JWT_SIGNING_KEY` must differ, or a user token can be replayed as a service token.',
        },
        {
          title: 'Close the network surface',
          description: 'Only the Git webhook ingress needs to be reachable from outside. The API, dispatcher, and database do not.',
          verification: 'From outside the cluster or host, the dispatcher gRPC port and PostgreSQL port do not answer.',
        },
        {
          title: 'Move identity off local accounts',
          description: 'Configure an identity provider and reduce local accounts to break-glass. Provider ID, issuer, and subject are the identity; email is metadata.',
          verification: 'Signing in through the provider produces a session with the expected roles.',
        },
        {
          title: 'Confirm the startup gates pass',
          description: 'Production startup gates refuse to start a service that is still carrying development defaults. A clean start is the check.',
          commands: [
            { title: 'Check platform identity and health', language: 'bash', code: 'curl -s "$NOPSAI_URL/version" | jq\ncurl -s "$NOPSAI_URL/healthz"' },
          ],
          verification: 'Every service starts, `/healthz` is ready, and no gate error appears in the system logs.',
        },
      ],
      related: ['environment-index', 'networking-and-exposure', 'credentials', 'access-control'],
      sources: [
        { repositoryPath: 'doc/enterprise-gates.md', purpose: 'Production startup gates and verification commands.' },
        { repositoryPath: 'pkg/startupgates/startupgates.go', purpose: 'What the gates actually check.' },
      ],
    },
    {
      id: 'troubleshooting',
      title: 'Troubleshooting index',
      docType: 'troubleshooting',
      audiences: ['operator', 'administrator', 'automation-author'],
      summary: 'Symptom to likely cause, with the page that explains the fix.',
      keywords: ['troubleshooting', 'debug', 'problem', 'error', 'diagnose', 'fix'],
      keyFacts: [
        '**The UI will not let anyone in** — check `GET /v1/setup/preflight`; it names the blocker without a token.',
        '**`/healthz` never becomes ready** — the API is retrying an unreachable database. `/livez` still answers.',
        '**A run stays queued** — no reachable dispatch-enabled runner matches the run scope. Check the dispatcher fleet view.',
        '**A Git push starts nothing** — compare event name, branch, and changed paths against the trigger rule, then check deliveries.',
        '**A goal fails immediately** — test the Model; an unconfigured profile, a scope restriction, and an AAA denial produce different messages.',
        '**A pipeline is rejected on save** — validation names the exact step, task, or directive. Read the message before editing the graph.',
        '**PDF output fails** — no reachable Gotenberg at `FINAL_OUTPUT_PDF_RENDERER_URL`.',
        '**Runner logs show unavailable** — the System Logs provider cannot reach the Docker host or an owned pod in the runner namespace.',
        '**A GitOps change was reverted** — a UI or API edit created a database override; push the change back to the owning repository.',
        '**`ImagePullBackOff` on step pods** — a missing or wrong `imagePullSecret`, not a NopsAI credential assignment.',
      ],
      details: [
        'Two error classes are easy to confuse. A validation error happens before anything is queued and always names a directive. A runtime failure happens during execution and appears in the run logs against a specific step or task.',
        'When a value you expected to be masked appears in logs, check whether it was declared under `secrets` or marked `sensitive: true` — agent-side masking is driven by your declarations.',
      ],
      examples: [
        {
          title: 'Triage in the order that narrows fastest',
          language: 'bash',
          code: [
            '# 1. is the control plane healthy at all?',
            'curl -s "$NOPSAI_URL/livez"; curl -s "$NOPSAI_URL/healthz"',
            '',
            '# 2. is there anywhere to run work?',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/dispatcher" | jq \'.runners\'',
            '',
            '# 3. did the run start, and what did it say?',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID" | jq \'.status\'',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/runs/$RUN_ID/logs" | jq -r \'.[].line\' | tail -30',
            '',
            '# 4. if the run never started, the answer is in the platform logs',
            'curl -s -H "Authorization: Bearer $NOPSAI_TOKEN" "$NOPSAI_URL/v1/system/logs/sources" | jq',
          ].join('\n'),
          expectedOutput:
            'Each step rules out a layer. A run that never started is never a pipeline problem.',
        },
      ],
      related: ['browser-console-troubleshooting', 'known-limits', 'system-logs', 'pipeline-runs'],
      sources: [
        { repositoryPath: 'doc/runtime-flows.md', purpose: 'Where each stage of a run can fail.' },
      ],
    },
    {
      id: 'browser-console-troubleshooting',
      title: 'Browser console warnings',
      docType: 'troubleshooting',
      audiences: ['operator', 'developer'],
      summary: 'Telling injected browser-extension noise apart from real NopsAI UI errors.',
      keywords: ['console', 'devtools', 'extension', 'warning', 'objectmultiplex', 'contentscript'],
      keyFacts: [
        'A stack pointing at `contentscript.js` and mentioning `app-init-liveness` or `background-liveness` is almost always injected extension code, not NopsAI.',
        '`ObjectMultiplex` warnings come from wallet and similar extensions.',
        'Reproduce in a clean browser profile with extensions disabled before filing a UI bug.',
        'A genuine NopsAI error has a stack frame in a NopsAI source file.',
      ],
      details: [
        'The backend serves `GET /favicon.ico` as an empty cacheable response specifically so missing-favicon requests do not produce bearer token errors in the console or audit logs. A favicon 404 is therefore not expected and is worth investigating.',
      ],
      runbooks: [
        {
          id: 'triage-console-warning',
          title: 'Triage an unexplained browser console warning',
          symptoms: [
            'DevTools shows repeated warnings or errors while using the NopsAI UI.',
            'The stack trace does not obviously name a NopsAI source file.',
          ],
          impact: 'Usually none. The risk is spending time on injected extension code instead of a real defect.',
          requiredAccess: 'Browser access to the NopsAI UI. No platform permission needed.',
          initialChecks: [
            'Expand the stack trace and read the top frame file name.',
            'Check whether the frame is `contentscript.js` or another injected script.',
            'Note whether the message mentions `app-init-liveness`, `background-liveness`, or `ObjectMultiplex`.',
          ],
          diagnostics: [
            'Open the same page in a clean browser profile with all extensions disabled.',
            'Reload with DevTools open and the console cleared.',
            'Compare the console output between the two profiles.',
          ],
          resolution: [
            'If the warning disappears in the clean profile, it is injected extension code. No NopsAI change is needed.',
            'If it reproduces with a NopsAI source-file stack frame, capture the full stack, the route, and the reproduction steps.',
            'File the UI defect with that evidence rather than the original extension-polluted trace.',
          ],
          escalation: 'Escalate to the UI owners with the clean-profile stack trace and route.',
        },
      ],
      examples: [
        {
          title: 'Separate extension noise from product errors',
          language: 'text',
          code: [
            'DevTools > Console > filter box:',
            '  -chrome-extension://  -moz-extension://  -safari-web-extension://',
            '',
            'DevTools > Network: reproduce with "Disable cache" checked,',
            'then confirm the failing request is same-origin with the NopsAI API',
            'before reporting it as a product bug.',
          ].join('\n'),
          expectedOutput:
            'Most console warnings on the operator UI come from injected extension content scripts. Filtering them first is what makes the remaining messages meaningful.',
        },
      ],
      related: ['troubleshooting'],
      sources: [
        { repositoryPath: 'doc/browser-console-troubleshooting.md', purpose: 'How to triage DevTools warnings from injected content scripts.' },
      ],
    },
  ],
};
