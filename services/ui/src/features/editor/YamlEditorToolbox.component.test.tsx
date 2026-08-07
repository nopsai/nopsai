import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { YamlEditorToolbox } from './YamlEditorToolbox';

describe('YamlEditorToolbox', () => {
  it('renders validation, parameters, suggestions, samples, and inserts snippets', async () => {
    const onInsertSnippet = vi.fn();
    render(
      <YamlEditorToolbox
        resourceKind="pipeline"
        validationId="validation"
        validationErrors={[{ message: 'Unknown field', line: 2 }]}
        suggestionSlot={<div>Runtime pool suggestions</div>}
        onInsertSnippet={onInsertSnippet}
      />
    );

    expect(screen.getByRole('status')).toHaveTextContent('Validation issues');
    expect(screen.getByText('Runtime pool suggestions')).toBeInTheDocument();
    expect(screen.getByText('Pipeline Parameters')).toBeInTheDocument();
    expect(screen.getByText('Step Structures')).toBeInTheDocument();
    expect(screen.getByText('Small script pipeline')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: /script step/i }));
    expect(onInsertSnippet).toHaveBeenCalledWith(expect.stringContaining('- name: build'));
  });
});
