import { render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
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
    error: null,
    teamPath: 'platform',
    gitOpsTarget: 'teams/platform/ai-profiles.yaml',
    ...overrides,
  };

  render(
    <MemoryRouter>
      <TeamAIProfilesPanel {...props} />
    </MemoryRouter>
  );
  return props;
}

describe('TeamAIProfilesPanel', () => {
  it('summarizes team AI profile configuration and links to owning pages', () => {
    renderPanel();

    expect(screen.getByRole('heading', { name: 'Team AI Profiles' })).toBeVisible();
    expect(screen.getByText('GitOps target: teams/platform/ai-profiles.yaml')).toBeVisible();
    expect(screen.getByRole('link', { name: 'LLM Profiles' })).toHaveAttribute('href', '/llm-profiles?team=platform');
    expect(screen.getByRole('link', { name: 'Agent Profiles' })).toHaveAttribute('href', '/agent-profiles?team=platform');
    expect(screen.getByRole('link', { name: 'MCP' })).toHaveAttribute('href', '/mcp/profiles?team=platform');

    const llmSection = screen.getByRole('region', { name: 'LLM profiles' });
    expect(within(llmSection).getByText('fast')).toBeVisible();
    expect(within(llmSection).getByText('openai / gpt-4.1-mini / credential://openai/default')).toBeVisible();
    expect(within(llmSection).getByText('Default')).toBeVisible();

    expect(screen.getByText('Reviewer')).toBeVisible();
    expect(screen.getByText('engineering-tools')).toBeVisible();
    expect(screen.queryByRole('button', { name: /Save/ })).not.toBeInTheDocument();
  });

  it('uses global owner links and copy without a team path', () => {
    renderPanel({
      llmProfiles: { ...llmProfiles, team_id: 0, team_path: '', profiles: [] },
      agentProfiles: { ...agentProfiles, team_id: 0, team_path: '', profiles: [] },
      mcpProfiles: { ...mcpProfiles, team_id: 0, team_path: '', profiles: [] },
      teamPath: '',
      gitOpsTarget: '',
    });

    expect(screen.getByRole('heading', { name: 'Global AI Profiles' })).toBeVisible();
    expect(screen.getByText('Global profile defaults and tool access.')).toBeVisible();
    expect(screen.getByRole('link', { name: 'LLM Profiles' })).toHaveAttribute('href', '/llm-profiles?team=global');
    expect(screen.getByRole('link', { name: 'Agent Profiles' })).toHaveAttribute('href', '/agent-profiles?team=global');
    expect(screen.getByRole('link', { name: 'MCP' })).toHaveAttribute('href', '/mcp/profiles?team=global');
    expect(screen.getByText('No global LLM profiles')).toBeVisible();
    expect(screen.getByText('No global agent profiles')).toBeVisible();
    expect(screen.getByText('No global MCP profiles')).toBeVisible();
  });

  it('shows loading and empty states without edit controls', () => {
    const { rerender } = render(
      <MemoryRouter>
        <TeamAIProfilesPanel
          llmProfiles={null}
          agentProfiles={null}
          mcpProfiles={null}
          loading
          error={null}
          teamPath="platform"
          gitOpsTarget=""
        />
      </MemoryRouter>
    );

    expect(screen.getByText('Loading AI profiles...')).toBeVisible();

    rerender(
      <MemoryRouter>
        <TeamAIProfilesPanel
          llmProfiles={{ ...llmProfiles, profiles: [] }}
          agentProfiles={{ ...agentProfiles, profiles: [] }}
          mcpProfiles={{ ...mcpProfiles, profiles: [] }}
          loading={false}
          error="Unable to load team AI profiles"
          teamPath="platform"
          gitOpsTarget=""
        />
      </MemoryRouter>
    );

    expect(screen.getByText('Unable to load team AI profiles')).toBeVisible();
    expect(screen.getByText('No team LLM profiles')).toBeVisible();
    expect(screen.getByText('No team agent profiles')).toBeVisible();
    expect(screen.getByText('No team MCP profiles')).toBeVisible();
  });
});
