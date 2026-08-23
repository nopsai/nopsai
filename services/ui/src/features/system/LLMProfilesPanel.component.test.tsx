import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
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
  compareResourceTreeNodes: (left: { name: string }, right: { name: string }) => left.name.localeCompare(right.name),
  fetchResourceTeamPaths: vi.fn(async () => ['platform/ml']),
  isGlobalResourceTeamPath: (path?: string | null) => String(path || '').trim().replace(/^\/+|\/+$/g, '').toLowerCase() === 'global',
}));
const teamProfileMocks = vi.hoisted(() => {
  const teamProfiles = (teamPath = 'platform/ml', defaultProfile = 'reasoning') => ({
    team_id: 7,
    team_path: teamPath,
    default_profile: defaultProfile,
    profiles: [
      {
        name: 'reasoning',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://team/platform/ml/llm/openai',
        allowed_scopes: ['prod'],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
      {
        name: 'fast',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://team/platform/ml/llm/openai',
        allowed_scopes: [],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
    ],
  });
  return {
    deleteTeamLLMProfile: vi.fn(),
    fetchTeamLLMProfiles: vi.fn(async (teamPath: string) => teamProfiles(teamPath)),
    requestTeamsJson: vi.fn(async () => ({ allowed: true })),
    setTeamDefaultLLMProfile: vi.fn(async (teamPath: string, defaultProfile: string) => teamProfiles(teamPath, defaultProfile)),
    upsertTeamLLMProfile: vi.fn(async (teamPath: string, profileName: string, payload: Record<string, unknown>) => ({
      team_id: 7,
      team_path: teamPath,
      default_profile: profileName,
      profiles: [{ ...payload, name: profileName, status: 'valid' }],
    })),
  };
});

vi.mock('./models/api', () => apiMocks);
vi.mock('./teamProfileApi', () => teamProfileMocks);
vi.mock('../../lib/resourceTeams', () => teamMocks);

beforeEach(() => {
  vi.clearAllMocks();
});

test('renders provider labels and applies provider-aware profile defaults', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  expect((await screen.findAllByText('OpenAI / ChatGPT'))[0]).toBeVisible();
  expect(screen.getByRole('heading', { name: 'LLM Profiles' })).toHaveClass('sr-only');
  expect(document.getElementById('system-models-section')).toHaveClass('ai-resource-page');
  expect(screen.getByLabelText('LLM profile workspace')).toHaveClass('ai-resource-workspace-card');
  expect(screen.getByLabelText('LLM profile tree')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Select LLM profile hosted' })).toBeVisible();
  expect(screen.queryByLabelText('LLM profile detail')).not.toBeInTheDocument();
  expect(screen.queryByLabelText('Resource summary')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Search LLM profiles' }).closest('.ai-resource-table-controls')).toBe(
    screen.getByRole('button', { name: /new profile/i }).closest('.ai-resource-table-controls')
  );
  expect(screen.queryByText(/^Profiles$/)).not.toBeInTheDocument();
  expect(screen.queryByText('Credentials')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Select LLM profile hosted' }));
  expect(screen.getByLabelText('LLM profile detail')).toHaveClass('ai-resource-detail-fullscreen-main');
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  expect(screen.getByRole('link', { name: 'credential://system/llm/openai' })).toHaveAttribute(
    'href',
    '/credentials/system/llm/openai'
  );
  expect(screen.getByText('30s')).toBeVisible();
  expect(screen.getByText('2048 tokens')).toBeVisible();
  expect(screen.queryByRole('button', { name: /more actions/i })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('ai-resource-icon-action');
  expect(screen.getByRole('button', { name: /edit profile/i })).toHaveClass('ai-resource-icon-action');
  const llmDetailPanel = screen.getByLabelText('LLM profile detail');
  expect(within(llmDetailPanel).getByRole('button', { name: 'Delete profile' }).closest('.ai-resource-detail__actions')).toBeTruthy();
  expect(llmDetailPanel.querySelector('.ai-resource-detail__footer button')).toBeNull();
  expect(screen.getAllByText('Global')[0]).toBeVisible();

  await user.click(screen.getByRole('button', { name: /test connection/i }));
  await waitFor(() => expect(apiMocks.testLLMProfile).toHaveBeenCalledWith('hosted'));
  expect(await screen.findByText('hosted: ok')).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'List' }));
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
  expect(form.getByText('Expected type: api_key')).toBeVisible();
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

  expect(form.getByLabelText('Prompt cache')).toHaveValue('auto');
  expect(form.getByLabelText('Provider state')).toHaveValue('auto');
  await user.selectOptions(form.getByLabelText('Prompt cache'), 'required');
  await user.selectOptions(form.getByLabelText('Provider state'), 'disabled');
  await user.click(form.getByRole('button', { name: 'Save profile' }));
  await waitFor(() => expect(teamProfileMocks.upsertTeamLLMProfile).toHaveBeenCalledWith(
    'platform/ml',
    'reasoning',
    expect.objectContaining({
      name: 'reasoning',
      prompt_cache: { mode: 'required' },
      provider_state: { mode: 'disabled' },
    })
  ));
  await waitFor(() => expect(screen.queryByRole('heading', { name: 'New LLM profile' })).not.toBeInTheDocument());
  const detailPanel = screen.getByLabelText('LLM profile detail');
  expect(within(detailPanel).getByRole('heading', { name: 'platform/ml/reasoning' })).toBeVisible();
});

test('shows scoped catalog LLM profiles for the selected team and saves them as team defaults', async () => {
  apiMocks.fetchLLMProfiles.mockResolvedValueOnce({
    default_profile: 'hosted',
    profiles: [
      {
        name: 'hosted',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://system/llm/openai',
        allowed_scopes: [],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
      {
        name: 'platform/ml/catalog-chat',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://system/llm/openai',
        allowed_scopes: [],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
      },
    ],
  });
  teamProfileMocks.fetchTeamLLMProfiles.mockResolvedValueOnce({
    team_id: 7,
    team_path: 'platform/ml',
    default_profile: '',
    profiles: [],
  });

  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/models?team=platform%2Fml']}>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  const profileList = await screen.findByLabelText('LLM profiles');
  expect(within(profileList).getByText('platform/ml/catalog-chat')).toBeVisible();
  expect(within(profileList).queryByText('hosted')).not.toBeInTheDocument();

  const defaultSelect = screen.getByLabelText('Default LLM profile');
  await waitFor(() => expect(defaultSelect).toHaveValue(''));
  await user.selectOptions(defaultSelect, 'platform/ml/catalog-chat');

  await waitFor(() => expect(teamProfileMocks.setTeamDefaultLLMProfile).toHaveBeenCalledWith('platform/ml', 'platform/ml/catalog-chat'));
  expect(apiMocks.saveDefaultLLMProfile).not.toHaveBeenCalled();
});

test('updates the selected team LLM default through the team API', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/models?team=platform%2Fml']}>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  const defaultSelect = await screen.findByLabelText('Default LLM profile');
  await waitFor(() => expect(defaultSelect).toHaveValue('platform/ml/reasoning'));
  await user.selectOptions(defaultSelect, 'platform/ml/fast');

  await waitFor(() => expect(teamProfileMocks.setTeamDefaultLLMProfile).toHaveBeenCalledWith('platform/ml', 'fast'));
  expect(apiMocks.saveDefaultLLMProfile).not.toHaveBeenCalled();
});

test('moves an edited team LLM profile to the global catalog', async () => {
  apiMocks.saveLLMProfile.mockResolvedValueOnce({
    name: 'reasoning',
    payload: {
      default_profile: 'hosted',
      profiles: [
        {
          name: 'reasoning',
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
    },
  });
  const user = userEvent.setup();

  render(
    <MemoryRouter initialEntries={['/models?team=platform%2Fml']}>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  const profileList = await screen.findByLabelText('LLM profiles');
  await user.click(within(profileList).getByRole('button', { name: 'Select LLM profile reasoning' }));
  await user.click(screen.getByRole('button', { name: /edit profile/i }));

  expect(screen.getByLabelText('Team placement')).toHaveValue('platform/ml');
  expect(screen.getByLabelText('Name')).toHaveValue('reasoning');
  await user.selectOptions(screen.getByLabelText('Team placement'), '');
  expect(screen.getByLabelText('Name')).toHaveValue('reasoning');
  await user.click(screen.getByRole('button', { name: 'Save profile' }));

  await waitFor(() => expect(apiMocks.saveLLMProfile).toHaveBeenCalledWith(expect.objectContaining({ name: 'reasoning' })));
  expect(teamProfileMocks.deleteTeamLLMProfile).toHaveBeenCalledWith('platform/ml', 'reasoning');
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Save profile' })).not.toBeInTheDocument());
  const detailPanel = screen.getByLabelText('LLM profile detail');
  expect(within(detailPanel).getByRole('heading', { name: 'reasoning' })).toBeVisible();
});

test('applies the team filter from the route query', async () => {
  render(
    <MemoryRouter initialEntries={['/models?team=platform%2Fml']}>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  expect(await screen.findByRole('button', { name: 'Open team platform/ml' })).toHaveClass('active');
  await waitFor(() => expect(teamProfileMocks.fetchTeamLLMProfiles).toHaveBeenCalledWith('platform/ml'));
  const profileList = screen.getByLabelText('LLM profiles');
  expect(within(profileList).getByText('platform/ml/reasoning')).toBeVisible();
  expect(within(profileList).queryByText('hosted')).not.toBeInTheDocument();
});

test('counts cached team-owned LLM profiles in the tree', async () => {
  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  await user.click(await screen.findByRole('button', { name: 'Expand platform' }));
  const teamButton = await screen.findByRole('button', { name: 'Open team platform/ml' });
  await waitFor(() => expect(teamProfileMocks.fetchTeamLLMProfiles).toHaveBeenCalledWith('platform/ml'));
  await waitFor(() => expect(within(teamButton).getByText('2')).toBeVisible());
});

test('sends a team model rate card through the team profile API', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const user = userEvent.setup();
  render(
    <MemoryRouter initialEntries={['/models?team=platform%2Fml']}>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  const profileList = await screen.findByLabelText('LLM profiles');
  // The team profile fetch re-renders the panel when it resolves and resets the
  // edit form with it. Interacting before it settles loses the typed rate card,
  // so wait for the load to finish rather than racing it.
  await waitFor(() => expect(teamProfileMocks.fetchTeamLLMProfiles).toHaveBeenCalledWith('platform/ml'));

  await user.click(within(profileList).getByRole('button', { name: 'Select LLM profile reasoning' }));
  await user.click(screen.getByRole('button', { name: /edit profile/i }));
  await screen.findByLabelText('Input');
  // fireEvent.change sets the rate card in one event. user.type commits one
  // keystroke at a time, and a re-render landing between keystrokes drops the
  // digit and submits a zeroed rate card.
  fireEvent.change(screen.getByLabelText('Input'), { target: { value: '3' } });
  fireEvent.change(screen.getByLabelText('Output'), { target: { value: '15' } });
  // The rate card inputs mount while the edit form is still settling, so a
  // keystroke can land on an element that is replaced before it commits.
  // Assert the values are actually held before saving, otherwise the test
  // races the re-render and submits a zeroed rate card.
  await waitFor(() => expect(screen.getByLabelText('Input')).toHaveValue(3));
  await waitFor(() => expect(screen.getByLabelText('Output')).toHaveValue(15));
  await user.click(screen.getByRole('button', { name: /save profile/i }));

  await waitFor(() => expect(teamProfileMocks.upsertTeamLLMProfile).toHaveBeenCalledWith(
    'platform/ml',
    'reasoning',
    expect.objectContaining({ pricing: { input_per_million_usd: 3, output_per_million_usd: 15 } })
  ));
});

test('edits a model rate card instead of silently dropping it on save', async () => {
  Element.prototype.scrollIntoView = vi.fn();
  apiMocks.fetchLLMProfiles.mockResolvedValueOnce({
    default_profile: 'hosted',
    profiles: [
      {
        name: 'hosted',
        provider: 'openai',
        model: 'gpt-4.1-mini',
        base_url: 'https://api.openai.com/v1',
        credential_ref: 'credential://system/llm/openai',
        allowed_scopes: [],
        reasoning: '',
        timeout_seconds: 30,
        max_tokens: 2048,
        extra: {},
        status: 'valid',
        pricing: { input_per_million_usd: 1.25, output_per_million_usd: 10 },
      },
    ],
  });
  apiMocks.saveLLMProfile.mockResolvedValueOnce({ name: 'hosted', payload: { default_profile: 'hosted', profiles: [] } });

  const user = userEvent.setup();
  render(
    <MemoryRouter>
      <LLMProfilesPanel canManage />
    </MemoryRouter>
  );

  await user.click(await screen.findByRole('button', { name: 'Select LLM profile hosted' }));
  expect(screen.getByText('in $1.25/M · out $10/M')).toBeVisible();

  await user.click(screen.getByRole('button', { name: /edit profile/i }));
  const inputRate = screen.getByLabelText('Input') as HTMLInputElement;
  const outputRate = screen.getByLabelText('Output') as HTMLInputElement;
  expect(inputRate.value).toBe('1.25');
  expect(outputRate.value).toBe('10');

  await user.clear(outputRate);
  await user.type(outputRate, '9');
  await user.click(screen.getByRole('button', { name: /save profile/i }));

  await waitFor(() => expect(apiMocks.saveLLMProfile).toHaveBeenCalled());
  const saved = apiMocks.saveLLMProfile.mock.calls[0]?.[0] as { pricing_input: string; pricing_output: string };
  expect(saved.pricing_input).toBe('1.25');
  expect(saved.pricing_output).toBe('9');
});
