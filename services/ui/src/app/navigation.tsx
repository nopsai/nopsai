import { ObjectIcon } from '../components/ObjectIcon.js';
import type { NavItem } from './types.js';
export { pipelineRunsNavPath } from './navigationModel.js';

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
    label: 'Teams',
    path: '/teams',
    icon: <ObjectIcon type="team" />,
  },
  {
    label: 'Assistant',
    path: '/assistant',
    icon: <ObjectIcon type="assistant" />,
  },
  {
    label: 'LLM Profiles',
    path: '/llm-profiles',
    icon: <ObjectIcon type="llm-profile" />,
  },
  {
    label: 'Agent Profiles',
    path: '/agent-profiles',
    icon: <ObjectIcon type="agent-profile" />,
  },
  {
    label: 'MCP',
    path: '/mcp',
    icon: <ObjectIcon type="mcp-profile" />,
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
    label: 'External Triggers',
    path: '/external-triggers',
    icon: <ObjectIcon type="external-trigger" />,
  },
  {
    label: 'Git Webhook Sources',
    path: '/git-webhook-sources',
    icon: <ObjectIcon type="git-webhook-source" />,
  },
  {
    label: 'Scopes',
    path: '/scopes',
    icon: <ObjectIcon type="scope" />,
  },
  {
    label: 'Lab',
    path: '/lab',
    icon: <ObjectIcon type="lab" />,
  },
  {
    label: 'Steps',
    path: '/steps',
    icon: <ObjectIcon type="step" />,
  },
  {
    label: 'Knowledge Context',
    path: '/knowledge-context',
    icon: <ObjectIcon type="knowledge-context" />,
  },
  {
    label: 'System',
    path: '/system/config',
    icon: <ObjectIcon type="system" />,
  },
];

export const baseSystemSubNav: NavItem[] = [
  { label: 'Config', path: '/system/config', icon: <ObjectIcon type="system-config" /> },
  { label: 'Setup', path: '/system/setup', icon: <ObjectIcon type="setup" /> },
  { label: 'Credentials', path: '/system/credentials', icon: <ObjectIcon type="credential" /> },
  { label: 'Data Management', path: '/system/data-management', icon: <ObjectIcon type="data-management" /> },
  { label: 'Dispatcher', path: '/system/dispatcher', icon: <ObjectIcon type="dispatcher" /> },
  { label: 'Logs', path: '/system/logs', icon: <ObjectIcon type="system-logs" /> },
  { label: 'Access', path: '/system/access', icon: <ObjectIcon type="access" /> },
];

export const titleMap: Record<string, string> = {
  pipelineruns: 'Pipeline runs',
  monitoring: 'Monitoring',
  teams: 'Teams',
  assistant: 'Assistant',
  'llm-profiles': 'LLM Profiles',
  'agent-profiles': 'Agent Profiles',
  mcp: 'MCP',
  docs: 'Docs',
  pipelines: 'Pipelines',
  schedules: 'Schedules',
  triggers: 'Triggers',
  'external-triggers': 'External Triggers',
  'git-webhook-sources': 'Git Webhook Sources',
  scopes: 'Scopes',
  lab: 'Lab',
  steps: 'Steps',
  'knowledge-context': 'Knowledge Context',
  system: 'System',
  profile: 'Profile',
};
