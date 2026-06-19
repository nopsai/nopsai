import { act, renderHook, waitFor } from '@testing-library/react';
import { beforeEach, expect, test, vi } from 'vitest';
import {
  createAssistantConversation,
  fetchAssistantConfig,
  fetchAssistantConversation,
  fetchAssistantConversations,
  fetchAssistantLLMProfiles,
  sendAssistantMessage,
} from './api.js';
import type { AssistantConfig, AssistantConversation, AssistantMessagePayload } from './model.js';
import { emptyAssistantMemory } from './model.js';
import { useAssistantController } from './useAssistantController.js';

vi.mock('./api.js', () => ({
  createAssistantConversation: vi.fn(),
  fetchAssistantConfig: vi.fn(),
  fetchAssistantConversation: vi.fn(),
  fetchAssistantConversations: vi.fn(),
  fetchAssistantLLMProfiles: vi.fn(),
  sendAssistantMessage: vi.fn(),
}));

const fetchAssistantConfigMock = vi.mocked(fetchAssistantConfig);
const fetchAssistantConversationsMock = vi.mocked(fetchAssistantConversations);
const fetchAssistantConversationMock = vi.mocked(fetchAssistantConversation);
const fetchAssistantLLMProfilesMock = vi.mocked(fetchAssistantLLMProfiles);
const createAssistantConversationMock = vi.mocked(createAssistantConversation);
const sendAssistantMessageMock = vi.mocked(sendAssistantMessage);

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

beforeEach(() => {
  fetchAssistantConfigMock.mockReset();
  fetchAssistantConversationsMock.mockReset();
  fetchAssistantConversationMock.mockReset();
  fetchAssistantLLMProfilesMock.mockReset();
  createAssistantConversationMock.mockReset();
  sendAssistantMessageMock.mockReset();

  fetchAssistantConfigMock.mockResolvedValue(enabledConfig);
  fetchAssistantConversationsMock.mockResolvedValue({ conversations: [] });
  fetchAssistantLLMProfilesMock.mockResolvedValue({
    default_profile: 'assistant',
    profiles: [{ name: 'assistant', provider: 'openai', model: 'gpt-test', status: 'valid', allowed_in_scope: true }],
  });
});

test('clears the draft immediately and shows a pending message while sending', async () => {
  const conversation = assistantConversation('c1', []);
  createAssistantConversationMock.mockResolvedValue(conversation);

  let resolveMessage!: (payload: AssistantMessagePayload) => void;
  const messagePromise = new Promise<AssistantMessagePayload>(resolve => {
    resolveMessage = resolve;
  });
  sendAssistantMessageMock.mockReturnValue(messagePromise);

  const { result } = renderHook(() => useAssistantController());
  await waitFor(() => expect(result.current.enabled).toBe(true));

  act(() => result.current.setDraft('why did the deploy fail?'));
  let submitPromise!: Promise<void>;
  act(() => {
    submitPromise = result.current.submitMessage();
  });

  await waitFor(() => {
    expect(result.current.draft).toBe('');
    expect(result.current.sending).toBe(true);
    expect(result.current.activeMessages.at(-1)?.content).toBe('why did the deploy fail?');
  });

  const completedConversation = assistantConversation('c1', [
    assistantMessage('m1', 'c1', 'user', 'why did the deploy fail?'),
    assistantMessage('m2', 'c1', 'assistant', 'The image tag was invalid.'),
  ]);
  await act(async () => {
    resolveMessage({
      conversation: completedConversation,
      user_message: completedConversation.messages[0],
      reply: completedConversation.messages[1],
    });
    await submitPromise;
  });

  expect(result.current.sending).toBe(false);
  expect(result.current.activeMessages.map(message => message.id)).toEqual(['m1', 'm2']);
});

test('restores the draft if sending fails after optimistic clear', async () => {
  createAssistantConversationMock.mockResolvedValue(assistantConversation('c1', []));
  sendAssistantMessageMock.mockRejectedValue(new Error('assistant unavailable'));

  const { result } = renderHook(() => useAssistantController());
  await waitFor(() => expect(result.current.enabled).toBe(true));

  act(() => result.current.setDraft('check variable duplicates'));
  await act(async () => {
    await result.current.submitMessage();
  });

  expect(result.current.draft).toBe('check variable duplicates');
  expect(result.current.activeMessages).toEqual([]);
  expect(result.current.error).toBe('assistant unavailable');
});

function assistantConversation(id: string, messages: ReturnType<typeof assistantMessage>[]): AssistantConversation {
  return {
    id,
    user_id: 'user:viewer',
    title: '',
    selected_llm_profile: 'assistant',
    docs_version: 'auto',
    scope: '',
    memory: emptyAssistantMemory,
    messages,
    created_at: '2026-06-19T00:00:00Z',
    updated_at: '2026-06-19T00:00:00Z',
  };
}

function assistantMessage(id: string, conversationID: string, role: string, content: string) {
  return {
    id,
    conversation_id: conversationID,
    role,
    content,
    tool_calls: [],
    created_at: '2026-06-19T00:00:00Z',
  };
}
