import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildCredentialTeamResources,
  buildExternalTriggerTeamResources,
  buildGitWebhookSourceTeamResources,
  buildKnowledgeContextTeamResources,
  buildPipelineTeamResources,
  buildScheduleTeamResources,
  buildScopeTeamResources,
  buildStepTeamResources,
  buildTriggerTeamResources,
  filterApplicationLinkedResources,
  filterTeamLinkedResources,
  teamResourceBelongsToScope,
} from './resourceCatalogModel.js';

test('matches global resources at global and as public resources under team scopes', () => {
  assert.equal(teamResourceBelongsToScope('', ''), true);
  assert.equal(teamResourceBelongsToScope('platform', ''), false);
  assert.equal(teamResourceBelongsToScope('', 'platform'), true);
  assert.equal(teamResourceBelongsToScope('platform/payments', 'platform'), true);
  assert.equal(teamResourceBelongsToScope('platform-payments', 'platform'), false);
  assert.equal(teamResourceBelongsToScope('Platform/API', 'platform'), true);
});

test('builds linked resources with canonical detail links', () => {
  const pipelines = buildPipelineTeamResources([
    { id: 'platform/payments/deploy api', source: 'git' },
    { id: 'global-build' },
  ]);
  assert.deepEqual(pipelines.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['deploy api', 'platform/payments', '/pipelines/platform/payments/deploy%20api'],
    ['global-build', '', '/pipelines/global-build'],
  ]);

  const triggers = buildTriggerTeamResources([
    {
      slug: 'acme/checkout-api',
      source: 'database',
      teamPath: 'platform/payments',
      repositoryForWebhook: 'acme/checkout-api',
    },
  ]);
  assert.deepEqual(triggers.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['checkout-api', 'platform/payments', '/triggers/acme/checkout-api'],
  ]);

  const externalTriggers = buildExternalTriggerTeamResources([
    {
      id: 'platform/release',
      name: 'Release webhook',
      enabled: true,
      pipeline: 'platform/deploy',
      run_team_path: 'platform',
      source: 'database',
    },
  ]);
  assert.deepEqual(externalTriggers.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['Release webhook', 'platform', '/external-triggers/platform/release'],
  ]);

  const webhookSources = buildGitWebhookSourceTeamResources([
    {
      id: 'gitlab-platform',
      name: 'GitLab Platform',
      description: '',
      provider: 'gitlab',
      enabled: true,
      auth_mode: 'hmac',
      repository_allowlist: ['acme/platform'],
      rate_limit: {},
      team_path: 'platform',
    },
  ]);
  assert.deepEqual(webhookSources.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['GitLab Platform', 'platform', '/git-webhook-sources/gitlab-platform'],
  ]);

  const steps = buildStepTeamResources([{ id: 'platform/build', source: 'git' }]);
  assert.deepEqual(steps.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['build', 'platform', '/steps/platform/build'],
  ]);

  const schedules = buildScheduleTeamResources([
    {
      id: 'nightly',
      path: 'platform/nightly',
      name: 'Nightly deploy',
      identifier: 'platform/nightly',
      pipeline: 'platform/deploy',
      timezone: 'UTC',
      enabled: true,
      run_team_path: 'platform',
      visibility: 'team',
    },
  ]);
  assert.deepEqual(schedules.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['Nightly deploy', 'platform', '/schedules?pipeline=platform%2Fdeploy'],
  ]);

  const knowledgeContexts = buildKnowledgeContextTeamResources([
    {
      id: 'runbook/platform/restart',
      kind: 'runbook',
      team: 'platform',
      name: 'restart',
      visibility: 'public',
      source: 'git',
    },
  ]);
  assert.deepEqual(knowledgeContexts.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['restart', '', '/knowledge-context/runbook/platform/restart'],
  ]);

  const scopes = buildScopeTeamResources({
    secrets: [{ scope: 'platform/payments', secret_count: 2 }],
    variables: [{ scope: 'platform' }, 'default'],
  });
  assert.deepEqual(scopes.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['Default Scope', '', '/scopes/default'],
    ['platform', 'platform', '/scopes/platform'],
    ['payments', 'platform/payments', '/scopes/platform/payments'],
  ]);

  const credentials = buildCredentialTeamResources([
    credential('credential://team/platform/payments/openai', 'api_key'),
    credential('credential://system/llm/openai', 'api_key'),
  ]);
  assert.deepEqual(credentials.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['openai', 'platform/payments', '/credentials/team/platform/payments/openai'],
    ['openai', '', '/credentials/system/llm/openai'],
  ]);
});

test('filters and sorts linked resources for the active team subtree', () => {
  const resources = [
    ...buildPipelineTeamResources([{ id: 'deploy' }, { id: 'platform/payments/deploy' }]),
    ...buildTriggerTeamResources([
      { slug: 'platform/root-trigger', source: 'database', teamPath: 'root' },
      { slug: 'platform/payments/checkout-api', source: 'database', teamPath: 'platform' },
      { slug: 'team-1/test-app', source: 'database', teamPath: 'workspace' },
    ]),
    ...buildScopeTeamResources({ secrets: [], variables: [{ scope: 'platform' }] }),
    ...buildCredentialTeamResources([
      credential('credential://team/platform/openai', 'api_key'),
      credential('credential://system/llm/openai', 'api_key'),
    ]),
  ];

  assert.deepEqual(filterTeamLinkedResources(resources, '').map(resource => resource.id), [
    'pipeline:deploy',
    'trigger:platform/root-trigger',
    'scope:default',
    'credential:credential://system/llm/openai',
  ]);
  assert.deepEqual(filterTeamLinkedResources(resources, 'platform').map(resource => resource.id), [
    'pipeline:deploy',
    'pipeline:platform/payments/deploy',
    'trigger:platform/payments/checkout-api',
    'scope:default',
    'scope:platform',
    'credential:credential://team/platform/openai',
  ]);
  assert.deepEqual(filterTeamLinkedResources(resources, 'team-1').map(resource => resource.id), [
    'pipeline:deploy',
    'scope:default',
  ]);
});

test('filters application resources by app path and repository identity only', () => {
  const resources = [
    ...buildPipelineTeamResources([{ id: 'platform/payments/deploy' }, { id: 'platform/payments/checkout-api' }]),
    ...buildTriggerTeamResources([
      {
        slug: 'platform/payments/acme/checkout-api',
        source: 'database',
        teamPath: 'platform/payments',
        repositoryForWebhook: 'acme/checkout-api',
      },
      {
        slug: 'platform/payments/acme/billing-api',
        source: 'database',
        teamPath: 'platform/payments',
        repositoryForWebhook: 'acme/billing-api',
      },
    ]),
    ...buildScopeTeamResources({
      secrets: [{ scope: 'platform/payments', secret_count: 2 }, { scope: 'platform/payments/checkout-api', secret_count: 1 }],
      variables: [{ scope: 'platform' }],
    }),
    ...buildCredentialTeamResources([
      credential('credential://team/platform/payments/openai', 'api_key'),
      credential('credential://team/platform/payments/checkout-api/deploy-token', 'api_key'),
    ]),
  ];

  assert.deepEqual(
    filterApplicationLinkedResources(resources, {
      appPath: 'platform/payments/checkout-api',
      appName: 'checkout-api',
      repository: 'acme/checkout-api',
    }).map(resource => resource.id),
    [
      'pipeline:platform/payments/checkout-api',
      'trigger:platform/payments/acme/checkout-api',
      'scope:platform/payments/checkout-api',
      'credential:credential://team/platform/payments/checkout-api/deploy-token',
    ]
  );
});

test('keeps app repository triggers when Git owner differs from team path', () => {
  const allResources = buildTriggerTeamResources([
    {
      slug: 'nopsai/test-app',
      source: 'database',
      teamPath: 'team-1',
      repositoryForWebhook: 'nopsai/test-app',
    },
    {
      slug: 'nopsai/test-app22',
      source: 'database',
      teamPath: 'team-1',
      repositoryForWebhook: 'nopsai/test-app22',
    },
    {
      slug: 'workspace/test-app22',
      source: 'database',
      teamPath: 'workspace',
      repositoryForWebhook: 'workspace/test-app22',
    },
  ]);
  const teamResources = filterTeamLinkedResources(
    allResources,
    'team-1'
  );

  assert.deepEqual(teamResources.map(resource => [resource.id, resource.teamPath]), [
    ['trigger:nopsai/test-app', 'team-1'],
    ['trigger:nopsai/test-app22', 'team-1'],
  ]);
  assert.deepEqual(
    filterApplicationLinkedResources(teamResources, {
      appPath: 'team-1/test-app',
      appName: 'test-app',
      repository: 'nopsai/test-app',
    }).map(resource => resource.id),
    ['trigger:nopsai/test-app']
  );
  assert.deepEqual(
    filterApplicationLinkedResources(allResources, {
      appPath: 'team-1/t-app',
      appName: 't-app',
      repository: 'nopsai/t-app',
    }).map(resource => resource.id),
    []
  );
});

function credential(reference: string, kind: string) {
  return {
    id: reference,
    reference,
    kind,
    description: '',
    status: 'active',
    has_value: true,
    active_version: 1,
    managed_by_config_repo: false,
    created_at: '2026-07-12T00:00:00Z',
    updated_at: '2026-07-12T00:00:00Z',
    versions: [],
  };
}
