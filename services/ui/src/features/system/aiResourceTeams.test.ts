import assert from 'node:assert/strict';
import test from 'node:test';
import {
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  aiResourceLocalName,
  aiResourceMatchesTeamFilter,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  collectAIResourceTeamPaths,
  selectableAIResourceTeamPath,
} from './aiResourceTeams.js';

test('splits AI resource IDs into team placement and local names', () => {
  assert.deepEqual(aiResourceTeamScope('platform/ml/reasoning'), {
    teamPath: 'platform/ml',
    localName: 'reasoning',
    displayTeam: '/platform/ml',
  });
  assert.equal(aiResourceLocalName('hosted'), 'hosted');
  assert.equal(aiResourceLocalName('platform/ml/'), '');
  assert.equal(buildAIResourceScopedID('/platform/ml/', ' reasoning '), 'platform/ml/reasoning');
});

test('matches AI resources by team filter including nested teams', () => {
  assert.equal(aiResourceMatchesTeamFilter('platform/ml/reasoning', 'platform'), true);
  assert.equal(aiResourceMatchesTeamFilter('platform/ml/reasoning', 'platform/ml'), true);
  assert.equal(aiResourceMatchesTeamFilter('hosted', AI_RESOURCE_TEAM_FILTER_GLOBAL), true);
  assert.equal(aiResourceMatchesTeamFilter('platform/ml/reasoning', AI_RESOURCE_TEAM_FILTER_GLOBAL), false);
});

test('collects selectable team paths from the team catalog only', () => {
  assert.deepEqual(
    collectAIResourceTeamPaths(['platform/app/reasoning', 'ops/github', 'hosted'], ['security', 'platform']),
    ['platform', 'security']
  );
  assert.equal(selectableAIResourceTeamPath('platform', ['platform']), 'platform');
  assert.equal(selectableAIResourceTeamPath('platform/app', ['platform']), '');
});
