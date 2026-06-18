import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildEditorScopeList,
  normalizeAutocompleteList,
  normalizeProfilePayload,
  normalizeRuntimePoolNames,
  normalizeScopeLabel,
} from './autocomplete.js';

test('normalizes editor autocomplete list payloads', () => {
  assert.deepEqual(
    normalizeAutocompleteList([' TOKEN ', { name: 'DEPLOY_ENV' }, { id: 'agent-profile' }, '']),
    ['TOKEN', 'DEPLOY_ENV', 'agent-profile']
  );
  assert.deepEqual(normalizeAutocompleteList(null), []);
});

test('normalizes profile payload envelopes', () => {
  assert.deepEqual(normalizeProfilePayload({ profiles: [{ name: 'standard' }, { name: 'blocked', allowed_in_scope: false }] }), ['standard']);
  assert.deepEqual(normalizeProfilePayload({ profiles: [{ id: 'devops-engineer' }, { id: 'disabled', enabled: false }] }), ['devops-engineer']);
  assert.deepEqual(normalizeProfilePayload(['github-pr-review']), ['github-pr-review']);
});

test('normalizes scope labels and builds the union scope list', () => {
  assert.equal(normalizeScopeLabel(' default '), '');
  assert.equal(normalizeScopeLabel('/platform/dev/'), 'platform/dev');
  assert.equal(normalizeScopeLabel({ value: 'prod' }), 'prod');
  assert.deepEqual(buildEditorScopeList(['default', 'platform'], [{ scope: 'prod' }, { name: '/platform/' }]), ['', 'platform', 'prod']);
});

test('normalizes runtime pool names from system config payloads', () => {
  assert.deepEqual(
    normalizeRuntimePoolNames({ runtime_pools: { ' high-memory ': {}, default: {}, '': {} } }),
    ['default', 'high-memory']
  );
  assert.deepEqual(normalizeRuntimePoolNames(['default', { name: 'gpu' }]), ['default', 'gpu']);
  assert.deepEqual(normalizeRuntimePoolNames(null), []);
});
