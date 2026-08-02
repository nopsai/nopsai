import { render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import { StepDetailModal } from './RunGraphModals';

test('renders step and task LLM usage in the step detail modal', () => {
  render(
    <StepDetailModal
      step={{
        name: 'plan',
        status: 'success',
        depends_on: [],
        duration: '12s',
        ai_usage: { total_tokens: 75, prompt_tokens: 50, completion_tokens: 25 },
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
            ai_usage: { total_tokens: 60, prompt_tokens: 42, completion_tokens: 18 },
          },
        ],
      }}
      onClose={vi.fn()}
      onViewLogs={vi.fn()}
      pipelineDefinition={{ steps: [{ name: 'plan', tasks: [{ name: 'summarize' }] }] }}
    />
  );

  expect(screen.getByText('LLM: 75 tokens')).toBeVisible();
  expect(screen.getByText('Step: plan').closest('.fixed')).toHaveClass('z-[110]');
  expect(screen.getByText('Step: plan').closest('.fixed')).toHaveAttribute('data-run-graph-floating-layer');
  expect(screen.getAllByText('LLM tokens').length).toBeGreaterThan(0);
  expect(screen.getByText('60 tokens')).toBeVisible();
  expect(screen.getByText('42 tokens prompt / 18 tokens completion')).toBeVisible();
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
            ai_usage: { total_tokens: 42 },
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
  expect(screen.getByText('42 tokens')).toBeVisible();
  expect(screen.getByText('Exit code')).toBeVisible();
  expect(screen.getAllByText('1').length).toBeGreaterThan(0);
});
