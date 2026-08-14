import type { WikiSection } from '../types.js';

/**
 * The three index articles carry no `fields` of their own: they are rendered by
 * dedicated index components that aggregate every documented field across the
 * whole wiki. See `indexes.ts`.
 */
export const referenceSection: WikiSection = {
  id: 'reference',
  title: 'Reference and limits',
  group: 'look-up',
  owner: 'Platform engineering',
  description: 'Complete indexes across the whole product, the hardening checklist, and the confirmed gaps.',
  articles: [
    {
      id: 'directive-index',
      title: 'All YAML directives',
      docType: 'reference',
      audiences: ['automation-author', 'developer'],
      summary: 'Every documented directive across pipeline, step, task, approval, output, trigger, schedule, and profile YAML, in one searchable table.',
      keywords: ['index', 'directives', 'all', 'yaml', 'search', 'fields', 'schema'],
      keyFacts: [
        'This index is generated from the same field data the individual reference pages render, so it can never drift from them.',
        'Filter by scope to narrow to pipeline, step, task, approval, output, trigger, schedule, or profile directives.',
        'Every row links to the article that explains the directive in context.',
      ],
      details: [
        'If a directive is missing here, it is missing from the wiki. That is a documentation gap worth reporting rather than a directive that does not exist.',
      ],
      related: ['pipeline-schema', 'step-task-directives', 'final-deliverables', 'environment-index'],
      sources: [
        { repositoryPath: 'services/nopsai/pkg/validation/pipeline.go', purpose: 'Authoritative validation for pipeline, step, task, approval, and output directives.' },
        { repositoryPath: 'pkg/models/model.go', purpose: 'Struct definitions and YAML tags.' },
      ],
    },
    {
      id: 'environment-index',
      title: 'All environment variables',
      docType: 'reference',
      audiences: ['administrator', 'operator', 'security'],
      summary: 'Every documented environment variable with its owning service, default, and security note.',
      keywords: ['index', 'environment', 'env', 'variables', 'settings', 'configuration', 'all'],
      keyFacts: [
        'Grouped by owning service so you can see what one component actually needs.',
        'Required values are the ones a deployment cannot start without.',
        'Security notes call out the values that must be rotated together or must never be reused across environments.',
      ],
      details: [
        'Bootstrap values are deliberately not the long-term source of truth. Once GitOps is loaded, most product settings move to configuration files under `setting/system/`.',
      ],
      related: ['environment-reference', 'production-hardening', 'docker-compose', 'helm-kubernetes'],
      sources: [
        { repositoryPath: 'config/config.go', purpose: 'Config struct with yaml and env tags.' },
        { repositoryPath: 'docker-compose.yaml', purpose: 'Local defaults and required-value enforcement.' },
      ],
    },
    {
      id: 'api-index',
      title: 'All REST endpoints',
      docType: 'reference',
      audiences: ['developer', 'operator', 'administrator'],
      summary: 'Every documented route with method, access class, and purpose, grouped by area.',
      keywords: ['index', 'api', 'routes', 'endpoints', 'rest', 'all', 'http'],
      keyFacts: [
        '**Public** routes need no token. **Authenticated** routes need any valid token. **Authorized** routes additionally pass an AAA check on the target resource.',
        '**Administrator** routes require the platform administrator role.',
        '**Service token** routes are internal component traffic and are not part of the public surface.',
        'Filter by area or search by path fragment to find the route you need.',
      ],
      details: [
        'Access class describes the gate before resource-level checks. An "authorized" route can still return an empty list if the caller has no matching resources.',
      ],
      related: ['api', 'access-control', 'tokens-and-service-accounts'],
      sources: [
        { repositoryPath: 'services/nopsai/routes.go', purpose: 'Route registration for every area.' },
        { repositoryPath: 'doc/api.md', purpose: 'Request and response shapes.' },
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
      related: ['troubleshooting'],
      sources: [
        { repositoryPath: 'doc/browser-console-troubleshooting.md', purpose: 'How to triage DevTools warnings from injected content scripts.' },
      ],
    },
    {
      id: 'known-limits',
      title: 'Confirmed gaps and limits',
      docType: 'reference',
      audiences: ['administrator', 'security', 'new-user'],
      summary: 'Capabilities this repository does not implement. Valid roadmap items, but not current behavior.',
      keywords: ['limits', 'gaps', 'not supported', 'roadmap', 'unsupported', 'boundaries'],
      keyFacts: [
        'No Terraform modules.',
        'No AWS-, Azure-, or Google-specific deployment automation.',
        'No built-in Redis dependency.',
        'No S3 or generic object-storage artifact or backup backend.',
        'No Helm-managed PostgreSQL beyond the bundled StatefulSet.',
        'No documented HPA or autoscaling implementation.',
        'No documented Kubernetes NetworkPolicy set.',
        'No complete air-gap installation workflow.',
        'No product-managed backup upload to object storage.',
        'No verified product-managed restore workflow.',
        'No published production sizing or throughput benchmark.',
        'No automatic certificate-management integration.',
        'No multi-region database or control-plane architecture.',
        'Raising every replica count does not by itself produce high availability.',
      ],
      details: [
        'This page exists so a sales claim, a design document, or an architecture review can be checked against implementation evidence. Anything listed here should not appear elsewhere in the wiki as implemented behavior.',
        'These are legitimate roadmap items. The distinction the wiki maintains is between what the current repository does and what is planned.',
      ],
      related: ['data-management', 'storage-and-persistence', 'deployment-models'],
      sources: [
        { repositoryPath: 'doc/enterprise-refactor-roadmap.md', purpose: 'Where future-state capability is tracked.' },
        { repositoryPath: 'doc/README.md', purpose: 'The code-grounded documentation set this wiki summarizes.' },
      ],
    },
  ],
};
