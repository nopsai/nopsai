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
      activeFolder="platform"
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
  render(<ToolbarHarness onBack={onBack} onCreate={onCreate} />);

  await user.click(screen.getByRole('button', { name: 'Back' }));
  await user.click(screen.getByRole('button', { name: 'Create new pipeline' }));
  await user.click(screen.getByRole('button', { name: 'Search pipelines' }));
  const search = screen.getByPlaceholderText('Search pipelines');
  await waitFor(() => expect(search).toHaveFocus());
  await user.type(search, 'release');
  expect(search).toHaveValue('release');
  await user.click(screen.getByRole('button', { name: 'Clear search' }));
  expect(search).toHaveValue('');

  expect(onBack).toHaveBeenCalledOnce();
  expect(onCreate).toHaveBeenCalledOnce();
});

test('hides creation and disables folder navigation when access or context is missing', () => {
  const onBack = vi.fn();
  const onCreate = vi.fn();
  render(
    <ResourceCollectionToolbar
      resourceLabel="step"
      activeFolder=""
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
