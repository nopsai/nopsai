import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildAAAResourceSelector,
  formatAccessActionSummary,
  formatAccessResourceSummary,
  normalizeAAAActionForResource,
  parseAAAActionValue,
  parseAAAResourceSelector,
} from './policyRuleModel.js';

test('round-trips supported Access resource selectors', () => {
  assert.deepEqual(parseAAAResourceSelector('pipeline:platform/build'), {
    resourceType: 'pipeline',
    resourceID: 'platform/build',
  });
  assert.deepEqual(parseAAAResourceSelector('system:agent-roles'), {
    resourceType: 'system',
    resourceID: 'agent-roles',
  });
  assert.equal(buildAAAResourceSelector('pipeline', '*'), 'pipeline:*');
  assert.equal(buildAAAResourceSelector('model', 'hosted'), 'model:hosted');
  assert.equal(formatAccessResourceSummary('secret:*'), 'all secret');
  assert.equal(formatAccessResourceSummary('mcp_profile:github-pr-review'), 'mcp profile github-pr-review');
  assert.equal(formatAccessResourceSummary('credential:system/llm/openai'), 'credential system/llm/openai');
  assert.equal(formatAccessResourceSummary('system:agent-roles'), 'system agent-roles');
  assert.equal(formatAccessResourceSummary('git_webhook_source:gitlab-platform'), 'git webhook source gitlab-platform');
});

test('normalizes Access actions for the selected resource and effect', () => {
  assert.deepEqual(parseAAAActionValue('deny pipeline.delete'), {
    effect: 'deny',
    action: 'pipeline.delete',
  });
  assert.equal(
    normalizeAAAActionForResource('scope:prod', 'pipeline.read', 'deny'),
    'deny scope.read'
  );
  assert.equal(formatAccessActionSummary('deny secret.read_value'), 'deny read value');
  assert.equal(
    normalizeAAAActionForResource('git_webhook_source:gitlab-platform', 'pipeline.read', 'allow'),
    'git_webhook_source.read'
  );
  assert.equal(
    normalizeAAAActionForResource('agent_role:sre', 'pipeline.read', 'allow'),
    'agent_role.read'
  );
  assert.equal(
    normalizeAAAActionForResource('credential:system/llm/openai', 'pipeline.read', 'allow'),
    'credential.list_metadata'
  );
});
