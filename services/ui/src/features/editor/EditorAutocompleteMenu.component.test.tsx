import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { EditorAutocompleteMenu } from './EditorAutocompleteMenu';

describe('EditorAutocompleteMenu', () => {
  it('renders teamed metadata and inserts the selected suggestion', async () => {
    const onSelect = vi.fn();
    render(
      <EditorAutocompleteMenu
        loading
        suggestion={{
          title: 'Variables',
          items: ['DEPLOY_ENV', 'RETRIES'],
          activeIndex: 1,
          teamedSections: [{ label: '/platform', items: ['DEPLOY_ENV', 'RETRIES'], totalCount: 4 }],
        }}
        onSelect={onSelect}
      />
    );

    expect(screen.getByRole('listbox', { name: 'Variables autocomplete' })).toBeInTheDocument();
    expect(screen.getByText('+2 more')).toBeInTheDocument();
    expect(screen.getByText(/Tab inserts/)).toBeInTheDocument();
    expect(screen.getByText(/Loading/)).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'RETRIES' })).toHaveAttribute('aria-selected', 'true');

    const option = screen.getByRole('option', { name: 'DEPLOY_ENV' });
    expect(fireEvent.mouseDown(option)).toBe(false);
    await userEvent.click(option);
    expect(onSelect).toHaveBeenCalledWith('DEPLOY_ENV');
  });
});
