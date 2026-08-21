import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { RunCard, RunCollection } from './PipelineRunCards';

test('renders run summary and delegates open and selection actions', async () => {
  const onOpen = vi.fn();
  const onSelect = vi.fn();
  const user = userEvent.setup();
  render(
    <RunCard
      run={{
        run_id: 'run-123456789',
        pipeline_name: 'release',
        pipeline_path: 'platform',
        status: 'failed',
        is_complete: true,
        git_repo_owner: 'acme',
        git_repo_name: 'api',
        git_ref: 'refs/heads/main',
        failure_reason: 'Deployment failed\nWhy: rollout timed out',
        ai_usage: { spend_usd: 4.2 },
        final_output_status: {
          status: 'generating',
          configured: 2,
          total: 2,
          pending: 0,
          generating: 1,
          generated: 1,
          failed: 0,
          cancelled: 0,
        },
      }}
      selected={false}
      onOpen={onOpen}
      onSelect={onSelect}
    />
  );

  expect(screen.getByText('release')).toBeInTheDocument();
  expect(screen.getByText('$4.20')).toBeInTheDocument();
  expect(screen.getByText('Output generating: 1 generated, 1 generating')).toBeInTheDocument();
  expect(screen.getByText('Deployment failed')).toBeInTheDocument();
  const card = screen.getByText('release').closest('[role="button"]');
  expect(card).not.toBeNull();
  await user.click(card!);
  expect(onOpen).toHaveBeenCalledOnce();

  const selectButton = screen.getByRole('button', { name: /select run/i });
  await user.click(selectButton);
  expect(onSelect).toHaveBeenCalledOnce();
});

test('renders all-runs list entries as compact one-line summaries', () => {
  render(
    <RunCollection
      viewMode="list"
      runs={[
        {
          run_id: 'run-123456789',
          pipeline_name: 'release',
          pipeline_path: 'platform',
          status: 'failed',
          is_complete: true,
          trigger_event_id: 'event-123456789',
          git_repo_owner: 'acme',
          git_repo_name: 'api',
          git_ref: 'refs/heads/main',
          git_commit_sha: 'abcdef123456',
          failure_reason: 'Deployment failed\nWhy: rollout timed out',
          ai_usage: { spend_usd: 4.2 },
          final_output_status: {
            status: 'success',
            configured: 1,
            total: 1,
            pending: 0,
            generating: 0,
            generated: 1,
            failed: 0,
            cancelled: 0,
          },
        },
      ]}
      selectedRunIds={new Set()}
      onOpenRun={vi.fn()}
      onSelectRun={vi.fn()}
    />
  );

  const row = screen.getByText('release').closest('.run-card--list');
  expect(row).not.toBeNull();
  expect(row?.getAttribute('title')).toContain('Deployment failed');
  expect(row?.getAttribute('title')).toContain('Output generated');
  expect(screen.getByText('generated')).toBeInTheDocument();
  expect(row?.querySelector('.run-list-chips')).toBeNull();
  expect(screen.queryByText('Deployment failed')).not.toBeInTheDocument();
});
