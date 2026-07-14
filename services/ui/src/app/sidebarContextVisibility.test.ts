import assert from 'node:assert/strict';
import { test } from 'node:test';
import { shouldShowSidebarContextNav } from './sidebarContextVisibility.js';

test('hides shell contextual navigation on trigger pages only', () => {
  assert.equal(shouldShowSidebarContextNav('/triggers', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/triggers/acme/api', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/pipelines', 'main'), true);
  assert.equal(shouldShowSidebarContextNav('/external-triggers', 'main'), true);
});

test('keeps pipeline run sidebar context hidden only on overview', () => {
  assert.equal(shouldShowSidebarContextNav('/pipelineruns/main', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/pipelineruns/recent', 'recent'), true);
  assert.equal(shouldShowSidebarContextNav('/pipelineruns/events', 'events'), true);
});
