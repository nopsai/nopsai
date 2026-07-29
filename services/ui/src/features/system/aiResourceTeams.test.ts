import assert from 'node:assert/strict';
import test from 'node:test';
import {
  AI_RESOURCE_TEAM_FILTER_GLOBAL,
  AI_RESOURCE_TEAM_FILTER_ALL,
  aiResourceRoute,
  aiResourceLocalName,
  aiResourceMatchesTeamFilter,
  aiResourceSearchParamsForTeamFilter,
  aiResourceTeamScope,
  buildAIResourceScopedID,
  collectAIResourceTeamPaths,
  decodeAIResourceRouteID,
  encodeAIResourceRouteID,
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

test('builds readable nested AI resource routes', () => {
  assert.equal(encodeAIResourceRouteID('platform/ml/reasoning profile'), 'platform/ml/reasoning%20profile');
  assert.equal(decodeAIResourceRouteID('/llm-profiles/platform/ml/reasoning%20profile', 'llm-profiles'), 'platform/ml/reasoning profile');
  assert.equal(
    aiResourceRoute('/llm-profiles', 'platform/ml/reasoning profile', new URLSearchParams('team=platform%2Fml')),
    '/llm-profiles/platform/ml/reasoning%20profile?team=platform%2Fml'
  );
});

test('writes team filter query params without legacy aliases', () => {
  assert.equal(
    aiResourceSearchParamsForTeamFilter(new URLSearchParams('team_path=old&query=fast'), 'platform/ml').toString(),
    'query=fast&team=platform%2Fml'
  );
  assert.equal(
    aiResourceSearchParamsForTeamFilter(new URLSearchParams('team=platform'), AI_RESOURCE_TEAM_FILTER_GLOBAL).toString(),
    'team=global'
  );
  assert.equal(
    aiResourceSearchParamsForTeamFilter(new URLSearchParams('team=platform&query=fast'), AI_RESOURCE_TEAM_FILTER_ALL).toString(),
    'query=fast'
  );
});
