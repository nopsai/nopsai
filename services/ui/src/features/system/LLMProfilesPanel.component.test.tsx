import { render, screen, waitFor } from '@testing-library/react';
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

vi.mock('./llm-profiles/api', () => apiMocks);

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
    '/system/credentials?credential=credential%3A%2F%2Fsystem%2Fllm%2Fopenai'
  );
  expect(screen.getByText('30s')).toBeVisible();
  expect(screen.getByText('2048 tokens')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /edit profile/i })).toHaveClass('ai-resource-icon-action');

  await user.click(screen.getByRole('button', { name: /test connection/i }));
  await waitFor(() => expect(apiMocks.testLLMProfile).toHaveBeenCalledWith('hosted'));
  expect(await screen.findByText('hosted: ok')).toBeVisible();

  await user.click(screen.getByRole('button', { name: /new profile/i }));
  const provider = screen.getByLabelText('Provider');

  await user.selectOptions(provider, 'openai');
  expect(screen.getByLabelText('Model')).toHaveValue('gpt-4.1-mini');
  expect(screen.getByLabelText('Base URL')).toHaveValue('https://api.openai.com/v1');
  expect(screen.getByLabelText(/Credential reference/)).toHaveValue('credential://system/llm/standard');
  expect(screen.queryByLabelText('Reasoning')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Temperature')).toHaveAttribute('max', '2');
  expect(screen.getByTitle(
    'Controls response randomness: lower values are more predictable, higher values are more varied.'
  )).toHaveTextContent('Temperature');
  expect(screen.getByTitle(
    'Maximum number of tokens the model may generate in its response.'
  )).toHaveTextContent('Max tokens');
  expect(screen.getByTitle(
    'Maximum time to wait for the provider before the request is cancelled.'
  )).toHaveTextContent('Timeout seconds');
  expect(screen.getByLabelText('Timeout seconds').closest('div')).toHaveClass('items-end');
  expect(screen.getByText(/uses max_completion_tokens/i)).toBeVisible();

  await user.selectOptions(provider, 'anthropic');
  expect(screen.getByLabelText('Temperature')).toHaveAttribute('max', '1');
  expect(screen.queryByLabelText('Reasoning')).not.toBeInTheDocument();

  await user.selectOptions(provider, 'ollama');
  expect(screen.getByLabelText('Model')).toHaveValue('qwen2.5-coder:14b');
  expect(screen.getByLabelText('Base URL *')).toHaveValue('http://ollama:11434/v1');
  expect(screen.getByLabelText(/Credential reference/)).toHaveValue('credential://system/llm/standard');

  await user.selectOptions(provider, 'lmstudio');
  expect(screen.getByTitle(
    'Controls how much internal reasoning the model performs before answering.'
  )).toHaveTextContent('Reasoning');
  expect(screen.getByTitle(
    "Turns the provider's extended thinking mode on or off when no reasoning level is selected."
  )).toHaveTextContent('Thinking');
});
