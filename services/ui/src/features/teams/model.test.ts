import assert from 'node:assert/strict';
import test from 'node:test';
import type { Team } from '../../lib/teamModels.js';
import {
  buildTeamScopeStats,
  buildTeamTree,
  formatTeamTimestamp,
  getLatestRunApplication,
  getTeamCreateParentOptions,
  getTeamDirectChildren,
  getTeamMoveParentOptions,
  getTeamParent,
  getVisibleTeamItems,
  teamKindLabel,
} from './model.js';
import { getTeamTableCopy, getTeamTableItems, visibleTeamDetailTabs } from './workspaceModel.js';

const teams: Team[] = [
  {
    id: 1,
    name: 'platform',
    kind: 'team',
    path: 'platform',
    description: 'Platform engineering',
    parent_id: null,
  },
  {
    id: 2,
    name: 'payments',
    kind: 'team',
    path: 'platform/payments',
    description: 'Payment systems',
    parent_id: 1,
  },
  {
    id: 3,
    name: 'checkout-api',
    kind: 'app',
    path: 'platform/payments/checkout-api',
    team_path: 'platform/payments',
    parent_id: 2,
    repository_full_name: 'acme/checkout-api',
    repo_url: 'https://github.com/acme/checkout-api',
    last_run_at: '2026-07-10T10:00:00Z',
  },
  {
    id: 4,
    name: 'docs',
    kind: 'app',
    path: 'platform/docs',
    parent_id: 1,
    repository_full_name: 'acme/docs',
  },
];

test('builds a sorted team hierarchy with applications after teams', () => {
  const tree = buildTeamTree(teams);

  assert.equal(tree.length, 1);
  assert.equal(tree[0].team.name, 'platform');
  assert.deepEqual(tree[0].children.map(node => node.team.name), ['payments', 'docs']);
  assert.equal(tree[0].children[0].children[0].team.name, 'checkout-api');
});

test('computes scoped stats from the selected team subtree', () => {
  assert.deepEqual(buildTeamScopeStats(teams, null), {
    teams: 2,
    applications: 2,
    repositories: 2,
    recentRuns: 1,
    directChildren: 1,
    totalItems: 4,
  });

  assert.deepEqual(buildTeamScopeStats(teams, 2), {
    teams: 1,
    applications: 1,
    repositories: 1,
    recentRuns: 1,
    directChildren: 1,
    totalItems: 2,
  });
});

test('filters visible team items by current scope or search text', () => {
  assert.deepEqual(getTeamDirectChildren(teams, 1).map(team => team.name), ['payments', 'docs']);
  assert.deepEqual(getVisibleTeamItems(teams, 1, '').map(team => team.name), ['payments', 'docs']);
  assert.deepEqual(getVisibleTeamItems(teams, 1, 'checkout').map(team => team.name), ['checkout-api']);
  assert.deepEqual(getVisibleTeamItems(teams, 1, 'acme/docs').map(team => team.name), ['docs']);
});

test('formats team labels, parent, and timestamps for detail rendering', () => {
  assert.equal(teamKindLabel(null), 'Organization');
  assert.equal(teamKindLabel(teams[2]), 'Application');
  assert.equal(getTeamParent(teams[2], teams)?.name, 'payments');
  assert.equal(formatTeamTimestamp(), 'Never');
  assert.equal(formatTeamTimestamp('not-a-date'), 'not-a-date');
  assert.notEqual(formatTeamTimestamp('2026-07-10T10:00:00Z'), '2026-07-10T10:00:00Z');
});

test('builds move parent options without invalid descendants or applications', () => {
  assert.deepEqual(getTeamCreateParentOptions(teams).map(option => option.label), ['Global', '/platform', '/platform/payments']);
  assert.deepEqual(getTeamMoveParentOptions(teams, teams[0]).map(option => option.label), ['Global']);
  assert.deepEqual(getTeamMoveParentOptions(teams, teams[1]).map(option => option.label), ['Global', '/platform']);
  assert.deepEqual(getTeamMoveParentOptions(teams, teams[2]).map(option => option.label), ['Global', '/platform', '/platform/payments']);
});

test('finds the latest run application inside the selected team scope', () => {
  assert.equal(getLatestRunApplication(teams, null)?.name, 'checkout-api');
  assert.equal(getLatestRunApplication(teams, 2)?.name, 'checkout-api');
  assert.equal(getLatestRunApplication(teams, 4), null);
});

test('builds Teams workspace table items and copy', () => {
  const directChildren = getTeamDirectChildren(teams, 1);
  const visibleItems = [teams[2]];

  assert.deepEqual(
    getTeamTableItems({
      directChildren,
      searching: false,
      visibleItems,
    }).map(team => team.name),
    ['payments', 'docs']
  );
  assert.deepEqual(
    getTeamTableItems({
      directChildren,
      searching: true,
      visibleItems,
    }).map(team => team.name),
    ['checkout-api']
  );
  assert.equal(getTeamTableCopy({ activeLabel: 'platform', searching: false }).title, 'Child Resources');
  assert.equal(getTeamTableCopy({ activeLabel: 'platform', searching: true }).title, 'Matching Resources');
});

test('keeps application detail navigation collapsed into overview only', () => {
  assert.deepEqual(visibleTeamDetailTabs(teams[2]).map(tab => tab.label), ['Overview']);
  assert.deepEqual(visibleTeamDetailTabs(teams[0]).map(tab => tab.label), ['Overview', 'GitOps', 'Defaults', 'Notifications', 'Access']);
  // Global scope keeps Defaults — only Notifications is team-scoped.
  assert.deepEqual(visibleTeamDetailTabs(null).map(tab => tab.label), ['Overview', 'GitOps', 'Defaults', 'Access']);
});
