import assert from 'node:assert/strict';
import test from 'node:test';
import { apiClient } from './api.js';
import {
  buildPipelineRunTeamPaths,
  buildResourceTeamPaths,
  fetchResourceTeamPaths,
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

test('buildPipelineRunTeamPaths keeps root and excludes application entries', () => {
  assert.deepEqual(buildPipelineRunTeamPaths(mixedTeams), [
    'root',
    'platform',
    'platform/ml',
    'platform/payments',
  ]);
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
    assert.deepEqual(await fetchResourceTeamPaths(), ['platform', 'platform/prod']);
  } finally {
    (apiClient as { fetch: typeof apiClient.fetch }).fetch = originalFetch;
  }
});
