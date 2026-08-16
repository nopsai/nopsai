import assert from 'node:assert/strict';
import test from 'node:test';
import { buildLoginRedirectState, resolvePostLoginPath } from './authRedirect.js';

test('preserves a pipeline run deep link through login', () => {
  const state = buildLoginRedirectState(
    '/pipelineruns/main/team/finance/accountant',
    '?run=0ca1f669-5ae5-4b3b-be6a-66a0dc03c99e'
  );
  assert.equal(
    resolvePostLoginPath(state),
    '/pipelineruns/main/team/finance/accountant?run=0ca1f669-5ae5-4b3b-be6a-66a0dc03c99e'
  );
});

test('rejects external and recursive login return paths', () => {
  assert.equal(resolvePostLoginPath({ returnTo: '//example.com' }), '/pipelineruns/main');
  assert.equal(resolvePostLoginPath({ returnTo: '/login?next=/system' }), '/pipelineruns/main');
});

test('rejects backslash-authority return paths that browsers treat as cross-origin', () => {
  assert.equal(resolvePostLoginPath({ returnTo: '/\\example.com' }), '/pipelineruns/main');
  assert.equal(resolvePostLoginPath({ returnTo: '/\\/example.com' }), '/pipelineruns/main');
});
