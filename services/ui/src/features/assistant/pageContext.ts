import { decodeTeamRouteSegments, splitRoutePath } from '../../lib/teamRoutes.js';

export type AssistantPageContext = {
  title: string;
  path: string;
  route: string;
  area: string;
  tab: string;
  team_path: string;
  resource_type: string;
  resource_id: string;
  resource_name: string;
  scope: string;
  pipeline_id: string;
  run_id: string;
  repository: string;
  query: Record<string, string>;
  params: Record<string, string>;
};

const allowedQueryKeys = new Set([
  'area',
  'credential',
  'dashboard',
  'id',
  'owner',
  'pipeline',
  'profile',
  'provider',
  'q',
  'resource',
  'run',
  'runid',
  'schedule',
  'scope',
  'source',
  'status',
  'tab',
  'team',
  'view',
]);

const allowedParamKeys = new Set([
  'dashboard_id',
  'pipeline_id',
  'repository',
  'resource_id',
  'resource_type',
  'run_id',
  'schedule_id',
  'scope',
  'tab',
  'team_path',
]);

const pageTitles: Record<string, string> = {
  pipelineruns: 'Pipeline runs',
  monitoring: 'Monitoring',
  dashboards: 'Dashboards',
  teams: 'Teams',
  assistant: 'Assistant',
  'llm-profiles': 'LLM profiles',
  'agent-profiles': 'Agent profiles',
  mcp: 'MCP',
  credentials: 'Credentials',
  docs: 'Docs',
  pipelines: 'Pipelines',
  schedules: 'Schedules',
  triggers: 'Triggers',
  'external-triggers': 'External triggers',
  'git-webhook-sources': 'Git webhook sources',
  scopes: 'Scopes',
  lab: 'Lab',
  steps: 'Steps',
  'knowledge-context': 'Knowledge context',
  system: 'System',
  profile: 'Profile',
};

export const emptyAssistantPageContext: AssistantPageContext = {
  title: '',
  path: '',
  route: '',
  area: '',
  tab: '',
  team_path: '',
  resource_type: '',
  resource_id: '',
  resource_name: '',
  scope: '',
  pipeline_id: '',
  run_id: '',
  repository: '',
  query: {},
  params: {},
};

export function buildAssistantPageContext(pathname: string, search = ''): AssistantPageContext {
  const path = normalizePath(pathname);
  const segments = splitRoutePath(path);
  const area = normalizeToken(segments[0] || 'pipelineruns');
  const query = normalizeContextRecord(readAllowedQuery(search));
  const context: AssistantPageContext = {
    ...emptyAssistantPageContext,
    title: pageTitles[area] || titleFromArea(area),
    path,
    area,
    route: area ? `/${area}` : '/',
    query,
  };

  switch (area) {
    case 'pipelineruns':
      applyPipelineRunsContext(context, segments, query);
      break;
    case 'pipelines':
      applyRoutedResourceContext(context, segments, query, 'pipeline', '/pipelines/:pipeline_id');
      break;
    case 'steps':
      applyRoutedResourceContext(context, segments, query, 'step', '/steps/:step_id');
      break;
    case 'scopes':
      applyScopeContext(context, segments, query);
      break;
    case 'schedules':
      applyPathResourceContext(context, segments, query.schedule || '', 'schedule', '/schedules/:schedule_id');
      context.pipeline_id = query.pipeline || '';
      context.scope = query.scope || query.team || '';
      break;
    case 'dashboards':
      applyPathResourceContext(context, segments, query.dashboard || '', 'dashboard', '/dashboards/:dashboard_id');
      context.tab = query.tab || '';
      break;
    case 'llm-profiles':
      applyPathResourceContext(context, segments, query.profile || '', 'llm_profile', '/llm-profiles/:profile_id');
      break;
    case 'agent-profiles':
      applyPathResourceContext(context, segments, query.profile || '', 'agent_profile', '/agent-profiles/:profile_id');
      break;
    case 'mcp':
      applyMCPContext(context, segments, query);
      break;
    case 'credentials':
      applyCredentialContext(context, segments, query);
      break;
    case 'triggers':
      applyRoutedResourceContext(context, segments, query, 'trigger', '/triggers/:trigger_id');
      context.team_path = query.owner || context.team_path;
      break;
    case 'external-triggers':
      applyRoutedResourceContext(context, segments, query, 'external_trigger', '/external-triggers/:trigger_id');
      break;
    case 'git-webhook-sources':
      applyRoutedResourceContext(context, segments, query, 'git_webhook_source', '/git-webhook-sources/:source_id');
      break;
    case 'knowledge-context':
      applyKnowledgeContext(context, segments, query);
      break;
    case 'lab':
      context.route = '/lab';
      context.resource_type = 'lab';
      context.pipeline_id = query.pipeline || '';
      context.scope = query.scope || '';
      break;
    case 'system':
      context.tab = normalizePathSegment(segments[1] || query.tab || '');
      context.route = context.tab ? '/system/:tab' : '/system';
      context.resource_type = context.tab ? `system_${context.tab.replace(/-/g, '_')}` : 'system';
      break;
    case 'monitoring':
      context.tab = normalizePathSegment(segments[1] || query.tab || '');
      context.route = context.tab ? '/monitoring/:tab' : '/monitoring';
      context.resource_type = 'monitoring';
      context.pipeline_id = query.pipeline || '';
      context.run_id = query.runid || query.run || '';
      context.resource_id = context.run_id;
      context.scope = query.team || query.scope || '';
      break;
    default:
      applyGenericContext(context, segments, query);
      break;
  }

  if (!context.scope) context.scope = context.team_path;
  context.params = normalizeContextRecord({
    tab: context.tab,
    team_path: context.team_path,
    resource_type: context.resource_type,
    resource_id: context.resource_id,
    scope: context.scope,
    pipeline_id: context.pipeline_id,
    run_id: context.run_id,
    repository: context.repository,
  });
  return normalizeAssistantPageContext(context);
}

export function normalizeAssistantPageContext(value?: Partial<AssistantPageContext> | null): AssistantPageContext {
  const source = value || {};
  const context: AssistantPageContext = {
    title: trimContextValue(source.title),
    path: normalizePath(source.path || ''),
    route: normalizePath(source.route || ''),
    area: normalizeToken(source.area || ''),
    tab: normalizeToken(source.tab || ''),
    team_path: normalizePathValue(source.team_path || ''),
    resource_type: normalizeToken(source.resource_type || ''),
    resource_id: normalizePathValue(source.resource_id || ''),
    resource_name: trimContextValue(source.resource_name),
    scope: normalizePathValue(source.scope || ''),
    pipeline_id: normalizePathValue(source.pipeline_id || ''),
    run_id: trimContextValue(source.run_id),
    repository: normalizePathValue(source.repository || ''),
    query: normalizeAllowedQueryRecord(source.query),
    params: normalizeAllowedParamRecord(source.params),
  };
  if (!context.resource_name) context.resource_name = resourceName(context.resource_id);
  if (!context.scope) context.scope = context.team_path;
  return context;
}

export function assistantPageContextIsEmpty(value?: Partial<AssistantPageContext> | null): boolean {
  const context = normalizeAssistantPageContext(value);
  return !context.title &&
    !context.path &&
    !context.route &&
    !context.area &&
    !context.tab &&
    !context.team_path &&
    !context.resource_type &&
    !context.resource_id &&
    !context.scope &&
    !context.pipeline_id &&
    !context.run_id &&
    !context.repository &&
    Object.keys(context.query).length === 0 &&
    Object.keys(context.params).length === 0;
}

export function assistantPageContextLabel(value?: Partial<AssistantPageContext> | null): string {
  const context = normalizeAssistantPageContext(value);
  const parts = [
    context.title,
    context.resource_name || context.resource_id || context.run_id || context.pipeline_id,
    context.tab,
    context.scope ? `/${context.scope}` : '',
  ].filter(Boolean);
  return unique(parts).join(' · ');
}

export function assistantPageContextScope(value?: Partial<AssistantPageContext> | null): string {
  const context = normalizeAssistantPageContext(value);
  return context.scope || context.team_path;
}

export function assistantPageContextKey(value?: Partial<AssistantPageContext> | null): string {
  const context = normalizeAssistantPageContext(value);
  if (assistantPageContextIsEmpty(context)) return '';
  return JSON.stringify({
    title: context.title,
    path: context.path,
    route: context.route,
    area: context.area,
    tab: context.tab,
    team_path: context.team_path,
    resource_type: context.resource_type,
    resource_id: context.resource_id,
    resource_name: context.resource_name,
    scope: context.scope,
    pipeline_id: context.pipeline_id,
    run_id: context.run_id,
    repository: context.repository,
    query: sortedContextRecord(context.query),
    params: sortedContextRecord(context.params),
  });
}

function applyPipelineRunsContext(context: AssistantPageContext, segments: string[], query: Record<string, string>) {
  const tab = normalizeToken(segments[1] || query.tab || 'main');
  context.tab = tab === 'recent' || tab === 'events' ? tab : 'main';
  context.route = '/pipelineruns/:tab';
  if (segments[2] === 'team') {
    context.team_path = decodeTeamRouteSegments(segments.slice(3));
    context.route = '/pipelineruns/:tab/team/:team_path';
  } else if (segments[2]) {
    context.run_id = normalizePathSegment(segments[2]);
    context.resource_id = context.run_id;
    context.route = '/pipelineruns/:tab/:run_id';
  }
  if (query.run) {
    context.run_id = query.run;
    context.resource_id = query.run;
  }
  context.team_path = context.team_path || query.team || '';
  context.resource_type = context.run_id ? 'pipeline_run' : 'pipeline_runs';
}

function applyRoutedResourceContext(
  context: AssistantPageContext,
  segments: string[],
  query: Record<string, string>,
  resourceType: string,
  selectedRoute: string,
) {
  const isTeamRoute = segments[1] === 'team';
  context.team_path = isTeamRoute ? decodeTeamRouteSegments(segments.slice(2)) : query.team || '';
  if (isTeamRoute) {
    context.route = `/${context.area}/team/:team_path`;
    context.resource_type = `${resourceType}s`;
    return;
  }
  const identifier = decodeTeamRouteSegments(segments.slice(1));
  context.route = identifier ? selectedRoute : `/${context.area}`;
  context.resource_type = identifier ? resourceType : `${resourceType}s`;
  context.resource_id = identifier;
  context.resource_name = resourceName(identifier);
  if (resourceType === 'pipeline') {
    context.pipeline_id = identifier;
    context.team_path = context.team_path || parentPath(identifier);
  }
  if (resourceType === 'step') context.team_path = context.team_path || parentPath(identifier);
}

function applyPathResourceContext(
  context: AssistantPageContext,
  segments: string[],
  legacyIdentifier: string,
  resourceType: string,
  selectedRoute: string,
) {
  const identifier = decodeTeamRouteSegments(segments.slice(1)) || legacyIdentifier;
  context.route = identifier ? selectedRoute : `/${context.area}`;
  context.resource_type = identifier ? resourceType : `${resourceType}s`;
  context.resource_id = identifier;
  context.resource_name = resourceName(identifier);
  context.team_path = context.team_path || parentPath(identifier);
}

function applyMCPContext(context: AssistantPageContext, segments: string[], query: Record<string, string>) {
  const view = segments[1] === 'profiles' || segments[1] === 'servers'
    ? segments[1]
    : normalizeToken(query.view || query.tab || 'servers');
  const identifier = view === segments[1] ? decodeTeamRouteSegments(segments.slice(2)) : '';
  context.tab = view === 'profiles' ? 'profiles' : 'servers';
  if (identifier) {
    context.route = context.tab === 'profiles' ? '/mcp/profiles/:profile_id' : '/mcp/servers/:server_id';
    context.resource_type = context.tab === 'profiles' ? 'mcp_profile' : 'mcp_server';
    context.resource_id = identifier;
    context.resource_name = resourceName(identifier);
    context.team_path = parentPath(identifier);
    return;
  }
  context.route = context.tab === 'profiles' ? '/mcp/profiles' : '/mcp/servers';
  context.resource_type = context.tab === 'profiles' ? 'mcp_profiles' : 'mcp_servers';
}

function applyCredentialContext(context: AssistantPageContext, segments: string[], query: Record<string, string>) {
  const routeReferencePath = decodeTeamRouteSegments(segments.slice(1));
  const reference = routeReferencePath ? `credential://${routeReferencePath}` : query.credential || '';
  context.route = reference ? '/credentials/:credential_ref' : '/credentials';
  context.resource_type = reference ? 'credential' : 'credentials';
  context.resource_id = reference;
  context.resource_name = resourceName(reference.replace(/^credential:\/\//i, ''));
  const referencePath = routeReferencePath || reference.replace(/^credential:\/\//i, '');
  const parts = referencePath.split('/').filter(Boolean);
  context.team_path = parts[0] === 'team' ? parts.slice(1, -1).join('/') : '';
}

function applyScopeContext(context: AssistantPageContext, segments: string[], query: Record<string, string>) {
  const isTeamRoute = segments[1] === 'team';
  context.team_path = isTeamRoute ? decodeTeamRouteSegments(segments.slice(2)) : query.team || '';
  if (isTeamRoute) {
    context.route = '/scopes/team/:team_path';
    context.resource_type = 'scopes';
    return;
  }
  const scope = decodeTeamRouteSegments(segments.slice(1));
  context.route = scope ? '/scopes/:scope' : '/scopes';
  context.resource_type = scope ? 'scope' : 'scopes';
  context.scope = scope || query.scope || context.team_path;
  context.resource_id = scope;
  context.resource_name = scope ? `/${scope}` : '';
}

function applyKnowledgeContext(context: AssistantPageContext, segments: string[], query: Record<string, string>) {
  const isTeamRoute = segments[1] === 'team';
  context.team_path = isTeamRoute ? decodeTeamRouteSegments(segments.slice(2)) : query.team || '';
  if (isTeamRoute) {
    context.route = '/knowledge-context/team/:team_path';
    context.resource_type = 'knowledge_contexts';
    return;
  }
  const identifier = decodeTeamRouteSegments(segments.slice(1));
  context.route = identifier ? '/knowledge-context/:document_id' : '/knowledge-context';
  context.resource_type = identifier ? 'knowledge_context' : 'knowledge_contexts';
  context.resource_id = identifier;
  context.resource_name = resourceName(identifier);
  context.team_path = context.team_path || parentPath(identifier);
  context.tab = query.tab || '';
}

function applyGenericContext(context: AssistantPageContext, segments: string[], query: Record<string, string>) {
  const identifier = decodeTeamRouteSegments(segments.slice(1));
  context.route = identifier && context.area ? `/${context.area}/:id` : context.route;
  context.resource_type = identifier ? context.area.replace(/-/g, '_') : context.area;
  context.resource_id = identifier || query.id || query.resource || '';
  context.resource_name = resourceName(context.resource_id);
  context.team_path = query.team || '';
  context.scope = query.scope || context.team_path;
}

function readAllowedQuery(search: string): Record<string, string> {
  const query: Record<string, string> = {};
  const params = new URLSearchParams(search.startsWith('?') ? search.slice(1) : search);
  params.forEach((value, key) => {
    const normalizedKey = normalizeToken(key);
    if (!allowedQueryKeys.has(normalizedKey)) return;
    const normalizedValue = trimContextValue(value);
    if (normalizedValue) query[normalizedKey] = normalizedValue;
  });
  return query;
}

function normalizeContextRecord(value: unknown): Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const out: Record<string, string> = {};
  Object.entries(value as Record<string, unknown>).slice(0, 16).forEach(([key, raw]) => {
    const normalizedKey = normalizeToken(key);
    const normalizedValue = trimContextValue(String(raw ?? ''));
    if (normalizedKey && normalizedValue) out[normalizedKey] = normalizedValue;
  });
  return out;
}

function normalizeAllowedQueryRecord(value: unknown): Record<string, string> {
  return normalizeAllowedContextRecord(value, allowedQueryKeys);
}

function normalizeAllowedParamRecord(value: unknown): Record<string, string> {
  return normalizeAllowedContextRecord(value, allowedParamKeys);
}

function normalizeAllowedContextRecord(value: unknown, allowedKeys: Set<string>): Record<string, string> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
  const out: Record<string, string> = {};
  for (const [key, raw] of Object.entries(value as Record<string, unknown>)) {
    const normalizedKey = normalizeToken(key);
    if (!allowedKeys.has(normalizedKey)) continue;
    const normalizedValue = trimContextValue(String(raw ?? ''));
    if (!normalizedValue) continue;
    out[normalizedKey] = normalizedValue;
    if (Object.keys(out).length >= 16) break;
  }
  return out;
}

function sortedContextRecord(record: Record<string, string>): Record<string, string> {
  return Object.fromEntries(Object.entries(record).sort(([left], [right]) => left.localeCompare(right)));
}

function normalizePath(value: string) {
  const normalized = trimContextValue(value).replace(/\/+/g, '/');
  if (!normalized) return '';
  return normalized.startsWith('/') ? normalized : `/${normalized}`;
}

function normalizePathValue(value: string) {
  const trimmed = trimContextValue(value);
  const schemeMatch = trimmed.match(/^([a-z][a-z0-9+.-]*:\/\/)(.*)$/i);
  if (schemeMatch) {
    return `${schemeMatch[1].toLowerCase()}${schemeMatch[2].replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '')}`;
  }
  return trimmed.replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '');
}

function normalizePathSegment(value: string) {
  try {
    return trimContextValue(decodeURIComponent(value));
  } catch {
    return trimContextValue(value);
  }
}

function normalizeToken(value: string) {
  return trimContextValue(value).toLowerCase().replace(/[^a-z0-9_-]+/g, '_').replace(/^_+|_+$/g, '');
}

function trimContextValue(value?: string) {
  return String(value || '').trim().replace(/\s+/g, ' ').slice(0, 240);
}

function parentPath(identifier: string) {
  const parts = identifier.split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function resourceName(identifier: string) {
  const parts = identifier.split('/').filter(Boolean);
  return parts.at(-1) || '';
}

function titleFromArea(area: string) {
  return area.split('-').filter(Boolean).map(part => part.charAt(0).toUpperCase() + part.slice(1)).join(' ');
}

function unique(values: string[]) {
  const seen = new Set<string>();
  return values.filter(value => {
    if (seen.has(value)) return false;
    seen.add(value);
    return true;
  });
}
