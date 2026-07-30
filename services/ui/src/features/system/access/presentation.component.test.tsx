import { describe, expect, it } from 'vitest';
import {
  ACCESS_ROLE_PRESETS,
  ACCESS_SECTION_CONTENT,
  accessPresetForRole,
  accessPresetIDForRole,
  accessPresetToneClass,
  buildAccessSummaryMetrics,
  formatAccessCount,
  formatAccessTimestamp,
  matchesAccessSearch,
} from './presentation';

describe('Access presentation', () => {
  it('maps enterprise roles to product presets', () => {
    expect(accessPresetIDForRole('NopsAI-Admin')).toBe('admin');
    expect(accessPresetIDForRole('platform-owner')).toBe('owner');
    expect(accessPresetIDForRole('team-developer')).toBe('developer');
    expect(accessPresetIDForRole('viewer')).toBe('viewer');
    expect(accessPresetIDForRole('custom-role')).toBeNull();
    expect(accessPresetIDForRole('')).toBeNull();
    expect(accessPresetForRole('team-developer')?.label).toBe('Developer');
    expect(accessPresetForRole('custom-role')).toBeNull();
    expect(accessPresetToneClass('custom-role')).toBe('access-chip--muted');
    expect(ACCESS_ROLE_PRESETS).toHaveLength(4);
  });

  it('supports access search, counts, timestamps, and section copy', () => {
    expect(matchesAccessSearch('', 'Ada Lovelace')).toBe(true);
    expect(matchesAccessSearch('ada', 'Ada Lovelace', 'admin')).toBe(true);
    expect(matchesAccessSearch('missing', 'Ada Lovelace', 'admin')).toBe(false);
    expect(formatAccessCount(1, 'policy', 'policies')).toBe('1 policy');
    expect(formatAccessCount(2, 'policy', 'policies')).toBe('2 policies');
    expect(formatAccessTimestamp()).toBe('—');
    expect(formatAccessTimestamp('invalid')).toBe('—');
    expect(formatAccessTimestamp('2026-06-09T10:00:00Z')).not.toBe('—');
    expect(ACCESS_SECTION_CONTENT['service-accounts'].resultsLabel).toBe('service accounts');
  });

  it('builds access summary metrics', () => {
    const metrics = buildAccessSummaryMetrics({
      users: [
        { id: 'user-1', sub: 'alice', email: 'alice@example.com', status: 'active' },
        { id: 'user-2', sub: 'bob', email: 'bob@example.com', status: 'disabled' },
      ],
      serviceAccounts: [
        { id: 'svc-1', sub: 'deploy-bot', email: '', status: 'active', token_count: 2 },
      ],
      roles: [
        {
          id: 'viewer',
          role: 'viewer',
          policies: [{ role: 'viewer', name: 'Read pipelines', obj: 'pipeline:*', act: 'pipeline.read' }],
        },
        { id: 'custom', role: 'release-manager', policies: [] },
      ],
      policies: [{ role: 'viewer', name: 'Read pipelines', obj: 'pipeline:*', act: 'pipeline.read' }],
      identityProviders: [
        {
          id: 'keycloak',
          type: 'oidc',
          display_name: 'Keycloak',
          issuer: 'https://idp.example.com',
          client_id: 'nopsai',
          scopes: [],
          allowed_email_domains: [],
          role_mapping: {},
          team_mapping: {},
          basic_role_mapping: {},
          enabled: true,
        },
      ],
    });

    expect(metrics.map(metric => metric.value)).toEqual(['1', '1', '2', '50%']);
  });
});
