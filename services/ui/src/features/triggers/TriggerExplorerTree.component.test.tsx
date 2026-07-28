import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerExplorerTree } from './TriggerExplorerTree';
import { buildTriggerTree } from './treeModel';
import type { TriggerListItem } from './model';

const triggers: TriggerListItem[] = [
  { slug: 'external/api', source: 'gitops', teamPath: 'platform' },
  { slug: 'security/checkout', source: 'database', teamPath: 'platform/apps' },
];

test('keeps owner and team branches collapsed by default and opens them on demand', async () => {
  const user = userEvent.setup();
  const onOpenOwner = vi.fn();
  const onOpenTeam = vi.fn();
  const onSelectTrigger = vi.fn();

  render(
    <TriggerExplorerTree
      rootNode={buildTriggerTree(triggers)}
      allTriggers={triggers}
      activeOwnerPath=""
      activeTeamPath=""
      selectedSlug={null}
      onOpenOwner={onOpenOwner}
      onOpenTeam={onOpenTeam}
      onSelectTrigger={onSelectTrigger}
    />
  );

  expect(screen.getByRole('button', { name: 'Expand external' })).toHaveAttribute('aria-expanded', 'false');
  expect(screen.queryByRole('button', { name: 'Open team platform under owner external' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand external' }));

  expect(screen.getByRole('button', { name: 'Open team platform under owner external' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Select trigger external/api' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand platform' }));

  expect(screen.getByRole('button', { name: 'Select trigger external/api' })).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Open team platform under owner external' }));
  expect(onOpenTeam).toHaveBeenCalledWith('external', 'platform');
});

test('opens selected trigger ancestry so deep-linked details remain visible', () => {
  render(
    <TriggerExplorerTree
      rootNode={buildTriggerTree(triggers)}
      allTriggers={triggers}
      activeOwnerPath="external"
      activeTeamPath="platform"
      selectedSlug="external/api"
      onOpenOwner={vi.fn()}
      onOpenTeam={vi.fn()}
      onSelectTrigger={vi.fn()}
    />
  );

  expect(screen.getByRole('button', { name: 'Collapse external' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('button', { name: 'Collapse platform' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('button', { name: 'Select trigger external/api' })).toHaveClass('active');
});
