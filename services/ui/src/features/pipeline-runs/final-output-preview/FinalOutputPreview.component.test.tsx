import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, test, vi } from 'vitest';
import { apiClient } from '../../../lib/api';
import type { PipelineRunFinalOutput } from '../contracts';
import { FinalOutputPreview } from './FinalOutputPreview';

const originalFetch = apiClient.fetch;
const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;

afterEach(() => {
  apiClient.fetch = originalFetch;
  URL.createObjectURL = originalCreateObjectURL;
  URL.revokeObjectURL = originalRevokeObjectURL;
});

test('renders Markdown with headings and tables', () => {
  render(<FinalOutputPreview runID="run-1" output={output('markdown', '# Summary\n\n| Name | Value |\n| --- | --- |\n| API | 42 |')} />);
  expect(screen.getByRole('heading', { name: 'Summary' })).toBeVisible();
  expect(screen.getByRole('table')).toHaveTextContent('API');
});

test('renders DocumentSpec sections, metadata, callouts, and real tables', () => {
  const content = JSON.stringify({
    version: '1',
    title: 'Release report',
    subtitle: 'Production',
    metadata: [{ label: 'Run', value: 'run-1' }],
    sections: [{
      title: 'Summary',
      blocks: [
        { type: 'paragraph', text: 'Everything passed.' },
        { type: 'bullet_list', items: ['Build complete'] },
        { type: 'numbered_list', items: ['Deploy'] },
        { type: 'table', table: { columns: ['Service', 'Status'], rows: [['API', 'Healthy']] } },
        { type: 'callout', tone: 'success', title: 'Result', text: 'Ready' },
      ],
    }],
  });
  render(<FinalOutputPreview runID="run-1" output={output('html', content)} />);
  expect(screen.getByRole('heading', { name: 'Release report' })).toBeVisible();
  expect(screen.getByText('run-1')).toBeVisible();
  expect(screen.getByRole('table')).toHaveTextContent('Healthy');
  expect(screen.getByText('Ready')).toBeVisible();
});

test('renders typed spreadsheet sheets and switches tabs', async () => {
  const user = userEvent.setup();
  const content = JSON.stringify({
    version: '1',
    title: 'Operations',
    sheets: [
      { name: 'Summary', columns: [{ key: 'active', header: 'Active' }], rows: [{ active: true }] },
      { name: 'Costs', columns: [{ key: 'amount', header: 'Amount' }], rows: [{ amount: 42.5 }] },
    ],
  });
  render(<FinalOutputPreview runID="run-1" output={output('excel', content)} />);
  expect(screen.getByRole('cell', { name: 'Yes' })).toBeVisible();
  await user.click(screen.getByRole('tab', { name: 'Costs' }));
  expect(screen.getByRole('cell', { name: '42.5' })).toBeVisible();
});

test('loads the rendered PDF into an inline viewer', async () => {
  const fetchSpy = vi.fn().mockResolvedValue(new Response('%PDF-1.7', { status: 200, headers: { 'content-type': 'application/pdf' } }));
  apiClient.fetch = fetchSpy;
  URL.createObjectURL = vi.fn(() => 'blob:report');
  URL.revokeObjectURL = vi.fn();
  render(<FinalOutputPreview runID="run/1" output={output('pdf', '{}')} />);
  expect(screen.getByText('Rendering PDF preview')).toBeVisible();
  await waitFor(() => expect(screen.getByTitle('Output PDF preview')).toHaveAttribute('src', 'blob:report'));
  expect(fetchSpy).toHaveBeenCalledWith('/v1/runs/run%2F1/outputs/output-1/download', expect.objectContaining({ cache: 'no-store' }));
});

test('shows structured preview errors and formats JSON', () => {
  const { rerender } = render(<FinalOutputPreview runID="run-1" output={output('excel', 'legacy,csv')} />);
  expect(screen.getByRole('alert')).toHaveTextContent('legacy spreadsheet');
  rerender(<FinalOutputPreview runID="run-1" output={output('json', '{"ok":true}')} />);
  expect(screen.getByText(/"ok": true/)).toBeVisible();
});

function output(type: string, content: string): PipelineRunFinalOutput {
  return { id: 'output-1', name: 'Output', type, status: 'success', content };
}
