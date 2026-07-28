import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerCollectionList } from './TriggerCollectionList';
import { buildTriggerTree, findTriggerTreeNode } from './treeModel';

const allTriggers = [
  { slug: 'platform/api', source: 'gitops', scopes: ['prod'], teamPath: 'platform' },
  { slug: 'platform/web', source: 'database', scopes: ['default'], teamPath: 'platform' },
  { slug: 'platform/apps/checkout', source: 'git', scopes: ['dev'], teamPath: 'platform/apps' },
  { slug: 'external/service', source: 'database', scopes: ['prod'], teamPath: 'platform' },
];
const treeRoot = buildTriggerTree(allTriggers);
const activeTreeNode = findTriggerTreeNode(treeRoot, 'external', 'platform');

test('renders trigger metrics, subtree table rows, and tree navigation', async () => {
  const user = userEvent.setup();
  const onSelectTrigger = vi.fn();
  const onOpenOwner = vi.fn();
  const onOpenTeam = vi.fn();
  const onDeleteTrigger = vi.fn();

  render(
    <TriggerCollectionList
      listLoading={false}
      listError={null}
      allTriggers={allTriggers}
      visibleTriggers={[
        { slug: 'external/service', source: 'database', scopes: ['prod'], teamPath: 'platform' },
      ]}
      treeRoot={treeRoot}
      activeTreeNode={activeTreeNode}
      activeOwnerPath="external"
      activeTeamPath="platform"
      searchTerm=""
      selectedSlug="external/service"
      canCreateTriggerHere
      canDeleteTriggers
      onSelectTrigger={onSelectTrigger}
      onOpenOwner={onOpenOwner}
      onOpenTeam={onOpenTeam}
      onDeleteTrigger={onDeleteTrigger}
    />
  );

  expect(screen.getByText('Trigger tree')).toBeVisible();
  expect(screen.getByRole('separator', { name: 'Resize trigger tree' })).toBeVisible();
  expect(screen.getByRole('button', { name: /All owners/ })).toBeVisible();
  expect(screen.getByText('Triggers')).toBeVisible();
  expect(screen.getByText('GitOps')).toBeVisible();
  expect(screen.getByText('Owners')).toBeVisible();
  expect(screen.getByText('Teams')).toBeVisible();
  expect(screen.getByRole('heading', { name: 'external / platform' })).toBeVisible();
  expect(screen.getByText('external/service').closest('tr')).toHaveClass('selected');
  expect(screen.queryByText('platform/api')).not.toBeInTheDocument();
  expect(screen.getByRole('columnheader', { name: 'Scopes' })).toBeVisible();
  expect(screen.getByText('prod')).toBeVisible();

  await user.click(screen.getByText('external/service'));
  expect(onSelectTrigger).toHaveBeenCalledTimes(1);
  expect(onSelectTrigger).toHaveBeenCalledWith('external/service');

  await user.click(screen.getByRole('button', { name: 'Delete service' }));
  expect(onDeleteTrigger).toHaveBeenCalledWith('external/service');

  await user.click(screen.getByRole('button', { name: 'Open owner platform' }));
  expect(onOpenOwner).toHaveBeenCalledWith('platform');

  await user.click(screen.getByRole('button', { name: 'Open team platform under owner external' }));
  expect(onOpenTeam).toHaveBeenCalledWith('external', 'platform');
});
