import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  formatStepDetailPath,
  formatStepDetailSource,
  formatStepUsageTeam,
  summarizeStepUsageSources,
} from './stepDetailPresentation.js';

test('formats step detail source state for GitOps, drafts, and database overrides', () => {
  assert.deepEqual(formatStepDetailSource('GitOps'), {
    label: 'GitOps',
    tone: 'success',
    description: 'Synced from configuration repository',
  });
  assert.deepEqual(formatStepDetailSource('draft'), {
    label: 'Draft',
    tone: 'warning',
    description: 'Local draft, save before use',
  });
  assert.deepEqual(formatStepDetailSource('database'), {
    label: 'Database',
    tone: 'neutral',
    description: 'Stored as database definition or override',
  });
});

test('formats step detail and usage teams without exposing the global path slug', () => {
  assert.equal(formatStepDetailPath({ path: '' }), 'Global');
  assert.equal(formatStepDetailPath({ path: 'global' }), 'Global');
  assert.equal(formatStepDetailPath({ path: 'platform/api' }), 'platform/api');
  assert.equal(formatStepUsageTeam({
    identifier: 'platform/api/release',
    name: 'release',
    path: '',
    source: 'git',
  }), 'platform/api');
});

test('summarizes step usage source counts for the side rail', () => {
  assert.deepEqual(
    summarizeStepUsageSources([
      { identifier: 'platform/release', name: 'release', path: 'platform', source: 'git' },
      { identifier: 'platform/nightly', name: 'nightly', path: 'platform', source: 'database' },
      { identifier: 'sandbox/test', name: 'test', path: 'sandbox', source: 'draft' },
    ]),
    { total: 3, gitOps: 1, database: 1, drafts: 1 }
  );
});
