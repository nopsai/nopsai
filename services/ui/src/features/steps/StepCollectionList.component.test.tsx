import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { StepCollectionList } from './StepCollectionList';

const treeRoot = {
  id: '__root__',
  name: '',
  fullPath: '',
  stepIds: [],
  children: [
    {
      id: 'library',
      name: 'library',
      fullPath: 'library',
      stepIds: ['library/build'],
      children: [
        {
          id: 'library/docker',
          name: 'docker',
          fullPath: 'library/docker',
          stepIds: ['library/docker/build-image'],
          children: [],
        },
      ],
    },
    {
      id: 'sandbox',
      name: 'sandbox',
      fullPath: 'sandbox',
      stepIds: ['sandbox/lint'],
      children: [],
    },
  ],
};

test('renders steps in Pipeline Runs style panels and tables', async () => {
  const user = userEvent.setup();
  const onSelectStep = vi.fn();
  const onOpenTeam = vi.fn();
  const onDeleteStep = vi.fn();

  render(
    <StepCollectionList
      listLoading={false}
      listError={null}
      visibleSteps={[
        { id: 'library/build', source: 'git', updatedAt: '2026-07-20T10:15:00Z' },
        { id: 'sandbox/lint', source: 'draft', updatedAt: '2026-07-22T09:30:00Z' },
      ]}
      treeRoot={treeRoot}
      activeTeam=""
      canCreateStepHere
      canUseStepDrafts
      canDeleteSteps
      onSelectStep={onSelectStep}
      onOpenTeam={onOpenTeam}
      onDeleteStep={onDeleteStep}
    />
  );

  expect(screen.getByRole('complementary', { name: 'Teams' })).toHaveClass('pipeline-runs-scope-rail');
  expect(screen.getByRole('button', { name: /All teams/ })).toHaveClass('pipeline-runs-scope-item--active');
  expect(screen.getByRole('region', { name: 'Reusable steps' })).toHaveClass('pipeline-runs-panel');
  expect(screen.queryByRole('heading', { name: 'Reusable steps' })).not.toBeInTheDocument();
  expect(screen.queryByText('2 visible')).not.toBeInTheDocument();
  expect(screen.queryByText('Reusable workflow step')).not.toBeInTheDocument();
  expect(screen.getByTestId('steps-resource-table')).toHaveClass('pipeline-runs-table', 'resource-collection-table');
  expect(screen.queryByTestId('steps-resource-grid')).not.toBeInTheDocument();
  expect(screen.queryByRole('article')).not.toBeInTheDocument();
  expect(screen.queryByRole('region', { name: 'Child teams' })).not.toBeInTheDocument();
  expect(screen.getByText('GitOps')).toBeVisible();
  expect(screen.getByText('Draft')).toBeVisible();
  expect(screen.getByText('2026-07-20')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Open step build' }));
  expect(onSelectStep).toHaveBeenCalledWith('library/build');

  await user.click(screen.getByRole('button', { name: 'Discard draft step lint' }));
  expect(onDeleteStep).toHaveBeenCalledWith('sandbox/lint', 'lint');
  expect(onSelectStep).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('button', { name: 'Expand team library' }));
  await user.click(screen.getByRole('button', { name: 'Open team library/docker' }));
  expect(onOpenTeam).toHaveBeenCalledWith('library/docker');
});

test('hides team tables during search and reports a filtered empty panel', () => {
  render(
    <StepCollectionList
      listLoading={false}
      listError={null}
      visibleSteps={[]}
      treeRoot={treeRoot}
      activeTeam="missing"
      canCreateStepHere
      canUseStepDrafts={false}
      canDeleteSteps={false}
      onSelectStep={() => undefined}
      onOpenTeam={() => undefined}
      onDeleteStep={() => undefined}
    />
  );

  expect(screen.queryByRole('button', { name: 'Open team library/docker' })).not.toBeInTheDocument();
  expect(screen.getByRole('region', { name: 'Reusable steps' })).toHaveClass('pipeline-runs-panel');
  expect(screen.getByRole('complementary', { name: 'Teams' })).toBeVisible();
  expect(screen.getByText('No steps found')).toBeVisible();
  expect(screen.getByText('Create a new step or adjust your filters.')).toBeVisible();
});
