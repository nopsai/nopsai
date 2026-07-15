import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerExplorerTree } from './TriggerExplorerTree';
import { buildTriggerTree } from './treeModel';
import type { TriggerListItem } from './model';

const triggers: TriggerListItem[] = [
  { slug: 'platform/api', source: 'gitops' },
  { slug: 'platform/apps/checkout', source: 'database' },
];

test('keeps owner branches collapsed by default and opens them on demand', async () => {
  const user = userEvent.setup();
  const onOpenOwner = vi.fn();
  const onSelectTrigger = vi.fn();

  render(
    <TriggerExplorerTree
      rootNode={buildTriggerTree(triggers)}
      allTriggers={triggers}
      activeOwner=""
      selectedSlug={null}
      onOpenOwner={onOpenOwner}
      onSelectTrigger={onSelectTrigger}
    />
  );

  expect(screen.getByRole('button', { name: 'Expand platform' })).toHaveAttribute('aria-expanded', 'false');
  expect(screen.queryByRole('button', { name: 'Select trigger platform/api' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Open owner platform/apps' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand platform' }));

  expect(screen.getByRole('button', { name: 'Open team Workspace under owner platform' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Open owner platform/apps' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Select trigger platform/api' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand Workspace' }));

  expect(screen.getByRole('button', { name: 'Select trigger platform/api' })).toBeVisible();
});

test('opens selected trigger ancestry so deep-linked details remain visible', () => {
  render(
    <TriggerExplorerTree
      rootNode={buildTriggerTree(triggers)}
      allTriggers={triggers}
      activeOwner="platform"
      selectedSlug="platform/api"
      onOpenOwner={vi.fn()}
      onSelectTrigger={vi.fn()}
    />
  );

  expect(screen.getByRole('button', { name: 'Collapse platform' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('button', { name: 'Select trigger platform/api' })).toHaveClass('active');
});
