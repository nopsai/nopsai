import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { MonitoringDashboard } from './MonitoringDashboard';

test('renders backend monitoring metrics and switches tabs', async () => {
  const user = userEvent.setup();
  const onTabChange = vi.fn();

  render(
    <MemoryRouter>
      <MonitoringDashboard
        activeTab="overview"
        onTabChange={onTabChange}
        loading={false}
        summary={{
          total_runs: 10,
          successful_runs: 8,
          failed_runs: 1,
          running_runs: 1,
          cancelled_runs: 0,
          success_rate: 0.8,
          failure_rate: 0.1,
          average_duration_seconds: 12,
          p95_duration_seconds: 20,
          ai_spend_usd: 4.2,
          total_steps_executed: 30,
          total_tasks_executed: 45,
          queued_jobs: 2,
          runner_utilization: 0.5,
        }}
        runAnalytics={{
          runs_over_time: [{ key: '2026-06-10', label: '2026-06-10', runs: 10, failures: 1, average_duration_seconds: 12 }],
          status_split: [
            { key: 'success', label: 'success', count: 8 },
            { key: 'failure', label: 'failure', count: 1 },
          ],
          trigger_source_split: [{ key: 'manual', label: 'manual', count: 10, failed: 1 }],
          failure_reasons: [{ key: 'timeout', label: 'timeout', count: 1 }],
        }}
        pipelinePerformance={{ items: [{ key: 'platform/release', pipeline_name: 'release', total_runs: 10, success_rate: 0.8, p95_duration_seconds: 20 }] }}
        stepPerformance={{ items: [] }}
        taskPerformance={{ items: [] }}
        triggerAnalytics={null}
        externalTriggerAnalytics={null}
        runnerHistory={null}
        aiUsage={null}
        reliability={null}
        efficiency={null}
        security={null}
        services={[{ id: 'dispatcher', label: 'Dispatcher', status: 'ok', message: 'Healthy' }]}
        runners={[
          {
            runnerId: 'runner-1',
            label: 'runner-1',
            status: 'online',
            runtime: 'docker',
            namespace: '',
            node: '',
            capacity: 2,
            activeJobs: 1,
            inflightJobs: 1,
            allowDispatch: true,
            activeRuns: [],
          },
        ]}
        runnerSummary={{ total: 1, online: 1, recovered: 0, stale: 0, unreachable: 0, disabled: 0, unknown: 0, docker: 1, kubernetes: 0, capacity: 2, activeJobs: 1, inflightJobs: 1, queuedJobs: 2 }}
        runtimeUnavailable={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('Pipeline runs')).toBeVisible();
  expect(screen.getByText('80%')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Runners' }));
  expect(onTabChange).toHaveBeenCalledWith('runners');
});

test('reports AI usage as one spend figure and warns when it is incomplete', () => {
  render(
    <MemoryRouter>
      <MonitoringDashboard
        activeTab="ai-usage"
        onTabChange={vi.fn()}
        loading={false}
        summary={null}
        runAnalytics={null}
        pipelinePerformance={null}
        stepPerformance={null}
        taskPerformance={null}
        triggerAnalytics={null}
        externalTriggerAnalytics={null}
        runnerHistory={null}
        aiUsage={{
          spend_usd: 12.34,
          priced_calls: 3,
          unpriced_calls: 1,
          assistant_spend_usd: 2,
          assistant_chat_messages: 4,
          by_pipeline: [{ key: 'platform/release', label: 'release', count: 4, cost_usd: 12.34 }],
          by_step: [{ key: 'plan', label: 'plan', count: 2, cost_usd: 9 }],
          by_task: [{ key: 'plan/summarize', label: 'plan/summarize', count: 1, cost_usd: 6 }],
          by_feature: [
            { key: 'log_analysis', label: 'log_analysis', count: 2, cost_usd: 7 },
            { key: 'assistant_chat', label: 'Assistant chat', count: 4, cost_usd: 2 },
          ],
          by_provider: [{ key: 'gemini', label: 'gemini', count: 2, cost_usd: 7 }],
          by_profile: [{ key: 'default', label: 'default', count: 2, cost_usd: 8 }],
          by_model: [{ key: 'gemini/gemini-2.5-pro', label: 'gemini/gemini-2.5-pro', count: 2, cost_usd: 7 }],
          trend: [{ key: '2026-06-12', label: '2026-06-12', runs: 1200 }],
          top_spend_runs: [{ key: 'run-1', label: 'run-1', count: 2, cost_usd: 12.34 }],
        }}
        reliability={null}
        efficiency={null}
        security={null}
        services={[]}
        runners={[]}
        runnerSummary={{ total: 0, online: 0, recovered: 0, stale: 0, unreachable: 0, disabled: 0, unknown: 0, docker: 0, kubernetes: 0, capacity: 0, activeJobs: 0, inflightJobs: 0, queuedJobs: 0 }}
        runtimeUnavailable={null}
      />
    </MemoryRouter>
  );

  // One number, in money.
  expect(screen.getByText('AI spend')).toBeVisible();
  // The hero figure, plus the breakdown rows that add up to it.
  expect(screen.getAllByText('$12.34').length).toBeGreaterThan(0);
  expect(screen.queryByText('Exact tokens')).toBeNull();
  expect(screen.queryByText('Estimated tokens')).toBeNull();
  // ...and an explicit warning that the number is missing an unpriced call,
  // rather than presenting a partial total as a final one.
  expect(screen.getByText(/This total is incomplete/)).toBeVisible();
  expect(screen.getByText(/1 call not priced/)).toBeVisible();
  expect(screen.getByText(/\$2\.00 assistant chat/)).toBeVisible();
  expect(screen.getByText('Assistant chat')).toBeVisible();
  expect(screen.getByText('By Step')).toBeVisible();
  expect(screen.getByText('By Task')).toBeVisible();
  expect(screen.getByText('By Provider')).toBeVisible();
  expect(screen.getByText('gemini')).toBeVisible();
  expect(screen.getByText('By LLM Profile')).toBeVisible();
  expect(screen.getByText('default')).toBeVisible();
  expect(screen.getByText('plan/summarize')).toBeVisible();
  expect(screen.getByText('Most Expensive Runs')).toBeVisible();
});

test('renders external trigger last-fired and rate-limit analytics', () => {
  render(
    <MemoryRouter>
      <MonitoringDashboard
        activeTab="external-triggers"
        onTabChange={vi.fn()}
        loading={false}
        summary={null}
        runAnalytics={null}
        pipelinePerformance={null}
        stepPerformance={null}
        taskPerformance={null}
        triggerAnalytics={null}
        externalTriggerAnalytics={{
          total_external_triggers: 2,
          enabled_external_triggers: 1,
          invocation_count: 9,
          failed_invocations: 2,
          pending_invocations: 1,
          successful_invocations: 7,
          invocation_to_run_rate: 0.77,
          idempotency_conflicts: 1,
          rate_limit_violations: 3,
          most_fired_triggers: [{ key: 'deploy', label: 'Deploy', count: 9, failed: 2 }],
          rate_limit_violation_triggers: [{ key: 'deploy', label: 'Deploy', count: 3 }],
          last_fired_triggers: [{ id: 'deploy', name: 'Deploy', enabled: true, last_used_at: '2026-06-12T10:00:00Z', rate_limit: '10/min' }],
        }}
        runnerHistory={null}
        aiUsage={null}
        reliability={null}
        efficiency={null}
        security={null}
        services={[]}
        runners={[]}
        runnerSummary={{ total: 0, online: 0, recovered: 0, stale: 0, unreachable: 0, disabled: 0, unknown: 0, docker: 0, kubernetes: 0, capacity: 0, activeJobs: 0, inflightJobs: 0, queuedJobs: 0 }}
        runtimeUnavailable={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('Rate limits')).toBeVisible();
  expect(screen.getByText('Last Fired')).toBeVisible();
  expect(screen.getByText('10/min')).toBeVisible();
});
