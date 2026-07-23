import assert from 'node:assert/strict';
import { test } from 'node:test';
import { systemLogSourceStatusLabel } from './presentation.js';
import type { SystemLogSource } from './types.js';

const source = (overrides: Partial<SystemLogSource>): SystemLogSource => ({
  id: 'dispatcher',
  display_name: 'Dispatcher',
  container_name: 'nopsai-dispatcher',
  available: true,
  state: 'running',
  ...overrides,
});

test('formats Docker health none as source state', () => {
  assert.equal(systemLogSourceStatusLabel(source({ health: 'none', state: 'running' })), 'running');
});

test('prefers explicit health and falls back to availability', () => {
  assert.equal(systemLogSourceStatusLabel(source({ health: 'healthy', state: 'running' })), 'healthy');
  assert.equal(systemLogSourceStatusLabel(source({ health: '', state: '' })), 'available');
  assert.equal(systemLogSourceStatusLabel(source({ available: false, health: 'healthy', state: 'running' })), 'unavailable');
});
