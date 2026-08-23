import { apiClient } from '../../lib/api.js';
import {
  emptyAssistantPageContext,
  normalizeAssistantPageContext,
  type AssistantPageContext,
} from './pageContext.js';

/**
 * The resources a chat can be pointed at. On a resource page the assistant picks
 * its context up from the route, but the assistant page has no resource of its
 * own, so the target has to be chosen in the composer instead.
 */
export type AssistantContextKind = 'pipeline' | 'pipeline_run' | 'scope' | 'team' | 'schedule';

export type AssistantContextOption = {
  kind: AssistantContextKind;
  /** Identifier the backend tools accept: a pipeline id, run id, scope, team path, or schedule id. */
  id: string;
  label: string;
  detail: string;
  /** Owning path, used to scope the context and to disambiguate similar names. */
  scope: string;
  /** Pipeline the resource belongs to, for runs and schedules. */
  pipeline: string;
};

export const assistantContextKinds: Array<{ kind: AssistantContextKind; label: string }> = [
  { kind: 'pipeline', label: 'Pipelines' },
  { kind: 'pipeline_run', label: 'Runs' },
  { kind: 'scope', label: 'Scopes' },
  { kind: 'team', label: 'Teams' },
  { kind: 'schedule', label: 'Schedules' },
];

const assistantContextOptionLimit = 200;

export async function fetchAssistantContextOptions(kind: AssistantContextKind): Promise<AssistantContextOption[]> {
  switch (kind) {
    case 'pipeline':
      return pipelineContextOptions(await readList('/v1/pipelines'));
    case 'pipeline_run':
      return runContextOptions(await readList('/v1/runs?limit=100'));
    case 'scope': {
      const [secrets, variables] = await Promise.all([readList('/v1/secrets/scopes'), readList('/v1/variables/scopes')]);
      return scopeContextOptions([...secrets, ...variables]);
    }
    case 'team':
      return teamContextOptions(await readList('/v1/teams'));
    case 'schedule':
      return scheduleContextOptions(await readList('/v1/schedules'));
  }
}

/** Matches on every visible part of a row, so "prod deploy" finds a scoped pipeline. */
export function filterAssistantContextOptions(options: AssistantContextOption[], query: string): AssistantContextOption[] {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return options;
  return options.filter(option => {
    const haystack = `${option.id} ${option.label} ${option.detail} ${option.scope}`.toLowerCase();
    return terms.every(term => haystack.includes(term));
  });
}

/**
 * Turns a picked resource into the same page-context shape a route produces, so
 * the planner grounds on it exactly as it does when the chat is opened from that
 * resource's own page.
 */
export function assistantPageContextFromOption(option: AssistantContextOption): AssistantPageContext {
  const base: AssistantPageContext = { ...emptyAssistantPageContext };
  switch (option.kind) {
    case 'pipeline':
      Object.assign(base, {
        title: 'Pipelines',
        area: 'pipelines',
        path: `/pipelines/${option.id}`,
        route: '/pipelines/:pipeline_id',
        resource_type: 'pipeline',
        resource_id: option.id,
        resource_name: option.label,
        pipeline_id: option.id,
        team_path: option.scope,
        scope: option.scope,
      });
      break;
    case 'pipeline_run':
      Object.assign(base, {
        title: 'Pipeline runs',
        area: 'pipelineruns',
        path: `/pipelineruns/main/${option.id}`,
        route: '/pipelineruns/:tab/:run_id',
        tab: 'main',
        resource_type: 'pipeline_run',
        resource_id: option.id,
        resource_name: option.label,
        run_id: option.id,
        pipeline_id: option.pipeline,
        team_path: option.scope,
        scope: option.scope,
      });
      break;
    case 'scope':
      Object.assign(base, {
        title: 'Scopes',
        area: 'scopes',
        path: `/scopes/${option.id}`,
        route: '/scopes/:scope',
        resource_type: 'scope',
        resource_id: option.id,
        resource_name: `/${option.id}`,
        scope: option.id,
      });
      break;
    case 'team':
      Object.assign(base, {
        title: 'Teams',
        area: 'teams',
        path: `/teams/${option.id}`,
        route: '/teams/:team_path',
        resource_type: 'team',
        resource_id: option.id,
        resource_name: option.label,
        team_path: option.id,
        scope: option.id,
      });
      break;
    case 'schedule':
      Object.assign(base, {
        title: 'Schedules',
        area: 'schedules',
        path: `/schedules/${option.id}`,
        route: '/schedules/:schedule_id',
        resource_type: 'schedule',
        resource_id: option.id,
        resource_name: option.label,
        pipeline_id: option.pipeline,
        team_path: option.scope,
        scope: option.scope,
      });
      break;
  }
  const context = normalizeAssistantPageContext(base);
  context.params = contextParams(context);
  return context;
}

function contextParams(context: AssistantPageContext): Record<string, string> {
  const params: Record<string, string> = {};
  const put = (key: string, value: string) => {
    if (value) params[key] = value;
  };
  put('tab', context.tab);
  put('team_path', context.team_path);
  put('resource_type', context.resource_type);
  put('resource_id', context.resource_id);
  put('scope', context.scope);
  put('pipeline_id', context.pipeline_id);
  put('run_id', context.run_id);
  return params;
}

async function readList(path: string): Promise<unknown[]> {
  const response = await apiClient.fetch(path, { cache: 'no-store' });
  if (!response.ok) throw new Error(`Failed to load context options (${response.status})`);
  const payload = await response.json();
  if (Array.isArray(payload)) return payload;
  for (const key of ['items', 'pipelines', 'runs', 'scopes', 'teams', 'schedules']) {
    const nested = (payload as Record<string, unknown>)?.[key];
    if (Array.isArray(nested)) return nested;
  }
  return [];
}

function pipelineContextOptions(rows: unknown[]): AssistantContextOption[] {
  return dedupe(rows.map(row => {
    const identifier = trimPath(
      typeof row === 'string'
        ? row
        : joinIdentifier(readString(row, 'path'), readString(row, 'name')) || readString(row, 'identifier') || readString(row, 'id')
    );
    if (!identifier) return null;
    return {
      kind: 'pipeline' as const,
      id: identifier,
      label: leafName(identifier),
      detail: identifier,
      scope: parentPath(identifier),
      pipeline: identifier,
    };
  }));
}

function runContextOptions(rows: unknown[]): AssistantContextOption[] {
  return dedupe(rows.map(row => {
    const runID = readString(row, 'run_id') || readString(row, 'id');
    if (!runID) return null;
    const pipeline = trimPath(joinIdentifier(readString(row, 'pipeline_path'), readString(row, 'pipeline_name')));
    return {
      kind: 'pipeline_run' as const,
      id: runID,
      label: pipeline ? `${leafName(pipeline)} · ${shortRunID(runID)}` : shortRunID(runID),
      // The run's own page shows status next to the pipeline, so the picker does
      // too: it is what tells two runs of the same pipeline apart.
      detail: [pipeline, readString(row, 'status')].filter(Boolean).join(' · '),
      scope: parentPath(pipeline),
      pipeline,
    };
  }), true);
}

function scopeContextOptions(rows: unknown[]): AssistantContextOption[] {
  return dedupe(rows.map(row => {
    const scope = trimPath(typeof row === 'string' ? row : readString(row, 'scope') || readString(row, 'name') || readString(row, 'path'));
    if (!scope) return null;
    return { kind: 'scope' as const, id: scope, label: `/${scope}`, detail: '', scope, pipeline: '' };
  }));
}

function teamContextOptions(rows: unknown[]): AssistantContextOption[] {
  return dedupe(rows.map(row => {
    const path = trimPath(readString(row, 'path') || readString(row, 'team_path') || readString(row, 'name'));
    if (!path) return null;
    return { kind: 'team' as const, id: path, label: `/${path}`, detail: readString(row, 'description'), scope: path, pipeline: '' };
  }));
}

function scheduleContextOptions(rows: unknown[]): AssistantContextOption[] {
  return dedupe(rows.map(row => {
    const id = readString(row, 'id');
    if (!id) return null;
    const pipeline = trimPath(
      readString(row, 'pipeline') || joinIdentifier(readString(row, 'pipeline_path'), readString(row, 'pipeline_name'))
    );
    const name = readString(row, 'identifier') || readString(row, 'name') || id;
    return {
      kind: 'schedule' as const,
      id,
      label: name,
      detail: pipeline,
      scope: trimPath(readString(row, 'scope')) || parentPath(pipeline),
      pipeline,
    };
  }));
}

// Names are easiest to scan alphabetically, but runs are not: the one a user
// wants is almost always a recent one, so their API order is kept.
function dedupe(options: Array<AssistantContextOption | null>, keepOrder = false): AssistantContextOption[] {
  const seen = new Set<string>();
  const rows: AssistantContextOption[] = [];
  options.forEach(option => {
    if (!option || seen.has(option.id)) return;
    seen.add(option.id);
    rows.push(option);
  });
  if (!keepOrder) rows.sort((left, right) => left.label.localeCompare(right.label));
  return rows.slice(0, assistantContextOptionLimit);
}

function readString(row: unknown, key: string): string {
  if (!row || typeof row !== 'object') return '';
  const value = (row as Record<string, unknown>)[key];
  return typeof value === 'string' ? value.trim() : '';
}

function joinIdentifier(path: string, name: string): string {
  const left = trimPath(path);
  const right = trimPath(name);
  if (!right) return left;
  if (!left || right.includes('/')) return right;
  return `${left}/${right}`;
}

function trimPath(value: string): string {
  return value.trim().replace(/^\/+|\/+$/g, '');
}

function leafName(identifier: string): string {
  const parts = trimPath(identifier).split('/');
  return parts[parts.length - 1] || identifier;
}

function parentPath(identifier: string): string {
  const trimmed = trimPath(identifier);
  const index = trimmed.lastIndexOf('/');
  return index > 0 ? trimmed.slice(0, index) : '';
}

function shortRunID(runID: string): string {
  return runID.length > 8 ? `${runID.slice(0, 8)}…` : runID;
}
