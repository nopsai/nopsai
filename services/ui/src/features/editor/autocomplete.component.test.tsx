import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiClient } from '../../lib/api';
import {
  buildEditorScopeList,
  fetchEditorAutocompleteMetadata,
  normalizeAutocompleteList,
  normalizeProfilePayload,
  normalizeRuntimePoolNames,
  normalizeScopeLabel,
} from './autocomplete';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('editor autocomplete metadata', () => {
  it('normalizes list, profile, and scope payloads', () => {
    expect(normalizeAutocompleteList([' TOKEN ', { name: 'DEPLOY_ENV' }, { id: 'agent-profile' }, '', null])).toEqual([
      'TOKEN',
      'DEPLOY_ENV',
      'agent-profile',
    ]);
    expect(normalizeAutocompleteList(null)).toEqual([]);
    expect(normalizeProfilePayload({ profiles: [{ name: 'standard' }, { name: 'blocked', allowed_in_scope: false }] })).toEqual([
      'standard',
    ]);
    expect(normalizeProfilePayload({ profiles: [{ id: 'devops-engineer' }, { id: 'disabled', enabled: false }] })).toEqual([
      'devops-engineer',
    ]);
    expect(normalizeProfilePayload(['github-pr-review'])).toEqual(['github-pr-review']);
    expect(normalizeRuntimePoolNames({ runtime_pools: { ' high-memory ': {}, default: {} } })).toEqual([
      'default',
      'high-memory',
    ]);
    expect(normalizeRuntimePoolNames(['default', { name: 'gpu' }])).toEqual(['default', 'gpu']);
    expect(normalizeScopeLabel(' default ')).toBe('');
    expect(normalizeScopeLabel('/platform/dev/')).toBe('platform/dev');
    expect(normalizeScopeLabel({ value: 'prod' })).toBe('prod');
    expect(normalizeScopeLabel(42)).toBe('');
    expect(buildEditorScopeList(['default', 'platform'], [{ scope: 'prod' }, { name: '/platform/' }])).toEqual([
      '',
      'platform',
      'prod',
    ]);
  });

  it('loads scoped resources and optional profile catalogs', async () => {
    vi.spyOn(apiClient, 'fetch').mockImplementation(async input => {
      const path = String(input);
      const payloads: Record<string, unknown> = {
        '/v1/secrets': ['GLOBAL_SECRET'],
        '/v1/variables': ['GLOBAL_VARIABLE'],
        '/v1/steps': [{ name: 'shared/checkout' }],
        '/v1/secrets/scopes': ['default', 'team'],
        '/v1/variables/scopes': [{ scope: 'team' }],
        '/v1/system/agent-profiles': { profiles: [{ id: 'devops-engineer' }, { id: 'sre' }] },
        '/v1/system/llm-profiles': { profiles: [{ name: 'reasoning' }] },
        '/v1/system/mcp/profiles': { profiles: [{ name: 'github' }] },
        '/v1/system/config': { runtime_pools: { default: {}, 'high-memory': {} } },
        '/v1/secrets?scope=team': ['TEAM_SECRET'],
        '/v1/variables?scope=team': ['TEAM_VARIABLE'],
      };
      return new Response(JSON.stringify(payloads[path] ?? []), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });

    const metadata = await fetchEditorAutocompleteMetadata({
      includeAgentProfiles: true,
      includeLLMProfiles: true,
      includeMCPProfiles: true,
      includeRuntimePools: true,
    });

    expect(metadata.secrets).toEqual(['GLOBAL_SECRET']);
    expect(metadata.variables).toEqual(['GLOBAL_VARIABLE']);
    expect(metadata.reusableSteps).toEqual(['shared/checkout']);
    expect(metadata.secretScopes).toEqual([
      { scope: '', items: ['GLOBAL_SECRET'] },
      { scope: 'team', items: ['TEAM_SECRET'] },
    ]);
    expect(metadata.variableScopes).toEqual([
      { scope: '', items: ['GLOBAL_VARIABLE'] },
      { scope: 'team', items: ['TEAM_VARIABLE'] },
    ]);
    expect(metadata.agentProfiles).toEqual(['devops-engineer', 'sre']);
    expect(metadata.llmProfiles).toEqual(['reasoning']);
    expect(metadata.mcpProfiles).toEqual(['github']);
    expect(metadata.runtimePools).toEqual(['default', 'high-memory']);
    expect(metadata.loading).toBe(false);
    expect(metadata.fetchedAt).toEqual(expect.any(Number));
  });
});
