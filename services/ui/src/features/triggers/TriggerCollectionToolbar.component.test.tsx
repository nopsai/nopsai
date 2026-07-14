import { useRef, useState } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { TriggerCollectionToolbar } from './TriggerCollectionToolbar';
import type { TriggerSourceFilter } from './model';

function ToolbarHarness({
  onCreate,
  onSourceFilterChange,
  canCreateTriggerHere = true,
}: {
  onCreate: () => void;
  onSourceFilterChange: (value: TriggerSourceFilter) => void;
  canCreateTriggerHere?: boolean;
}) {
  const [searchTerm, setSearchTerm] = useState('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [sourceFilter, setSourceFilter] = useState<TriggerSourceFilter>('all');
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  return (
    <MemoryRouter initialEntries={['/triggers']}>
      <TriggerCollectionToolbar
        searchTerm={searchTerm}
        sourceFilter={sourceFilter}
        searchOpen={searchOpen}
        searchInputRef={searchInputRef}
        canCreateTriggerHere={canCreateTriggerHere}
        onSearchTermChange={setSearchTerm}
        onSourceFilterChange={value => {
          setSourceFilter(value);
          onSourceFilterChange(value);
        }}
        onSearchOpenChange={setSearchOpen}
        onCreate={onCreate}
      />
    </MemoryRouter>
  );
}

test('renders the demo-style trigger toolbar and delegates controls', async () => {
  const user = userEvent.setup();
  const onCreate = vi.fn();
  const onSourceFilterChange = vi.fn();

  render(
    <ToolbarHarness
      onCreate={onCreate}
      onSourceFilterChange={onSourceFilterChange}
    />
  );

  expect(screen.queryByText('Event automation')).not.toBeInTheDocument();
  expect(screen.queryByText(/Event-to-pipeline rules/)).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Back' })).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Triggers' })).toHaveClass('active');
  expect(screen.getByRole('link', { name: 'External API' })).toHaveAttribute('href', '/external-triggers');

  await user.selectOptions(screen.getByLabelText('Filter triggers by source'), 'git');
  await user.click(screen.getByRole('button', { name: 'Search triggers' }));
  const search = screen.getByPlaceholderText('Search triggers');
  await waitFor(() => expect(search).toHaveFocus());
  await user.type(search, 'release');
  expect(search).toHaveValue('release');
  await user.click(screen.getByRole('button', { name: 'Clear search' }));
  expect(search).toHaveValue('');
  await user.click(screen.getByRole('button', { name: 'Create new trigger' }));

  expect(onSourceFilterChange).toHaveBeenCalledWith('git');
  expect(onCreate).toHaveBeenCalledOnce();
});

test('keeps trigger creation reachable when permission preflight is inconclusive', async () => {
  const user = userEvent.setup();
  const onCreate = vi.fn();

  render(
    <ToolbarHarness
      canCreateTriggerHere={false}
      onCreate={onCreate}
      onSourceFilterChange={vi.fn()}
    />
  );

  await user.click(screen.getByRole('button', { name: 'Create new trigger' }));
  expect(onCreate).toHaveBeenCalledOnce();
});
