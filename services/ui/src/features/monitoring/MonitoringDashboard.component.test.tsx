import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { MonitoringDashboard } from './MonitoringDashboard';

test('renders monitoring metrics and delegates group selection', async () => {
  const onSelectGroup = vi.fn();
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <MonitoringDashboard
        groups={[{ id: 7, name: 'Platform' }]}
        resourceCounts={{ pipelines: 4, steps: 8, triggers: 2 }}
        services={[{ id: 'dispatcher', label: 'Dispatcher', status: 'ok', message: 'Healthy' }]}
        runners={[
          {
            label: 'runner-1',
            status: 'online',
            runtime: 'docker',
            namespace: '',
            node: '',
            capacity: 2,
            activeJobs: 1,
            inflightJobs: 1,
            activeRuns: [],
          },
        ]}
        runnerSummary={{ total: 1, online: 1, docker: 1, kubernetes: 0, capacity: 2, activeJobs: 1 }}
        runtimeUnavailable={null}
        loading={false}
        summary={{
          totalRuns: 10,
          successRuns: 8,
          failedRuns: 1,
          runningRuns: 1,
          cancelledRuns: 0,
          successRate: 0.8,
          totalDurationMs: 120000,
          averageDurationMs: 12000,
        }}
        dailyBuckets={[{ label: 'Jun 8', runs: 10, failures: 1, averageDurationMs: 12000 }]}
        statusCounts={{ success: 8, failure: 1, running: 1, cancelled: 0, pending: 0, skipped: 0 }}
        groupMetrics={[
          {
            group: { id: 7, name: 'Platform' },
            label: 'Platform',
            depth: 0,
            totalRuns: 10,
            successRate: 0.8,
            totalDurationMs: 120000,
            averageDurationMs: 12000,
          },
        ]}
        pipelineMetrics={[
          {
            id: 'platform/release',
            pipelineName: 'release',
            groupLabel: 'Platform',
            totalRuns: 10,
            successRate: 0.8,
            averageDurationMs: 12000,
            totalDurationMs: 120000,
          },
        ]}
        onSelectGroup={onSelectGroup}
      />
    </MemoryRouter>
  );

  expect(screen.getAllByText('80%')).toHaveLength(3);
  expect(screen.getByText('Dispatcher')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Platform' }));
  expect(onSelectGroup).toHaveBeenCalledWith(7);
});
