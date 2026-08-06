import { createRef, type ComponentProps } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { PipelineDetailView } from './PipelineDetailView';

vi.mock('../pipeline-runs/RunGraph', () => ({
  StepsGraph: ({
    steps,
    onSelectStep,
    presentation,
    ariaLabel,
  }: {
    steps: Array<{ name: string }>;
    onSelectStep: (step: string | null) => void;
    presentation?: string;
    ariaLabel?: string;
  }) => (
    <button
      type="button"
      data-aria-label={ariaLabel}
      data-presentation={presentation}
      onClick={() => onSelectStep(steps[0]?.name || null)}
    >
      Graph with {steps.length} steps
    </button>
  ),
}));

function buildProps(overrides: Partial<ComponentProps<typeof PipelineDetailView>> = {}): ComponentProps<typeof PipelineDetailView> {
  const props: ComponentProps<typeof PipelineDetailView> = {
    detail: {
      id: 'platform/release',
      name: 'release',
      description: 'Build and publish service release.',
      version: 'latest',
      path: 'platform',
      rawYaml: [
        'name: release',
        'description: Build and publish service release.',
        'container_image: alpine:3.20',
        'steps:',
        '  - name: build',
        '    script: echo build',
      ].join('\n'),
      stepNames: ['build', 'publish'],
      variables: [],
      includedDependencies: ['pipeline:platform/build-base', 'step:shared/notify'],
      dependencyEdges: [{ from: 'build', to: 'publish' }],
      containerImage: 'alpine:3.20',
      source: 'git',
      updatedAt: '2026-07-29T09:00:00Z',
    },
    graphData: {
      error: null,
      definition: {
        name: 'release',
        steps: [{ name: 'build' }, { name: 'publish', depends_on: ['build'] }],
      },
      steps: [
        {
          name: 'build',
          status: 'success',
          depends_on: [],
          tasks: [],
          configuration: { script: 'echo build' },
        },
        {
          name: 'publish',
          status: 'success',
          depends_on: ['build'],
          tasks: [{
            task_id: 'publish-upload',
            step_name: 'publish',
            task_name: 'upload',
            status: 'pending',
            task_index: 0,
          }],
          configuration: { tasks: [{ name: 'upload' }] },
        },
      ],
    },
    selectedGraphStep: null,
    isEditing: false,
    editorValue: 'name: release\nsteps: []',
    validationErrors: [],
    validationErrorLines: new Set(),
    editorSuggestion: null,
    autocompleteLoading: false,
    editorRef: createRef<HTMLTextAreaElement>(),
    highlightContentRef: createRef<HTMLPreElement>(),
    lineNumbersRef: createRef<HTMLDivElement>(),
    canUpdateSelectedPipeline: true,
    canCreatePipelineHere: true,
    canExecuteSelectedPipeline: true,
    saving: false,
    editablePipelineName: 'release',
    editablePipelineTeam: 'platform',
    triggers: [{ repoSlug: 'acme/api', source: 'git', trigger: { on: 'push', branches: ['main'] } }],
    triggersLoading: false,
    triggersError: null,
    recentRuns: [{
      run_id: 'run-123456789',
      pipeline_name: 'release',
      status: 'success',
      git_ref: 'refs/heads/main',
      started_at: '2026-07-29T09:05:00Z',
    }],
    runsLoading: false,
    runsError: null,
    onBack: vi.fn(),
    onExecute: vi.fn(),
    onCopy: vi.fn(),
    onDownload: vi.fn(),
    onEdit: vi.fn(),
    onClone: vi.fn(),
    onDiscard: vi.fn(),
    onSave: vi.fn(),
    onEditablePipelineNameChange: vi.fn(),
    onEditablePipelineTeamChange: vi.fn(),
    onSelectGraphStep: vi.fn(),
    onOpenTrigger: vi.fn(),
    onOpenDependency: vi.fn(),
    onCopyDependency: vi.fn(),
    onOpenRun: vi.fn(),
    onEditorTextChange: vi.fn(),
    onOpenSuggestion: vi.fn(),
    onMoveSuggestion: vi.fn(),
    onDismissSuggestion: vi.fn(),
    onSelectSuggestion: vi.fn(),
    onEditorScroll: vi.fn(),
    onAutoIndentEnter: vi.fn(),
  };
  return { ...props, ...overrides };
}

test('keeps pipeline detail actions and tab callbacks wired after redesign', async () => {
  const user = userEvent.setup();
  const props = buildProps();

  render(<PipelineDetailView {...props} />);

  expect(screen.getByRole('heading', { name: 'release' })).toBeVisible();
  expect(screen.queryByLabelText('Pipeline summary')).not.toBeInTheDocument();
  expect(screen.getByText('Graph with 2 steps')).toHaveAttribute('data-presentation', 'embedded');
  expect(screen.getByText('Graph with 2 steps')).toHaveAttribute('data-aria-label', 'Pipeline graph');

  await user.click(screen.getByRole('button', { name: 'Execute' }));
  expect(props.onExecute).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('button', { name: 'Edit' }));
  expect(props.onEdit).toHaveBeenCalledTimes(1);
  expect(screen.getByText('Pipeline Definition (YAML)')).toBeVisible();

  await user.click(screen.getByLabelText('Copy YAML'));
  expect(props.onCopy).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('tab', { name: /Trigger rules/ }));
  await user.click(screen.getByTitle('Open trigger acme/api'));
  expect(props.onOpenTrigger).toHaveBeenCalledWith('acme/api');

  await user.click(screen.getByRole('tab', { name: /Runs/ }));
  await user.click(screen.getByTitle('Open run run-123456789'));
  expect(props.onOpenRun).toHaveBeenCalledWith('run-123456789');

  await user.click(screen.getByRole('tab', { name: /Dependencies/ }));
  await user.click(screen.getByTitle('Open platform/build-base'));
  expect(props.onOpenDependency).toHaveBeenCalledWith(expect.objectContaining({
    kind: 'pipeline',
    identifier: 'platform/build-base',
  }));
  await user.click(screen.getByTitle('Open shared/notify'));
  expect(props.onOpenDependency).toHaveBeenCalledWith(expect.objectContaining({
    kind: 'step',
    identifier: 'shared/notify',
  }));
  await user.click(screen.getByTitle('Open build'));
  expect(props.onOpenDependency).toHaveBeenCalledWith(expect.objectContaining({
    kind: 'local-step',
    targetStep: 'build',
    sourceStep: 'publish',
  }));
  expect(screen.getByText('Graph with 2 steps')).toBeVisible();
});
