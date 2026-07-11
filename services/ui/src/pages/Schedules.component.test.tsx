import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
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
    const list = screen.getByTestId('schedule-card-list');
    expect(list).toHaveClass('compact-resource-grid');
    expect(list.querySelectorAll('.compact-resource-card')).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Search schedules' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Refresh schedules' })).toBeVisible();
    expect(screen.queryByLabelText('Filter by team')).not.toBeInTheDocument();
    expect(screen.queryByText('Show disabled')).not.toBeInTheDocument();
    expect(screen.queryByText('1 total')).not.toBeInTheDocument();
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
    await userEvent.click(screen.getByTitle('Delete schedule'));
    await waitFor(() => expect(api.deleteSchedule).toHaveBeenCalledWith('schedule-1'));
  });
});
