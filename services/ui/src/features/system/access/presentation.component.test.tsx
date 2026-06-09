import { describe, expect, it } from 'vitest';
import {
  ACCESS_ROLE_PRESETS,
  ACCESS_SECTION_CONTENT,
  accessPresetForRole,
  accessPresetIDForRole,
  accessPresetToneClass,
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
});
