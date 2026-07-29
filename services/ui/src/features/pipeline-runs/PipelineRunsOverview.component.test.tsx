import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import type { RunListItem } from './contracts';
import { PipelineRunsOverview } from './PipelineRunsOverview';

const runs: RunListItem[] = [
  {
    run_id: 'run-running',
    pipeline_name: 'deploy-api',
    pipeline_path: 'platform/api',
    status: 'running',
    is_complete: false,
    trigger_event_id: 'event-related',
    git_repo_owner: 'acme',
    git_repo_name: 'api',
    git_ref: 'refs/heads/main',
    git_commit_sha: 'abcdef123456',
    started_at: '2026-07-12T11:58:00Z',
    final_output_status: {
      status: 'generating',
      configured: 2,
      total: 2,
      pending: 0,
      generating: 1,
      generated: 1,
      failed: 0,
      cancelled: 0,
      updated_at: '2026-07-12T11:59:00Z',
    },
  },
  {
    run_id: 'run-failed',
    pipeline_name: 'nightly-ledger',
    status: 'failure',
    is_complete: true,
    trigger_source: 'schedule',
    schedule_name: 'nightly',
    started_at: '2026-07-12T11:00:00Z',
    finished_at: '2026-07-12T11:06:00Z',
    failure_reason: 'Ledger mismatch\nWhy: totals diverged',
  },
  {
    run_id: 'run-feature',
    pipeline_name: 'deploy-feature',
    pipeline_path: 'platform/api',
    status: 'success',
    is_complete: true,
    trigger_event_id: 'event-related',
    git_repo_owner: 'acme',
    git_repo_name: 'api',
    git_ref: 'refs/heads/feature/api',
    git_commit_sha: '123456abcdef',
    started_at: '2026-07-12T10:30:00Z',
    finished_at: '2026-07-12T10:35:00Z',
    final_output_status: {
      status: 'success',
      configured: 1,
      total: 1,
      pending: 0,
      generating: 0,
      generated: 1,
      failed: 0,
      cancelled: 0,
      updated_at: '2026-07-12T10:35:00Z',
    },
  },
];

const teams = [
  { id: 1, name: 'Platform', kind: 'team' as const },
  { id: 2, name: 'platform/api', kind: 'app' as const, parent_id: 1, last_run_at: '2026-07-12T10:00:00Z' },
  { id: 3, name: 'Payments', kind: 'team' as const },
];

test('renders redesigned pipeline run overview and delegates user actions', async () => {
  const user = userEvent.setup();
  const onSelectTeam = vi.fn();
  const onOpenRun = vi.fn();
  const onSelectRun = vi.fn();

  const defaultProps = {
    teams,
    teamsLoading: false,
    teamsError: null,
    activeTeamId: null,
    activeTeamURLValue: '',
    runs,
    runsLoading: false,
    searchTerm: '',
    sourceFilter: 'all' as const,
    statusFilter: 'all' as const,
    selectedRunIds: new Set<string>(),
    onSelectTeam,
    onOpenRun,
    onSelectRun,
  };

  const { container, rerender } = render(
    <MemoryRouter>
      <PipelineRunsOverview {...defaultProps} />
    </MemoryRouter>
  );

  expect(screen.queryByText('Operations')).not.toBeInTheDocument();
  expect(screen.queryByRole('heading', { name: 'All pipeline runs' })).not.toBeInTheDocument();
  const metrics = screen.getByTestId('pipeline-runs-metrics');
  expect(within(metrics).getByText('Running now')).toBeVisible();
  expect(within(metrics).getByText('1 failed, 0 waiting approval')).toBeVisible();
  expect(screen.getByText('deploy-api')).toBeVisible();
  expect(screen.getByText('deploy-feature')).toBeVisible();
  expect(container.querySelectorAll('[data-trigger-id="event-related"]')).toHaveLength(2);
  expect(screen.getByRole('columnheader', { name: 'Repository' })).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Run ID' })).toBeVisible();
  expect(screen.getByRole('columnheader', { name: 'Outputs' })).toBeVisible();
  expect(screen.getAllByRole('columnheader').map(header => header.textContent?.trim()).slice(0, 8)).toEqual([
    'Status',
    'Pipeline run',
    'Repository',
    'Run ID',
    'Branch',
    'Started',
    'Duration',
    'Outputs',
  ]);
  const runningRow = container.querySelector('[data-trigger-id="event-related"]');
  expect(runningRow).toHaveTextContent('generating');
  expect(within(runningRow as HTMLElement).getByTitle('Output generating: 1 generated, 1 generating')).toBeVisible();
  const featureRunButton = screen.getByRole('button', { name: 'deploy-feature' });
  const featureRow = featureRunButton.closest('tr');
  expect(featureRow).toHaveTextContent('success');
  expect(featureRow).not.toHaveTextContent('generated');
  expect(screen.getByText('nightly-ledger')).toBeVisible();
  expect(screen.queryByRole('heading', { name: 'Source mix' })).not.toBeInTheDocument();
  expect(screen.queryByRole('heading', { name: 'Current scope' })).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: /view all/i })).toHaveAttribute('href', '/pipelineruns/recent');
  expect(screen.getByRole('link', { name: 'Show pipelines that need attention' })).toHaveAttribute('href', '/pipelineruns/recent?status=attention');
  expect(screen.getByRole('separator', { name: 'Resize pipeline run team tree' })).toBeVisible();
  expect(screen.queryByPlaceholderText('Find team or app')).not.toBeInTheDocument();

  expect(screen.getByRole('button', { name: 'Open team Platform' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Open team Payments' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Open application api' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand team Platform' }));
  expect(screen.getByRole('button', { name: 'Open application api' })).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Collapse team Platform' }));
  expect(screen.queryByRole('button', { name: 'Open application api' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Expand team Platform' }));
  await user.click(screen.getByRole('button', { name: 'Open team Platform' }));
  expect(onSelectTeam).toHaveBeenCalledWith(1);

  rerender(
    <MemoryRouter>
      <PipelineRunsOverview {...defaultProps} activeTeamId={1} activeTeamURLValue="platform" />
    </MemoryRouter>
  );

  expect(screen.getByRole('button', { name: 'Open team Payments' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Open application api' })).toBeVisible();
  expect(screen.getByRole('link', { name: 'Show pipelines that need attention' })).toHaveAttribute('href', '/pipelineruns/recent/team/platform?status=attention');

  await user.click(screen.getByRole('button', { name: 'Open application api' }));
  expect(onSelectTeam).toHaveBeenCalledWith(2);

  rerender(
    <MemoryRouter>
      <PipelineRunsOverview {...defaultProps} activeTeamId={2} activeTeamURLValue="platform/api" />
    </MemoryRouter>
  );

  const branchFilter = screen.getByLabelText('Filter application runs by branch');
  expect(branchFilter).toBeVisible();
  expect(screen.getByRole('option', { name: 'main - 1 run' })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: 'feature/api - 1 run' })).toBeInTheDocument();

  await user.selectOptions(branchFilter, screen.getByRole('option', { name: 'feature/api - 1 run' }));
  expect(screen.getByText('deploy-feature')).toBeVisible();
  expect(screen.queryByText('deploy-api')).not.toBeInTheDocument();

  await user.selectOptions(branchFilter, 'all');
  expect(screen.getByText('deploy-api')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Select run run-running' }));
  expect(onSelectRun).toHaveBeenCalledWith('run-running');

  await user.click(screen.getByText('deploy-api'));
  expect(onOpenRun).toHaveBeenCalledWith('run-running');

  await user.click(screen.getByText('nightly-ledger'));
  expect(onOpenRun).toHaveBeenCalledWith('run-failed');
});
