import { createRef, type ComponentProps } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { StepDetailView } from './StepDetailView';

function buildProps(overrides: Partial<ComponentProps<typeof StepDetailView>> = {}): ComponentProps<typeof StepDetailView> {
  const props: ComponentProps<typeof StepDetailView> = {
    detail: {
      id: 'library/docker/build-image',
      name: 'build-image',
      path: 'library/docker',
      description: 'Build and push reusable OCI images.',
      rawYaml: [
        'name: build-image',
        'description: Build and push reusable OCI images.',
        'script: docker build .',
      ].join('\n'),
      source: 'git',
      updatedAt: '2026-07-29T09:00:00Z',
    },
    isEditing: false,
    editorValue: 'name: build-image\nscript: docker build .',
    validationErrors: [],
    validationErrorLines: new Set(),
    editorSuggestion: null,
    autocompleteLoading: false,
    editorRef: createRef<HTMLTextAreaElement>(),
    highlightContentRef: createRef<HTMLPreElement>(),
    lineNumbersRef: createRef<HTMLDivElement>(),
    canUpdateSelectedStep: true,
    canCreateStepHere: true,
    saving: false,
    editableStepName: 'build-image',
    editableStepTeam: 'library/docker',
    usage: [
      {
        identifier: 'platform/release',
        name: 'release',
        path: 'platform',
        source: 'git',
        description: 'Release pipeline',
      },
      {
        identifier: 'sandbox/nightly',
        name: 'nightly',
        path: 'sandbox',
        source: 'database',
        description: 'Nightly validation',
      },
    ],
    usageLoading: false,
    usageError: null,
    onBack: vi.fn(),
    onCopy: vi.fn(),
    onDownload: vi.fn(),
    onEdit: vi.fn(),
    onClone: vi.fn(),
    onDiscard: vi.fn(),
    onSave: vi.fn(),
    onCopyIdentifier: vi.fn(),
    onEditableStepNameChange: vi.fn(),
    onEditableStepTeamChange: vi.fn(),
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

test('renders the redesigned step detail hero, definition tab, and usage table', async () => {
  const user = userEvent.setup();
  const props = buildProps();

  render(
    <MemoryRouter>
      <StepDetailView {...props} />
    </MemoryRouter>
  );

  expect(screen.getByRole('heading', { name: 'build-image' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Back' })).toBeVisible();
  expect(screen.getAllByText('library/docker').some(node => node.classList.contains('pipeline-detail-back-context'))).toBe(true);
  expect(screen.getAllByText('GitOps').length).toBeGreaterThan(0);
  expect(screen.getByLabelText('Step metadata')).toHaveTextContent('library/docker');
  expect(screen.getByRole('tabpanel')).toHaveAttribute('id', 'step-detail-panel-definition');
  expect(screen.getByText('Step Definition (YAML)')).toBeVisible();
  expect(screen.getByText('YAML valid')).toBeVisible();
  expect(screen.queryByText('Identifier:')).not.toBeInTheDocument();

  await user.click(screen.getByLabelText('Copy identifier library/docker/build-image'));
  expect(props.onCopyIdentifier).toHaveBeenCalledWith('library/docker/build-image');

  await user.click(screen.getByLabelText('Copy YAML'));
  expect(props.onCopy).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('button', { name: 'Clone' }));
  expect(props.onClone).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('button', { name: 'Edit' }));
  expect(props.onEdit).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('tab', { name: /Used in pipelines/ }));
  const pipelineLink = screen.getByRole('link', { name: 'Open pipeline platform/release' });
  expect(pipelineLink).toHaveAttribute('href', '/pipelines/platform/release');
  expect(screen.getByRole('table', { name: 'Step usage' })).toBeVisible();
});

test('keeps save and discard actions in the hero while editing', async () => {
  const user = userEvent.setup();
  const props = buildProps({ isEditing: true, saveBlocked: true });

  render(
    <MemoryRouter>
      <StepDetailView {...props} />
    </MemoryRouter>
  );

  expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
  expect(screen.getByLabelText('Expand YAML editor')).toBeVisible();

  fireEvent.change(screen.getByLabelText('Team'), { target: { value: 'platform/shared' } });
  expect(props.onEditableStepTeamChange).toHaveBeenLastCalledWith('platform/shared');

  fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'publish-image' } });
  expect(props.onEditableStepNameChange).toHaveBeenLastCalledWith('publish-image');

  await user.click(screen.getByRole('button', { name: 'Discard' }));
  expect(props.onDiscard).toHaveBeenCalledTimes(1);
});
