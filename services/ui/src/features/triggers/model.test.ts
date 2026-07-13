import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildTriggerSummary,
  buildTriggerCollectionMetrics,
  deriveDefaultPipelinePath,
  encodeTriggerSlug,
  filterTriggerListItems,
  parseTriggerOverrideList,
  triggerBelongsToOwner,
  parseTriggerYaml,
  splitTriggerSlug,
  triggerSlugLabel,
  validateTriggerYaml,
} from './model.js';

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

test('builds owner-scoped trigger collection metrics', () => {
  const triggers = [
    { slug: 'platform/api', source: 'gitops' },
    { slug: 'platform/web', source: 'database' },
    { slug: 'platform/apps/checkout', source: 'git' },
    { slug: 'security/audit', source: 'database' },
  ];

  assert.equal(triggerBelongsToOwner('platform/apps/checkout', 'platform'), true);
  assert.equal(triggerBelongsToOwner('security/audit', 'platform'), false);
  assert.deepEqual(buildTriggerCollectionMetrics(triggers, 'platform'), {
    total: 3,
    gitManaged: 2,
    databaseManaged: 1,
    ownerCount: 2,
  });
});

test('filters trigger list items by query and source label', () => {
  const triggers = [
    { slug: 'platform/api', source: 'gitops' },
    { slug: 'platform/web', source: 'database' },
    { slug: 'security/audit', source: 'database' },
  ];

  assert.deepEqual(filterTriggerListItems(triggers, { query: 'git', source: 'all' }), [
    { slug: 'platform/api', source: 'gitops' },
  ]);
  assert.deepEqual(filterTriggerListItems(triggers, { query: 'platform', source: 'database' }), [
    { slug: 'platform/web', source: 'database' },
  ]);
});

test('validates trigger manifests and repository routes', () => {
  assert.deepEqual(splitTriggerSlug('acme/platform/api'), { owner: 'acme/platform', repo: 'api' });
  assert.equal(encodeTriggerSlug('acme/platform api'), 'acme/platform%20api');
  assert.deepEqual(triggerSlugLabel('acme/platform/api'), { name: 'api', path: 'acme/platform' });

  const valid = validateTriggerYaml(`
triggers:
  - on: push
    branches: [main]
    pipelines:
      - pipelines/platform/deploy.yaml
    scope: production
`);
  assert.deepEqual(valid.errors, []);

  const invalid = validateTriggerYaml(`
steps: []
triggers:
  - on: unsupported
    branches: [""]
    pipelines: []
    scope: ""
`);
  assert.ok(invalid.errors.some(error => error.message.includes('appears to be a pipeline')));
  assert.ok(invalid.errors.some(error => error.message.includes("unsupported event 'unsupported'")));
  assert.ok(invalid.errors.some(error => error.message.includes('at least one pipeline reference')));
  assert.ok(invalid.errors.some(error => error.message.includes('empty branches entry')));
  assert.ok(invalid.errors.some(error => error.message.includes("empty 'scope'")));
});
