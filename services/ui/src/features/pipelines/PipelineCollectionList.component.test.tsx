import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { PipelineCollectionList } from './PipelineCollectionList';

const treeRoot = {
  id: '__root__',
  name: '',
  fullPath: '',
  pipelineIds: [],
  children: [
    {
      id: 'platform',
      name: 'platform',
      fullPath: 'platform',
      pipelineIds: ['platform/deploy'],
      children: [
        {
          id: 'platform/api',
          name: 'api',
          fullPath: 'platform/api',
          pipelineIds: ['platform/api/build'],
          children: [],
        },
      ],
    },
    {
      id: 'sandbox',
      name: 'sandbox',
      fullPath: 'sandbox',
      pipelineIds: ['sandbox/release'],
      children: [],
    },
  ],
};

test('renders pipelines in Pipeline Runs style panels and tables', async () => {
  const user = userEvent.setup();
  const onSelectPipeline = vi.fn();
  const onOpenTeam = vi.fn();
  const onDeletePipeline = vi.fn();

  render(
    <PipelineCollectionList
      listLoading={false}
      listError={null}
      visiblePipelines={[
        { id: 'platform/deploy', source: 'git', version: 'v2', updatedAt: '2026-07-24T09:30:00Z' },
        { id: 'sandbox/release', source: 'draft', updatedAt: '2026-07-25T11:00:00Z' },
      ]}
      treeRoot={treeRoot}
      activeTeam=""
      canCreatePipelineHere
      canUsePipelineDrafts
      canDeletePipelines
      onSelectPipeline={onSelectPipeline}
      onOpenTeam={onOpenTeam}
      onDeletePipeline={onDeletePipeline}
    />
  );

  expect(screen.getByRole('complementary', { name: 'Teams' })).toHaveClass('pipeline-runs-scope-rail');
  expect(screen.getByRole('button', { name: /All teams/ })).toHaveClass('pipeline-runs-scope-item--active');
  expect(screen.getByRole('region', { name: 'Pipeline definitions' })).toHaveClass('pipeline-runs-panel');
  expect(screen.queryByRole('heading', { name: 'Pipeline definitions' })).not.toBeInTheDocument();
  expect(screen.queryByText('2 visible')).not.toBeInTheDocument();
  expect(screen.getByTestId('pipelines-resource-table')).toHaveClass('pipeline-runs-table', 'resource-collection-table');
  expect(screen.queryByTestId('pipelines-resource-grid')).not.toBeInTheDocument();
  expect(screen.queryByRole('article')).not.toBeInTheDocument();
  expect(screen.queryByRole('region', { name: 'Child teams' })).not.toBeInTheDocument();
  expect(screen.getByText('GitOps')).toBeVisible();
  expect(screen.getByText('Draft')).toBeVisible();
  expect(screen.getByText('2026-07-24')).toBeVisible();
  expect(screen.getByText('v2')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Open pipeline deploy' }));
  expect(onSelectPipeline).toHaveBeenCalledWith('platform/deploy');

  await user.click(screen.getByRole('button', { name: 'Discard draft pipeline release' }));
  expect(onDeletePipeline).toHaveBeenCalledWith('sandbox/release', 'release');
  expect(onSelectPipeline).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole('button', { name: 'Expand team platform' }));
  await user.click(screen.getByRole('button', { name: 'Open team platform/api' }));
  expect(onOpenTeam).toHaveBeenCalledWith('platform/api');
});

test('shows a Pipeline Runs style empty panel when no pipeline or team matches', () => {
  render(
    <PipelineCollectionList
      listLoading={false}
      listError={null}
      visiblePipelines={[]}
      treeRoot={{ ...treeRoot, children: [] }}
      activeTeam="missing"
      canCreatePipelineHere={false}
      canUsePipelineDrafts={false}
      canDeletePipelines={false}
      onSelectPipeline={() => undefined}
      onOpenTeam={() => undefined}
      onDeletePipeline={() => undefined}
    />
  );

  expect(screen.getByRole('region', { name: 'Pipeline definitions' })).toHaveClass('pipeline-runs-panel');
  expect(screen.getByRole('complementary', { name: 'Teams' })).toBeVisible();
  expect(screen.getByText('No pipelines found')).toBeVisible();
  expect(screen.getByText('Adjust your filters or check your access.')).toBeVisible();
});
