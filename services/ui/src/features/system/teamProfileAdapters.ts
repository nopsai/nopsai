import type { AgentProfileRecord } from './agent-profiles/model';
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
} from './teamProfileApi';
import type { LLMProfileRecord } from './llm-profiles/model';

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
