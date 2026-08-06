import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { RunDetailWorkspaceTabs } from './RunDetailWorkspaceTabs';

test('switches between the run graph and final outputs tabs', async () => {
  const user = userEvent.setup();
  const { container } = render(
    <MemoryRouter>
      <RunDetailWorkspaceTabs
        runID="run-1"
        steps={[{ name: 'build', status: 'success', depends_on: [], tasks: [] }]}
        selectedStep={null}
        onSelectStep={vi.fn()}
        onOpenStepLogs={vi.fn()}
        onOpenTaskLogs={vi.fn()}
        onOpenStepDetail={vi.fn()}
        childRuns={[]}
        outputs={[
          {
            id: 'output-1',
            name: 'Summary',
            type: 'markdown',
            status: 'success',
            content: '# Summary',
          },
        ]}
        onCancelOutput={vi.fn()}
        onRetryOutput={vi.fn()}
      />
    </MemoryRouter>
  );

  expect(screen.getByRole('tab', { name: 'Graph' })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByText('Execution Graph')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Expand graph' }));
  expect(screen.getByRole('dialog', { name: 'Pipeline graph' })).toBeVisible();
  expect(screen.getByRole('region', { name: 'Expanded pipeline graph' })).toBeVisible();
  expect(screen.getByRole('region', { name: 'Expanded pipeline graph' })).toHaveClass('run-graph-redesign--expanded-frame');
  const expandedGraphSurface = container.querySelector('.run-detail-graph-modal .run-graph-workspace') as HTMLElement | null;
  expect(expandedGraphSurface).not.toBeNull();
  fireEvent.wheel(expandedGraphSurface!, { deltaY: -200 });
  await waitFor(() => {
    expect(expandedGraphScale(container)).toBeGreaterThan(1);
  });
  fireEvent.keyDown(window, { key: 'Escape' });
  expect(screen.queryByRole('dialog', { name: 'Pipeline graph' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('tab', { name: /Outputs.*1/ }));

  expect(screen.getByRole('tab', { name: /Outputs.*1/ })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByText('Final Outputs')).toBeVisible();
  expect(screen.getByText('Summary')).toBeVisible();
});

function expandedGraphScale(container: HTMLElement) {
  const transform = container.querySelector('.run-detail-graph-modal .run-graph-overview > g')?.getAttribute('transform') || '';
  return Number(transform.match(/scale\(([^)]+)\)/)?.[1] || 0);
}
