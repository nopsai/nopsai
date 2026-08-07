import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';

import type { DashboardWorkspaceProps } from '../features/dashboards/DashboardWorkspace';
import type { DashboardFormState, DashboardSummary, DashboardView } from '../features/dashboards/model';

type DashboardModalMockProps = {
  form: DashboardFormState;
  error: string | null;
  onChange: (next: DashboardFormState) => void;
  onSubmit: () => void;
};

const api = vi.hoisted(() => ({
  cancelDashboardRefresh: vi.fn(),
  deleteDashboard: vi.fn(),
  deleteDashboardPublication: vi.fn(),
  deleteDashboardRefreshSchedule: vi.fn(),
  deleteDashboardSection: vi.fn(),
  deleteDashboardSource: vi.fn(),
  fetchDashboardHistory: vi.fn(),
  fetchDashboardMetadata: vi.fn(),
  fetchDashboardPipelineCatalog: vi.fn(),
  fetchDashboardPipelineOutputs: vi.fn(),
  fetchDashboardRefreshes: vi.fn(),
  fetchDashboardRefreshSchedules: vi.fn(),
  fetchDashboards: vi.fn(),
  fetchDashboardView: vi.fn(),
  retryDashboardRefreshFailed: vi.fn(),
  runDashboardRefreshSchedule: vi.fn(),
  saveDashboard: vi.fn(),
  saveDashboardRefreshSchedule: vi.fn(),
  saveDashboardSection: vi.fn(),
  saveDashboardSource: vi.fn(),
  setDashboardRefreshScheduleEnabled: vi.fn(),
  startDashboardRefresh: vi.fn(),
}));

vi.mock('../features/dashboards/api', () => api);

vi.mock('../features/dashboards/DashboardModals', () => ({
  DashboardDeleteModal: () => null,
  DashboardModal: (props: DashboardModalMockProps) => (
    <div role="dialog" aria-label="Mock dashboard modal">
      <div data-testid="dashboard-form-slug">{props.form.slug}</div>
      <button
        type="button"
        onClick={() => props.onChange({
          ...props.form,
          teamPath: 'team-1',
          slug: 'empty-dashboard',
          title: 'Empty Dashboard',
          description: 'Created before source bindings exist.',
        })}
      >
        Fill dashboard identity
      </button>
      <button type="button" onClick={props.onSubmit}>Submit dashboard</button>
      {props.error ? <p role="alert">{props.error}</p> : null}
    </div>
  ),
  RefreshModal: () => null,
  RefreshScheduleModal: () => null,
  SectionModal: () => null,
  SourceModal: () => null,
}));

vi.mock('../features/dashboards/DashboardWorkspace', () => ({
  DashboardWorkspace: (props: DashboardWorkspaceProps) => (
    <main>
      <div data-testid="selected-dashboard">{props.selectedID}</div>
      <div data-testid="active-section">{props.activeSectionKey}</div>
      {props.canWriteDashboards ? (
        <button type="button" onClick={props.onCreateDashboard}>New dashboard</button>
      ) : null}
      <a data-testid="service-section-link" href={props.sectionTabHref('service-metrics')}>
        Service Metrics
      </a>
    </main>
  ),
}));

import DashboardsPage from './Dashboards';

const canonicalDashboardID = 'b6d7f0b9-c5fe-437b-9385-3b9eb4dddc82';

const dashboard: DashboardSummary = {
  id: canonicalDashboardID,
  team_path: 'team-1',
  ref: 'team-1/ops-dashboard',
  slug: 'ops-dashboard',
  title: 'Ops Dashboard',
  description: '',
  visibility: 'team',
  current_publication_count: 0,
};

const dashboardView: DashboardView = {
  dashboard,
  sections: [
    {
      id: 'section-service-metrics',
      section_key: 'service-metrics',
      title: 'Service Metrics',
      display_order: 0,
    },
  ],
  publications: [],
  sources: [],
};

beforeEach(() => {
  vi.clearAllMocks();
  api.fetchDashboardMetadata.mockResolvedValue({ teams: ['team-1'], pipelines: [], scopes: [''] });
  api.fetchDashboardPipelineCatalog.mockResolvedValue([]);
  api.fetchDashboards.mockResolvedValue([dashboard]);
  api.fetchDashboardView.mockResolvedValue(dashboardView);
  api.fetchDashboardHistory.mockResolvedValue([]);
  api.fetchDashboardRefreshes.mockResolvedValue([]);
  api.fetchDashboardRefreshSchedules.mockResolvedValue([]);
});

test('dashboard route aliases resolve before detail endpoints load and stay stable in section links', async () => {
  render(
    <MemoryRouter initialEntries={['/dashboards?dashboard=team-1%2Fops-dashboard&tab=service-metrics']}>
      <LocationProbe />
      <DashboardsPage canWriteDashboards={false} canDeleteDashboards={false} />
    </MemoryRouter>
  );

  expect(api.fetchDashboardView).not.toHaveBeenCalled();

  await waitFor(() => expect(api.fetchDashboardView).toHaveBeenCalledWith(canonicalDashboardID));

  expect(api.fetchDashboardView).not.toHaveBeenCalledWith('team-1/ops-dashboard');
  expect(api.fetchDashboardHistory).toHaveBeenCalledWith(canonicalDashboardID);
  expect(api.fetchDashboardRefreshes).toHaveBeenCalledWith(canonicalDashboardID);
  expect(api.fetchDashboardRefreshSchedules).toHaveBeenCalledWith(canonicalDashboardID);
  expect(screen.getByTestId('selected-dashboard')).toHaveTextContent(canonicalDashboardID);
  expect(screen.getByTestId('active-section')).toHaveTextContent('service-metrics');
  expect(screen.getByTestId('location')).toHaveTextContent(
    '/dashboards/team-1/ops-dashboard?tab=service-metrics'
  );
  expect(screen.getByTestId('service-section-link')).toHaveAttribute(
    'href',
    '/dashboards/team-1/ops-dashboard?tab=service-metrics'
  );
});

test('shows a validation error when creating without dashboard-output pipeline bindings', async () => {
  render(
    <MemoryRouter initialEntries={['/dashboards']}>
      <DashboardsPage canWriteDashboards canDeleteDashboards={false} />
    </MemoryRouter>
  );

  await screen.findByRole('button', { name: 'New dashboard' });
  fireEvent.click(screen.getByRole('button', { name: 'New dashboard' }));
  fireEvent.click(screen.getByRole('button', { name: 'Fill dashboard identity' }));
  await waitFor(() => expect(screen.getByTestId('dashboard-form-slug')).toHaveTextContent('empty-dashboard'));

  fireEvent.click(screen.getByRole('button', { name: 'Submit dashboard' }));

  await waitFor(() => {
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Select at least one pipeline with dashboard outputs targeting team-1/empty-dashboard before creating the dashboard.'
    );
  });
  expect(api.saveDashboard).not.toHaveBeenCalled();
  expect(api.saveDashboardSource).not.toHaveBeenCalled();
});

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}{location.search}</div>;
}
