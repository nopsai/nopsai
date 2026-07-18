import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { dashboardAttentionSignals } from './dashboardAttention.js';
import type { DashboardRefresh } from './model.js';

describe('dashboard attention signals', () => {
  it('explains cancelled refreshes with operator action instead of raw cancellation text', () => {
    const refresh: DashboardRefresh = {
      id: 'refresh-1',
      dashboard_id: 'dashboard-1',
      dashboard_ref: 'platform/ops',
      trigger_type: 'manual',
      scope_type: 'dashboard',
      mode: 'strict',
      status: 'cancelled',
      total_sources: 2,
      required_sources: 2,
      queued_sources: 0,
      running_sources: 0,
      successful_sources: 0,
      failed_sources: 0,
      skipped_sources: 2,
      max_concurrency: 4,
      timeout_seconds: 2700,
      error: 'dashboard refresh cancelled',
      started_at: '2026-07-15T10:00:00Z',
      finished_at: '2026-07-15T10:00:30Z',
      created_at: '2026-07-15T10:00:00Z',
      updated_at: '2026-07-15T10:00:30Z',
      sources: [],
    };

    const signals = dashboardAttentionSignals({
      sections: [],
      sources: [],
      publications: [],
      latestRefresh: refresh,
      refreshSchedules: [],
    });

    assert.equal(signals.length, 1);
    assert.equal(signals[0].title, 'Latest refresh was cancelled');
    assert.match(signals[0].detail, /stopped this refresh before it finished/);
    assert.match(signals[0].action, /Start another refresh/);
    assert.doesNotMatch(signals[0].detail, /dashboard refresh cancelled/);
  });

  it('prioritizes failed output attempts over lower severity dashboard issues', () => {
    const refresh: DashboardRefresh = {
      id: 'refresh-1',
      dashboard_id: 'dashboard-1',
      dashboard_ref: 'platform/ops',
      trigger_type: 'manual',
      scope_type: 'section',
      scope: { section_key: 'overview' },
      mode: 'strict',
      status: 'failed',
      total_sources: 1,
      required_sources: 1,
      queued_sources: 0,
      running_sources: 0,
      successful_sources: 0,
      failed_sources: 1,
      skipped_sources: 0,
      max_concurrency: 4,
      timeout_seconds: 2700,
      started_at: '2026-07-15T10:00:00Z',
      finished_at: '2026-07-15T10:01:00Z',
      created_at: '2026-07-15T10:00:00Z',
      updated_at: '2026-07-15T10:01:00Z',
      sources: [
        {
          id: 'refresh-source-1',
          refresh_id: 'refresh-1',
          source_binding_id: 'source-1',
          pipeline_id: 'platform/service-health',
          output_name: 'Service Health',
          section_key: 'overview',
          entry_key: 'service-health',
          required: true,
          status: 'failed',
          output_status: 'failure',
          error: 'Dashboard publication validation failed.',
          created_at: '2026-07-15T10:00:00Z',
          updated_at: '2026-07-15T10:01:00Z',
        },
      ],
    };

    const signals = dashboardAttentionSignals({
      sections: [{ id: 'section-1', section_key: 'empty', title: 'Empty', display_order: 10 }],
      sources: [],
      publications: [],
      latestRefresh: refresh,
      refreshSchedules: [],
    });

    assert.equal(signals[0].tone, 'danger');
    assert.equal(signals[0].title, 'platform/service-health / Service Health');
    assert.match(signals[0].action, /retry failed sources/);
  });
});
