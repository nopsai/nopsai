import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  assistantPageContextIsEmpty,
  assistantPageContextKey,
  assistantPageContextLabel,
  assistantPageContextScope,
  buildAssistantPageContext,
  normalizeAssistantPageContext,
} from './pageContext.js';

describe('assistant page context', () => {
  it('extracts selected pipeline run context from route and safe filters', () => {
    const context = buildAssistantPageContext(
      '/pipelineruns/recent/00000000-0000-0000-0000-000000000123',
      '?status=failure&source=schedule&token=secret'
    );

    assert.equal(context.title, 'Pipeline runs');
    assert.equal(context.route, '/pipelineruns/:tab/:run_id');
    assert.equal(context.tab, 'recent');
    assert.equal(context.resource_type, 'pipeline_run');
    assert.equal(context.run_id, '00000000-0000-0000-0000-000000000123');
    assert.equal(context.query.status, 'failure');
    assert.equal(context.query.source, 'schedule');
    assert.equal('token' in context.query, false);
  });

  it('extracts selected pipeline and scope context without scraping page content', () => {
    const context = buildAssistantPageContext('/pipelines/platform/api/deploy', '');

    assert.equal(context.route, '/pipelines/:pipeline_id');
    assert.equal(context.resource_type, 'pipeline');
    assert.equal(context.pipeline_id, 'platform/api/deploy');
    assert.equal(context.team_path, 'platform/api');
    assert.equal(context.scope, 'platform/api');
    assert.equal(assistantPageContextLabel(context), 'Pipelines · deploy · /platform/api');
    assert.equal(assistantPageContextScope(context), 'platform/api');
  });

  it('extracts path-backed resource context for routed workspaces', () => {
    const schedule = buildAssistantPageContext('/schedules/prod/nightly-api', '?pipeline=general/api');
    assert.equal(schedule.route, '/schedules/:schedule_id');
    assert.equal(schedule.resource_type, 'schedule');
    assert.equal(schedule.resource_id, 'prod/nightly-api');
    assert.equal(schedule.pipeline_id, 'general/api');

    const dashboard = buildAssistantPageContext('/dashboards/team-1/ops-dashboard', '?tab=service-metrics');
    assert.equal(dashboard.route, '/dashboards/:dashboard_id');
    assert.equal(dashboard.resource_type, 'dashboard');
    assert.equal(dashboard.resource_id, 'team-1/ops-dashboard');
    assert.equal(dashboard.tab, 'service-metrics');

    const llmProfile = buildAssistantPageContext('/models/platform/ml/reasoning', '');
    assert.equal(llmProfile.route, '/models/:profile_id');
    assert.equal(llmProfile.resource_type, 'model');
    assert.equal(llmProfile.team_path, 'platform/ml');

    const mcpServer = buildAssistantPageContext('/mcp/servers/platform/ml/github', '');
    assert.equal(mcpServer.route, '/mcp/servers/:server_id');
    assert.equal(mcpServer.resource_type, 'mcp_server');
    assert.equal(mcpServer.resource_id, 'platform/ml/github');

    const credential = buildAssistantPageContext('/credentials/team/platform/payments/openai', '');
    assert.equal(credential.route, '/credentials/:credential_ref');
    assert.equal(credential.resource_type, 'credential');
    assert.equal(credential.resource_id, 'credential://team/platform/payments/openai');
    assert.equal(credential.team_path, 'platform/payments');

    const monitoring = buildAssistantPageContext('/monitoring/ai-usage', '?runId=run-1');
    assert.equal(monitoring.route, '/monitoring/:tab');
    assert.equal(monitoring.resource_type, 'monitoring');
    assert.equal(monitoring.tab, 'ai-usage');
    assert.equal(monitoring.run_id, 'run-1');
  });

  it('normalizes route-state context before sending it to the assistant', () => {
    const context = normalizeAssistantPageContext({
      title: '  Scopes  ',
      area: 'Scopes!',
      scope: ' /prod/api/ ',
      query: { TOKEN: 'hidden', status: ' failure ' },
      params: { run_id: '  run-1  ' },
    });

    assert.equal(context.title, 'Scopes');
    assert.equal(context.area, 'scopes');
    assert.equal(context.scope, 'prod/api');
    assert.deepEqual(context.query, { status: 'failure' });
    assert.deepEqual(context.params, { run_id: 'run-1' });
    assert.equal(assistantPageContextIsEmpty(context), false);
    assert.equal(assistantPageContextIsEmpty(null), true);
  });

  it('builds a stable key for equivalent sanitized context', () => {
    const first = assistantPageContextKey({
      title: 'Pipelines',
      path: '/pipelines/platform/deploy',
      query: { status: ' failure ', owner: 'platform', token: 'hidden' },
      params: { pipeline_id: ' platform/deploy ', ignored: 'nope' },
    });
    const second = assistantPageContextKey({
      title: 'Pipelines',
      path: '/pipelines/platform/deploy',
      query: { owner: 'platform', token: 'different-hidden', status: 'failure' },
      params: { ignored: 'still-nope', pipeline_id: 'platform/deploy' },
    });

    assert.notEqual(first, '');
    assert.equal(first, second);
    assert.equal(assistantPageContextKey(null), '');
  });
});
