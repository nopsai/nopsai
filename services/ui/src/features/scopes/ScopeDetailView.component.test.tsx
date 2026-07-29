import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import { copyTextToClipboard } from '../../lib/clipboard';
import type { DispatcherStatusState } from '../system/dispatcher/model';
import { ScopeDetailView } from './ScopeDetailView';
import type { ScopeData } from './model';

vi.mock('../../lib/clipboard', () => ({
  copyTextToClipboard: vi.fn(),
}));

const copyTextToClipboardMock = vi.mocked(copyTextToClipboard);

const scopeData: ScopeData = {
  variables: ['DOCKER_HOST', 'acme/api/API_URL'],
  variableMeta: {
    DOCKER_HOST: { source: 'git', createdAt: '2026-06-01T10:00:00Z', updatedAt: '2026-06-02T10:00:00Z' },
    'acme/api/API_URL': { source: 'database' },
  },
  variablesLoaded: true,
  variablesLoading: false,
  secrets: ['GH_TOKEN'],
  secretMeta: {
    GH_TOKEN: { source: 'database', createdAt: '2026-06-01T11:00:00Z', updatedAt: '2026-06-02T11:00:00Z' },
  },
  secretsLoaded: true,
  secretsLoading: false,
};

const runnerStatus: DispatcherStatusState = {
  queuedJobs: 0,
  runners: [
    {
      runnerId: 'general-runner',
      scopes: ['nopsai'],
      capacity: 20,
      activeJobs: 6,
      inflightJobs: 2,
      lastHeartbeatUnix: Math.floor(Date.now() / 1000),
      allowDispatch: true,
      reachable: true,
      connectionStatus: 'online',
      metadata: { runtime: 'docker' },
    },
    {
      runnerId: 'legacy-runner',
      scopes: ['nopsai'],
      capacity: 4,
      activeJobs: 0,
      inflightJobs: 0,
      lastHeartbeatUnix: Math.floor(Date.now() / 1000) - 7200,
      allowDispatch: true,
      reachable: false,
      connectionStatus: 'offline',
      metadata: { runtime: 'shell' },
    },
  ],
  routing: {},
  effectiveRouting: { nopsai: ['general-runner', 'legacy-runner'] },
  fetchedAt: Date.now(),
};

function renderScopeDetail(overrides: Partial<Parameters<typeof ScopeDetailView>[0]> = {}) {
  const props: Parameters<typeof ScopeDetailView>[0] = {
    selectedScope: 'nopsai',
    scopeDataByScope: { nopsai: scopeData },
    selectedVariable: 'DOCKER_HOST',
    selectedSecret: null,
    expandedVariableKey: 'DOCKER_HOST@@nopsai',
    variableValueLoadingKey: null,
    variableValues: { 'DOCKER_HOST@@nopsai': 'tcp://docker:2375' },
    pipelineVariableIndex: new Map([
      ['DOCKER_HOST', new Set(['platform/release'])],
      ['acme/api/API_URL', new Set(['platform/release', 'platform/smoke'])],
    ]),
    pipelineSecretIndex: new Map([['GH_TOKEN', new Set(['platform/release'])]]),
    pipelineMetadata: new Map([
      [
        'platform/release',
        {
          identifier: 'platform/release',
          name: 'release-flow',
          description: 'Enterprise release flow',
          path: 'platform',
          version: 'v2',
          source: 'GitOps',
        },
      ],
    ]),
    triggersByScope: new Map([
      [
        'nopsai',
        [
          {
            slug: 'acme/api',
            scope: 'nopsai',
            pipelines: ['platform/release'],
            event: 'push',
            branches: ['main'],
            tags: [],
          },
        ],
      ],
    ]),
    usageLoading: false,
    usageError: null,
    runnerStatus,
    runnerStatusLoading: false,
    runnerStatusError: null,
    canWriteVariablesInSelectedScope: true,
    canWriteSecretsInSelectedScope: true,
    canDeleteScopes: true,
    onSelectVariable: vi.fn(),
    onSelectSecret: vi.fn(),
    onToggleVariableValue: vi.fn(),
    onCreateVariable: vi.fn(),
    onUpdateVariable: vi.fn(),
    onCloneVariable: vi.fn(),
    onCreateSecret: vi.fn(),
    onUpdateSecret: vi.fn(),
    onCloneSecret: vi.fn(),
    onDeleteValue: vi.fn(),
    onOpenGitOpsEncrypt: vi.fn(),
    onBack: vi.fn(),
    ...overrides,
  };

  return {
    props,
    ...render(
      <MemoryRouter>
        <ScopeDetailView {...props} />
      </MemoryRouter>
    ),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  copyTextToClipboardMock.mockResolvedValue(undefined);
});

test('renders the redesigned searchable variables and secrets workspace', async () => {
  const user = userEvent.setup();
  const onSelectSecret = vi.fn();
  const onCreateVariable = vi.fn();
  const onCreateSecret = vi.fn();

  renderScopeDetail({ onSelectSecret, onCreateVariable, onCreateSecret });

  expect(screen.getByRole('heading', { name: '/nopsai' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Access' })).toHaveClass('scope-detail-action--ghost');
  expect(screen.getByText('Variables and secrets')).toBeVisible();
  expect(screen.getByRole('button', { name: /DOCKER_HOST/i })).toHaveClass('scope-detail-item--active');

  await user.clear(screen.getByLabelText('Search variables and secrets'));
  await user.type(screen.getByLabelText('Search variables and secrets'), 'api');
  expect(screen.getByRole('button', { name: /API_URL/i })).toBeVisible();
  expect(screen.queryByRole('button', { name: /GH_TOKEN/i })).not.toBeInTheDocument();

  await user.clear(screen.getByLabelText('Search variables and secrets'));
  await user.click(screen.getByRole('button', { name: 'Secrets' }));
  expect(screen.getByRole('button', { name: /GH_TOKEN/i })).toBeVisible();
  expect(screen.queryByRole('button', { name: /API_URL/i })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: /GH_TOKEN/i }));
  expect(onSelectSecret).toHaveBeenCalledWith('GH_TOKEN');

  await user.click(screen.getByRole('button', { name: 'New item' }));
  const createMenu = screen.getByRole('menu', { name: 'Create scoped item' });
  await user.click(within(createMenu).getByRole('menuitem', { name: 'Variable' }));
  expect(onCreateVariable).toHaveBeenCalledWith('nopsai');

  await user.click(screen.getByRole('button', { name: 'New item' }));
  await user.click(screen.getByRole('menuitem', { name: 'Secret' }));
  expect(onCreateSecret).toHaveBeenCalledWith('nopsai');
});

test('keeps variable reveal, copy, edit, clone, delete, and usage links wired', async () => {
  const user = userEvent.setup();
  const onToggleVariableValue = vi.fn().mockResolvedValue(undefined);
  const onUpdateVariable = vi.fn();
  const onCloneVariable = vi.fn();
  const onDeleteValue = vi.fn();

  renderScopeDetail({
    onToggleVariableValue,
    onUpdateVariable,
    onCloneVariable,
    onDeleteValue,
  });

  expect(screen.getByText('tcp://docker:2375')).toBeVisible();
  await user.click(screen.getByRole('button', { name: 'Copy value' }));
  expect(copyTextToClipboardMock).toHaveBeenCalledWith('tcp://docker:2375');
  expect(await screen.findByRole('button', { name: 'Copied' })).toBeVisible();

  await user.click(screen.getByRole('button', { name: 'Hide value' }));
  expect(onToggleVariableValue).toHaveBeenCalledWith('nopsai', 'DOCKER_HOST');

  await user.click(screen.getByRole('button', { name: 'Edit database override' }));
  expect(onUpdateVariable).toHaveBeenCalledWith('nopsai', 'DOCKER_HOST');
  await user.click(screen.getByRole('button', { name: 'Clone variable' }));
  expect(onCloneVariable).toHaveBeenCalledWith('nopsai', 'DOCKER_HOST');
  await user.click(screen.getByRole('button', { name: 'Delete variable' }));
  expect(onDeleteValue).toHaveBeenCalledWith('variable', 'nopsai', 'DOCKER_HOST');

  expect(screen.getByRole('link', { name: /release-flow/i })).toHaveAttribute('href', '#/pipelines/platform/release');
  expect(screen.getByRole('link', { name: /acme\/api/i })).toHaveAttribute('href', '#/triggers/acme/api');
});

test('masks secrets while retaining secret mutation actions', async () => {
  const user = userEvent.setup();
  const onUpdateSecret = vi.fn();
  const onCloneSecret = vi.fn();
  const onDeleteValue = vi.fn();

  renderScopeDetail({
    selectedVariable: null,
    selectedSecret: 'GH_TOKEN',
    expandedVariableKey: null,
    onUpdateSecret,
    onCloneSecret,
    onDeleteValue,
  });

  expect(screen.getByText('Secret value is encrypted and never displayed')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Copy value' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Reveal value' })).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Edit secret' }));
  expect(onUpdateSecret).toHaveBeenCalledWith('nopsai', 'GH_TOKEN');
  await user.click(screen.getByRole('button', { name: 'Clone secret' }));
  expect(onCloneSecret).toHaveBeenCalledWith('nopsai', 'GH_TOKEN');
  await user.click(screen.getByRole('button', { name: 'Delete secret' }));
  expect(onDeleteValue).toHaveBeenCalledWith('secret', 'nopsai', 'GH_TOKEN');
});

test('opens runner assignments from the metric card and preserves dispatcher navigation', async () => {
  const user = userEvent.setup();

  renderScopeDetail();

  const runnerMetric = screen.getByRole('button', { name: /Runner assignments/i });
  expect(runnerMetric).toHaveAttribute('aria-expanded', 'false');

  await user.click(runnerMetric);
  expect(runnerMetric).toHaveAttribute('aria-expanded', 'true');
  expect(screen.getByRole('region', { name: 'Runner assignments' })).toBeVisible();
  expect(screen.getByRole('button', { name: /general-runner/i })).toHaveClass('scope-runner-card--active');
  await user.click(screen.getByRole('button', { name: /legacy-runner/i }));
  expect(screen.getByRole('button', { name: /legacy-runner/i })).toHaveClass('scope-runner-card--active');
  expect(screen.getByRole('link', { name: 'Dispatcher' })).toHaveAttribute('href', '/system/dispatcher');

  await user.click(screen.getByRole('button', { name: 'Hide runner assignments' }));
  expect(screen.queryByRole('region', { name: 'Runner assignments' })).not.toBeInTheDocument();
});
