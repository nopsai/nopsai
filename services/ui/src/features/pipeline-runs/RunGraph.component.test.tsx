import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { StepsGraph } from './RunGraph';

const graphSteps = [
  {
    name: 'build',
    status: 'running',
    depends_on: [],
    tasks: [
      {
        task_id: 'task-1',
        step_name: 'build',
        task_name: 'compile',
        status: 'running',
        task_index: 0,
        started_at: '2026-06-08T10:00:00Z',
      },
    ],
  },
  {
    name: 'deploy',
    status: 'failure',
    depends_on: ['build'],
    configuration: { include: 'pipelines/release' },
    tasks: [
      {
        task_id: 'task-2',
        step_name: 'deploy',
        task_name: 'publish',
        status: 'failure',
        exit_code: 1,
        task_index: 0,
      },
    ],
  },
];

test('expands the graph and dispatches task-log and step-detail interactions', async () => {
  const onSelectStep = vi.fn();
  const onOpenTaskLogs = vi.fn();
  const onOpenStepDetail = vi.fn();
  const user = userEvent.setup();

  const { container } = render(
    <StepsGraph
      steps={graphSteps}
      selectedStep={null}
      onSelectStep={onSelectStep}
      onOpenTaskLogs={onOpenTaskLogs}
      onOpenStepDetail={onOpenStepDetail}
      childRuns={[
        {
          run_id: 'child-1',
          pipeline_name: 'release',
          status: 'running',
          parent_step_name: 'deploy',
        },
      ]}
      pipelineDefinition={{
        steps: [
          {
            name: 'build',
            tasks: [{ name: 'compile' }],
          },
          {
            name: 'deploy',
            tasks: [{ name: 'publish' }],
          },
        ],
      }}
    />
  );

  expect(screen.getByText('2 steps')).toBeVisible();
  expect(screen.getByText('2 tasks')).toBeVisible();
  expect(screen.getByText('build')).toBeVisible();
  expect(screen.getByText('Included Pipeline')).toBeVisible();
  expect(container).toHaveTextContent('Child run');

  await user.click(screen.getByRole('button', { name: 'Expand all steps' }));
  expect(screen.getByText('compile')).toBeVisible();
  expect(screen.getByText('publish')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Collapse all steps' })).toBeVisible();

  screen.getByRole('button', { name: /Open logs for compile task/ }).focus();
  await user.keyboard('{Enter}');
  expect(onOpenTaskLogs).toHaveBeenCalledWith('build', 'compile');

  const buildDetails = screen.getByRole('button', { name: 'Open details for build step' });
  fireEvent.mouseEnter(buildDetails, { clientX: 20, clientY: 20 });
  await waitFor(() => expect(screen.getByText(/^Duration:/)).toBeVisible());
  buildDetails.focus();
  await user.keyboard(' ');
  expect(onOpenStepDetail).toHaveBeenCalledWith('build');

  await user.click(screen.getByRole('button', { name: 'Collapse all steps' }));
  expect(screen.queryByText('compile')).not.toBeInTheDocument();
});

test('auto-expands a selected step and supports graph zoom and pan controls', async () => {
  const onSelectStep = vi.fn();
  const { container } = render(
    <StepsGraph
      steps={graphSteps}
      selectedStep="deploy"
      onSelectStep={onSelectStep}
      childRuns={[]}
      hideStatusLegend
      statusVariant="dot"
    />
  );

  await waitFor(() => expect(screen.getByText('publish')).toBeVisible());
  expect(screen.queryByText('Success')).not.toBeInTheDocument();

  const graphLayer = container.querySelector('svg > g');
  expect(graphLayer).not.toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
  expect(graphLayer).toHaveAttribute('transform', expect.stringContaining('scale(1.2)'));
  fireEvent.click(screen.getByRole('button', { name: 'Zoom out' }));
  expect(graphLayer).toHaveAttribute('transform', expect.stringContaining('scale(1)'));

  const graphSurface = container.querySelector('svg')?.parentElement;
  expect(graphSurface).not.toBeNull();
  fireEvent.wheel(graphSurface!, { deltaY: -200 });
  expect(graphLayer).toHaveAttribute('transform', expect.stringContaining('scale(1.2)'));

  fireEvent.mouseDown(graphSurface!, { button: 0, clientX: 40, clientY: 50 });
  fireEvent.mouseMove(graphSurface!, { clientX: 80, clientY: 90 });
  fireEvent.mouseUp(graphSurface!);
  expect(graphLayer?.getAttribute('transform')).toContain('translate(');
});
