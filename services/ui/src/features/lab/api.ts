import { apiClient } from '../../lib/api';
import { normalizeLabScopeLabel } from './suggestions';

export type LabPipelineListItem = { id: string; source?: string };

type LabAutocompleteMetadata = {
  secrets: unknown;
  variables: unknown;
  steps: unknown;
  agentProfiles: unknown;
  llmProfiles: unknown;
  mcpProfiles: unknown;
  runtimeConfig: unknown;
};

async function fetchOptionalJSON(path: string, fallback: unknown): Promise<unknown> {
  const response = await apiClient.fetch(path);
  return response.ok ? response.json() : fallback;
}

export async function fetchLabAgentProfilesMetadata(): Promise<unknown> {
  return fetchOptionalJSON('/v1/system/agent-roles', null);
}

export async function fetchLabAutocompleteMetadata(scope: string): Promise<LabAutocompleteMetadata> {
  const scopeParam = scope ? `?scope=${encodeURIComponent(scope)}` : '';
  const [secrets, variables, steps, agentProfiles, llmProfiles, mcpProfiles, runtimeConfig] = await Promise.all([
    fetchOptionalJSON(`/v1/secrets${scopeParam}`, []),
    fetchOptionalJSON(`/v1/variables${scopeParam}`, []),
    fetchOptionalJSON('/v1/steps', []),
    fetchLabAgentProfilesMetadata(),
    fetchOptionalJSON(`/v1/system/models${scopeParam}`, null),
    fetchOptionalJSON('/v1/system/mcp/profiles', null),
    fetchOptionalJSON('/v1/system/config', null),
  ]);

  return { secrets, variables, steps, agentProfiles, llmProfiles, mcpProfiles, runtimeConfig };
}

export async function fetchLabPipelines(): Promise<LabPipelineListItem[]> {
  const response = await apiClient.fetch('/v1/pipelines?include_source=true');
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Failed to load pipelines (${response.status})`);
  }

  const payload = await response.json();
  const pipelines: LabPipelineListItem[] = Array.isArray(payload)
    ? payload
        .map((item: unknown): LabPipelineListItem | null => {
          if (typeof item === 'string') return { id: item };
          if (!item || typeof item !== 'object') return null;
          const record = item as Record<string, unknown>;
          const id = typeof record.id === 'string' ? record.id : typeof record.identifier === 'string' ? record.identifier : '';
          const source = typeof record.source === 'string' ? record.source : undefined;
          return id ? { id, source } : null;
        })
        .filter((item: LabPipelineListItem | null): item is LabPipelineListItem => Boolean(item))
    : [];

  return pipelines.sort((left, right) => left.id.localeCompare(right.id));
}

export async function fetchLabScopes(): Promise<string[]> {
  const [secretPayload, variablePayload] = await Promise.all([
    fetchOptionalJSON('/v1/secrets/scopes', []),
    fetchOptionalJSON('/v1/variables/scopes', []),
  ]);

  const scopes = new Set<string>(['']);
  if (Array.isArray(secretPayload)) {
    secretPayload.forEach(entry => {
      if (!entry || typeof entry !== 'object') return;
      scopes.add(normalizeLabScopeLabel((entry as Record<string, unknown>).scope));
    });
  }

  if (Array.isArray(variablePayload)) {
    variablePayload.forEach(entry => {
      if (typeof entry === 'string') {
        scopes.add(normalizeLabScopeLabel(entry));
        return;
      }
      if (!entry || typeof entry !== 'object') return;
      const record = entry as Record<string, unknown>;
      scopes.add(normalizeLabScopeLabel(record.scope ?? record.name ?? record.value));
    });
  }

  return Array.from(scopes)
    .map(normalizeLabScopeLabel)
    .sort((left, right) => left.localeCompare(right));
}
