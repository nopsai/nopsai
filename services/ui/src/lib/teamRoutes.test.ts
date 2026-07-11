import assert from 'node:assert/strict';
import test from 'node:test';
import {
  buildPipelineRunsRoute,
  decodeTeamRouteSegments,
  extractTeamPathFromRoute,
  teamScopedRoute,
} from './teamRoutes.js';

test('builds readable team-scoped routes with path segments', () => {
  assert.equal(teamScopedRoute('/pipelines', 'team-2/bank/account'), '/pipelines/team/team-2/bank/account');
  assert.equal(
    buildPipelineRunsRoute('main', 'team-2/bank/account'),
    '/pipelineruns/main/team/team-2/bank/account'
  );
});

test('decodes team route segments without treating slashes as one query value', () => {
  assert.equal(
    extractTeamPathFromRoute('/pipelineruns/recent/team/team-2/bank/account', 'pipelineruns'),
    'team-2/bank/account'
  );
  assert.equal(decodeTeamRouteSegments(['team-2', 'bank%20ops', 'account']), 'team-2/bank ops/account');
});

