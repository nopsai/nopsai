import { useEffect } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, expect, test, vi } from 'vitest';
import type { TriggerDetail, TriggerListItem } from '../features/triggers/model';

const triggerApi = vi.hoisted(() => ({
  checkTriggerPermission: vi.fn(),
  fetchTriggerAutocompleteResources: vi.fn(),
  fetchTriggerDetail: vi.fn(),
  fetchTriggerPipelineYaml: vi.fn(),
  fetchTriggerRuns: vi.fn(),
  fetchTriggers: vi.fn(),
  saveTrigger: vi.fn(),
  deleteTrigger: vi.fn(),
}));

vi.mock('../features/triggers/api', () => triggerApi);

import TriggersPage from './Triggers';

function LocationRecorder({ onChange }: { onChange: (location: string) => void }) {
  const location = useLocation();
  useEffect(() => {
    onChange(`${location.pathname}${location.search}`);
  }, [location.pathname, location.search, onChange]);
  return null;
}

const triggerList: TriggerListItem[] = [
  { slug: 'platform/checkout', source: 'gitops', teamPath: 'platform' },
  { slug: 'external/billing', source: 'database', teamPath: 'platform' },
];

const triggerDetail: TriggerDetail = {
  slug: 'platform/checkout',
  source: 'gitops',
  teamPath: 'platform',
  rawYaml: [
    'team_path: platform',
    'triggers:',
    '  - on: push',
    '    branches:',
    '      - main',
    '    pipelines:',
    '      - pipelines/platform/deploy.yaml',
    '    scope: production',
    '',
  ].join('\n'),
  summary: {
    triggerCount: 1,
    pipelines: [{ identifier: 'platform/deploy', display: 'deploy', pathLabel: 'platform' }],
    events: ['push'],
    branches: ['main'],
    skipBranches: [],
    tags: [],
    scopes: ['production'],
  },
};

beforeEach(() => {
  triggerApi.checkTriggerPermission.mockResolvedValue(true);
  triggerApi.fetchTriggerAutocompleteResources.mockResolvedValue({ pipelines: [], scopes: [] });
  triggerApi.fetchTriggerDetail.mockResolvedValue(triggerDetail);
  triggerApi.fetchTriggerPipelineYaml.mockResolvedValue(null);
  triggerApi.fetchTriggerRuns.mockResolvedValue([]);
  triggerApi.fetchTriggers.mockResolvedValue(triggerList);
});

test('renders selected triggers as a fullscreen route with persistent tree navigation', async () => {
  render(
    <MemoryRouter initialEntries={['/triggers/platform/checkout']}>
      <Routes>
        <Route path="/triggers/*" element={<TriggersPage canDeleteTriggers />} />
      </Routes>
    </MemoryRouter>
  );

  const detailRegion = await screen.findByLabelText('Trigger detail');
  expect(detailRegion).toHaveClass('triggers-detail-fullscreen', 'triggers-detail-fullscreen--with-tree');
  expect(await screen.findByText('Trigger tree')).toBeVisible();
  expect(screen.getByRole('button', { name: 'Select trigger platform/checkout' })).toHaveClass('active');
  expect(await screen.findByRole('heading', { name: 'checkout' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Overview' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Trigger definition' })).toBeVisible();
  expect(screen.getByRole('heading', { name: 'Recent runs' })).toBeVisible();
  expect(screen.getByRole('button', { name: 'List' })).toBeVisible();
  expect(screen.queryByRole('tablist')).not.toBeInTheDocument();

  await waitFor(() => {
    expect(document.querySelector('.triggers-workspace-panel--summary')).toBeNull();
  });
});

test('uses repository owners with NopsAI teams and migrates legacy team routes', async () => {
  const locations: string[] = [];
  const recordLocation = (location: string) => locations.push(location);

  render(
    <MemoryRouter initialEntries={['/triggers/team/platform']}>
      <Routes>
        <Route
          path="/triggers/*"
          element={(
            <>
              <LocationRecorder onChange={recordLocation} />
              <TriggersPage />
            </>
          )}
        />
      </Routes>
    </MemoryRouter>
  );

  await waitFor(() => {
    expect(locations[locations.length - 1]).toBe('/triggers?team=platform');
  });
  expect(await screen.findByRole('button', { name: /All owners/ })).toBeVisible();
  expect(screen.getByRole('button', { name: 'Open owner platform' })).toHaveClass('active');
  expect(screen.getByRole('columnheader', { name: 'Trigger' })).toBeVisible();
  expect(screen.getByText('external/billing')).toBeVisible();
  expect(screen.queryByRole('columnheader', { name: 'Owner' })).not.toBeInTheDocument();
  expect(screen.queryByText('All teams')).not.toBeInTheDocument();
});
