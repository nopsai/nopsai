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
      { label: 'Monitoring', path: '/monitoring' },
      { label: 'Dashboards', path: '/dashboards' },
      { label: 'Pipelines', path: '/pipelines' },
      { label: 'Schedules', path: '/schedules' },
      { label: 'Triggers', path: '/triggers' },
      { label: 'Steps', path: '/steps' },
      { label: 'Lab', path: '/lab' },
      { label: 'Assistant', path: '/assistant' },
      { label: 'Agent roles', path: '/agent-profiles' },
      { label: 'Models', path: '/llm-profiles' },
      { label: 'Knowledge', path: '/knowledge-context' },
      { label: 'MCP', path: '/mcp' },
      { label: 'Teams', path: '/teams' },
      { label: 'Scopes', path: '/scopes' },
      { label: 'Credentials', path: '/credentials' },
      { label: 'Custom Console', path: '/custom-console' },
    ]);

    assert.deepEqual(
      topics.map(topic => [topic.label, topic.items.map(item => item.label)]),
      [
        ['Observe', ['Pipeline runs', 'Monitoring', 'Dashboards']],
        ['Build & Automate', ['Pipelines', 'Schedules', 'Triggers', 'Steps']],
        ['Lab', ['Lab']],
        ['AI & Knowledge', ['Assistant', 'Agent roles', 'Models', 'Knowledge', 'MCP']],
        ['Workspace', ['Teams', 'Scopes', 'Credentials']],
        ['Other', ['Custom Console']],
      ]
    );
  });

  it('omits topics with no visible items', () => {
    const topics = groupNavItemsByTopic([{ label: 'Monitoring', path: '/monitoring' }]);

    assert.deepEqual(
      topics.map(topic => topic.label),
      ['Observe']
    );
  });
});
