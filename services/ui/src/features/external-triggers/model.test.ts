import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildExternalTriggerTreeItems,
  buildExternalTriggerCollectionMetrics,
  externalTriggerBelongsToTeam,
  externalTriggerTeamLabel,
  externalTriggerTeamPath,
  externalTriggerRelativeLabel,
  externalTriggerScopeLabel,
  filterExternalTriggers,
} from './model.js';

test('formats external trigger scope and team labels', () => {
  assert.equal(externalTriggerScopeLabel(), 'default');
  assert.equal(externalTriggerScopeLabel('default'), 'default');
  assert.equal(externalTriggerScopeLabel('.nopsai/pipelines/platform.yaml'), 'platform');
  assert.equal(externalTriggerTeamLabel(), 'Root');
  assert.equal(externalTriggerTeamLabel('root'), 'Root');
  assert.equal(externalTriggerTeamLabel('platform/prod'), 'platform/prod');
  assert.equal(externalTriggerTeamPath('root'), '');
  assert.equal(externalTriggerTeamPath('.nopsai/external-triggers/platform/prod.yaml'), 'platform/prod');
});

test('formats external trigger relative timestamps', () => {
  const now = Date.parse('2026-06-15T12:00:00Z');
  assert.equal(externalTriggerRelativeLabel(undefined, now), 'Never');
  assert.equal(externalTriggerRelativeLabel('invalid', now), 'Never');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T12:00:30Z', now), 'just now');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T11:59:30Z', now), 'just now');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T11:30:00Z', now), '30m ago');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T10:00:00Z', now), '2h ago');
  assert.equal(externalTriggerRelativeLabel('2026-06-13T12:00:00Z', now), '2d ago');
});

test('builds external trigger workspace metrics and filters by searchable fields', () => {
  const triggers = [
    {
      id: 'servicenow-approved',
      name: 'ServiceNow approved',
      enabled: true,
      pipeline: 'platform/deploy',
      allowed_callers: [{ type: 'service_account' as const, id: 'servicenow' }],
      managed_by_config_repo: true,
    },
    {
      id: 'deploy-dev',
      name: 'Deploy dev',
      enabled: false,
      pipeline: 'platform/dev',
      source: 'database',
    },
  ];

  assert.deepEqual(buildExternalTriggerCollectionMetrics(triggers), {
    total: 2,
    enabled: 1,
    gitManaged: 1,
    callerPolicies: 1,
  });
  assert.deepEqual(filterExternalTriggers(triggers, 'servicenow').map(trigger => trigger.id), ['servicenow-approved']);
  assert.deepEqual(filterExternalTriggers(triggers, 'GitOps').map(trigger => trigger.id), ['servicenow-approved']);
});

test('builds external trigger team tree items and filters team subtrees', () => {
  const triggers = [
    {
      id: 'prod',
      name: 'Prod deploy',
      enabled: true,
      pipeline: 'platform/deploy',
      run_team_path: 'platform/prod',
    },
    {
      id: 'dev',
      name: '',
      enabled: true,
      pipeline: 'platform/dev',
      run_team_path: 'platform/dev',
      source: 'git',
    },
    {
      id: 'global',
      name: 'Global deploy',
      enabled: true,
      pipeline: 'global',
      run_team_path: 'root',
    },
  ];

  assert.deepEqual(buildExternalTriggerTreeItems(triggers), [
    { id: 'dev', label: 'dev', path: 'platform/dev', source: 'git' },
    { id: 'global', label: 'Global deploy', path: '', source: undefined },
    { id: 'prod', label: 'Prod deploy', path: 'platform/prod', source: undefined },
  ]);
  assert.deepEqual(
    triggers.filter(trigger => externalTriggerBelongsToTeam(trigger, 'platform')).map(trigger => trigger.id),
    ['prod', 'dev']
  );
  assert.deepEqual(
    triggers.filter(trigger => externalTriggerBelongsToTeam(trigger, 'platform/prod')).map(trigger => trigger.id),
    ['prod']
  );
});
