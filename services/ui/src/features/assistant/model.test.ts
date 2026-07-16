import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  assistantConversationClipboardText,
  assistantConversationUsageLabel,
  assistantExecutionPlanFromMessage,
  assistantLastUserMessage,
  assistantMessageUsageLabel,
  assistantVisibleToolActivity,
  normalizeAssistantConfig,
  normalizeAssistantConversation,
  normalizeAssistantConversationsPayload,
  normalizeAssistantLLMProfilesPayload,
  normalizeAssistantMessagePayload,
  proposedChangesFromMessages,
} from './model.js';

describe('assistant model', () => {
  it('normalizes conversations with memory and messages', () => {
    const conversation = normalizeAssistantConversation({
      id: 'c1',
      selected_llm_profile: 'standard',
      memory: {
        summary: 'Investigating run failure',
        open_tasks: ['fix yaml', 'fix yaml', ''],
        previous_proposed_fixes: ['update image'],
        selected_run: 'run-1',
      },
      messages: [
        {
          id: 'm1',
          role: 'assistant',
          content: 'ready',
          usage: {
            content_tokens: 2,
            prompt_tokens: 10,
            completion_tokens: 4,
            total_tokens: 14,
            estimated: false,
            duration_ms: 1200,
            llm_calls: 1,
          },
          tool_calls: [{
            name: 'nopsai.get_pipeline_run',
            input: { run_id: 'run-1' },
            output: { status: 'failure' },
            status: 'success',
            resource_uris: ['nopsai://pipeline-runs'],
          }],
        },
      ],
    });

    assert.equal(conversation.docs_version, 'auto');
    assert.equal(conversation.memory.summary, 'Investigating run failure');
    assert.deepEqual(conversation.memory.open_tasks, ['fix yaml']);
    assert.equal(conversation.messages[0].tool_calls[0].name, 'nopsai.get_pipeline_run');
    assert.equal(conversation.messages[0].tool_calls[0].input.run_id, 'run-1');
    assert.equal(conversation.messages[0].tool_calls[0].output.status, 'failure');
    assert.equal(assistantMessageUsageLabel(conversation.messages[0]), '14 LLM tokens · 1.2s');
    assert.equal(assistantConversationUsageLabel(conversation), '14 LLM tokens · 1 message · 1.2s');
  });

  it('normalizes list and message response payloads', () => {
    assert.equal(normalizeAssistantConversationsPayload({ conversations: [{ id: 'c1' }] }).conversations.length, 1);
    assert.equal(normalizeAssistantMessagePayload({
      conversation: { id: 'c1' },
      user_message: { role: 'user', content: 'why failed?' },
      reply: { role: 'assistant', content: 'checking' },
    }).reply.content, 'checking');
  });

  it('normalizes assistant LLM profile picker payloads without admin-only fields', () => {
    const payload = normalizeAssistantLLMProfilesPayload({
      default_profile: 'standard',
      profiles: [
        {
          name: ' blocked ',
          provider: 'openai',
          model: 'gpt-test',
          status: 'valid',
          allowed_in_scope: false,
          disabled_reason: 'not allowed',
          credential_ref: 'credential://system/llm/blocked',
        },
        {
          name: 'standard',
          provider: 'lmstudio',
          model: 'qwen',
          status: 'valid',
          allowed_in_scope: true,
        },
        { name: '   ' },
      ],
    });

    assert.equal(payload.default_profile, 'standard');
    assert.deepEqual(payload.profiles.map(profile => profile.name), ['blocked', 'standard']);
    assert.deepEqual(payload.profiles[0], {
      name: 'blocked',
      provider: 'openai',
      model: 'gpt-test',
      status: 'valid',
      validation: undefined,
      allowed_in_scope: false,
      disabled_reason: 'not allowed',
    });
    assert.equal('credential_ref' in payload.profiles[0], false);
  });

  it('normalizes safe assistant config without credential material', () => {
    const config = normalizeAssistantConfig({
      enabled: true,
      provider: 'gemini',
      model: 'gemini-2.5-pro',
      default_docs_version: '2026.06',
      credential_configured: true,
      credential_ref: 'credential://system/assistant/api-key',
      api_key_secret: 'NOPSAI_ASSISTANT_API_KEY',
      features: {
        docs: false,
        pipeline_debugging: true,
        action_execution: false,
      },
      actions: {
        require_confirmation: true,
      },
    });

    assert.equal(config.enabled, true);
    assert.equal(config.provider, 'gemini');
    assert.equal(config.default_docs_version, '2026.06');
    assert.equal(config.credential_configured, true);
    assert.equal(config.features.docs, false);
    assert.equal(config.features.pipeline_debugging, true);
    assert.equal(config.features.config_generation, true);
    assert.equal(config.features.action_execution, false);
    assert.equal(config.actions.require_confirmation, true);
    assert.equal('credential_ref' in config, false);
    assert.equal('api_key_secret' in config, false);
  });

  it('keeps retry/export helpers focused on user-visible chat content and evidence tools', () => {
    const conversation = normalizeAssistantConversation({
      id: 'c1',
      messages: [
        { id: 'm1', role: 'user', content: 'show pipeline' },
        {
          id: 'm2',
          role: 'assistant',
          content: 'Pipeline loaded',
          tool_calls: [
            { name: 'nopsai.llm.plan', status: 'success', resource_uris: ['nopsai://features'] },
            { name: 'nopsai.get_pipeline', status: 'success', resource_uris: ['nopsai://pipelines'] },
            { name: 'nopsai.llm.complete', status: 'success', resource_uris: ['nopsai://system/llm-profiles'] },
            { name: 'nopsai.propose_pipeline_update', status: 'success', output: { proposal_type: 'pipeline_update', yaml: 'name: deploy', applies: false } },
          ],
        },
        { id: 'm3', role: 'user', content: 'validate it' },
      ],
    });

    assert.equal(assistantLastUserMessage(conversation.messages)?.content, 'validate it');
    assert.match(assistantConversationClipboardText(conversation), /Assistant:\nPipeline loaded/);
    assert.deepEqual(assistantVisibleToolActivity(conversation.messages).map(tool => tool.name), ['nopsai.get_pipeline', 'nopsai.propose_pipeline_update']);
    assert.deepEqual(proposedChangesFromMessages(conversation.messages), [{
      key: 'm2-nopsai.propose_pipeline_update-3',
      title: 'Pipeline Update',
      body: 'name: deploy',
      note: 'Proposal only. No changes were applied.',
    }]);
  });

  it('extracts execution plans while keeping them out of evidence lists', () => {
    const conversation = normalizeAssistantConversation({
      id: 'c1',
      messages: [{
        id: 'm1',
        role: 'assistant',
        content: 'Feature coverage was checked.',
        tool_calls: [
          {
            name: 'nopsai.assistant.execution_plan',
            status: 'success',
            source: 'llm',
            phase: 'planning',
            confidence: 'medium',
            output: {
              execution_plan: {
                goal: 'Check feature coverage',
                intent: 'llm_planned',
                summary: 'Use MCP first, then synthesize.',
                requires_confirmation: false,
                steps: [
                  {
                    index: 1,
                    title: 'Read capability evidence',
                    source: 'mcp',
                    phase: 'evidence',
                    confidence: 'high',
                    tool: 'nopsai.get_feature_capabilities',
                    status: 'planned',
                  },
                  {
                    index: 2,
                    title: 'Synthesize answer',
                    source: 'llm',
                    phase: 'synthesis',
                    confidence: 'medium',
                    status: 'planned',
                  },
                ],
              },
            },
          },
          {
            name: 'nopsai.get_feature_capabilities',
            status: 'success',
            source: 'mcp',
            phase: 'evidence',
            confidence: 'high',
          },
        ],
      }],
    });

    const plan = assistantExecutionPlanFromMessage(conversation.messages[0]);
    assert.equal(plan?.goal, 'Check feature coverage');
    assert.equal(plan?.steps[0].source, 'mcp');
    assert.equal(plan?.steps[1].phase, 'synthesis');
    assert.deepEqual(assistantVisibleToolActivity(conversation.messages).map(tool => tool.name), ['nopsai.get_feature_capabilities']);
  });
});
