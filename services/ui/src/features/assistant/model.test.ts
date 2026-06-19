import { describe, expect, it } from 'vitest';
import {
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
});
