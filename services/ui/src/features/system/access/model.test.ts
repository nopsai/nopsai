import assert from 'node:assert/strict';
import test from 'node:test';
import {
  accessGrantEditKey,
  isProtectedAccessRole,
  normalizeAccessGrantRecord,
  normalizeBasicGrantInputs,
} from './model.js';

test('normalizes and deduplicates enterprise basic access grants', () => {
  const grants = normalizeBasicGrantInputs([
    { role: 'Owner', resourceType: 'folder', resourceID: '/platform/' },
    { role: 'owner', resourceType: 'folder', resourceID: 'platform' },
    { role: 'admin', resourceType: 'platform', resourceID: 'platform' },
  ]);

  assert.equal(grants.length, 2);
  assert.equal(accessGrantEditKey(grants[0]), 'owner::folder::platform');
  assert.equal(accessGrantEditKey(grants[1]), 'admin::platform::platform');
  assert.equal(isProtectedAccessRole('NopsAI-Admin'), true);
});

test('maps API access grant records into the UI contract', () => {
  assert.deepEqual(
    normalizeAccessGrantRecord({
      id: 'grant-1',
      subject_type: 'service_account',
      subject_id: 'deploy-bot',
      role: 'developer',
      resource_type: 'folder',
      resource_id: 'platform',
      inherit: true,
      granted_by: 'admin',
    }),
    {
      id: 'grant-1',
      subjectType: 'service_account',
      subjectID: 'deploy-bot',
      subjectDisplay: undefined,
      role: 'developer',
      resourceType: 'folder',
      resourceID: 'platform',
      inherit: true,
      grantedBy: 'admin',
      createdAt: undefined,
    }
  );
});
