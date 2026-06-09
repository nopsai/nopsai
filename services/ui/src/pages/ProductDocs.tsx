import { useMemo, useState, type ReactNode } from 'react';
import {
  Activity,
  BookOpen,
  Bot,
  Braces,
  CheckCircle2,
  Cloud,
  Code2,
  Container,
  Database,
  FileText,
  GitBranch,
  HardDrive,
  History,
  KeyRound,
  Layers3,
  LifeBuoy,
  Lock,
  Network,
  PackageCheck,
  Rocket,
  Search,
  Server,
  ShieldCheck,
  SlidersHorizontal,
  TerminalSquare,
  Workflow,
  type LucideIcon,
} from 'lucide-react';

type Version = {
  id: string;
  label: string;
  lifecycle: string;
  audience: string;
  notes: string[];
};

type Article = {
  id: string;
  title: string;
  summary: string;
  level: 'Start' | 'Operate' | 'Reference' | 'Admin' | 'Troubleshoot';
  bullets: string[];
};

type ConfigRow = {
  key: string;
  area: string;
  description: string;
  example: string;
};

type DocSection = {
  id: string;
  title: string;
  description: string;
  icon: LucideIcon;
  owner: string;
  articles: Article[];
  config: ConfigRow[];
  runbooks: string[];
};

const versions: Version[] = [
  {
    id: 'v1-current',
    label: 'v1 Current',
    lifecycle: 'Production',
    audience: 'Recommended for active installations and enterprise rollouts.',
    notes: ['Full RBAC and resource access model', 'Monitoring, dispatcher, LLM, MCP, and config repository coverage', 'Upgrade and rollback notes included per release'],
  },
  {
    id: 'v0-lts',
    label: 'v0 LTS',
    lifecycle: 'Maintenance',
    audience: 'Existing customers that need migration guidance and compatibility notes.',
    notes: ['Legacy trigger and runner behavior', 'Migration mapping to v1 resource model', 'Security fixes and compatibility documentation only'],
  },
  {
    id: 'next',
    label: 'Next',
    lifecycle: 'Preview',
    audience: 'Design partners validating upcoming AI, MCP, and enterprise controls.',
    notes: ['AI assistant workflows', 'Hosted MCP profiles', 'Policy-driven recommendations', 'Advanced usage and cost optimization'],
  },
];

const deploymentMatrix = [
  { target: 'Docker', icon: Container, bestFor: 'Local installs, demos, small private teams', includes: 'Compose stack, persistent volumes, reverse proxy, backup commands' },
  { target: 'Kubernetes', icon: GitBranch, bestFor: 'Enterprise production clusters', includes: 'Helm values, ingress, autoscaling, network policies, runner isolation' },
  { target: 'Cloud', icon: Cloud, bestFor: 'Managed platform accounts and private VPCs', includes: 'OIDC, object storage, managed databases, private endpoints, region controls' },
  { target: 'On-prem', icon: HardDrive, bestFor: 'Restricted networks and regulated data centers', includes: 'Air-gap bundles, image mirrors, offline upgrades, support bundle redaction' },
];

const sections: DocSection[] = [
  {
    id: 'getting-started',
    title: 'Getting started',
    description: 'Orientation for first-time users, administrators, and platform teams adopting Nopsai.',
    icon: Rocket,
    owner: 'Product enablement',
    articles: [
      { id: 'overview', title: 'Product overview', level: 'Start', summary: 'Core concepts: pipeline runs, pipelines, scopes, steps, triggers, schedules, knowledge context, monitoring, and system administration.', bullets: ['Map the UI to product capabilities.', 'Understand how groups and repositories organize resources.', 'Explain the difference between Git-managed resources, database resources, and local drafts.'] },
      { id: 'quickstart', title: 'Quickstart path', level: 'Start', summary: 'Create a group, connect a repository, define a scope, create a pipeline, run it from Lab, and review the run output.', bullets: ['Create an admin and baseline access roles.', 'Run a sample pipeline.', 'Inspect logs, graph, duration, and generated artifacts.'] },
      { id: 'concepts', title: 'Core concepts glossary', level: 'Reference', summary: 'Definitions and relationships for enterprise users and support teams.', bullets: ['Run group, app group, resource group, scope, context, trigger event, dispatcher, runner, MCP profile, LLM profile.', 'Common status names and failure states.', 'Ownership and inheritance model.'] },
    ],
    config: [
      { key: 'NOPS_PUBLIC_URL', area: 'Runtime', description: 'Canonical product URL used for callbacks, links, docs, and webhooks.', example: 'https://nopsai.example.com' },
      { key: 'workspace.default_group', area: 'Workspace', description: 'Default resource group for starter content and first-run examples.', example: 'platform' },
    ],
    runbooks: ['First admin creation', 'Starter workspace verification', 'Sample pipeline validation'],
  },
  {
    id: 'installation',
    title: 'Installation and deployment',
    description: 'Complete deployment documentation for Docker, Kubernetes, cloud, and on-prem environments.',
    icon: Server,
    owner: 'Platform engineering',
    articles: [
      { id: 'docker-install', title: 'Docker installation', level: 'Start', summary: 'Single-node install with compose, persistent storage, TLS, health checks, and local runner execution.', bullets: ['Prepare .env and secrets.', 'Start API, UI, database, Redis, dispatcher, and runners.', 'Validate health endpoints and first run.'] },
      { id: 'k8s-install', title: 'Kubernetes installation', level: 'Admin', summary: 'Helm-based install with namespace isolation, ingress, service accounts, secrets, autoscaling, and node placement.', bullets: ['Create namespace and secrets.', 'Configure ingress, TLS, storage classes, and network policies.', 'Split control-plane and runner workloads.'] },
      { id: 'cloud-onprem', title: 'Cloud and on-prem architectures', level: 'Admin', summary: 'Reference architectures for managed cloud, private VPC, proxy-only, and air-gapped installations.', bullets: ['Choose database and artifact storage backends.', 'Document outbound network requirements.', 'Mirror images and packages for offline operation.'] },
      { id: 'upgrades', title: 'Upgrades and rollback', level: 'Operate', summary: 'Release planning, backup, migration checks, smoke tests, rollback, and compatibility tracking.', bullets: ['Snapshot database and artifacts.', 'Run preflight and migration plan.', 'Verify core workflows after upgrade.'] },
    ],
    config: [
      { key: 'NOPS_ENV', area: 'Runtime', description: 'Runtime profile for local, staging, production, cloud, or on-prem behavior.', example: 'production' },
      { key: 'POSTGRES_DSN', area: 'Database', description: 'Primary metadata database connection string.', example: 'postgres://nopsai:***@postgres:5432/nopsai' },
      { key: 'REDIS_URL', area: 'Queue', description: 'Queue and short-lived cache endpoint.', example: 'redis://redis:6379/0' },
      { key: 'NOPS_STORAGE_BACKEND', area: 'Artifacts', description: 'Artifact storage backend for logs, bundles, and generated output.', example: 's3' },
    ],
    runbooks: ['Docker compose install', 'Helm install', 'Air-gapped package import', 'Upgrade rollback test'],
  },
  {
    id: 'configuration',
    title: 'Configuration reference',
    description: 'Versioned configuration docs for runtime, repositories, pipelines, scopes, runners, LLMs, MCP, and access.',
    icon: SlidersHorizontal,
    owner: 'Platform administrators',
    articles: [
      { id: 'runtime-config', title: 'Runtime config', level: 'Reference', summary: 'Service URLs, runner images, Docker network, timeouts, cleanup behavior, and global config repository settings.', bullets: ['Explain each runtime field.', 'Show safe defaults.', 'Document restart requirements.'] },
      { id: 'pipeline-yaml', title: 'Pipeline YAML schema', level: 'Reference', summary: 'Pipeline fields, steps, tasks, scripts, variables, secrets, context, MCP profiles, LLM profiles, retry, timeout, and failure behavior.', bullets: ['Provide minimal and advanced examples.', 'Document validation errors.', 'Explain inheritance and includes.'] },
      { id: 'scope-config', title: 'Scopes and secrets', level: 'Reference', summary: 'Runtime variables, secret placeholders, encrypted values, repository overrides, and rotation patterns.', bullets: ['Separate variables from secrets.', 'Document repo-specific overrides.', 'Describe encrypted GitOps workflows.'] },
      { id: 'config-repos', title: 'Config repositories', level: 'Admin', summary: 'GitOps source of truth for resources, drift detection, sync behavior, generated starter files, and conflict handling.', bullets: ['Configure global repository.', 'Describe path layout.', 'Show drift and reconciliation process.'] },
    ],
    config: [
      { key: 'runner.default_image', area: 'Runner', description: 'Default image used for pipeline execution when no resource-specific image is set.', example: 'ghcr.io/nopsai/runner:v1' },
      { key: 'pipeline.default_timeout', area: 'Pipelines', description: 'Default timeout for pipeline runs that do not declare a timeout.', example: '45m' },
      { key: 'gitops.config_repo.branch', area: 'GitOps', description: 'Branch used as the global configuration source.', example: 'main' },
      { key: 'secrets.provider', area: 'Secrets', description: 'Backend used to resolve runtime secrets.', example: 'vault' },
    ],
    runbooks: ['Validate config file', 'Export runtime config', 'Detect GitOps drift', 'Rotate secret provider credentials'],
  },
  {
    id: 'automation',
    title: 'Automation authoring',
    description: 'Docs for building pipelines, reusable steps, schedules, triggers, external triggers, and Lab workflows.',
    icon: Workflow,
    owner: 'Automation teams',
    articles: [
      { id: 'pipelines', title: 'Pipelines', level: 'Reference', summary: 'Create, clone, validate, access-control, and run pipeline definitions.', bullets: ['Document YAML fields.', 'Explain graph and task model.', 'Show examples for scripts and AI goals.'] },
      { id: 'steps', title: 'Reusable steps', level: 'Reference', summary: 'Design shared step libraries and understand blast radius through used-in-pipelines references.', bullets: ['Version reusable steps.', 'Manage access to shared steps.', 'Plan breaking changes.'] },
      { id: 'lab', title: 'Lab execution', level: 'Start', summary: 'Try pipeline definitions safely with target scope selection, local session edits, overrides, validation, and one-off runs.', bullets: ['Validate before run.', 'Use variable overrides.', 'Inspect generated run details.'] },
      { id: 'triggers-schedules', title: 'Triggers and schedules', level: 'Operate', summary: 'Configure Git events, external webhooks, schedules, payload mapping, branch filtering, and trigger auditing.', bullets: ['Document branch and event matching.', 'Explain webhook secret rotation.', 'Review fired counts and failures.'] },
    ],
    config: [
      { key: 'pipeline.container_image', area: 'Pipeline YAML', description: 'Container image used by a pipeline or step.', example: 'alpine:3.20' },
      { key: 'trigger.branches', area: 'Triggers', description: 'Branch filters that decide when a repository event starts pipelines.', example: '["main", "release/*"]' },
      { key: 'schedule.cron', area: 'Schedules', description: 'Cron expression for recurring pipeline execution.', example: '0 2 * * 1-5' },
    ],
    runbooks: ['Create first pipeline', 'Clone Git-managed trigger', 'Debug failed schedule', 'Safely test in Lab'],
  },
  {
    id: 'ai-mcp',
    title: 'AI, LLM, MCP, and knowledge',
    description: 'Complete guidance for AI-enabled automation, model profiles, MCP tool boundaries, and versioned knowledge context.',
    icon: Bot,
    owner: 'AI platform team',
    articles: [
      { id: 'llm-profiles', title: 'LLM profiles', level: 'Admin', summary: 'Providers, models, base URLs, API key secret names, allowed scopes, testing, reasoning behavior, and migration.', bullets: ['Document provider-specific fields.', 'Explain allowed scopes.', 'Show test and migration flows.'] },
      { id: 'mcp-profiles', title: 'MCP servers and profiles', level: 'Admin', summary: 'Servers, headers, auth secret references, tool allow-lists, timeouts, scope limits, and reusable profiles.', bullets: ['Register server safely.', 'Create least-privilege profiles.', 'Map MCP tools to pipeline tasks.'] },
      { id: 'knowledge-context', title: 'Knowledge context', level: 'Reference', summary: 'Architecture, guardrails, policies, ADRs, runbooks, references, examples, access controls, and pipeline usage.', bullets: ['Classify documents by kind.', 'Control access to sensitive runbooks.', 'Track where context is used.'] },
      { id: 'assistant', title: 'Internal assistant readiness', level: 'Admin', summary: 'Docs structure for assistant answers, version-aware knowledge, MCP usage, safe boundaries, and escalation.', bullets: ['Bind docs to product version.', 'Enforce user capabilities.', 'Log assistant actions and recommendations.'] },
    ],
    config: [
      { key: 'llm.default_profile', area: 'LLM', description: 'Default profile for AI tasks that do not explicitly select one.', example: 'reasoning' },
      { key: 'llm.profiles[].allowed_scopes', area: 'LLM', description: 'Scopes where the profile may be used.', example: '["dev", "internal"]' },
      { key: 'mcp.servers[].timeout', area: 'MCP', description: 'Timeout for MCP server calls.', example: '30s' },
      { key: 'mcp.profiles[].allowed_tools', area: 'MCP', description: 'Tool allow-list exposed to a profile.', example: '["repos_get", "issues_list"]' },
    ],
    runbooks: ['Add LLM profile', 'Test MCP server', 'Restrict MCP tools', 'Publish knowledge context for a release'],
  },
  {
    id: 'security',
    title: 'Security, access, and compliance',
    description: 'RBAC, resource access, service accounts, tokens, audit logs, secret handling, and regulated deployment guidance.',
    icon: ShieldCheck,
    owner: 'Security administrators',
    articles: [
      { id: 'rbac', title: 'RBAC and resource access', level: 'Admin', summary: 'Users, roles, policies, folders, inheritance, resource visibility, and advanced access rules.', bullets: ['Explain viewer, developer, owner, and admin roles.', 'Document inheritance behavior.', 'Show restricted resource sharing.'] },
      { id: 'tokens', title: 'Tokens and service accounts', level: 'Admin', summary: 'Personal access tokens, service accounts, rotation, expiration, and automation-safe permissions.', bullets: ['Create least-privilege service accounts.', 'Rotate tokens.', 'Audit token usage.'] },
      { id: 'secrets', title: 'Secrets and sensitive data', level: 'Reference', summary: 'Secret providers, encrypted config values, masking, redaction, support bundles, and log safety.', bullets: ['Mask logs and prompts.', 'Redact support bundles.', 'Separate operational secrets by scope.'] },
      { id: 'compliance', title: 'Compliance checklist', level: 'Admin', summary: 'Controls for data residency, retention, auditability, change management, backup testing, and break-glass access.', bullets: ['Document control owners.', 'Review access regularly.', 'Validate restore and incident response.'] },
    ],
    config: [
      { key: 'access.default_visibility', area: 'Access', description: 'Default visibility for new resources.', example: 'group' },
      { key: 'audit.enabled', area: 'Audit', description: 'Enables security and configuration audit events.', example: 'true' },
      { key: 'audit.redaction', area: 'Audit', description: 'Redaction policy for logs, prompts, and support bundles.', example: 'strict' },
      { key: 'token.max_age_days', area: 'Tokens', description: 'Maximum lifetime for personal or service tokens.', example: '90' },
    ],
    runbooks: ['Quarterly access review', 'Emergency access process', 'Token rotation', 'Support bundle redaction review'],
  },
  {
    id: 'operations',
    title: 'Operations and monitoring',
    description: 'Monitoring, backups, restore, incident response, capacity planning, cost control, and enterprise efficiency documentation.',
    icon: Activity,
    owner: 'SRE and operations',
    articles: [
      { id: 'monitoring', title: 'Monitoring dashboard', level: 'Operate', summary: 'Run counts, status trends, duration averages, longest runs, runners, services, group activity, pipeline metrics, and usage signals.', bullets: ['Interpret run duration and failure rate.', 'Review runner health and capacity.', 'Identify expensive or slow pipelines.'] },
      { id: 'backup-restore', title: 'Backup and restore', level: 'Operate', summary: 'Database, artifacts, config repo, secrets metadata, restore drills, RPO, RTO, and validation.', bullets: ['Create repeatable backups.', 'Test restore per release.', 'Document ownership and retention.'] },
      { id: 'incident-response', title: 'Incident response', level: 'Troubleshoot', summary: 'Triage failed runs, stuck runners, trigger storms, broken config sync, LLM errors, MCP failures, and degraded services.', bullets: ['Use run timeline and logs.', 'Compare recent config changes.', 'Escalate with redacted support bundle.'] },
      { id: 'cost-efficiency', title: 'Cost and efficiency', level: 'Operate', summary: 'Track token usage, runner utilization, queue time, retry volume, slow pipelines, and suggested optimization paths.', bullets: ['Set monthly token budgets.', 'Review longest runs.', 'Optimize duplicate or idle automation.'] },
    ],
    config: [
      { key: 'monitoring.retention_days', area: 'Monitoring', description: 'How long run metrics and service history remain queryable.', example: '180' },
      { key: 'backup.schedule', area: 'Backup', description: 'Backup cadence for metadata and artifacts.', example: '0 1 * * *' },
      { key: 'cost.token_budget_monthly', area: 'Cost', description: 'Soft budget for AI-assisted operations.', example: '25000000' },
      { key: 'runner.max_concurrency', area: 'Runners', description: 'Maximum concurrent jobs per runner pool.', example: '12' },
    ],
    runbooks: ['Failed pipeline investigation', 'Runner capacity incident', 'Trigger storm response', 'Monthly efficiency review'],
  },
  {
    id: 'api-reference',
    title: 'API, integration, and support reference',
    description: 'Developer and support docs for REST APIs, webhooks, CLI commands, events, integrations, and diagnostics.',
    icon: Code2,
    owner: 'Developer experience',
    articles: [
      { id: 'rest-api', title: 'REST API reference', level: 'Reference', summary: 'Document auth, pagination, errors, resource endpoints, run endpoints, monitoring endpoints, and admin APIs.', bullets: ['Use examples for curl and SDK clients.', 'Document error codes.', 'Describe rate limits.'] },
      { id: 'webhooks', title: 'Webhook and event contracts', level: 'Reference', summary: 'Incoming Git and external events, payload schemas, signature verification, retry behavior, and audit metadata.', bullets: ['Document payload examples.', 'Verify signatures.', 'Replay and debug failed events.'] },
      { id: 'cli', title: 'CLI reference', level: 'Reference', summary: 'Setup, validation, export, backup, support, and diagnostic commands.', bullets: ['Group commands by use case.', 'Provide examples.', 'Show required permissions.'] },
      { id: 'support', title: 'Support diagnostics', level: 'Troubleshoot', summary: 'Health checks, support bundle collection, redaction, log locations, version info, and safe escalation data.', bullets: ['Collect minimal safe evidence.', 'Redact secrets.', 'Attach version and deployment profile.'] },
    ],
    config: [
      { key: 'api.rate_limit.requests_per_minute', area: 'API', description: 'Per-user API rate limit.', example: '600' },
      { key: 'webhooks.signature_header', area: 'Webhooks', description: 'Header used to verify incoming webhook signatures.', example: 'X-Nopsai-Signature' },
      { key: 'support.bundle.max_log_bytes', area: 'Support', description: 'Maximum log bytes included in generated support bundles.', example: '52428800' },
    ],
    runbooks: ['Generate support bundle', 'Replay webhook safely', 'Validate API token', 'Document integration onboarding'],
  },
];

function matchesQuery(section: DocSection, query: string) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  const haystack = [
    section.title,
    section.description,
    section.owner,
    ...section.articles.flatMap(article => [article.title, article.summary, article.level, ...article.bullets]),
    ...section.config.flatMap(row => [row.key, row.area, row.description, row.example]),
    ...section.runbooks,
  ]
    .join(' ')
    .toLowerCase();
  return haystack.includes(normalized);
}

function Card({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <section className={`rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-secondary)] shadow-sm ${className}`}>{children}</section>;
}

function LevelPill({ level }: { level: Article['level'] }) {
  return <span className="rounded-full border border-[var(--border-primary)] px-2.5 py-1 text-[11px] font-semibold text-[var(--text-secondary)]">{level}</span>;
}

export default function ProductDocsPage() {
  const [versionID, setVersionID] = useState(versions[0].id);
  const [activeID, setActiveID] = useState(sections[0].id);
  const [query, setQuery] = useState('');
  const version = versions.find(item => item.id === versionID) || versions[0];
  const visibleSections = useMemo(() => sections.filter(section => matchesQuery(section, query)), [query]);
  const active = visibleSections.find(section => section.id === activeID) || visibleSections[0] || sections[0];
  const ActiveIcon = active.icon;

  return (
    <div className="min-h-full bg-[var(--bg-primary)]">
      <div className="mx-auto w-full max-w-[1600px] px-4 py-5 sm:px-6 lg:px-8 space-y-6">
        <Card className="overflow-hidden">
          <div className="grid gap-0 xl:grid-cols-[1.35fr_0.65fr]">
            <div className="p-6 lg:p-8">
              <div className="inline-flex items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-tertiary)] px-3 py-1.5 text-xs font-semibold text-[var(--text-secondary)]">
                <BookOpen className="h-3.5 w-3.5 text-[var(--text-accent)]" />
                Versioned product wiki
              </div>
              <h1 className="mt-4 text-3xl font-semibold text-[var(--text-primary)]">Nopsai documentation</h1>
              <p className="mt-3 max-w-4xl text-sm leading-6 text-[var(--text-secondary)]">
                A complete product wiki for every released version: installation, architecture, configuration, pipelines, AI, MCP, security, operations, integrations, troubleshooting, and support. The page is structured for enterprise usage where every article belongs to a version, owner, lifecycle, and runbook.
              </p>
              <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                {[
                  ['Sections', String(sections.length), Layers3],
                  ['Articles', String(sections.reduce((sum, section) => sum + section.articles.length, 0)), FileText],
                  ['Config keys', String(sections.reduce((sum, section) => sum + section.config.length, 0)), Braces],
                  ['Runbooks', String(sections.reduce((sum, section) => sum + section.runbooks.length, 0)), LifeBuoy],
                ].map(([label, value, Icon]) => (
                  <div key={label as string} className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
                    <Icon className="h-4 w-4 text-[var(--text-accent)]" />
                    <p className="mt-3 text-2xl font-semibold text-[var(--text-primary)]">{value as string}</p>
                    <p className="text-xs font-medium text-[var(--text-secondary)]">{label as string}</p>
                  </div>
                ))}
              </div>
            </div>
            <div className="border-t border-[var(--border-primary)] bg-[var(--bg-tertiary)]/40 p-6 xl:border-l xl:border-t-0 lg:p-8">
              <label className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">
                Product version
                <select
                  value={versionID}
                  onChange={event => setVersionID(event.target.value)}
                  className="mt-2 h-11 w-full rounded-xl border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 text-sm normal-case tracking-normal text-[var(--text-primary)] focus:border-[var(--border-input-focus)] focus:outline-none focus:ring-2 focus:ring-[var(--border-accent-focus-ring)]"
                >
                  {versions.map(item => <option key={item.id} value={item.id}>{item.label}</option>)}
                </select>
              </label>
              <div className="mt-5 rounded-2xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-lg font-semibold text-[var(--text-primary)]">{version.label}</p>
                  <span className="rounded-full bg-[var(--bg-active)] px-3 py-1 text-xs font-semibold text-[var(--text-primary)]">{version.lifecycle}</span>
                </div>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">{version.audience}</p>
                <ul className="mt-4 space-y-2 text-sm text-[var(--text-secondary)]">
                  {version.notes.map(note => <li key={note} className="flex gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />{note}</li>)}
                </ul>
              </div>
            </div>
          </div>
        </Card>

        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          {deploymentMatrix.map(item => {
            const Icon = item.icon;
            return (
              <Card key={item.target} className="p-5">
                <Icon className="h-5 w-5 text-[var(--text-accent)]" />
                <h2 className="mt-3 text-base font-semibold text-[var(--text-primary)]">{item.target}</h2>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">{item.bestFor}</p>
                <p className="mt-3 text-xs leading-5 text-[var(--text-tertiary)]">{item.includes}</p>
              </Card>
            );
          })}
        </div>

        <div className="grid gap-6 lg:grid-cols-[20rem_1fr]">
          <Card className="h-fit p-3 lg:sticky lg:top-6">
            <div className="px-2 py-2">
              <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Search wiki</p>
              <span className="mt-2 flex h-10 items-center gap-2 rounded-xl border border-[var(--border-input)] bg-[var(--bg-primary)] px-3 focus-within:border-[var(--border-input-focus)] focus-within:ring-2 focus-within:ring-[var(--border-accent-focus-ring)]">
                <Search className="h-4 w-4 text-[var(--text-tertiary)]" />
                <input value={query} onChange={event => setQuery(event.target.value)} placeholder="runner, docker, MCP..." className="min-w-0 flex-1 bg-transparent text-sm outline-none placeholder:text-[var(--text-placeholder)]" />
              </span>
            </div>
            <div className="mt-3 space-y-1">
              {visibleSections.map(section => {
                const Icon = section.icon;
                const selected = section.id === active.id;
                return (
                  <button
                    key={section.id}
                    type="button"
                    onClick={() => setActiveID(section.id)}
                    className={`flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm transition ${selected ? 'bg-[var(--bg-active)] text-[var(--text-primary)]' : 'text-[var(--text-secondary)] hover:bg-[var(--bg-tertiary)] hover:text-[var(--text-primary)]'}`}
                  >
                    <Icon className="h-4 w-4 shrink-0" />
                    <span className="min-w-0 flex-1 truncate">{section.title}</span>
                  </button>
                );
              })}
              {visibleSections.length === 0 ? <p className="px-3 py-4 text-sm text-[var(--text-secondary)]">No matching documentation section.</p> : null}
            </div>
          </Card>

          <div className="space-y-6">
            <Card className="p-6">
              <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                <div className="flex gap-4">
                  <span className="rounded-2xl bg-[var(--bg-tertiary)] p-3 text-[var(--text-accent)]"><ActiveIcon className="h-6 w-6" /></span>
                  <div>
                    <p className="text-xs font-semibold uppercase tracking-wide text-[var(--text-tertiary)]">Owner: {active.owner}</p>
                    <h2 className="mt-1 text-2xl font-semibold text-[var(--text-primary)]">{active.title}</h2>
                    <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--text-secondary)]">{active.description}</p>
                  </div>
                </div>
                <span className="inline-flex items-center gap-2 rounded-full border border-[var(--border-primary)] px-3 py-1 text-xs font-semibold text-[var(--text-secondary)]">
                  <History className="h-3.5 w-3.5" /> {version.label}
                </span>
              </div>
            </Card>

            <div className="grid gap-4 xl:grid-cols-2">
              {active.articles.map(article => (
                <Card key={article.id} className="p-5">
                  <div className="flex items-start justify-between gap-3">
                    <div>
                      <h3 className="text-base font-semibold text-[var(--text-primary)]">{article.title}</h3>
                      <p className="mt-2 text-sm leading-6 text-[var(--text-secondary)]">{article.summary}</p>
                    </div>
                    <LevelPill level={article.level} />
                  </div>
                  <ul className="mt-4 space-y-2 text-sm text-[var(--text-secondary)]">
                    {article.bullets.map(bullet => <li key={bullet} className="flex gap-2"><CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />{bullet}</li>)}
                  </ul>
                </Card>
              ))}
            </div>

            <Card className="overflow-hidden">
              <div className="border-b border-[var(--border-primary)] p-6">
                <h3 className="flex items-center gap-2 text-base font-semibold text-[var(--text-primary)]"><Braces className="h-5 w-5 text-[var(--text-accent)]" /> Configuration reference</h3>
                <p className="mt-1 text-sm text-[var(--text-secondary)]">Every section can expose version-specific config keys, examples, restart requirements, and security notes.</p>
              </div>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-[var(--border-primary)] text-left text-sm">
                  <thead className="bg-[var(--bg-tertiary)] text-xs uppercase tracking-wide text-[var(--text-tertiary)]">
                    <tr><th className="px-5 py-3 font-semibold">Key</th><th className="px-5 py-3 font-semibold">Area</th><th className="px-5 py-3 font-semibold">Description</th><th className="px-5 py-3 font-semibold">Example</th></tr>
                  </thead>
                  <tbody className="divide-y divide-[var(--border-primary)]">
                    {active.config.map(row => (
                      <tr key={row.key}>
                        <td className="px-5 py-4 font-mono text-xs text-[var(--text-primary)]">{row.key}</td>
                        <td className="px-5 py-4 text-[var(--text-secondary)]">{row.area}</td>
                        <td className="px-5 py-4 text-[var(--text-secondary)]">{row.description}</td>
                        <td className="px-5 py-4 font-mono text-xs text-[var(--text-secondary)]">{row.example}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              {active.runbooks.map(runbook => (
                <Card key={runbook} className="p-4">
                  <TerminalSquare className="h-5 w-5 text-[var(--text-accent)]" />
                  <p className="mt-3 text-sm font-semibold text-[var(--text-primary)]">{runbook}</p>
                  <p className="mt-2 text-xs leading-5 text-[var(--text-secondary)]">Runbook placeholder for prerequisites, steps, validation, rollback, audit notes, and expected evidence.</p>
                </Card>
              ))}
            </div>

            <Card className="p-6">
              <h3 className="flex items-center gap-2 text-base font-semibold text-[var(--text-primary)]"><PackageCheck className="h-5 w-5 text-[var(--text-accent)]" /> Documentation governance model</h3>
              <div className="mt-4 grid gap-4 md:grid-cols-3">
                <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4"><Database className="h-5 w-5 text-[var(--text-accent)]" /><p className="mt-3 font-semibold">Versioned sources</p><p className="mt-2 text-sm text-[var(--text-secondary)]">Docs should be sourced from version folders, generated schema references, and release-owned runbooks.</p></div>
                <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4"><KeyRound className="h-5 w-5 text-[var(--text-accent)]" /><p className="mt-3 font-semibold">Access-aware content</p><p className="mt-2 text-sm text-[var(--text-secondary)]">Sensitive admin, secret, and support content can be capability-filtered later if required.</p></div>
                <div className="rounded-xl border border-[var(--border-primary)] bg-[var(--bg-primary)] p-4"><Network className="h-5 w-5 text-[var(--text-accent)]" /><p className="mt-3 font-semibold">Assistant-ready</p><p className="mt-2 text-sm text-[var(--text-secondary)]">The same structure can feed the internal assistant with version, article, owner, and runbook metadata.</p></div>
              </div>
            </Card>
          </div>
        </div>
      </div>
    </div>
  );
}
