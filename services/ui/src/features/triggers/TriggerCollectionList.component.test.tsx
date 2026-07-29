import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerCollectionList } from './TriggerCollectionList';
import { buildTriggerTree } from './treeModel';

const allTriggers = [
  { slug: 'platform/api', source: 'gitops', scopes: ['prod'], teamPath: 'platform' },
  { slug: 'platform/web', source: 'database', scopes: ['default'], teamPath: 'platform' },
  { slug: 'platform/apps/checkout', source: 'git', scopes: ['dev'], teamPath: 'platform/apps' },
  { slug: 'external/service', source: 'database', scopes: ['prod'], teamPath: 'platform' },
];
const treeRoot = buildTriggerTree(allTriggers);

test('renders subtree table rows and tree navigation', async () => {
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
      activeOwner="platform"
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
  expect(screen.queryByLabelText('Trigger summary')).not.toBeInTheDocument();
  expect(screen.getByText('platform/api').closest('tr')).toHaveClass('selected');
  expect(screen.getByText('platform/apps/checkout')).toBeVisible();
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
