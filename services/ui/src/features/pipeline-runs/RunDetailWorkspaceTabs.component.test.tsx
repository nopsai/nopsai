import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { RunDetailWorkspaceTabs } from './RunDetailWorkspaceTabs';

test('switches between the run graph and final outputs tabs', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <RunDetailWorkspaceTabs
        runID="run-1"
        steps={[]}
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
      />
    </MemoryRouter>
  );

  expect(screen.getByRole('tab', { name: 'Graph' })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByText('Execution Graph')).toBeVisible();

  await user.click(screen.getByRole('tab', { name: /Outputs.*1/ }));

  expect(screen.getByRole('tab', { name: /Outputs.*1/ })).toHaveAttribute('aria-selected', 'true');
  expect(screen.getByText('Final Outputs')).toBeVisible();
  expect(screen.getByText('Summary')).toBeVisible();
});
