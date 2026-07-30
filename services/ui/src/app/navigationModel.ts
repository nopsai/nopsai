export const SIDEBAR_NAV_ADMINISTRATION_TOPIC_ID = 'administration';
export const SIDEBAR_NAV_ADMINISTRATION_TOPIC_LABEL = 'Administration';

export const sidebarNavTopics = [
  {
    id: 'observe',
    label: 'Observe',
    itemLabels: ['Pipeline runs', 'Monitoring', 'Dashboards'],
  },
  {
    id: 'build-automate',
    label: 'Build & Automate',
    itemLabels: ['Pipelines', 'Schedules', 'Triggers', 'Steps'],
  },
  {
    id: 'lab',
    label: 'Lab',
    itemLabels: ['Lab'],
  },
  {
    id: 'ai-knowledge',
    label: 'AI & Knowledge',
    itemLabels: ['Assistant', 'Agent roles', 'Models', 'Knowledge', 'MCP'],
  },
  {
    id: 'workspace',
    label: 'Workspace',
    itemLabels: ['Teams', 'Scopes', 'Credentials'],
  },
] as const;

export type SidebarNavTopicID = typeof sidebarNavTopics[number]['id'] | 'other';

export type SidebarNavTopic<TItem> = {
  id: SidebarNavTopicID;
  label: string;
  items: TItem[];
};

const topicByItemLabel = new Map<string, typeof sidebarNavTopics[number]>();

for (const topic of sidebarNavTopics) {
  for (const label of topic.itemLabels) {
    topicByItemLabel.set(label, topic);
  }
}

export function pipelineRunsNavPath(pathname: string) {
  if (pathname.startsWith('/pipelineruns/recent')) return '/pipelineruns/recent';
  if (pathname.startsWith('/pipelineruns/events')) return '/pipelineruns/events';
  return '/pipelineruns/main';
}

export function eventAutomationNavPath({
  canViewTriggers,
  canViewExternalTriggers,
  canViewGitWebhookSources,
}: {
  canViewTriggers: boolean;
  canViewExternalTriggers: boolean;
  canViewGitWebhookSources: boolean;
}) {
  if (canViewTriggers) return '/triggers';
  if (canViewExternalTriggers) return '/external-triggers';
  if (canViewGitWebhookSources) return '/git-webhook-sources';
  return '/triggers';
}

export function isEventAutomationRoute(pathname: string) {
  return pathname.startsWith('/triggers') ||
    pathname.startsWith('/external-triggers') ||
    pathname.startsWith('/git-webhook-sources');
}

export function sidebarNavItemIsActive(itemPath: string, pathname: string) {
  if (itemPath === '/triggers' || itemPath === '/external-triggers' || itemPath === '/git-webhook-sources') {
    return isEventAutomationRoute(pathname);
  }
  return pathname === itemPath || pathname.startsWith(itemPath);
}

export function groupNavItemsByTopic<TItem extends { label: string }>(
  items: readonly TItem[]
): SidebarNavTopic<TItem>[] {
  const grouped = new Map<SidebarNavTopicID, SidebarNavTopic<TItem>>(
    sidebarNavTopics.map(topic => [topic.id, { id: topic.id, label: topic.label, items: [] }])
  );
  const uncategorized: TItem[] = [];

  for (const item of items) {
    const topic = topicByItemLabel.get(item.label);
    if (!topic) {
      uncategorized.push(item);
      continue;
    }
    grouped.get(topic.id)?.items.push(item);
  }

  const topics = sidebarNavTopics
    .map(topic => grouped.get(topic.id))
    .filter((topic): topic is SidebarNavTopic<TItem> => Boolean(topic && topic.items.length > 0));

  if (uncategorized.length > 0) {
    topics.push({ id: 'other', label: 'Other', items: uncategorized });
  }

  return topics;
}
