import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { YamlEditorToolbox } from './YamlEditorToolbox';

describe('YamlEditorToolbox', () => {
  it('renders collapsed parameters, suggestions, samples, and inserts snippets', async () => {
    const onInsertSnippet = vi.fn();
    const user = userEvent.setup();
    render(
      <YamlEditorToolbox
        resourceKind="pipeline"
        suggestionSlot={<div>Runtime pool suggestions</div>}
        onInsertSnippet={onInsertSnippet}
      />
    );

    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(screen.getByText('Runtime pool suggestions')).toBeInTheDocument();
    const pipelineGroup = screen.getByText('Pipeline Parameters').closest('details') as HTMLDetailsElement;
    expect(pipelineGroup.open).toBe(false);
    const stepStructures = screen.getByText('Step Structures').closest('details') as HTMLDetailsElement;
    expect(stepStructures.open).toBe(false);
    expect(screen.getByText('Small script pipeline')).toBeInTheDocument();

    await user.click(screen.getByText('Pipeline Parameters'));
    expect(pipelineGroup.open).toBe(true);

    const llmParam = screen.getByText('llm_enabled').closest('details') as HTMLDetailsElement;
    expect(llmParam.open).toBe(false);
    await user.click(within(llmParam).getByText('llm_enabled'));
    expect(llmParam.open).toBe(true);
    expect(within(llmParam).getByText('Values')).toBeInTheDocument();
    expect(within(llmParam).getByText('true')).toBeInTheDocument();
    expect(within(llmParam).getByText('false')).toBeInTheDocument();

    await user.click(screen.getByText('Step Structures'));
    expect(stepStructures.open).toBe(true);
    await user.click(screen.getByRole('button', { name: /script step/i }));
    expect(onInsertSnippet).toHaveBeenCalledWith(expect.stringContaining('- name: build'));
  });
});
