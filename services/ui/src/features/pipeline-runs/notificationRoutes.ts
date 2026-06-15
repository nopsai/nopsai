export type NotificationEventKey =
  | 'failure'
  | 'success'
  | 'pending'
  | 'running'
  | 'waiting_approval'
  | 'approval_requested'
  | 'approval_approved'
  | 'approval_rejected'
  | 'cancelled'
  | 'skipped';

export type NotificationRecipientSet = {
  teams?: string[];
  users?: string[];
  groups?: string[];
};

export type NotificationPatternFilter = {
  include?: string[];
  exclude?: string[];
};

export type NotificationRouteRule = {
  name: string;
  enabled: boolean;
  recipients: {
    include?: NotificationRecipientSet;
    exclude?: NotificationRecipientSet;
  };
  events: Record<NotificationEventKey, boolean>;
  filters: {
    pipelines?: NotificationPatternFilter;
    repos?: NotificationPatternFilter;
    branches?: NotificationPatternFilter;
  };
  delivery: {
    channels?: string[];
    throttle?: {
      dedupe_window?: string;
      max_per_run?: number;
    };
  };
};

export type NotificationRouteDefinition = {
  enabled: boolean;
  recipients: NotificationRouteRule['recipients'];
  events: NotificationRouteRule['events'];
  filters: NotificationRouteRule['filters'];
  delivery: NotificationRouteRule['delivery'];
  routes?: NotificationRouteRule[];
};

export type NotificationRouteRecord = {
  id?: number;
  group_id?: number;
  group_path?: string;
  definition: NotificationRouteDefinition;
  source?: string;
  config_source_path?: string;
  managed_by_config_repo?: boolean;
  updated_at?: string;
};

export type NotificationRouteFormState = {
  routeName: string;
  selectedRouteIndex: number;
  routes: NotificationRouteRule[];
  enabled: boolean;
  includeSameGroup: boolean;
  includeUsers: string;
  includeGroups: string;
  excludeUsers: string;
  excludeGroups: string;
  events: Record<NotificationEventKey, boolean>;
  pipelineInclude: string;
  pipelineExclude: string;
  repoInclude: string;
  repoExclude: string;
  branchInclude: string;
  branchExclude: string;
  dedupeWindow: string;
  maxPerRun: string;
};

export const NOTIFICATION_EVENTS: Array<{ key: NotificationEventKey; label: string }> = [
  { key: 'failure', label: 'Failure' },
  { key: 'success', label: 'Success' },
  { key: 'pending', label: 'Pending' },
  { key: 'running', label: 'Running' },
  { key: 'waiting_approval', label: 'Waiting approval' },
  { key: 'approval_requested', label: 'Approval requested' },
  { key: 'approval_approved', label: 'Approval approved' },
  { key: 'approval_rejected', label: 'Approval rejected' },
  { key: 'cancelled', label: 'Cancelled' },
  { key: 'skipped', label: 'Skipped' },
];

export function folderNotificationGitOpsTarget(basePath: string): string {
  const normalizedBasePath = basePath.trim().replaceAll('\\', '/').replace(/^\/+|\/+$/g, '');
  return normalizedBasePath ? `${normalizedBasePath}/notifications.yaml` : 'notifications.yaml';
}

export function defaultNotificationEventState(): Record<NotificationEventKey, boolean> {
  return {
    failure: true,
    success: false,
    pending: false,
    running: false,
    waiting_approval: true,
    approval_requested: true,
    approval_approved: false,
    approval_rejected: true,
    cancelled: true,
    skipped: false,
  };
}

export function defaultNotificationRouteRule(name: string): NotificationRouteRule {
  return {
    name,
    enabled: true,
    recipients: {
      include: { teams: ['same_group'], users: [], groups: [] },
      exclude: { users: [], groups: [] },
    },
    events: defaultNotificationEventState(),
    filters: {
      pipelines: { include: ['*'], exclude: [] },
      repos: { include: ['*'], exclude: [] },
      branches: { include: ['*'], exclude: [] },
    },
    delivery: {
      channels: ['mail'],
      throttle: {
        dedupe_window: '10m',
        max_per_run: 5,
      },
    },
  };
}

export function defaultNotificationRouteDefinition(): NotificationRouteDefinition {
  const route = defaultNotificationRouteRule('default');
  return {
    enabled: true,
    recipients: route.recipients,
    events: route.events,
    filters: route.filters,
    delivery: route.delivery,
    routes: [route],
  };
}

export function createEmptyNotificationRouteForm(): NotificationRouteFormState {
  return {
    routeName: 'default',
    selectedRouteIndex: 0,
    routes: [defaultNotificationRouteRule('default')],
    enabled: true,
    includeSameGroup: true,
    includeUsers: '',
    includeGroups: '',
    excludeUsers: '',
    excludeGroups: '',
    events: defaultNotificationEventState(),
    pipelineInclude: '*',
    pipelineExclude: '',
    repoInclude: '*',
    repoExclude: '',
    branchInclude: '*',
    branchExclude: '',
    dedupeWindow: '10m',
    maxPerRun: '5',
  };
}

export function normalizeNotificationRouteRecord(payload: unknown): NotificationRouteRecord {
  if (!payload || typeof payload !== 'object') {
    return { definition: defaultNotificationRouteDefinition(), source: 'database', managed_by_config_repo: false };
  }
  const record = payload as Record<string, unknown>;
  const id = typeof record.id === 'number' ? record.id : Number(record.id);
  const groupID = typeof record.group_id === 'number' ? record.group_id : Number(record.group_id);
  return {
    id: Number.isFinite(id) && id > 0 ? id : undefined,
    group_id: Number.isFinite(groupID) && groupID > 0 ? groupID : undefined,
    group_path: typeof record.group_path === 'string' ? record.group_path : undefined,
    definition: normalizeNotificationRouteDefinition(record.definition),
    source: typeof record.source === 'string' ? record.source : 'database',
    config_source_path: typeof record.config_source_path === 'string' ? record.config_source_path : undefined,
    managed_by_config_repo: Boolean(record.managed_by_config_repo),
    updated_at: typeof record.updated_at === 'string' ? record.updated_at : undefined,
  };
}

export function normalizeNotificationRouteDefinition(payload: unknown): NotificationRouteDefinition {
  const fallback = defaultNotificationRouteDefinition();
  if (!payload || typeof payload !== 'object') return fallback;
  const record = payload as Record<string, unknown>;
  const legacyRoute = normalizeNotificationRouteRule(record, 'default');
  const rawRoutes = Array.isArray(record.routes) ? record.routes : [];
  const routes = rawRoutes.length > 0
    ? rawRoutes.map((item, index) => normalizeNotificationRouteRule(item, `route-${index + 1}`))
    : [legacyRoute];
  const first = routes[0] || legacyRoute;
  return {
    enabled: typeof record.enabled === 'boolean' ? record.enabled : fallback.enabled,
    recipients: first.recipients,
    events: first.events,
    filters: first.filters,
    delivery: first.delivery,
    routes,
  };
}

export function normalizeNotificationRouteRule(payload: unknown, fallbackName: string): NotificationRouteRule {
  const record = payload && typeof payload === 'object' ? payload as Record<string, unknown> : {};
  const rawRecipients = asRecord(record.recipients);
  const rawEvents = asRecord(record.events);
  const rawFilters = asRecord(record.filters);
  const rawDelivery = asRecord(record.delivery);
  const rawThrottle = asRecord(rawDelivery.throttle);
  const events = defaultNotificationEventState();
  NOTIFICATION_EVENTS.forEach(option => {
    if (typeof rawEvents[option.key] === 'boolean') events[option.key] = rawEvents[option.key] as boolean;
  });
  const channels = normalizeStringArray(rawDelivery.channels);
  return {
    name: typeof record.name === 'string' && record.name.trim() ? record.name.trim() : fallbackName,
    enabled: typeof record.enabled === 'boolean' ? record.enabled : true,
    recipients: {
      include: normalizeNotificationRecipientSet(rawRecipients.include),
      exclude: normalizeNotificationRecipientSet(rawRecipients.exclude),
    },
    events,
    filters: {
      pipelines: normalizeNotificationPatternFilter(rawFilters.pipelines),
      repos: normalizeNotificationPatternFilter(rawFilters.repos),
      branches: normalizeNotificationPatternFilter(rawFilters.branches),
    },
    delivery: {
      channels: channels.length > 0 ? channels : ['mail'],
      throttle: {
        dedupe_window: typeof rawThrottle.dedupe_window === 'string' && rawThrottle.dedupe_window.trim()
          ? rawThrottle.dedupe_window
          : '10m',
        max_per_run: typeof rawThrottle.max_per_run === 'number' && rawThrottle.max_per_run > 0
          ? rawThrottle.max_per_run
          : 5,
      },
    },
  };
}

export function notificationRouteFormFromDefinition(definition: NotificationRouteDefinition): NotificationRouteFormState {
  const routes = (definition.routes?.length ? definition.routes : [normalizeNotificationRouteRule(definition, 'default')])
    .map((route, index) => normalizeNotificationRouteRule(route, route.name || `route-${index + 1}`));
  return notificationRouteFormFromRule(routes, 0);
}

export function notificationRoutePayloadFromForm(form: NotificationRouteFormState): NotificationRouteDefinition {
  const committed = notificationRouteFormCommitCurrentRoute(form);
  const routes = committed.routes.length > 0 ? committed.routes : [defaultNotificationRouteRule('default')];
  const first = routes[0];
  return {
    enabled: routes.some(route => route.enabled),
    recipients: first.recipients,
    events: first.events,
    filters: first.filters,
    delivery: first.delivery,
    routes,
  };
}

export function notificationRouteFormSelectRoute(form: NotificationRouteFormState, index: number): NotificationRouteFormState {
  const committed = notificationRouteFormCommitCurrentRoute(form);
  return notificationRouteFormFromRule(committed.routes, index);
}

export function notificationRouteFormAddRoute(form: NotificationRouteFormState): NotificationRouteFormState {
  const committed = notificationRouteFormCommitCurrentRoute(form);
  let nextNumber = committed.routes.length + 1;
  const existingNames = new Set(committed.routes.map(route => route.name.toLowerCase()));
  let name = `route-${nextNumber}`;
  while (existingNames.has(name.toLowerCase())) {
    nextNumber += 1;
    name = `route-${nextNumber}`;
  }
  const routes = [...committed.routes, defaultNotificationRouteRule(name)];
  return notificationRouteFormFromRule(routes, routes.length - 1);
}

export function notificationRouteFormRemoveSelectedRoute(form: NotificationRouteFormState): NotificationRouteFormState {
  const committed = notificationRouteFormCommitCurrentRoute(form);
  if (committed.routes.length <= 1) return committed;
  const routes = committed.routes.filter((_, index) => index !== committed.selectedRouteIndex);
  return notificationRouteFormFromRule(routes, Math.min(committed.selectedRouteIndex, routes.length - 1));
}

function notificationRouteFormFromRule(
  routes: NotificationRouteRule[],
  selectedRouteIndex: number
): NotificationRouteFormState {
  const safeIndex = Math.min(Math.max(selectedRouteIndex, 0), Math.max(routes.length - 1, 0));
  const route = routes[safeIndex] || defaultNotificationRouteRule('default');
  const include = route.recipients.include || {};
  const exclude = route.recipients.exclude || {};
  const events = defaultNotificationEventState();
  NOTIFICATION_EVENTS.forEach(option => {
    events[option.key] = Boolean(route.events?.[option.key]);
  });
  return {
    routeName: route.name || `route-${safeIndex + 1}`,
    selectedRouteIndex: safeIndex,
    routes,
    enabled: route.enabled,
    includeSameGroup: (include.teams || []).includes('same_group'),
    includeUsers: notificationListToText(include.users),
    includeGroups: notificationListToText(include.groups),
    excludeUsers: notificationListToText(exclude.users),
    excludeGroups: notificationListToText(exclude.groups),
    events,
    pipelineInclude: notificationListToText(route.filters.pipelines?.include || ['*']),
    pipelineExclude: notificationListToText(route.filters.pipelines?.exclude),
    repoInclude: notificationListToText(route.filters.repos?.include || ['*']),
    repoExclude: notificationListToText(route.filters.repos?.exclude),
    branchInclude: notificationListToText(route.filters.branches?.include || ['*']),
    branchExclude: notificationListToText(route.filters.branches?.exclude),
    dedupeWindow: route.delivery.throttle?.dedupe_window || '10m',
    maxPerRun: String(route.delivery.throttle?.max_per_run || 5),
  };
}

function notificationRouteFormCommitCurrentRoute(form: NotificationRouteFormState): NotificationRouteFormState {
  const routes = form.routes.length > 0 ? [...form.routes] : [defaultNotificationRouteRule('default')];
  const safeIndex = Math.min(Math.max(form.selectedRouteIndex, 0), routes.length - 1);
  routes[safeIndex] = notificationRouteRuleFromForm(form);
  return { ...form, routes, selectedRouteIndex: safeIndex };
}

function notificationRouteRuleFromForm(form: NotificationRouteFormState): NotificationRouteRule {
  const maxPerRun = Number.parseInt(form.maxPerRun, 10);
  const events = defaultNotificationEventState();
  NOTIFICATION_EVENTS.forEach(option => {
    events[option.key] = Boolean(form.events[option.key]);
  });
  return {
    name: form.routeName.trim() || `route-${form.selectedRouteIndex + 1}`,
    enabled: form.enabled,
    recipients: {
      include: {
        teams: form.includeSameGroup ? ['same_group'] : [],
        users: notificationTextToList(form.includeUsers),
        groups: notificationTextToList(form.includeGroups),
      },
      exclude: {
        users: notificationTextToList(form.excludeUsers),
        groups: notificationTextToList(form.excludeGroups),
      },
    },
    events,
    filters: {
      pipelines: notificationPatternPayload(form.pipelineInclude, form.pipelineExclude),
      repos: notificationPatternPayload(form.repoInclude, form.repoExclude),
      branches: notificationPatternPayload(form.branchInclude, form.branchExclude),
    },
    delivery: {
      channels: ['mail'],
      throttle: {
        dedupe_window: form.dedupeWindow.trim() || '10m',
        max_per_run: Number.isFinite(maxPerRun) && maxPerRun > 0 ? maxPerRun : 5,
      },
    },
  };
}

function normalizeNotificationRecipientSet(payload: unknown): NotificationRecipientSet {
  const record = asRecord(payload);
  return {
    teams: normalizeStringArray(record.teams),
    users: normalizeStringArray(record.users),
    groups: normalizeStringArray(record.groups),
  };
}

function normalizeNotificationPatternFilter(payload: unknown): NotificationPatternFilter {
  const record = asRecord(payload);
  const include = normalizeStringArray(record.include);
  return {
    include: include.length > 0 ? include : ['*'],
    exclude: normalizeStringArray(record.exclude),
  };
}

function notificationPatternPayload(includeText: string, excludeText: string): NotificationPatternFilter {
  const include = notificationTextToList(includeText);
  return {
    include: include.length > 0 ? include : ['*'],
    exclude: notificationTextToList(excludeText),
  };
}

function notificationListToText(values?: string[]): string {
  return (values || []).join('\n');
}

function notificationTextToList(value: string): string[] {
  const seen = new Set<string>();
  return value
    .split(/[,\n]/)
    .map(item => item.trim())
    .filter(Boolean)
    .filter(item => {
      const key = item.toLowerCase();
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function normalizeStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === 'string').map(item => item.trim()).filter(Boolean);
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {};
}
