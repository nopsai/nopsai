import assert from 'node:assert/strict';
import { test } from 'node:test';
import { isInitialSetupAllowedRoute } from './initialSetupGate.js';

test('first-install setup gate only allows setup and forced password change routes', () => {
  assert.equal(isInitialSetupAllowedRoute('/system/setup', false), true);
  assert.equal(isInitialSetupAllowedRoute('/profile', true), true);
  assert.equal(isInitialSetupAllowedRoute('/profile', false), false);
  assert.equal(isInitialSetupAllowedRoute('/teams', false), false);
});
