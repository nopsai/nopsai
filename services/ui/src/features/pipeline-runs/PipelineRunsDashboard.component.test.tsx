import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { RunCard } from './PipelineRunsDashboard';

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
      }}
      selected={false}
      onOpen={onOpen}
      onSelect={onSelect}
    />
  );

  expect(screen.getByText('release')).toBeInTheDocument();
  expect(screen.getByText('Deployment failed')).toBeInTheDocument();
  const card = screen.getByText('release').closest('[role="button"]');
  expect(card).not.toBeNull();
  await user.click(card!);
  expect(onOpen).toHaveBeenCalledOnce();

  const selectButton = screen.getByRole('button', { name: /select run/i });
  await user.click(selectButton);
  expect(onSelect).toHaveBeenCalledOnce();
});
