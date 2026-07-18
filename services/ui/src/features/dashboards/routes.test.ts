import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  dashboardTabHref,
  dashboardTabSearchParams,
  normalizeDashboardRouteValue,
} from './routes.js';

test('builds dashboard tab URLs while preserving unrelated query parameters', () => {
  const current = new URLSearchParams('team=platform&dashboard=old&tab=overview');
  const params = dashboardTabSearchParams(current, 'dashboard-1', 'build timeline');

  assert.equal(params.get('team'), 'platform');
  assert.equal(params.get('dashboard'), 'dashboard-1');
  assert.equal(params.get('tab'), 'build timeline');
  assert.equal(
    dashboardTabHref(current, 'dashboard-1', 'build timeline'),
    '/dashboards?team=platform&dashboard=dashboard-1&tab=build+timeline'
  );
});

test('removes dashboard tab URL parameters when no dashboard is selected', () => {
  const params = dashboardTabSearchParams(new URLSearchParams('dashboard=old&tab=overview'), '', '');

  assert.equal(params.has('dashboard'), false);
  assert.equal(params.has('tab'), false);
  assert.equal(dashboardTabHref(new URLSearchParams('dashboard=old&tab=overview'), '', ''), '/dashboards');
});

test('normalizes dashboard route values', () => {
  assert.equal(normalizeDashboardRouteValue('  releases  '), 'releases');
  assert.equal(normalizeDashboardRouteValue(null), '');
});
