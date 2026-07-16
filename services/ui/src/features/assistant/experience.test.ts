import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { assistantProgressLabel, assistantProgressSteps, assistantReadyLine } from './experience.js';
import { emptyAssistantMessageUsage, type AssistantConfig } from './model.js';

describe('assistant experience helpers', () => {
  it('summarizes readiness with the active confirmation policy', () => {
    assert.match(assistantReadyLine({ ...enabledConfig, actions: { require_confirmation: true } }), /changes always need your review/);
    assert.match(assistantReadyLine({ ...enabledConfig, actions: { require_confirmation: false } }), /review policy is relaxed/);
  });

  it('uses contextual progress copy for common operational asks', () => {
    assert.equal(assistantProgressLabel([message('Why did this run fail?')], null), 'Plan the request with current permissions');
    assert.deepEqual(progressLabels(assistantProgressSteps([message('Analyze AI usage cost by provider')], null)), [
      'Plan the request with current permissions',
      'Read AI usage, profile, and cost evidence',
      'Compare recorded usage with configured profiles',
      'Synthesize an evidence-backed answer',
      'Save and reconcile the chat result',
    ]);
    assert.deepEqual(progressLabels(assistantProgressSteps([message('Check system health')], null)), [
      'Plan the request with current permissions',
      'Check system and runner health evidence',
      'Separate warnings from blocking issues',
      'Synthesize an evidence-backed answer',
      'Save and reconcile the chat result',
    ]);
  });

  it('advances progress state by elapsed time', () => {
    assert.deepEqual(assistantProgressSteps([message('Analyze AI usage cost by provider')], null, 12000).map(step => step.state), [
      'done',
      'done',
      'active',
      'pending',
      'pending',
    ]);
  });
});

const enabledConfig: AssistantConfig = {
  enabled: true,
  provider: 'openai',
  model: 'gpt-test',
  default_docs_version: 'auto',
  conversation_retention_days: 30,
  max_input_logs_bytes: 120000,
  max_conversation_turns: 30,
  docs_enabled: true,
  docs_version_aware: true,
  credential_configured: true,
  dedicated_profile: 'assistant',
  memory: { enabled: true, scope: 'conversation' },
  mcp: { enabled: true },
  features: {
    docs: true,
    pipeline_debugging: true,
    config_generation: true,
    statistics_insights: true,
    maintenance_recommendations: true,
    cost_recommendations: true,
    action_execution: false,
  },
  actions: { require_confirmation: true },
};

function message(content: string) {
  return {
    id: 'm1',
    conversation_id: 'c1',
    role: 'user',
    content,
    tool_calls: [],
    usage: emptyAssistantMessageUsage,
    created_at: '2026-06-20T00:00:00Z',
  };
}

function progressLabels(steps: ReturnType<typeof assistantProgressSteps>) {
  return steps.map(step => step.label);
}
