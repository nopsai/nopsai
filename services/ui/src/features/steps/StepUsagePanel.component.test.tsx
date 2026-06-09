import { MemoryRouter } from 'react-router-dom';
import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { StepUsagePanel } from './StepUsagePanel';

test('renders pipeline usage links with normalized source labels', () => {
  render(
    <MemoryRouter>
      <StepUsagePanel
        usage={[{ identifier: 'platform/release', description: 'Release pipeline', source: 'GitOps' }]}
        loading={false}
        error={null}
      />
    </MemoryRouter>
  );

  expect(screen.getByRole('link', { name: /platform\/release/i })).toHaveAttribute(
    'href',
    '/pipelines/platform/release'
  );
  expect(screen.getByText('git')).toBeInTheDocument();
});
