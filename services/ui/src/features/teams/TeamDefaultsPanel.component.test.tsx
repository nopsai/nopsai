import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import { TeamDefaultsPanel } from './TeamDefaultsPanel';
import type { TeamAgentProfilesResponse, TeamDefaultsResponse, TeamLLMProfilesResponse } from './api';
import type { KnowledgeContextListItem } from '../knowledge-context/model';

const llmProfiles: TeamLLMProfilesResponse = {
  team_id: 1,
  team_path: 'platform',
  default_profile: 'fast',
  profiles: [
    {
      name: 'fast',
      provider: 'openai',
      model: 'gpt-4.1-mini',
      credential_ref: 'credential://openai/default',
      allowed_scopes: ['pipeline_run', 'assistant'],
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    },
    {
      name: 'careful',
      provider: 'openai',
      model: 'gpt-4.1',
      credential_ref: 'credential://openai/default',
      allowed_scopes: ['assistant'],
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    },
  ],
};

const agentProfiles: TeamAgentProfilesResponse = {
  team_id: 1,
  team_path: 'platform',
  default_profile: 'reviewer',
  profiles: [
    {
      id: 'reviewer',
      display_name: 'Reviewer',
      role: 'review',
      instructions: 'Review pipeline changes',
      enabled: true,
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    },
    {
      id: 'ops',
      display_name: 'Ops Reviewer',
      role: 'ops',
      instructions: 'Review operations.',
      enabled: true,
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    },
  ],
};

const teamDefaults: TeamDefaultsResponse = {
  team_id: 1,
  team_path: 'platform',
  model: 'fast',
  agent_role: 'reviewer',
  knowledge_context: {
    guardrail: 'platform/runtime-safety',
    runbook: 'platform/restart',
  },
};

const knowledgeContexts: KnowledgeContextListItem[] = [
  {
    id: 'platform/runtime-safety',
    kind: 'guardrail',
    team: 'platform',
    name: 'runtime-safety',
    description: 'Runtime output safety',
    visibility: 'team',
    source: 'git',
  },
  {
    id: 'platform/no-env-print',
    kind: 'guardrail',
    team: 'platform',
    name: 'no-env-print',
    description: 'No environment dumping',
    visibility: 'team',
    source: 'database',
  },
  {
    id: 'platform/restart',
    kind: 'runbook',
    team: 'platform',
    name: 'restart',
    description: 'Restart procedure',
    visibility: 'team',
    source: 'git',
  },
];

function renderPanel(overrides: Partial<Parameters<typeof TeamDefaultsPanel>[0]> = {}) {
  const props = {
    llmProfiles,
    agentProfiles,
    teamDefaults,
    knowledgeContexts,
    loading: false,
    error: null,
    teamPath: 'platform',
    ...overrides,
  };

  render(
    <MemoryRouter>
      <TeamDefaultsPanel {...props} />
    </MemoryRouter>
  );
  return props;
}

describe('TeamDefaultsPanel', () => {
  it('renders one selector row per default subject', () => {
    renderPanel();

    expect(screen.getByRole('heading', { name: 'Team Defaults' })).toBeVisible();
    expect(screen.queryByRole('link', { name: 'LLM Profiles' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Agent Profiles' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Knowledge' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'MCP' })).not.toBeInTheDocument();

    expect(screen.getByLabelText('Model')).toHaveValue('fast');
    expect(screen.getByLabelText('Agent role')).toHaveValue('reviewer');
    expect(screen.getByLabelText('Guardrail knowledge')).toHaveValue('platform/runtime-safety');
    expect(screen.getByLabelText('Runbook knowledge')).toHaveValue('platform/restart');
    expect(screen.getByLabelText('Architecture knowledge')).toHaveValue('');
    expect(screen.getAllByRole('combobox')).toHaveLength(10);
  });

  it('saves defaults through the generic defaults payload', async () => {
    const user = userEvent.setup();
    const onSaveDefaults = vi.fn().mockResolvedValue(undefined);
    renderPanel({ canManageDefaults: true, onSaveDefaults });

    await user.selectOptions(screen.getByLabelText('Model'), 'careful');
    await waitFor(() => expect(onSaveDefaults).toHaveBeenCalledWith('platform', { model: 'careful' }));

    await user.selectOptions(screen.getByLabelText('Agent role'), 'ops');
    await waitFor(() => expect(onSaveDefaults).toHaveBeenCalledWith('platform', { agent_role: 'ops' }));

    await user.selectOptions(screen.getByLabelText('Guardrail knowledge'), 'platform/no-env-print');
    await waitFor(() => expect(onSaveDefaults).toHaveBeenCalledWith('platform', { knowledge_context: { guardrail: 'platform/no-env-print' } }));
  });

  it('uses global owner links without edit controls', () => {
    renderPanel({
      llmProfiles: { ...llmProfiles, team_id: 0, team_path: '', profiles: [] },
      agentProfiles: { ...agentProfiles, team_id: 0, team_path: '', profiles: [] },
      teamDefaults: null,
      knowledgeContexts: [],
      teamPath: '',
    });

    expect(screen.getByRole('heading', { name: 'Global Defaults' })).toBeVisible();
    expect(screen.getByLabelText('Model')).toBeDisabled();
  });

  it('shows loading and error states', () => {
    const { rerender } = render(
      <MemoryRouter>
        <TeamDefaultsPanel
          llmProfiles={null}
          agentProfiles={null}
          teamDefaults={null}
          knowledgeContexts={[]}
          loading
          error={null}
          teamPath="platform"
        />
      </MemoryRouter>
    );

    expect(screen.getByText('Loading defaults...')).toBeVisible();

    rerender(
      <MemoryRouter>
        <TeamDefaultsPanel
          llmProfiles={llmProfiles}
          agentProfiles={agentProfiles}
          teamDefaults={teamDefaults}
          knowledgeContexts={knowledgeContexts}
          loading={false}
          error="Unable to load team defaults"
          teamPath="platform"
        />
      </MemoryRouter>
    );

    expect(screen.getByText('Unable to load team defaults')).toBeVisible();
  });
});
