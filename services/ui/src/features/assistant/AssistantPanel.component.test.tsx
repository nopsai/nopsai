import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, expect, test, vi } from 'vitest';
import { AssistantPanel } from './AssistantPanel';
import {
  emptyAssistantConversationUsage,
  emptyAssistantMemory,
  emptyAssistantMessageUsage,
  type AssistantConfig,
  type AssistantConversation,
  type AssistantMessage,
  type AssistantToolActivity,
} from './model';
import { useAssistantController } from './useAssistantController';

vi.mock('./useAssistantController', () => ({
  useAssistantController: vi.fn(),
}));

const useAssistantControllerMock = vi.mocked(useAssistantController);
type AssistantControllerState = ReturnType<typeof useAssistantController>;

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

test('renders a clean transcript with per-message actions and no evidence or memory panels', async () => {
  const user = userEvent.setup();
  const copyMessage = vi.fn();
  const retryMessage = vi.fn();
  const messages = [
    assistantMessage('m1', 'user', 'show pipeline'),
    {
      ...assistantMessage('m2', 'assistant', 'Pipeline loaded.'),
      tool_calls: [
        toolActivity('nopsai.llm.plan', 'success'),
        {
          ...toolActivity('nopsai.assistant.execution_plan', 'success'),
          output: {
            execution_plan: {
              goal: 'Show pipeline',
              intent: 'llm_planned',
              summary: 'Use MCP first, then synthesize a concise answer.',
              requires_confirmation: false,
              steps: [
                { index: 1, title: 'Read pipeline metadata', source: 'mcp', phase: 'evidence', confidence: 'high', status: 'planned' },
                { index: 2, title: 'Synthesize the answer', source: 'llm', phase: 'synthesis', confidence: 'medium', status: 'planned' },
              ],
            },
          },
        },
        { ...toolActivity('nopsai.get_pipeline', 'success'), resource_uris: ['nopsai://pipelines'] },
        toolActivity('nopsai.llm.complete', 'success'),
      ],
    },
  ];
  const conversation = assistantConversation(messages);

  mockController({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    copyMessage,
    retryMessage,
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Pipeline loaded.')).toBeVisible();
  expect(screen.getByText('Execution plan')).toBeVisible();
  const mcpPlanStep = screen.getByText('Read pipeline metadata').closest('li');
  expect(within(mcpPlanStep as HTMLElement).getByText('MCP')).toBeVisible();
  expect(screen.getByText('Synthesize the answer')).toBeVisible();

  // The removed side panel took memory, evidence and proposed changes with it.
  expect(screen.queryByText('Memory')).toBeNull();
  expect(screen.queryByText(/NopsAI evidence/)).toBeNull();
  expect(screen.queryByText(/Proposed changes/)).toBeNull();
  expect(screen.queryByText('Session details')).toBeNull();
  expect(screen.queryByText(/nopsai\.get_pipeline/)).toBeNull();
  expect(screen.queryByText(/nopsai:\/\/pipelines/)).toBeNull();

  await user.click(screen.getAllByRole('button', { name: 'Copy message' })[1]);
  expect(copyMessage).toHaveBeenCalledWith(messages[1]);

  await user.click(screen.getByRole('button', { name: 'Retry this prompt' }));
  expect(retryMessage).toHaveBeenCalledWith(messages[0]);
});

test('renders welcome starters, the page context chip and the review footnote', async () => {
  const user = userEvent.setup();
  const setDraft = vi.fn();

  mockController({ setDraft });

  render(
    <AssistantPanel
      variant="dock"
      pageContext={{
        title: 'Pipelines',
        path: '/pipelines/platform/deploy',
        route: '/pipelines/:pipeline_id',
        area: 'pipelines',
        resource_type: 'pipeline',
        resource_id: 'platform/deploy',
        pipeline_id: 'platform/deploy',
        scope: 'platform',
      }}
    />
  );

  expect(screen.getByText("Hi, I'm NopsAI. What are we solving today?")).toBeVisible();
  expect(screen.getByText('Context')).toBeVisible();
  expect(screen.getByText('Pipelines · deploy · /platform')).toBeVisible();
  expect(screen.getByPlaceholderText('Message NopsAI...')).toHaveClass('resize-none');
  await user.click(screen.getByRole('button', { name: 'Explain a failed run' }));
  expect(setDraft).toHaveBeenCalledWith('Explain a failed run');
  expect(screen.getByText(/changes always need your review/i)).toBeVisible();
});

test('lets users remove page context from the composer', async () => {
  const user = userEvent.setup();
  const pageContext = {
    title: 'Pipelines',
    path: '/pipelines/platform/deploy',
    route: '/pipelines/:pipeline_id',
    area: 'pipelines',
    resource_type: 'pipeline',
    resource_id: 'platform/deploy',
    pipeline_id: 'platform/deploy',
    scope: 'platform',
  };

  mockController({});

  render(<AssistantPanel variant="dock" pageContext={pageContext} />);

  expect(useAssistantControllerMock).toHaveBeenLastCalledWith({ startFresh: false, pageContext });
  expect(screen.getByText('Context')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Remove page context' }));

  expect(screen.queryByText('Context')).toBeNull();
  expect(useAssistantControllerMock).toHaveBeenLastCalledWith({ startFresh: false, pageContext: null });
});

test('sends the draft and attaches a text file into it', async () => {
  const user = userEvent.setup();
  const setDraft = vi.fn();
  const submitMessage = vi.fn();

  mockController({ draft: 'why is this failing?', setDraft, submitMessage });

  render(<AssistantPanel variant="dock" />);

  await user.upload(
    screen.getByLabelText('Attach a text file'),
    new File(['boom: exit code 1'], 'run.log', { type: 'text/plain' })
  );
  await waitFor(() => expect(setDraft).toHaveBeenCalledWith(
    'why is this failing?\n\nAttached file run.log:\n```\nboom: exit code 1\n```'
  ));

  await user.click(screen.getByRole('button', { name: 'Send message' }));
  expect(submitMessage).toHaveBeenCalled();
});

test('prefills the composer once when handed a draft from another surface', () => {
  const setDraft = vi.fn();
  mockController({ setDraft });

  const { rerender } = render(<AssistantPanel variant="dock" initialDraft="Walk me through the analysis of team Platform." />);

  expect(setDraft).toHaveBeenCalledWith('Walk me through the analysis of team Platform.');
  setDraft.mockClear();
  rerender(<AssistantPanel variant="dock" initialDraft="Walk me through the analysis of team Platform." />);
  expect(setDraft).not.toHaveBeenCalled();
});

test('offers the analysis next step as a one-click follow-up', async () => {
  const user = userEvent.setup();
  const setDraft = vi.fn();
  const messages = [
    assistantMessage('m1', 'user', 'how is the platform team doing?'),
    {
      ...assistantMessage('m2', 'assistant', 'Platform scores 60/100 over the last 30 days.'),
      tool_calls: [
        {
          ...toolActivity('nopsai.analyze_team', 'success'),
          output: {
            health_score: 60,
            next_actions: [
              { label: 'Analyse platform/deploy-api, the least reliable pipeline in this window', tool: 'nopsai.analyze_pipeline' },
            ],
          },
        },
      ],
    },
  ];
  const conversation = assistantConversation(messages);

  mockController({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    setDraft,
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Suggested next step')).toBeVisible();
  await user.click(screen.getByRole('button', { name: /least reliable pipeline/ }));
  expect(setDraft).toHaveBeenCalledWith('Analyse platform/deploy-api, the least reliable pipeline in this window');
});

test('groups conversations by recency and keeps idle conversations deletable during a send', async () => {
  const user = userEvent.setup();
  const deleteConversation = vi.fn();
  const selectConversation = vi.fn();
  const running = assistantConversation([assistantMessage('m1', 'user', 'Analyze AI usage cost by provider')]);
  const lastWeek = {
    ...assistantConversation([]),
    id: 'c2',
    title: 'Previous chat',
    created_at: daysAgoISO(4),
    updated_at: daysAgoISO(4),
  };

  mockController({
    conversations: [running, lastWeek],
    activeConversation: running,
    activeMessages: running.messages,
    sending: true,
    sendingConversationID: 'c1',
    activeConversationSending: true,
    activeConversationSendingStartedAt: Date.now(),
    deleteConversation,
    selectConversation,
  });

  render(<AssistantPanel />);

  const rail = within(screen.getByLabelText('Assistant conversations'));
  expect(rail.getByText('Today')).toBeVisible();
  expect(rail.getByText('Previous 7 days')).toBeVisible();
  expect(rail.queryByLabelText('LLM profile', { selector: 'select' })).toBeNull();

  expect(screen.getByRole('button', { name: 'Delete conversation Pipeline' })).toBeDisabled();
  const idleDelete = screen.getByRole('button', { name: 'Delete conversation Previous chat' });
  expect(idleDelete).toBeEnabled();
  await user.click(idleDelete);
  expect(deleteConversation).toHaveBeenCalledWith('c2');

  await user.click(rail.getByRole('button', { name: 'Previous chat' }));
  expect(selectConversation).toHaveBeenCalledWith('c2');
});

test('renders staged progress while the active conversation is sending', () => {
  const messages = [assistantMessage('m1', 'user', 'Analyze AI usage cost by provider')];
  const conversation = assistantConversation(messages);

  mockController({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    sending: true,
    sendingConversationID: 'c1',
    activeConversationSending: true,
    activeConversationSendingStartedAt: Date.now(),
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Working through the request')).toBeVisible();
  expect(screen.getByText('Plan the request with current permissions')).toBeVisible();
  expect(screen.getByText('Read AI usage, profile, and cost evidence')).toBeVisible();
  expect(screen.getByText('Synthesize an evidence-backed answer')).toBeVisible();
});

test('surfaces a provider failure as a card with the raw reason and a retry', async () => {
  const user = userEvent.setup();
  const retryMessage = vi.fn();
  const detail = 'failed to discover lm studio models: Get "http://172.16.205.64:1234/api/v1/models": dial tcp 172.16.205.64:1234: i/o timeout';
  const messages = [
    assistantMessage('m1', 'user', 'make this pipeline faster'),
    {
      ...assistantMessage(
        'm2',
        'assistant',
        `I could not create a validated NopsAI tool plan for that request because the assistant LLM planner was unavailable or returned an invalid plan: ${detail}. No changes were applied.`
      ),
      tool_calls: [{ ...toolActivity('nopsai.llm.plan', 'error'), output: { fallback_reason: detail } }],
    },
  ];
  const conversation = assistantConversation(messages);

  mockController({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    retryMessage,
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Connection timeout')).toBeVisible();
  expect(screen.getByText(new RegExp('dial tcp 172.16.205.64:1234'))).toBeVisible();
  // The reason lives in the card, so the prose above it stays readable.
  expect(screen.getByText(/returned an invalid plan\. No changes were applied\./)).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Retry request' }));
  expect(retryMessage).toHaveBeenCalledWith(messages[0]);
});

test('shows a send failure card that retries the last prompt', async () => {
  const user = userEvent.setup();
  const retryLastUserMessage = vi.fn();
  const messages = [assistantMessage('m1', 'user', 'make this pipeline faster')];
  const conversation = assistantConversation(messages);

  mockController({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    error: 'Failed to send message (429): {"error":{"code":429,"status":"RESOURCE_EXHAUSTED"}}',
    canRetry: true,
    retryLastUserMessage,
  });

  render(<AssistantPanel variant="dock" />);

  expect(screen.getByText('Rate limit or quota exceeded')).toBeVisible();
  expect(screen.getByText(/"status": "RESOURCE_EXHAUSTED"/)).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Retry request' }));
  expect(retryLastUserMessage).toHaveBeenCalled();
});

test('reports conversation spend and switches the model profile', async () => {
  const user = userEvent.setup();
  const setSelectedProfile = vi.fn();
  const messages = [
    assistantMessage('m1', 'user', 'show summary'),
    {
      ...assistantMessage('m2', 'assistant', '## Summary\n- Pipeline `deploy-api` is healthy.\n\n```yaml\nname: deploy-api\n```'),
      usage: { cost_usd: 0.04, estimated: false, duration_ms: 1250, llm_calls: 2 },
    },
  ];
  const conversation = {
    ...assistantConversation(messages),
    usage: { message_count: 2, spend_usd: 0.04, unpriced_turns: 1, duration_ms: 1250, llm_calls: 2 },
  };

  mockController({
    conversations: [conversation],
    activeConversation: conversation,
    activeMessages: messages,
    profileOptions: ['assistant', 'local-lab'],
    setSelectedProfile,
  });

  render(<AssistantPanel />);

  expect(screen.getByRole('heading', { name: 'Summary' })).toBeVisible();
  expect(screen.getAllByText('deploy-api').length).toBeGreaterThan(0);
  // Usage is reported as the one number tied to money, not as a token split.
  expect(screen.getByText('$0.04')).toBeVisible();
  expect(screen.getByTitle(/\$0\.04 · 2 messages · 1\.3s · 1 turn not priced/)).toBeVisible();
  expect(screen.queryByText(/tokens/i)).toBeNull();

  await user.selectOptions(screen.getByLabelText('Model'), 'local-lab');
  expect(setSelectedProfile).toHaveBeenCalledWith('local-lab');
});

function mockController(overrides: Partial<AssistantControllerState>) {
  useAssistantControllerMock.mockReturnValue({
    conversations: [],
    activeConversation: null,
    activeMessages: [],
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
    ...overrides,
  });
}

function daysAgoISO(days: number): string {
  const startOfToday = new Date();
  startOfToday.setHours(0, 0, 0, 0);
  return new Date(startOfToday.getTime() - days * 24 * 60 * 60 * 1000 + 6 * 60 * 60 * 1000).toISOString();
}

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
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
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

function toolActivity(name: string, status: string): AssistantToolActivity {
  return { name, input: {}, output: {}, status, resource_uris: [] };
}
