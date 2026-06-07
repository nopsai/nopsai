import assert from 'node:assert/strict';
import { test } from 'node:test';
import { can, getAppAccess, normalizeCurrentUser } from './capabilities.js';

test('normalizes API capability payloads into UI capabilities', () => {
  const user = normalizeCurrentUser({
    sub: 'admin',
    email: 'admin@example.test',
    roles: ['nopsai-admin', 42],
    capabilities: {
      pipelines: { write: 1, delete: 0 },
      schedules: { read: true, write: false, delete: true },
      system: {
        config_read: true,
        config_write: false,
        llm_profiles_read: false,
        mcp_write: true,
        access: true,
      },
    },
  });

  assert.equal(user.sub, 'admin');
  assert.deepEqual(user.roles, ['nopsai-admin']);
  assert.equal(can(user, 'pipelines.write'), true);
  assert.equal(can(user, 'pipelines.delete'), false);
  assert.equal(can(user, 'schedules.delete'), true);
  assert.equal(can(user, 'system.config.read'), true);
  assert.equal(can(user, 'system.mcp.write'), true);
});

test('derives app and system access from normalized capabilities', () => {
  const user = normalizeCurrentUser({
    sub: 'operator',
    capabilities: {
      schedules: { read: true },
      knowledge_contexts: { read: true, write: true },
      system: {
        config_read: true,
        config_repos_read: true,
        dispatcher_read: true,
      },
    },
  });

  const access = getAppAccess(user, { sub: 'operator' });

  assert.equal(access.canViewSchedules, true);
  assert.equal(access.canWriteSchedules, false);
  assert.equal(access.canViewKnowledge, true);
  assert.equal(access.canWriteKnowledge, true);
  assert.equal(access.canViewAnySystem, true);
  assert.equal(access.preferredSystemPath, '/system/config');
  assert.equal(access.systemPermissions.canViewLLMProfiles, true);
  assert.equal(access.systemPermissions.canManageLLMProfiles, false);
  assert.equal(access.systemPermissions.canViewDataManagement, true);
  assert.equal(access.systemPermissions.canManageDataManagement, false);
});
