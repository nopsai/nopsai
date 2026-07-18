import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  createDashboardForm,
  createRefreshScheduleForm,
  createSourceForm,
  dashboardRequestFromForm,
  normalizeDashboardRefreshSchedule,
  normalizeDashboardRefresh,
  normalizeDashboardSpec,
  refreshScheduleFormFromSchedule,
  refreshScheduleRequestFromForm,
  sectionRequestFromForm,
  sourceRequestFromForm,
} from './model.js';

describe('dashboard model normalization', () => {
  it('normalizes refresh schedule and GitOps provenance fields', () => {
    const schedule = normalizeDashboardRefreshSchedule({
      id: 'schedule-1',
      dashboard_id: 'dashboard-1',
      dashboard_ref: 'platform/health',
      name: 'Hourly health',
      cron: '0 * * * *',
      cron_expression: '0 * * * *',
      timezone: 'Europe/Amsterdam',
      enabled: true,
      scope_type: 'section',
      scope: { section_key: 'overview' },
      mode: 'best_effort',
      variables: { environment: 'prod' },
      max_concurrency: 8,
      timeout_seconds: 1800,
      source: 'config_repository',
      config_source_path: 'dashboards/platform-health.yaml',
      config_source_commit_sha: 'abc123',
      managed_by_config_repo: true,
      service_account_id: 'dashboard-refresh:platform-health:hourly-health',
      created_at: '2026-07-15T10:00:00Z',
      updated_at: '2026-07-15T10:00:00Z',
    });

    assert.equal(schedule.dashboard_ref, 'platform/health');
    assert.equal(schedule.scope_type, 'section');
    assert.equal(schedule.max_concurrency, 8);
    assert.equal(schedule.managed_by_config_repo, true);
    assert.equal(schedule.config_source_path, 'dashboards/platform-health.yaml');
  });

  it('preserves chart blocks for renderer-safe display', () => {
    const spec = normalizeDashboardSpec({
      version: '1',
      title: 'Ops',
      blocks: [
        {
          type: 'series',
          title: 'Latency',
          chart: {
            type: 'line',
            aggregation_interval: '5m',
            series: [
              {
                key: 'api.p95',
                points: [{ timestamp: '2026-07-15T10:00:00Z', value: 120 }],
              },
            ],
          },
        },
      ],
    });

    assert.equal(spec.blocks?.[0]?.type, 'series');
    assert.equal(spec.blocks?.[0]?.chart?.series?.[0]?.key, 'api.p95');
  });

  it('omits create-only sections from dashboard edit requests', () => {
    const form = {
      ...createDashboardForm('platform'),
      slug: 'health',
      title: 'Health',
      description: 'Runtime health.',
      sectionKey: 'overview',
      sectionTitle: 'Overview',
    };

    assert.deepEqual(dashboardRequestFromForm(form, { includeSections: false }), {
      team_path: 'platform',
      slug: 'health',
      title: 'Health',
      description: 'Runtime health.',
      visibility: 'team',
    });
    assert.equal(dashboardRequestFromForm(form).sections?.[0]?.section_key, 'overview');
  });

  it('uses pipeline-derived section seeds for dashboard create requests', () => {
    const form = {
      ...createDashboardForm('platform'),
      slug: 'health',
      title: 'Health',
    };

    assert.deepEqual(dashboardRequestFromForm(form, {
      sections: [
        { sectionKey: 'service-health', title: 'Service Health', displayOrder: 10 },
        { sectionKey: 'deployments', displayOrder: 20 },
      ],
    }).sections, [
      {
        section_key: 'service-health',
        title: 'Service Health',
        description: '',
        display_order: 10,
      },
      {
        section_key: 'deployments',
        title: 'Deployments',
        description: '',
        display_order: 20,
      },
    ]);
  });

  it('builds section requests with a generated title and numeric display order', () => {
    assert.deepEqual(sectionRequestFromForm({
      sectionKey: 'service-health',
      title: '',
      description: 'Current service state.',
      displayOrder: '20',
    }), {
      section_key: 'service-health',
      title: 'Service Health',
      description: 'Current service state.',
      display_order: 20,
    });
  });

  it('includes normalized run scope in source binding requests', () => {
    assert.deepEqual(sourceRequestFromForm({
      ...createSourceForm('overview'),
      pipelineID: 'team-1/dashboard',
      outputName: 'Service Health',
      entryKey: 'health',
      runScope: '/prod/',
      refreshOrder: '5',
    }), {
      section_key: 'overview',
      pipeline_id: 'team-1/dashboard',
      output_name: 'Service Health',
      entry_key: 'health',
      run_scope: 'prod',
      enabled: true,
      required_for_refresh: true,
      refresh_order: 5,
    });
  });

  it('preserves an empty source entry key so bindings can use the output name', () => {
    assert.deepEqual(sourceRequestFromForm({
      ...createSourceForm('overview'),
      pipelineID: 'team-1/dashboard',
      outputName: 'Service Health',
      entryKey: '',
    }).entry_key, '');
  });

  it('normalizes separate pipeline and output refresh source statuses', () => {
    const refresh = normalizeDashboardRefresh({
      id: 'refresh-1',
      dashboard_id: 'dashboard-1',
      dashboard_ref: 'team-1/ops',
      trigger_type: 'manual',
      scope_type: 'dashboard',
      mode: 'strict',
      status: 'running',
      total_sources: 1,
      required_sources: 1,
      created_at: '2026-07-18T10:00:00Z',
      updated_at: '2026-07-18T10:03:00Z',
      sources: [{
        id: 'source-run-1',
        refresh_id: 'refresh-1',
        pipeline_id: 'team-1/dashboard',
        output_name: 'Service Health',
        section_key: 'overview',
        required: true,
        status: 'running',
        pipeline_status: 'success',
        pipeline_finished_at: '2026-07-18T10:02:00Z',
        output_status: 'generating',
        output_created_at: '2026-07-18T10:02:01Z',
        output_updated_at: '2026-07-18T10:03:00Z',
        output_duration: '59s',
        output_duration_seconds: 59,
        created_at: '2026-07-18T10:00:00Z',
        updated_at: '2026-07-18T10:03:00Z',
      }],
    });

    assert.equal(refresh.sources?.[0]?.status, 'running');
    assert.equal(refresh.sources?.[0]?.pipeline_status, 'success');
    assert.equal(refresh.sources?.[0]?.output_status, 'generating');
    assert.equal(refresh.sources?.[0]?.output_duration_seconds, 59);
  });

  it('builds refresh schedule requests with scoped cadence and guardrails', () => {
    const form = {
      ...createRefreshScheduleForm({ scopeType: 'section', sectionKey: 'overview' }),
      name: 'hourly-health',
      description: 'Refresh before the review.',
      cronMode: 'hourly' as const,
      cronMinute: '0',
      intervalValue: '1',
      cron_expression: '0 * * * *',
      timezone: 'Europe/Amsterdam',
      mode: 'best_effort' as const,
      timeout: '30m',
      maxConcurrency: '2',
    };

    assert.deepEqual(refreshScheduleRequestFromForm(form), {
      name: 'hourly-health',
      description: 'Refresh before the review.',
      cron_expression: '0 * * * *',
      timezone: 'Europe/Amsterdam',
      enabled: true,
      scope: {
        type: 'section',
        section_key: 'overview',
        source_id: undefined,
      },
      mode: 'best_effort',
      timeout: '30m',
      max_concurrency: 2,
    });
  });

  it('creates refresh schedule forms from backend array scopes', () => {
    const form = refreshScheduleFormFromSchedule(normalizeDashboardRefreshSchedule({
      id: 'schedule-1',
      dashboard_id: 'dashboard-1',
      dashboard_ref: 'platform/health',
      name: 'source-refresh',
      cron_expression: '*/15 * * * *',
      timezone: 'UTC',
      enabled: false,
      scope_type: 'source',
      scope: { source_ids: ['source-1'] },
      mode: 'strict',
      max_concurrency: 3,
      timeout_seconds: 3600,
      source: 'database',
      service_account_id: 'dashboard-schedule:schedule-1',
      managed_by_config_repo: false,
      created_at: '2026-07-15T10:00:00Z',
      updated_at: '2026-07-15T10:00:00Z',
    }));

    assert.equal(form.scopeType, 'source');
    assert.equal(form.sourceID, 'source-1');
    assert.equal(form.timeout, '1h');
    assert.equal(form.cronMode, 'minutes');
    assert.equal(form.enabled, false);
  });
});
