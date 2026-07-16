import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { normalizeDashboardRefreshSchedule, normalizeDashboardSpec } from './model.js';

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
});
