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
      pipelineDefinition={{
        steps: [
          { name: 'build', tasks: [{ name: 'compile' }] },
          { name: 'deploy', tasks: [{ name: 'publish' }] },
        ],
      }}
    />
  );
}

test('reveals a selected step task graph and dispatches task-log and step-detail interactions', async () => {
  const onSelectStep = vi.fn();
  const onOpenTaskLogs = vi.fn();
  const onOpenStepDetail = vi.fn();
  const user = userEvent.setup();

  render(
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
          { name: 'build', tasks: [{ name: 'compile' }] },
          { name: 'deploy', tasks: [{ name: 'publish' }] },
        ],
      }}
    />
  );

  expect(screen.getByText('Execution Graph')).toBeVisible();
  expect(screen.getByText('2 steps')).toBeVisible();
  expect(screen.getByText('2 tasks')).toBeVisible();
  expect(screen.getByText('Included Pipeline')).toBeVisible();
  expect(screen.queryByText('Child run')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Open full details for build step' }));
  expect(onOpenStepDetail).toHaveBeenCalledWith('build');
  expect(onSelectStep).not.toHaveBeenCalled();

  await user.click(screen.getByRole('button', { name: /Reveal build step/ }));

  expect(onSelectStep).toHaveBeenCalledWith('build');
  expect(screen.getByText('compile')).toBeVisible();
  expect(screen.queryByText('Selection Details')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Open full details for compile task in build' }));
  expect(onOpenStepDetail).toHaveBeenCalledWith('build', 'compile');

  screen.getByRole('button', { name: /Open logs for compile task in build/ }).focus();
  await user.keyboard('{Enter}');
  expect(onOpenTaskLogs).toHaveBeenCalledWith('build', 'compile');
});

test('embedded presentation renders the graph canvas without nested graph chrome', () => {
  render(
    <StepsGraph
      steps={graphSteps}
      selectedStep={null}
      onSelectStep={() => undefined}
      childRuns={[]}
      hideStatusLegend
      ariaLabel="Pipeline graph"
      presentation="embedded"
      pipelineDefinition={{
        steps: [
          { name: 'build', tasks: [{ name: 'compile' }] },
          { name: 'deploy', tasks: [{ name: 'publish' }] },
        ],
      }}
    />
  );

  expect(screen.getByRole('region', { name: 'Pipeline graph' })).toHaveAttribute('data-presentation', 'embedded');
  expect(screen.queryByText('Execution Graph')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Search graph nodes')).toBeVisible();
  expect(screen.getByRole('button', { name: /Reveal build step/ })).toBeVisible();
});

test('auto-reveals a selected step and supports graph zoom and pan controls', async () => {
  const onSelectStep = vi.fn();
  const { container } = render(
    <StepsGraph
      steps={graphSteps}
      selectedStep="deploy"
      onSelectStep={onSelectStep}
      childRuns={[]}
      hideStatusLegend
      statusVariant="dot"
      pipelineDefinition={{ steps: [{ name: 'deploy', tasks: [{ name: 'publish' }] }] }}
    />
  );

  await waitFor(() => expect(screen.getByText('publish')).toBeVisible());

  const graphLayer = container.querySelector('.run-graph-overview > g');
  expect(graphLayer).not.toBeNull();
  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
  expect(graphLayer).toHaveAttribute('transform', expect.stringContaining('scale(1.32)'));
  fireEvent.click(screen.getByRole('button', { name: 'Zoom out' }));
  expect(graphLayer).toHaveAttribute('transform', expect.stringContaining('scale(1)'));

  const graphSurface = container.querySelector('.run-graph-workspace') as HTMLElement | null;
  expect(graphSurface).not.toBeNull();
  fireEvent.wheel(graphSurface!, { deltaY: -200 });
  expect(graphTransform(container).scale).toBe(1);
  fireEvent.wheel(graphSurface!, { ctrlKey: true, deltaY: -200 });
  await waitFor(() => {
    const scaleValue = Number(graphLayer?.getAttribute('transform')?.match(/scale\(([^)]+)\)/)?.[1] || 0);
    expect(scaleValue).toBeGreaterThan(1.4);
  });
  expect(screen.getByRole('group', { name: 'Graph navigator' })).toBeVisible();

  const minimap = container.querySelector('.run-graph-minimap-svg') as SVGSVGElement | null;
  expect(minimap).not.toBeNull();
  vi.spyOn(minimap!, 'getBoundingClientRect').mockReturnValue({
    x: 0,
    y: 0,
    left: 0,
    top: 0,
    right: 168,
    bottom: 82,
    width: 168,
    height: 82,
    toJSON: () => ({}),
  } as DOMRect);
  const zoomedTransform = graphTransform(container);
  fireEvent.mouseDown(minimap!, { button: 0, clientX: 150, clientY: 68 });
  await waitFor(() => {
    expect(graphTransform(container).x).not.toBeCloseTo(zoomedTransform.x);
  });
  fireEvent.mouseUp(document);

  fireEvent.mouseDown(graphSurface!, { button: 0, clientX: 40, clientY: 50 });
  fireEvent.mouseMove(graphSurface!, { clientX: 80, clientY: 90 });
  fireEvent.mouseUp(graphSurface!);
  expect(graphLayer?.getAttribute('transform')).toContain('translate(');
});

test('centers the graph by default and recenters when the run changes', () => {
  const onSelectStep = vi.fn();
  const singleStep = [{ name: 'generate-data', status: 'success', depends_on: [], tasks: [] }];
  const { container, rerender } = render(
    <StepsGraph
      graphKey="run-1"
      steps={singleStep}
      selectedStep={null}
      onSelectStep={onSelectStep}
      childRuns={[]}
      hideStatusLegend
    />
  );

  const initial = graphTransform(container);
  expect(initial.scale).toBe(1);
  expect(initial.x).toBeGreaterThan(300);
  expect(initial.y).toBeGreaterThan(50);

  fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
  expect(graphTransform(container).scale).toBeGreaterThan(1);

  rerender(
    <StepsGraph
      graphKey="run-2"
      steps={singleStep}
      selectedStep={null}
      onSelectStep={onSelectStep}
      childRuns={[]}
      hideStatusLegend
    />
  );

  const recentered = graphTransform(container);
  expect(recentered.scale).toBe(1);
  expect(recentered.x).toBeCloseTo(initial.x);
  expect(recentered.y).toBeCloseTo(initial.y);
});

test('clicking an open step collapses the task reveal', async () => {
  render(<ControlledStepsGraph />);

  await userEvent.click(screen.getByRole('button', { name: /Reveal build step/ }));
  expect(await screen.findByText('compile')).toBeVisible();

  await userEvent.click(screen.getByRole('button', { name: /Collapse build step/ }));
  await waitFor(() => {
    expect(screen.queryByText('compile')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Reveal build step/ })).toBeVisible();
  });
});

test('modified wheel zoom keeps an open task reveal selected and click-away closes it', async () => {
  const onSelectionChange = vi.fn();
  const { container } = render(<ControlledStepsGraph onSelectionChange={onSelectionChange} />);
  const graphSurface = container.querySelector('.run-graph-workspace') as HTMLElement | null;
  const graphLayer = container.querySelector('.run-graph-overview > g');
  expect(graphSurface).not.toBeNull();
  expect(graphLayer).not.toBeNull();

  await userEvent.click(screen.getByRole('button', { name: /Reveal build step/ }));
  expect(await screen.findByText('compile')).toBeVisible();
  await waitFor(() => expect(onSelectionChange).toHaveBeenCalledWith('build'));

  fireEvent.wheel(graphSurface!, { deltaY: -200 });
  expect(graphTransform(container).scale).toBe(1);

  fireEvent.wheel(graphSurface!, { ctrlKey: true, deltaY: -200 });
  await waitFor(() => {
    expect(graphTransform(container).scale).toBeGreaterThan(1);
  });
  expect(screen.getByText('compile')).toBeVisible();
  expect(onSelectionChange).not.toHaveBeenCalledWith(null);

  fireEvent.mouseDown(document.body);
  await waitFor(() => {
    expect(screen.queryByText('compile')).not.toBeInTheDocument();
  });
  expect(onSelectionChange).toHaveBeenLastCalledWith(null);

  await userEvent.click(screen.getByRole('button', { name: /Reveal build step/ }));
  expect(await screen.findByText('compile')).toBeVisible();

  fireEvent.mouseDown(graphSurface!, { button: 0, clientX: 24, clientY: 24 });
  fireEvent.mouseUp(graphSurface!, { clientX: 24, clientY: 24 });
  await waitFor(() => {
    expect(screen.queryByText('compile')).not.toBeInTheDocument();
  });
  expect(onSelectionChange).toHaveBeenLastCalledWith(null);
});

test('clicking a step without tasks opens step logs instead of revealing an empty graph', async () => {
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

function graphTransform(container: HTMLElement) {
  const graphLayer = container.querySelector('.run-graph-overview > g');
  const transform = graphLayer?.getAttribute('transform') || '';
  const match = /translate\(([^,]+), ([^)]+)\) scale\(([^)]+)\)/.exec(transform);
  if (!match) throw new Error(`Unexpected graph transform: ${transform}`);
  return {
    x: Number(match[1]),
    y: Number(match[2]),
    scale: Number(match[3]),
  };
}
