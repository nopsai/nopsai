import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { StepUsagePanel } from './StepUsagePanel';

test('renders pipeline usage links with normalized source labels', () => {
  render(
    <MemoryRouter>
      <StepUsagePanel
        usage={[{
          identifier: 'platform/release',
          name: 'release',
          path: 'platform',
          description: 'Release pipeline',
          source: 'GitOps',
        }]}
        loading={false}
        error={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByRole('link', { name: 'Open pipeline platform/release' })).toHaveAttribute(
    'href',
    '/pipelines/platform/release'
  );
  expect(screen.getByRole('table', { name: 'Step usage' })).toBeVisible();
  expect(screen.getByText('GitOps')).toBeInTheDocument();
});
