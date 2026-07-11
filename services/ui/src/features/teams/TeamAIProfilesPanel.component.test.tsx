import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { TeamAIProfilesPanel } from './TeamAIProfilesPanel';
import type { TeamAgentProfilesResponse, TeamLLMProfilesResponse, TeamMCPProfilesResponse } from './api';

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
  ],
};

const mcpProfiles: TeamMCPProfilesResponse = {
  team_id: 1,
  team_path: 'platform',
  profiles: [
    {
      name: 'engineering-tools',
      description: 'Team MCP profile',
      enabled: true,
      servers: [{ server: 'github', tools: ['issues'] }],
      allowed_scopes: ['assistant'],
      scope: 'team',
      team_id: 1,
      team_path: 'platform',
    },
  ],
};

function renderPanel(overrides: Partial<Parameters<typeof TeamAIProfilesPanel>[0]> = {}) {
  const props = {
    llmProfiles,
    agentProfiles,
    mcpProfiles,
    loading: false,
    saving: false,
    error: null,
    canManage: true,
    gitOpsTarget: 'teams/platform/ai-profiles.yaml',
    onSaveLLM: vi.fn().mockResolvedValue(undefined),
    onSetDefaultLLM: vi.fn().mockResolvedValue(undefined),
    onDeleteLLM: vi.fn().mockResolvedValue(undefined),
    onSaveAgent: vi.fn().mockResolvedValue(undefined),
    onSetDefaultAgent: vi.fn().mockResolvedValue(undefined),
    onDeleteAgent: vi.fn().mockResolvedValue(undefined),
    onSaveMCP: vi.fn().mockResolvedValue(undefined),
    onDeleteMCP: vi.fn().mockResolvedValue(undefined),
    onCheckDrift: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };

  render(<TeamAIProfilesPanel {...props} />);
  return props;
}

describe('TeamAIProfilesPanel', () => {
  it('saves LLM profiles with trimmed identifiers and normalized payloads', async () => {
    const user = userEvent.setup();
    const props = renderPanel();

    const editor = screen.getByRole('tabpanel', { name: 'AI profiles' });
    await user.clear(within(editor).getByLabelText('Name'));
    await user.type(within(editor).getByLabelText('Name'), '  release-review  ');
    await user.clear(within(editor).getByLabelText('Allowed scopes'));
    await user.type(within(editor).getByLabelText('Allowed scopes'), 'pipeline_run, assistant');
    await user.clear(within(editor).getByLabelText('Timeout seconds'));
    await user.type(within(editor).getByLabelText('Timeout seconds'), '45');
    await user.click(within(editor).getByRole('button', { name: 'Save LLM profile' }));

    expect(props.onSaveLLM).toHaveBeenCalledWith('release-review', expect.objectContaining({
      name: 'release-review',
      allowed_scopes: ['pipeline_run', 'assistant'],
      timeout_seconds: 45,
    }));
  });

  it('switches agent and MCP editors and saves canonical profile payloads', async () => {
    const user = userEvent.setup();
    const props = renderPanel();

    await user.click(screen.getByRole('tab', { name: 'Agents' }));
    await user.click(screen.getByRole('button', { name: 'New' }));
    await user.type(screen.getByLabelText('ID'), '  deploy-agent  ');
    await user.type(screen.getByLabelText('Display name'), 'Deploy Agent');
    await user.type(screen.getByLabelText('Instructions'), '  Ship safely  ');
    await user.click(screen.getByRole('button', { name: 'Save agent profile' }));

    expect(props.onSaveAgent).toHaveBeenCalledWith('deploy-agent', expect.objectContaining({
      id: 'deploy-agent',
      display_name: 'Deploy Agent',
      instructions: 'Ship safely',
      enabled: true,
    }));

    await user.click(screen.getByRole('tab', { name: 'MCP' }));
    await user.click(screen.getByRole('button', { name: 'New' }));
    await user.type(screen.getByLabelText('Name'), '  release-tools  ');
    await user.type(screen.getByLabelText('Allowed scopes'), 'assistant, pipeline_run');
    await user.type(screen.getByLabelText('Servers'), 'github:issues,pulls\nslack:*');
    await user.click(screen.getByRole('button', { name: 'Save MCP profile' }));

    expect(props.onSaveMCP).toHaveBeenCalledWith('release-tools', expect.objectContaining({
      name: 'release-tools',
      allowed_scopes: ['assistant', 'pipeline_run'],
      servers: [
        { server: 'github', tools: ['issues', 'pulls'] },
        { server: 'slack', tools: ['*'] },
      ],
    }));
  });
});
