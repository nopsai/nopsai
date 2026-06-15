import {
  IconBell,
  IconCalendarSchedule,
  IconCog,
  IconDatabase,
  IconDispatch,
  IconFlask,
  IconFlow,
  IconKnowledge,
  IconMonitoring,
  IconPlay,
  IconScope,
  IconShield,
  IconSteps,
  IconZap,
} from './icons';
import type { NavItem } from './types';

export const baseNavItems: NavItem[] = [
  {
    label: 'Pipeline runs',
    path: '/pipelineruns/main',
    icon: <IconPlay />,
  },
  {
    label: 'Monitoring',
    path: '/monitoring',
    icon: <IconMonitoring />,
  },
  {
    label: 'Pipelines',
    path: '/pipelines',
    icon: <IconFlow />,
  },
  {
    label: 'Schedules',
    path: '/schedules',
    icon: <IconCalendarSchedule />,
  },
  {
    label: 'Triggers',
    path: '/triggers',
    icon: <IconBell />,
  },
  {
    label: 'External Triggers',
    path: '/external-triggers',
    icon: <IconZap />,
  },
  {
    label: 'Scopes',
    path: '/scopes',
    icon: <IconScope />,
  },
  {
    label: 'Lab',
    path: '/lab',
    icon: <IconFlask />,
  },
  {
    label: 'Steps',
    path: '/steps',
    icon: <IconSteps />,
  },
  {
    label: 'Knowledge Context',
    path: '/knowledge-context',
    icon: <IconKnowledge />,
  },
  {
    label: 'System',
    path: '/system/config',
    icon: <IconCog />,
  },
];

export const baseSystemSubNav: NavItem[] = [
  { label: 'Config', path: '/system/config', icon: <IconCog /> },
  { label: 'Setup', path: '/system/setup', icon: <IconShield /> },
  { label: 'LLM Profiles', path: '/system/llm-profiles', icon: <IconFlask /> },
  { label: 'Agent Profiles', path: '/system/agent-profiles', icon: <IconShield /> },
  { label: 'MCP', path: '/system/mcp', icon: <IconFlask /> },
  { label: 'Credentials', path: '/system/credentials', icon: <IconShield /> },
  { label: 'Data Management', path: '/system/data-management', icon: <IconDatabase /> },
  { label: 'Dispatcher', path: '/system/dispatcher', icon: <IconDispatch /> },
  { label: 'Access', path: '/system/access', icon: <IconShield /> },
];

export const titleMap: Record<string, string> = {
  pipelineruns: 'Pipeline runs',
  monitoring: 'Monitoring',
  docs: 'Docs',
  pipelines: 'Pipelines',
  schedules: 'Schedules',
  triggers: 'Triggers',
  'external-triggers': 'External Triggers',
  scopes: 'Scopes',
  lab: 'Lab',
  steps: 'Steps',
  'knowledge-context': 'Knowledge Context',
  system: 'System',
  profile: 'Profile',
};
