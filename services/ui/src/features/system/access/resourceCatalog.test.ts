import assert from 'node:assert/strict';
import test from 'node:test';
import { buildAccessResourceCatalog } from './resourceCatalog.js';

test('builds and sorts Access resource options from API catalog sources', () => {
  const catalog = buildAccessResourceCatalog({
    groups: [
      { id: 2, name: 'payments', parent_id: 1 },
      { id: 1, name: 'platform' },
    ],
    pipelines: ['platform/deploy', 'platform/deploy', 'payments/reconcile'],
    triggers: ['acme/release'],
    externalTriggers: [{ id: 'deploy-hook' }, { name: 'release-hook' }],
    secretScopes: [{ scope: 'prod' }, { scope: '' }],
    variableScopes: [{ scope: 'staging' }, { scope: 'default' }],
  });

  assert.deepEqual(catalog.folderOptions, [
    { value: 'platform', label: '/platform' },
    { value: 'platform/payments', label: '/platform/payments' },
  ]);
  assert.deepEqual(catalog.pipelineOptions.map(option => option.value), ['payments/reconcile', 'platform/deploy']);
  assert.deepEqual(catalog.externalTriggerOptions.map(option => option.value), ['deploy-hook', 'release-hook']);
  assert.deepEqual(catalog.scopeOptions.map(option => option.value), ['default', 'prod', 'staging']);
  assert.deepEqual(catalog.secretScopeOptions.map(option => option.label), ['Default scope', 'prod', 'staging']);
  assert.strictEqual(catalog.repositoryOptions, catalog.triggerOptions);
});

test('ignores malformed catalog records and breaks cyclic group paths', () => {
  const catalog = buildAccessResourceCatalog({
    groups: [
      { id: 1, name: 'one', parent_id: 2 },
      { id: 2, name: 'two', parent_id: 1 },
      { id: 'bad', name: 'ignored' },
    ],
    pipelines: [null, 42, 'valid'],
    triggers: [],
    externalTriggers: [{ unknown: true }],
    secretScopes: [{ scope: 12 }],
    variableScopes: [],
  });

  assert.deepEqual(catalog.folderOptions.map(option => option.value), ['one/two', 'two/one']);
  assert.deepEqual(catalog.pipelineOptions.map(option => option.value), ['valid']);
  assert.deepEqual(catalog.externalTriggerOptions, []);
});
