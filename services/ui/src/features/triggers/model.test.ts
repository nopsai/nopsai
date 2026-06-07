import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildTriggerSummary, deriveDefaultPipelinePath, parseTriggerOverrideList, parseTriggerYaml } from './model.js';

test('normalizes trigger manifests and linked pipeline summaries', () => {
  const summary = buildTriggerSummary(
    parseTriggerYaml(`
triggers:
  - on: push
    branches: [main]
    pipelines:
      - pipelines/platform/deploy.yaml
    scope: default
`)
  );
  assert.equal(summary.triggerCount, 1);
  assert.deepEqual(summary.pipelines.map(item => item.identifier), ['platform/deploy']);
  assert.deepEqual(summary.scopes, ['']);
});

test('normalizes trigger lists and default pipeline paths', () => {
  assert.deepEqual(parseTriggerOverrideList(['acme/api', { name: 'acme/web', source: 'gitops' }]), [
    { slug: 'acme/api', source: 'database' },
    { slug: 'acme/web', source: 'git' },
  ]);
  assert.equal(deriveDefaultPipelinePath('acme/payment api'), 'pipelines/payment-api.yaml');
});
