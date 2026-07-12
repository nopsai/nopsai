import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import type { RunListItem } from '../features/pipeline-runs/contracts';

const api = vi.hoisted(() => ({
  fetchTeams: vi.fn(),
  requestPipelineRunsJson: vi.fn(),
}));

vi.mock('../features/teams/api', () => ({
  fetchTeams: api.fetchTeams,
}));

vi.mock('../features/pipeline-runs/api', () => ({
  requestPipelineRunsJson: api.requestPipelineRunsJson,
}));

import PipelineRunsPage from './PipelineRuns';

const runs: RunListItem[] = [
  {
    run_id: 'run-failed',
    pipeline_name: 'deploy-api',
    pipeline_path: 'platform/api',
    status: 'failure',
    is_complete: true,
    trigger_event_id: 'event-abcdef123456',
    git_repo_owner: 'acme',
    git_repo_name: 'api',
    git_ref: 'refs/heads/main',
    git_commit_sha: 'abcdef123456',
    started_at: '2026-07-12T11:00:00Z',
    finished_at: '2026-07-12T11:06:00Z',
  },
];

beforeEach(() => {
  localStorage.clear();
  api.fetchTeams.mockResolvedValue([]);
  api.requestPipelineRunsJson.mockImplementation((path: string) => {
    if (path.startsWith('/v1/runs?offset=')) return Promise.resolve(runs);
    if (path === '/v1/runs?teamId=root') return Promise.resolve({});
    return Promise.resolve({});
  });
});

test('opens the events tab with event groups collapsed by default', async () => {
  const user = userEvent.setup();
  renderPipelineRunsPage('/pipelineruns/events');

  const eventTitle = await screen.findByText('Event: event-ab');
  expect(document.querySelector('[data-page="pipelineruns"]')).toHaveClass('h-full', 'min-h-0', 'overflow-hidden');
  expect(document.getElementById('main-content-runs')).toHaveClass('pipeline-runs-main-scroll');
  expect(screen.queryByText('deploy-api')).not.toBeInTheDocument();

  await user.click(eventTitle.closest('button')!);
  expect(screen.getByText('deploy-api')).toBeVisible();
});

test('keeps the all-runs view toggle aligned with the status filter controls', async () => {
  renderPipelineRunsPage('/pipelineruns/recent');

  await waitFor(() => expect(api.requestPipelineRunsJson).toHaveBeenCalled());
  const statusFilter = screen.getByLabelText('Filter by run status');
  expect(screen.getByRole('option', { name: 'Needs attention' })).toBeInTheDocument();
  expect(screen.getByRole('group', { name: 'Pipeline run layout' }).closest('.pipeline-runs-filterbar')).toBe(
    statusFilter.closest('.pipeline-runs-filterbar')
  );
});

function renderPipelineRunsPage(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <Routes>
        <Route path="/pipelineruns/:tab" element={<PipelineRunsPage />} />
        <Route path="/pipelineruns/:tab/team/*" element={<PipelineRunsPage />} />
      </Routes>
    </MemoryRouter>
  );
}
