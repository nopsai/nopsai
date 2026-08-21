import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { SetupBootstrapResult, SetupStatusOverview } from './SetupStatusPanels';
import type { BootstrapResponse, SetupStatus } from './model';

const setupStatus: SetupStatus = {
  completed: false,
  counts: {
    users: 2,
    pipelines: 1,
    steps: 3,
    triggers: 4,
    teams: 1,
    access_grants: 2,
    llm_profiles: 1,
    mcp_servers: 0,
    mcp_profiles: 0,
    knowledge_contexts: 1,
    config_repositories: 1,
  },
  checks: [
    {
      id: 'database',
      label: 'Database ready',
      status: 'success',
      message: 'Connected',
      blocking: true,
    },
  ],
  github: {},
};

test('renders setup health checks and resource counts', () => {
  render(<SetupStatusOverview status={setupStatus} />);

  expect(screen.getByText('Database ready')).toBeInTheDocument();
  expect(screen.getByText('Connected')).toBeInTheDocument();
  expect(screen.getByText('access grants')).toBeInTheDocument();
  expect(screen.getAllByText('2').length).toBeGreaterThan(0);
});

test('renders bootstrap result warnings, messages, secrets, and temporary credentials', () => {
  const bootstrapResult: BootstrapResponse = {
    status: setupStatus,
    warnings: ['Restart git-bot after secrets are mounted'],
    messages: ['Setup saved'],
    generated_secrets: ['AAA_SHARED_INTERNAL_TOKEN'],
    temporary_credentials: [{ sub: 'alice', email: 'alice@example.com', temporary_password: 'temporary' }],
  };

  render(<SetupBootstrapResult bootstrapResult={bootstrapResult} />);

  expect(screen.getByText('Restart git-bot after secrets are mounted')).toBeInTheDocument();
  expect(screen.getByText('Setup saved')).toBeInTheDocument();
  expect(screen.getByText('AAA_SHARED_INTERNAL_TOKEN')).toBeInTheDocument();
  expect(screen.getByText(/alice@example.com/)).toBeInTheDocument();
});

