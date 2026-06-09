import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { LabDependencyPanel } from './LabDependencyPanel';

test('renders included dependencies and empty states', () => {
  const { rerender } = render(
    <LabDependencyPanel dependencies={{ status: 'ok', items: ['pipeline:platform/deploy'] }} />
  );
  expect(screen.getByText('pipeline:platform/deploy')).toBeInTheDocument();

  rerender(<LabDependencyPanel dependencies={{ status: 'no-steps', items: [] }} />);
  expect(screen.getByText('No steps defined yet.')).toBeInTheDocument();
});
