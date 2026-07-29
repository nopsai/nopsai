import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  dashboardRouteIDFromPath,
  dashboardRouteParamForSelectedID,
  dashboardRouteSelectedID,
  dashboardTabHref,
  dashboardTabSearchParams,
  normalizeDashboardRouteValue,
  resolveDashboardActiveSectionKey,
} from './routes.js';

test('builds dashboard tab URLs while preserving unrelated query parameters', () => {
  const current = new URLSearchParams('team=platform&dashboard=old&tab=overview');
  const params = dashboardTabSearchParams(current, 'build timeline');

  assert.equal(params.get('team'), 'platform');
  assert.equal(params.has('dashboard'), false);
  assert.equal(params.get('tab'), 'build timeline');
  assert.equal(
    dashboardTabHref(current, 'dashboard-1', 'build timeline'),
    '/dashboards/dashboard-1?team=platform&tab=build+timeline'
  );
});

test('removes dashboard tab URL parameters when no dashboard is selected', () => {
  const params = dashboardTabSearchParams(new URLSearchParams('dashboard=old&tab=overview'), '');

  assert.equal(params.has('dashboard'), false);
  assert.equal(params.has('tab'), false);
  assert.equal(dashboardTabHref(new URLSearchParams('dashboard=old&tab=overview'), '', ''), '/dashboards');
});

test('normalizes dashboard route values', () => {
  assert.equal(normalizeDashboardRouteValue('  releases  '), 'releases');
  assert.equal(normalizeDashboardRouteValue(null), '');
  assert.equal(dashboardRouteIDFromPath('/dashboards/team-1/ops%20dashboard'), 'team-1/ops dashboard');
});

test('resolves dashboard route refs to canonical dashboard ids', () => {
  const dashboards = [
    { id: 'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82', team_path: 'team-1', ref: 'team-1/ops-dashboard', slug: 'ops-dashboard' },
    { id: 'dashboard-2', team_path: 'team-1', ref: 'team-1/release-dashboard', slug: 'release-dashboard' },
  ];

  assert.equal(
    dashboardRouteSelectedID(dashboards, 'team-1/ops-dashboard', ''),
    'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82'
  );
  assert.equal(
    dashboardRouteSelectedID(dashboards, 'team-1/release-dashboard', 'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82'),
    'dashboard-2'
  );
  assert.equal(
    dashboardRouteSelectedID(dashboards, '', 'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82'),
    'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82'
  );
});

test('keeps the current dashboard route alias when it matches the selected dashboard', () => {
  const dashboards = [
    { id: 'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82', team_path: 'team-1', ref: 'team-1/ops-dashboard', slug: 'ops-dashboard' },
    { id: 'dashboard-2', team_path: 'team-1', ref: 'team-1/release-dashboard', slug: 'release-dashboard' },
  ];

  assert.equal(
    dashboardRouteParamForSelectedID(dashboards, 'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82', 'team-1/ops-dashboard'),
    'team-1/ops-dashboard'
  );
  assert.equal(
    dashboardRouteParamForSelectedID(dashboards, 'dashboard-2', 'team-1/ops-dashboard'),
    'dashboard-2'
  );
});

test('preserves requested dashboard section while sections are loading', () => {
  assert.equal(resolveDashboardActiveSectionKey(undefined, 'service-metrics'), 'service-metrics');
  assert.equal(resolveDashboardActiveSectionKey([], 'service-metrics'), 'service-metrics');
  assert.equal(
    resolveDashboardActiveSectionKey([{ section_key: 'overview' }, { section_key: 'service-metrics' }], 'service-metrics'),
    'service-metrics'
  );
  assert.equal(resolveDashboardActiveSectionKey([{ section_key: 'overview' }], 'missing'), 'overview');
});
