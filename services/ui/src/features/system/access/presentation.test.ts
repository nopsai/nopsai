import assert from 'node:assert/strict';
import test from 'node:test';
import {
  ACCESS_SECTION_CONTENT,
  accessPresetForRole,
  accessPresetIDForRole,
  accessPresetToneClass,
  formatAccessCount,
  formatAccessTimestamp,
  matchesAccessSearch,
} from './presentation.js';

test('maps enterprise role names to the expected access presets', () => {
  assert.equal(accessPresetIDForRole('NopsAI-Admin'), 'admin');
  assert.equal(accessPresetIDForRole('platform-owner'), 'owner');
  assert.equal(accessPresetForRole('team-developer')?.label, 'Developer');
  assert.equal(accessPresetToneClass('custom-role'), 'access-chip--muted');
});

test('supports access search, counts, timestamps, and section copy', () => {
  assert.equal(matchesAccessSearch('ada', 'Ada Lovelace', 'admin'), true);
  assert.equal(matchesAccessSearch('missing', 'Ada Lovelace', 'admin'), false);
  assert.equal(formatAccessCount(1, 'policy', 'policies'), '1 policy');
  assert.equal(formatAccessCount(2, 'policy', 'policies'), '2 policies');
  assert.equal(formatAccessTimestamp('invalid'), '—');
  assert.equal(ACCESS_SECTION_CONTENT['service-accounts'].resultsLabel, 'service accounts');
});
