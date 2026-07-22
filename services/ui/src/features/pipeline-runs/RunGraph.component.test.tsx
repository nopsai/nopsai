import { useState, type ComponentProps } from 'react';
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

type StepsGraphSteps = ComponentProps<typeof StepsGraph>['steps'];

function ControlledStepsGraph({
  steps = graphSteps,
  onSelectionChange,
}: {
  steps?: StepsGraphSteps;
  onSelectionChange?: (step: string | null) => void;
}) {
  const [selectedStep, setSelectedStep] = useState<string | null>(null);
  const handleSelectStep = (step: string | null) => {
    onSelectionChange?.(step);
    setSelectedStep(step);
  };
  return (
    <StepsGraph
      steps={steps}
      selectedStep={selectedStep}
      onSelectStep={handleSelectStep}
      childRuns={[]}
      hideStatusLegend
    />
  );
}

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

test('clicking a step frames that step and its direct graph neighborhood', async () => {
  const onSelectStep = vi.fn();
  const onOpenStepLogs = vi.fn();
  const onOpenTaskLogs = vi.fn();
  const { container } = render(
    <StepsGraph
      steps={[
        { name: 'setup', status: 'success', depends_on: [], tasks: [] },
        {
          name: 'build',
          status: 'running',
          depends_on: ['setup'],
          tasks: [
            {
              task_id: 'task-build',
              step_name: 'build',
              task_name: 'compile',
              status: 'running',
              task_index: 0,
            },
          ],
        },
        { name: 'deploy', status: 'pending', depends_on: ['build'], tasks: [] },
        { name: 'notify', status: 'pending', depends_on: ['deploy'], tasks: [] },
      ]}
      selectedStep={null}
      onSelectStep={onSelectStep}
      onOpenStepLogs={onOpenStepLogs}
      onOpenTaskLogs={onOpenTaskLogs}
      childRuns={[]}
      hideStatusLegend
      pipelineDefinition={{ steps: [{ name: 'build', tasks: [{ name: 'compile' }] }] }}
    />
  );

  const graphSurface = container.querySelector('svg')?.parentElement as HTMLElement | null;
  const graphLayer = container.querySelector('svg > g');
  expect(graphSurface).not.toBeNull();
  Object.defineProperty(graphSurface, 'clientWidth', { configurable: true, value: 960 });
  Object.defineProperty(graphSurface, 'clientHeight', { configurable: true, value: 480 });

  await userEvent.click(screen.getByRole('button', { name: /Expand build step/ }));

  await waitFor(() => {
    expect(onSelectStep).toHaveBeenCalledWith('build');
    const scale = Number(graphLayer?.getAttribute('transform')?.match(/scale\(([^)]+)\)/)?.[1] || 0);
    expect(scale).toBeGreaterThan(1);
    expect(scale).toBeLessThanOrEqual(1.4);
  });
  expect(screen.getByRole('button', { name: /Open logs for compile task in build/ })).toBeVisible();
  expect(onOpenStepLogs).not.toHaveBeenCalled();
});

test('clicking an expanded step collapses it instead of re-expanding from selected state', async () => {
  render(<ControlledStepsGraph />);

  await userEvent.click(screen.getByRole('button', { name: /Expand build step/ }));
  expect(await screen.findByText('compile')).toBeVisible();

  await userEvent.click(screen.getByRole('button', { name: /Collapse build step/ }));
  await waitFor(() => {
    expect(screen.queryByText('compile')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Expand build step/ })).toBeVisible();
  });
});

test('manual graph navigation clears clicked step focus', async () => {
  const onSelectionChange = vi.fn();
  const { container } = render(<ControlledStepsGraph onSelectionChange={onSelectionChange} />);
  const graphSurface = container.querySelector('svg')?.parentElement as HTMLElement | null;
  expect(graphSurface).not.toBeNull();
  Object.defineProperty(graphSurface, 'clientWidth', { configurable: true, value: 960 });
  Object.defineProperty(graphSurface, 'clientHeight', { configurable: true, value: 480 });

  await userEvent.click(screen.getByRole('button', { name: /Expand build step/ }));
  expect(await screen.findByText('compile')).toBeVisible();
  await waitFor(() => expect(onSelectionChange).toHaveBeenCalledWith('build'));

  fireEvent.wheel(graphSurface!, { deltaY: 200 });
  await waitFor(() => expect(onSelectionChange).toHaveBeenLastCalledWith(null));
});

test('clicking a step without tasks opens step logs instead of expanding an empty graph', async () => {
  const onSelectStep = vi.fn();
  const onOpenStepLogs = vi.fn();
  render(
    <StepsGraph
      steps={[{ name: 'deploy', status: 'success', depends_on: [], tasks: [] }]}
      selectedStep={null}
      onSelectStep={onSelectStep}
      onOpenStepLogs={onOpenStepLogs}
      childRuns={[]}
      hideStatusLegend
    />
  );

  await userEvent.click(screen.getByRole('button', { name: /Open logs for deploy step/ }));

  expect(onOpenStepLogs).toHaveBeenCalledWith('deploy');
  expect(onSelectStep).not.toHaveBeenCalled();
});

test('does not render a placeholder task that only repeats the step name', async () => {
  const onOpenStepLogs = vi.fn();
  render(
    <StepsGraph
      steps={[
        {
          name: 'approve',
          status: 'success',
          depends_on: [],
          tasks: [
            {
              task_id: 'placeholder',
              step_name: 'approve',
              task_name: 'approve',
              status: 'success',
              task_index: 0,
            },
          ],
        },
      ]}
      selectedStep={null}
      onSelectStep={() => undefined}
      onOpenStepLogs={onOpenStepLogs}
      childRuns={[]}
      hideStatusLegend
      pipelineDefinition={{ steps: [{ name: 'approve', tasks: [] }] }}
    />
  );

  expect(screen.getByText('0 tasks')).toBeVisible();
  await userEvent.click(screen.getByRole('button', { name: /Open logs for approve step/ }));
  expect(onOpenStepLogs).toHaveBeenCalledWith('approve');
  expect(screen.queryByRole('button', { name: /Open logs for approve task/ })).not.toBeInTheDocument();
});
