import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';
import { YamlValidationPanel } from './YamlValidationPanel';

test('renders validation errors, examples, overflow, and valid state', () => {
  const { rerender } = render(
    <YamlValidationPanel
      id="yaml-status"
      errors={[
        { message: 'Missing name', line: 2 },
        { message: 'Missing steps', line: 4 },
      ]}
      maxVisible={1}
      inline
      invalidLabel="Validation issues"
      renderExample={message => <pre>{message === 'Missing name' ? 'name: release' : ''}</pre>}
    />
  );

  expect(screen.getByRole('status')).toHaveClass('validation-box--inline', 'validation-box--error');
  expect(screen.getByText('Line 2')).toBeInTheDocument();
  expect(screen.getByText('name: release')).toBeInTheDocument();
  expect(screen.getByText('+ 1 more…')).toBeInTheDocument();

  rerender(<YamlValidationPanel id="yaml-status" errors={[]} />);
  expect(screen.getByText('Valid')).toBeInTheDocument();
});
