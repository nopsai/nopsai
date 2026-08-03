import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from './api.js';
import {
  buildPipelineRunTeamPaths,
  buildResourceTeamPaths,
  fetchResourceTeamPaths,
  insertTeamPath,
  isGlobalResourceTeamPath,
  resourceTeamPathsWithGlobal,
  type ResourceTeam,
} from './resourceTeams.js';

const mixedTeams: ResourceTeam[] = [
  { id: 1, name: 'platform', kind: 'team', parent_id: null },
  { id: 2, name: 'payments', kind: 'team', parent_id: 1 },
  {
    id: 3,
    name: 'checkout-api',
    kind: 'app',
    parent_id: 2,
    repository_full_name: 'acme/checkout-api',
  },
  {
    id: 4,
    name: 'acme/docs',
    parent_id: 1,
    repository_full_name: 'acme/docs',
  },
  { id: 5, name: 'ml', kind: 'team', parent_id: 1 },
];

test('buildResourceTeamPaths returns team paths without applications', () => {
  assert.deepEqual(buildResourceTeamPaths(mixedTeams), [
    'platform',
    'platform/ml',
    'platform/payments',
  ]);
});

test('buildPipelineRunTeamPaths keeps global and excludes application entries', () => {
  assert.deepEqual(buildPipelineRunTeamPaths(mixedTeams), [
    'global',
    'platform',
    'platform/ml',
    'platform/payments',
  ]);
});

test('resourceTeamPathsWithGlobal keeps global first without accepting root aliases', () => {
  assert.deepEqual(resourceTeamPathsWithGlobal(['platform', 'global', 'platform']), ['global', 'platform']);
  assert.equal(isGlobalResourceTeamPath('/root/'), false);
  assert.equal(isGlobalResourceTeamPath('/global/'), true);
});

test('insertTeamPath keeps global first at each resource tree level', () => {
  type Node = {
    id: string;
    name: string;
    fullPath: string;
    children: Node[];
  };
  const root: Node = { id: '__root__', name: '', fullPath: '', children: [] };
  const createNode = (id: string, name: string, fullPath: string): Node => ({ id, name, fullPath, children: [] });

  ['platform/prod', 'data', 'global', 'platform/global', 'platform/dev'].forEach(path => {
    insertTeamPath(root, path, createNode);
  });

  assert.deepEqual(root.children.map(child => child.fullPath), ['global', 'data', 'platform']);
  assert.deepEqual(
    root.children.find(child => child.fullPath === 'platform')?.children.map(child => child.fullPath),
    ['platform/global', 'platform/dev', 'platform/prod']
  );
});

test('fetchResourceTeamPaths filters application records from access teams', async () => {
  const originalFetch = apiClient.fetch.bind(apiClient);
  (apiClient as { fetch: typeof apiClient.fetch }).fetch = async input => {
    assert.equal(String(input), '/v1/access/teams');
    return Response.json([
      { id: 'platform', name: '/platform' },
      { id: 'platform/app', name: '/platform/app', kind: 'app' },
      { id: 'platform/repo', name: '/platform/repo', repository_full_name: 'acme/repo' },
      { name: '/platform/prod' },
      { id: 'platform/prod' },
    ]);
  };
  try {
    assert.deepEqual(await fetchResourceTeamPaths(), ['global', 'platform', 'platform/prod']);
  } finally {
    (apiClient as { fetch: typeof apiClient.fetch }).fetch = originalFetch;
  }
});
