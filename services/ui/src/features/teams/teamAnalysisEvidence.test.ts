import assert from 'node:assert/strict';
import test from 'node:test';
import { buildTeamAnalysisPromptContext } from './teamAnalysisEvidence.js';
import type { TeamLinkedResource } from './resourceCatalogModel.js';

const operationsSummary = {
  teamPath: 'platform/payments',
  loading: false,
  configRepo: {
    id: 1,
    scope_type: 'team',
    scope_id: 'platform/payments',
    provider: 'github',
    repo_url: 'https://github.com/acme/config password=never-show-this',
    branch: 'main',
    base_path: 'teams/platform/payments',
    credential_ref: 'credential://team/platform/payments/git-token',
    enabled: true,
    write_enabled: false,
    write_branch: 'nopsai/ui-changes',
    last_sync_status: 'success',
    last_sync_completed_at: '2026-07-24T10:00:00Z',
  },
  configRepoError: null,
  notificationRoute: {
    team_path: 'platform/payments',
    definition: {
      enabled: true,
      recipients: { include: { teams: ['platform/payments'] } },
      events: {
        failure: true,
        success: false,
        pending: false,
        running: false,
        waiting_approval: true,
        approval_requested: true,
        approval_approved: false,
        approval_rejected: true,
        cancelled: true,
        skipped: false,
      },
      filters: {},
      delivery: { channels: ['slack'] },
      routes: [{
        name: 'release-failures',
        enabled: true,
        recipients: { include: { teams: ['platform/payments'] } },
        events: {
          failure: true,
          success: false,
          pending: false,
          running: false,
          waiting_approval: false,
          approval_requested: false,
          approval_approved: false,
          approval_rejected: false,
          cancelled: true,
          skipped: false,
        },
        filters: { pipelines: { include: ['deploy*'] } },
        delivery: { channels: ['slack'], throttle: { dedupe_window: '15m', max_per_run: 2 } },
      }],
    },
    source: 'git',
  },
  notificationError: null,
  llmProfiles: {
    team_id: 1,
    team_path: 'platform/payments',
    default_profile: 'payments-reviewer',
    profiles: [{
      name: 'payments-reviewer',
      provider: 'openai',
      model: 'gpt-5',
      credential_ref: 'credential://team/platform/payments/openai',
      allowed_scopes: ['platform/payments'],
      scope: 'team',
      team_id: 1,
      team_path: 'platform/payments',
      status: 'valid',
      allowed_in_scope: true,
    }],
  },
  agentProfiles: {
    team_id: 1,
    team_path: 'platform/payments',
    default_profile: 'release-agent',
    profiles: [{
      id: 'release-agent',
      display_name: 'Release Agent',
      instructions: 'Keep deployment actions reviewable.',
      scope: 'team',
      team_id: 1,
      team_path: 'platform/payments',
      enabled: true,
      source: 'git',
    }],
  },
  mcpProfiles: {
    team_id: 1,
    team_path: 'platform/payments',
    profiles: [{
      name: 'readonly-prod',
      description: 'Read-only production evidence',
      enabled: true,
      servers: [{ server: 'logs', tools: ['search'] }],
      scope: 'team',
      team_id: 1,
      team_path: 'platform/payments',
    }],
  },
  aiProfilesError: null,
  accessGrants: [{
    id: 'grant-1',
    subjectType: 'team',
    subjectID: 'sre',
    role: 'nopsai-operator',
    resourceType: 'team',
    resourceID: 'platform/payments',
    inherit: true,
  }],
  accessGrantsError: null,
  permissions: [
    { action: 'team.read', label: 'View team', allowed: true },
    { action: 'config_repo.sync', label: 'Sync GitOps', allowed: false },
  ],
  permissionsError: null,
};

test('builds team AI context from visible resources and operations metadata', () => {
  const resources: TeamLinkedResource[] = [
    {
      id: 'pipeline:platform/payments/deploy',
      kind: 'pipeline',
      label: 'deploy',
      description: 'Pipeline in platform/payments password=never-show-this',
      href: '/pipelines/platform/payments/deploy',
      teamPath: 'platform/payments',
      source: 'git',
    },
    {
      id: 'pipeline:platform/payments/deploy-copy',
      kind: 'pipeline',
      label: 'deploy copy',
      description: 'Pipeline in platform/payments',
      href: '/pipelines/platform/payments/deploy-copy',
      teamPath: 'platform/payments',
      source: 'database',
    },
    {
      id: 'credential:credential://team/platform/payments/prod-admin',
      kind: 'credential',
      label: 'prod-admin',
      description: 'production admin credential token=never-show-this',
      href: '/credentials?credential=credential%3A%2F%2Fteam%2Fplatform%2Fpayments%2Fprod-admin',
      teamPath: 'platform/payments',
      source: 'database',
    },
  ];
  const context = buildTeamAnalysisPromptContext({
    team: {
      id: 1,
      name: 'payments',
      kind: 'team',
      path: 'platform/payments',
      description: 'Payments platform team',
      repository_full_name: 'acme/payments-config',
    },
    teams: [],
    stats: {
      teams: 1,
      applications: 2,
      repositories: 1,
      recentRuns: 1,
      directChildren: 2,
      totalItems: 3,
    },
    subjectLabel: 'Payments',
    scopePath: 'platform/payments',
    resources,
    activeResource: resources[0],
    operationsSummary,
    resourceCatalog: {
      teamPath: 'platform/payments',
      loading: false,
      error: null,
      resources,
    },
  });

  const serialized = JSON.stringify(context);
  assert.match(serialized, /Team\/resource page snapshot/);
  assert.match(serialized, /Selected resource context/);
  assert.match(serialized, /Visible resource distribution/);
  assert.match(serialized, /Team operations metadata/);
  assert.match(serialized, /Visible resource rows/);
  assert.match(serialized, /payments-reviewer/);
  assert.match(serialized, /readonly-prod/);
  assert.match(serialized, /deploy-copy/);
  assert.match(serialized, /team\.read/);
  assert.doesNotMatch(serialized, /never-show-this/);
  assert.match(serialized, /\[redacted\]/);
});
