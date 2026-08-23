import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import { assistantProgressElapsedLabel, assistantProgressLabel, assistantReadyLine } from './experience.js';
import {
  emptyAssistantConversationUsage,
  emptyAssistantMemory,
  type AssistantConfig,
  type AssistantConversation,
} from './model.js';

describe('assistant experience helpers', () => {
  it('summarizes readiness with the active confirmation policy', () => {
    assert.match(assistantReadyLine({ ...enabledConfig, actions: { require_confirmation: true } }), /changes always need your review/);
    assert.match(assistantReadyLine({ ...enabledConfig, actions: { require_confirmation: false } }), /review policy is relaxed/);
  });

  it('shows the step the running turn reported', () => {
    assert.equal(assistantProgressLabel(conversationInProgress('Reading pipeline run logs')), 'Reading pipeline run logs');
  });

  it('says something neutral when the turn has reported nothing yet', () => {
    assert.equal(assistantProgressLabel(null), 'Working on it');
    assert.equal(assistantProgressLabel(conversationInProgress('   ')), 'Working on it');
  });

  it('reports elapsed time in the header, where it is shown once', () => {
    assert.equal(assistantProgressElapsedLabel(500), 'just started');
    assert.equal(assistantProgressElapsedLabel(12000), '12s');
    assert.equal(assistantProgressElapsedLabel(125000), '2m 5s');
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

function conversationInProgress(turnProgress: string): AssistantConversation {
  return {
    id: 'c1',
    user_id: 'u1',
    title: 'Conversation',
    selected_llm_profile: 'standard',
    docs_version: 'auto',
    scope: '',
    memory: emptyAssistantMemory,
    messages: [],
    usage: emptyAssistantConversationUsage,
    created_at: '2026-06-20T00:00:00Z',
    updated_at: '2026-06-20T00:00:00Z',
    turn_running: true,
    running_turn_started_at: '2026-06-20T00:00:00Z',
    turn_progress: turnProgress,
  };
}

