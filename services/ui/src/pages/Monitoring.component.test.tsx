import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';

const api = vi.hoisted(() => ({
  fetchTeams: vi.fn(),
  requestMonitoringJson: vi.fn(),
  sendMonitoringJson: vi.fn(),
}));

vi.mock('../features/teams/api', () => ({
  fetchTeams: api.fetchTeams,
}));

vi.mock('../features/monitoring/api', () => ({
  requestMonitoringJson: api.requestMonitoringJson,
  sendMonitoringJson: api.sendMonitoringJson,
}));

vi.mock('../features/monitoring/MonitoringDashboard', () => ({
  MonitoringDashboard: ({
    activeTab,
    onTabChange,
  }: {
    activeTab: string;
    onTabChange: (tab: 'runners') => void;
  }) => (
    <section>
      <span data-testid="active-monitoring-tab">{activeTab}</span>
      <button type="button" onClick={() => onTabChange('runners')}>
        Runners
      </button>
    </section>
  ),
}));

import MonitoringPage from './Monitoring';

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname}{location.search}</span>;
}

beforeEach(() => {
  window.localStorage.clear();
  api.fetchTeams.mockResolvedValue([]);
  api.sendMonitoringJson.mockResolvedValue({});
  api.requestMonitoringJson.mockImplementation((path: string) => {
    if (path === '/v1/monitoring/dispatcher') {
      return Promise.resolve({ services: [], runners: [], runner_summary: {} });
    }
    if (
      path === '/v1/monitoring/views' ||
      path === '/v1/monitoring/alert-rules' ||
      path === '/v1/monitoring/alert-events' ||
      path === '/v1/monitoring/recommendations?status=open'
    ) {
      return Promise.resolve([]);
    }
    return Promise.resolve({});
  });
});

test('changes monitoring tabs without bouncing back to the previous route tab', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/monitoring/overview']}>
      <MonitoringPage />
      <LocationProbe />
    </MemoryRouter>
  );

  expect(screen.getByTestId('active-monitoring-tab')).toHaveTextContent('overview');

  await user.click(screen.getByRole('button', { name: 'Runners' }));

  await waitFor(() => {
    expect(screen.getByTestId('active-monitoring-tab')).toHaveTextContent('runners');
    expect(screen.getByTestId('location')).toHaveTextContent('/monitoring/runners');
  });
});
