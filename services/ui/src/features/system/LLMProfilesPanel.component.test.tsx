import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { expect, test, vi } from 'vitest';
import LLMProfilesPanel from './LLMProfilesPanel';

const apiMocks = vi.hoisted(() => ({
  deleteLLMProfile: vi.fn(),
  fetchLLMProfiles: vi.fn(async () => ({
    default_profile: 'hosted',
    profiles: [
      {
        name: 'hosted',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://system/llm/openai',
        allowed_scopes: ['prod'],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
    ],
  })),
  saveDefaultLLMProfile: vi.fn(),
  saveLLMProfile: vi.fn(),
  testLLMProfile: vi.fn(async () => 'ok'),
}));
const teamMocks = vi.hoisted(() => ({
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
}));

vi.mock('./llm-profiles/api', () => apiMocks);
vi.mock('../../lib/resourceTeams', () => teamMocks);

test('renders provider labels and applies provider-aware profile defaults', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  expect((await screen.findAllByText('OpenAI / ChatGPT'))[0]).toBeVisible();
  expect(screen.getByRole('link', { name: 'credential://system/llm/openai' })).toHaveAttribute(
    'href',
    '/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai'
  );
  expect(screen.getByText('30s')).toBeVisible();
  expect(screen.getByText('2048 tokens')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /edit profile/i })).toHaveClass('ai-resource-icon-action');
  expect(screen.getAllByText('Global')[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: /test connection/i }));
  await waitFor(() => expect(apiMocks.testLLMProfile).toHaveBeenCalledWith('hosted'));
  expect(await screen.findByText('hosted: ok')).toBeVisible();

  await user.click(screen.getByRole('button', { name: /new profile/i }));
  const profileForm = screen.getByRole('heading', { name: 'New LLM profile' }).closest('section') as HTMLElement;
  const form = within(profileForm);
  await form.findByRole('option', { name: '/platform/ml' });
  await user.selectOptions(form.getByLabelText('Team placement'), 'platform/ml');
  await user.clear(form.getByLabelText('Name'));
  await user.type(form.getByLabelText('Name'), 'reasoning');
  expect(form.getByText('platform/ml/reasoning')).toBeVisible();

  const provider = form.getByLabelText('Provider');

  await user.selectOptions(provider, 'openai');
  expect(form.getByLabelText('Model')).toHaveValue('gpt-4.1-mini');
  expect(form.getByLabelText('Base URL')).toHaveValue('https://api.openai.com/v1');
  expect(form.getByLabelText(/Credential reference/)).toHaveValue('credential://system/llm/standard');
  expect(form.queryByLabelText('Reasoning')).not.toBeInTheDocument();
  expect(form.getByLabelText('Temperature')).toHaveAttribute('max', '2');
  expect(form.getByTitle(
    'Controls response randomness: lower values are more predictable, higher values are more varied.'
  )).toHaveTextContent('Temperature');
  expect(form.getByTitle(
    'Maximum number of tokens the model may generate in its response.'
  )).toHaveTextContent('Max tokens');
  expect(form.getByTitle(
    'Maximum time to wait for the provider before the request is cancelled.'
  )).toHaveTextContent('Timeout seconds');
  expect(form.getByLabelText('Timeout seconds').closest('div')).toHaveClass('items-end');
  expect(form.getByText(/uses max_completion_tokens/i)).toBeVisible();

  await user.selectOptions(provider, 'anthropic');
  expect(form.getByLabelText('Temperature')).toHaveAttribute('max', '1');
  expect(form.queryByLabelText('Reasoning')).not.toBeInTheDocument();

  await user.selectOptions(provider, 'ollama');
  expect(form.getByLabelText('Model')).toHaveValue('qwen2.5-coder:14b');
  expect(form.getByLabelText('Base URL *')).toHaveValue('http://ollama:11434/v1');
  expect(form.getByLabelText(/Credential reference/)).toHaveValue('credential://system/llm/standard');

  await user.selectOptions(provider, 'lmstudio');
  expect(form.getByTitle(
    'Controls how much internal reasoning the model performs before answering.'
  )).toHaveTextContent('Reasoning');
  expect(form.getByTitle(
    "Turns the provider's extended thinking mode on or off when no reasoning level is selected."
  )).toHaveTextContent('Thinking');
});

test('applies the team filter from the route query', async () => {
  apiMocks.fetchLLMProfiles.mockResolvedValueOnce({
    default_profile: 'platform/ml/reasoning',
    profiles: [
      {
        name: 'hosted',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://system/llm/openai',
        allowed_scopes: ['prod'],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
      {
        name: 'platform/ml/reasoning',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://system/llm/openai',
        allowed_scopes: ['prod'],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
    ],
  });

  render(
    <MemoryRouter initialEntries={['/llm-profiles?team=platform%2Fml']}>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByLabelText('Filter by team')).toHaveValue('platform/ml');
  const profileList = screen.getByLabelText('LLM profiles');
  expect(within(profileList).getByText('platform/ml/reasoning')).toBeVisible();
  expect(within(profileList).queryByText('hosted')).not.toBeInTheDocument();
});
