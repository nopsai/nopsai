import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  assistantPageContextFromOption,
  filterAssistantContextOptions,
  type AssistantContextOption,
} from './contextOptions.js';
import { assistantPageContextIsEmpty, assistantPageContextLabel } from './pageContext.js';

function option(overrides: Partial<AssistantContextOption>): AssistantContextOption {
  return { kind: 'pipeline', id: '', label: '', detail: '', scope: '', pipeline: '', ...overrides };
}

describe('assistant context options', () => {
  it('matches every visible part of a row so a scoped name can be found by its path', () => {
    const options = [
      option({ id: 'platform/deploy-api', label: 'deploy-api', detail: 'platform/deploy-api', scope: 'platform', pipeline: 'platform/deploy-api' }),
      option({ id: 'prod/deploy-web', label: 'deploy-web', detail: 'prod/deploy-web', scope: 'prod', pipeline: 'prod/deploy-web' }),
    ];

    assert.deepEqual(filterAssistantContextOptions(options, 'platform deploy').map(row => row.id), ['platform/deploy-api']);
    assert.equal(filterAssistantContextOptions(options, '  ').length, 2);
    assert.equal(filterAssistantContextOptions(options, 'missing').length, 0);
  });

  it('builds a pipeline context the planner can ground on', () => {
    const context = assistantPageContextFromOption(option({
      id: 'nopsai/nopsai-platform-release',
      label: 'nopsai-platform-release',
      detail: 'nopsai/nopsai-platform-release',
      scope: 'nopsai',
      pipeline: 'nopsai/nopsai-platform-release',
    }));

    assert.equal(context.resource_type, 'pipeline');
    assert.equal(context.pipeline_id, 'nopsai/nopsai-platform-release');
    assert.equal(context.scope, 'nopsai');
    assert.equal(context.params.pipeline_id, 'nopsai/nopsai-platform-release');
    assert.equal(assistantPageContextIsEmpty(context), false);
    assert.equal(assistantPageContextLabel(context), 'Pipelines · nopsai-platform-release · /nopsai');
  });

  it('keeps a run pointing at its own id and its pipeline', () => {
    const context = assistantPageContextFromOption(option({
      kind: 'pipeline_run',
      id: '00000000-0000-0000-0000-000000000123',
      label: 'deploy-api · 00000000…',
      detail: 'platform/deploy-api · failure',
      scope: 'platform',
      pipeline: 'platform/deploy-api',
    }));

    assert.equal(context.resource_type, 'pipeline_run');
    assert.equal(context.run_id, '00000000-0000-0000-0000-000000000123');
    assert.equal(context.pipeline_id, 'platform/deploy-api');
    assert.equal(context.params.run_id, '00000000-0000-0000-0000-000000000123');
  });

  it('builds scope, team and schedule contexts from their own identifiers', () => {
    const scope = assistantPageContextFromOption(option({ kind: 'scope', id: 'prod/api', label: '/prod/api', scope: 'prod/api' }));
    assert.equal(scope.resource_type, 'scope');
    assert.equal(scope.scope, 'prod/api');

    const team = assistantPageContextFromOption(option({ kind: 'team', id: 'platform/ml', label: '/platform/ml', scope: 'platform/ml' }));
    assert.equal(team.team_path, 'platform/ml');
    assert.equal(team.scope, 'platform/ml');

    const schedule = assistantPageContextFromOption(option({
      kind: 'schedule',
      id: 'nightly-release',
      label: 'nightly-release',
      detail: 'nopsai/nopsai-platform-release',
      scope: 'nopsai',
      pipeline: 'nopsai/nopsai-platform-release',
    }));
    assert.equal(schedule.resource_type, 'schedule');
    assert.equal(schedule.resource_id, 'nightly-release');
    assert.equal(schedule.pipeline_id, 'nopsai/nopsai-platform-release');
  });
});
