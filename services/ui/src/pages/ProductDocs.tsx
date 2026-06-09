import { useMemo, useState, type ReactNode } from 'react';
import {
  BookOpen,
  Braces,
  CheckCircle2,
  Cloud,
  Copy,
  FileText,
  Folder,
  GitBranch,
  HardDrive,
  History,
  Lock,
  Search,
  Server,
  ShieldCheck,
  type LucideIcon,
} from 'lucide-react';

type Version = {
  id: string;
  label: string;
  status: string;
  release: string;
  highlights: string[];
};

type ConfigurationItem = {
  key: string;
  purpose: string;
  example: string;
};

type DocArea = {
  id: string;
  title: string;
  summary: string;
  icon: LucideIcon;
  topics: string[];
  checklist: string[];
  commands: string[];
  configuration: ConfigurationItem[];
};

const versions: Version[] = [
  {
    id: 'v1-current',
    label: 'v1 Current',
    status: 'Recommended for production',
    release: 'Stable product documentation',
    highlights: ['Access controlled runs', 'Monitoring dashboard', 'Config repositories', 'MCP-ready system settings'],
  },
  {
    id: 'v0-lts',
    label: 'v0 LTS',
    status: 'Maintenance mode',
    release: 'Legacy installation and migration notes',
    highlights: ['Classic pipelines', 'Manual runners', 'Basic trigger support', 'Migration checklist'],
  },
  {
    id: 'next',
    label: 'Next',
    status: 'Preview',
    release: 'Forward-looking docs for upcoming enterprise modules',
    highlights: ['AI assistant guidance', 'Hosted MCP profiles', 'Advanced cost controls', 'Policy-driven docs'],
  },
];

const docAreas: DocArea[] = [
  {
    id: 'installation',
    title: 'Installation',
    summary: 'Install Nopsai locally, in Kubernetes, in managed cloud environments, or in private on-prem networks.',
    icon: Server,
    topics: ['Docker quickstart', 'Kubernetes deployment', 'Cloud bootstrap', 'On-prem hardened install'],
    checklist: ['Pick deployment profile', 'Configure database and object storage', 'Create initial admin', 'Verify runner connectivity'],
    commands: [
      'docker compose up -d nopsai postgres redis',
      'helm upgrade --install nopsai ./charts/nopsai -f values.production.yaml',
      'nopsai setup bootstrap --profile onprem --admin admin@example.com',
    ],
    configuration: [
      { key: 'NOPS_ENV', purpose: 'Selects local, staging, production, cloud, or onprem behavior.', example: 'production' },
      { key: 'NOPS_PUBLIC_URL', purpose: 'Canonical URL used for webhooks, callbacks, and documentation links.', example: 'https://nopsai.example.com' },
      { key: 'NOPS_STORAGE_BACKEND', purpose: 'Chooses local, S3-compatible, or managed artifact storage.', example: 's3' },
    ],
  },
  {
    id: 'configuration',
    title: 'Configuration reference',
    summary: 'Document every runtime, runner, LLM, MCP, trigger, schedule, and access configuration with examples.',
    icon: Braces,
    topics: ['Runtime config', 'Pipeline YAML', 'Runner pools', 'LLM and MCP profiles'],
    checklist: ['Define config ownership', 'Validate sensitive values', 'Track drift against config repos', 'Publish version-specific examples'],
    commands: [
      'nopsai config validate ./config/production.yaml',
      'nopsai config diff --source git --target runtime',
      'nopsai config export --format yaml > backup.yaml',
    ],
    configuration: [
      { key: 'runner.max_concurrency', purpose: 'Maximum active jobs per runner pool.', example: '12' },
      { key: 'pipeline.default_timeout', purpose: 'Default timeout applied when a pipeline omits one.', example: '45m' },
      { key: 'mcp.profiles[].allowed_tools', purpose: 'Boundary-controlled tool access for internal assistant workflows.', example: '["github", "slack", "jira"]' },
    ],
  },
  {
    id: 'docker',
    title: 'Docker deployment',
    summary: 'A complete single-node deployment guide for demos, development, labs, and small private installations.',
    icon: Copy,
    topics: ['Compose services', 'Volume layout', 'TLS reverse proxy', 'Backup and restore'],
    checklist: ['Pin images by version', 'Mount persistent volumes', 'Set admin credentials through secrets', 'Enable health checks'],
    commands: [
      'cp deploy/docker/.env.example .env',
      'docker compose pull && docker compose up -d',
      'docker compose exec nopsai nopsai health check',
    ],
    configuration: [
      { key: 'POSTGRES_DSN', purpose: 'Database connection used by the API service.', example: 'postgres://nopsai:***@postgres:5432/nopsai' },
      { key: 'REDIS_URL', purpose: 'Queue and short-lived cache endpoint.', example: 'redis://redis:6379/0' },
      { key: 'RUNNER_DOCKER_SOCKET', purpose: 'Optional host socket for Docker-based runner execution.', example: '/var/run/docker.sock' },
    ],
  },
  {
    id: 'kubernetes',
    title: 'Kubernetes deployment',
    summary: 'Production-grade installation guide for clusters with ingress, autoscaling, secrets, and runner isolation.',
    icon: GitBranch,
    topics: ['Helm values', 'Ingress and TLS', 'Runner namespaces', 'Horizontal scaling'],
    checklist: ['Create namespace', 'Install secrets', 'Apply network policies', 'Separate control plane and runner workloads'],
    commands: [
      'kubectl create namespace nopsai',
      'kubectl create secret generic nopsai-secrets --from-env-file=.env -n nopsai',
      'helm upgrade --install nopsai ./charts/nopsai -n nopsai -f values.production.yaml',
    ],
    configuration: [
      { key: 'api.replicas', purpose: 'API pod replica count.', example: '3' },
      { key: 'runner.podTemplate', purpose: 'Default runner pod resources, labels, tolerations, and node selectors.', example: 'runner.large' },
      { key: 'ingress.hosts', purpose: 'Public hostnames exposed through ingress.', example: 'nopsai.company.internal' },
    ],
  },
  {
    id: 'cloud-onprem',
    title: 'Cloud and on-prem',
    summary: 'Runbook for managed cloud accounts, private VPCs, restricted data centers, and air-gapped environments.',
    icon: Cloud,
    topics: ['Cloud identity', 'Private networking', 'Air-gapped packages', 'Enterprise support bundle'],
    checklist: ['Define identity provider', 'Map data residency requirements', 'Mirror images and packages', 'Prepare offline upgrade process'],
    commands: [
      'nopsai bundle create --version v1-current --target airgap',
      'nopsai bundle verify ./nopsai-airgap.tar.zst',
      'nopsai support collect --redact --output support-bundle.zip',
    ],
    configuration: [
      { key: 'auth.oidc.issuer', purpose: 'OIDC issuer for SSO and role mapping.', example: 'https://login.example.com' },
      { key: 'network.egress_mode', purpose: 'Controls public, private, proxy-only, or air-gapped behavior.', example: 'proxy-only' },
      { key: 'telemetry.mode', purpose: 'Telemetry behavior for regulated deployments.', example: 'local-only' },
    ],
  },
  {
    id: 'operations',
    title: 'Operations and maintenance',
    summary: 'Operational docs for monitoring, backups, upgrades, incident response, and performance tuning.',
    icon: History,
    topics: ['Monitoring', 'Backups', 'Upgrades', 'Incident response'],
    checklist: ['Review run latency', 'Audit token usage', 'Check failed triggers', 'Test restore every release'],
    commands: [
      'nopsai backup create --include-config --include-artifacts',
      'nopsai upgrade plan --from v0-lts --to v1-current',
      'nopsai incidents timeline --run-id RUN_ID',
    ],
    configuration: [
      { key: 'monitoring.retention_days', purpose: 'How long run metrics and service history remain queryable.', example: '180' },
      { key: 'audit.enabled', purpose: 'Enables security and configuration audit logs.', example: 'true' },
      { key: 'cost.token_budget_monthly', purpose: 'Soft budget for AI-assisted pipeline operations.', example: '25000000' },
    ],
  },
  {
    id: 'security',
    title: 'Security and access',
    summary: 'Versioned security docs for RBAC, resource visibility, service accounts, secrets, and audit controls.',
    icon: Lock,
    topics: ['RBAC', 'Resource access', 'Secrets', 'Service accounts'],
    checklist: ['Use least privilege roles', 'Rotate personal access tokens', 'Scope service accounts', 'Document break-glass access'],
    commands: [
      'nopsai access review --group platform',
      'nopsai tokens rotate --service-account runner-prod',
      'nopsai secrets scan --scope workspace',
    ],
    configuration: [
      { key: 'access.default_visibility', purpose: 'Default visibility for newly created pipelines, scopes, and contexts.', example: 'group' },
      { key: 'secrets.provider', purpose: 'Secret backend used by runtime and runners.', example: 'vault' },
      { key: 'audit.redaction', purpose: 'Redaction policy applied to logs, prompts, and support bundles.', example: 'strict' },
    ],
  },
];

function matchesQuery(area: DocArea, query: string) {
  const haystack = [
    area.title,
    area.summary,
    ...area.topics,
    ...area.checklist,
    ...area.commands,
    ...area.configuration.flatMap(item => [item.key, item.purpose, item.example]),
  ]
    .join(' ')
    .toLowerCase();
  return haystack.includes(query.trim().toLowerCase());
}

function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <section className={`rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm ${className}`}>{children}</section>;
}

export default function ProductDocsPage() {
  const [selectedVersionId, setSelectedVersionId] = useState(versions[0].id);
  const [activeAreaId, setActiveAreaId] = useState(docAreas[0].id);
  const [query, setQuery] = useState('');

  const selectedVersion = versions.find(version => version.id === selectedVersionId) || versions[0];
  const visibleAreas = useMemo(() => (query.trim() ? docAreas.filter(area => matchesQuery(area, query)) : docAreas), [query]);
  const activeArea = visibleAreas.find(area => area.id === activeAreaId) || visibleAreas[0] || docAreas[0];
  const ActiveIcon = activeArea.icon;

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto w-full max-w-[1500px] px-4 py-5 sm:px-6 lg:px-8 space-y-6">
        <div className="rounded-3xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] p-6 shadow-sm lg:p-8">
          <div className="flex flex-col gap-6 xl:flex-row xl:items-end xl:justify-between">
            <div className="max-w-3xl">
              <div className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-3 py-1.5 text-xs font-semibold text-[var(--text-secondary)]">
                <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
                Versioned product wiki
              </div>
              <h1 className="mt-4 text-3xl font-semibold text-[var(--text-primary)]">Product documentation</h1>
              <p className="mt-3 text-sm leading-6 text-[var(--text-secondary)]">
                Centralized product docs for installation, deployment, configuration, security, and operations. Each section is structured so every Nopsai release can keep its own tested instructions and examples.
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-2 xl:w-[34rem]">
              <label className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">
                Version
                <select
                  value={selectedVersionId}
                  onChange={event => setSelectedVersionId(event.target.value)}
                  className="mt-2 h-11 w-full rounded-xl border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 text-sm normal-case tracking-normal text-[var(--text-primary)] focus:border-[var(--border-input-focus)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent-focus-ring)]"
                >
                  {versions.map(version => (
                    <option key={version.id} value={version.id}>{version.label}</option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">
                Search docs
                <span className="mt-2 flex h-11 items-center gap-2 rounded-xl border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 focus-within:border-[var(--border-input-focus)] focus-within:ring-2 focus-within:ring-[var(--border-accent-focus-ring)]">
                  <Search className="h-4 w-4 text-[var(--text-tertiary)]" />
                  <input
                    value={query}
                    onChange={event => setQuery(event.target.value)}
                    placeholder="docker, mcp, secrets..."
                    className="min-w-0 flex-1 bg-transparent text-sm text-[var(--text-primary)] outline-none placeholder:text-[var(--text-placeholder)]"
                  />
                </span>
              </label>
            </div>
          </div>

          <div className="mt-6 grid gap-3 md:grid-cols-4">
            <div className="rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 md:col-span-1">
              <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Selected release</p>
              <p className="mt-2 text-lg font-semibold text-[var(--text-primary)]">{selectedVersion.label}</p>
              <p className="text-sm text-[var(--text-secondary)]">{selectedVersion.status}</p>
            </div>
            <div className="rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4 md:col-span-3">
              <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Release scope</p>
              <p className="mt-2 text-sm text-[var(--text-secondary)]">{selectedVersion.release}</p>
              <div className="mt-3 flex flex-wrap gap-2">
                {selectedVersion.highlights.map(highlight => (
                  <span key={highlight} className="rounded-full border border-[var(--border-primary)] px-3 py-1 text-xs font-medium text-[var(--text-secondary)]">{highlight}</span>
                ))}
              </div>
            </div>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-[18rem_1fr]">
          <Card className="h-fit p-3 lg:sticky lg:top-6">
            <div className="px-2 py-2">
              <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Wiki areas</p>
            </div>
            <div className="space-y-1">
              {visibleAreas.map(area => {
                const AreaIcon = area.icon;
                const active = area.id === activeArea.id;
                return (
                  <button
                    key={area.id}
                    type="button"
                    onClick={() => setActiveAreaId(area.id)}
                    className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition ${
                      active ? 'bg-[var(--bg-active)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'
                    }`}
                  >
                    <AreaIcon className="h-4 w-4 shrink-0" />
                    <span className="min-w-0 flex-1 truncate">{area.title}</span>
                  </button>
                );
              })}
            </div>
          </Card>

          <div className="space-y-6">
            <Card className="p-6">
              <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                <div>
                  <div className="flex items-center gap-3">
                    <span className="rounded-xl bg-[var(--bg-tertiary)] p-2 text-[var(--text-accent)]"><ActiveIcon className="h-5 w-5" /></span>
                    <div>
                      <h2 className="text-2xl font-semibold text-[var(--text-primary)]">{activeArea.title}</h2>
                      <p className="mt-1 text-sm text-[var(--text-secondary)]">{activeArea.summary}</p>
                    </div>
                  </div>
                </div>
                <span className="inline-flex items-center gap-2 rounded-full border border-[var(--border-primary)] px-3 py-1 text-xs font-semibold text-[var(--text-secondary)]">
                  <BookOpen className="h-3.5 w-3.5" /> {selectedVersion.label}
                </span>
              </div>

              <div className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                {activeArea.topics.map(topic => (
                  <div key={topic} className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
                    <Folder className="h-4 w-4 text-[var(--text-accent)]" />
                    <p className="mt-3 text-sm font-semibold text-[var(--text-primary)]">{topic}</p>
                  </div>
                ))}
              </div>
            </Card>

            <div className="grid gap-6 xl:grid-cols-2">
              <Card className="p-6">
                <h3 className="flex items-center gap-2 text-base font-semibold text-[var(--text-primary)]"><CheckCircle2 className="h-5 w-5 text-emerald-500" /> Implementation checklist</h3>
                <div className="mt-4 space-y-3">
                  {activeArea.checklist.map(item => (
                    <div key={item} className="flex gap-3 rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3 text-sm text-[var(--text-secondary)]">
                      <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
                      <span>{item}</span>
                    </div>
                  ))}
                </div>
              </Card>

              <Card className="p-6">
                <h3 className="flex items-center gap-2 text-base font-semibold text-[var(--text-primary)]"><FileText className="h-5 w-5 text-[var(--text-accent)]" /> Command examples</h3>
                <div className="mt-4 overflow-hidden rounded-xl border border-[var(--border-primary)] bg-[var(--bg-code)]">
                  {activeArea.commands.map(command => (
                    <div key={command} className="border-b border-[var(--border-primary)] px-4 py-3 font-mono text-xs text-[var(--text-primary)] last:border-0">
                      <span className="text-[var(--text-tertiary)]">$ </span>{command}
                    </div>
                  ))}
                </div>
              </Card>
            </div>

            <Card className="overflow-hidden">
              <div className="border-b border-[var(--border-primary)] p-6">
                <h3 className="flex items-center gap-2 text-base font-semibold text-[var(--text-primary)]"><Braces className="h-5 w-5 text-[var(--text-accent)]" /> Configuration keys</h3>
                <p className="mt-1 text-sm text-[var(--text-secondary)]">Use this table as the pattern for each versioned product doc page.</p>
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-[var(--border-primary)] text-left text-sm">
                  <thead className="bg-[var(--bg-tertiary)] text-xs uppercase tracking-wide text-[var(--text-tertiary)]">
                    <tr>
                      <th className="px-5 py-3 font-semibold">Key</th>
                      <th className="px-5 py-3 font-semibold">Purpose</th>
                      <th className="px-5 py-3 font-semibold">Example</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[var(--border-primary)]">
                    {activeArea.configuration.map(item => (
                      <tr key={item.key} className="bg-[var(--bg-secondary)]">
                        <td className="px-5 py-4 font-mono text-xs text-[var(--text-primary)]">{item.key}</td>
                        <td className="px-5 py-4 text-[var(--text-secondary)]">{item.purpose}</td>
                        <td className="px-5 py-4 font-mono text-xs text-[var(--text-secondary)]">{item.example}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>

            <div className="grid gap-4 md:grid-cols-3">
              <Card className="p-5">
                <HardDrive className="h-5 w-5 text-[var(--text-accent)]" />
                <h3 className="mt-3 font-semibold text-[var(--text-primary)]">Versioned content model</h3>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">Store docs by version, deployment target, and product area so customers can pin instructions to their installed release.</p>
              </Card>
              <Card className="p-5">
                <Server className="h-5 w-5 text-[var(--text-accent)]" />
                <h3 className="mt-3 font-semibold text-[var(--text-primary)]">Enterprise deployment coverage</h3>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">Docker, Kubernetes, cloud, and on-prem guides live together with prerequisites, validation, upgrades, and rollback notes.</p>
              </Card>
              <Card className="p-5">
                <ShieldCheck className="h-5 w-5 text-[var(--text-accent)]" />
                <h3 className="mt-3 font-semibold text-[var(--text-primary)]">Governed knowledge</h3>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">Security, access, LLM, MCP, and operations docs are written in the same product language users see across the app.</p>
              </Card>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
