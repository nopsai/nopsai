import type { WikiSection } from '../types.js';

export const getStartedSection: WikiSection = {
  id: 'get-started',
  title: 'Get started',
  group: 'learn',
  owner: 'Product',
  description: 'A first-run path: install locally, finish setup, then build a script pipeline, an AI goal, a Git trigger, an approval, and a deliverable.',
  articles: [
    {
      id: 'install-local-docker-compose',
      title: 'Install locally with Docker Compose',
      docType: 'tutorial',
      audiences: ['new-user', 'developer', 'administrator'],
      summary: 'Start a local stack, confirm the control-plane services, and verify that the Docker runner registers.',
      keywords: ['install', 'local', 'compose', 'evaluation', 'quickstart'],
      keyFacts: [
        'The checked-in Compose file is for local evaluation and development, not production.',
        'The UI is published on http://localhost/ and the API on http://localhost:8080.',
        'The Docker runner needs Docker socket access because it creates agent and step containers. Treat the runner host as a trusted execution boundary.',
        'Compose fails fast if the required bootstrap secrets are missing, so set them in `.env` before starting.',
      ],
      prerequisites: [
        { label: 'Repository', value: 'A current checkout of the NopsAI repository', verification: 'git rev-parse --short HEAD' },
        { label: 'Runtime', value: 'Docker 26+ with Compose v2', verification: 'docker compose version' },
        { label: 'Ports', value: '80, 8080, 9091, and 5432 free on the workstation' },
        { label: 'Permission', value: 'Local Docker access for the current user', verification: 'docker ps' },
      ],
      steps: [
        {
          title: 'Set the required bootstrap secrets',
          description:
            'Compose refuses to start without them. Generate high-entropy values rather than reusing anything from another environment.',
          commands: [
            {
              title: 'Create .env',
              language: 'bash',
              code: [
                'cat >> .env <<EOF',
                'POSTGRES_PASSWORD=$(openssl rand -hex 16)',
                'DATABASE_URL=postgres://nopsai:$(grep POSTGRES_PASSWORD .env | cut -d= -f2)@db:5432/nopsai?sslmode=disable',
                'NOPSAI_MASTER_KEY=$(openssl rand -hex 32)',
                'JWT_SIGNING_KEY=$(openssl rand -hex 32)',
                'SERVICE_JWT_SIGNING_KEY=$(openssl rand -hex 32)',
                'AAA_SHARED_INTERNAL_TOKEN=$(openssl rand -hex 32)',
                'NOPSAI_BOOTSTRAP_ADMIN_PASSWORD=$(openssl rand -hex 12)',
                'EOF',
              ].join('\n'),
              placeholders: ['Adjust the database user and host if you changed the Compose defaults.'],
            },
          ],
          warning: 'JWT_SIGNING_KEY and SERVICE_JWT_SIGNING_KEY must be different values.',
        },
        {
          title: 'Build and start the stack',
          description: 'Run the Compose topology from the repository root.',
          commands: [{ title: 'Start Compose', language: 'bash', code: 'docker compose up -d --build' }],
          expectedOutput:
            'Compose creates the nopsai-net network and starts PostgreSQL, API, AAA, dispatcher, git-bot, UI, Gotenberg, the socket proxy, and the Docker runner.',
        },
        {
          title: 'Check service health',
          description: 'Confirm the core services are up before opening the setup flow.',
          commands: [
            {
              title: 'Inspect local services',
              language: 'bash',
              code: 'docker compose ps\ncurl -s localhost:8080/livez\ncurl -s localhost:8080/healthz',
            },
          ],
          verification: '`/livez` answers immediately; `/healthz` becomes ready once PostgreSQL is reachable.',
        },
        {
          title: 'Follow the setup logs',
          description: 'Keep the services that own first-run setup and runner registration visible while you complete the wizard.',
          commands: [
            { title: 'Follow logs', language: 'bash', code: 'docker compose logs -f nopsai aaa dispatcher docker-runner' },
          ],
          verification: 'Open http://localhost/ and confirm the setup page loads.',
        },
      ],
      details: [
        'The Compose stack is optimized for local inspection. It is the fastest way to validate product behavior before moving to a release bundle or a Helm deployment.',
        'Everything you configure through the wizard can later be exported to a configuration repository, so a local evaluation is not throwaway work.',
      ],
      examples: [
        {
          title: 'Tear the stack down again',
          language: 'bash',
          code: 'docker compose down\n# add -v to also drop the database volume\ndocker compose down -v',
          expectedOutput: 'All containers stop and the nopsai-net network is removed.',
        },
      ],
      limits: ['Local fallback secrets exist for development only and must be replaced outside a workstation.'],
      related: ['complete-first-install-wizard', 'docker-compose', 'production-hardening'],
      sources: [
        { repositoryPath: 'docker-compose.yaml', purpose: 'The exact local topology, ports, and environment variables.' },
        { repositoryPath: 'doc/enterprise-gates.md', purpose: 'Startup gates that separate local defaults from production requirements.' },
      ],
    },
    {
      id: 'complete-first-install-wizard',
      title: 'Complete the first-install wizard',
      docType: 'tutorial',
      audiences: ['new-user', 'administrator'],
      summary: 'Run the one-time bootstrap that unlocks the platform, creates the first administrator, and seeds GitOps.',
      keywords: ['setup', 'bootstrap', 'wizard', 'first install', 'onboarding'],
      keyFacts: [
        'Normal authenticated routes and APIs stay locked until setup completes once.',
        '`GET /v1/setup/preflight` reports exactly what still blocks setup, including a database that is still starting.',
        'The bootstrap administrator must rotate the provisioned password at first login by default.',
        'The wizard can seed starter profiles, generate secrets, and create the initial GitOps layout and repository teams.',
      ],
      prerequisites: [
        { label: 'Stack', value: 'A running control plane', verification: 'curl -s localhost:8080/healthz' },
        { label: 'Bootstrap admin', value: 'NOPSAI_BOOTSTRAP_ADMIN_EMAIL and NOPSAI_BOOTSTRAP_ADMIN_PASSWORD set' },
        { label: 'Database', value: 'PostgreSQL reachable from the API' },
        { label: 'Browser access', value: 'http://localhost/ reachable from your workstation' },
      ],
      steps: [
        {
          title: 'Check preflight',
          description: 'Preflight tells you whether the platform can be set up right now and, if not, why.',
          commands: [{ title: 'Read preflight', language: 'bash', code: 'curl -s localhost:8080/v1/setup/preflight | jq' }],
          expectedOutput: 'A JSON document listing outstanding blockers. An empty blocker list means setup can proceed.',
        },
        {
          title: 'Run the wizard',
          description: 'Open the UI and work through the setup steps: administrator, secrets, starter profiles, and GitOps seeding.',
          verification: '`GET /v1/setup/status` reports that setup has completed.',
          warning:
            'Setup apply errors include the actionable write or configuration reason. Read the message rather than retrying blindly.',
        },
        {
          title: 'Rotate the bootstrap password',
          description: 'Log in as the bootstrap administrator. The forced rotation runs before you reach the application.',
          verification: 'You can log in with the new password and reach the dashboard.',
        },
        {
          title: 'Confirm the runner registered',
          description: 'Open System, then the dispatcher workspace, and confirm at least one runner is connected and dispatchable.',
          verification: 'The runner appears in the fleet table with a reachable, dispatch-enabled status.',
        },
      ],
      details: [
        'Setup preflight is deliberately available before authentication so an operator can diagnose a stuck install without a token.',
        'First-install setup does not force an env-file write for dispatcher TLS when a valid effective service JWT fallback is already configured.',
      ],
      related: ['first-install-wizard', 'authentication-and-sso', 'gitops-and-config-repositories'],
      sources: [
        { repositoryPath: 'doc/first-install-wizard.md', purpose: 'The bootstrap flow, starter profiles, and production guardrails.' },
        { repositoryPath: 'services/nopsai/setup_preflight.go', purpose: 'Preflight behavior during cold starts.' },
      ],
    },
    {
      id: 'first-script-pipeline',
      title: 'Create and run your first script pipeline',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author'],
      summary: 'Author a minimal deterministic pipeline, validate it, run it, and read the result.',
      keywords: ['first pipeline', 'script', 'hello world', 'tutorial'],
      keyFacts: [
        'A script-only pipeline needs no LLM configuration at all — set `llm_enabled: false` and the validator enforces it.',
        'Validation runs before anything is queued, so a bad pipeline fails immediately with a specific message.',
        '`container_image` is required unless every executable step sets its own `image`.',
      ],
      prerequisites: [
        { label: 'Setup', value: 'First-install setup completed' },
        { label: 'Runner', value: 'At least one connected, dispatchable runner' },
        { label: 'Permission', value: 'Create access on the target team path' },
        { label: 'Scope', value: 'A runtime scope you can run in' },
      ],
      steps: [
        {
          title: 'Write the pipeline',
          description: 'Two steps, one dependency, no AI.',
          commands: [
            {
              title: 'hello.yaml',
              language: 'yaml',
              code: [
                'name: hello-world',
                'description: Smallest useful NopsAI pipeline.',
                'container_image: alpine:3.20',
                'llm_enabled: false',
                'steps:',
                '  - name: greet',
                '    script: echo "hello from $(hostname)"',
                '  - name: report',
                '    depends_on: [greet]',
                '    script: echo "greet finished"',
              ].join('\n'),
            },
          ],
        },
        {
          title: 'Validate before saving',
          description: 'The validate route checks schema, limits, and the dependency graph without persisting anything.',
          commands: [
            {
              title: 'Validate',
              language: 'bash',
              code: 'curl -sX POST localhost:8080/v1/pipelines/validate \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -H "Content-Type: application/yaml" \\\n  --data-binary @hello.yaml',
              placeholders: ['$NOPSAI_TOKEN — a personal access token from Profile.'],
            },
          ],
          expectedOutput: 'A success response. Errors name the exact step, task, or directive at fault.',
        },
        {
          title: 'Save and run it',
          description: 'Create the pipeline through the UI or API, then execute it.',
          commands: [
            {
              title: 'Run',
              language: 'bash',
              code: 'curl -sX POST localhost:8080/v1/run/hello-world \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
            },
          ],
          verification: 'The run appears under Pipeline Runs and finishes with status `success`.',
        },
      ],
      details: [
        'Execution order comes from `depends_on`, not from list order. Remove the dependency and both steps run concurrently.',
        'Open the run detail Graph tab to see the executed graph, then Outputs for anything the run produced.',
      ],
      related: ['pipeline-schema', 'step-task-directives', 'first-ai-assisted-pipeline', 'pipeline-runs'],
      sources: [
        { repositoryPath: 'examples/gitops-quickstart/README.md', purpose: 'Copyable GitOps sample with pipelines, steps, scopes, and a dashboard.' },
      ],
    },
    {
      id: 'first-ai-assisted-pipeline',
      title: 'Create your first AI-assisted pipeline',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author'],
      summary: 'Add an LLM-backed goal to a pipeline and understand which control layer decides what.',
      keywords: ['ai', 'goal', 'llm', 'first ai pipeline'],
      keyFacts: [
        'A goal needs a usable Model. Configure provider, model, and credential reference before authoring.',
        '`agent_role` selects the persona; `model` selects the provider and model. They are independent.',
        'AAA still decides whether the original caller may use the profile you named.',
        '`governance_level` defaults to `strict`, which proceeds only on a clear policy allow.',
      ],
      prerequisites: [
        { label: 'Model', value: 'At least one configured profile with a working credential', verification: 'Use the profile Test action in Models.' },
        { label: 'Scope access', value: 'The profile must be allowed in your run scope' },
        { label: 'Pipeline', value: 'A working script pipeline from the previous tutorial' },
        { label: 'Permission', value: 'Use access on the selected profile' },
      ],
      steps: [
        {
          title: 'Test the Model first',
          description: 'Confirm the provider and credential work before a run depends on them.',
          commands: [
            {
              title: 'Test the profile',
              language: 'bash',
              code: 'curl -sX POST localhost:8080/v1/system/models/standard/test \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
            },
          ],
          expectedOutput: 'A successful test result. A failure here will fail every goal that uses the profile.',
        },
        {
          title: 'Add a goal step',
          description: 'Mix deterministic script work with an LLM goal in the same graph.',
          commands: [
            {
              title: 'release-notes.yaml',
              language: 'yaml',
              code: [
                'name: release-notes',
                'container_image: alpine/git:latest',
                'model: standard',
                'steps:',
                '  - name: collect',
                '    script: git log --oneline -20 > /workspace/commits.txt',
                '  - name: summarize',
                '    depends_on: [collect]',
                '    goal: Read /workspace/commits.txt and write user-facing release notes to /workspace/NOTES.md.',
              ].join('\n'),
            },
          ],
          warning:
            'Without `llm_content_preload: true`, workspace contents are not shared automatically. The agent can still retrieve files on demand through bounded workspace tools.',
        },
        {
          title: 'Run it and read the reasoning',
          description: 'Open the run, select the goal task, and inspect its log to see how the goal was resolved.',
          verification: 'The task completes and NOTES.md exists in the workspace for downstream steps.',
        },
      ],
      details: [
        'If the run is rejected before execution, check the message: an unconfigured profile, a scope restriction, and an AAA denial all produce different, specific errors.',
        'Add `mcp_profiles` only when the goal genuinely needs external tools. Script steps and include steps cannot use them.',
      ],
      related: ['ai-control-layers', 'models', 'agent-roles', 'governance-and-policy'],
      sources: [
        { repositoryPath: 'doc/llm-model-selection.md', purpose: 'Provider and model selection notes.' },
      ],
    },
    {
      id: 'connect-git-repository',
      title: 'Connect a Git repository',
      docType: 'tutorial',
      audiences: ['new-user', 'developer', 'administrator'],
      summary: 'Give NopsAI access to a repository so it can read pipelines, knowledge, and trigger manifests.',
      keywords: ['git', 'github', 'repository', 'connect', 'integration'],
      keyFacts: [
        'GitHub uses a GitHub App with one or more installations, configured in `setting/git-apps/github.yaml`.',
        'GitLab, Bitbucket, Gitea, and generic providers use managed Git Webhook Sources instead.',
        'Repository access is what lets a run check out code and read repo-local Knowledge Context at the run commit.',
      ],
      prerequisites: [
        { label: 'Provider', value: 'A repository on GitHub, GitLab, Bitbucket, or Gitea' },
        { label: 'Permission', value: 'Administrator access to configure Git Apps or webhook sources' },
        { label: 'Reachability', value: 'The provider must be able to reach your NopsAI webhook endpoint' },
        { label: 'Credentials', value: 'A GitHub App private key, or a webhook signing secret for other providers' },
      ],
      steps: [
        {
          title: 'Register the App or webhook source',
          description:
            'For GitHub, add the App ID and private key credential reference, then add the installation. For other providers, create a Git Webhook Source and copy its signing secret into the provider.',
          verification: 'Use the installation Verify action, or check recent deliveries on the webhook source.',
        },
        {
          title: 'Confirm repository visibility',
          description: 'List the repositories the installation can reach.',
          commands: [
            {
              title: 'List installation repositories',
              language: 'bash',
              code: 'curl -s localhost:8080/v1/git-apps/github/installations/$INSTALLATION_ID/repositories \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
              placeholders: ['$INSTALLATION_ID — from the installations list.'],
            },
          ],
        },
        {
          title: 'Send a test event',
          description: 'Push a commit, then check that the delivery arrived and was accepted.',
          verification: 'The delivery appears with a success status under the webhook source or App installation.',
        },
      ],
      details: [
        'Store the App private key and webhook secret as credential references, not as inline environment values. The legacy inline variables remain only for migration.',
        'Internal service URLs such as `git_bot_api_url` stay in system configuration and do not belong in the app-scoped Git App file.',
      ],
      related: ['git-triggers', 'git-webhook-sources', 'credentials'],
      sources: [
        { repositoryPath: 'doc/git-apps.md', purpose: 'Multi-installation management, GitOps schema, and git-bot routing.' },
        { repositoryPath: 'doc/git-webhook-sources.md', purpose: 'Non-GitHub provider configuration and security.' },
      ],
    },
    {
      id: 'trigger-pipeline-from-git',
      title: 'Trigger a pipeline from Git',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author', 'developer'],
      summary: 'Add a `.nopsai.yaml` trigger manifest so pushes and pull requests start pipelines automatically.',
      keywords: ['trigger', 'ci', 'push', 'pull request', 'webhook'],
      keyFacts: [
        'The trigger manifest lives in the repository, so trigger rules are reviewed like code.',
        'Path filters fail open: when the provider does not report changed files, the rule still matches so CI is not silently skipped.',
        '`branches` and `skip_branches` are evaluated together — includes first, then exclusions.',
      ],
      prerequisites: [
        { label: 'Repository', value: 'A connected repository from the previous tutorial' },
        { label: 'Pipeline', value: 'A pipeline file committed in the repository' },
        { label: 'Permission', value: 'Write access to the repository' },
        { label: 'Scope', value: 'A runtime scope the triggered run may use' },
      ],
      steps: [
        {
          title: 'Add the manifest',
          description: 'Commit `.nopsai.yaml` at the repository root.',
          commands: [
            {
              title: '.nopsai.yaml',
              language: 'yaml',
              code: [
                'team: platform/payments',
                'triggers:',
                '  - on: push',
                '    branches: [main]',
                '    include_paths:',
                '      - "services/payments/**"',
                '    pipelines:',
                '      - .nopsai/pipelines/ci.yaml',
                '    scope: platform/staging',
              ].join('\n'),
            },
          ],
        },
        {
          title: 'Push and watch',
          description: 'Push a change under the filtered path and confirm a run starts.',
          verification: 'A run appears under Pipeline Runs with the trigger recorded as its entry point.',
        },
        {
          title: 'Diagnose a non-match',
          description:
            'If no run starts, check the delivery on the webhook source or App installation, then compare the event name, branch, and changed paths against your rule.',
          commands: [
            {
              title: 'Check trigger analytics',
              language: 'bash',
              code: 'curl -s "localhost:8080/v1/monitoring/triggers/analytics" \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
            },
          ],
        },
      ],
      details: [
        'For non-GitHub providers the manifest must also name the `webhook_source` that receives deliveries.',
        'Setting `management` marks the manifest as NopsAI-managed so platform trigger overrides apply.',
      ],
      related: ['git-triggers', 'git-webhook-sources', 'monitoring'],
      sources: [
        { repositoryPath: 'doc/triggering.md', purpose: 'Local GitHub and generic Git webhook simulation.' },
        { repositoryPath: 'pkg/gittrigger/matcher.go', purpose: 'Exact branch, tag, and path matching behavior.' },
      ],
    },
    {
      id: 'add-approval-checkpoint',
      title: 'Add an approval checkpoint',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author', 'operator'],
      summary: 'Pause a run for a human decision without holding runner capacity.',
      keywords: ['approval', 'gate', 'checkpoint', 'human in the loop', 'four eyes'],
      keyFacts: [
        'An approval step is a durable checkpoint: the run pauses, the runner is released, and the state survives a restart.',
        'At least one approval team is required, and teams must be relative paths.',
        'Rejecting requires a comment; approving does not.',
        'An expired `timeout` marks the run `timed_out`.',
      ],
      prerequisites: [
        { label: 'Teams', value: 'At least one team path that can approve', verification: 'GET /v1/access/teams' },
        { label: 'Pipeline', value: 'An existing pipeline you can edit' },
        { label: 'Permission', value: 'Membership in an assigned approval team' },
        { label: 'Notifications', value: 'Optional, but useful so approvers hear about the request' },
      ],
      steps: [
        {
          title: 'Insert the approval step',
          description: 'Place it between the work that prepares a change and the work that applies it.',
          commands: [
            {
              title: 'Approval step',
              language: 'yaml',
              code: [
                '  - name: release-gate',
                '    depends_on: [build]',
                '    approval:',
                '      type: production-release',
                '      teams:',
                '        - platform/sre',
                '      allow_self_approval: false',
                '      timeout: 24h',
                '  - name: deploy',
                '    depends_on: [release-gate]',
                '    script: ./deploy.sh',
              ].join('\n'),
            },
          ],
          warning: 'Leaving `allow_self_approval` false keeps requester and approver separate.',
        },
        {
          title: 'Run and approve',
          description: 'Start the run, open its detail view, and approve the pending checkpoint.',
          commands: [
            {
              title: 'Approve from the API',
              language: 'bash',
              code: 'curl -sX POST localhost:8080/v1/runs/$RUN_ID/approvals/$APPROVAL_ID/approve \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN" \\\n  -d \'{"comment":"Change window confirmed"}\'',
              placeholders: ['$RUN_ID and $APPROVAL_ID — from GET /v1/runs/{runID}/approvals.'],
            },
          ],
          verification: 'The run resumes and the downstream step executes.',
        },
        {
          title: 'Try a rejection',
          description: 'Reject a second run to see the required comment and the resulting run state.',
          expectedOutput: 'The rejection is recorded with its comment and the run stops without executing downstream steps.',
        },
      ],
      details: [
        'Approval steps cannot declare task outputs and cannot be combined with `tasks`, `goal`, `script`, or `include`.',
        'Approval failures always fail closed. `ignore_failure` does not apply to them.',
      ],
      related: ['approvals', 'pipeline-runs', 'notifications'],
      sources: [
        { repositoryPath: 'services/nopsai/approval_schema.go', purpose: 'Approval persistence and decision handling.' },
      ],
    },
    {
      id: 'create-final-deliverable',
      title: 'Create a final deliverable',
      docType: 'tutorial',
      audiences: ['new-user', 'automation-author'],
      summary: 'Generate a Markdown, PDF, or dashboard artifact from run evidence after execution finishes.',
      keywords: ['output', 'report', 'pdf', 'deliverable', 'artifact'],
      keyFacts: [
        'Final outputs are generated after the run finishes and are stored separately from raw task logs.',
        'Supported types are `markdown`, `pdf`, `excel`, `json`, `html`, and `dashboard`.',
        '`when` controls whether an item is produced on `success`, `failure`, or `always` — omitted means `always`.',
        'PDF rendering requires a reachable Gotenberg service.',
      ],
      prerequisites: [
        { label: 'Model', value: 'A usable profile — final outputs are LLM-generated' },
        { label: 'LLM enabled', value: 'The pipeline must not set `llm_enabled: false`' },
        { label: 'Gotenberg', value: 'Required only for `type: pdf`', verification: 'FINAL_OUTPUT_PDF_RENDERER_URL is set' },
        { label: 'Permission', value: 'Read access to the run to download the result' },
      ],
      steps: [
        {
          title: 'Declare the output',
          description: 'Add an `output` block with one or more items.',
          commands: [
            {
              title: 'Output block',
              language: 'yaml',
              code: [
                'output:',
                '  model: report-writer',
                '  items:',
                '    - name: Executive summary',
                '      type: markdown',
                '      when: always',
                '      prompt: Summarize the run, its approvals, and any ignored failures.',
                '    - name: Failure report',
                '      type: pdf',
                '      when: failure',
                '      prompt: Explain what failed, the likely cause, and the next diagnostic step.',
              ].join('\n'),
            },
          ],
        },
        {
          title: 'Run and open the output',
          description: 'After the run completes, open the run detail Outputs tab.',
          verification: 'Ready output rows are clickable preview toggles.',
        },
        {
          title: 'Download it',
          description: 'Fetch the generated content through the API when you need it outside the UI.',
          commands: [
            {
              title: 'Download',
              language: 'bash',
              code: 'curl -sOJ localhost:8080/v1/runs/$RUN_ID/outputs/$OUTPUT_ID/download \\\n  -H "Authorization: Bearer $NOPSAI_TOKEN"',
            },
          ],
        },
      ],
      details: [
        'Generated content stays on authorized detail and read paths. Run list surfaces expose only lightweight aggregate `final_output_status` metadata.',
        'Provider and network errors during generation are not retried automatically — use the retry action on the output.',
      ],
      related: ['final-deliverables', 'dashboards', 'pipeline-runs'],
      sources: [
        { repositoryPath: 'doc/final-output-rendering.md', purpose: 'Generation contracts, retry and audit behavior, current renderers.' },
      ],
    },
  ],
};
