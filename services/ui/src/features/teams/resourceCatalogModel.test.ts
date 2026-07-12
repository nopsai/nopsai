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
    { slug: 'platform/payments/acme/checkout-api', source: 'database' },
  ]);
  assert.deepEqual(triggers.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['checkout-api', 'platform/payments/acme', '/triggers/platform/payments/acme/checkout-api'],
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
    },
  ]);
  assert.deepEqual(webhookSources.map(resource => [resource.label, resource.teamPath, resource.href]), [
    ['GitLab Platform', '', '/git-webhook-sources/gitlab-platform'],
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
    ['openai', 'platform/payments', '/credentials?credential=credential%3A%2F%2Fteam%2Fplatform%2Fpayments%2Fopenai'],
    ['openai', '', '/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai'],
  ]);
});

test('filters and sorts linked resources for the active team subtree', () => {
  const resources = [
    ...buildPipelineTeamResources([{ id: 'deploy' }, { id: 'platform/payments/deploy' }]),
    ...buildScopeTeamResources({ secrets: [], variables: [{ scope: 'platform' }] }),
    ...buildCredentialTeamResources([credential('credential://team/platform/openai', 'api_key')]),
  ];

  assert.deepEqual(filterTeamLinkedResources(resources, '').map(resource => resource.id), [
    'pipeline:deploy',
    'scope:default',
  ]);
  assert.deepEqual(filterTeamLinkedResources(resources, 'platform').map(resource => resource.id), [
    'pipeline:deploy',
    'pipeline:platform/payments/deploy',
    'scope:default',
    'scope:platform',
    'credential:credential://team/platform/openai',
  ]);
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
