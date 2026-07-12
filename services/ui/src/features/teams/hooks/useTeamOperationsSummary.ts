import { useEffect, useMemo, useState } from 'react';
import { isAppTeam, teamPathForURL, type Team } from '../../../lib/teamModels';
import {
  fetchTeamAgentProfiles,
  fetchTeamConfigRepository,
  fetchTeamLLMProfiles,
  fetchTeamMCPProfiles,
  type TeamAgentProfilesResponse,
  type TeamLLMProfilesResponse,
  type TeamMCPProfilesResponse,
} from '../api';
import { fetchAgentProfiles } from '../../system/agent-profiles/api';
import { fetchLLMProfiles } from '../../system/llm-profiles/api';
import { fetchMCPRegistry } from '../../system/mcp/api';
import { aiResourceTeamScope } from '../../system/aiResourceTeams';
import {
  normalizeNotificationRouteRecord,
  type NotificationRouteRecord,
} from '../notificationRoutes';
import type { PipelineRunsConfigRepository } from './useTeamConfigRepositoryController';
import {
  DEFAULT_ADMIN_ROLE,
  normalizeAccessGrantRecord,
  type AccessGrantRecord,
} from '../../system/access/model';
import { asRecord, normalizeListPayload, readOptionalString, readString } from '../../system/data';

type FetchJson = <T>(path: string, options?: RequestInit) => Promise<T>;

type PermissionCheck = {
  action: string;
  label: string;
};

export type TeamAccessPermissionSummary = PermissionCheck & {
  allowed: boolean;
};

export type TeamOperationsSummaryState = {
  teamPath: string;
  loading: boolean;
  configRepo: PipelineRunsConfigRepository | null;
  configRepoError: string | null;
  notificationRoute: NotificationRouteRecord | null;
  notificationError: string | null;
  llmProfiles: TeamLLMProfilesResponse | null;
  agentProfiles: TeamAgentProfilesResponse | null;
  mcpProfiles: TeamMCPProfilesResponse | null;
  aiProfilesError: string | null;
  accessGrants: AccessGrantRecord[];
  accessGrantsError: string | null;
  permissions: TeamAccessPermissionSummary[];
  permissionsError: string | null;
};

export const TEAM_ACCESS_PERMISSION_CHECKS: PermissionCheck[] = [
  { action: 'team.read', label: 'View team' },
  { action: 'team.update', label: 'Update team' },
  { action: 'team.manage_acl', label: 'Manage access' },
  { action: 'config_repo.manage', label: 'Manage GitOps' },
  { action: 'config_repo.sync', label: 'Sync GitOps' },
];

export const ROOT_ACCESS_PERMISSION_CHECKS: PermissionCheck[] = [
  { action: 'system.read', label: 'View system' },
  { action: 'system.update', label: 'Update system' },
  { action: 'config_repo.manage', label: 'Manage global GitOps' },
  { action: 'config_repo.sync', label: 'Sync global GitOps' },
];

function createEmptySummary(teamPath = '', loading = false): TeamOperationsSummaryState {
  return {
    teamPath,
    loading,
    configRepo: null,
    configRepoError: null,
    notificationRoute: null,
    notificationError: null,
    llmProfiles: null,
    agentProfiles: null,
    mcpProfiles: null,
    aiProfilesError: null,
    accessGrants: [],
    accessGrantsError: null,
    permissions: [],
    permissionsError: null,
  };
}

function resultValue<T>(result: PromiseSettledResult<T>, fallback: T): T {
  return result.status === 'fulfilled' ? result.value : fallback;
}

function resultError(result: PromiseSettledResult<unknown>, fallback: string): string | null {
  if (result.status === 'fulfilled') return null;
  return result.reason instanceof Error ? result.reason.message : fallback;
}

function normalizeAccessGrantList(payload: unknown): AccessGrantRecord[] {
  const record = payload && typeof payload === 'object' ? payload as Record<string, unknown> : null;
  const values = Array.isArray(payload)
    ? payload
    : Array.isArray(record?.grants)
      ? record.grants
      : Array.isArray(record?.access_grants)
        ? record.access_grants
        : Array.isArray(record?.items)
          ? record.items
          : [];
  return values
    .map(item => normalizeAccessGrantRecord(item))
    .filter(Boolean) as AccessGrantRecord[];
}

function isGlobalAIResource(resourceID: string) {
  return !aiResourceTeamScope(resourceID).teamPath;
}

function normalizeGlobalAccessGrants(grants: AccessGrantRecord[]) {
  return grants.filter(grant => {
    const resourceType = grant.resourceType.trim().toLowerCase();
    const resourceID = grant.resourceID.trim().replace(/^\/+|\/+$/g, '').toLowerCase();
    return resourceType === 'platform' && resourceID === 'platform';
  });
}

function normalizeGlobalAdminUserGrants(payload: unknown): AccessGrantRecord[] {
  const users = normalizeListPayload(payload, ['users', 'items', 'data', 'records', 'results']) ?? [];
  return users
    .map(item => {
      const record = asRecord(item);
      if (!record) return null;
      const roles = Array.isArray(record.roles) ? record.roles : [];
      const adminRole = roles.some(role => {
        const roleRecord = asRecord(role);
        const roleName = (typeof role === 'string' ? role : readString(roleRecord?.role)).trim().toLowerCase();
        return roleName === DEFAULT_ADMIN_ROLE || roleName === 'admin';
      });
      if (!adminRole) return null;

      const id = readString(record.id).trim() || readString(record.sub).trim() || readString(record.email).trim();
      if (!id) return null;
      const displayName =
        readOptionalString(record.display_name)?.trim() ||
        readString(record.sub).trim() ||
        readString(record.email).trim() ||
        id;

      return {
        id: `global-admin-user-${id}`,
        subjectType: 'user',
        subjectID: id,
        subjectDisplay: displayName,
        role: DEFAULT_ADMIN_ROLE,
        resourceType: 'platform',
        resourceID: 'platform',
        inherit: false,
      } satisfies AccessGrantRecord;
    })
    .filter(Boolean) as AccessGrantRecord[];
}

function mergeGlobalAccessGrants(...grantLists: AccessGrantRecord[][]): AccessGrantRecord[] {
  const grants = new Map<string, AccessGrantRecord>();
  grantLists.flat().forEach(grant => {
    const subjectID = grant.subjectID.trim().toLowerCase();
    const role = grant.role.trim().toLowerCase();
    if (!subjectID || !role) return;
    const key = [
      grant.subjectType.trim().toLowerCase(),
      subjectID,
      role,
      grant.resourceType.trim().toLowerCase(),
      grant.resourceID.trim().toLowerCase(),
    ].join('::');
    grants.set(key, grant);
  });
  return Array.from(grants.values()).sort((left, right) =>
    accessSubjectSortLabel(left).localeCompare(accessSubjectSortLabel(right), undefined, { sensitivity: 'base' })
  );
}

function accessSubjectSortLabel(grant: AccessGrantRecord) {
  return `${grant.subjectDisplay || grant.subjectID || grant.subjectType}::${grant.role}`;
}

function globalLLMProfilesSummary(payload: Awaited<ReturnType<typeof fetchLLMProfiles>>): TeamLLMProfilesResponse {
  const profiles = payload.profiles
    .filter(profile => isGlobalAIResource(profile.name))
    .map(profile => ({
      ...profile,
      scope: 'global' as const,
      team_id: 0,
      team_path: '',
    }));
  const defaultProfile = profiles.some(profile => profile.name === payload.default_profile)
    ? payload.default_profile
    : profiles[0]?.name || '';
  return {
    team_id: 0,
    team_path: '',
    default_profile: defaultProfile,
    profiles,
  };
}

function globalAgentProfilesSummary(payload: Awaited<ReturnType<typeof fetchAgentProfiles>>): TeamAgentProfilesResponse {
  const profiles = payload.profiles
    .filter(profile => isGlobalAIResource(profile.id))
    .map(profile => ({
      id: profile.id,
      display_name: profile.display_name,
      role: profile.role,
      description: profile.description,
      instructions: profile.instructions,
      enabled: profile.enabled,
      source: profile.source,
      scope: 'global' as const,
      team_id: 0,
      team_path: '',
    }));
  const defaultProfile = profiles.some(profile => profile.id === payload.default_profile)
    ? payload.default_profile
    : profiles[0]?.id || '';
  return {
    team_id: 0,
    team_path: '',
    default_profile: defaultProfile,
    profiles,
  };
}

function globalMCPProfilesSummary(payload: Awaited<ReturnType<typeof fetchMCPRegistry>>): TeamMCPProfilesResponse {
  return {
    team_id: 0,
    team_path: '',
    profiles: payload.profiles
      .filter(profile => isGlobalAIResource(profile.name))
      .map(profile => ({
        ...profile,
        scope: 'global' as const,
        team_id: 0,
        team_path: '',
      })),
  };
}

function normalizeConfigRepository(payload: unknown): PipelineRunsConfigRepository | null {
  if (!payload || typeof payload !== 'object') return null;
  const record = payload as Record<string, unknown>;
  const id = typeof record.id === 'number' ? record.id : Number(record.id);
  return {
    id: Number.isFinite(id) ? id : 0,
    scope_type: typeof record.scope_type === 'string' ? record.scope_type : '',
    scope_id: typeof record.scope_id === 'string' ? record.scope_id : '',
    repo_url: typeof record.repo_url === 'string' ? record.repo_url : '',
    branch: typeof record.branch === 'string' && record.branch.trim() ? record.branch : 'main',
    base_path: typeof record.base_path === 'string' ? record.base_path : '',
    enabled: Boolean(record.enabled),
    write_enabled: Boolean(record.write_enabled),
    write_branch: typeof record.write_branch === 'string' && record.write_branch.trim() ? record.write_branch : 'nopsai/ui-changes',
    last_sync_status: typeof record.last_sync_status === 'string' ? record.last_sync_status : '',
    last_sync_message: typeof record.last_sync_message === 'string' ? record.last_sync_message : undefined,
    last_sync_started_at: typeof record.last_sync_started_at === 'string' ? record.last_sync_started_at : undefined,
    last_sync_completed_at: typeof record.last_sync_completed_at === 'string' ? record.last_sync_completed_at : undefined,
    last_sync_commit_sha: typeof record.last_sync_commit_sha === 'string' ? record.last_sync_commit_sha : undefined,
  };
}

export function useTeamOperationsSummary({
  team,
  teams,
  fetchJson,
  checkAccessPermission,
}: {
  team: Team | null;
  teams: Team[];
  fetchJson: FetchJson;
  checkAccessPermission: (action: string, resourceType: string, resourceID: string) => Promise<boolean>;
}) {
  const teamPath = useMemo(() => (team ? teamPathForURL(team, teams) : ''), [team, teams]);
  const [summary, setSummary] = useState<TeamOperationsSummaryState>(() => createEmptySummary(teamPath, !team));

  useEffect(() => {
    if (!team) {
      let cancelled = false;

      const loadRootSummary = async () => {
        const [
          llmResult,
          agentResult,
          mcpResult,
          accessGrantsResult,
          adminUsersResult,
          permissionsResult,
        ] = await Promise.allSettled([
          fetchLLMProfiles().then(globalLLMProfilesSummary),
          fetchAgentProfiles().then(globalAgentProfilesSummary),
          fetchMCPRegistry().then(globalMCPProfilesSummary),
          fetchJson<unknown>('/v1/access/grants?resource_type=platform&resource_id=platform')
            .then(normalizeAccessGrantList)
            .then(normalizeGlobalAccessGrants),
          fetchJson<unknown>('/v1/admin/users').then(normalizeGlobalAdminUserGrants),
          Promise.all(
            ROOT_ACCESS_PERMISSION_CHECKS.map(async check => ({
              ...check,
              allowed: await checkAccessPermission(check.action, 'platform', 'platform'),
            }))
          ),
        ] as const);

        if (cancelled) return;
        setSummary({
          ...createEmptySummary(''),
          loading: false,
          llmProfiles: resultValue(llmResult, null),
          agentProfiles: resultValue(agentResult, null),
          mcpProfiles: resultValue(mcpResult, null),
          aiProfilesError: [llmResult, agentResult, mcpResult]
            .map(result => resultError(result, 'Unable to load global AI profiles'))
            .find(Boolean) || null,
          accessGrants: mergeGlobalAccessGrants(
            resultValue(accessGrantsResult, []),
            resultValue(adminUsersResult, [])
          ),
          accessGrantsError: [accessGrantsResult, adminUsersResult]
            .map(result => resultError(result, 'Unable to load global admins'))
            .find(Boolean) || null,
          permissions: resultValue(permissionsResult, []),
          permissionsError: resultError(permissionsResult, 'Unable to load platform permissions'),
        });
      };

      void loadRootSummary();
      return () => {
        cancelled = true;
      };
    }

    if (isAppTeam(team) || !teamPath) {
      return undefined;
    }

    let cancelled = false;

    const encodedTeamPath = encodeURIComponent(teamPath);
    const load = async () => {
      const [
        repoResult,
        notificationResult,
        llmResult,
        agentResult,
        mcpResult,
        accessGrantsResult,
        permissionsResult,
      ] = await Promise.allSettled([
        fetchTeamConfigRepository(teamPath).then(normalizeConfigRepository),
        fetchJson<unknown>(`/v1/teams/${encodedTeamPath}/notifications`).then(normalizeNotificationRouteRecord),
        fetchTeamLLMProfiles(teamPath),
        fetchTeamAgentProfiles(teamPath),
        fetchTeamMCPProfiles(teamPath),
        fetchJson<unknown>(`/v1/access/grants?resource_type=team&resource_id=${encodedTeamPath}`).then(normalizeAccessGrantList),
        Promise.all(
          TEAM_ACCESS_PERMISSION_CHECKS.map(async check => ({
            ...check,
            allowed: await checkAccessPermission(check.action, 'team', teamPath),
          }))
        ),
      ] as const);

      if (cancelled) return;
      setSummary({
        teamPath,
        loading: false,
        configRepo: resultValue(repoResult, null),
        configRepoError: resultError(repoResult, 'Unable to load GitOps configuration'),
        notificationRoute: resultValue(notificationResult, null),
        notificationError: resultError(notificationResult, 'Unable to load notification policy'),
        llmProfiles: resultValue(llmResult, null),
        agentProfiles: resultValue(agentResult, null),
        mcpProfiles: resultValue(mcpResult, null),
        aiProfilesError: [llmResult, agentResult, mcpResult]
          .map(result => resultError(result, 'Unable to load AI profiles'))
          .find(Boolean) || null,
        accessGrants: resultValue(accessGrantsResult, []),
        accessGrantsError: resultError(accessGrantsResult, 'Unable to load access grants'),
        permissions: resultValue(permissionsResult, []),
        permissionsError: resultError(permissionsResult, 'Unable to load effective permissions'),
      });
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [checkAccessPermission, fetchJson, team, teamPath, teams]);

  if (summary.teamPath === teamPath) {
    return summary;
  }
  if (team && (isAppTeam(team) || !teamPath)) {
    return createEmptySummary(teamPath);
  }
  return createEmptySummary(teamPath, true);
}
