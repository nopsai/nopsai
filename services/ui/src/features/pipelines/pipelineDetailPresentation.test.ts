import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { AnalysisResult } from '../analysis/model.js';
import type { PipelineGraphData } from './model.js';
import {
  buildPipelineDetailHealthSummary,
  countPipelineGraphTasks,
  formatPipelineDetailSource,
  summarizePipelineLatestRun,
} from './pipelineDetailPresentation.js';

const graphData: PipelineGraphData = {
  error: null,
  definition: undefined,
  steps: [
    {
      name: 'prepare',
      status: 'success',
      depends_on: [],
      tasks: [
        {
          task_id: 'prepare-install',
          step_name: 'prepare',
          task_name: 'install',
          status: 'pending',
          task_index: 0,
        },
      ],
      configuration: { tasks: [{ name: 'install' }] },
    },
    {
      name: 'publish',
      status: 'success',
      depends_on: ['prepare'],
      tasks: [],
      configuration: { script: 'echo publish' },
    },
  ],
};

test('counts graph work units from explicit tasks and single-action steps', () => {
  assert.equal(countPipelineGraphTasks(graphData), 2);
});

test('formats source state for GitOps and drafts', () => {
  assert.deepEqual(formatPipelineDetailSource('git'), {
    label: 'GitOps',
    tone: 'success',
    description: 'Synced from configuration repository',
  });
  assert.deepEqual(formatPipelineDetailSource('draft'), {
    label: 'Draft',
    tone: 'warning',
    description: 'Local draft, save before execution',
  });
});

test('builds health summary from current page data', () => {
  const analysis = {
    healthScore: 72,
    findings: [
      {
        severity: 'high',
        confidence: 91,
        title: 'Container images are not pinned',
        category: 'security',
      },
    ],
  } as unknown as AnalysisResult;

  assert.deepEqual(buildPipelineDetailHealthSummary(analysis), {
    score: 72,
    label: 'Needs attention',
    tone: 'danger',
    findingLabel: '1 high-priority finding',
  });
});

test('summarizes the latest run without leaking long identifiers into the header', () => {
  assert.deepEqual(summarizePipelineLatestRun([{
    run_id: 'run-123456789',
    pipeline_name: 'release',
    status: 'failed',
    git_ref: 'refs/heads/feature/api',
  }]), {
    statusLabel: 'Failed',
    branchLabel: 'feature/api',
    runLabel: 'run-1234',
  });
});
