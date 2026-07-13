import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { BookOpen, CircleHelp, ExternalLink, X } from 'lucide-react';
import { Link, useLocation } from 'react-router-dom';
import { buildDocumentationHref, resolveHelpTopicKey } from './appHelpModel';
import { useDialogFocus } from './useDialogFocus';

type HelpSection = {
  title: string;
  body: string;
  items?: string[];
  example?: string;
};

type HelpTopic = {
  title: string;
  summary: string;
  docsPath: string;
  sections: HelpSection[];
};

const HELP_TOPICS: Record<string, HelpTopic> = {
  'pipelineruns/main': {
    title: 'Pipeline Runs',
    summary: 'Watch automation work move through team and application runs, source filters, failures, and approvals.',
    docsPath: 'pipeline-runs',
    sections: [
      {
        title: 'What this page covers',
        body: 'Pipeline Runs is the operational entry point for run history, status, graph inspection, logs, child runs, source metadata, trigger events, and troubleshooting.',
        items: ['Browse by team, application, source, and status.', 'Open run details for graph, YAML, tasks, logs, and timing.', 'Search by pipeline, repository, branch, commit, actor, status, or run ID.'],
      },
    ],
  },
  'pipelineruns/recent': {
    title: 'All Runs',
    summary: 'Review the newest executions across the workspace without choosing a team first.',
    docsPath: 'pipeline-runs/recent',
    sections: [
      {
        title: 'What this page covers',
        body: 'All Runs is useful for status scanning, broad troubleshooting, and comparing pipeline activity across teams.',
        items: ['Switch between grid and list views.', 'Filter by source and status.', 'Open any run for full operational detail.'],
      },
    ],
  },
  'pipelineruns/events': {
    title: 'Trigger Events',
    summary: 'Review the Git or external events that started one or more pipeline runs.',
    docsPath: 'pipeline-runs/events',
    sections: [
      {
        title: 'What this page covers',
        body: 'Events correlate related executions by repository event, commit, branch, and pusher so teams can investigate one change across all affected pipelines.',
      },
    ],
  },
  monitoring: {
    title: 'Monitoring',
    summary: 'Monitor run efficiency, service health, runner capacity, trends, and product usage.',
    docsPath: 'monitoring',
    sections: [
      {
        title: 'What this page covers',
        body: 'Monitoring helps administrators understand throughput, failures, durations, runner health, trigger activity, and enterprise efficiency indicators.',
      },
    ],
  },
  pipelines: {
    title: 'Pipelines',
    summary: 'Define reusable automation flows made of steps, tasks, scripts, LLM goals, and context.',
    docsPath: 'pipelines',
    sections: [
      {
        title: 'What this page covers',
        body: 'Pipelines can be browsed, edited, cloned, validated, downloaded, access-controlled, and linked to recent runs or triggers.',
        example: 'name: build-and-test\ndescription: Build and test service changes\nsteps:\n  - name: test\n    script: npm test',
      },
    ],
  },
  schedules: {
    title: 'Schedules',
    summary: 'Run pipelines on recurring time-based rules.',
    docsPath: 'schedules',
    sections: [
      {
        title: 'What this page covers',
        body: 'Schedules define cadence, scope, pipeline target, status, ownership, and operational review points for recurring automation.',
      },
    ],
  },
  triggers: {
    title: 'Triggers',
    summary: 'Connect repository events to one or more pipeline definitions.',
    docsPath: 'triggers',
    sections: [
      {
        title: 'What this page covers',
        body: 'Triggers document repository matching, branch rules, event types, linked pipelines, Git-managed definitions, and database overrides.',
        example: 'triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - pipelines/service-api.yaml',
      },
    ],
  },
  'external-triggers': {
    title: 'External Triggers',
    summary: 'Expose controlled webhook endpoints for non-Git systems.',
    docsPath: 'external-triggers',
    sections: [
      {
        title: 'What this page covers',
        body: 'External triggers cover endpoint URLs, authentication, payload mapping, rate limits, pipeline selection, and auditability.',
      },
    ],
  },
  'git-webhook-sources': {
    title: 'Git Webhook Sources',
    summary: 'Connect non-GitHub Git providers to repository-driven pipeline triggers.',
    docsPath: 'git-webhook-sources',
    sections: [
      {
        title: 'What this page covers',
        body: 'Git webhook sources define provider normalization, team ownership, authentication, repository allowlists, delivery limits, GitOps ownership, and delivery audit history.',
        example: 'team_path: platform\nprovider: gitlab\nauth_mode: static_token\ncredential_ref: credential://system/webhooks/gitlab\nrepository_allowlist:\n  - platform/*',
      },
    ],
  },
  scopes: {
    title: 'Scopes',
    summary: 'Manage runtime variables, secrets, and repository-specific overrides.',
    docsPath: 'scopes',
    sections: [
      {
        title: 'What this page covers',
        body: 'Scopes provide runtime configuration boundaries for pipelines, including plain variables, sensitive secrets, and repository-prefixed overrides.',
      },
    ],
  },
  lab: {
    title: 'Lab',
    summary: 'Run or adjust a pipeline definition safely before committing it elsewhere.',
    docsPath: 'lab',
    sections: [
      {
        title: 'What this page covers',
        body: 'Lab supports pipeline selection, target scope selection, session-local YAML edits, validation, overrides, and one-off runs.',
      },
    ],
  },
  steps: {
    title: 'Steps',
    summary: 'Create reusable building blocks for pipelines.',
    docsPath: 'steps',
    sections: [
      {
        title: 'What this page covers',
        body: 'Steps can include scripts, LLM goals, tasks, variables, secrets, MCP profiles, failure behavior, and access controls.',
      },
    ],
  },
  'knowledge-context': {
    title: 'Knowledge Context',
    summary: 'Store curated markdown used by AI-enabled pipeline steps.',
    docsPath: 'knowledge-context',
    sections: [
      {
        title: 'What this page covers',
        body: 'Knowledge context covers architecture, guardrails, policies, ADRs, guidelines, runbooks, references, and examples.',
      },
    ],
  },
  'system/config': {
    title: 'System Config',
    summary: 'Control runtime defaults, service discovery, and global GitOps repository settings.',
    docsPath: 'system/config',
    sections: [
      {
        title: 'What this page covers',
        body: 'System Config covers runner images, Docker network, service URLs, cleanup, timeouts, and config repository synchronization.',
      },
    ],
  },
  'system/setup': {
    title: 'First-Install Setup',
    summary: 'Guide initial control-plane setup and starter GitOps output.',
    docsPath: 'system/setup',
    sections: [
      {
        title: 'What this page covers',
        body: 'Setup covers preflight checks, runtime settings, repository teams, LLM defaults, users, and generated installation output.',
      },
    ],
  },
  'llm-profiles': {
    title: 'LLM Profiles',
    summary: 'Define model providers, models, keys, and scope limits for AI steps.',
    docsPath: 'llm-profiles',
    sections: [
      {
        title: 'What this page covers',
        body: 'LLM profiles cover provider selection, model configuration, base URLs, API key secret names, reasoning behavior, testing, and migrations.',
      },
    ],
  },
  'agent-profiles': {
    title: 'Agent Profiles',
    summary: 'Define reusable AI roles, instructions, sources, defaults, and usage checks.',
    docsPath: 'agent-profiles',
    sections: [
      {
        title: 'What this page covers',
        body: 'Agent profiles cover reusable agent instructions, defaults, built-in profiles, GitOps-managed profiles, usage, and source inspection.',
      },
    ],
  },
  mcp: {
    title: 'MCP',
    summary: 'Register external tool servers and assemble reusable MCP profiles.',
    docsPath: 'mcp',
    sections: [
      {
        title: 'What this page covers',
        body: 'MCP covers server URLs, headers, auth secret references, timeouts, allowed scopes, tool allow-lists, and profile composition.',
      },
    ],
  },
  'system/data-management': {
    title: 'Data Management',
    summary: 'Manage retention, backups, data cleanup, imports, exports, and restore workflows.',
    docsPath: 'system/data-management',
    sections: [
      {
        title: 'What this page covers',
        body: 'Data Management covers operational data retention, safe cleanup, backup and restore validation, and migration support.',
      },
    ],
  },
  'system/dispatcher': {
    title: 'Dispatcher',
    summary: 'Monitor runners, capacity, routing, and deployment instructions.',
    docsPath: 'system/dispatcher',
    sections: [
      {
        title: 'What this page covers',
        body: 'Dispatcher covers runner heartbeats, capacity, active jobs, scope routing, deployment templates, and scheduling behavior.',
      },
    ],
  },
  'system/access': {
    title: 'Access',
    summary: 'Manage users, roles, resource policies, and inheritance.',
    docsPath: 'system/access',
    sections: [
      {
        title: 'What this page covers',
        body: 'Access covers users, basic roles, advanced policy rules, resource grants, service accounts, and inheritance across teams.',
      },
    ],
  },
  profile: {
    title: 'Profile',
    summary: 'Manage account details, password changes, and personal access tokens.',
    docsPath: 'profile',
    sections: [
      {
        title: 'What this page covers',
        body: 'Profile covers account identity, email update, password change, token creation, token revocation, and permitted system links.',
      },
    ],
  },
  docs: {
    title: 'Product Documentation',
    summary: 'Browse the full versioned product wiki.',
    docsPath: 'overview',
    sections: [
      {
        title: 'What this page covers',
        body: 'Docs contains product-wide installation, deployment, configuration, operations, security, reference, and troubleshooting guidance for each version.',
      },
    ],
  },
  default: {
    title: 'Help',
    summary: 'This panel explains the current page, its major controls, and common workflows.',
    docsPath: 'overview',
    sections: [
      {
        title: 'Using Help',
        body: 'Open the help sign from any authenticated page to get page-specific guidance. Use the Docs link beside it for the complete product wiki.',
      },
    ],
  },
};

function resolveHelpTopic(pathname: string): HelpTopic {
  const topicKey = resolveHelpTopicKey(pathname);
  if (topicKey.startsWith('pipelineruns/')) return HELP_TOPICS[topicKey] || HELP_TOPICS['pipelineruns/main'];
  if (topicKey.startsWith('system/')) return HELP_TOPICS[topicKey] || HELP_TOPICS['system/config'];
  return HELP_TOPICS[topicKey] || HELP_TOPICS.default;
}

export default function AppHelp() {
  const location = useLocation();
  const topic = useMemo(() => resolveHelpTopic(location.pathname), [location.pathname]);
  const docsHref = useMemo(() => buildDocumentationHref(topic.docsPath), [topic.docsPath]);
  const [openState, setOpenState] = useState({ pathname: location.pathname, open: false });
  const open = openState.pathname === location.pathname && openState.open;
  const rootRef = useRef<HTMLDivElement | null>(null);
  const panelID = useId();
  const titleID = useId();
  const summaryID = useId();
  const closeHelp = useCallback(() => {
    setOpenState({ pathname: location.pathname, open: false });
  }, [location.pathname]);
  const dialogRef = useDialogFocus(closeHelp, open);

  useEffect(() => {
    if (!open) return;
    const handlePointerDown = (event: MouseEvent) => {
      if (event.target instanceof Node && !rootRef.current?.contains(event.target)) closeHelp();
    };
    document.addEventListener('mousedown', handlePointerDown);
    return () => document.removeEventListener('mousedown', handlePointerDown);
  }, [closeHelp, open]);

  return (
    <div className="flex items-center gap-2" ref={rootRef}>
      <Link
        to="/docs"
        className="hidden h-10 items-center gap-2 rounded-full border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 text-sm font-semibold text-[var(--text-primary)] shadow-sm transition hover:border-[var(--border-accent)] hover:bg-[var(--bg-tertiary)] sm:inline-flex"
        aria-label="Open product documentation"
        title="Product documentation"
      >
        <BookOpen className="h-4 w-4" aria-hidden="true" />
        <span>Docs</span>
      </Link>
      <div className="app-help">
        <button
          type="button"
          className={`app-help__trigger ${open ? 'app-help__trigger--open' : ''}`}
          onClick={() => setOpenState(value => ({ pathname: location.pathname, open: value.pathname === location.pathname ? !value.open : true }))}
          aria-label={`Help for ${topic.title}`}
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-controls={open ? panelID : undefined}
          title="Help"
        >
          <CircleHelp className="h-5 w-5" aria-hidden="true" />
        </button>
        {open && (
          <div
            id={panelID}
            ref={dialogRef}
            className="app-help__panel"
            role="dialog"
            aria-labelledby={titleID}
            aria-describedby={summaryID}
            tabIndex={-1}
          >
            <div className="app-help__header">
              <div className="app-help__header-copy">
                <span className="app-help__eyebrow">Help</span>
                <h2 id={titleID}>{topic.title}</h2>
                <p id={summaryID}>{topic.summary}</p>
              </div>
              <button type="button" className="app-help__close" onClick={closeHelp} aria-label="Close help" data-dialog-initial-focus>
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>

            <div className="app-help__body">
              {topic.sections.map(section => (
                <section className="app-help__section" key={section.title}>
                  <h3>{section.title}</h3>
                  <p>{section.body}</p>
                  {section.items?.length ? (
                    <ul>
                      {section.items.map(item => (
                        <li key={item}>{item}</li>
                      ))}
                    </ul>
                  ) : null}
                  {section.example ? (
                    <pre>
                      <code>{section.example}</code>
                    </pre>
                  ) : null}
                </section>
              ))}
            </div>

            <div className="app-help__footer">
              {docsHref ? (
                <a className="app-help__docs-link" href={docsHref} target="_blank" rel="noreferrer">
                  <BookOpen className="h-4 w-4" aria-hidden="true" />
                  <span>Open external documentation</span>
                  <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
                </a>
              ) : (
                <Link className="app-help__docs-link" to="/docs" onClick={closeHelp}>
                  <BookOpen className="h-4 w-4" aria-hidden="true" />
                  <span>Open product documentation</span>
                </Link>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
