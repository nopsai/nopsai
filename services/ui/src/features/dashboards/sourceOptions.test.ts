import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import {
  buildDashboardEntryOptions,
  parseDashboardPipelineOutputOptions,
} from './sourceOptions.js';

describe('dashboard source options', () => {
  it('extracts dashboard final outputs and dashboard target mapping from pipeline YAML', () => {
    const outputs = parseDashboardPipelineOutputOptions(`
name: dashboard-sample
output:
  items:
    - name: Service metrics
      type: dashboard
      when: success
      dashboard:
        ref: team-1/ops-dashboard
        section: service-metrics
        entry_key: dashboard-sample
        mode: replace
        preset: metrics
        ttl: 24h
      prompt: Publish metrics.
    - name: Executive summary
      type: markdown
      prompt: Summarize the run.
steps:
  - name: collect
    script: echo ok
`);

    assert.equal(outputs.length, 1);
    assert.deepEqual(outputs[0], {
      name: 'Service metrics',
      type: 'dashboard',
      when: 'success',
      dashboardRef: 'team-1/ops-dashboard',
      sectionKey: 'service-metrics',
      entryKey: 'dashboard-sample',
      mode: 'replace',
      preset: 'metrics',
      ttl: '24h',
    });
  });

  it('builds entry dropdown options from the selected output without duplicates', () => {
    const options = buildDashboardEntryOptions({
      output: {
        name: 'Service metrics',
        type: 'dashboard',
        when: 'success',
        dashboardRef: 'team-1/ops-dashboard',
        sectionKey: 'service-metrics',
        entryKey: 'dashboard-sample',
        mode: 'replace',
        preset: 'metrics',
        ttl: '',
      },
      currentEntryKey: 'dashboard-sample',
      existingEntryKeys: ['legacy-entry', 'dashboard-sample'],
    });

    assert.deepEqual(options, [
      { value: '', label: 'Use output name (Service metrics)' },
      { value: 'dashboard-sample', label: 'dashboard-sample' },
      { value: 'legacy-entry', label: 'legacy-entry' },
    ]);
  });

  it('returns no options for invalid YAML or non-dashboard outputs', () => {
    assert.deepEqual(parseDashboardPipelineOutputOptions('output: ['), []);
    assert.deepEqual(parseDashboardPipelineOutputOptions(`
output:
  items:
    - name: Report
      type: pdf
      prompt: Write a report.
`), []);
  });
});
