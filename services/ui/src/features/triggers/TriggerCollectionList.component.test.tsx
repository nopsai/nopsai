import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerCollectionList, TriggerMetricGrid } from './TriggerCollectionList';
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

  expect(screen.queryByText('Trigger tree')).not.toBeNull();
  expect(screen.queryByText('Trigger list')).toBeNull();
  expect(screen.queryByRole('separator', { name: 'Resize trigger tree' })).not.toBeNull();
  expect(screen.queryByRole('button', { name: /All owners/ })).not.toBeNull();
  expect(screen.queryByLabelText('Trigger summary')).toBeNull();
  expect(screen.getByText('external/service').closest('tr')?.classList.contains('selected')).toBe(true);
  expect(screen.queryByRole('columnheader', { name: 'Scopes' })).not.toBeNull();
  expect(screen.queryByText('prod')).not.toBeNull();

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

test('omits override and owner metric boxes from trigger summaries', () => {
  render(
    <TriggerMetricGrid
      metrics={{
        total: 4,
        gitManaged: 2,
        databaseManaged: 2,
        ownerCount: 3,
        teamCount: 2,
      }}
    />
  );

  expect(screen.queryByText('Triggers')).not.toBeNull();
  expect(screen.queryByText('GitOps')).not.toBeNull();
  expect(screen.queryByText('Teams')).not.toBeNull();
  expect(screen.queryByText('Overrides')).toBeNull();
  expect(screen.queryByText('Owners')).toBeNull();
});
