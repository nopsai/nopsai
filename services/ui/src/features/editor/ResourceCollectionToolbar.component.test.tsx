import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ResourceCollectionToolbar } from './ResourceCollectionToolbar';

function ToolbarHarness({ onBack, onCreate }: { onBack: () => void; onCreate: () => void }) {
  const [searchTerm, setSearchTerm] = useState('');
  return (
    <ResourceCollectionToolbar
      resourceLabel="pipeline"
      activeTeam="platform"
      searchTerm={searchTerm}
      canCreate
      onBack={onBack}
      onSearchTermChange={setSearchTerm}
      onCreate={onCreate}
    />
  );
}

test('delegates navigation and creation while owning collection search behavior', async () => {
  const user = userEvent.setup();
  const onBack = vi.fn();
  const onCreate = vi.fn();
  const { container } = render(<ToolbarHarness onBack={onBack} onCreate={onCreate} />);

  const toolbarRow = container.querySelector('.resource-collection-toolbar-row');
  expect(toolbarRow).not.toBeNull();
  expect(toolbarRow).not.toHaveClass('justify-between');
  expect(toolbarRow).not.toHaveClass('border-b');
  const leading = container.querySelector('.resource-collection-toolbar-leading');
  const actions = container.querySelector('.resource-collection-toolbar-actions');
  expect(leading).not.toBeNull();
  expect(actions).not.toBeNull();
  expect(leading).toContainElement(screen.getByRole('button', { name: 'Back' }));
  expect(actions).toContainElement(screen.getByRole('searchbox', { name: 'Search pipelines' }));
  expect(actions).toContainElement(screen.getByRole('button', { name: 'Create new pipeline' }));
  expect(container.querySelector('.resource-collection-toolbar')).not.toBeNull();
  expect(toolbarRow).toHaveClass('pipeline-runs-filterbar');

  await user.click(screen.getByRole('button', { name: 'Back' }));
  await user.click(screen.getByRole('button', { name: 'Create new pipeline' }));
  await user.click(screen.getByRole('button', { name: 'Search pipelines' }));
  const search = screen.getByRole('searchbox', { name: 'Search pipelines' });
  await waitFor(() => expect(search.closest('.resource-collection-search-shell')).toHaveClass('open'));
  await user.type(search, 'release');
  expect(search).toHaveValue('release');
  await user.click(screen.getByRole('button', { name: 'Clear search' }));
  expect(search).toHaveValue('');

  expect(onBack).toHaveBeenCalledOnce();
  expect(onCreate).toHaveBeenCalledOnce();
});

test('hides creation and disables team navigation when access or context is missing', () => {
  const onBack = vi.fn();
  const onCreate = vi.fn();
  render(
    <ResourceCollectionToolbar
      resourceLabel="step"
      activeTeam=""
      searchTerm=""
      canCreate={false}
      onBack={onBack}
      onSearchTermChange={() => undefined}
      onCreate={onCreate}
    />
  );

  expect(screen.getByRole('button', { name: 'Back' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Create new step' })).not.toBeInTheDocument();
});

test('supports resource page actions, filters, summaries, and visible read-only creation', async () => {
  const user = userEvent.setup();
  const onRefresh = vi.fn();
  const { container } = render(
    <ResourceCollectionToolbar
      resourceLabel="schedule"
      searchTerm=""
      canCreate={false}
      createLabel="New schedule"
      createDisabledReason="Read-only schedules"
      showCreateWhenDisabled
      summary={<span>3 total</span>}
      filters={<label><input type="checkbox" /> Show disabled</label>}
      onRefresh={onRefresh}
      onSearchTermChange={() => undefined}
      onCreate={() => undefined}
    />
  );

  expect(screen.getByText('3 total')).toBeVisible();
  expect(screen.getByLabelText('Show disabled')).toBeVisible();
  const toolbarRow = container.querySelector('.resource-collection-toolbar-row');
  expect(toolbarRow).not.toHaveClass('justify-between');
  expect(toolbarRow).not.toHaveClass('border-b');
  const leading = container.querySelector('.resource-collection-toolbar-leading');
  const actions = container.querySelector('.resource-collection-toolbar-actions');
  expect(leading).not.toBeNull();
  expect(actions).not.toBeNull();
  expect(leading).toContainElement(screen.getByLabelText('Show disabled'));
  expect(actions).toContainElement(screen.getByRole('button', { name: 'Refresh schedules' }));
  expect(actions).toContainElement(screen.getByRole('button', { name: 'New schedule' }));
  expect(screen.getByRole('button', { name: 'New schedule' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'New schedule' })).toHaveAttribute('title', 'Read-only schedules');
  await user.click(screen.getByRole('button', { name: 'Refresh schedules' }));
  expect(onRefresh).toHaveBeenCalledOnce();
});
