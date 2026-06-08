import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { ScopeUsagePanel } from './ScopeUsagePanel';

test('renders scope impact analysis links for the selected item', () => {
  render(
    <ScopeUsagePanel
      selection={{
        type: 'secret',
        name: 'registry-token',
        pipelines: ['platform/release'],
        meta: { createdAt: '2026-06-08T10:00:00Z', updatedAt: '2026-06-08T11:00:00Z' },
      }}
      pipelineMetadata={
        new Map([
          [
            'platform/release',
            {
              name: 'release',
              description: 'Enterprise release flow',
              path: 'platform',
              version: 'v2',
              source: 'Config Repository',
            },
          ],
        ])
      }
      triggers={[
        {
          slug: 'acme/platform',
          scope: 'production',
          pipelines: ['platform/release'],
          event: 'push',
          branches: ['main'],
          tags: [],
        },
      ]}
      loading={false}
      error={null}
    />
  );

  expect(screen.getByRole('link', { name: /release/i })).toHaveAttribute('href', '#/pipelines/platform/release');
  expect(screen.getByRole('link', { name: /acme\/platform/i })).toHaveAttribute('href', '#/triggers/acme/platform');
  expect(screen.getByText('Enterprise release flow')).toBeVisible();
});
