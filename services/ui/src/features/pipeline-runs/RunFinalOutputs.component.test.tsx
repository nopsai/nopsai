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

  await user.click(screen.getAllByRole('button', { name: 'Preview' })[0]);
  expect(screen.getByText(/Everything passed/)).toBeVisible();

  await user.click(screen.getAllByRole('button', { name: 'Copy' })[0]);
  await waitFor(() => expect(writeText).toHaveBeenCalledWith('# Summary\n\nEverything passed.'));
  expect(screen.getByText('Executive Summary copied')).toBeVisible();
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
    await user.click(screen.getByRole('button', { name: 'Download' }));

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
  await user.click(screen.getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(onCancelOutput).toHaveBeenCalledWith('output-2'));
  expect(screen.getByText('Comparison Report cancellation requested')).toBeVisible();
});

test('shows cancelled final outputs as terminal', () => {
  render(
    <RunFinalOutputs
      runID="run-1"
      outputs={[{ ...outputs[1], status: 'cancelled', error: 'cancelled by user' }]}
      onCancelOutput={vi.fn()}
    />
  );

  expect(screen.getByText('Cancelled')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();
});
