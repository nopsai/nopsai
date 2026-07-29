import { useRef, useState } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerCollectionToolbar } from './TriggerCollectionToolbar';

function ToolbarHarness({
  onCreate,
  canCreateTriggerHere = true,
}: {
  onCreate: () => void;
  canCreateTriggerHere?: boolean;
}) {
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  return (
    <MemoryRouter initialEntries={['/triggers']}>
      <TriggerCollectionToolbar
        searchTerm={searchTerm}
        searchOpen={searchOpen}
        searchInputRef={searchInputRef}
        canCreateTriggerHere={canCreateTriggerHere}
        summary={<div aria-label="Trigger summary">4 boxes</div>}
        onSearchTermChange={setSearchTerm}
        onSearchOpenChange={setSearchOpen}
        onCreate={onCreate}
      />
    </MemoryRouter>
  );
}

test('renders the demo-style trigger toolbar and delegates controls', async () => {
  const user = userEvent.setup();
  const onCreate = vi.fn();

  render(
    <ToolbarHarness
      onCreate={onCreate}
    />
  );

  expect(screen.queryByText('Event automation')).not.toBeInTheDocument();
  expect(screen.queryByText(/Event-to-pipeline rules/)).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Triggers' })).toHaveClass('active');
  expect(screen.getByRole('link', { name: 'External API' })).toHaveAttribute('href', '/external-triggers');
  expect(screen.getByLabelText('Trigger summary')).toBeVisible();
  expect(screen.queryByLabelText('Filter triggers by source')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Search triggers' }));
  const search = screen.getByPlaceholderText('Search triggers');
  await waitFor(() => expect(search).toHaveFocus());
  await user.type(search, 'release');
  expect(search).toHaveValue('release');
  await user.click(screen.getByRole('button', { name: 'Clear search' }));
  expect(search).toHaveValue('');
  await user.click(screen.getByRole('button', { name: 'Create new trigger' }));

  expect(onCreate).toHaveBeenCalledOnce();
});

test('keeps trigger creation reachable when permission preflight is inconclusive', async () => {
  const user = userEvent.setup();
  const onCreate = vi.fn();

  render(
    <ToolbarHarness
      canCreateTriggerHere={false}
      onCreate={onCreate}
    />
  );

  await user.click(screen.getByRole('button', { name: 'Create new trigger' }));
  expect(onCreate).toHaveBeenCalledOnce();
});
