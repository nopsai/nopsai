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
  assert.deepEqual(parseAAAResourceSelector('system:agent-profiles'), {
    resourceType: 'system',
    resourceID: 'agent-profiles',
  });
  assert.equal(buildAAAResourceSelector('pipeline', '*'), 'pipeline:*');
  assert.equal(formatAccessResourceSummary('secret:*'), 'all secret');
  assert.equal(formatAccessResourceSummary('system:agent-profiles'), 'system agent-profiles');
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
});
