import { fireEvent, render, screen } from '@testing-library/react';
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

test('submits approval comments and requires rejection comments', () => {
  const onApprovalDecision = vi.fn();
  render(
    <MemoryRouter>
      <RunDetailView
        detail={{
          run_info: {
            run_id: 'run-approval',
            pipeline_name: 'Enterprise pipeline',
            status: 'waiting_approval',
            is_complete: false,
          },
          steps: [],
          child_runs: [],
          final_outputs: [],
          approvals: [
            {
              id: 'approval-1',
              run_id: 'run-approval',
              step_name: 'production-gate',
              task_name: 'production-gate',
              approval_type: 'production-deploy',
              assigned_teams: ['platform/prod'],
              allow_self_approval: false,
              status: 'pending',
              requested_at: '2026-07-15T10:00:00Z',
            },
          ],
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
        onApprovalDecision={onApprovalDecision}
        approvalDecisionPending={null}
        comparisonRuns={[]}
      />
    </MemoryRouter>
  );

  const comment = screen.getByLabelText('Approval comment for production-gate');
  const approve = screen.getByRole('button', { name: 'Approve' });
  const reject = screen.getByRole('button', { name: 'Reject' });

  expect(reject).toBeDisabled();
  fireEvent.click(approve);
  expect(onApprovalDecision).toHaveBeenCalledWith(expect.objectContaining({ id: 'approval-1' }), 'approve', '');

  fireEvent.change(comment, { target: { value: 'Deployment window closed' } });
  expect(reject).not.toBeDisabled();
  fireEvent.click(reject);
  expect(onApprovalDecision).toHaveBeenLastCalledWith(
    expect.objectContaining({ id: 'approval-1' }),
    'reject',
    'Deployment window closed'
  );
});
