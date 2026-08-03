import { useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { PipelineSchedule } from './model';
import { ScheduleWorkspace } from './ScheduleWorkspace';

const nightlySchedule: PipelineSchedule = {
  id: 'schedule-1',
  path: 'platform/prod',
  name: 'Nightly deploy',
  description: 'Deploys the platform every night.',
  identifier: 'platform/prod/nightly-deploy',
  pipeline: 'platform/deploy',
  schedule_kind: 'cron',
  cron_expression: '0 2 * * *',
  timezone: 'UTC',
  scope: 'production',
  run_team_path: 'platform/prod',
  variables: { ENV: 'prod' },
  enabled: true,
  source: 'database',
  next_run_at: '2026-06-16T02:00:00Z',
  latest_run: {
    run_id: 'run-1',
    status: 'success',
    started_at: '2026-06-15T02:00:00Z',
  },
};

const releaseWindowSchedule: PipelineSchedule = {
  id: 'schedule-2',
  path: '',
  name: 'Release window',
  identifier: 'release-window',
  pipeline: 'platform/release',
  schedule_kind: 'once',
  run_at: '2026-06-18T09:30:00Z',
  timezone: 'UTC',
  scope: '',
  run_team_path: 'global',
  enabled: false,
  source: 'git',
  managed_by_config_repo: true,
  config_source_path: 'schedules/release-window.yaml',
  last_status: 'failed',
};

function callbacks() {
  return {
    onSearchTermChange: vi.fn(),
    onClearPipelineFilter: vi.fn(),
    onSelectedScheduleIDChange: vi.fn(),
    onCreate: vi.fn(),
    onEdit: vi.fn(),
    onEnable: vi.fn(),
    onRun: vi.fn(),
    onDelete: vi.fn(),
    onOpenRun: vi.fn(),
  };
}

function renderWorkspace(overrides: Partial<Parameters<typeof ScheduleWorkspace>[0]> = {}) {
  const props = {
    schedules: [nightlySchedule, releaseWindowSchedule],
    teams: ['platform', 'platform/prod'],
    loading: false,
    error: null,
    saving: false,
    busyScheduleID: null,
    searchTerm: '',
    pipelineFilter: '',
    selectedScheduleID: '',
    canWriteSchedules: true,
    canDeleteSchedules: true,
    ...callbacks(),
    ...overrides,
  };
  function Harness() {
    const [selectedScheduleID, setSelectedScheduleID] = useState(props.selectedScheduleID);
    return (
      <ScheduleWorkspace
        {...props}
        selectedScheduleID={selectedScheduleID}
        onSelectedScheduleIDChange={scheduleID => {
          props.onSelectedScheduleIDChange(scheduleID);
          setSelectedScheduleID(scheduleID);
        }}
      />
    );
  }
  render(<Harness />);
  return props;
}

describe('ScheduleWorkspace', () => {
  it('renders the schedule registry and opens a schedule detail with action callbacks', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace({ pipelineFilter: 'platform/deploy' });

    expect(screen.getByRole('region', { name: 'Pipeline schedule workspace' })).toBeVisible();
    expect(screen.getAllByText('platform/deploy').length).toBeGreaterThan(0);
    expect(screen.getByRole('tab', { name: /Enabled.*1/ })).toBeVisible();
    expect(screen.getByRole('table', { name: 'Pipeline schedules' })).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Select schedule Nightly deploy' }));

    expect(screen.getByRole('heading', { name: 'Execution' })).toBeVisible();
    expect(screen.getByText('Deploys the platform every night.')).toBeVisible();
    const variablesSection = screen.getByRole('heading', { name: 'Variables' }).closest('section');
    expect(variablesSection).not.toBeNull();
    expect(within(variablesSection as HTMLElement).getByText('ENV')).toBeVisible();
    expect(within(variablesSection as HTMLElement).getByText('prod')).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Run Nightly deploy now' }));
    await user.click(screen.getByRole('button', { name: 'Disable Nightly deploy' }));
    await user.click(screen.getByRole('button', { name: 'Edit Nightly deploy' }));
    await user.click(screen.getByRole('button', { name: 'Open latest run for Nightly deploy' }));
    await user.click(screen.getByRole('button', { name: 'Delete Nightly deploy' }));

    expect(props.onRun).toHaveBeenCalledWith(nightlySchedule);
    expect(props.onEnable).toHaveBeenCalledWith(nightlySchedule, false);
    expect(props.onEdit).toHaveBeenCalledWith(nightlySchedule);
    expect(props.onOpenRun).toHaveBeenCalledWith('run-1');
    expect(props.onDelete).toHaveBeenCalledWith(nightlySchedule);
  });

  it('links the latest run directly from the schedule table', async () => {
    const user = userEvent.setup();
    const props = renderWorkspace();

    await user.click(screen.getByRole('button', { name: 'Open latest run for Nightly deploy' }));

    expect(props.onOpenRun).toHaveBeenCalledWith('run-1');
    expect(screen.queryByRole('heading', { name: 'Execution' })).not.toBeInTheDocument();
  });

  it('filters visible schedules by state and tree team path', async () => {
    const user = userEvent.setup();
    renderWorkspace();

    await user.click(screen.getByRole('tab', { name: /GitOps/ }));
    const table = screen.getByRole('table', { name: 'Pipeline schedules' });
    expect(within(table).queryByText('Nightly deploy')).not.toBeInTheDocument();
    expect(within(table).getByText('Release window')).toBeVisible();

    await user.click(screen.getByRole('button', { name: 'Expand platform' }));
    await user.click(screen.getByRole('button', { name: 'Open team platform/prod' }));
    expect(screen.getByText('No schedules match the current filters.')).toBeVisible();
  });

  it('keeps delete available in detail when schedules are otherwise read-only', async () => {
    const user = userEvent.setup();
    renderWorkspace({ canWriteSchedules: false, canDeleteSchedules: true });

    expect(screen.getByRole('button', { name: 'New schedule' })).toBeDisabled();
    expect(screen.queryByRole('button', { name: 'Run Nightly deploy now' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete Nightly deploy' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Select schedule Nightly deploy' }));

    expect(screen.getByRole('button', { name: 'Delete Nightly deploy' })).toBeVisible();
  });
});
