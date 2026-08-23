import type { AgentProfileRecord } from './agent-roles/model';
import {
  aiResourceLocalName,
  buildAIResourceScopedID,
  normalizeAIResourceTeamPath,
} from './aiResourceTeams';
import type {
  TeamAgentProfile,
  TeamAgentProfilesResponse,
  TeamLLMProfile,
  TeamLLMProfilesResponse,
  TeamMCPProfile,
  TeamMCPProfilesResponse,
} from './teamProfileApi';
import { normalizeLLMPricing, type LLMProfileRecord } from './models/model';
import type { MCPProfileRecord } from './mcp/model';

export function teamScopedResourceID(teamPath: string, localName: string) {
  return buildAIResourceScopedID(normalizeAIResourceTeamPath(teamPath), teamLocalResourceID(localName));
}

export function teamLocalResourceID(resourceID: string) {
  return aiResourceLocalName(resourceID) || normalizeAIResourceTeamPath(resourceID);
}

export function teamScopedDefaultID(teamPath: string, defaultProfile: string) {
  const localName = teamLocalResourceID(defaultProfile);
  return localName ? teamScopedResourceID(teamPath, localName) : '';
}

export function teamDefaultProfileAPIValue(teamPath: string, selectedProfileID: string, teamOwnedProfileIDs: string[]) {
  if (!selectedProfileID) return '';
  const localID = teamLocalResourceID(selectedProfileID);
  const ownedLocalIDs = new Set(teamOwnedProfileIDs.map(teamLocalResourceID));
  return normalizeAIResourceTeamPath(teamPath) && ownedLocalIDs.has(localID) ? localID : selectedProfileID;
}

export function teamLLMProfileRecords(payload: TeamLLMProfilesResponse | null): LLMProfileRecord[] {
  if (!payload) return [];
  return payload.profiles.map(profile => teamLLMProfileRecord(payload.team_path, profile)).sort((a, b) => a.name.localeCompare(b.name));
}

export function teamAgentProfileRecords(payload: TeamAgentProfilesResponse | null): AgentProfileRecord[] {
  if (!payload) return [];
  return payload.profiles.map(profile => teamAgentProfileRecord(payload.team_path, profile)).sort((a, b) =>
    a.display_name.localeCompare(b.display_name, undefined, { sensitivity: 'base' })
  );
}

export function teamMCPProfileRecords(payload: TeamMCPProfilesResponse | null): MCPProfileRecord[] {
  if (!payload) return [];
  return payload.profiles.map(profile => teamMCPProfileRecord(payload.team_path, profile)).sort((a, b) => a.name.localeCompare(b.name));
}

function teamLLMProfileRecord(teamPath: string, profile: TeamLLMProfile): LLMProfileRecord {
  const normalizedTeamPath = normalizeAIResourceTeamPath(profile.team_path || teamPath);
  const localName = teamLocalResourceID(profile.name);
  return {
    name: teamScopedResourceID(normalizedTeamPath, localName),
    scope: 'team',
    team_path: normalizedTeamPath,
    team_local_name: localName,
    provider: profile.provider || '',
    model: profile.model || '',
    base_url: profile.base_url || '',
    credential_ref: profile.credential_ref || '',
    allowed_scopes: profile.allowed_scopes || [],
    reasoning: profile.reasoning || '',
    thinking: typeof profile.thinking === 'boolean' ? profile.thinking : undefined,
    timeout_seconds: typeof profile.timeout_seconds === 'number' ? profile.timeout_seconds : 0,
    max_tokens: typeof profile.max_tokens === 'number' ? profile.max_tokens : 0,
    temperature: typeof profile.temperature === 'number' ? profile.temperature : undefined,
    prompt_cache: profile.prompt_cache,
    provider_state: profile.provider_state,
    pricing: normalizeLLMPricing(profile.pricing),
    extra: profile.extra || {},
    status: profile.status || 'unknown',
    validation: profile.validation,
    allowed_in_scope: profile.allowed_in_scope,
  };
}

function teamAgentProfileRecord(teamPath: string, profile: TeamAgentProfile): AgentProfileRecord {
  const normalizedTeamPath = normalizeAIResourceTeamPath(profile.team_path || teamPath);
  const localName = teamLocalResourceID(profile.id);
  return {
    id: teamScopedResourceID(normalizedTeamPath, localName),
    scope: 'team',
    team_path: normalizedTeamPath,
    team_local_name: localName,
    display_name: profile.display_name || localName,
    role: profile.role || '',
    description: profile.description || '',
    instructions: profile.instructions || '',
    enabled: profile.enabled !== false,
    built_in: false,
    source: profile.source || 'team',
    usage_count: 0,
    references: [],
  };
}

function teamMCPProfileRecord(teamPath: string, profile: TeamMCPProfile): MCPProfileRecord {
  const normalizedTeamPath = normalizeAIResourceTeamPath(profile.team_path || teamPath);
  const localName = teamLocalResourceID(profile.name);
  return {
    name: teamScopedResourceID(normalizedTeamPath, localName),
    scope: 'team',
    team_path: normalizedTeamPath,
    team_local_name: localName,
    description: profile.description || '',
    enabled: profile.enabled !== false,
    servers: (profile.servers || []).map(ref => ({
      server: ref.server || '',
      tools: ref.tools || [],
    })).filter(ref => Boolean(ref.server)),
    allowed_scopes: profile.allowed_scopes || [],
  };
}
