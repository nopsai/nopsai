import { ObjectIcon } from '../components/ObjectIcon.js';
import type { NavItem } from './types.js';
export { eventAutomationNavPath, pipelineRunsNavPath, sidebarNavItemIsActive } from './navigationModel.js';

export const baseNavItems: NavItem[] = [
  {
    label: 'Pipeline runs',
    path: '/pipelineruns/main',
    icon: <ObjectIcon type="pipeline-run" />,
  },
  {
    label: 'Monitoring',
    path: '/monitoring',
    icon: <ObjectIcon type="monitoring" />,
  },
  {
    label: 'Dashboards',
    path: '/dashboards',
    icon: <ObjectIcon type="dashboard" />,
  },
  {
    label: 'Pipelines',
    path: '/pipelines',
    icon: <ObjectIcon type="pipeline" />,
  },
  {
    label: 'Schedules',
    path: '/schedules',
    icon: <ObjectIcon type="schedule" />,
  },
  {
    label: 'Triggers',
    path: '/triggers',
    icon: <ObjectIcon type="trigger" />,
  },
  {
    label: 'Steps',
    path: '/steps',
    icon: <ObjectIcon type="step" />,
  },
  {
    label: 'Lab',
    path: '/lab',
    icon: <ObjectIcon type="lab" />,
  },
  {
    label: 'Assistant',
    path: '/assistant',
    icon: <ObjectIcon type="assistant" />,
  },
  {
    label: 'Agent roles',
    path: '/agent-profiles',
    icon: <ObjectIcon type="agent-profile" />,
  },
  {
    label: 'Models',
    path: '/llm-profiles',
    icon: <ObjectIcon type="llm-profile" />,
  },
  {
    label: 'Knowledge',
    path: '/knowledge-context',
    icon: <ObjectIcon type="knowledge-context" />,
  },
  {
    label: 'MCP',
    path: '/mcp',
    icon: <ObjectIcon type="mcp-profile" />,
  },
  {
    label: 'Teams',
    path: '/teams',
    icon: <ObjectIcon type="team" />,
  },
  {
    label: 'Scopes',
    path: '/scopes',
    icon: <ObjectIcon type="scope" />,
  },
  {
    label: 'Credentials',
    path: '/credentials',
    icon: <ObjectIcon type="credential" />,
  },
];

export const baseSystemSubNav: NavItem[] = [
  { label: 'Identity & Access', path: '/system/access', icon: <ObjectIcon type="access" /> },
  { label: 'General', path: '/system/config', icon: <ObjectIcon type="system-config" /> },
  { label: 'Git Apps', path: '/system/git-apps', icon: <ObjectIcon type="git-app" /> },
  { label: 'Security', path: '/system/setup', icon: <ObjectIcon type="setup" /> },
  { label: 'Data', path: '/system/data-management', icon: <ObjectIcon type="data-management" /> },
  { label: 'Runtime', path: '/system/dispatcher', icon: <ObjectIcon type="dispatcher" /> },
  { label: 'System Logs', path: '/system/logs', icon: <ObjectIcon type="system-logs" /> },
];

export const titleMap: Record<string, string> = {
  pipelineruns: 'Pipeline runs',
  monitoring: 'Monitoring',
  dashboards: 'Dashboards',
  teams: 'Teams',
  assistant: 'Assistant',
  'llm-profiles': 'Models',
  'agent-profiles': 'Agent roles',
  mcp: 'MCP',
  credentials: 'Credentials',
  docs: 'Wiki',
  pipelines: 'Pipelines',
  schedules: 'Schedules',
  triggers: 'Triggers',
  'external-triggers': 'External Triggers',
  'git-webhook-sources': 'Git Webhook Sources',
  scopes: 'Scopes',
  lab: 'Lab',
  steps: 'Steps',
  'knowledge-context': 'Knowledge',
  system: 'Administration',
  profile: 'Profile',
};

export const descriptionMap: Record<string, string> = {
  pipelineruns: 'Track executions, approvals, logs, and reruns.',
  monitoring: 'Watch platform health, recommendations, AI usage, and reliability.',
  dashboards: 'Publish and review pipeline output dashboards.',
  teams: 'Manage team hierarchy, repository ownership, and config scopes.',
  assistant: 'Ask NopsAI for operational guidance and controlled actions.',
  'llm-profiles': 'Manage approved model providers and defaults.',
  'agent-profiles': 'Manage reusable agent roles, behavior, and runtime permissions.',
  mcp: 'Manage MCP servers, profiles, tools, and access.',
  credentials: 'Manage credential metadata and encrypted versions.',
  docs: 'Read product documentation and operating guidance.',
  pipelines: 'Create, inspect, and run pipeline definitions.',
  schedules: 'Manage timed pipeline automation.',
  triggers: 'Manage repository event automation.',
  'external-triggers': 'Manage webhook endpoints for external automation.',
  'git-webhook-sources': 'Manage Git webhook ingress sources.',
  scopes: 'Manage scoped variables and secrets.',
  lab: 'Experiment with pipeline YAML before saving.',
  steps: 'Build reusable step definitions.',
  'knowledge-context': 'Manage run-time knowledge documents and provider connections.',
  system: 'Configure identity, Git apps, data, runtime, logs, and security posture.',
  profile: 'Manage your profile, password, and tokens.',
};
