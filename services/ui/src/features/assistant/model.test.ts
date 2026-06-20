import { describe, expect, it } from 'vitest';
import {
  assistantConversationClipboardText,
  assistantLastUserMessage,
  assistantVisibleToolActivity,
  normalizeAssistantConfig,
  normalizeAssistantConversation,
  normalizeAssistantConversationsPayload,
  normalizeAssistantLLMProfilesPayload,
  normalizeAssistantMessagePayload,
} from './model';

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

    expect(conversation.docs_version).toBe('auto');
    expect(conversation.memory.summary).toBe('Investigating run failure');
    expect(conversation.memory.open_tasks).toEqual(['fix yaml']);
    expect(conversation.messages[0].tool_calls[0].name).toBe('nopsai.get_pipeline_run');
    expect(conversation.messages[0].tool_calls[0].input.run_id).toBe('run-1');
    expect(conversation.messages[0].tool_calls[0].output.status).toBe('failure');
  });

  it('normalizes list and message response payloads', () => {
    expect(normalizeAssistantConversationsPayload({ conversations: [{ id: 'c1' }] }).conversations).toHaveLength(1);
    expect(normalizeAssistantMessagePayload({
      conversation: { id: 'c1' },
      user_message: { role: 'user', content: 'why failed?' },
      reply: { role: 'assistant', content: 'checking' },
    }).reply.content).toBe('checking');
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

    expect(payload.default_profile).toBe('standard');
    expect(payload.profiles.map(profile => profile.name)).toEqual(['blocked', 'standard']);
    expect(payload.profiles[0]).toEqual({
      name: 'blocked',
      provider: 'openai',
      model: 'gpt-test',
      status: 'valid',
      validation: undefined,
      allowed_in_scope: false,
      disabled_reason: 'not allowed',
    });
    expect('credential_ref' in payload.profiles[0]).toBe(false);
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

    expect(config.enabled).toBe(true);
    expect(config.provider).toBe('gemini');
    expect(config.default_docs_version).toBe('2026.06');
    expect(config.credential_configured).toBe(true);
    expect(config.features.docs).toBe(false);
    expect(config.features.pipeline_debugging).toBe(true);
    expect(config.features.config_generation).toBe(true);
    expect(config.features.action_execution).toBe(false);
    expect(config.actions.require_confirmation).toBe(true);
    expect('credential_ref' in config).toBe(false);
    expect('api_key_secret' in config).toBe(false);
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
          ],
        },
        { id: 'm3', role: 'user', content: 'validate it' },
      ],
    });

    expect(assistantLastUserMessage(conversation.messages)?.content).toBe('validate it');
    expect(assistantConversationClipboardText(conversation)).toContain('Assistant:\nPipeline loaded');
    expect(assistantVisibleToolActivity(conversation.messages).map(tool => tool.name)).toEqual(['nopsai.get_pipeline']);
  });
});
