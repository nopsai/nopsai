import { fireEvent, render, screen } from '@testing-library/react';
import { expect, test, vi } from 'vitest';
import type { PipelineSchedule } from './model';
import { ScheduleCard } from './ScheduleCards';

const schedule: PipelineSchedule = {
  id: 'schedule-1',
  path: 'platform',
  name: 'Nightly deploy',
  description: 'Deploys the platform every night.',
  identifier: 'platform/nightly-deploy',
  pipeline: 'platform/deploy',
  schedule_kind: 'cron',
  cron_expression: '0 2 * * *',
  timezone: 'UTC',
  scope: 'production',
  run_group_path: 'platform/prod',
  enabled: true,
  source: 'database',
  next_run_at: '2026-06-16T02:00:00Z',
  latest_run: {
    run_id: 'run-1',
    status: 'success',
    started_at: '2026-06-15T02:00:00Z',
  },
};

test('routes every compact schedule action through its feature callback', () => {
  const callbacks = {
    onEdit: vi.fn(),
    onEnable: vi.fn(),
    onRun: vi.fn(),
    onDelete: vi.fn(),
    onOpenRun: vi.fn(),
  };

  render(
    <ScheduleCard
      schedule={schedule}
      canWriteSchedules
      canDeleteSchedules
      busy={false}
      {...callbacks}
    />
  );

  const card = screen.getByRole('article');
  const identityRow = card.querySelector('.compact-resource-card__identity-row');
  const headingActions = card.querySelector('.compact-resource-card__heading-actions');
  const actionRow = card.querySelector('.compact-resource-card__actions');
  const footerActions = card.querySelector('.compact-resource-card__footer-actions');
  expect(card).toHaveClass('compact-resource-card--bordered', 'schedule-card');
  expect(identityRow).not.toBeNull();
  expect(headingActions).not.toBeNull();
  expect(actionRow).not.toBeNull();
  expect(footerActions).not.toBeNull();

  const runButton = screen.getByRole('button', { name: 'Run Nightly deploy now' });
  const deleteButton = screen.getByRole('button', { name: 'Delete Nightly deploy' });
  const latestRunButton = screen.getByRole('button', { name: 'Open latest run for Nightly deploy' });
  expect(runButton).toHaveClass('schedule-card__icon-button');
  expect(deleteButton).toHaveClass('pipelines-delete-button');
  expect(latestRunButton).toHaveClass('schedule-card__latest-run-button');
  expect(identityRow).toContainElement(headingActions);
  expect(headingActions).toContainElement(runButton);
  expect(headingActions).toContainElement(deleteButton);
  const headingActionButtons = Array.from(headingActions?.children || []);
  expect(headingActionButtons[headingActionButtons.length - 1]).toBe(deleteButton);
  expect(headingActionButtons[headingActionButtons.length - 2]).toBe(runButton);
  expect(actionRow).not.toContainElement(runButton);
  expect(actionRow).not.toContainElement(deleteButton);
  expect(footerActions).toContainElement(latestRunButton);
  expect(actionRow).not.toContainElement(latestRunButton);

  fireEvent.click(screen.getByRole('button', { name: 'Run Nightly deploy now' }));
  fireEvent.click(screen.getByRole('button', { name: 'Disable Nightly deploy' }));
  fireEvent.click(screen.getByRole('button', { name: 'Edit Nightly deploy' }));
  fireEvent.click(screen.getByRole('button', { name: 'Delete Nightly deploy' }));
  fireEvent.click(screen.getByRole('button', { name: 'Open latest run for Nightly deploy' }));

  expect(callbacks.onRun).toHaveBeenCalledWith(schedule);
  expect(callbacks.onEnable).toHaveBeenCalledWith(schedule, false);
  expect(callbacks.onEdit).toHaveBeenCalledWith(schedule);
  expect(callbacks.onDelete).toHaveBeenCalledWith(schedule);
  expect(callbacks.onOpenRun).toHaveBeenCalledWith('run-1');
  expect(screen.getByText('Deploys the platform every night.')).toBeVisible();
  expect(screen.getByText('Daily at 02:00')).toBeVisible();
  expect(screen.getByText('Success')).toBeVisible();
});

test('allows GitOps schedule mutation actions with override affordances', () => {
  const callbacks = {
    onEdit: vi.fn(),
    onEnable: vi.fn(),
    onRun: vi.fn(),
    onDelete: vi.fn(),
    onOpenRun: vi.fn(),
  };
  const managedSchedule = {
    ...schedule,
    name: '',
    description: '',
    enabled: false,
    source: 'git',
    managed_by_config_repo: true,
    latest_run: undefined,
    last_run_id: '',
    last_status: '',
  };
  render(
    <ScheduleCard
      schedule={managedSchedule}
      canWriteSchedules
      canDeleteSchedules
      busy
      {...callbacks}
    />
  );

  expect(screen.getByText('GitOps')).toBeVisible();
  expect(screen.getByText('Disabled')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Run platform/nightly-deploy now' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Edit platform/nightly-deploy' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Delete platform/nightly-deploy' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Enable platform/nightly-deploy' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: /Open latest run/ })).not.toBeInTheDocument();
});

test('allows database schedule delete with delete permission even when writes are read-only', () => {
  const callbacks = {
    onEdit: vi.fn(),
    onEnable: vi.fn(),
    onRun: vi.fn(),
    onDelete: vi.fn(),
    onOpenRun: vi.fn(),
  };

  render(
    <ScheduleCard
      schedule={{ ...schedule, latest_run: undefined, last_run_id: '', last_status: '' }}
      canWriteSchedules={false}
      canDeleteSchedules
      busy={false}
      {...callbacks}
    />
  );

  expect(screen.getByRole('button', { name: 'Delete Nightly deploy' })).toBeVisible();
  expect(screen.queryByRole('button', { name: 'Run Nightly deploy now' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Edit Nightly deploy' })).not.toBeInTheDocument();
});
