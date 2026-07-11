import { apiClient } from '../../lib/api.js';
import type { Team } from '../../lib/teamModels.js';

async function responseError(response: Response, fallback: string) {
  const text = await response.text();
  return text || fallback;
}

export async function requestTeamsJson<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await apiClient.fetch(path, { cache: 'no-store', ...options });
  if (!response.ok) {
    throw new Error(await responseError(response, `Request failed: ${response.status}`));
  }
  const text = await response.text();
  if (!text) return undefined as T;
  try {
    return JSON.parse(text) as T;
  } catch {
    return text as T;
  }
}

type TeamRecord = {
  id: number;
  name?: string;
  slug?: string;
  display_name?: string;
  description?: string;
  parent_team_id?: number | null;
  parent_id?: number | null;
  last_run_at?: string;
  navigation_only?: boolean;
};

type ApplicationRecord = {
  id: number;
  name?: string;
  slug?: string;
  display_name?: string;
  team_id?: number | null;
  parent_id?: number | null;
  repo_url?: string;
  repository_full_name?: string;
  last_run_at?: string;
  navigation_only?: boolean;
};

type TeamListResponse = {
  teams?: TeamRecord[];
  applications?: ApplicationRecord[];
};

export type TeamLLMProfilePayload = {
  name?: string;
  provider: string;
  model?: string;
  base_url?: string;
  credential_ref?: string;
  allowed_scopes?: string[];
  reasoning?: string;
  thinking?: boolean;
  timeout_seconds?: number;
  max_tokens?: number;
  temperature?: number;
  extra?: Record<string, string>;
};

export type TeamLLMProfilesPayload = {
  default_profile?: string;
  profiles?: TeamLLMProfilePayload[];
  llm_profiles?: Record<string, Omit<TeamLLMProfilePayload, 'name'>>;
};

export type TeamLLMProfile = TeamLLMProfilePayload & {
  name: string;
  scope: 'team';
  team_id: number;
  team_path: string;
  status?: string;
  validation?: string;
  allowed_in_scope?: boolean;
};

export type TeamLLMProfilesResponse = {
  team_id: number;
  team_path: string;
  default_profile: string;
  profiles: TeamLLMProfile[];
};

export type TeamAgentProfilePayload = {
  id?: string;
  display_name: string;
  role?: string;
  description?: string;
  instructions: string;
  enabled?: boolean;
};

export type TeamAgentProfile = Required<Pick<TeamAgentProfilePayload, 'id' | 'display_name' | 'instructions'>> &
  Omit<TeamAgentProfilePayload, 'id' | 'display_name' | 'instructions'> & {
    scope: 'team';
    team_id: number;
    team_path: string;
    source?: string;
  };

export type TeamAgentProfilesResponse = {
  team_id: number;
  team_path: string;
  default_profile: string;
  profiles: TeamAgentProfile[];
};

export type TeamMCPProfilePayload = {
  name?: string;
  description?: string;
  enabled?: boolean;
  servers?: Array<{ server: string; tools?: string[] }>;
  allowed_scopes?: string[];
};

export type TeamMCPProfile = TeamMCPProfilePayload & {
  name: string;
  scope: 'team';
  team_id: number;
  team_path: string;
};

export type TeamMCPProfilesResponse = {
  team_id: number;
  team_path: string;
  profiles: TeamMCPProfile[];
};

export async function fetchTeams(): Promise<Team[]> {
  const payload = await requestTeamsJson<TeamListResponse | Team[]>('/v1/teams?include=applications');
  if (Array.isArray(payload)) return payload;
  const teams = Array.isArray(payload?.teams) ? payload.teams : [];
  const applications = Array.isArray(payload?.applications) ? payload.applications : [];
  return [
    ...teams.map(teamRecordToTeam),
    ...applications.map(applicationRecordToTeam),
  ];
}

export async function fetchTeamConfigRepository(teamPath: string): Promise<unknown | null> {
  const response = await apiClient.fetch(`/v1/teams/${encodeURIComponent(teamPath)}/config-repository`, { cache: 'no-store' });
  if (response.status === 404) return null;
  if (!response.ok) {
    throw new Error(await responseError(response, `Unable to load config repository (${response.status})`));
  }
  return response.json();
}

export function fetchTeamLLMProfiles(teamID: number | string): Promise<TeamLLMProfilesResponse> {
  return requestTeamsJson<TeamLLMProfilesResponse>(`${teamRoute(teamID)}/llm-profiles`);
}

export function replaceTeamLLMProfiles(teamID: number | string, payload: TeamLLMProfilesPayload): Promise<TeamLLMProfilesResponse> {
  return requestTeamsJson<TeamLLMProfilesResponse>(`${teamRoute(teamID)}/llm-profiles`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function upsertTeamLLMProfile(
  teamID: number | string,
  profileName: string,
  payload: TeamLLMProfilePayload
): Promise<TeamLLMProfilesResponse> {
  return requestTeamsJson<TeamLLMProfilesResponse>(`${teamRoute(teamID)}/llm-profiles/${encodeURIComponent(profileName)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function setTeamDefaultLLMProfile(teamID: number | string, defaultProfile: string): Promise<TeamLLMProfilesResponse> {
  return requestTeamsJson<TeamLLMProfilesResponse>(`${teamRoute(teamID)}/llm-profiles/default`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ default_profile: defaultProfile }),
  });
}

export function deleteTeamLLMProfile(teamID: number | string, profileName: string): Promise<void> {
  return requestTeamsJson<void>(`${teamRoute(teamID)}/llm-profiles/${encodeURIComponent(profileName)}`, { method: 'DELETE' });
}

export function fetchTeamAgentProfiles(teamID: number | string): Promise<TeamAgentProfilesResponse> {
  return requestTeamsJson<TeamAgentProfilesResponse>(`${teamRoute(teamID)}/agent-profiles`);
}

export function createTeamAgentProfile(
  teamID: number | string,
  payload: TeamAgentProfilePayload
): Promise<TeamAgentProfilesResponse> {
  return requestTeamsJson<TeamAgentProfilesResponse>(`${teamRoute(teamID)}/agent-profiles`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function upsertTeamAgentProfile(
  teamID: number | string,
  profileID: string,
  payload: TeamAgentProfilePayload
): Promise<TeamAgentProfilesResponse> {
  return requestTeamsJson<TeamAgentProfilesResponse>(`${teamRoute(teamID)}/agent-profiles/${encodeURIComponent(profileID)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function setTeamDefaultAgentProfile(teamID: number | string, defaultProfile: string): Promise<TeamAgentProfilesResponse> {
  return requestTeamsJson<TeamAgentProfilesResponse>(`${teamRoute(teamID)}/agent-profiles/default`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ default_profile: defaultProfile }),
  });
}

export function deleteTeamAgentProfile(teamID: number | string, profileID: string): Promise<void> {
  return requestTeamsJson<void>(`${teamRoute(teamID)}/agent-profiles/${encodeURIComponent(profileID)}`, { method: 'DELETE' });
}

export function fetchTeamMCPProfiles(teamID: number | string): Promise<TeamMCPProfilesResponse> {
  return requestTeamsJson<TeamMCPProfilesResponse>(`${teamRoute(teamID)}/mcp-profiles`);
}

export function createTeamMCPProfile(teamID: number | string, payload: TeamMCPProfilePayload): Promise<TeamMCPProfilesResponse> {
  return requestTeamsJson<TeamMCPProfilesResponse>(`${teamRoute(teamID)}/mcp-profiles`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function upsertTeamMCPProfile(
  teamID: number | string,
  profileName: string,
  payload: TeamMCPProfilePayload
): Promise<TeamMCPProfilesResponse> {
  return requestTeamsJson<TeamMCPProfilesResponse>(`${teamRoute(teamID)}/mcp-profiles/${encodeURIComponent(profileName)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
}

export function deleteTeamMCPProfile(teamID: number | string, profileName: string): Promise<void> {
  return requestTeamsJson<void>(`${teamRoute(teamID)}/mcp-profiles/${encodeURIComponent(profileName)}`, { method: 'DELETE' });
}

function teamRoute(teamID: number | string): string {
  return `/v1/teams/${encodeURIComponent(String(teamID))}`;
}

function teamRecordToTeam(record: TeamRecord): Team {
  return {
    id: record.id,
    name: record.name || record.slug || record.display_name || '',
    kind: 'team',
    parent_id: record.parent_team_id ?? record.parent_id ?? null,
    description: record.description || '',
    last_run_at: record.last_run_at,
    navigation_only: Boolean(record.navigation_only),
  };
}

function applicationRecordToTeam(record: ApplicationRecord): Team {
  return {
    id: record.id,
    name: record.name || record.slug || record.display_name || record.repository_full_name || '',
    kind: 'app',
    parent_id: record.team_id ?? record.parent_id ?? null,
    repo_url: record.repo_url || '',
    repository_full_name: record.repository_full_name || '',
    last_run_at: record.last_run_at,
    navigation_only: Boolean(record.navigation_only),
  };
}
