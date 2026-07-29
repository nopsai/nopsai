import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { PipelineSchedule } from '../features/schedules/model';

const schedule: PipelineSchedule = {
  id: 'schedule-1',
  path: 'platform',
  name: 'Nightly deploy',
  identifier: 'platform/nightly-deploy',
  pipeline: 'platform/deploy',
  schedule_kind: 'cron',
  cron_expression: '0 2 * * *',
  timezone: 'UTC',
  enabled: true,
  source: 'database',
  latest_run: {
    run_id: 'run-1',
    status: 'success',
  },
};

const api = vi.hoisted(() => ({
  fetchSchedules: vi.fn(),
  fetchScheduleMetadata: vi.fn(),
  saveSchedule: vi.fn(),
  setScheduleEnabled: vi.fn(),
  runSchedule: vi.fn(),
  deleteSchedule: vi.fn(),
}));

vi.mock('../features/schedules/api', () => ({
  deleteSchedule: api.deleteSchedule,
  fetchScheduleMetadata: api.fetchScheduleMetadata,
  fetchSchedules: api.fetchSchedules,
  runSchedule: api.runSchedule,
  saveSchedule: api.saveSchedule,
  setScheduleEnabled: api.setScheduleEnabled,
}));

import SchedulesPage from './Schedules';

function LocationProbe() {
  const location = useLocation();
  return <span data-testid="location">{location.pathname}{location.search}</span>;
}

describe('SchedulesPage modal flows', () => {
  beforeEach(() => {
    api.fetchSchedules.mockResolvedValue([schedule]);
    api.fetchScheduleMetadata.mockResolvedValue({
      pipelines: ['platform/deploy'],
      teams: ['platform'],
      scopes: [''],
    });
    api.saveSchedule.mockResolvedValue({ ...schedule, id: 'schedule-2', name: 'Morning deploy' });
    api.deleteSchedule.mockResolvedValue(undefined);
    vi.spyOn(window, 'confirm').mockReturnValue(true);
  });

  it('saves a schedule through the feature API', async () => {
    render(
      <MemoryRouter>
        <SchedulesPage canWriteSchedules canDeleteSchedules />
      </MemoryRouter>
    );

    await screen.findByText('Nightly deploy');
    const table = screen.getByTestId('schedule-workspace-table');
    expect(table).toHaveClass('schedule-workspace__table-shell');
    expect(screen.getByRole('region', { name: 'Pipeline schedule workspace' })).toBeVisible();
    expect(screen.getByRole('table', { name: 'Pipeline schedules' })).toBeVisible();
    expect(screen.getByLabelText('Search schedules')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Reload schedules' })).toBeVisible();
    expect(screen.getByLabelText('Filter by schedule path')).toBeVisible();
    expect(screen.getByRole('tab', { name: 'Enabled' })).toBeVisible();
    const opener = screen.getByRole('button', { name: 'New schedule' });
    await userEvent.click(opener);
    const dialog = screen.getByRole('dialog', { name: 'New schedule' });
    expect(dialog).toBeVisible();
    expect(dialog).toHaveClass(
      'pipelines-modal-card',
      'workflow-form-dialog',
      'workflow-form-dialog--wide'
    );
    expect(dialog.querySelector('.pipelines-modal-header')).not.toBeNull();
    expect(dialog.querySelector('.pipelines-modal-body')).not.toBeNull();
    expect(dialog.querySelector('.pipelines-modal-footer')).not.toBeNull();
    expect(screen.getByLabelText('Name')).toHaveFocus();
    await userEvent.type(screen.getByLabelText('Name'), 'Morning deploy');
    await userEvent.selectOptions(screen.getByLabelText('Pipeline'), 'platform/deploy');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => expect(api.saveSchedule).toHaveBeenCalledOnce());
    expect(api.saveSchedule.mock.calls[0]?.[0]).toEqual(expect.objectContaining({
      name: 'Morning deploy',
      pipeline: 'platform/deploy',
    }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(opener).toHaveFocus();
  });

  it('deletes a schedule after confirmation', async () => {
    render(
      <MemoryRouter>
        <SchedulesPage canWriteSchedules canDeleteSchedules />
      </MemoryRouter>
    );

    await screen.findByText('Nightly deploy');
    await userEvent.click(screen.getByRole('button', { name: 'Select schedule Nightly deploy' }));
    await userEvent.click(screen.getByRole('button', { name: 'Delete Nightly deploy' }));
    await waitFor(() => expect(api.deleteSchedule).toHaveBeenCalledWith('schedule-1'));
  });

  it('reflects selected schedules in the URL and opens direct schedule links', async () => {
    const firstView = render(
      <MemoryRouter initialEntries={['/schedules']}>
        <SchedulesPage canWriteSchedules canDeleteSchedules />
        <LocationProbe />
      </MemoryRouter>
    );

    await screen.findByText('Nightly deploy');
    await userEvent.click(screen.getByRole('button', { name: 'Select schedule Nightly deploy' }));
    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent('/schedules/platform/nightly-deploy');
    });
    firstView.unmount();

    render(
      <MemoryRouter initialEntries={['/schedules/platform/nightly-deploy']}>
        <SchedulesPage canWriteSchedules canDeleteSchedules />
      </MemoryRouter>
    );

    await screen.findByRole('heading', { name: 'Execution' });
    expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  });

  it('migrates legacy schedule query links to route-backed detail links', async () => {
    render(
      <MemoryRouter initialEntries={['/schedules?schedule=platform/nightly-deploy']}>
        <SchedulesPage canWriteSchedules canDeleteSchedules />
        <LocationProbe />
      </MemoryRouter>
    );

    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent('/schedules/platform/nightly-deploy');
    });
    expect(await screen.findByRole('button', { name: 'List' })).toBeVisible();
  });

  it('opens latest runs on direct pipeline run detail routes', async () => {
    render(
      <MemoryRouter initialEntries={['/schedules']}>
        <SchedulesPage canWriteSchedules canDeleteSchedules />
        <LocationProbe />
      </MemoryRouter>
    );

    await screen.findByText('Nightly deploy');
    await userEvent.click(screen.getByRole('button', { name: 'Open latest run for Nightly deploy' }));

    await waitFor(() => {
      expect(screen.getByTestId('location')).toHaveTextContent('/pipelineruns/recent/run-1');
    });
  });
});
