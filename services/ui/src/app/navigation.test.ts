import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  eventAutomationNavPath,
  groupNavItemsByTopic,
  pipelineRunsNavPath,
  sidebarNavItemIsActive,
} from './navigationModel.js';

describe('pipelineRunsNavPath', () => {
  it('keeps the current pipeline runs tab for primary navigation', () => {
    assert.equal(pipelineRunsNavPath('/pipelineruns/recent'), '/pipelineruns/recent');
    assert.equal(pipelineRunsNavPath('/pipelineruns/events'), '/pipelineruns/events');
    assert.equal(pipelineRunsNavPath('/pipelineruns/main'), '/pipelineruns/main');
    assert.equal(pipelineRunsNavPath('/pipelines'), '/pipelineruns/main');
  });
});

describe('eventAutomationNavPath', () => {
  it('routes the combined Triggers sidebar item to the first permitted automation page', () => {
    assert.equal(eventAutomationNavPath({
      canViewTriggers: true,
      canViewExternalTriggers: true,
      canViewGitWebhookSources: true,
    }), '/triggers');
    assert.equal(eventAutomationNavPath({
      canViewTriggers: false,
      canViewExternalTriggers: true,
      canViewGitWebhookSources: true,
    }), '/external-triggers');
    assert.equal(eventAutomationNavPath({
      canViewTriggers: false,
      canViewExternalTriggers: false,
      canViewGitWebhookSources: true,
    }), '/git-webhook-sources');
  });
});

describe('sidebarNavItemIsActive', () => {
  it('keeps Triggers active across event automation routes', () => {
    assert.equal(sidebarNavItemIsActive('/triggers', '/triggers/acme/repo'), true);
    assert.equal(sidebarNavItemIsActive('/triggers', '/external-triggers/deploy'), true);
    assert.equal(sidebarNavItemIsActive('/triggers', '/git-webhook-sources/gitlab'), true);
    assert.equal(sidebarNavItemIsActive('/triggers', '/pipelines'), false);
  });
});

describe('groupNavItemsByTopic', () => {
  it('groups sidebar navigation into enterprise product topics', () => {
    const topics = groupNavItemsByTopic([
      { label: 'Pipeline runs', path: '/pipelineruns/main' },
      { label: 'Dashboards', path: '/dashboards' },
      { label: 'Teams', path: '/teams' },
      { label: 'Pipelines', path: '/pipelines' },
      { label: 'Triggers', path: '/triggers' },
      { label: 'Assistant', path: '/assistant' },
      { label: 'Credentials', path: '/credentials' },
      { label: 'MCP', path: '/mcp' },
      { label: 'Scopes', path: '/scopes' },
      { label: 'Custom Console', path: '/custom-console' },
    ]);

    assert.deepEqual(
      topics.map(topic => [topic.label, topic.items.map(item => item.label)]),
      [
        ['Operate', ['Pipeline runs', 'Dashboards']],
        ['Build & Automate', ['Pipelines', 'Triggers']],
        ['AI & Knowledge', ['Assistant', 'MCP']],
        ['Organization', ['Teams', 'Scopes']],
        ['Platform', ['Credentials']],
        ['Other', ['Custom Console']],
      ]
    );
  });

  it('omits topics with no visible items', () => {
    const topics = groupNavItemsByTopic([{ label: 'Monitoring', path: '/monitoring' }]);

    assert.deepEqual(
      topics.map(topic => topic.label),
      ['Operate']
    );
  });
});
