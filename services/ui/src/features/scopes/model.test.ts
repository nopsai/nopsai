import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildScopeTree,
  decodeScopeFromRoute,
  encodeScopeForRoute,
  normalizeItemListPayload,
  normalizeRepositorySlug,
  normalizeScopeLabel,
  parseScopedIdentity,
  suggestCloneName,
} from './model.js';

test('normalizes scope routes and repository identities', () => {
  assert.equal(normalizeScopeLabel('/Default/'), '');
  assert.equal(encodeScopeForRoute('teams/platform'), 'teams/platform');
  assert.equal(decodeScopeFromRoute(['teams', 'platform']), 'teams/platform');
  assert.equal(normalizeRepositorySlug('/acme/control-plane/'), 'acme/control-plane');
  assert.deepEqual(parseScopedIdentity('acme/control-plane/token'), {
    repoOwner: 'acme',
    repoName: 'control-plane',
    repoSlug: 'acme/control-plane',
    name: 'token',
    fullName: 'acme/control-plane/token',
  });
});

test('normalizes scoped item metadata and clone names', () => {
  const result = normalizeItemListPayload([
    'GLOBAL_TOKEN',
    { name: 'acme/app/API_KEY', source: 'git repository', updated_at: '2026-06-07T10:00:00Z' },
    'GLOBAL_TOKEN',
  ]);
  assert.deepEqual(result.names, ['acme/app/API_KEY', 'GLOBAL_TOKEN']);
  assert.equal(result.meta['acme/app/API_KEY']?.source, 'git');
  assert.equal(suggestCloneName(['acme/app/token_copy'], 'acme/app', 'token'), 'token_copy_2');
});

test('builds scope trees with empty enterprise group folders', () => {
  const root = buildScopeTree(
    [{ scope: '', label: 'Default', folderPath: '', description: '', secretCountHint: 0 }],
    ['teams/platform']
  );
  assert.deepEqual(root.scopes, ['']);
  assert.equal(root.children[0]?.fullPath, 'teams');
  assert.equal(root.children[0]?.children[0]?.fullPath, 'teams/platform');
});
