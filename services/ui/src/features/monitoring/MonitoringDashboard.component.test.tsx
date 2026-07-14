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
          estimated_ai_tokens: 4200,
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
        runnerSummary={{ total: 1, online: 1, stale: 0, unreachable: 0, disabled: 0, unknown: 0, docker: 1, kubernetes: 0, capacity: 2, activeJobs: 1, inflightJobs: 1, queuedJobs: 2 }}
        runtimeUnavailable={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('Pipeline runs')).toBeVisible();
  expect(screen.getByText('80%')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Runners' }));
  expect(onTabChange).toHaveBeenCalledWith('runners');
});

test('renders exact, estimated, and profile LLM token usage separately', () => {
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
          total_prompt_tokens: 800,
          total_completion_tokens: 400,
          total_tokens: 1200,
          exact_tokens: 900,
          estimated_tokens: 300,
          exact_token_events: 3,
          estimated_token_events: 1,
          assistant_chat_tokens: 200,
          assistant_chat_messages: 4,
          by_pipeline: [{ key: 'platform/release', label: 'release', count: 4, tokens: 1200 }],
          by_step: [{ key: 'plan', label: 'plan', count: 2, tokens: 900 }],
          by_task: [{ key: 'plan/summarize', label: 'plan/summarize', count: 1, tokens: 600 }],
          by_feature: [
            { key: 'log_analysis', label: 'log_analysis', count: 2, tokens: 700 },
            { key: 'assistant_chat', label: 'Assistant chat', count: 4, tokens: 200 },
          ],
          by_profile: [{ key: 'default', label: 'default', count: 2, tokens: 800 }],
          by_model: [{ key: 'gemini/gemini-2.5-pro', label: 'gemini/gemini-2.5-pro', count: 2, tokens: 700 }],
          trend: [{ key: '2026-06-12', label: '2026-06-12', runs: 1200 }],
          top_token_runs: [{ key: 'run-1', label: 'run-1', count: 2, tokens: 1200 }],
        }}
        reliability={null}
        efficiency={null}
        security={null}
        services={[]}
        runners={[]}
        runnerSummary={{ total: 0, online: 0, stale: 0, unreachable: 0, disabled: 0, unknown: 0, docker: 0, kubernetes: 0, capacity: 0, activeJobs: 0, inflightJobs: 0, queuedJobs: 0 }}
        runtimeUnavailable={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('Exact tokens')).toBeVisible();
  expect(screen.getByText('Estimated tokens')).toBeVisible();
  expect(screen.getByText('3 provider events')).toBeVisible();
  expect(screen.getByText(/200 assistant chat/)).toBeVisible();
  expect(screen.getByText('Assistant chat')).toBeVisible();
  expect(screen.getByText('1 estimated events')).toBeVisible();
  expect(screen.getByText('By Step')).toBeVisible();
  expect(screen.getByText('By Task')).toBeVisible();
  expect(screen.getByText('By LLM Profile')).toBeVisible();
  expect(screen.getByText('default')).toBeVisible();
  expect(screen.getByText('plan/summarize')).toBeVisible();
  expect(screen.getByText('Top Token Runs')).toBeVisible();
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
        runnerSummary={{ total: 0, online: 0, stale: 0, unreachable: 0, disabled: 0, unknown: 0, docker: 0, kubernetes: 0, capacity: 0, activeJobs: 0, inflightJobs: 0, queuedJobs: 0 }}
        runtimeUnavailable={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByText('Rate limits')).toBeVisible();
  expect(screen.getByText('Last Fired')).toBeVisible();
  expect(screen.getByText('10/min')).toBeVisible();
});
