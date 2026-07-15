import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { AutomationResourceTree } from './AutomationResourceTree';
import { buildAutomationResourceTree, type AutomationResourceTreeItem } from './resourceTreeModel';

const items: AutomationResourceTreeItem[] = [
  { id: 'prod', label: 'Deploy production', path: 'platform/prod', source: 'database' },
];

test('keeps automation team branches collapsed by default', async () => {
  const user = userEvent.setup();
  render(
    <AutomationResourceTree
      title="Team tree"
      rootLabel="All teams"
      rootNode={buildAutomationResourceTree(items)}
      items={items}
      activePath=""
      selectedID={null}
      leafIconType="external-trigger"
      leafAriaLabel="Select external trigger"
      onOpenPath={vi.fn()}
      onSelectItem={vi.fn()}
    />
  );

  expect(screen.getByRole('button', { name: 'All teams (1)' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Expand platform' })).toHaveAttribute('aria-expanded', 'false');
  expect(screen.queryByRole('button', { name: 'Open team platform/prod' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Select external trigger Deploy production' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand platform' }));

  expect(screen.getByRole('button', { name: 'Open team platform/prod' })).toBeVisible();
});

test('opens selected automation resource ancestry for detail routes', () => {
  render(
    <AutomationResourceTree
      title="Team tree"
      rootLabel="All teams"
      rootNode={buildAutomationResourceTree(items)}
      items={items}
      activePath=""
      selectedID="prod"
      leafIconType="git-webhook-source"
      leafAriaLabel="Select webhook source"
      onOpenPath={vi.fn()}
      onSelectItem={vi.fn()}
    />
  );

  expect(screen.getByRole('button', { name: 'Collapse platform' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('button', { name: 'Collapse platform/prod' })).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('button', { name: 'Select webhook source Deploy production' })).toHaveClass('active');
});
