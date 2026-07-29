import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ScopeCollectionList } from './ScopeCollectionList';
import type { ScopeData, ScopeEntry, ScopeTreeNode } from './model';

const treeRoot: ScopeTreeNode = {
  id: '__root__',
  name: 'All scopes',
  fullPath: '',
  scopes: [''],
  children: [
    {
      id: 'platform',
      name: 'platform',
      fullPath: 'platform',
      scopes: [],
      children: [
        {
          id: 'platform/dev',
          name: 'dev',
          fullPath: 'platform/dev',
          scopes: ['platform/dev'],
          children: [],
        },
      ],
    },
  ],
};

const visibleScopes: ScopeEntry[] = [
  { scope: '', label: 'Default Scope', teamPath: '', description: 'Fallback scope', secretCountHint: 1 },
  { scope: 'platform/dev', label: 'dev', teamPath: 'platform/dev', description: 'Development scope', secretCountHint: 0 },
];

const scopeDataByScope: Record<string, ScopeData> = {
  '': {
    variables: ['REGION'],
    variableMeta: {},
    variablesLoaded: true,
    variablesLoading: false,
    secrets: ['TOKEN'],
    secretMeta: {},
    secretsLoaded: true,
    secretsLoading: false,
  },
  'platform/dev': {
    variables: ['IMAGE_TAG', 'REGION'],
    variableMeta: {},
    variablesLoaded: true,
    variablesLoading: false,
    secrets: ['API_KEY', 'DB_PASSWORD', 'TOKEN'],
    secretMeta: {},
    secretsLoaded: true,
    secretsLoading: false,
  },
};

test('renders scopes in the shared resource collection rail and table design', async () => {
  const user = userEvent.setup();
  const onOpenTeam = vi.fn();
  const onSelectScope = vi.fn();

  render(
    <ScopeCollectionList
      listLoading={false}
      listError={null}
      visibleScopes={visibleScopes}
      treeRoot={treeRoot}
      activeTeam=""
      canCreateScopeHere
      onOpenTeam={onOpenTeam}
      onSelectScope={onSelectScope}
      scopeDataByScope={scopeDataByScope}
    />
  );

  expect(screen.getByRole('complementary', { name: 'Teams' })).toHaveClass('pipeline-runs-scope-rail');
  expect(screen.getByRole('button', { name: /All teams/ })).toHaveClass('pipeline-runs-scope-item--active');
  expect(screen.getByRole('region', { name: 'Scopes' })).toHaveClass('pipeline-runs-panel');
  expect(screen.queryByRole('heading', { name: 'Scopes' })).not.toBeInTheDocument();
  expect(screen.queryByText('2 visible')).not.toBeInTheDocument();
  expect(screen.getByTestId('scopes-resource-table')).toHaveClass('pipeline-runs-table', 'resource-collection-table');
  expect(screen.queryByRole('article')).not.toBeInTheDocument();
  expect(screen.getByText('2 variables')).toBeVisible();
  expect(screen.getByText('3 secrets')).toBeVisible();
  expect(screen.getAllByText('platform/dev').length).toBeGreaterThanOrEqual(2);

  await user.click(screen.getByRole('button', { name: 'Open scope dev' }));
  expect(onSelectScope).toHaveBeenCalledWith('platform/dev');

  await user.click(screen.getByRole('button', { name: 'Expand team platform' }));
  await user.click(screen.getByRole('button', { name: 'Open team platform/dev' }));
  expect(onOpenTeam).toHaveBeenCalledWith('platform/dev');
});

test('renders the shared empty panel state', () => {
  render(
    <ScopeCollectionList
      listLoading={false}
      listError={null}
      visibleScopes={[]}
      treeRoot={treeRoot}
      activeTeam="missing"
      canCreateScopeHere={false}
      onOpenTeam={() => undefined}
      onSelectScope={() => undefined}
      scopeDataByScope={{}}
    />
  );

  expect(screen.getByRole('region', { name: 'Scopes' })).toHaveClass('pipeline-runs-panel');
  expect(screen.getByRole('complementary', { name: 'Teams' })).toBeVisible();
  expect(screen.getByText('No scopes found')).toBeVisible();
  expect(screen.getByText('Adjust your filters or check your access.')).toBeVisible();
});
