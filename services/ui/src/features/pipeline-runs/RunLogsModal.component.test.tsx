import { useState } from 'react';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { fetchRunLogs } from './api';
import { RunLogsModal } from './RunLogsModal';

vi.mock('./api', async importOriginal => {
  const actual = await importOriginal<typeof import('./api')>();
  return {
    ...actual,
    fetchRunLogs: vi.fn(),
  };
});

const fetchRunLogsMock = vi.mocked(fetchRunLogs);

beforeEach(() => {
  fetchRunLogsMock.mockReset();
  window.location.hash = '#/pipelineruns/events/run-1';
});

afterEach(() => {
  vi.useRealTimers();
});

function RunLogsHarness({ onClose }: { onClose: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Open logs</button>
      {open ? (
        <RunLogsModal
          runId="run-1"
          runName="Enterprise pipeline"
          onClose={() => {
            setOpen(false);
            onClose();
          }}
          steps={[
            { name: 'build', status: 'success' },
            { name: 'deploy', status: 'failure' },
          ]}
        />
      ) : null}
    </>
  );
}

test('loads, filters, and closes the run log dialog accessibly', async () => {
  fetchRunLogsMock.mockResolvedValueOnce([
    {
      id: 1,
      timestamp: '2026-06-08T10:00:00Z',
      line: '{"level":"info","step":"build","message":"compiled"}',
    },
    {
      id: 2,
      timestamp: '2026-06-08T10:00:01Z',
      line: '{"level":"error","step":"deploy","message":"deployment failed"}',
    },
  ]);
  const onClose = vi.fn();
  const user = userEvent.setup();

  render(<RunLogsHarness onClose={onClose} />);
  const opener = screen.getByRole('button', { name: 'Open logs' });
  await user.click(opener);

  expect(await screen.findByRole('dialog', { name: 'Run Logs for Enterprise pipeline' })).toBeVisible();
  expect(screen.getByRole('dialog', { name: 'Run Logs for Enterprise pipeline' }).closest('.fixed')).toHaveClass('z-[110]');
  expect(screen.getByRole('dialog', { name: 'Run Logs for Enterprise pipeline' }).closest('.fixed')).toHaveAttribute('data-run-graph-floating-layer');
  expect(screen.getByRole('searchbox', { name: 'Search run logs' })).toHaveFocus();
  expect(await screen.findByText(/compiled/)).toBeVisible();
  expect(screen.getByRole('log', { name: 'Logs for Enterprise pipeline' })).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'ERROR' }));
  expect(screen.queryByText(/compiled/)).not.toBeInTheDocument();
  expect(screen.getByText(/deployment failed/)).toBeVisible();

  screen.getByRole('button', { name: 'Reset filters' }).focus();
  await user.tab();
  expect(screen.getByRole('button', { name: 'Download' })).toHaveFocus();

  await user.keyboard('{Escape}');
  expect(onClose).toHaveBeenCalledOnce();
  expect(opener).toHaveFocus();
});

test('announces run log loading failures', async () => {
  fetchRunLogsMock.mockRejectedValueOnce(new Error('Log service unavailable'));

  render(<RunLogsModal runId="run-2" onClose={() => undefined} />);

  expect(await screen.findByRole('alert')).toHaveTextContent('Log service unavailable');
});

test('polls incrementally from the latest received line', async () => {
  vi.useFakeTimers();
  fetchRunLogsMock
    .mockResolvedValueOnce([
      { id: 7, timestamp: '2026-06-08T10:00:00Z', line: '{"level":"info","message":"first batch"}' },
    ])
    .mockResolvedValueOnce([
      { id: 9, timestamp: '2026-06-08T10:00:05Z', line: '{"level":"info","message":"second batch"}' },
    ]);

  render(<RunLogsModal runId="run-poll" onClose={() => undefined} />);
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });

  expect(screen.getByText(/first batch/)).toBeVisible();
  expect(fetchRunLogsMock).toHaveBeenNthCalledWith(1, 'run-poll', 0);

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });

  expect(screen.getByText(/second batch/)).toBeVisible();
  expect(fetchRunLogsMock).toHaveBeenNthCalledWith(2, 'run-poll', 7);
});

test('opens task logs with both step and task filters applied', async () => {
  fetchRunLogsMock.mockResolvedValueOnce([
    {
      id: 1,
      timestamp: '2026-06-08T10:00:00Z',
      line: '{"level":"info","step":"build","task":"compile","message":"compiled"}',
    },
    {
      id: 2,
      timestamp: '2026-06-08T10:00:01Z',
      line: '{"level":"info","step":"build","task":"test","message":"tested"}',
    },
    {
      id: 3,
      timestamp: '2026-06-08T10:00:02Z',
      line: '{"level":"info","step":"deploy","task":"compile","message":"deployed"}',
    },
  ]);

  render(
    <RunLogsModal
      runId="run-1"
      runName="Enterprise pipeline"
      onClose={() => undefined}
      steps={[{ name: 'build', status: 'success' }, { name: 'deploy', status: 'success' }]}
      initialStep="build"
      initialTask="compile"
    />
  );

  expect(await screen.findByText(/compiled/)).toBeVisible();
  expect(screen.getByRole('button', { name: 'Task: compile' })).toBeVisible();
  expect(screen.queryByText(/tested/)).not.toBeInTheDocument();
  expect(screen.queryByText(/deployed/)).not.toBeInTheDocument();
});

test('loads included pipeline logs from the parent run and labels child lines', async () => {
  fetchRunLogsMock.mockResolvedValueOnce([
    {
      id: 4,
      timestamp: '2026-06-08T10:00:03Z',
      run_id: 'child-run',
      pipeline_name: 'child-deploy',
      parent_run_id: 'run-1',
      parent_step_name: 'included',
      line: '{"level":"info","step":"child-build","message":"child log visible"}',
    },
  ]);

  render(
    <RunLogsModal
      runId="run-1"
      runName="Enterprise pipeline"
      includeChildren
      onClose={() => undefined}
      steps={[{ name: 'included', status: 'success' }]}
      initialStep="included"
    />
  );

  expect(await screen.findByText(/child log visible/)).toBeVisible();
  expect(screen.getByText('child-deploy')).toBeVisible();
  expect(screen.getByText(/Includes included pipeline logs/)).toBeVisible();
  expect(fetchRunLogsMock).toHaveBeenCalledWith('run-1', 0, { includeChildren: true });
});

test('keeps following new lines even when the scroll position was previously above the bottom', async () => {
  vi.useFakeTimers();
  fetchRunLogsMock
    .mockResolvedValueOnce([
      { id: 1, timestamp: '2026-06-08T10:00:00Z', line: '{"level":"info","message":"first batch"}' },
    ])
    .mockResolvedValueOnce([
      { id: 2, timestamp: '2026-06-08T10:00:05Z', line: '{"level":"info","message":"second batch"}' },
    ]);

  render(<RunLogsModal runId="run-follow" onClose={() => undefined} />);
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  const log = screen.getByRole('log', { name: 'Logs for run-follow' });
  Object.defineProperty(log, 'scrollHeight', { configurable: true, value: 500 });
  Object.defineProperty(log, 'clientHeight', { configurable: true, value: 100 });
  log.scrollTop = 0;

  await act(async () => {
    await vi.advanceTimersByTimeAsync(1000);
  });

  expect(screen.getByText(/second batch/)).toBeVisible();
  expect(log.scrollTop).toBe(500);
});
