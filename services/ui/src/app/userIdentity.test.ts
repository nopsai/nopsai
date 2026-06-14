import assert from 'node:assert/strict';
import { test } from 'node:test';
import { currentUserDisplayName, currentUserInitials } from './userIdentity.js';

test('prefers email over opaque OIDC subject for display name', () => {
  assert.equal(
    currentUserDisplayName({
      sub: 'oidc:nopsai:d0ebc10d-379d-468e-a45b-bdf4657692be',
      email: 'jip@example.com',
    }),
    'jip@example.com',
  );
});

test('prefers explicit display name over email', () => {
  assert.equal(
    currentUserDisplayName({
      sub: 'oidc:nopsai:alice',
      email: 'alice@example.com',
      displayName: 'Alice TeamOwner',
    }),
    'Alice TeamOwner',
  );
});

test('hides opaque OIDC subject when no readable identity exists', () => {
  assert.equal(currentUserDisplayName({ sub: 'oidc:nopsai:opaque-subject' }), 'User');
});

test('builds initials from email or display name', () => {
  assert.equal(currentUserInitials({ sub: 'oidc:nopsai:jip', email: 'jip@example.com' }), 'J');
  assert.equal(currentUserInitials({ sub: 'alice', displayName: 'Alice Owner' }), 'AO');
});
