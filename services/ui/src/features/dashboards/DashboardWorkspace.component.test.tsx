import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';

import { DashboardWorkspace } from './DashboardWorkspace';
import type {
  DashboardEvent,
  DashboardRefresh,
  DashboardRefreshSchedule,
  DashboardSummary,
  DashboardView,
} from './model';

vi.mock('../../components/ResourceAccessCard', () => ({
  default: ({ label }: { label: string }) => <button type="button">Access {label}</button>,
}));

const dashboard: DashboardSummary = {
  id: 'dashboard-1',
  team_path: 'platform',
  ref: 'platform/ops',
  slug: 'ops',
  title: 'Ops',
  description: 'Operations health.',
  visibility: 'team',
  current_publication_count: 1,
  last_published_at: '2026-07-15T10:00:00Z',
};

const view: DashboardView = {
  dashboard,
  sections: [
    {
      id: 'section-1',
      section_key: 'overview',
      title: 'Overview',
      display_order: 0,
    },
  ],
  publications: [
    {
      id: 'publication-1',
      section_key: 'overview',
      entry_key: 'service-health',
      mode: 'replace',
      content: {
        title: 'Service Health',
        blocks: [{ type: 'status', label: 'API', status: 'ok', text: 'Healthy' }],
      },
      revision: 2,
      run_id: 'run-123456789',
      pipeline_id: 'platform/service-health',
      output_name: 'Service Health',
      published_at: '2026-07-15T10:00:00Z',
      status: 'current',
      stale: false,
    },
  ],
  sources: [
    {
      id: 'source-1',
      section_key: 'overview',
      pipeline_id: 'platform/service-health',
      output_name: 'Service Health',
      entry_key: 'service-health',
      enabled: true,
      required_for_refresh: true,
      refresh_order: 0,
    },
  ],
};

const refreshes: DashboardRefresh[] = [
  {
    id: 'refresh-1',
    dashboard_id: 'dashboard-1',
    dashboard_ref: 'platform/ops',
    trigger_type: 'manual',
    scope_type: 'section',
    scope: { section_key: 'overview' },
    mode: 'strict',
    status: 'complete',
    total_sources: 1,
    required_sources: 1,
    queued_sources: 0,
    running_sources: 0,
    successful_sources: 1,
    failed_sources: 0,
    skipped_sources: 0,
    max_concurrency: 4,
    timeout_seconds: 2700,
    started_at: '2026-07-15T10:00:00Z',
    created_at: '2026-07-15T10:00:00Z',
    updated_at: '2026-07-15T10:01:00Z',
    sources: [
      {
        id: 'refresh-source-1',
        refresh_id: 'refresh-1',
        source_binding_id: 'source-1',
        pipeline_id: 'platform/service-health',
        output_name: 'Service Health',
        section_key: 'overview',
        entry_key: 'service-health',
        required: true,
        status: 'complete',
        created_at: '2026-07-15T10:00:00Z',
        updated_at: '2026-07-15T10:01:00Z',
      },
    ],
  },
];

const schedules: DashboardRefreshSchedule[] = [
  {
    id: 'schedule-1',
    dashboard_id: 'dashboard-1',
    dashboard_ref: 'platform/ops',
    name: 'Hourly',
    cron: '0 * * * *',
    cron_expression: '0 * * * *',
    timezone: 'UTC',
    enabled: true,
    scope_type: 'section',
    scope: { section_key: 'overview' },
    mode: 'best_effort',
    max_concurrency: 2,
    timeout_seconds: 1800,
    service_account_id: 'dashboard-schedule:schedule-1',
    source: 'database',
    managed_by_config_repo: false,
    created_at: '2026-07-15T10:00:00Z',
    updated_at: '2026-07-15T10:00:00Z',
  },
];

const history: DashboardEvent[] = [
  {
    id: 'event-1',
    section_key: 'overview',
    entry_key: 'service-health',
    revision: 2,
    event_type: 'published',
    created_at: '2026-07-15T10:00:00Z',
  },
];

test('dashboard workspace uses a dashboard dropdown and details-on-demand panels', () => {
  const onSelectDashboard = vi.fn();
  const onDeleteDashboard = vi.fn();
  const onScheduleDashboard = vi.fn();
  const onCreateSchedule = vi.fn();
  const onEditSchedule = vi.fn();
  const onDeleteSchedule = vi.fn();

  render(
    <MemoryRouter>
      <DashboardWorkspace
        dashboards={[dashboard]}
        teams={['platform']}
        selectedID="dashboard-1"
        selectedDashboard={dashboard}
        view={view}
        history={history}
        refreshes={refreshes}
        refreshSchedules={schedules}
        loading={false}
        detailLoading={false}
        error={null}
        searchTerm=""
        teamFilter=""
        saving={false}
        canWriteDashboards
        canDeleteDashboards
        onSearchTermChange={vi.fn()}
        onTeamFilterChange={vi.fn()}
        onSelectDashboard={onSelectDashboard}
        onReloadDashboards={vi.fn()}
        onCreateDashboard={vi.fn()}
        onEditDashboard={vi.fn()}
        onDeleteDashboard={onDeleteDashboard}
        onRefreshDashboard={vi.fn()}
        onScheduleDashboard={onScheduleDashboard}
        onEditSource={vi.fn()}
        onDeleteSource={vi.fn()}
        onRefreshSource={vi.fn()}
        onCancelRefresh={vi.fn()}
        onRetryRefresh={vi.fn()}
        onCreateSchedule={onCreateSchedule}
        onEditSchedule={onEditSchedule}
        onDeleteSchedule={onDeleteSchedule}
        onToggleSchedule={vi.fn()}
        onRunSchedule={vi.fn()}
      />
    </MemoryRouter>
  );

  const selector = screen.getByLabelText('Dashboard');
  expect(selector).toHaveValue('dashboard-1');
  fireEvent.change(selector, { target: { value: 'dashboard-1' } });
  expect(onSelectDashboard).toHaveBeenCalledWith('dashboard-1');

  expect(screen.getByRole('heading', { name: 'Overview' })).toBeVisible();
  expect(screen.getByRole('link', { name: 'Run run-1234' })).toHaveAttribute('href', '/pipelineruns/recent/run-123456789');
  expect(screen.queryByText('Sources')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Section actions' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Refresh section' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit section' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Delete section' })).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Show details' }));
  expect(screen.getByText('Sources')).toBeVisible();
  expect(screen.getByText('Hourly')).toBeVisible();
  expect(screen.getByText('Latest runs')).toBeVisible();
  expect(screen.getAllByText('published').length).toBeGreaterThan(0);
  expect(screen.queryByRole('button', { name: 'New schedule' })).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Collapse section' }));
  expect(screen.queryByText('Service Health')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Expand section' })).toBeVisible();

  fireEvent.click(screen.getByRole('button', { name: 'Dashboard actions' }));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Schedule refresh' }));
  expect(onScheduleDashboard).toHaveBeenCalled();

  fireEvent.click(screen.getByRole('button', { name: 'Dashboard actions' }));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Show dashboard details' }));
  expect(screen.getByText('Dashboard details')).toBeVisible();
  fireEvent.click(screen.getByRole('button', { name: 'New schedule' }));
  expect(onCreateSchedule).toHaveBeenCalledWith({ scopeType: 'dashboard' });
  fireEvent.click(screen.getAllByRole('button', { name: 'Edit Hourly' })[0]);
  expect(onEditSchedule).toHaveBeenCalledWith(expect.objectContaining({ id: 'schedule-1' }));
  fireEvent.click(screen.getAllByRole('button', { name: 'Delete Hourly' })[0]);
  expect(onDeleteSchedule).toHaveBeenCalledWith(expect.objectContaining({ id: 'schedule-1' }));

  fireEvent.click(screen.getByRole('button', { name: 'Dashboard actions' }));
  fireEvent.click(screen.getByRole('menuitem', { name: 'Delete dashboard' }));
  expect(onDeleteDashboard).toHaveBeenCalledWith(expect.objectContaining({ id: 'dashboard-1' }));
});

test('section details include failed latest run attempts even without a publication', () => {
  render(
    <MemoryRouter>
      <DashboardWorkspace
        dashboards={[dashboard]}
        teams={['platform']}
        selectedID="dashboard-1"
        selectedDashboard={dashboard}
        view={{ ...view, publications: [] }}
        history={[]}
        refreshes={[
          {
            ...refreshes[0],
            id: 'refresh-failed-publication',
            status: 'failed',
            successful_sources: 0,
            failed_sources: 1,
            finished_at: '2026-07-15T10:06:00Z',
            updated_at: '2026-07-15T10:06:00Z',
            sources: [
              {
                ...refreshes[0].sources![0],
                id: 'refresh-source-failed-publication',
                status: 'failed',
                error: 'Dashboard publication validation failed.',
                run_id: 'run-failed-publication-123',
                finished_at: '2026-07-15T10:06:00Z',
                updated_at: '2026-07-15T10:06:00Z',
              },
            ],
          },
        ]}
        refreshSchedules={schedules}
        loading={false}
        detailLoading={false}
        error={null}
        searchTerm=""
        teamFilter=""
        saving={false}
        canWriteDashboards
        canDeleteDashboards
        onSearchTermChange={vi.fn()}
        onTeamFilterChange={vi.fn()}
        onSelectDashboard={vi.fn()}
        onReloadDashboards={vi.fn()}
        onCreateDashboard={vi.fn()}
        onEditDashboard={vi.fn()}
        onDeleteDashboard={vi.fn()}
        onRefreshDashboard={vi.fn()}
        onScheduleDashboard={vi.fn()}
        onEditSource={vi.fn()}
        onDeleteSource={vi.fn()}
        onRefreshSource={vi.fn()}
        onCancelRefresh={vi.fn()}
        onRetryRefresh={vi.fn()}
        onCreateSchedule={vi.fn()}
        onEditSchedule={vi.fn()}
        onDeleteSchedule={vi.fn()}
        onToggleSchedule={vi.fn()}
        onRunSchedule={vi.fn()}
      />
    </MemoryRouter>
  );

  fireEvent.click(screen.getByRole('button', { name: 'Show details' }));
  expect(screen.getByText('Latest runs')).toBeVisible();
  expect(screen.getAllByText('Dashboard publication validation failed.').length).toBeGreaterThan(0);
  expect(screen.getByRole('link', { name: 'Run run-fail' })).toHaveAttribute('href', '/pipelineruns/recent/run-failed-publication-123');
  expect(screen.queryByRole('button', { name: 'New schedule' })).not.toBeInTheDocument();
});

test('dashboard workspace shows generating dashboard outputs inside their section', () => {
  render(
    <MemoryRouter>
      <DashboardWorkspace
        dashboards={[dashboard]}
        teams={['platform']}
        selectedID="dashboard-1"
        selectedDashboard={dashboard}
        view={view}
        history={history}
        refreshes={[
          {
            ...refreshes[0],
            id: 'refresh-running',
            status: 'running',
            sources: [
              {
                ...refreshes[0].sources![0],
                id: 'refresh-source-running',
                status: 'running',
                run_id: 'run-active-123456',
              },
            ],
          },
        ]}
        refreshSchedules={schedules}
        loading={false}
        detailLoading={false}
        error={null}
        searchTerm=""
        teamFilter=""
        saving={false}
        canWriteDashboards
        canDeleteDashboards
        onSearchTermChange={vi.fn()}
        onTeamFilterChange={vi.fn()}
        onSelectDashboard={vi.fn()}
        onReloadDashboards={vi.fn()}
        onCreateDashboard={vi.fn()}
        onEditDashboard={vi.fn()}
        onDeleteDashboard={vi.fn()}
        onRefreshDashboard={vi.fn()}
        onScheduleDashboard={vi.fn()}
        onEditSource={vi.fn()}
        onDeleteSource={vi.fn()}
        onRefreshSource={vi.fn()}
        onCancelRefresh={vi.fn()}
        onRetryRefresh={vi.fn()}
        onCreateSchedule={vi.fn()}
        onEditSchedule={vi.fn()}
        onDeleteSchedule={vi.fn()}
        onToggleSchedule={vi.fn()}
        onRunSchedule={vi.fn()}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('Dashboard output generating')).toBeVisible();
  expect(screen.getByRole('link', { name: 'Run run-acti' })).toHaveAttribute('href', '/pipelineruns/recent/run-active-123456');
});
