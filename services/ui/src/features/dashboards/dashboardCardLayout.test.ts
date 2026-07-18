import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  dashboardCardLayoutItemKey,
  dashboardCardLayoutStorageKey,
  moveDashboardCard,
  normalizeDashboardCardLayout,
  orderDashboardCards,
  placeDashboardCard,
  setDashboardCardSize,
  type DashboardCardLayout,
} from './dashboardCardLayout.js';

test('normalizes dashboard card layout to known cards and safe values', () => {
  const layout = normalizeDashboardCardLayout(
    {
      first: { order: 20.4, size: 'wide' },
      second: { order: -1, size: 'compact' },
      stale: { order: 0, size: 'standard' },
      invalid: 'wide',
    },
    ['first', 'second']
  );

  assert.deepEqual(layout, {
    second: { order: 0, size: 'compact' },
    first: { order: 1, size: 'wide' },
  });
});

test('orders, moves, and resizes dashboard cards without changing unknown cards', () => {
  const cards = [{ id: 'first' }, { id: 'second' }, { id: 'third' }];
  let layout: DashboardCardLayout = {};

  layout = moveDashboardCard(layout, cards.map(card => card.id), 'first', 'later');
  assert.deepEqual(orderDashboardCards(cards, card => card.id, layout).map(card => card.id), ['second', 'first', 'third']);

  layout = placeDashboardCard(layout, cards.map(card => card.id), 'third', 'second', 'before');
  assert.deepEqual(orderDashboardCards(cards, card => card.id, layout).map(card => card.id), ['third', 'second', 'first']);

  layout = setDashboardCardSize(layout, cards.map(card => card.id), 'first', 'wide');
  assert.equal(layout.first?.size, 'wide');

  layout = setDashboardCardSize(layout, cards.map(card => card.id), 'missing', 'compact');
  assert.equal(layout.missing, undefined);
});

test('builds stable storage and publication keys', () => {
  assert.equal(
    dashboardCardLayoutStorageKey(' dashboard-1 ', ' overview '),
    'nopsai.dashboard-card-layout.v1:dashboard-1:overview'
  );
  assert.equal(
    dashboardCardLayoutItemKey({
      id: '',
      section_key: 'overview',
      entry_key: 'health',
      pipeline_id: 'platform/health',
      output_name: 'Service Health',
    }),
    'overview:health:platform/health:Service Health'
  );
});
