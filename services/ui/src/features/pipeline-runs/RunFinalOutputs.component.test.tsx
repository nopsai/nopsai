import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { apiClient } from '../../lib/api';
import type { PipelineRunFinalOutput } from './contracts';
import { RunFinalOutputs } from './RunFinalOutputs';

const outputs: PipelineRunFinalOutput[] = [
  {
    id: 'output-1',
    name: 'Executive Summary',
    type: 'markdown',
    status: 'success',
    content: '# Summary\n\nEverything passed.',
    llm_profile: 'report-writer',
    created_at: '2026-07-18T10:00:00Z',
    updated_at: '2026-07-18T10:02:00Z',
    generation_duration: '2m0s',
  },
  {
    id: 'output-2',
    name: 'Comparison Report',
    type: 'pdf',
    status: 'generating',
  },
  {
    id: 'output-3',
    name: 'Data Table',
    type: 'excel',
    status: 'failure',
    error: 'LLM profile missing',
  },
];

test('renders final outputs with preview and copy actions', async () => {
  const user = userEvent.setup();
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  });

  render(<RunFinalOutputs runID="run-1" outputs={outputs} />);

  expect(screen.getByText('Final Outputs')).toBeVisible();
  expect(screen.getByText('3 deliverables')).toBeVisible();
  expect(screen.getByText('Executive Summary')).toBeVisible();
  expect(screen.getByText('Generating')).toBeVisible();
  expect(screen.getByText('LLM profile missing')).toBeVisible();
  expect(screen.getByText(/2m0s duration/)).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Actions for Executive Summary' }));
  await user.click(screen.getByRole('menuitem', { name: 'Preview' }));
  expect(screen.getByText(/Everything passed/)).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Actions for Executive Summary' }));
  await user.click(screen.getByRole('menuitem', { name: 'Copy' }));
  await waitFor(() => expect(writeText).toHaveBeenCalledWith('# Summary\n\nEverything passed.'));
  expect(screen.getByText('Executive Summary copied')).toBeVisible();
});

test('uses per-output generation start times for final output durations', () => {
  render(
    <RunFinalOutputs
      runID="run-1"
      outputs={[
        {
          id: 'output-1',
          name: 'First output',
          type: 'markdown',
          status: 'success',
          created_at: '2026-07-18T10:00:00Z',
          generation_started_at: '2026-07-18T10:00:05Z',
          updated_at: '2026-07-18T10:00:15Z',
        },
        {
          id: 'output-2',
          name: 'Later output',
          type: 'markdown',
          status: 'success',
          created_at: '2026-07-18T10:00:00Z',
          generation_started_at: '2026-07-18T10:01:00Z',
          updated_at: '2026-07-18T10:01:20Z',
        },
        {
          id: 'output-3',
          name: 'Queued output',
          type: 'markdown',
          status: 'pending',
          created_at: '2026-07-18T10:00:00Z',
          updated_at: '2026-07-18T10:05:00Z',
        },
      ]}
    />
  );

  expect(screen.getByText(/10s duration/)).toBeVisible();
  expect(screen.getByText(/20s duration/)).toBeVisible();
  expect(screen.queryByText(/1m 20s duration/)).not.toBeInTheDocument();
  expect(screen.queryByText(/5m duration/)).not.toBeInTheDocument();
});

test('downloads final outputs through the authenticated API client', async () => {
  const user = userEvent.setup();
  const originalFetch = apiClient.fetch;
  const originalCreateObjectURL = URL.createObjectURL;
  const originalRevokeObjectURL = URL.revokeObjectURL;
  const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  const fetchSpy = vi.fn().mockResolvedValue(
    new Response('download payload', {
      status: 200,
      headers: { 'content-disposition': 'attachment; filename="executive-summary.md"' },
    })
  );
  apiClient.fetch = fetchSpy;
  URL.createObjectURL = vi.fn(() => 'blob:nopsai-output');
  URL.revokeObjectURL = vi.fn();

  try {
    render(<RunFinalOutputs runID="run-1" outputs={[outputs[0]]} />);
    await user.click(screen.getByRole('button', { name: 'Actions for Executive Summary' }));
    await user.click(screen.getByRole('menuitem', { name: 'Download' }));

    await waitFor(() => {
      expect(fetchSpy).toHaveBeenCalledWith('/v1/runs/run-1/outputs/output-1/download', { cache: 'no-store' });
      expect(clickSpy).toHaveBeenCalled();
    });
  } finally {
    apiClient.fetch = originalFetch;
    URL.createObjectURL = originalCreateObjectURL;
    URL.revokeObjectURL = originalRevokeObjectURL;
    clickSpy.mockRestore();
  }
});

test('cancels pending final output generation', async () => {
  const user = userEvent.setup();
  const onCancelOutput = vi.fn().mockResolvedValue(undefined);

  render(<RunFinalOutputs runID="run-1" outputs={[outputs[1]]} onCancelOutput={onCancelOutput} />);
  await user.click(screen.getByRole('button', { name: 'Actions for Comparison Report' }));
  await user.click(screen.getByRole('menuitem', { name: 'Cancel' }));

  await waitFor(() => expect(onCancelOutput).toHaveBeenCalledWith('output-2'));
  expect(screen.getByText('Comparison Report cancellation requested')).toBeVisible();
});

test('shows cancelled final outputs as terminal', async () => {
  const user = userEvent.setup();
  render(
    <RunFinalOutputs
      runID="run-1"
      outputs={[{ ...outputs[1], status: 'cancelled', error: 'cancelled by user' }]}
      onCancelOutput={vi.fn()}
    />
  );

  expect(screen.getByText('Cancelled')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Actions for Comparison Report' }));
  expect(screen.getByRole('menuitem', { name: 'Cancel' })).toBeDisabled();
});
