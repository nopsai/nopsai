import { encodeId, splitIdentifier, type PipelineListItem } from '../pipelines/model.js';
import { encodeScopeForRoute, normalizeScopeLabel } from '../scopes/model.js';
import { encodeKnowledgeID, splitKnowledgePath, type KnowledgeContextListItem } from '../knowledge-context/model.js';
import {
  normalizeIdentifier as normalizeScheduleIdentifier,
  splitIdentifier as splitScheduleIdentifier,
  type PipelineSchedule,
} from '../schedules/model.js';
import { encodeId as encodeStepID, splitIdentifier as splitStepIdentifier } from '../steps/model.js';
import { encodeTriggerSlug, triggerSlugLabel, type TriggerListItem } from '../triggers/model.js';
import type { ExternalTrigger } from '../external-triggers/model.js';
import type { GitWebhookSource } from '../git-webhook-sources/model.js';
import {
  credentialReferenceRoute,
  parseCredentialReference,
  type CredentialRecord,
} from '../system/credentials/model.js';

export type TeamLinkedResourceKind =
  | 'application'
  | 'llm_profile'
  | 'agent_profile'
  | 'mcp_profile'
  | 'pipeline'
  | 'step'
  | 'trigger'
  | 'external_trigger'
  | 'git_webhook_source'
  | 'schedule'
  | 'knowledge_context'
  | 'scope'
  | 'credential';

export type TeamLinkedResource = {
  id: string;
  kind: TeamLinkedResourceKind;
  label: string;
  description: string;
  href: string;
  teamPath: string;
  source?: string;
};

export type TeamResourceCatalogState = {
  teamPath: string;
  loading: boolean;
  error: string | null;
  resources: TeamLinkedResource[];
};

export const TEAM_LINKED_RESOURCE_LABELS: Record<TeamLinkedResourceKind, string> = {
  application: 'Applications',
  llm_profile: 'LLM Profiles',
  agent_profile: 'Agent Profiles',
  mcp_profile: 'MCP Profiles',
  pipeline: 'Pipelines',
  step: 'Steps',
  trigger: 'Triggers',
  external_trigger: 'External Triggers',
  git_webhook_source: 'Git Webhook Sources',
  schedule: 'Schedules',
  knowledge_context: 'Knowledge Context',
  scope: 'Scopes',
  credential: 'Credentials',
};

const RESOURCE_KIND_ORDER: TeamLinkedResourceKind[] = [
  'application',
  'llm_profile',
  'agent_profile',
  'mcp_profile',
  'pipeline',
  'step',
  'trigger',
  'external_trigger',
  'git_webhook_source',
  'schedule',
  'knowledge_context',
  'scope',
  'credential',
];

export function normalizeTeamResourcePath(value: unknown): string {
  const normalized = String(value ?? '').trim().replace(/\/+/g, '/').replace(/^\/+|\/+$/g, '');
  return normalized.toLowerCase() === 'root' ? '' : normalized;
}

export function teamResourceBelongsToScope(resourceTeamPath: string, activeTeamPath: string): boolean {
  const resourcePath = normalizeTeamResourcePath(resourceTeamPath);
  const activePath = normalizeTeamResourcePath(activeTeamPath);
  if (!activePath) return !resourcePath;
  if (!resourcePath) return true;

  const resourceKey = resourcePath.toLowerCase();
  const activeKey = activePath.toLowerCase();
  return resourceKey === activeKey || resourceKey.startsWith(`${activeKey}/`);
}

export function buildPipelineTeamResources(items: PipelineListItem[]): TeamLinkedResource[] {
  return items
    .map(item => {
      const { name, path } = splitIdentifier(item.id);
      const teamPath = normalizeTeamResourcePath(path);
      const label = name || item.id;
      return {
        id: `pipeline:${item.id}`,
        kind: 'pipeline' as const,
        label,
        description: teamPath ? `Pipeline in ${teamPath}` : 'Global pipeline',
        href: `/pipelines/${encodeId(item.id)}`,
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildTriggerTeamResources(items: TriggerListItem[]): TeamLinkedResource[] {
  return items
    .map(item => {
      const { name, path } = triggerSlugLabel(item.slug);
      const teamPath = normalizeTeamResourcePath(path);
      const label = name || item.slug;
      return {
        id: `trigger:${item.slug}`,
        kind: 'trigger' as const,
        label,
        description: teamPath ? `Trigger in ${teamPath}` : 'Global trigger',
        href: `/triggers/${encodeTriggerSlug(item.slug)}`,
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildExternalTriggerTeamResources(items: ExternalTrigger[]): TeamLinkedResource[] {
  return items
    .map(item => {
      const { name, path } = splitRouteIdentifier(item.id);
      const teamPath = publicResourceTeamPath(item.run_team_path || path);
      const label = item.name || name || item.id;
      return {
        id: `external_trigger:${item.id}`,
        kind: 'external_trigger' as const,
        label,
        description: [
          item.enabled ? 'Enabled external trigger' : 'Disabled external trigger',
          item.pipeline,
          teamPath ? `runs in ${teamPath}` : 'global',
        ].filter(Boolean).join(' / '),
        href: `/external-triggers/${encodeRouteIdentifier(item.id)}`,
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildGitWebhookSourceTeamResources(items: GitWebhookSource[]): TeamLinkedResource[] {
  return items
    .map(item => {
      const allowlistCount = item.repository_allowlist?.length ?? 0;
      const teamPath = publicResourceTeamPath(item.team_path || item.run_team_path || '');
      return {
        id: `git_webhook_source:${item.id}`,
        kind: 'git_webhook_source' as const,
        label: item.name || item.id,
        description: [
          item.enabled ? 'Enabled webhook source' : 'Disabled webhook source',
          item.provider,
          `${allowlistCount} allowed repos`,
        ].filter(Boolean).join(' / '),
        href: `/git-webhook-sources/${encodeURIComponent(item.id)}`,
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildStepTeamResources(items: Array<{ id: string; source?: string }>): TeamLinkedResource[] {
  return items
    .map(item => {
      const { name, path } = splitStepIdentifier(item.id);
      const teamPath = normalizeTeamResourcePath(path);
      const label = name || item.id;
      return {
        id: `step:${item.id}`,
        kind: 'step' as const,
        label,
        description: teamPath ? `Step in ${teamPath}` : 'Global step',
        href: `/steps/${encodeStepID(item.id)}`,
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildScheduleTeamResources(items: PipelineSchedule[]): TeamLinkedResource[] {
  return items
    .map(item => {
      const identifier = item.identifier || item.path || item.id;
      const { name, path } = splitScheduleIdentifier(identifier);
      const runTeamPath = item.run_team_path || path;
      const teamPath = publicResourceTeamPath(runTeamPath, item.visibility);
      const pipeline = normalizeScheduleIdentifier(item.pipeline_path || item.pipeline);
      const label = item.name || name || item.id;
      return {
        id: `schedule:${item.id}`,
        kind: 'schedule' as const,
        label,
        description: [
          item.enabled ? 'Enabled schedule' : 'Disabled schedule',
          item.schedule_kind || 'cron',
          pipeline ? `pipeline ${pipeline}` : '',
          teamPath ? `runs in ${teamPath}` : 'global',
        ].filter(Boolean).join(' / '),
        href: pipeline ? `/schedules?pipeline=${encodeURIComponent(pipeline)}` : '/schedules',
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildKnowledgeContextTeamResources(items: KnowledgeContextListItem[]): TeamLinkedResource[] {
  return items
    .map(item => {
      const fallback = splitKnowledgePath(item.id);
      const teamPath = publicResourceTeamPath(item.team || fallback.team, item.visibility);
      return {
        id: `knowledge_context:${item.id}`,
        kind: 'knowledge_context' as const,
        label: item.name || fallback.name || item.id,
        description: [
          item.kind,
          item.visibility?.toLowerCase() === 'public' ? 'public' : teamPath ? `owned by ${teamPath}` : 'global',
          item.description,
        ].filter(Boolean).join(' / '),
        href: `/knowledge-context/${encodeKnowledgeID(item.id)}`,
        teamPath,
        source: item.source,
      };
    })
    .filter(resource => resource.label);
}

export function buildScopeTeamResources(catalogs: { secrets: unknown; variables: unknown }): TeamLinkedResource[] {
  const secretCounts = new Map<string, number>();
  if (Array.isArray(catalogs.secrets)) {
    catalogs.secrets.forEach(entry => {
      if (!entry || typeof entry !== 'object') return;
      const record = entry as Record<string, unknown>;
      const scope = normalizeScopeLabel(record.scope);
      const count = typeof record.secret_count === 'number' ? record.secret_count : 0;
      secretCounts.set(scope, count);
    });
  }

  const scopeSet = new Set<string>();
  scopeSet.add('');
  if (Array.isArray(catalogs.variables)) {
    catalogs.variables.forEach(entry => {
      if (typeof entry === 'string') {
        scopeSet.add(normalizeScopeLabel(entry));
        return;
      }
      if (!entry || typeof entry !== 'object') return;
      const record = entry as Record<string, unknown>;
      scopeSet.add(normalizeScopeLabel(record.scope ?? record.name ?? record.value));
    });
  }
  secretCounts.forEach((_, scope) => scopeSet.add(scope));

  return Array.from(scopeSet).map(scope => {
    const teamPath = normalizeTeamResourcePath(scope);
    const parts = teamPath.split('/').filter(Boolean);
    const label = parts.at(-1) || 'Default Scope';
    const secretCount = secretCounts.get(teamPath) || 0;
    const description = teamPath
      ? `${secretCount} secret${secretCount === 1 ? '' : 's'} in ${teamPath}`
      : `${secretCount} global secret${secretCount === 1 ? '' : 's'}`;
    return {
      id: `scope:${teamPath || 'default'}`,
      kind: 'scope' as const,
      label,
      description,
      href: `/scopes/${encodeScopeForRoute(teamPath)}`,
      teamPath,
    };
  });
}

export function buildCredentialTeamResources(credentials: CredentialRecord[]): TeamLinkedResource[] {
  return credentials
    .map(credential => {
      const reference = parseCredentialReference(credential.reference);
      const teamPath = credentialTeamPath(reference.namespace, reference.name);
      const label = reference.displayName || credential.reference;
      const description = teamPath
        ? `${credential.kind || 'credential'} in ${teamPath}`
        : `${reference.namespace || 'system'} ${credential.kind || 'credential'}`;
      return {
        id: `credential:${credential.reference}`,
        kind: 'credential' as const,
        label,
        description,
        href: credentialReferenceRoute(credential.reference),
        teamPath,
        source: credential.managed_by_config_repo ? 'git' : 'database',
      };
    })
    .filter(resource => resource.label);
}

export function filterTeamLinkedResources(
  resources: TeamLinkedResource[],
  activeTeamPath: string
): TeamLinkedResource[] {
  const activePath = normalizeTeamResourcePath(activeTeamPath);
  return resources
    .filter(resource => {
      const resourcePath = normalizeTeamResourcePath(resource.teamPath);
      if (resource.kind === 'credential' && !resourcePath && activePath) return false;
      return teamResourceBelongsToScope(resourcePath, activePath);
    })
    .sort(compareTeamLinkedResources);
}

function credentialTeamPath(namespace: string, name: string): string {
  if (namespace.trim().toLowerCase() !== 'team') return '';
  const segments = name.split('/').filter(Boolean);
  segments.pop();
  return normalizeTeamResourcePath(segments.join('/'));
}

function publicResourceTeamPath(resourceTeamPath: string, visibility?: string): string {
  if (String(visibility || '').trim().toLowerCase() === 'public') return '';
  return normalizeTeamResourcePath(resourceTeamPath);
}

function splitRouteIdentifier(identifier: string) {
  const parts = String(identifier || '').split('/').filter(Boolean);
  const name = parts.pop() || '';
  return { name, path: parts.join('/') };
}

function encodeRouteIdentifier(identifier: string) {
  return String(identifier || '').split('/').filter(Boolean).map(encodeURIComponent).join('/');
}

function compareTeamLinkedResources(left: TeamLinkedResource, right: TeamLinkedResource): number {
  const kindCompare = RESOURCE_KIND_ORDER.indexOf(left.kind) - RESOURCE_KIND_ORDER.indexOf(right.kind);
  if (kindCompare !== 0) return kindCompare;
  const pathCompare = left.teamPath.localeCompare(right.teamPath, undefined, { sensitivity: 'base' });
  if (pathCompare !== 0) return pathCompare;
  return left.label.localeCompare(right.label, undefined, { sensitivity: 'base' });
}
