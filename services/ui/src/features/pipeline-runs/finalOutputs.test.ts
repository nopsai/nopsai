import assert from 'node:assert/strict';
import test from 'node:test';
import type { PipelineDefinition, PipelineRunFinalOutput } from './contracts.js';
import { finalOutputDashboardLink, finalOutputDashboardTarget, runFinalOutputStatusPresentation } from './finalOutputs.js';

test('uses stored dashboard target metadata for final output links', () => {
  const output = finalOutput({
    dashboard_target: {
      ref: ' /platform/ops ',
      section: 'overview',
      entry_key: '',
      mode: 'replace',
      preset: 'summary',
    },
  });

  assert.deepEqual(finalOutputDashboardTarget(output), {
    ref: 'platform/ops',
    section: 'overview',
    entryKey: 'Release health',
    mode: 'replace',
    preset: 'summary',
    ttl: '',
  });
  assert.deepEqual(finalOutputDashboardLink(output), {
    ref: 'platform/ops',
    section: 'overview',
    entryKey: 'Release health',
    mode: 'replace',
    preset: 'summary',
    ttl: '',
    href: '/dashboards/platform/ops?tab=overview',
    label: 'platform/ops / overview',
  });
});

test('falls back to pipeline definition dashboard target metadata', () => {
  const output = finalOutput({ dashboard_target: undefined });
  const definition: PipelineDefinition = {
    output: {
      items: [
        { name: 'Other', type: 'dashboard', prompt: 'other', dashboard: { ref: 'ignored', section: 'overview' } },
        { name: 'Release health', type: 'dashboard', prompt: 'health', dashboard: { ref: 'platform/ops', section: 'releases', entry_key: 'release-health' } },
      ],
    },
  };

  assert.equal(finalOutputDashboardTarget(output, definition)?.section, 'releases');
  assert.equal(finalOutputDashboardTarget({ ...output, type: 'markdown' }, definition), null);
});

test('formats run list final output status summaries', () => {
  assert.deepEqual(runFinalOutputStatusPresentation({
    status: 'generating',
    configured: 2,
    total: 2,
    pending: 0,
    generating: 1,
    generated: 1,
    failed: 0,
    cancelled: 0,
  }), {
    label: 'Output generating',
    detail: '1 generated, 1 generating',
    title: 'Output generating: 1 generated, 1 generating',
    className: 'runner-pill--warning',
  });

  assert.deepEqual(runFinalOutputStatusPresentation({
    status: 'waiting',
    configured: 1,
    total: 0,
    pending: 0,
    generating: 0,
    generated: 0,
    failed: 0,
    cancelled: 0,
  }), {
    label: 'Output waiting',
    detail: '1 configured',
    title: 'Output waiting: 1 configured',
    className: 'runner-pill--warning',
  });

  assert.equal(runFinalOutputStatusPresentation(undefined), null);
});

function finalOutput(overrides: Partial<PipelineRunFinalOutput> = {}): PipelineRunFinalOutput {
  return {
    id: 'output-1',
    name: 'Release health',
    type: 'dashboard',
    status: 'success',
    ...overrides,
  };
}
