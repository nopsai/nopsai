export type WikiArticleLevel = 'Start' | 'Operate' | 'Reference' | 'Admin' | 'Troubleshoot' | 'Security';

export type WikiConfigRow = {
  key: string;
  area: string;
  description: string;
  example: string;
};

export type WikiExample = {
  title: string;
  language: string;
  code: string;
};

export type WikiArticle = {
  id: string;
  title: string;
  level: WikiArticleLevel;
  audience: string;
  summary: string;
  keyFacts: string[];
  details: string[];
  configRows: WikiConfigRow[];
  examples: WikiExample[];
  relatedDocs: string[];
  runbooks: string[];
  caveats: string[];
};

export type WikiSection = {
  id: string;
  title: string;
  owner: string;
  description: string;
  articles: WikiArticle[];
};

export type WikiSummary = {
  sections: number;
  articles: number;
  configKeys: number;
  runbooks: number;
  caveats: number;
};

export const wikiMetadata = {
  title: 'NopsAI Product Wiki',
  status: 'Repository-grounded current implementation',
  apiVersion: 'Product API v1',
  audience: 'Users, platform engineers, administrators, security teams, automation authors, and operators',
  sourceOrder: [
    'Runtime code, schemas, and route behavior',
    'Docker Compose and Helm deployment files',
    'Focused markdown under doc/',
    'Root README examples',
    'UI help text only when it agrees with implementation',
  ],
};

export const unsupportedClaims = [
  'Terraform modules or cloud-provider-specific EKS, AKS, or GKE automation',
  'Built-in Redis dependency',
  'S3 or generic object-storage artifact and backup backend',
  'Helm-managed PostgreSQL',
  'Documented HPA/autoscaling or Kubernetes NetworkPolicy set',
  'Complete air-gap installation workflow',
  'Product-managed restore workflow',
  'Validated production sizing, throughput, or multi-region HA guarantees',
];

export const supportedDeploymentModels = [
  {
    target: 'Docker Compose',
    purpose: 'Development, demos, evaluation, and small single-host installs',
    execution: 'Docker runner',
    storage: 'Included PostgreSQL 15 with a named database volume',
  },
  {
    target: 'Release bundle',
    purpose: 'Version-pinned single-host deployment',
    execution: 'Docker runner',
    storage: 'Deployment-managed PostgreSQL and deployment-owned backups',
  },
  {
    target: 'Helm/Kubernetes',
    purpose: 'Cluster-based production deployment',
    execution: 'Kubernetes runner',
    storage: 'External PostgreSQL and PVC-backed run workspaces',
  },
  {
    target: 'Hybrid runners',
    purpose: 'Central control plane with runners in separate Docker hosts or Kubernetes namespaces',
    execution: 'Docker and Kubernetes runners',
    storage: 'Control-plane PostgreSQL plus runner-local workspace storage',
  },
];

export const wikiSections: WikiSection[] = [
  {
    id: 'architecture',
    title: 'Product and Architecture',
    owner: 'Platform engineering',
    description: 'What the product is, how the control plane and execution plane fit together, and how a run moves through the system.',
    articles: [
      {
        id: 'what-nopsai-is',
        title: 'What NopsAI Is',
        level: 'Start',
        audience: 'New users, buyers, architects, and automation owners',
        summary:
          'NopsAI is a self-hosted automation control plane for executing YAML-defined workflows through Docker or Kubernetes runners.',
        keyFacts: [
          'Pipelines can combine deterministic shell scripts, AI goals, reusable steps, child pipelines, and human approval gates.',
          'Runtime variables, encrypted secrets, governed knowledge, LLM profiles, Agent Profiles, and MCP profiles are resolved before execution.',
          'Runs may be started manually, by schedules, by GitHub App events, by Git webhook sources, by external triggers, or by parent pipelines.',
          'Generated final outputs can be Markdown, JSON, HTML, PDF, or Excel and are stored separately from raw task logs.',
        ],
        details: [
          'The product is built around GitOps-friendly configuration but keeps durable execution, audit, credential, monitoring, and setup state in PostgreSQL.',
          'LLM-backed tasks run inside the per-run agent. There is no separate always-on LLM service in the current repository shape.',
          'The UI, CLI, REST API, hosted MCP surface, and runners all go through the same authentication, authorization, and audit boundaries.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/architecture-overview.md', 'doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Map an existing manual operation to a NopsAI pipeline', 'Review GitOps ownership before onboarding a team'],
        caveats: [
          'The current repository does not define cloud-specific infrastructure modules. Cloud installs should treat NopsAI as a portable Kubernetes workload.',
        ],
      },
      {
        id: 'control-execution-plane',
        title: 'Control Plane and Execution Plane',
        level: 'Reference',
        audience: 'Platform engineers and operators',
        summary:
          'The API, AAA service, dispatcher, git-bot, UI, PostgreSQL, Gotenberg, and socket proxy form the control plane; Docker and Kubernetes runners with per-run agents form the execution plane.',
        keyFacts: [
          '`nopsai-api` owns REST APIs, validation, orchestration, setup, monitoring, credentials, notifications, GitOps, auth integration, and run records.',
          '`aaa` owns authorization decisions, policy checks, ACL expansion, filtering, and decision audit records.',
          '`dispatcher` owns runner registration, queueing, routing, capacity selection, and job assignment over gRPC.',
          '`git-bot` owns GitHub App webhooks, repository access, check-runs, and GitHub-specific integration.',
          'Runners start one agent for each assigned run; the agent then starts step containers or pods and reports status back.',
        ],
        details: [
          'The API submits jobs to the dispatcher. Runners maintain long-lived outbound connections to the dispatcher, which keeps runner registration and capacity visible to the control plane.',
          'Docker runners create containers and Docker volumes. Kubernetes runners create an agent pod, PVC-backed workspace, and step pods in their namespace.',
          'Gotenberg is used for PDF rendering. The Docker socket proxy is restricted to allow-listed System Logs reads and is not used for runner execution.',
        ],
        configRows: [
          {
            key: 'DISPATCHER_GRPC_ADDRESS',
            area: 'Dispatcher',
            description: 'Address used by API, runners, and agents to reach dispatcher gRPC.',
            example: 'dispatcher:9090',
          },
          {
            key: 'GIT_BOT_API_URL',
            area: 'GitHub integration',
            description: 'Internal URL used by the API to reach git-bot.',
            example: 'http://nopsai-git-bot:8081',
          },
          {
            key: 'AAA_API_URL',
            area: 'Authorization',
            description: 'Internal URL used by the API for AAA checks and introspection.',
            example: 'http://aaa:8082',
          },
        ],
        examples: [],
        relatedDocs: ['doc/service-reference.md', 'doc/runtime-flows.md', 'doc/system-logs.md'],
        runbooks: ['Verify service-to-service URLs after a deployment change', 'Check dispatcher and runner health before onboarding workloads'],
        caveats: ['Service bind addresses and client dial addresses are separate; services usually listen on 0.0.0.0 while peers use DNS names.'],
      },
      {
        id: 'normal-run-lifecycle',
        title: 'Normal Run Lifecycle',
        level: 'Reference',
        audience: 'Automation authors, SREs, and support teams',
        summary:
          'A run is authenticated, authorized, validated, dispatched to an eligible runner, executed by an agent, finalized, monitored, audited, and optionally reported back to Git.',
        keyFacts: [
          'Manual, scheduled, Git, external-trigger, and child-pipeline entry points all converge into durable run records.',
          'Required scopes, variables, secrets, profiles, reusable steps, child pipelines, and knowledge documents are resolved before dispatch.',
          'Approvals pause and checkpoint the run without retaining runner capacity.',
          'Final output generation happens after execution status is known and records generation, contract, and render metrics.',
        ],
        details: [
          'The dispatcher selects a runner by availability, allowed scope, routing policy, affinity, and load.',
          'The agent executes dependency-ready tasks, so independent work can run concurrently where the schema allows it.',
          'Logs and task status are streamed back through the dispatcher and stored by the API; Git checks and notifications follow the run lifecycle.',
        ],
        configRows: [
          {
            key: 'SERVICE_JWT_SIGNING_KEY',
            area: 'Internal auth',
            description: 'Signs service tokens for dispatcher, runner, agent, API, and git-bot callbacks.',
            example: 'openssl rand -base64 48',
          },
        ],
        examples: [],
        relatedDocs: ['doc/runtime-flows.md', 'doc/final-output-rendering.md'],
        runbooks: ['Trace a stuck run from API record to dispatcher queue to runner logs'],
        caveats: ['A queued run normally means no eligible runner is currently available for the requested scope or route.'],
      },
    ],
  },
  {
    id: 'installation',
    title: 'Installation',
    owner: 'Platform engineering',
    description: 'First install, local Compose, Helm, runtime secrets, and runner installation boundaries.',
    articles: [
      {
        id: 'first-install-wizard',
        title: 'First-Install Wizard',
        level: 'Admin',
        audience: 'Administrators bootstrapping an empty workspace',
        summary:
          'The System > Setup wizard moves an empty database into a functioning workspace with runtime checks, starter teams, starter users, GitOps files, and optional AI/MCP seed data.',
        keyFacts: [
          'Preflight checks cover database reachability, master key, user JWT key, admin state, internal service secrets, service addresses, LLM/MCP readiness, starter pipeline, and runner health.',
          'The wizard can generate missing internal service secrets and output environment, secret-manager, or container snippets.',
          'Starter GitOps content includes teams, a first-run pipeline, reusable step, triggers, scopes, access bootstrap, knowledge docs, LLM profile, MCP examples, and config repository structure.',
          'Production-gate mode does not silently seed an unsafe missing administrator.',
        ],
        details: [
          'Optional integrations may be skipped and configured later, but required readiness gates must be resolved before normal operation.',
          'The default local admin state is reported until changed. For production, rotate secrets and replace the bootstrap administrator password before accepting real workload access.',
        ],
        configRows: [
          {
            key: 'DATABASE_URL',
            area: 'Bootstrap',
            description: 'PostgreSQL connection used by API and AAA.',
            example: 'postgres://nopsai_user:***@nopsai-db:5432/nopsai_db',
          },
          {
            key: 'NOPSAI_MASTER_KEY',
            area: 'Bootstrap',
            description: 'Root encryption material for pipeline secrets and credential wrapping.',
            example: 'openssl rand -base64 32',
          },
          {
            key: 'JWT_SIGNING_KEY',
            area: 'Bootstrap',
            description: 'Signs user/API access tokens for browser and API sessions.',
            example: 'openssl rand -base64 48',
          },
          {
            key: 'AAA_SHARED_INTERNAL_TOKEN',
            area: 'Bootstrap',
            description: 'Shared internal token used by NopsAI when calling AAA protected endpoints.',
            example: 'openssl rand -base64 32',
          },
        ],
        examples: [],
        relatedDocs: ['doc/first-install-wizard.md', 'doc/enterprise-gates.md'],
        runbooks: ['Complete setup preflight', 'Replace default administrator password', 'Run setup/first-run'],
        caveats: ['Changing generated service secrets usually requires restarting affected services.'],
      },
      {
        id: 'docker-compose',
        title: 'Docker Compose',
        level: 'Start',
        audience: 'Developers, evaluators, and single-host operators',
        summary:
          'The Compose stack starts PostgreSQL 15, API, AAA, dispatcher, git-bot, UI, Gotenberg, Docker socket proxy, Docker runner, and build-only runtime images.',
        keyFacts: [
          'Default local entry points are UI on http://localhost/, API on http://localhost:8080, git-bot on http://localhost:8081, dispatcher on localhost:9090, and PostgreSQL on localhost:5432.',
          'The Docker runner mounts /var/run/docker.sock so it can create agent and step containers.',
          'The API reads System Logs through the restricted Docker socket proxy at tcp://docker-socket-proxy:2375.',
          'Compose contains local-development fallback secrets and must be overridden for non-local use.',
        ],
        details: [
          'The Compose network is `nopsai-net`. Internal URLs use service DNS names, while published host ports are for browsers, API clients, and local tooling.',
          'The checked-in Compose definition persists PostgreSQL with a named volume. Product-generated backups should get a separate durable mount at /data/backups for real deployments.',
        ],
        configRows: [
          {
            key: 'NOPSAI_API_URL',
            area: 'Runner and agent',
            description: 'Internal API URL supplied to runtime containers.',
            example: 'http://nopsai:8080',
          },
          {
            key: 'FINAL_OUTPUT_PDF_RENDERER_URL',
            area: 'Final outputs',
            description: 'Internal Gotenberg URL used by server-owned PDF rendering.',
            example: 'http://gotenberg:3000',
          },
          {
            key: 'SYSTEM_LOGS_DOCKER_HOST',
            area: 'System Logs',
            description: 'Restricted Docker socket proxy endpoint for allow-listed log reads.',
            example: 'tcp://docker-socket-proxy:2375',
          },
          {
            key: 'DOCKER_NETWORK_NAME',
            area: 'Docker runner',
            description: 'Network used for agent and step containers in local Docker execution.',
            example: 'nopsai-net',
          },
        ],
        examples: [
          {
            title: 'Local stack',
            language: 'bash',
            code: 'docker compose up -d --build\ndocker compose ps\ndocker compose logs -f nopsai dispatcher docker-runner',
          },
        ],
        relatedDocs: ['docker-compose.yaml', 'deploy/docker-compose.release.yaml', 'doc/enterprise-gates.md'],
        runbooks: ['Start local stack', 'Check runner registration', 'Persist /data/backups for non-local Compose'],
        caveats: ['The Docker runner socket mount is a privileged operational boundary and should be isolated to trusted runner hosts.'],
      },
      {
        id: 'helm-kubernetes',
        title: 'Kubernetes and Helm',
        level: 'Admin',
        audience: 'Cluster operators and platform teams',
        summary:
          'The Helm chart deploys API, AAA, dispatcher, git-bot, UI, Gotenberg, and a Kubernetes runner. PostgreSQL and bootstrap secrets are intentionally external.',
        keyFacts: [
          'Create the Secret referenced by `secrets.existingSecret` before installing the chart.',
          'Default secret keys are database-url, master-key, jwt-signing-key, service-jwt-signing-key, and aaa-shared-internal-token.',
          'The chart defaults to Kubernetes System Logs and can create read-only pods and pods/log RBAC for the API service account.',
          'The Kubernetes runner starts one agent pod per run and step pods that share a PVC-backed workspace.',
        ],
        details: [
          'Use External Secrets, Sealed Secrets, SOPS, or a platform secret manager instead of committing secret values to Helm values.',
          'The chart exposes replica counts and resource blocks but leaves sizing empty by default. Establish capacity through load testing and metrics.',
        ],
        configRows: [
          {
            key: 'secrets.existingSecret',
            area: 'Helm',
            description: 'Name of the Kubernetes Secret holding bootstrap values.',
            example: 'nopsai-secrets',
          },
          {
            key: 'k8sRunner.workspace.volumeMode',
            area: 'Runner',
            description: 'Workspace mode for agent and step pods.',
            example: 'pvc',
          },
          {
            key: 'k8sRunner.affinityEnabled',
            area: 'Runner',
            description: 'Pins step pods to the agent node for ReadWriteOnce storage compatibility.',
            example: 'true',
          },
          {
            key: 'systemLogs.provider',
            area: 'System Logs',
            description: 'Log provider used by the API in the chart.',
            example: 'kubernetes',
          },
        ],
        examples: [
          {
            title: 'Install from packaged chart',
            language: 'bash',
            code:
              'kubectl create namespace nopsai\nkubectl -n nopsai create secret generic nopsai-secrets \\\n  --from-literal=database-url="postgres://..." \\\n  --from-literal=master-key="..." \\\n  --from-literal=jwt-signing-key="..." \\\n  --from-literal=service-jwt-signing-key="..." \\\n  --from-literal=aaa-shared-internal-token="..."\nhelm upgrade --install nopsai ./nopsai-<version>.tgz \\\n  --namespace nopsai \\\n  --create-namespace \\\n  --set secrets.existingSecret=nopsai-secrets',
          },
        ],
        relatedDocs: ['deploy/helm/nopsai/README.md', 'deploy/helm/nopsai/values.yaml', 'doc/kubernetes-runner.md'],
        runbooks: ['Create bootstrap Secret', 'Render Helm plan', 'Verify Kubernetes runner workspace PVCs'],
        caveats: ['The repository does not provide Terraform modules, managed database provisioning, ingress TLS automation, or cloud IAM wiring.'],
      },
      {
        id: 'additional-runners',
        title: 'Additional Runners',
        level: 'Operate',
        audience: 'Operators scaling or isolating execution capacity',
        summary:
          'Runner installs are generated from System > Dispatcher > Runner Installs and use one-time download tokens to produce Docker or Kubernetes runner configuration.',
        keyFacts: [
          'Runner tokens expire after ten minutes and are consumed after the first successful download.',
          'Each runner should have a unique ID, capacity, allowed scope list, and network path to the dispatcher.',
          'Separate runners are the natural boundary for production versus non-production, region, team, security zone, or workload class.',
        ],
        details: [
          'The dispatcher checks runner availability, scope compatibility, routing, affinity, and load before assignment.',
          'For Kubernetes, runner manifests include namespace, ServiceAccount, namespace-scoped Role, RoleBinding, dispatcher auth Secret, runtime ConfigMap, and Deployment.',
        ],
        configRows: [
          {
            key: 'RUNNER_ID',
            area: 'Runner',
            description: 'Stable unique runner identity used by dispatcher routing and monitoring.',
            example: 'prod-eu-1',
          },
          {
            key: 'RUNNER_SCOPES',
            area: 'Runner',
            description: 'Comma-separated scopes the runner may execute.',
            example: 'dev,staging,prod',
          },
          {
            key: 'RUNNER_CAPACITY',
            area: 'Runner',
            description: 'Maximum concurrent jobs for the runner.',
            example: '10',
          },
        ],
        examples: [],
        relatedDocs: ['doc/kubernetes-runner.md', 'doc/runtime-flows.md'],
        runbooks: ['Generate one-time runner installer', 'Restrict runner scopes', 'Retire stale runner registration'],
        caveats: ['Increasing every replica count does not by itself guarantee high availability or capacity.'],
      },
    ],
  },
  {
    id: 'platform-admin',
    title: 'Platform Administration',
    owner: 'Administrators and security teams',
    description: 'Networking, storage, GitOps authority, credentials, authentication, AAA, teams, scopes, and runtime ownership.',
    articles: [
      {
        id: 'networking',
        title: 'Networking and Exposure',
        level: 'Admin',
        audience: 'Network, platform, and security operators',
        summary:
          'Expose the UI, API, and required webhook endpoints through TLS; keep AAA, PostgreSQL, Gotenberg, Docker socket proxy, and usually dispatcher private.',
        keyFacts: [
          'Browser and CLI traffic reaches the API on port 8080 by default.',
          'GitHub App webhooks reach git-bot on port 8081 by default.',
          'Generic Git webhook sources enter through the API and should use external HTTPS.',
          'Dispatcher traffic is long-lived gRPC from API, runners, and agents on port 9090 by default.',
        ],
        details: [
          'Required outbound access depends on selected Git providers, container registries, LLM providers, MCP endpoints, SMTP, OIDC, and external PostgreSQL.',
          'Dispatcher transport supports mtls, tls, and disabled modes. Production gates reject disabled dispatcher transport security.',
        ],
        configRows: [
          {
            key: 'DISPATCHER_TLS_MODE',
            area: 'Dispatcher',
            description: 'Transport security mode for dispatcher gRPC.',
            example: 'mtls',
          },
          {
            key: 'DISPATCHER_TLS_SECRET',
            area: 'Dispatcher',
            description: 'Trust seed for derived in-memory dispatcher certificates.',
            example: 'managed secret value',
          },
        ],
        examples: [],
        relatedDocs: ['doc/jwt-authentication.md', 'doc/enterprise-gates.md', 'doc/system-logs.md'],
        runbooks: ['Review public ingress list', 'Rotate dispatcher trust seed', 'Validate outbound provider access'],
        caveats: ['A fully offline or air-gapped installation workflow is not currently defined.'],
      },
      {
        id: 'storage-persistence',
        title: 'Storage and Persistence',
        level: 'Reference',
        audience: 'Database administrators, operators, and auditors',
        summary:
          'PostgreSQL is the primary durable store for product, identity, configuration, execution, monitoring, audit, credential metadata, and setup state.',
        keyFacts: [
          'Compose mounts PostgreSQL at /var/lib/postgresql/data in the `db` named volume.',
          'Docker workspaces are shared named volumes, normally `vol-<run-id>`.',
          'Kubernetes workspaces are PVC-backed so agent and step pods can share files.',
          'Approval checkpoints store compressed workspace archives in PostgreSQL so a run can resume on a new agent.',
        ],
        details: [
          'Final-output source and generation status are stored in PostgreSQL. PDF/HTML use validated DocumentSpec, and Excel uses validated SpreadsheetSpec.',
          'Product-generated backups are gzip-compressed JSON Lines files stored under /data/backups by default, with checksum and file mode metadata.',
        ],
        configRows: [
          {
            key: 'FINAL_OUTPUT_PDF_TIMEOUT_SECONDS',
            area: 'Final outputs',
            description: 'Timeout for PDF rendering through Gotenberg.',
            example: '45',
          },
        ],
        examples: [
          {
            title: 'Persist product backups in Compose',
            language: 'yaml',
            code: 'services:\n  nopsai:\n    volumes:\n      - nopsai-backups:/data/backups\n\nvolumes:\n  nopsai-backups:',
          },
        ],
        relatedDocs: ['doc/final-output-rendering.md', 'doc/kubernetes-runner.md'],
        runbooks: ['Verify backup file checksum', 'Test database recovery outside NopsAI', 'Check workspace PVC events'],
        caveats: [
          'The default Compose topology does not mount dedicated durable storage at /data/backups.',
          'The repository does not implement an S3 or generic object-storage backup backend.',
        ],
      },
      {
        id: 'gitops-authority',
        title: 'Configuration Authority and GitOps',
        level: 'Admin',
        audience: 'Platform administrators and repository owners',
        summary:
          'Bootstrap environment, durable runtime settings, and GitOps repositories form separate configuration layers with different ownership and recovery behavior.',
        keyFacts: [
          'Bootstrap values include database URL, master key, JWT signing keys, AAA token, dispatcher trust, service addresses, renderer, and System Logs topology.',
          'GitOps can own pipelines, steps, schedules, triggers, external triggers, Git webhook sources, scopes, knowledge, access, config repositories, and setting/system files.',
          'Global configuration sync runs before delegated team repositories.',
          'UI edits to GitOps-managed resources create database overrides until the change is pushed back to Git.',
        ],
        details: [
          'Canonical system files include credentials.yaml, github.yaml, runner.yaml, auth.yaml, mail.yaml, llm_profile.yaml, mcp.yaml, and agent-profiles.yaml.',
          'Config sync can import, update, prune Git-managed resources, detect drift, generate commit-ready changes, push to a review branch, and adopt database-created resources when matching files exist.',
        ],
        configRows: [
          {
            key: 'setting/system/runner.yaml',
            area: 'GitOps',
            description: 'Runtime topology, runner defaults, dispatcher routing, and assistant settings.',
            example: 'setting/system/runner.yaml',
          },
          {
            key: 'setting/system/credentials.yaml',
            area: 'GitOps',
            description: 'Encrypted credential envelopes for integration credentials.',
            example: 'setting/system/credentials.yaml',
          },
        ],
        examples: [],
        relatedDocs: ['doc/mcp-pipeline-integration.md', 'doc/credential-management.md', 'doc/git-webhook-sources.md'],
        runbooks: ['Resolve GitOps drift', 'Promote a UI override back to Git', 'Review delegated team repository ownership'],
        caveats: ['Database-only edits to GitOps-managed resources may be overwritten or recreated by the next sync.'],
      },
      {
        id: 'credentials-secrets',
        title: 'Credentials, Secrets, and Variables',
        level: 'Security',
        audience: 'Security teams, administrators, and automation authors',
        summary:
          'Bootstrap secrets remain in deployment Secrets; integration credentials live in the encrypted credential registry; pipeline secrets and variables resolve by scope and repository.',
        keyFacts: [
          'Credential references use stable URIs such as credential://system/llm/openai-primary or credential://team/platform/llm/openai.',
          'Credential versions use envelope encryption with AES-256-GCM and versioned key wrapping.',
          'Human-facing reads return credential metadata only; normal credential-value read is not available.',
          'Pipeline secrets are encrypted at rest, selected by scope/repository, delivered only in protected run payloads, and masked from logs and history.',
        ],
        details: [
          'Resolution prefers the selected run scope; unscoped defaults are used only for unscoped runs. Repository-specific values override global values within the same scope.',
          'Variables can come from scope configuration, repository-specific values, manual run overrides, schedule overrides, external trigger mappings, step-level overrides, and task-level overrides.',
        ],
        configRows: [
          {
            key: 'credential://system/llm/openai-primary',
            area: 'Credential reference',
            description: 'Example stable reference for a system LLM credential.',
            example: 'credential://system/llm/openai-primary',
          },
          {
            key: 'credential://system/mcp/github-readonly',
            area: 'Credential reference',
            description: 'Example stable reference for an MCP bearer token.',
            example: 'credential://system/mcp/github-readonly',
          },
        ],
        examples: [],
        relatedDocs: ['doc/credential-management.md', 'doc/access-control.md'],
        runbooks: ['Rotate an integration credential', 'Review credential consumer access logs', 'Add scoped pipeline secret'],
        caveats: ['Bootstrap secrets cannot be stored inside the same database they unlock.'],
      },
      {
        id: 'auth-aaa-teams-scopes',
        title: 'Authentication, AAA, Teams, and Scopes',
        level: 'Security',
        audience: 'Security administrators and enterprise platform owners',
        summary:
          'NopsAI authenticates callers, maps routes to actions/resources, asks AAA for decisions, and keeps runtime authorization tied to the original caller.',
        keyFacts: [
          'Supported auth paths include local access JWTs, refresh tokens, personal tokens, service-account tokens, internal service JWTs, dispatcher service JWTs, and OIDC login with PKCE.',
          'AAA handles Check, BatchCheck, Filter, ACL expansion, inheritance, and decision auditing, with a short-outage in-process fallback in the API.',
          'Product roles are viewer, developer, owner, and platform admin; admin is granted only on the platform resource.',
          'Scopes are runtime context such as dev, test, production, or platform/prod. They are not run-navigation parents.',
        ],
        details: [
          'Teams form hierarchical product boundaries for ownership, access, run navigation, config repository delegation, notifications, team AI overlays, and repository-to-application matching.',
          'Cross-team execution requires access to every concrete resource used by the run, not only the pipeline.',
        ],
        configRows: [
          {
            key: 'setting/system/auth.yaml',
            area: 'GitOps',
            description: 'Local login and OIDC provider settings.',
            example: 'setting/system/auth.yaml',
          },
          {
            key: 'auth_identity_providers.client_credential_ref',
            area: 'OIDC',
            description: 'Credential reference for OIDC client secret.',
            example: 'credential://system/oidc/corporate/client-secret',
          },
        ],
        examples: [
          {
            title: 'OIDC provider sketch',
            language: 'yaml',
            code:
              'oidc:\n  enabled: true\n  auto_create_users: true\n  providers:\n    corporate:\n      type: oidc\n      display_name: Company SSO\n      issuer: https://idp.company.com\n      client_id: nopsai\n      client_credential_ref: credential://system/oidc/corporate/client-secret\n      scopes: [openid, email, profile]',
          },
        ],
        relatedDocs: ['doc/access-control.md', 'doc/jwt-authentication.md', 'doc/local-keycloak-sso.md', 'doc/team-resource-ownership-design.md'],
        runbooks: ['Create least-privilege service account', 'Audit cross-team resource grants', 'Configure corporate OIDC provider'],
        caveats: ['Public pipeline visibility does not grant dependent scopes, secrets, variables, runners, credentials, or knowledge context.'],
      },
    ],
  },
  {
    id: 'automation',
    title: 'Automation Authoring',
    owner: 'Automation teams',
    description: 'Pipeline YAML, steps, dependencies, approvals, schedules, Git triggers, Git webhook sources, and external triggers.',
    articles: [
      {
        id: 'pipeline-schema',
        title: 'Pipeline YAML Schema',
        level: 'Reference',
        audience: 'Automation authors and platform reviewers',
        summary:
          'Pipelines define container images, variables, steps, timeouts, AI controls, profiles, runtime pools, knowledge context, and final outputs.',
        keyFacts: [
          'Top-level fields include name, version, description, container_image, working_directory, variables, steps, timeout, llm_enabled, agent_profile, llm_profile, mcp_profiles, runtime_pool, affinity_enabled, knowledge_context, output, and LLM sharing controls.',
          'Every step must contain exactly one mode: include, tasks, goal, script, or approval.',
          'Independent ready tasks may execute concurrently; depends_on defines graph edges.',
          'Script-only pipelines can set llm_enabled: false to avoid requiring an LLM registry.',
        ],
        details: [
          'Within one step, NopsAI reuses a step container or pod. Across steps, separate containers or pods may be used. All steps share the run workspace.',
          'Script tasks with effective guardrail or policy knowledge are submitted for LLM validation before execution and fail closed on conflicts or unavailable validation.',
        ],
        configRows: [
          {
            key: 'llm_enabled',
            area: 'Pipeline YAML',
            description: 'Disables LLM-backed behavior for script-only pipelines.',
            example: 'false',
          },
          {
            key: 'runtime_pool',
            area: 'Pipeline YAML',
            description: 'Selects a Kubernetes runtime pool for scheduling.',
            example: 'high-memory',
          },
        ],
        examples: [
          {
            title: 'Release readiness pipeline',
            language: 'yaml',
            code:
              'name: release-readiness\nversion: "1.0"\ncontainer_image: alpine:3.20\nworking_directory: /workspace\ntimeout: 2h\nvariables: [ENVIRONMENT, RELEASE_SHA]\nagent_profile: devops-engineer\nllm_profile: standard\nmcp_profiles: [github-pr-review]\nknowledge_context:\n  - kind: guardrail\n    ref: security/production-deployment\n    required: true\nsteps:\n  - name: test\n    image: golang:1.24\n    script: go test ./...\n  - name: reliability-review\n    depends_on: [test]\n    goal: Review release reliability, rollback readiness, and operational evidence.\n  - name: production-approval\n    depends_on: [reliability-review]\n    approval:\n      type: production-deploy\n      teams: [platform/production]\n      allow_self_approval: false',
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Validate a new pipeline', 'Review dependency graph before production use', 'Convert manual shell script to script-only pipeline'],
        caveats: ['Goal tasks, conditions, and explicit MCP profiles are invalid when LLM behavior is disabled.'],
      },
      {
        id: 'approvals-reuse-children',
        title: 'Approvals, Reusable Steps, and Child Pipelines',
        level: 'Operate',
        audience: 'Release managers and workflow authors',
        summary:
          'Human approvals checkpoint work and release runner capacity; reusable steps and child pipelines let shared automation be composed with explicit authorization.',
        keyFacts: [
          'Approval steps store execution history, completed task keys, variables, pipeline definition, and a compressed workspace archive.',
          'Approval moves a run to waiting_approval and exits the agent so no runner capacity stays occupied.',
          'Approval resumes with a fresh agent that restores the checkpoint.',
          'Include steps can reference step:<identifier> or pipeline:<identifier>.',
        ],
        details: [
          'Cross-team reusable-step and child-pipeline includes require explicit step.use or pipeline.use authorization.',
          'Parent run aggregation remains active while direct child runs execute and surfaces child failures.',
        ],
        configRows: [
          {
            key: 'approval.allow_self_approval',
            area: 'Pipeline YAML',
            description: 'Controls whether the requester may approve their own gate.',
            example: 'false',
          },
        ],
        examples: [
          {
            title: 'Approval step',
            language: 'yaml',
            code: 'steps:\n  - name: deploy-gate\n    approval:\n      type: production-deploy\n      teams:\n        - platform/prod\n      allow_self_approval: false',
          },
        ],
        relatedDocs: ['doc/feature-reference.md', 'doc/access-control.md'],
        runbooks: ['Approve or reject a pending gate', 'Resume an approval checkpoint', 'Audit cross-team include permissions'],
        caveats: ['Rejecting an approval marks the approval task failed and the run rejected.'],
      },
      {
        id: 'git-triggers',
        title: 'Git Triggers and Git Webhook Sources',
        level: 'Reference',
        audience: 'Repository owners and integration authors',
        summary:
          'GitHub events enter through git-bot; GitLab, Bitbucket, Gitea, and generic Git events enter through managed Git Webhook Sources in the API.',
        keyFacts: [
          'GitHub webhook flow verifies HMAC, forwards the payload to the API, matches triggers, authorizes repository access, creates a run, and updates GitHub checks.',
          'Git Webhook Sources support provider, enabled state, team path, visibility, hmac/static_token/none auth modes, credential_ref, repository allowlist, and rate limits.',
          'Trigger manifests support events, branches, skip_branches, tags, skip_repos, include_paths, exclude_paths, pipelines, and scope.',
          'When changed-file metadata is unavailable, path matching fails open so automation is not silently skipped.',
        ],
        details: [
          'Non-GitHub providers do not currently fetch pipeline or trigger files directly from the source repository. Their triggers and pipelines must already exist in NopsAI, normally through configuration sync.',
          'Delivery history records accepted events, authentication failures, idempotency behavior, and no_match outcomes.',
        ],
        configRows: [
          {
            key: 'git-webhook-sources/',
            area: 'GitOps',
            description: 'Declarative Git webhook source definitions.',
            example: 'git-webhook-sources/gitlab-platform.yaml',
          },
          {
            key: 'triggers/',
            area: 'GitOps',
            description: 'Repository trigger manifests.',
            example: 'triggers/platform/api.yaml',
          },
        ],
        examples: [
          {
            title: 'Repository trigger',
            language: 'yaml',
            code:
              'provider: gitlab\nteam: platform\nwebhook_source: gitlab-platform\ntriggers:\n  - on: push\n    branches: [main, release/*]\n    include_paths:\n      - services/api/**\n    exclude_paths:\n      - docs/**\n    pipelines:\n      - platform/api-ci\n    scope: platform/prod',
          },
        ],
        relatedDocs: ['doc/triggering.md', 'doc/git-webhook-sources.md'],
        runbooks: ['Debug GitHub webhook no-run outcome', 'Rotate webhook credential', 'Review repository allowlist'],
        caveats: ['Use auth_mode: none only on isolated private ingress.'],
      },
      {
        id: 'schedules-external-triggers',
        title: 'Schedules and External Triggers',
        level: 'Operate',
        audience: 'Automation authors and integration owners',
        summary:
          'Schedules run stored pipelines on time; External Triggers let a user or service account invoke one configured pipeline with payload validation and variable mapping.',
        keyFacts: [
          'Schedules support cron, one-time timestamps, timezone, enabled state, scope, variable overrides, run team, immediate run, latest-run linkage, and status.',
          'Schedules run under a schedule-owned service account with explicit grants for pipeline and referenced resources.',
          'External triggers support allowed callers, payload schema, variable mapping, rate limit, idempotency key, and invocation history.',
          'External trigger callers must appear in allowed_callers and pass external_trigger.invoke.',
        ],
        details: [
          'Variable mappings can come from payload, variables, or event_type.',
          'Run team controls where scheduled or externally triggered runs appear and which notification policy lineage receives events.',
        ],
        configRows: [
          {
            key: 'schedules/',
            area: 'GitOps',
            description: 'Declarative recurring and one-time pipeline schedules.',
            example: 'schedules/platform/nightly.yaml',
          },
          {
            key: 'external-triggers/',
            area: 'GitOps',
            description: 'Declarative integration trigger definitions.',
            example: 'external-triggers/deployments/service-now.yaml',
          },
        ],
        examples: [],
        relatedDocs: ['doc/feature-reference.md', 'doc/runtime-flows.md'],
        runbooks: ['Run a schedule immediately', 'Investigate missed schedule', 'Replay an external trigger safely'],
        caveats: ['GitOps-managed schedules and triggers can be replaced by sync unless changes are pushed back to Git.'],
      },
    ],
  },
  {
    id: 'ai-mcp-knowledge',
    title: 'AI, MCP, and Knowledge',
    owner: 'AI platform team',
    description: 'Agent Profiles, LLM Profiles, external MCP, hosted MCP, Knowledge Context, and final-output generation.',
    articles: [
      {
        id: 'ai-control-layers',
        title: 'AI Control Layers',
        level: 'Reference',
        audience: 'AI platform teams and security reviewers',
        summary:
          'Agent Profiles, LLM Profiles, MCP Profiles, Knowledge Context, AAA, and pipeline schema each own a different part of AI-enabled automation.',
        keyFacts: [
          'Agent Profiles control persona, role, and prompt instructions.',
          'LLM Profiles control provider, model, endpoint, credential, token settings, and generation settings.',
          'MCP Profiles control approved external servers and tools.',
          'Knowledge Context supplies architecture, rules, policies, examples, and runbooks to tasks.',
          'Selecting an Agent Profile does not select an LLM, grant tools, reveal secrets, or grant permissions.',
        ],
        details: [
          'Profile resolution is independent and layered. Step settings can override pipeline settings, and task settings can override LLM profile selection where supported.',
          'AAA still decides whether the original caller may use each selected profile, tool boundary, knowledge document, secret, or scope.',
        ],
        configRows: [
          {
            key: 'setting/system/agent-profiles.yaml',
            area: 'GitOps',
            description: 'System AI personas and default persona.',
            example: 'setting/system/agent-profiles.yaml',
          },
          {
            key: 'ai-profiles.yaml',
            area: 'Team GitOps',
            description: 'Team-level AI profile overlays.',
            example: 'ai-profiles.yaml',
          },
        ],
        examples: [],
        relatedDocs: ['doc/agent-profiles.md', 'doc/llm-model-selection.md'],
        runbooks: ['Review a new Agent Profile', 'Limit AI profile use by scope', 'Audit team AI overlays'],
        caveats: ['Team profiles overlay the system catalog for runs owned by that team.'],
      },
      {
        id: 'llm-profiles',
        title: 'LLM Profiles',
        level: 'Admin',
        audience: 'AI platform owners and administrators',
        summary:
          'LLM Profiles define provider, model, endpoint, credential reference, allowed scopes, timeouts, tokens, temperature, and provider-specific extra settings.',
        keyFacts: [
          'Supported providers include gemini, lmstudio, openai, anthropic, groq, mistral, ollama, openrouter, and azure-openai.',
          'Resolution order is task, step, pipeline, then default profile.',
          'Generic reasoning and thinking fields are supported only for LM Studio; other providers reject those generic settings.',
          'Script-only pipelines with llm_enabled: false can run without a configured LLM registry.',
        ],
        details: [
          'Profiles should use credential_ref for hosted provider API keys instead of plaintext secrets.',
          'allowed_scopes lets administrators restrict model use by runtime context.',
        ],
        configRows: [
          {
            key: 'setting/system/llm_profile.yaml',
            area: 'GitOps',
            description: 'System LLM profile registry.',
            example: 'setting/system/llm_profile.yaml',
          },
          {
            key: 'profiles[].allowed_scopes',
            area: 'LLM profile',
            description: 'Scopes where a profile may be used.',
            example: '[dev, internal]',
          },
        ],
        examples: [
          {
            title: 'LLM profile registry',
            language: 'yaml',
            code:
              'default_profile: standard\nprofiles:\n  - name: standard\n    provider: openai\n    model: gpt-4.1-mini\n    credential_ref: credential://system/llm/openai-hosted\n    timeout_seconds: 60\n    max_tokens: 4096\n  - name: local\n    provider: ollama\n    model: qwen2.5-coder:14b\n    base_url: http://ollama:11434/v1',
          },
        ],
        relatedDocs: ['doc/llm-model-selection.md', 'doc/credential-management.md'],
        runbooks: ['Add LLM profile', 'Test profile reachability', 'Migrate default model profile'],
        caveats: ['Provider-specific reasoning fields should be validated per provider instead of copied between providers.'],
      },
      {
        id: 'mcp',
        title: 'External and Hosted MCP',
        level: 'Admin',
        audience: 'AI platform teams, tool owners, and automation authors',
        summary:
          'Pipelines may reference approved external streamable-HTTP MCP profiles, and NopsAI also exposes a first-party hosted MCP endpoint at POST /v1/mcp.',
        keyFacts: [
          'External MCP servers define display name, provider, transport, URL, auth type, credential reference, timeout, enabled state, and tools.',
          'MCP profile inheritance for goals is additive across pipeline, step, and task profiles.',
          'Explicit MCP profiles are rejected on script steps, script tasks, and include placeholders.',
          'When MCP profiles resolve for a goal, the agent requires at least one successful MCP tool call before accepting the final action.',
          'Hosted NopsAI MCP filters tools and resources against the authenticated subject AAA permissions.',
        ],
        details: [
          'Hosted MCP supports initialize, tools/list, tools/call, resources/list, and resources/read JSON-RPC methods.',
          'Mutation tools require confirmation. GitOps proposal tools return commit-ready files with applies: false rather than silently applying production changes.',
        ],
        configRows: [
          {
            key: 'setting/system/mcp.yaml',
            area: 'GitOps',
            description: 'System MCP server and profile registry.',
            example: 'setting/system/mcp.yaml',
          },
          {
            key: 'mcp_servers[].transport',
            area: 'MCP server',
            description: 'Current external MCP transport.',
            example: 'streamable_http',
          },
        ],
        examples: [
          {
            title: 'External MCP profile',
            language: 'yaml',
            code:
              'mcp_servers:\n  github:\n    display_name: GitHub MCP\n    enabled: true\n    provider: github\n    transport: streamable_http\n    url: https://api.githubcopilot.com/mcp/x/all/readonly\n    auth_type: bearer_token\n    credential_ref: credential://system/mcp/github-readonly\n    timeout: 30s\nmcp_profiles:\n  github-pr-review:\n    enabled: true\n    servers:\n      - server: github\n        tools: ["*"]',
          },
        ],
        relatedDocs: ['doc/mcp-pipeline-integration.md', 'doc/mcp-feature-coverage.md'],
        runbooks: ['Register MCP server', 'Restrict MCP tools', 'Review hosted MCP confirmation policy'],
        caveats: ['Stdio and sidecar MCP server transports are future extensions, not current deployment behavior.'],
      },
      {
        id: 'knowledge-context',
        title: 'Knowledge Context',
        level: 'Reference',
        audience: 'Automation authors, architects, and governance teams',
        summary:
          'Knowledge Context lets pipelines attach governed project knowledge to LLM-backed work and snapshots the resolved content with each run.',
        keyFacts: [
          'Supported kinds are architecture, guardrail, policy, adr, guideline, runbook, reference, and example.',
          'Knowledge may be declared at pipeline, step, and task level, then merged into effective task context.',
          'Managed documents require knowledge_context.use. Repo-local files are loaded from the run repository at the run commit.',
          'Guardrails and policies are strict prompt constraints for goals, commands, scripts, file writes, MCP calls, MCP arguments, and conditions.',
        ],
        details: [
          'A reference must use kind plus ref for managed knowledge, or kind plus a safe relative path for repo-local markdown.',
          'If a guardrail or policy conflicts with a requested action, the agent should explain the block rather than perform the prohibited action, and that block is treated as task failure.',
        ],
        configRows: [
          {
            key: 'knowledge/',
            area: 'GitOps',
            description: 'Managed markdown knowledge documents grouped by kind and team.',
            example: 'knowledge/guardrail/security/repo-check.md',
          },
        ],
        examples: [
          {
            title: 'Knowledge references',
            language: 'yaml',
            code:
              'knowledge_context:\n  - kind: guardrail\n    ref: security/repository-policy\n    required: true\n  - kind: architecture\n    path: .nopsai/docs/backend.md\n    required: true',
          },
        ],
        relatedDocs: ['doc/knowledge-context.md', 'doc/sample-config-repo/README.md'],
        runbooks: ['Publish release knowledge context', 'Investigate required knowledge resolution failure', 'Review policy and guardrail changes'],
        caveats: ['Repo-local paths must be safe relative paths and cannot include path traversal.'],
      },
      {
        id: 'final-deliverables',
        title: 'Final Deliverables',
        level: 'Reference',
        audience: 'Automation authors and stakeholders consuming run results',
        summary:
          'Final outputs are generated after execution and can produce Markdown, JSON, PDF, HTML, and Excel deliverables from completed run context.',
        keyFacts: [
          'Supported when values are success, failure, and always.',
          'Providers must return a single <final_output> envelope.',
          'Malformed, duplicate, empty, or missing envelopes fail validation and allow one corrective retry.',
          'PDF and HTML use validated DocumentSpec; Excel uses typed SpreadsheetSpec and rejects formulas and object/array cell values.',
        ],
        details: [
          'PDF rendering uses Gotenberg through FINAL_OUTPUT_PDF_RENDERER_URL. Pipeline YAML never contains renderer infrastructure URLs.',
          'Provider and network failures are not retried by this feature. Contract and schema violations get one corrective retry.',
        ],
        configRows: [
          {
            key: 'output.items[].type',
            area: 'Pipeline YAML',
            description: 'Final deliverable type.',
            example: 'pdf',
          },
          {
            key: 'output.items[].when',
            area: 'Pipeline YAML',
            description: 'Execution status that triggers generation.',
            example: 'always',
          },
        ],
        examples: [
          {
            title: 'Final output configuration',
            language: 'yaml',
            code:
              'output:\n  llm_profile: report-writer\n  items:\n    - name: Executive summary\n      type: markdown\n      when: success\n      prompt: Summarize the successful run.\n    - name: Failure analysis\n      type: pdf\n      when: failure\n      prompt: Explain the failure and recommend corrective actions.',
          },
        ],
        relatedDocs: ['doc/final-output-rendering.md'],
        runbooks: ['Diagnose failed PDF render', 'Review final-output contract violations', 'Validate Excel deliverable schema'],
        caveats: ['Final-output failure is tracked separately from execution failure.'],
      },
    ],
  },
  {
    id: 'operations',
    title: 'Operations',
    owner: 'SRE and operations',
    description: 'Monitoring, metrics, logs, notifications, backups, cleanup, troubleshooting, and release integrity.',
    articles: [
      {
        id: 'monitoring-metrics',
        title: 'Monitoring and Metrics',
        level: 'Operate',
        audience: 'SREs, administrators, and reliability owners',
        summary:
          'Monitoring covers runs, pipelines, steps, tasks, triggers, external triggers, runners, LLM usage, reliability, efficiency, and security, with authorization-filtered responses.',
        keyFacts: [
          'Monitoring filters include time range, team, pipeline, repository, run ID, trigger source, status, subject identity, external trigger, schedule, duration, and AI dimensions.',
          'Monitoring responses are filtered through pipeline_run.list so users see only authorized runs.',
          'The API exposes Prometheus metrics at GET /metrics.',
          'Metrics cover run status, queue duration, final outputs, external triggers, LLM usage, runner utilization, approvals, audit, notifications, System Logs, and build identity.',
        ],
        details: [
          'The Helm API Service includes Prometheus scrape annotations by default.',
          'Use monitoring data to size runners and resource requests because the repository does not publish validated production sizing tiers.',
        ],
        configRows: [
          {
            key: 'api.service.annotations.prometheus.io/scrape',
            area: 'Helm',
            description: 'Prometheus scrape annotation for the API Service.',
            example: 'true',
          },
        ],
        examples: [],
        relatedDocs: ['doc/system-logs.md', 'doc/enterprise-gates.md'],
        runbooks: ['Build runner capacity dashboard', 'Review AI usage by profile', 'Alert on approval wait time'],
        caveats: ['Do not infer production throughput from default Helm resource blocks; they are intentionally empty.'],
      },
      {
        id: 'system-logs',
        title: 'Pipeline Logs and System Logs',
        level: 'Troubleshoot',
        audience: 'Operators and support engineers',
        summary:
          'Pipeline logs are durable run records in PostgreSQL; System Logs are live allow-listed platform container or pod logs streamed through authenticated SSE.',
        keyFacts: [
          'System Logs allow-listed sources are nopsai, aaa, dispatcher, git-bot, ui, docker-runner, and k8s-runner.',
          'Docker deployments read through a least-privilege socket proxy.',
          'Kubernetes deployments read label-selected pods through read-only pods and pods/log RBAC.',
          'System Logs redaction is best effort and system_log.read should be narrowly assigned.',
        ],
        details: [
          'System Logs apply line and stream limits, buffer recent sanitized entries in memory, and do not copy platform logs into pipeline_run_logs.',
          'Build-only base, agent, and pipeline images are not exposed as System Logs sources.',
        ],
        configRows: [
          {
            key: 'systemLogs.enabled',
            area: 'Helm',
            description: 'Enables System Logs support in the chart.',
            example: 'true',
          },
          {
            key: 'systemLogs.kubernetes.rbac.create',
            area: 'Helm',
            description: 'Creates read-only pods and pods/log permissions for API System Logs.',
            example: 'true',
          },
        ],
        examples: [],
        relatedDocs: ['doc/system-logs.md'],
        runbooks: ['Inspect dispatcher logs', 'Audit system_log.read grants', 'Tune System Logs line limits'],
        caveats: ['System Logs are live operational evidence, not durable pipeline history.'],
      },
      {
        id: 'notifications',
        title: 'Notifications',
        level: 'Operate',
        audience: 'Operations and team administrators',
        summary:
          'SMTP settings use credential references, and team notification policies route run and approval events by team, pipeline, repository, branch, recipients, exclusions, and throttle behavior.',
        keyFacts: [
          'Mail settings include enabled, from_address, smtp_host, smtp_port, smtp_start_tls, smtp_username, and smtp_password_credential_ref.',
          'Supported events include running, pending, success, failure, cancelled, approval requested, approval approved, and approval rejected.',
          'Notification policies can inherit through team lineage.',
          'Delivery records and dedupe behavior are persisted for operational review.',
        ],
        details: [
          'SMTP password is a credential reference, never plaintext configuration.',
          'Run team path is important for schedule and external-trigger notification routing.',
        ],
        configRows: [
          {
            key: 'setting/system/mail.yaml',
            area: 'GitOps',
            description: 'SMTP notification settings.',
            example: 'setting/system/mail.yaml',
          },
          {
            key: 'smtp_password_credential_ref',
            area: 'Mail',
            description: 'Credential reference for SMTP password.',
            example: 'credential://system/mail/smtp-primary',
          },
        ],
        examples: [],
        relatedDocs: ['doc/feature-reference.md', 'doc/team-resource-ownership-design.md'],
        runbooks: ['Test SMTP settings', 'Review notification delivery failures', 'Add production approval route'],
        caveats: ['Notification routing follows run ownership, not only pipeline source repository.'],
      },
      {
        id: 'backups-cleanup',
        title: 'Backups, Cleanup, and Restore Boundary',
        level: 'Operate',
        audience: 'Operators and data-management owners',
        summary:
          'The product can create, list, checksum, download, and clean up gzip-compressed JSON Lines backups, but a complete product-managed restore workflow is not currently implemented.',
        keyFacts: [
          'Backup types are full, runs, and logs.',
          'Cleanup targets are runs and logs.',
          'Cleanup modes include keep_last, older_than_days, all_terminal_runs, and all_logs.',
          'Cleanup may create a backup before deleting data and has scheduled job metadata.',
        ],
        details: [
          'Backup files are written with mode 0600 and the backup directory is created with mode 0700.',
          'Database recovery should use an operator-controlled, tested process until a formal NopsAI restore workflow exists.',
        ],
        configRows: [
          {
            key: '/data/backups',
            area: 'Filesystem',
            description: 'Default API container path for product-generated backup files.',
            example: '/data/backups',
          },
        ],
        examples: [],
        relatedDocs: ['doc/feature-reference.md', 'doc/enterprise-gates.md'],
        runbooks: ['Download and verify backup checksum', 'Run cleanup with pre-cleanup backup', 'Test external database restore'],
        caveats: ['Product backup files can disappear after container restart unless /data/backups is mounted or exported.'],
      },
      {
        id: 'release-integrity',
        title: 'Release Integrity and Compatibility',
        level: 'Admin',
        audience: 'Release engineers and platform operators',
        summary:
          'A production release is one compatible bundle containing API, AAA, dispatcher, git-bot, UI, agent, Docker runner, Kubernetes runner, Helm chart, and CLI metadata.',
        keyFacts: [
          'Release manifests pin image and chart digests and record compatibility and migration policy.',
          'Do not combine services from different manifests or deploy floating tags.',
          'The CLI can verify release manifests and chart digests, render plans, and perform Helm deployments.',
          'Release locks are GitOps-readable evidence for deployed version state.',
        ],
        details: [
          'Enterprise gates test release compatibility, manifest strictness, digest pinning, chart verification, Helm rendering, /version, and build metadata.',
          'The user-facing CLI binary is `nopsai`; the control-plane server binary is `nopsai-api`.',
        ],
        configRows: [
          {
            key: 'global.releaseVersion',
            area: 'Helm',
            description: 'Release version written into chart values.',
            example: '1.2.3',
          },
          {
            key: 'global.sourceCommit',
            area: 'Helm',
            description: 'Source commit associated with a release bundle.',
            example: '494d57c',
          },
        ],
        examples: [],
        relatedDocs: ['doc/release-bundles.md', 'doc/cli.md', 'doc/enterprise-gates.md'],
        runbooks: ['Verify release manifest before deploy', 'Render Helm deployment plan', 'Preserve deployment release lock'],
        caveats: ['Platform services should not be selected independently in production.'],
      },
    ],
  },
  {
    id: 'interfaces',
    title: 'Interfaces',
    owner: 'Developer experience',
    description: 'REST API, CLI, hosted MCP, API discovery, and support diagnostics.',
    articles: [
      {
        id: 'api',
        title: 'REST API',
        level: 'Reference',
        audience: 'Integration authors and support engineers',
        summary:
          'The API covers authentication, setup, teams, pipelines, runs, approvals, outputs, schedules, triggers, scopes, credentials, knowledge, profiles, monitoring, logs, dispatch, access, backups, cleanup, and hosted MCP.',
        keyFacts: [
          'Protected requests go through bearer authentication, route authorization, resource checks, and audit regardless of UI, CLI, or API origin.',
          'API categories include auth, setup, teams, pipelines, steps, runs, schedules, triggers, Git webhook sources, external triggers, scopes, credentials, knowledge, LLM, Agent, MCP, notifications, monitoring, System Logs, dispatcher, access, users, service accounts, identity providers, audit, backups, cleanup, and hosted MCP.',
          'Binary downloads and SSE responses are preserved by CLI generic API commands.',
        ],
        details: [
          'Route registration parity is tested against the generated CLI API catalog so new server APIs are discoverable through the CLI.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/api.md', 'doc/cli.md'],
        runbooks: ['Validate API token', 'Describe a route from the CLI', 'Replay a safe read-only request'],
        caveats: ['Use route-level and resource-level authorization together; route access alone is not enough for runtime resource use.'],
      },
      {
        id: 'cli',
        title: 'CLI',
        level: 'Reference',
        audience: 'Operators and automation authors',
        summary:
          'The released `nopsai` CLI manages contexts, credentials, generic API access, platform diagnostics, completion files, GitOps deployment, and release verification.',
        keyFacts: [
          'Accepted token types are access JWTs, nopat_ personal tokens, and nopsat_ service-account tokens.',
          'Local config and credential files are atomically written with 0600 permissions inside a 0700 directory.',
          'The CLI rejects credential files with broader permissions.',
          'platform doctor checks local tools, API readiness, setup preflight, metrics, token acceptance, dispatcher monitoring, and runner count.',
        ],
        details: [
          'Optional missing local tools and missing dispatcher-read permission are warnings. API readiness, connectivity, malformed responses, metrics failures, and rejected tokens are errors.',
        ],
        configRows: [
          {
            key: 'nopsai context add',
            area: 'CLI',
            description: 'Adds a named API context.',
            example: 'nopsai context add prod --api https://api.nopsai.example',
          },
          {
            key: 'nopsai platform doctor',
            area: 'CLI',
            description: 'Runs platform diagnostics.',
            example: 'nopsai platform doctor --output json',
          },
        ],
        examples: [
          {
            title: 'Generic API access',
            language: 'bash',
            code:
              'nopsai api routes --domain monitoring --method GET\nnopsai api describe GET "/v1/pipelines/{pipelineName...}"\nnopsai api request GET /v1/monitoring/summary',
          },
        ],
        relatedDocs: ['doc/cli.md', 'doc/release-bundles.md'],
        runbooks: ['Create production CLI context', 'Run platform doctor after deploy', 'Download support evidence safely'],
        caveats: ['Install or upgrade local Helm when release chart validation requires a newer Helm version.'],
      },
      {
        id: 'hosted-mcp-interface',
        title: 'Hosted NopsAI MCP Interface',
        level: 'Reference',
        audience: 'Assistant authors and automation integrators',
        summary:
          'NopsAI exposes itself as a first-party MCP server with tool and resource filtering backed by authenticated user permissions.',
        keyFacts: [
          'The endpoint is POST /v1/mcp.',
          'Supported JSON-RPC methods include initialize, tools/list, tools/call, resources/list, and resources/read.',
          'Capabilities include setup inspection, pipeline listing/search/validation, run analysis, approvals, schedules, knowledge, Git webhook sources, external triggers, config sync, notifications, monitoring, credentials metadata, runner controls, access, audit, users, service accounts, identity providers, backups, and cleanup.',
        ],
        details: [
          'Confirmed run operations and mutation tools require explicit confirmation, preserving enterprise change-control boundaries.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/mcp-feature-coverage.md', 'doc/mcp-pipeline-integration.md'],
        runbooks: ['Review hosted MCP tool allow-list', 'Confirm a mutation action', 'Audit assistant-triggered recommendations'],
        caveats: ['Hosted MCP exposes only what the authenticated subject may see or do.'],
      },
    ],
  },
  {
    id: 'security-reference',
    title: 'Security and Reference',
    owner: 'Security and enterprise platform owners',
    description: 'Production hardening, troubleshooting, data model, feature combinations, and confirmed implementation gaps.',
    articles: [
      {
        id: 'production-hardening',
        title: 'Production Hardening Checklist',
        level: 'Security',
        audience: 'Enterprise administrators and security reviewers',
        summary:
          'Production mode requires strong independent secrets, dispatcher transport security, private control-plane networking, least-privilege runners, metrics, backups, and release locks.',
        keyFacts: [
          'Set NOPSAI_ENVIRONMENT=production or NOPSAI_REQUIRE_PRODUCTION_GATES=true.',
          'Replace every local fallback secret and use separate user/API and internal service JWT signing keys.',
          'Configure DISPATCHER_TLS_MODE=mtls or tls and protect the dispatcher trust seed.',
          'Keep PostgreSQL, AAA, Gotenberg, and Docker socket proxy private.',
          'Scrape /metrics, test SMTP, test LLM/MCP profiles, run platform doctor, and test recovery procedures.',
        ],
        details: [
          'Startup gates cover critical secret and transport requirements, but they do not replace infrastructure hardening, access reviews, external database backups, alerting, or recovery drills.',
        ],
        configRows: [
          {
            key: 'NOPSAI_ENVIRONMENT',
            area: 'Production gates',
            description: 'Enables production environment behavior.',
            example: 'production',
          },
          {
            key: 'NOPSAI_REQUIRE_PRODUCTION_GATES',
            area: 'Production gates',
            description: 'Fails closed when hardening gates are incomplete.',
            example: 'true',
          },
        ],
        examples: [],
        relatedDocs: ['doc/enterprise-gates.md', 'doc/license-compliance.md', 'doc/release-bundles.md'],
        runbooks: ['Run enterprise gates locally', 'Review production preflight', 'Perform quarterly access review'],
        caveats: ['Production gates are a baseline, not a full security program.'],
      },
      {
        id: 'troubleshooting',
        title: 'Troubleshooting Index',
        level: 'Troubleshoot',
        audience: 'Support and operations engineers',
        summary:
          'Common failures usually map to bootstrap configuration, dispatcher/runner reachability, storage, Git trigger matching, LLM/MCP validation, PDF rendering, GitOps drift, or backup durability.',
        keyFacts: [
          'Login setup preflight usually points to DATABASE_URL, NOPSAI_MASTER_KEY, or JWT_SIGNING_KEY.',
          'Runner registration failures usually involve dispatcher reachability, service JWT mismatch, TLS mode/trust mismatch, duplicate runner ID, invalid scopes, or missing Kubernetes RBAC.',
          'Runs remain queued when no runner is eligible or capacity is exhausted.',
          'Git webhook no-run outcomes often involve source auth, allowlists, trigger assignment, branch/path filters, synchronized pipelines, or resource access.',
          'PDF failures usually involve Gotenberg health, FINAL_OUTPUT_PDF_RENDERER_URL, timeout, or API-to-Gotenberg networking.',
        ],
        details: [
          'Config sync reverting a UI change means the resource is GitOps-managed. Commit the desired change to the owning config repository or push drift through review-branch workflow.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/system-logs.md', 'doc/git-webhook-sources.md', 'doc/final-output-rendering.md'],
        runbooks: ['Debug queued run', 'Investigate Kubernetes workspace mount failure', 'Resolve GitOps sync revert'],
        caveats: ['System Logs redaction is best effort; avoid attaching unrestricted logs to support cases.'],
      },
      {
        id: 'known-limits',
        title: 'Confirmed Gaps and Limits',
        level: 'Reference',
        audience: 'Product, sales, security, and implementation teams',
        summary:
          'The current implementation should not claim unsupported deployment automation, storage backends, autoscaling, air-gap, restore, sizing, or HA guarantees without new code and docs.',
        keyFacts: unsupportedClaims,
        details: [
          'Generic Git providers cannot fetch repository pipeline files in v1.',
          'Docker runners ignore Kubernetes runtime pools and affinity.',
          'Kubernetes emptyDir is not used for shared run workspaces.',
          'Credential values are intentionally not readable after submission.',
          'Final-output provider and network errors are not retried by final-output generation.',
        ],
        configRows: [],
        examples: [],
        relatedDocs: ['doc/README.md', 'doc/enterprise-refactor-roadmap.md'],
        runbooks: ['Check a sales or docs claim against implementation evidence', 'Track future-state capability in roadmap docs'],
        caveats: ['Future additions are valid roadmap items, but they should not appear in the current-version wiki as implemented behavior.'],
      },
    ],
  },
];

export function summarizeWiki(sections: WikiSection[] = wikiSections): WikiSummary {
  const articles = sections.flatMap(section => section.articles);
  return {
    sections: sections.length,
    articles: articles.length,
    configKeys: collectWikiConfigKeys(sections).length,
    runbooks: new Set(articles.flatMap(article => article.runbooks)).size,
    caveats: articles.reduce((total, article) => total + article.caveats.length, 0),
  };
}

export function collectWikiConfigKeys(sections: WikiSection[] = wikiSections) {
  return Array.from(new Set(sections.flatMap(section => section.articles.flatMap(article => article.configRows.map(row => row.key))))).sort();
}

export function getFirstWikiArticleID(sections: WikiSection[] = wikiSections) {
  return sections[0]?.articles[0]?.id || '';
}

export function findWikiArticle(sections: WikiSection[], articleID: string) {
  for (const section of sections) {
    const article = section.articles.find(candidate => candidate.id === articleID);
    if (article) return article;
  }
  return undefined;
}

export function findWikiSectionForArticle(sections: WikiSection[], articleID: string) {
  return sections.find(section => section.articles.some(article => article.id === articleID));
}

export function filterWikiSections(sections: WikiSection[], query: string) {
  const normalized = normalizeWikiQuery(query);
  if (!normalized) return sections;

  return sections
    .map(section => {
      const sectionMatches = sectionHaystack(section).includes(normalized);
      const articles = section.articles.filter(article => sectionMatches || articleHaystack(article).includes(normalized));
      return { ...section, articles };
    })
    .filter(section => section.articles.length > 0);
}

export function countWikiArticles(sections: WikiSection[]) {
  return sections.reduce((total, section) => total + section.articles.length, 0);
}

function normalizeWikiQuery(query: string) {
  return query.trim().toLowerCase();
}

function sectionHaystack(section: WikiSection) {
  return [section.id, section.title, section.owner, section.description].join(' ').toLowerCase();
}

function articleHaystack(article: WikiArticle) {
  return [
    article.id,
    article.title,
    article.level,
    article.audience,
    article.summary,
    ...article.keyFacts,
    ...article.details,
    ...article.configRows.flatMap(row => [row.key, row.area, row.description, row.example]),
    ...article.examples.flatMap(example => [example.title, example.language, example.code]),
    ...article.relatedDocs,
    ...article.runbooks,
    ...article.caveats,
  ]
    .join(' ')
    .toLowerCase();
}
