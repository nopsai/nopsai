import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { AssistantPanel } from './AssistantPanel';
import { emptyAssistantMemory, type AssistantConfig, type AssistantConversation, type AssistantMessage } from './model';
import { useAssistantController } from './useAssistantController';

vi.mock('./useAssistantController', () => ({
  useAssistantController: vi.fn(),
}));

const useAssistantControllerMock = vi.mocked(useAssistantController);

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
  useAssistantControllerMock.mockReset();
});

test('renders clean chat messages without inline tool-call details', async () => {
  const user = userEvent.setup();
  const copyMessage = vi.fn();
  const deleteConversation = vi.fn();
  const retryLastUserMessage = vi.fn();
  const messages = [
    assistantMessage('m1', 'user', 'show pipeline'),
    {
      ...assistantMessage('m2', 'assistant', 'Pipeline loaded.'),
      tool_calls: [
        { name: 'nopsai.llm.plan', input: {}, output: {}, status: 'success', resource_uris: ['nopsai://features'] },
        { name: 'nopsai.get_pipeline', input: {}, output: {}, status: 'success', resource_uris: ['nopsai://pipelines'] },
        { name: 'nopsai.llm.complete', input: {}, output: {}, status: 'success', resource_uris: ['nopsai://system/llm-profiles'] },
      ],
    },
  ];

  useAssistantControllerMock.mockReturnValue({
    conversations: [assistantConversation(messages)],
    activeConversation: assistantConversation(messages),
    activeMessages: messages,
    profiles: [],
    profileOptions: ['assistant'],
    selectedProfile: 'assistant',
    setSelectedProfile: vi.fn(),
    draft: '',
    setDraft: vi.fn(),
    loading: false,
    sending: false,
    retrying: false,
    deletingConversationID: '',
    copiedMessageID: '',
    conversationCopied: false,
    error: null,
    config: enabledConfig,
    enabled: true,
    canRetry: true,
    load: vi.fn(),
    selectConversation: vi.fn(),
    startConversation: vi.fn(),
    deleteConversation,
    retryLastUserMessage,
    copyMessage,
    copyConversation: vi.fn(),
    submitMessage: vi.fn(),
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Pipeline loaded.')).toBeVisible();
  expect(screen.queryByText(/nopsai\.llm\.plan/)).toBeNull();
  expect(screen.queryByText(/nopsai\.get_pipeline/)).toBeNull();
  expect(screen.queryByText(/nopsai:\/\/features/)).toBeNull();

  await user.click(screen.getAllByRole('button', { name: 'Copy message' })[1]);
  expect(copyMessage).toHaveBeenCalledWith(messages[1]);

  await user.click(screen.getByRole('button', { name: 'Retry this prompt' }));
  expect(retryLastUserMessage).toHaveBeenCalledOnce();

  await user.click(screen.getByRole('button', { name: 'Delete conversation' }));
  expect(deleteConversation).toHaveBeenCalledOnce();
});

function assistantConversation(messages: AssistantMessage[]): AssistantConversation {
  return {
    id: 'c1',
    user_id: 'user:viewer',
    title: 'Pipeline',
    selected_llm_profile: 'assistant',
    docs_version: 'auto',
    scope: '',
    memory: emptyAssistantMemory,
    messages,
    created_at: '2026-06-20T00:00:00Z',
    updated_at: '2026-06-20T00:00:00Z',
  };
}

function assistantMessage(id: string, role: 'user' | 'assistant', content: string): AssistantMessage {
  return {
    id,
    conversation_id: 'c1',
    role,
    content,
    tool_calls: [],
    created_at: '2026-06-20T00:00:00Z',
  };
}
