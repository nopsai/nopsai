import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { StepDetailModal } from './RunGraphModals';

test('renders step and task AI spend in the step detail modal', () => {
  render(
    <StepDetailModal
      step={{
        name: 'plan',
        status: 'success',
        depends_on: [],
        duration: '12s',
        ai_usage: { spend_usd: 0.75 },
        configuration: {
          tasks: [{ name: 'summarize' }],
        },
        tasks: [
          {
            task_id: 'task-1',
            step_name: 'plan',
            task_name: 'summarize',
            status: 'success',
            task_index: 0,
            ai_usage: { spend_usd: 0.6, unpriced_calls: 2 },
          },
        ],
      }}
      onClose={vi.fn()}
      onViewLogs={vi.fn()}
      pipelineDefinition={{ steps: [{ name: 'plan', tasks: [{ name: 'summarize' }] }] }}
    />
  );

  expect(screen.getByText('AI: $0.75')).toBeVisible();
  expect(screen.getByText('Step: plan').closest('.fixed')).toHaveClass('z-[110]');
  expect(screen.getByText('Step: plan').closest('.fixed')).toHaveAttribute('data-run-graph-floating-layer');
  expect(screen.getAllByText('AI spend').length).toBeGreaterThan(0);
  expect(screen.getByText('$0.60')).toBeVisible();
  expect(screen.getByText('2 calls not priced')).toBeVisible();
});

test('opens the step detail modal with an initial task selection', () => {
  render(
    <StepDetailModal
      step={{
        name: 'build',
        status: 'failure',
        depends_on: [],
        configuration: {
          tasks: [
            { name: 'compile' },
            { name: 'package' },
          ],
        },
        tasks: [
          {
            task_id: 'task-1',
            step_name: 'build',
            task_name: 'compile',
            status: 'success',
            task_index: 0,
          },
          {
            task_id: 'task-2',
            step_name: 'build',
            task_name: 'package',
            status: 'failure',
            exit_code: 1,
            task_index: 1,
            ai_usage: { spend_usd: 0.42 },
          },
        ],
      }}
      initialTaskName="package"
      onClose={vi.fn()}
      onViewLogs={vi.fn()}
      pipelineDefinition={{ steps: [{ name: 'build', tasks: [{ name: 'compile' }, { name: 'package' }] }] }}
    />
  );

  expect(screen.getAllByText('package').length).toBeGreaterThan(0);
  expect(screen.getByText('$0.42')).toBeVisible();
  expect(screen.getByText('Exit code')).toBeVisible();
  expect(screen.getAllByText('1').length).toBeGreaterThan(0);
});
