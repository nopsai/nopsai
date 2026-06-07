import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  buildEditorScopeList,
  normalizeAutocompleteList,
  normalizeProfilePayload,
  normalizeScopeLabel,
} from './autocomplete.js';

test('normalizes editor autocomplete list payloads', () => {
  assert.deepEqual(
    normalizeAutocompleteList([' TOKEN ', { name: 'DEPLOY_ENV' }, { id: 'ignored' }, '']),
    ['TOKEN', 'DEPLOY_ENV']
  );
  assert.deepEqual(normalizeAutocompleteList(null), []);
});

test('normalizes profile payload envelopes', () => {
  assert.deepEqual(normalizeProfilePayload({ profiles: [{ name: 'standard' }, { name: 'reasoning' }] }), ['standard', 'reasoning']);
  assert.deepEqual(normalizeProfilePayload(['github-pr-review']), ['github-pr-review']);
});

test('normalizes scope labels and builds the union scope list', () => {
  assert.equal(normalizeScopeLabel(' default '), '');
  assert.equal(normalizeScopeLabel('/platform/dev/'), 'platform/dev');
  assert.equal(normalizeScopeLabel({ value: 'prod' }), 'prod');
  assert.deepEqual(buildEditorScopeList(['default', 'platform'], [{ scope: 'prod' }, { name: '/platform/' }]), ['', 'platform', 'prod']);
});
