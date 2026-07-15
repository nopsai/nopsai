import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerCollectionList } from './TriggerCollectionList';
import { buildTriggerTree, findTriggerTreeNode } from './treeModel';

const allTriggers = [
  { slug: 'platform/api', source: 'gitops' },
  { slug: 'platform/web', source: 'database' },
  { slug: 'platform/apps/checkout', source: 'git' },
];
const treeRoot = buildTriggerTree(allTriggers);
const activeOwnerNode = findTriggerTreeNode(treeRoot, 'platform');

test('renders trigger metrics, subtree table rows, and tree navigation', async () => {
  const user = userEvent.setup();
  const onSelectTrigger = vi.fn();
  const onOpenOwner = vi.fn();
  const onDeleteTrigger = vi.fn();

  render(
    <TriggerCollectionList
      listLoading={false}
      listError={null}
      allTriggers={allTriggers}
      visibleTriggers={[
        { slug: 'platform/api', source: 'gitops' },
        { slug: 'platform/web', source: 'database' },
        { slug: 'platform/apps/checkout', source: 'git' },
      ]}
      treeRoot={treeRoot}
      activeOwnerNode={activeOwnerNode}
      activeOwner="platform"
      searchTerm=""
      selectedSlug="platform/api"
      canCreateTriggerHere
      canDeleteTriggers
      onSelectTrigger={onSelectTrigger}
      onOpenOwner={onOpenOwner}
      onDeleteTrigger={onDeleteTrigger}
    />
  );

  expect(screen.getByText('Trigger tree')).toBeVisible();
  expect(screen.getByRole('separator', { name: 'Resize trigger tree' })).toBeVisible();
  expect(screen.getByRole('button', { name: /All owners/ })).toBeVisible();
  expect(screen.getByText('Triggers')).toBeVisible();
  expect(screen.getByText('GitOps')).toBeVisible();
  expect(screen.getByText('Owners')).toBeVisible();
  expect(screen.getByText('platform/api').closest('tr')).toHaveClass('selected');
  expect(screen.getByText('platform/apps/checkout')).toBeVisible();

  await user.click(screen.getByText('platform/web'));
  expect(onSelectTrigger).toHaveBeenCalledTimes(1);
  expect(onSelectTrigger).toHaveBeenCalledWith('platform/web');

  await user.click(screen.getByRole('button', { name: 'Delete api' }));
  expect(onDeleteTrigger).toHaveBeenCalledWith('platform/api');

  await user.click(screen.getByRole('button', { name: 'Open owner platform/apps' }));
  expect(onOpenOwner).toHaveBeenCalledWith('platform/apps');
});
