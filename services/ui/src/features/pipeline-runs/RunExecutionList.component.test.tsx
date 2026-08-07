import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import type { StepDetail } from './contracts';
import { RunExecutionList } from './RunExecutionList';

const runSteps: StepDetail[] = [
  {
    name: 'checkout',
    status: 'success',
    depends_on: [],
    duration: '1m 34s',
    tasks: [],
  },
  {
    name: 'build',
    status: 'failure',
    depends_on: ['checkout'],
    duration: '1m 10s',
    configuration: { include: 'steps/shared/build' },
    tasks: [
      {
        task_id: 'task-1',
        step_name: 'build',
        task_name: 'compile',
        status: 'success',
        task_index: 0,
        started_at: '2026-06-08T10:00:00Z',
        finished_at: '2026-06-08T10:00:22Z',
      },
      {
        task_id: 'task-2',
        step_name: 'build',
        task_name: 'test',
        status: 'failure',
        task_index: 1,
        started_at: '2026-06-08T10:00:00Z',
        finished_at: '2026-06-08T10:00:20Z',
        exit_code: 1,
      },
      {
        task_id: 'task-3',
        step_name: 'build',
        task_name: 'lint',
        status: 'pending',
        task_index: 2,
      },
    ],
  },
  {
    name: 'deploy',
    status: 'pending',
    depends_on: ['build'],
    tasks: [],
  },
];

test('renders flat execution log lines with status, unit names, and log actions', async () => {
  const user = userEvent.setup();
  const onSelectStep = vi.fn();
  const onOpenStepLogs = vi.fn();
  const onOpenTaskLogs = vi.fn();
  const onOpenStepDetail = vi.fn();

  render(
    <RunExecutionList
      runID="run-1"
      steps={runSteps}
      selectedStep={null}
      onSelectStep={onSelectStep}
      onOpenStepLogs={onOpenStepLogs}
      onOpenTaskLogs={onOpenTaskLogs}
      onOpenStepDetail={onOpenStepDetail}
      childRuns={[]}
      pipelineDefinition={{
        steps: [
          { name: 'checkout' },
          {
            name: 'build',
            tasks: [
              { name: 'compile' },
              { name: 'test', depends_on: ['compile'] },
              { name: 'lint', depends_on: ['compile'] },
            ],
          },
          { name: 'deploy' },
        ],
      }}
    />
  );

  expect(screen.getByRole('region', { name: 'Pipeline execution list' })).toBeVisible();
  expect(screen.queryByText(/stage/i)).not.toBeInTheDocument();
  expect(screen.queryByText(/^step$/i)).not.toBeInTheDocument();
  expect(screen.queryByText(/^task$/i)).not.toBeInTheDocument();
  expect(screen.getAllByText('success').length).toBeGreaterThan(0);
  expect(screen.getByText('failure')).toBeVisible();
  expect(screen.getByText('skipped')).toBeVisible();
  expect(screen.getAllByText('checkout').length).toBeGreaterThan(0);
  expect(screen.getByText('compile')).toBeVisible();
  expect(screen.getByText('(took 1m 34s)')).toBeVisible();
  expect(screen.getByText('(took 22s)')).toBeVisible();

  await user.click(screen.getByRole('button', { name: /checkout.*success/i }));
  expect(onSelectStep).toHaveBeenCalledWith(null);
  expect(onOpenStepLogs).toHaveBeenCalledWith('checkout');

  await user.click(screen.getByRole('button', { name: /build.*compile.*success/i }));
  expect(onSelectStep).toHaveBeenCalledWith('build');
  expect(onOpenTaskLogs).toHaveBeenCalledWith('build', 'compile');
  expect(onOpenStepDetail).not.toHaveBeenCalled();
});
