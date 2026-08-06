import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import { RunDetailView } from './RunDetailPanel';

test('warns when a successful run contains ignored failed work', () => {
  render(
    <MemoryRouter>
      <RunDetailView
        detail={{
          run_info: {
            run_id: 'run-ignored',
            pipeline_name: 'Enterprise pipeline',
            status: 'success',
            is_complete: true,
          },
          steps: [
            {
              name: 'build',
              status: 'success',
              depends_on: [],
              tasks: [
                {
                  task_id: 'task-1',
                  step_name: 'build',
                  task_name: 'lint',
                  status: 'failure (ignored)',
                  task_index: 0,
                },
              ],
            },
          ],
          child_runs: [],
          final_outputs: [],
        }}
        loading={false}
        error={null}
        onClose={vi.fn()}
        onCancel={vi.fn()}
        onCancelOutput={vi.fn()}
        onRetryOutput={vi.fn()}
        onRerun={vi.fn()}
        onDelete={vi.fn()}
        selectedStep={null}
        onSelectStep={vi.fn()}
        onOpenLogs={vi.fn()}
        onOpenStepLogs={vi.fn()}
        onOpenTaskLogs={vi.fn()}
        onOpenStepDetail={vi.fn()}
        onOpenRun={vi.fn()}
        onShowDefinition={vi.fn()}
        onApprovalDecision={vi.fn()}
        approvalDecisionPending={null}
        comparisonRuns={[]}
      />
    </MemoryRouter>
  );

  expect(screen.getByRole('status', { name: 'Ignored failures detected' })).toBeVisible();
  expect(screen.getByText('Task build / lint')).toBeVisible();
  expect(screen.getByText(/run continued/i)).toBeVisible();
});
