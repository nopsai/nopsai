import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import SystemLogsPanel from './SystemLogsPanel';

const hookMock = vi.hoisted(() => ({
  selectSource: vi.fn(),
  setLive: vi.fn(),
  togglePaused: vi.fn(),
  clear: vi.fn(),
  refreshSources: vi.fn(async () => undefined),
}));

vi.mock('./useSystemLogs', () => ({
  useSystemLogs: () => ({
    sources: [{ id: 'dispatcher', display_name: 'Dispatcher', container_name: 'nopsai-dispatcher', available: true, state: 'running', health: 'healthy' }],
    selectedSourceID: 'dispatcher',
    entries: [
      { id: 'one', source_id: 'dispatcher', container_name: 'nopsai-dispatcher', container_instance: 'instance-1', emitted_at: '2026-06-21T12:00:00Z', observed_at: '2026-06-21T12:00:01Z', stream: 'stdout', line: 'ready' },
      { id: 'two', source_id: 'dispatcher', container_name: 'nopsai-dispatcher', container_instance: 'instance-2', emitted_at: '2026-06-21T12:00:02Z', observed_at: '2026-06-21T12:00:03Z', stream: 'stderr', line: 'request failed' },
    ],
    connectionState: 'connected',
    error: null,
    redactionWarning: 'Secret redaction is best effort.',
    live: true,
    paused: false,
    unseenCount: 0,
    cursorExpired: true,
    reconnectAttempt: 0,
    ...hookMock,
  }),
}));

test('renders status, safety warnings, restart boundaries, and filters', async () => {
  const user = userEvent.setup();
  render(<SystemLogsPanel />);

  expect(screen.getByText('Secret redaction is best effort.')).toBeVisible();
  expect(screen.getByText(/Log gap detected/)).toBeVisible();
  expect(screen.getByText('Service instance restarted')).toBeVisible();
  expect(screen.getByText('ready')).toBeVisible();
  expect(screen.getByText('request failed')).toBeVisible();

  const stdoutFilter = screen.getByRole('button', { name: 'stdout' });
  const stderrFilter = screen.getByRole('button', { name: 'stderr' });
  expect(stdoutFilter).toHaveAttribute('aria-pressed', 'true');
  expect(stderrFilter).toHaveAttribute('aria-pressed', 'true');
  expect(stderrFilter).toHaveClass('ring-1', 'bg-[var(--bg-tertiary)]');

  await user.click(stderrFilter);
  expect(stderrFilter).toHaveAttribute('aria-pressed', 'false');
  expect(stderrFilter).not.toHaveClass('ring-1');
  expect(screen.queryByText('request failed')).not.toBeInTheDocument();
  expect(screen.getByText('ready')).toBeVisible();

  const wrapLines = screen.getByRole('button', { name: 'Wrap lines' });
  expect(wrapLines).toHaveAttribute('aria-pressed', 'false');
  await user.click(wrapLines);
  expect(wrapLines).toHaveAttribute('aria-pressed', 'true');
  expect(wrapLines).toHaveClass('ring-1', 'bg-[var(--bg-tertiary)]');

  await user.type(screen.getByLabelText('Search system logs'), 'missing');
  expect(screen.getByText('No matching log lines.')).toBeVisible();
});

test('delegates live, pause, clear, and refresh controls to the hook', async () => {
  const user = userEvent.setup();
  render(<SystemLogsPanel />);
  expect(screen.getByRole('button', { name: 'Stop live' })).toHaveClass('ring-1', 'bg-[var(--bg-tertiary)]');
  expect(screen.getByRole('button', { name: 'Pause' })).not.toHaveClass('ring-1');
  await user.click(screen.getByRole('button', { name: 'Stop live' }));
  await user.click(screen.getByRole('button', { name: 'Pause' }));
  await user.click(screen.getByRole('button', { name: 'Clear' }));
  await user.click(screen.getByRole('button', { name: 'Refresh sources' }));
  expect(hookMock.setLive).toHaveBeenCalledWith(false);
  expect(hookMock.togglePaused).toHaveBeenCalled();
  expect(hookMock.clear).toHaveBeenCalled();
  expect(hookMock.refreshSources).toHaveBeenCalled();
});
