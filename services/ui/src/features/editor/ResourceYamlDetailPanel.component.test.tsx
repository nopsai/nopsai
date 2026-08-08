import { createRef } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ResourceYamlDetailPanel } from './ResourceYamlDetailPanel';

const ids = {
  content: 'test-yaml-content',
  editorContainer: 'test-editor-container',
  lineNumbers: 'test-line-numbers',
  stage: 'test-yaml-stage',
  highlight: 'test-yaml-highlight',
  editor: 'test-yaml-editor',
  validation: 'test-validation',
  autocomplete: 'test-autocomplete',
};

test('renders YAML actions and delegates edit, clone, and copy actions', async () => {
  const user = userEvent.setup();
  const onCopy = vi.fn();
  const onClone = vi.fn();
  const onEdit = vi.fn();

  render(
    <ResourceYamlDetailPanel
      title="Pipeline Definition (YAML)"
      rawYaml="name: build"
      isEditing={false}
      editorValue="name: build"
      validationErrors={[]}
      validationErrorLines={new Set()}
      editorSuggestion={null}
      autocompleteLoading={false}
      editorRef={createRef()}
      highlightContentRef={createRef()}
      lineNumbersRef={createRef()}
      ids={ids}
      editorLabel="Pipeline YAML editor"
      access={null}
      canUpdate
      canCreate
      isGitSource={false}
      saving={false}
      onCopy={onCopy}
      onDownload={vi.fn()}
      onEdit={onEdit}
      onClone={onClone}
      onDiscard={vi.fn()}
      onSave={vi.fn()}
      onEditorTextChange={vi.fn()}
      onOpenSuggestion={vi.fn()}
      onMoveSuggestion={vi.fn()}
      onDismissSuggestion={vi.fn()}
      onSelectSuggestion={vi.fn()}
      onEditorScroll={vi.fn()}
      onAutoIndentEnter={vi.fn()}
    />
  );

  expect(screen.getByText('Pipeline Definition (YAML)')).toBeInTheDocument();
  expect(screen.getByText('YAML valid')).toBeInTheDocument();
  expect(document.getElementById(ids.content)).toHaveTextContent(/name:\s*build/);

  await user.click(screen.getByRole('button', { name: /copy yaml/i }));
  await user.click(screen.getByRole('button', { name: /edit/i }));
  await user.click(screen.getByRole('button', { name: /clone/i }));

  expect(onCopy).toHaveBeenCalledOnce();
  expect(onEdit).toHaveBeenCalledOnce();
  expect(onClone).toHaveBeenCalledOnce();
});

test('renders edit mode with validation and autocomplete keyboard behavior', async () => {
  const user = userEvent.setup();
  const onEditorTextChange = vi.fn();
  const onMoveSuggestion = vi.fn();
  const onDismissSuggestion = vi.fn();
  const onSelectSuggestion = vi.fn();
  const onAutoIndentEnter = vi.fn();

  render(
    <ResourceYamlDetailPanel
      title="Step Definition (YAML)"
      rawYaml="name: build"
      isEditing
      editorValue="name: build"
      validationErrors={[{ message: 'Missing task', line: 1 }]}
      validationErrorLines={new Set([1])}
      editorSuggestion={{ title: 'Suggestions', items: ['script'], activeIndex: 0 }}
      autocompleteLoading={false}
      editorRef={createRef()}
      highlightContentRef={createRef()}
      lineNumbersRef={createRef()}
      ids={ids}
      editorLabel="Step YAML editor"
      access={null}
      canUpdate
      canCreate={false}
      isGitSource={false}
      saving={false}
      onCopy={vi.fn()}
      onDownload={vi.fn()}
      onEdit={vi.fn()}
      onClone={vi.fn()}
      onDiscard={vi.fn()}
      onSave={vi.fn()}
      onEditorTextChange={onEditorTextChange}
      onOpenSuggestion={vi.fn()}
      onMoveSuggestion={onMoveSuggestion}
      onDismissSuggestion={onDismissSuggestion}
      onSelectSuggestion={onSelectSuggestion}
      onEditorScroll={vi.fn()}
      onAutoIndentEnter={onAutoIndentEnter}
    />
  );

  expect(screen.getByText('1 issue')).toBeInTheDocument();
  expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  const normalEditor = screen.getByRole('textbox', { name: /step yaml editor/i });
  expect(normalEditor).toHaveAttribute('wrap', 'off');
  await user.click(normalEditor);
  fireEvent.change(normalEditor, { target: { value: 'name: build\n', selectionStart: 12 } });

  expect(onDismissSuggestion).toHaveBeenCalled();
  expect(onEditorTextChange).toHaveBeenLastCalledWith('name: build\n', 12, { openSuggestion: false });

  await user.click(screen.getByRole('button', { name: /expand yaml editor/i }));
  expect(screen.getByRole('dialog', { name: /step definition/i })).toHaveClass('yaml-editor-fullscreen-modal');
  expect(screen.getByText('1 validation issue')).toBeInTheDocument();
  expect(screen.getByRole('listbox', { name: /suggestions autocomplete/i })).toBeInTheDocument();
  const editor = screen.getByRole('textbox', { name: /step yaml editor/i });
  expect(editor).toHaveAttribute('wrap', 'off');
  fireEvent.change(editor, { target: { value: 'name: build\nscript: echo ok', selectionStart: 27 } });
  await user.keyboard('{ArrowDown}{Enter}');

  expect(screen.getByText('Missing task')).toBeInTheDocument();
  expect(onEditorTextChange).toHaveBeenLastCalledWith('name: build\nscript: echo ok', 27, { openSuggestion: true });
  expect(onMoveSuggestion).not.toHaveBeenCalled();
  expect(onAutoIndentEnter).toHaveBeenCalled();
  expect(onSelectSuggestion).not.toHaveBeenCalled();

  fireEvent.keyDown(editor, { key: 'ArrowDown', altKey: true });
  expect(onMoveSuggestion).toHaveBeenCalledWith(1);

  await user.keyboard('{Tab}');

  expect(onSelectSuggestion).toHaveBeenCalledWith('script');
});

test('inserts toolbox snippets at the current editor cursor when expanded', async () => {
  const user = userEvent.setup();
  const onEditorTextChange = vi.fn();
  const editorRef = createRef<HTMLTextAreaElement>();

  render(
    <ResourceYamlDetailPanel
      resourceKind="pipeline"
      title="Pipeline Definition (YAML)"
      rawYaml="steps:\n  "
      isEditing
      editorValue="steps:\n  "
      validationErrors={[]}
      validationErrorLines={new Set()}
      editorSuggestion={null}
      autocompleteLoading={false}
      editorRef={editorRef}
      highlightContentRef={createRef()}
      lineNumbersRef={createRef()}
      ids={ids}
      editorLabel="Pipeline YAML editor"
      access={null}
      canUpdate
      canCreate={false}
      isGitSource={false}
      saving={false}
      onCopy={vi.fn()}
      onDownload={vi.fn()}
      onEdit={vi.fn()}
      onClone={vi.fn()}
      onDiscard={vi.fn()}
      onSave={vi.fn()}
      onEditorTextChange={onEditorTextChange}
      onOpenSuggestion={vi.fn()}
      onMoveSuggestion={vi.fn()}
      onDismissSuggestion={vi.fn()}
      onSelectSuggestion={vi.fn()}
      onEditorScroll={vi.fn()}
      onAutoIndentEnter={vi.fn()}
    />
  );

  await user.click(screen.getByRole('button', { name: /expand yaml editor/i }));
  expect(screen.getByRole('dialog', { name: /pipeline definition/i })).toHaveClass('yaml-editor-fullscreen-modal');
  const editor = screen.getByRole('textbox', { name: /pipeline yaml editor/i });
  editor.focus();
  editor.setSelectionRange('steps:\n  '.length, 'steps:\n  '.length);
  await user.click(screen.getByRole('button', { name: /script step/i }));

  expect(onEditorTextChange).toHaveBeenCalledWith(expect.stringContaining('- name: build'), expect.any(Number));
});
