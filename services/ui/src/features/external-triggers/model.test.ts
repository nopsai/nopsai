import assert from 'node:assert/strict';
import test from 'node:test';
import {
  externalTriggerTeamLabel,
  externalTriggerRelativeLabel,
  externalTriggerScopeLabel,
} from './model.js';

test('formats external trigger scope and team labels', () => {
  assert.equal(externalTriggerScopeLabel(), 'default');
  assert.equal(externalTriggerScopeLabel('default'), 'default');
  assert.equal(externalTriggerScopeLabel('.nopsai/pipelines/platform.yaml'), 'platform');
  assert.equal(externalTriggerTeamLabel(), 'Root');
  assert.equal(externalTriggerTeamLabel('root'), 'Root');
  assert.equal(externalTriggerTeamLabel('platform/prod'), 'platform/prod');
});

test('formats external trigger relative timestamps', () => {
  const now = Date.parse('2026-06-15T12:00:00Z');
  assert.equal(externalTriggerRelativeLabel(undefined, now), 'Never');
  assert.equal(externalTriggerRelativeLabel('invalid', now), 'Never');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T12:00:30Z', now), 'just now');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T11:59:30Z', now), 'just now');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T11:30:00Z', now), '30m ago');
  assert.equal(externalTriggerRelativeLabel('2026-06-15T10:00:00Z', now), '2h ago');
  assert.equal(externalTriggerRelativeLabel('2026-06-13T12:00:00Z', now), '2d ago');
});
