import { useEffect, useMemo, useRef, useState } from 'react';
import { BookOpen, CircleHelp, ExternalLink, X } from 'lucide-react';
import { useLocation } from 'react-router-dom';

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

// Update this when the product documentation route or external docs site exists.
const DOCUMENTATION_BASE_URL = '';

const HELP_TOPICS: Record<string, HelpTopic> = {
  'pipelineruns/main': {
    title: 'Pipeline Runs',
    summary: 'Use this page to watch automation work move through groups, inspect failures, and clean up completed runs.',
    docsPath: 'pipeline-runs',
    sections: [
      {
        title: 'Options and Features',
        body: 'The Main tab is organized by pipeline-run groups. Select a folder in the sidebar, search runs by pipeline, repository, branch, commit, actor, or status, then open a run to see its graph, YAML, tasks, logs, and child runs.',
        items: [
          'New Group creates a folder for organizing runs and repository ownership.',
          'Bulk selection appears when one or more runs are selected and supports clearing or deleting selected runs.',
          'Run details show timing, source repository, trigger event, step status, task logs, and pipeline definition.',
        ],
        example: 'Example workflow: open Main, select team-1/service-api, search "deploy main", open a failed run, then expand the failed step logs.',
      },
      {
        title: 'When to Use It',
        body: 'Start here when an automation did not behave as expected or when you need the operational history for a repository group.',
      },
    ],
  },
  'pipelineruns/recent': {
    title: 'Recent Runs',
    summary: 'Recent shows the newest pipeline executions across the workspace without requiring you to pick a folder first.',
    docsPath: 'pipeline-runs/recent',
    sections: [
      {
        title: 'Options and Features',
        body: 'Switch between grid and list layout, search across recent runs, and open any run for the same graph, logs, and YAML details available from the Main tab.',
        items: [
          'Grid view is useful for quick status scanning.',
          'List view is denser when you are comparing branch, repository, commit, and trigger metadata.',
          'Search narrows the run stream by text such as status, pipeline name, repository, branch, or run ID.',
        ],
        example: 'Example search: "failure service-api main" to find recent failed runs for the service-api repository on main.',
      },
    ],
  },
  'pipelineruns/events': {
    title: 'Trigger Events',
    summary: 'Events groups runs by the Git or trigger event that started them, so related pipeline executions can be reviewed together.',
    docsPath: 'pipeline-runs/events',
    sections: [
      {
        title: 'Options and Features',
        body: 'Use this tab when one push or pull request started multiple pipelines. Event cards keep repository, branch, commit, pusher, and latest run status together.',
        items: [
          'Open a run from an event to inspect the detailed graph and logs.',
          'Search helps narrow noisy event streams by repository, branch, actor, commit, or pipeline name.',
          'Status is summarized from the runs attached to the same event.',
        ],
        example: 'Example workflow: open Events after a GitHub push, find the commit SHA, then compare each pipeline run created by that push.',
      },
    ],
  },
  pipelines: {
    title: 'Pipelines',
    summary: 'Pipelines define reusable automation flows made of steps, tasks, scripts, LLM goals, and knowledge context.',
    docsPath: 'pipelines',
    sections: [
      {
        title: 'Options and Features',
        body: 'Browse pipeline folders, search definitions, create database-backed drafts, clone Git-managed definitions for customization, copy or download YAML, and open related triggers and recent runs.',
        items: [
          'Source labels show whether a pipeline came from Git, the database, or a local draft.',
          'Access controls decide who can use a pipeline and whether it is group-only, restricted, or public.',
          'Validation highlights YAML issues and provides examples when the editor can infer a fix.',
        ],
        example: 'name: build-and-test\ndescription: Build and test service changes\nsteps:\n  - name: test\n    script: npm test',
      },
      {
        title: 'When to Use It',
        body: 'Use this page to create or review the automation contract before running it from Lab, triggers, or Git events.',
      },
    ],
  },
  triggers: {
    title: 'Triggers',
    summary: 'Triggers connect Git repository events to one or more pipeline definitions.',
    docsPath: 'triggers',
    sections: [
      {
        title: 'Options and Features',
        body: 'Browse repository trigger overrides, search by owner/repo, create or replace database overrides, clone Git-managed triggers, inspect linked pipelines, and review recent runs started by a repository.',
        items: [
          'Repository uses the owner/repo format, for example acme/service-api.',
          'The YAML preview creates a push trigger for main and points at a pipeline file path.',
          'Git-managed triggers are read-only in the UI; clone them to make an editable database override.',
        ],
        example: 'triggers:\n  - on: push\n    branches:\n      - main\n    pipelines:\n      - pipelines/service-api.yaml',
      },
    ],
  },
  scopes: {
    title: 'Scopes',
    summary: 'Scopes hold runtime variables and secrets for pipelines, with optional repository-specific overrides.',
    docsPath: 'scopes',
    sections: [
      {
        title: 'Options and Features',
        body: 'Create scopes, search by scope path or key, edit variables and secret keys, clone values between scopes, and encrypt secrets for GitOps-managed scope files.',
        items: [
          'Variables are plain text values injected into matching pipeline runs.',
          'Secrets are sensitive values; GitOps files can store empty placeholders or encrypted values generated by this instance.',
          'Repository-prefixed keys such as owner/repo/NAME override a value for one repository.',
        ],
        example: 'variables:\n  DEPLOY_TARGET: "development"\n  acme/service-api/IMAGE_TAG: "dev"\nsecrets:\n  GEMINI_API_KEY:\n  acme/service-api/DEPLOY_TOKEN:',
      },
    ],
  },
  lab: {
    title: 'Lab',
    summary: 'Lab is a safe place to run or adjust a pipeline definition against a chosen scope before committing it elsewhere.',
    docsPath: 'lab',
    sections: [
      {
        title: 'Options and Features',
        body: 'Pick a pipeline, choose a target scope, edit the YAML for the current lab session, add variable overrides, validate the definition, and launch a scoped run.',
        items: [
          'Session saves keep Lab edits local to the browser and do not overwrite saved pipelines.',
          'Validation suggestions explain common YAML issues and often include a small fix example.',
          'Overrides are useful for trying one value without changing the underlying scope.',
        ],
        example: 'Example: select team-1/build-and-test, target dev, override IMAGE_TAG=pr-42, validate, then run once.',
      },
    ],
  },
  steps: {
    title: 'Steps',
    summary: 'Steps are reusable building blocks that pipelines can include instead of repeating scripts or LLM tasks.',
    docsPath: 'steps',
    sections: [
      {
        title: 'Options and Features',
        body: 'Browse step libraries, create drafts, clone read-only Git steps, edit YAML, copy or download definitions, and see which pipelines include each step.',
        items: [
          'A step can contain a script, a goal, tasks, variables, secrets, MCP profiles, and failure behavior.',
          'Access controls decide which groups, repositories, or service accounts can reuse a step.',
          'Used in Pipelines shows the blast radius before editing or deleting a reusable step.',
        ],
        example: 'name: shared/checkout\ndescription: Checkout repository\nscript: |\n  git fetch --all\n  git checkout "$GIT_REF"',
      },
    ],
  },
  'knowledge-context': {
    title: 'Knowledge Context',
    summary: 'Knowledge context stores curated markdown used by AI-enabled pipeline steps as architecture, guardrails, policies, runbooks, and examples.',
    docsPath: 'knowledge-context',
    sections: [
      {
        title: 'Options and Features',
        body: 'Browse by kind and group, create documents, edit descriptions and content, copy or download markdown, manage access, and see which pipelines reference each document.',
        items: [
          'Kinds include architecture, guardrail, policy, ADR, guideline, runbook, reference, and example.',
          'Access controls keep sensitive operational knowledge limited to the right groups, repositories, or service accounts.',
          'Used in Pipelines helps you understand which automations depend on a document.',
        ],
        example: '---\nname: release-evidence\nkind: policy\n---\n# Release Evidence\nEvery production deploy must attach test results and rollout notes.',
      },
    ],
  },
  'system/config': {
    title: 'System Config',
    summary: 'System Config controls runtime defaults, service discovery, and the global GitOps config repository.',
    docsPath: 'system/config',
    sections: [
      {
        title: 'Options and Features',
        body: 'Configure runner images, Docker network, pipeline timeouts, agent and git-bot URLs, automatic container cleanup, and the global config repository sync source.',
        items: [
          'Runners & timeouts affect how jobs are executed and how long they may run.',
          'Service discovery URLs let containers call the NopsAI API, agent, and git-bot services.',
          'Global config repository settings point sync at the Git source of truth for shared resources.',
        ],
        example: 'Example config repo: https://github.com/acme/nopsai-config on branch main with base path nopsai.',
      },
    ],
  },
  'system/setup': {
    title: 'First-Install Setup',
    summary: 'Setup guides the initial control-plane configuration and can generate starter files for GitOps.',
    docsPath: 'system/setup',
    sections: [
      {
        title: 'Options and Features',
        body: 'Step through readiness checks, service-level runtime values, global config repository setup, GitHub App wiring, repository groups, AI profile defaults, starter users, and generated setup output.',
        items: [
          'Blocking preflight errors must be resolved before the install is considered ready.',
          'Generated output separates runtime environment values from files that should be committed to Git.',
          'MCP examples can be included disabled, so teams can review and enable tools later.',
        ],
        example: 'Example starter group: team-1 with repositories acme/service-api and acme/web-app, plus developer access for alice@example.com.',
      },
    ],
  },
  'system/llm-profiles': {
    title: 'LLM Profiles',
    summary: 'LLM profiles define which model providers pipeline AI steps can use and where API keys are stored.',
    docsPath: 'system/llm-profiles',
    sections: [
      {
        title: 'Options and Features',
        body: 'Set the default profile, create or edit provider profiles, test a profile, limit allowed scopes, configure reasoning or thinking behavior, and migrate references before deleting profiles.',
        items: [
          'Provider chooses the backend, such as gemini or lmstudio.',
          'Model and base URL identify the concrete model endpoint.',
          'API key secret names the scope secret that supplies credentials at runtime.',
        ],
        example: 'Name: reasoning\nProvider: gemini\nModel: gemini-2.5-pro\nAPI key secret: GEMINI_API_KEY\nAllowed scopes: dev, internal',
      },
    ],
  },
  'system/mcp': {
    title: 'MCP',
    summary: 'MCP settings register external tool servers and assemble tool profiles for AI-enabled tasks.',
    docsPath: 'system/mcp',
    sections: [
      {
        title: 'Options and Features',
        body: 'Manage MCP servers, headers, auth secret references, timeouts, allowed scopes, tool allow-lists, and reusable profiles that pipeline steps can request.',
        items: [
          'Servers describe where tools are hosted and how to authenticate to them.',
          'Profiles choose which tools from one or more servers are available to a task.',
          'Allowed scopes prevent a profile or server from being used outside approved runtime scopes.',
        ],
        example: 'Server: github\nURL: https://api.githubcopilot.com/mcp/x/all/readonly\nProfile: github-pr-review\nTools: issues_list, repos_get',
      },
    ],
  },
  'system/dispatcher': {
    title: 'Dispatcher',
    summary: 'Dispatcher shows registered runners, active capacity, routing state, and deployment instructions for adding runners.',
    docsPath: 'system/dispatcher',
    sections: [
      {
        title: 'Options and Features',
        body: 'Monitor runner heartbeats, capacity, active jobs, routing metadata, and generate runner deployment templates for specific scopes.',
        items: [
          'Runners can be scoped so only matching jobs are dispatched to them.',
          'Capacity and inflight jobs help explain queueing and scheduling behavior.',
          'The deployment guide creates compose and command examples for new runners.',
        ],
        example: 'Example: generate a runner for scopes dev,prod with capacity 2, then deploy it on the same Docker network as the dispatcher.',
      },
    ],
  },
  'system/access': {
    title: 'Access',
    summary: 'Access manages who can sign in and what they can do across folders, resources, and low-level policies.',
    docsPath: 'system/access',
    sections: [
      {
        title: 'Options and Features',
        body: 'Use basic mode for users and product roles, or advanced mode for role bundles and policy rules. Assign viewer, developer, owner, or admin roles to folders, pipelines, scopes, and repositories.',
        items: [
          'Users define sign-in identity, status, password, and assigned roles.',
          'Basic roles are friendlier product-level grants that can inherit through folders.',
          'Advanced roles and policies expose the lower-level resource/action model for platform administrators.',
        ],
        example: 'Example grant: alice@example.com gets developer on folder team-1 with inherit enabled, so nested pipelines and scopes are covered.',
      },
    ],
  },
  profile: {
    title: 'Profile',
    summary: 'Profile contains your account details, password change flow, personal tokens, and quick links to permitted system pages.',
    docsPath: 'profile',
    sections: [
      {
        title: 'Options and Features',
        body: 'Update your email where permitted, change your password, create personal access tokens for scripts, revoke old tokens, and sign out.',
        items: [
          'Personal tokens are shown once when created; revoke any token that is no longer needed.',
          'Password changes may be required after first install or after an administrator reset.',
          'System links appear only when your account has access to those pages.',
        ],
        example: 'Example token name: Deployment script. Store the generated token in your automation secret manager and revoke it during rotation.',
      },
    ],
  },
  default: {
    title: 'Help',
    summary: 'This panel explains the current page, its major controls, and a small example for common workflows.',
    docsPath: 'overview',
    sections: [
      {
        title: 'Using Help',
        body: 'Open the help sign from any authenticated page to get page-specific guidance. When full documentation is available, the documentation link can be enabled from one central setting.',
      },
    ],
  },
};

function buildDocumentationHref(docsPath: string) {
  const base = DOCUMENTATION_BASE_URL.trim().replace(/\/+$/, '');
  if (!base) return '';
  return `${base}/${docsPath.replace(/^\/+/, '')}`;
}

function resolveHelpTopic(pathname: string): HelpTopic {
  const segments = pathname.split('/').filter(Boolean);
  const primary = segments[0] || '';
  const secondary = segments[1] || '';

  if (primary === 'pipelineruns') {
    return HELP_TOPICS[`pipelineruns/${secondary || 'main'}`] || HELP_TOPICS['pipelineruns/main'];
  }
  if (primary === 'system') {
    return HELP_TOPICS[`system/${secondary || 'config'}`] || HELP_TOPICS['system/config'];
  }
  if (primary === 'knowledge-context') {
    return HELP_TOPICS['knowledge-context'];
  }

  return HELP_TOPICS[primary] || HELP_TOPICS.default;
}

export default function AppHelp() {
  const location = useLocation();
  const topic = useMemo(() => resolveHelpTopic(location.pathname), [location.pathname]);
  const docsHref = useMemo(() => buildDocumentationHref(topic.docsPath), [topic.docsPath]);
  const [openState, setOpenState] = useState({ pathname: location.pathname, open: false });
  const open = openState.pathname === location.pathname && openState.open;
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const handlePointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpenState({ pathname: location.pathname, open: false });
      }
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpenState({ pathname: location.pathname, open: false });
      }
    };
    document.addEventListener('mousedown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('mousedown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [location.pathname, open]);

  return (
    <div className="app-help" ref={rootRef}>
      <button
        type="button"
        className={`app-help__trigger ${open ? 'app-help__trigger--open' : ''}`}
        onClick={() => setOpenState(value => ({ pathname: location.pathname, open: value.pathname === location.pathname ? !value.open : true }))}
        aria-label={`Help for ${topic.title}`}
        aria-haspopup="dialog"
        aria-expanded={open}
        title="Help"
      >
        <CircleHelp className="h-5 w-5" aria-hidden="true" />
      </button>
      {open && (
        <div className="app-help__panel" role="dialog" aria-label={`${topic.title} help`}>
          <div className="app-help__header">
            <div className="app-help__header-copy">
              <span className="app-help__eyebrow">Help</span>
              <h2>{topic.title}</h2>
              <p>{topic.summary}</p>
            </div>
            <button type="button" className="app-help__close" onClick={() => setOpenState({ pathname: location.pathname, open: false })} aria-label="Close help">
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
                <span>Open full documentation</span>
                <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
              </a>
            ) : (
              <div className="app-help__docs-link app-help__docs-link--placeholder">
                <BookOpen className="h-4 w-4" aria-hidden="true" />
                <span>Documentation link ready for</span>
                <code>{topic.docsPath}</code>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
