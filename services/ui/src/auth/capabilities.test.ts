import assert from 'node:assert/strict';
import { test } from 'node:test';
import { can, getAppAccess, normalizeCurrentUser } from './capabilities.js';

test('normalizes API capability payloads into UI capabilities', () => {
  const user = normalizeCurrentUser({
    sub: 'admin',
    email: 'admin@example.test',
    display_name: 'Admin User',
    roles: ['nopsai-admin', 42],
    capabilities: {
      pipelines: { write: 1, delete: 0 },
      schedules: { read: true, write: false, delete: true },
      git_webhook_sources: { read: true, write: true, delete: false },
      knowledge_connections: { read: true, write: true, delete: false },
      system: {
        config_read: true,
        config_write: false,
        llm_profiles_read: false,
        agent_profiles_read: true,
        agent_profiles_write: false,
        mcp_write: true,
        credentials_read: true,
        credentials_write: false,
        logs_read: true,
        access: true,
      },
    },
  });

  assert.equal(user.sub, 'admin');
  assert.equal(user.displayName, 'Admin User');
  assert.deepEqual(user.roles, ['nopsai-admin']);
  assert.equal(can(user, 'pipelines.write'), true);
  assert.equal(can(user, 'pipelines.delete'), false);
  assert.equal(can(user, 'schedules.delete'), true);
  assert.equal(can(user, 'git_webhook_sources.read'), true);
  assert.equal(can(user, 'git_webhook_sources.write'), true);
  assert.equal(can(user, 'git_webhook_sources.delete'), false);
  assert.equal(can(user, 'knowledge_connections.read'), true);
  assert.equal(can(user, 'knowledge_connections.write'), true);
  assert.equal(can(user, 'knowledge_connections.delete'), false);
  assert.equal(can(user, 'system.config.read'), true);
  assert.equal(can(user, 'system.agent_profiles.read'), true);
  assert.equal(can(user, 'system.agent_profiles.write'), false);
  assert.equal(can(user, 'system.mcp.write'), true);
  assert.equal(can(user, 'system.credentials.read'), true);
  assert.equal(can(user, 'system.credentials.write'), false);
  assert.equal(can(user, 'system.logs.read'), true);
});

test('derives app and system access from normalized capabilities', () => {
  const user = normalizeCurrentUser({
    sub: 'operator',
    capabilities: {
      schedules: { read: true },
      dashboards: { read: true, write: true, delete: false },
      git_webhook_sources: { read: true, write: false },
      knowledge_contexts: { read: true, write: true },
      knowledge_connections: { read: true, write: false },
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
  assert.equal(access.canViewDashboards, true);
  assert.equal(access.canWriteDashboards, true);
  assert.equal(access.canDeleteDashboards, false);
  assert.equal(access.canViewGitWebhookSources, true);
  assert.equal(access.canWriteGitWebhookSources, false);
  assert.equal(access.canViewKnowledge, true);
  assert.equal(access.canWriteKnowledge, true);
  assert.equal(access.canViewKnowledgeConnections, true);
  assert.equal(access.canWriteKnowledgeConnections, true);
  assert.equal(access.canViewAnySystem, true);
  assert.equal(access.isNopsAIAdmin, false);
  assert.equal(access.preferredSystemPath, '/system/config');
  assert.equal(access.systemPermissions.canViewLLMProfiles, true);
  assert.equal(access.systemPermissions.canManageLLMProfiles, false);
  assert.equal(access.systemPermissions.canViewAgentProfiles, true);
  assert.equal(access.systemPermissions.canManageAgentProfiles, false);
  assert.equal(access.systemPermissions.canViewDataManagement, true);
  assert.equal(access.systemPermissions.canManageDataManagement, false);
  assert.equal(access.systemPermissions.canViewGitApps, true);
  assert.equal(access.systemPermissions.canManageGitApps, false);
});

test('keeps profile-only capabilities out of the System area', () => {
  const user = normalizeCurrentUser({
    sub: 'operator',
    capabilities: {
      system: {
        agent_profiles_read: true,
        agent_profiles_write: true,
      },
    },
  });

  const access = getAppAccess(user, { sub: 'operator' });

  assert.equal(access.canViewAnySystem, false);
  assert.equal(access.preferredSystemPath, '/system/config');
  assert.equal(access.canViewSystemAgentProfiles, true);
  assert.equal(access.canManageSystemAgentProfiles, true);
});

test('exposes the credential registry only through credential capabilities', () => {
  const user = normalizeCurrentUser({
    sub: 'security-admin',
    capabilities: {
      system: {
        credentials_read: true,
        credentials_write: true,
      },
    },
  });

  const access = getAppAccess(user, { sub: 'security-admin' });

  assert.equal(access.canViewSystemCredentials, true);
  assert.equal(access.canManageSystemCredentials, true);
  assert.equal(access.isNopsAIAdmin, false);
  assert.equal(access.canViewAnySystem, false);
  assert.equal(access.preferredSystemPath, '/system/config');
});

test('detects NopsAI admin users independently from credential capabilities', () => {
  const user = normalizeCurrentUser({
    sub: 'security-admin',
    roles: ['nopsai-admin'],
    capabilities: {
      system: {
        credentials_read: true,
      },
    },
  });

  const access = getAppAccess(user, { sub: 'security-admin' });

  assert.equal(access.canViewSystemCredentials, true);
  assert.equal(access.canManageSystemCredentials, false);
  assert.equal(access.isNopsAIAdmin, true);
  assert.equal(access.isInitialAdminUser, true);
});
