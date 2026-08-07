import assert from 'node:assert/strict';
import { test } from 'node:test';
import { shouldShowSidebarContextNav } from './sidebarContextVisibility.js';

test('hides shell contextual navigation on pages with their own local navigation', () => {
  assert.equal(shouldShowSidebarContextNav('/triggers', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/triggers/acme/api', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/knowledge-context', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/knowledge-context/platform/restart', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/scopes', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/scopes/platform/dev', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/pipelines', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/pipelines/platform/deploy', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/steps', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/steps/platform/build', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/external-triggers', 'main'), true);
});

test('keeps pipeline run sidebar context hidden only on overview', () => {
  assert.equal(shouldShowSidebarContextNav('/pipelineruns/main', 'main'), false);
  assert.equal(shouldShowSidebarContextNav('/pipelineruns/recent', 'recent'), true);
  assert.equal(shouldShowSidebarContextNav('/pipelineruns/events', 'events'), true);
});
