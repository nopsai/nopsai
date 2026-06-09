import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { WorkflowToastRegion } from './WorkflowToastRegion';

test('announces workflow notifications with status and alert semantics', () => {
  render(
    <WorkflowToastRegion
      toasts={[
        { id: 1, message: 'Saved', tone: 'success' },
        { id: 2, message: 'Save failed', tone: 'error' },
      ]}
      classPrefix="triggers"
    />
  );

  expect(screen.getByRole('status')).toHaveTextContent('Saved');
  expect(screen.getByRole('alert')).toHaveTextContent('Save failed');
  expect(screen.getByLabelText('Notifications')).toHaveAttribute('aria-live', 'polite');
});
