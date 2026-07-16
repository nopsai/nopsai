import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { AssistantPanel } from './AssistantPanel';
import { emptyAssistantConversationUsage, emptyAssistantMemory, emptyAssistantMessageUsage, type AssistantConfig, type AssistantConversation, type AssistantMessage } from './model';
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
  const retryMessage = vi.fn();
  const messages = [
    assistantMessage('m1', 'user', 'show pipeline'),
    {
      ...assistantMessage('m2', 'assistant', 'Pipeline loaded.'),
      tool_calls: [
        { name: 'nopsai.llm.plan', input: {}, output: {}, status: 'success', resource_uris: ['nopsai://features'] },
        {
          name: 'nopsai.assistant.execution_plan',
          input: {},
          output: {
            execution_plan: {
              goal: 'Show pipeline',
              intent: 'llm_planned',
              summary: 'Use MCP first, then synthesize a concise answer.',
              requires_confirmation: false,
              steps: [
                {
                  index: 1,
                  title: 'Read pipeline metadata',
                  source: 'mcp',
                  phase: 'evidence',
                  confidence: 'high',
                  status: 'planned',
                },
                {
                  index: 2,
                  title: 'Synthesize the answer',
                  source: 'llm',
                  phase: 'synthesis',
                  confidence: 'medium',
                  status: 'planned',
                },
              ],
            },
          },
          status: 'success',
          resource_uris: ['nopsai://assistant/execution-plan'],
        },
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
    sendingConversationID: '',
    activeConversationSending: false,
    activeConversationSendingStartedAt: 0,
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
    retryMessage,
    retryLastUserMessage: vi.fn(),
    copyMessage,
    copyConversation: vi.fn(),
    submitMessage: vi.fn(),
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Pipeline loaded.')).toBeVisible();
  expect(screen.getByText('Execution plan')).toBeVisible();
  expect(screen.getByText('Read pipeline metadata')).toBeVisible();
  expect(screen.getByText('MCP')).toBeVisible();
  expect(screen.getByText('Synthesize the answer')).toBeVisible();
  expect(screen.queryByText(/nopsai\.llm\.plan/)).toBeNull();
  expect(screen.queryByText(/nopsai\.get_pipeline/)).toBeNull();
  expect(screen.queryByText(/nopsai:\/\/features/)).toBeNull();
  expect(screen.queryByRole('button', { name: 'Refresh assistant' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Retry last prompt' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Delete conversation' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'New' })).toBeNull();
  expect(screen.queryByRole('button', { name: 'Copy conversation' })).toBeNull();

  await user.click(screen.getAllByRole('button', { name: 'Copy message' })[1]);
  expect(copyMessage).toHaveBeenCalledWith(messages[1]);

  await user.click(screen.getByRole('button', { name: 'Retry this prompt' }));
  expect(retryMessage).toHaveBeenCalledWith(messages[0]);
  expect(deleteConversation).not.toHaveBeenCalled();
});

test('renders welcome starters that prefill the composer', async () => {
  const user = userEvent.setup();
  const setDraft = vi.fn();

  useAssistantControllerMock.mockReturnValue({
    conversations: [],
    activeConversation: null,
    activeMessages: [],
    profiles: [],
    profileOptions: ['assistant'],
    selectedProfile: 'assistant',
    setSelectedProfile: vi.fn(),
    draft: '',
    setDraft,
    loading: false,
    sending: false,
    sendingConversationID: '',
    activeConversationSending: false,
    activeConversationSendingStartedAt: 0,
    retrying: false,
    deletingConversationID: '',
    copiedMessageID: '',
    conversationCopied: false,
    error: null,
    config: enabledConfig,
    enabled: true,
    canRetry: false,
    load: vi.fn(),
    selectConversation: vi.fn(),
    startConversation: vi.fn(),
    deleteConversation: vi.fn(),
    retryMessage: vi.fn(),
    retryLastUserMessage: vi.fn(),
    copyMessage: vi.fn(),
    copyConversation: vi.fn(),
    submitMessage: vi.fn(),
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText("Hi, I'm NopsAI. What are we solving today?")).toBeVisible();
  expect(screen.getByRole('separator', { name: 'Resize message composer' })).toBeVisible();
  expect(screen.getByPlaceholderText('Describe what you are trying to achieve...')).toHaveClass('resize-none');
  await user.click(screen.getByRole('button', { name: 'Explain a failed run' }));
  expect(setDraft).toHaveBeenCalledWith('Explain a failed run');
  expect(screen.getAllByText(/changes always need your review/i).length).toBeGreaterThan(0);
});

test('renders staged progress while the active conversation is sending', () => {
  const messages = [
    assistantMessage('m1', 'user', 'Analyze AI usage cost by provider'),
  ];
  const conversation = assistantConversation(messages);

  useAssistantControllerMock.mockReturnValue({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    profiles: [],
    profileOptions: ['assistant'],
    selectedProfile: 'assistant',
    setSelectedProfile: vi.fn(),
    draft: '',
    setDraft: vi.fn(),
    loading: false,
    sending: true,
    sendingConversationID: 'c1',
    activeConversationSending: true,
    activeConversationSendingStartedAt: Date.now(),
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
    deleteConversation: vi.fn(),
    retryMessage: vi.fn(),
    retryLastUserMessage: vi.fn(),
    copyMessage: vi.fn(),
    copyConversation: vi.fn(),
    submitMessage: vi.fn(),
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Working through the request')).toBeVisible();
  expect(screen.getByText('Plan the request with current permissions')).toBeVisible();
  expect(screen.getByText('Read AI usage, profile, and cost evidence')).toBeVisible();
  expect(screen.getByText('Compare recorded usage with configured profiles')).toBeVisible();
  expect(screen.getByText('Synthesize an evidence-backed answer')).toBeVisible();
  expect(screen.getByText('Save and reconcile the chat result')).toBeVisible();
});

test('keeps non-running conversation delete actions available during a send', async () => {
  const user = userEvent.setup();
  const deleteConversation = vi.fn();
  const runningConversation = assistantConversation([assistantMessage('m1', 'user', 'Analyze AI usage cost by provider')]);
  const otherConversation = {
    ...assistantConversation([assistantMessage('m2', 'user', 'previous chat')]),
    id: 'c2',
    title: 'Previous chat',
  };

  useAssistantControllerMock.mockReturnValue({
    conversations: [runningConversation, otherConversation],
    activeConversation: runningConversation,
    activeMessages: runningConversation.messages,
    profiles: [],
    profileOptions: ['assistant'],
    selectedProfile: 'assistant',
    setSelectedProfile: vi.fn(),
    draft: '',
    setDraft: vi.fn(),
    loading: false,
    sending: true,
    sendingConversationID: 'c1',
    activeConversationSending: true,
    activeConversationSendingStartedAt: Date.now(),
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
    retryMessage: vi.fn(),
    retryLastUserMessage: vi.fn(),
    copyMessage: vi.fn(),
    copyConversation: vi.fn(),
    submitMessage: vi.fn(),
  });

  render(<AssistantPanel />);

  expect(screen.getByRole('button', { name: 'Delete conversation Pipeline' })).toBeDisabled();
  const otherDelete = screen.getByRole('button', { name: 'Delete conversation Previous chat' });
  expect(otherDelete).toBeEnabled();
  await user.click(otherDelete);
  expect(deleteConversation).toHaveBeenCalledWith('c2');
});

test('renders assistant markdown and toggles usage details in the full page', async () => {
  const user = userEvent.setup();
  const messages = [
    assistantMessage('m1', 'user', 'show summary'),
    {
      ...assistantMessage('m2', 'assistant', '## Summary\n- Pipeline `deploy-api` is healthy.\n\n```yaml\nname: deploy-api\n```'),
      usage: {
        content_tokens: 12,
        prompt_tokens: 30,
        completion_tokens: 10,
        total_tokens: 40,
        estimated: false,
        duration_ms: 1250,
        llm_calls: 2,
      },
      tool_calls: [
        {
          name: 'nopsai.get_pipeline',
          input: { pipeline_id: 'deploy-api' },
          output: { status: 'ready' },
          status: 'success',
          resource_uris: ['nopsai://pipelines/deploy-api'],
        },
      ],
    },
  ];
  const conversation = {
    ...assistantConversation(messages),
    usage: {
      message_count: 2,
      content_tokens: 20,
      prompt_tokens: 30,
      completion_tokens: 10,
      total_tokens: 40,
      estimated_token_messages: 1,
      duration_ms: 1250,
      llm_calls: 2,
    },
  };

  useAssistantControllerMock.mockReturnValue({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    profiles: [],
    profileOptions: ['assistant'],
    selectedProfile: 'assistant',
    setSelectedProfile: vi.fn(),
    draft: '',
    setDraft: vi.fn(),
    loading: false,
    sending: false,
    sendingConversationID: '',
    activeConversationSending: false,
    activeConversationSendingStartedAt: 0,
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
    deleteConversation: vi.fn(),
    retryMessage: vi.fn(),
    retryLastUserMessage: vi.fn(),
    copyMessage: vi.fn(),
    copyConversation: vi.fn(),
    submitMessage: vi.fn(),
  });

  render(<AssistantPanel />);

  const conversationsRail = within(screen.getByLabelText('Assistant conversations'));
  expect(conversationsRail.queryByLabelText('LLM profile', { selector: 'select' })).toBeNull();
  expect(conversationsRail.queryByText('assistant')).toBeNull();
  expect(screen.getByText('Session details')).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Summary' })).toBeVisible();
  expect(screen.getAllByText(/Pipeline/).length).toBeGreaterThan(0);
  expect(screen.getAllByText('deploy-api').length).toBeGreaterThan(0);
  expect(screen.getByText(/40 LLM tokens · 1.3s · 2 LLM calls/)).toBeVisible();
  expect(screen.getAllByText(/40 LLM tokens · 2 messages · 1.3s · 1 estimated/).length).toBeGreaterThan(0);
  expect(screen.getByText('Provider input')).toBeVisible();
  expect(screen.getByText('Provider output')).toBeVisible();
  expect(screen.getByText('nopsai.get_pipeline')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Hide details' }));
  expect(screen.queryByText('nopsai.get_pipeline')).toBeNull();

  await user.click(screen.getByRole('button', { name: 'Show details' }));
  expect(screen.getByText('nopsai.get_pipeline')).toBeVisible();
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
    usage: emptyAssistantConversationUsage,
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
    usage: emptyAssistantMessageUsage,
    created_at: '2026-06-20T00:00:00Z',
  };
}
