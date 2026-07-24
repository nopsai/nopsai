import type { AnalysisAiPromptContext } from '../analysis/ai.js';
import { compactPromptList, formatPromptTimestamp, redactAnalysisPromptText } from '../analysis/promptContext.js';
import { isAppTeam, teamPathForURL, teamRepositoryLabel, type Team } from '../../lib/teamModels.js';
import type { AnalyzableResource } from '../analysis/model.js';
import type { TeamScopeStats } from './model.js';
import { TEAM_LINKED_RESOURCE_LABELS, type TeamLinkedResourceKind, type TeamResourceCatalogState } from './resourceCatalogModel.js';

const MAX_RESOURCE_LINES = 60;
const MAX_PEER_LINES = 24;
const MAX_LINE_LENGTH = 700;

export type TeamAnalysisPromptContextInput = {
  team: Team | null;
  teams: Team[];
  stats: TeamScopeStats;
  subjectLabel: string;
  scopePath: string;
  resources: AnalyzableResource[];
  activeResource?: AnalyzableResource | null;
  operationsSummary: TeamAnalysisOperationsSummary;
  resourceCatalog: TeamResourceCatalogState;
};

type TeamAnalysisOperationsSummary = {
  teamPath: string;
  loading: boolean;
  configRepo: {
    provider?: string;
    repo_url?: string;
    branch?: string;
    base_path?: string;
    enabled?: boolean;
    write_enabled?: boolean;
    last_sync_status?: string;
    last_sync_completed_at?: string;
  } | null;
  configRepoError: string | null;
  notificationRoute: { definition?: { routes?: unknown[] } } | null;
  notificationError: string | null;
  llmProfiles: {
    default_profile?: string;
    profiles: Array<{
      name: string;
      provider?: string;
      model?: string;
      scope?: string;
      team_path?: string;
      status?: string;
      allowed_in_scope?: boolean;
    }>;
  } | null;
  agentProfiles: {
    default_profile?: string;
    profiles: Array<{
      id: string;
      display_name?: string;
      role?: string;
      scope?: string;
      team_path?: string;
      enabled?: boolean;
      source?: string;
    }>;
  } | null;
  mcpProfiles: {
    profiles: Array<{
      name: string;
      scope?: string;
      team_path?: string;
      enabled?: boolean;
      servers?: Array<{ server?: string }>;
    }>;
  } | null;
  aiProfilesError: string | null;
  accessGrants: unknown[];
  accessGrantsError: string | null;
  permissions: Array<{ action: string; allowed: boolean }>;
  permissionsError: string | null;
};

export function buildTeamAnalysisPromptContext(input: TeamAnalysisPromptContextInput): AnalysisAiPromptContext {
  const sections: AnalysisAiPromptContext['sections'] = [
    buildTeamScopeSection(input),
    buildTeamResourceDistributionSection(input.resources),
    buildTeamOperationsSection(input.operationsSummary),
    buildTeamVisibleResourcesSection(input.resources),
  ];
  if (input.activeResource) {
    sections.splice(1, 0, buildSelectedResourceSection(input.activeResource, input.resources));
  }
  return { sections };
}

function buildTeamScopeSection(input: TeamAnalysisPromptContextInput): AnalysisAiPromptContext['sections'][number] {
  const team = input.team;
  const app = Boolean(team && isAppTeam(team));
  const path = team ? teamPathForURL(team, input.teams) : '';
  const limitations = [
    input.resourceCatalog.loading ? 'Resource catalog was still loading when AI context was built.' : '',
    input.resourceCatalog.error ? `Resource catalog error: ${redactAnalysisPromptText(input.resourceCatalog.error, 400)}` : '',
    input.operationsSummary.loading ? 'Team operations summary was still loading when AI context was built.' : '',
    input.operationsSummary.aiProfilesError ? `AI profile summary error: ${redactAnalysisPromptText(input.operationsSummary.aiProfilesError, 400)}` : '',
    input.operationsSummary.accessGrantsError ? `Access grant summary error: ${redactAnalysisPromptText(input.operationsSummary.accessGrantsError, 400)}` : '',
    input.operationsSummary.permissionsError ? `Permission summary error: ${redactAnalysisPromptText(input.operationsSummary.permissionsError, 400)}` : '',
    input.operationsSummary.configRepoError ? `GitOps config error: ${redactAnalysisPromptText(input.operationsSummary.configRepoError, 400)}` : '',
    input.operationsSummary.notificationError ? `Notification policy error: ${redactAnalysisPromptText(input.operationsSummary.notificationError, 400)}` : '',
  ].filter(Boolean);

  return {
    title: 'Team/resource page snapshot',
    summary: 'Visible team or application metadata and loader state from the Teams page.',
    items: [
      { label: 'Subject', value: input.subjectLabel, kind: 'fact' },
      { label: 'Scope path', value: input.scopePath || 'Global', kind: 'fact' },
      { label: 'Selected resource mode', value: input.activeResource ? 'single resource' : 'team resource catalog', kind: 'fact' },
      { label: 'Team id', value: team ? String(team.id) : 'global', kind: 'fact' },
      { label: 'Team kind', value: team ? app ? 'application' : 'team' : 'global', kind: 'fact' },
      { label: 'Computed team path', value: path || 'Global', kind: 'fact' },
      { label: 'Description', value: redactAnalysisPromptText(team?.description || '-'), kind: 'redacted' },
      { label: 'Repository', value: team ? teamRepositoryLabel(team) || '-' : '-', kind: 'fact' },
      { label: 'Total teams/applications in scope', value: String(input.stats.totalItems), kind: 'metric' },
      { label: 'Teams in scope', value: String(input.stats.teams), kind: 'metric' },
      { label: 'Applications in scope', value: String(input.stats.applications), kind: 'metric' },
      { label: 'Repositories in scope', value: String(input.stats.repositories), kind: 'metric' },
      { label: 'Recent run markers', value: String(input.stats.recentRuns), kind: 'metric' },
      { label: 'Visible linked resources', value: String(input.resources.length), kind: 'metric' },
    ],
    limitations,
  };
}

function buildSelectedResourceSection(
  resource: AnalyzableResource,
  resources: AnalyzableResource[]
): AnalysisAiPromptContext['sections'][number] {
  const peers = resources
    .filter(item => item.kind === resource.kind && item.id !== resource.id)
    .slice(0, MAX_PEER_LINES);
  return {
    title: 'Selected resource context',
    summary: 'Focused metadata for the resource being analyzed plus visible same-kind peers for duplicate, ownership, and reuse checks.',
    items: [
      { label: 'Resource id', value: resource.id, kind: 'fact' },
      { label: 'Kind', value: resource.kind, kind: 'fact' },
      { label: 'Label', value: resource.label, kind: 'fact' },
      { label: 'Description', value: redactAnalysisPromptText(resource.description), kind: 'redacted' },
      { label: 'Team path', value: resource.teamPath || 'Global', kind: 'fact' },
      { label: 'Source', value: resource.source || '-', kind: 'fact' },
      { label: 'Route', value: resource.href || '-', kind: 'fact' },
      { label: 'Visible same-kind peers', value: String(peers.length), kind: 'metric' },
    ],
    lines: peers.map(formatResourceLine),
    limitations: resources.filter(item => item.kind === resource.kind && item.id !== resource.id).length > MAX_PEER_LINES
      ? [`Same-kind peer list was capped at ${MAX_PEER_LINES}.`]
      : [],
  };
}

function buildTeamResourceDistributionSection(resources: AnalyzableResource[]): AnalysisAiPromptContext['sections'][number] {
  const byKind = countBy(resources, resource => resource.kind);
  const bySource = countBy(resources, resource => resource.source || 'unknown');
  return {
    title: 'Visible resource distribution',
    summary: 'Counts derived from the resource rows visible to the current Teams view.',
    lines: [
      ...Array.from(byKind.entries())
        .sort((left, right) => kindSortLabel(left[0]).localeCompare(kindSortLabel(right[0]), undefined, { sensitivity: 'base' }))
        .map(([kind, count]) => `kind=${kindSortLabel(kind)} count=${count}`),
      ...Array.from(bySource.entries())
        .sort((left, right) => left[0].localeCompare(right[0], undefined, { sensitivity: 'base' }))
        .map(([source, count]) => `source=${source || 'unknown'} count=${count}`),
    ],
  };
}

function buildTeamOperationsSection(summary: TeamAnalysisOperationsSummary): AnalysisAiPromptContext['sections'][number] {
  const configRepo = summary.configRepo;
  const notificationRoutes = Array.isArray(summary.notificationRoute?.definition?.routes)
    ? summary.notificationRoute?.definition?.routes || []
    : [];
  const llmProfiles = summary.llmProfiles?.profiles || [];
  const agentProfiles = summary.agentProfiles?.profiles || [];
  const mcpProfiles = summary.mcpProfiles?.profiles || [];
  const allowedPermissions = summary.permissions.filter(permission => permission.allowed);
  return {
    title: 'Team operations metadata',
    summary: 'Visible GitOps, notification, AI profile, access, and permission state loaded by the Teams page.',
    items: [
      { label: 'Summary team path', value: summary.teamPath || 'Global', kind: 'fact' },
      { label: 'GitOps repo enabled', value: configRepo ? String(configRepo.enabled) : '-', kind: 'fact' },
      { label: 'GitOps provider', value: configRepo?.provider || '-', kind: 'fact' },
      { label: 'GitOps repository', value: redactAnalysisPromptText(configRepo?.repo_url || '-'), kind: 'redacted' },
      { label: 'GitOps branch', value: configRepo?.branch || '-', kind: 'fact' },
      { label: 'GitOps base path', value: configRepo?.base_path || '-', kind: 'fact' },
      { label: 'GitOps write enabled', value: configRepo ? String(configRepo.write_enabled) : '-', kind: 'fact' },
      { label: 'GitOps last sync', value: compactPromptList([configRepo?.last_sync_status, formatPromptTimestamp(configRepo?.last_sync_completed_at)], '-'), kind: 'fact' },
      { label: 'Notification routes', value: String(notificationRoutes.length), kind: 'metric' },
      { label: 'LLM profiles', value: String(llmProfiles.length), kind: 'metric' },
      { label: 'Default LLM profile', value: summary.llmProfiles?.default_profile || '-', kind: 'fact' },
      { label: 'Agent profiles', value: String(agentProfiles.length), kind: 'metric' },
      { label: 'Default agent profile', value: summary.agentProfiles?.default_profile || '-', kind: 'fact' },
      { label: 'MCP profiles', value: String(mcpProfiles.length), kind: 'metric' },
      { label: 'Access grants', value: String(summary.accessGrants.length), kind: 'metric' },
      { label: 'Allowed current-user permissions', value: compactPromptList(allowedPermissions.map(permission => permission.action)), kind: 'fact' },
    ],
    lines: [
      ...notificationRoutes.slice(0, 12).map((route, index) => `notification_route_${index + 1}=${redactAnalysisPromptText(JSON.stringify(route), 500)}`),
      ...llmProfiles.slice(0, 12).map(profile =>
        `llm_profile=${profile.name} provider=${profile.provider || '-'} model=${profile.model || '-'} scope=${profile.scope} team=${profile.team_path || 'global'} status=${profile.status || '-'} allowed=${profile.allowed_in_scope === false ? 'false' : 'true'}`
      ),
      ...agentProfiles.slice(0, 12).map(profile =>
        `agent_profile=${profile.id} display=${redactAnalysisPromptText(profile.display_name || profile.id, 160)} role=${profile.role || '-'} scope=${profile.scope} team=${profile.team_path || 'global'} enabled=${profile.enabled === false ? 'false' : 'true'} source=${profile.source || '-'}`
      ),
      ...mcpProfiles.slice(0, 12).map(profile =>
        `mcp_profile=${profile.name} scope=${profile.scope} team=${profile.team_path || 'global'} enabled=${profile.enabled === false ? 'false' : 'true'} servers=${compactPromptList((profile.servers || []).map(server => server.server))}`
      ),
    ],
    limitations: [
      notificationRoutes.length > 12 ? `${notificationRoutes.length - 12} notification route${notificationRoutes.length - 12 === 1 ? '' : 's'} omitted.` : '',
      llmProfiles.length > 12 ? `${llmProfiles.length - 12} LLM profile${llmProfiles.length - 12 === 1 ? '' : 's'} omitted.` : '',
      agentProfiles.length > 12 ? `${agentProfiles.length - 12} agent profile${agentProfiles.length - 12 === 1 ? '' : 's'} omitted.` : '',
      mcpProfiles.length > 12 ? `${mcpProfiles.length - 12} MCP profile${mcpProfiles.length - 12 === 1 ? '' : 's'} omitted.` : '',
    ].filter(Boolean),
  };
}

function buildTeamVisibleResourcesSection(resources: AnalyzableResource[]): AnalysisAiPromptContext['sections'][number] {
  return {
    title: 'Visible resource rows',
    summary: resources.length > 0
      ? 'Bounded resource rows visible to the current Teams scope. Use these for ownership, duplicate, reuse, and GitOps-drift evaluation.'
      : 'No visible linked resource rows were available for this Teams scope.',
    lines: resources.slice(0, MAX_RESOURCE_LINES).map(formatResourceLine),
    limitations: resources.length > MAX_RESOURCE_LINES ? [`${resources.length - MAX_RESOURCE_LINES} additional resource row${resources.length - MAX_RESOURCE_LINES === 1 ? '' : 's'} omitted.`] : [],
  };
}

function formatResourceLine(resource: AnalyzableResource) {
  return redactAnalysisPromptText([
    `kind=${kindSortLabel(resource.kind)}`,
    `id=${resource.id}`,
    `label=${resource.label}`,
    `team=${resource.teamPath || 'Global'}`,
    `source=${resource.source || '-'}`,
    `href=${resource.href || '-'}`,
    `description=${resource.description || '-'}`,
  ].join(' '), MAX_LINE_LENGTH);
}

function countBy<T>(items: T[], keyFn: (item: T) => string) {
  const counts = new Map<string, number>();
  items.forEach(item => {
    const key = keyFn(item).trim() || 'unknown';
    counts.set(key, (counts.get(key) || 0) + 1);
  });
  return counts;
}

function kindSortLabel(kind: string) {
  return TEAM_LINKED_RESOURCE_LABELS[kind as TeamLinkedResourceKind] || kind;
}
