import { useState } from 'react';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, test, vi } from 'vitest';
import { ScopeWorkflowModals } from './ScopeWorkflowModals';
import type {
  GitOpsSecretEncryptModalState,
  ScopeModalState,
  ScopedValueDeleteModalState,
  ScopedValueModalState,
} from './useScopeModalMutations';

type OpenDialog = 'scope' | 'variable' | 'gitops' | 'delete' | null;

function ScopeModalHarness({
  onSubmitScope,
  onSubmitVariable,
  onEncrypt,
  onCopy,
  onDelete,
}: {
  onSubmitScope: () => void;
  onSubmitVariable: () => void;
  onEncrypt: () => void;
  onCopy: () => void;
  onDelete: () => void;
}) {
  const [open, setOpen] = useState<OpenDialog>(null);
  const [scopeModal, setScopeModal] = useState<ScopeModalState>({
    parent: 'team',
    name: '',
    pending: false,
    error: 'Scope name is required.',
  });
  const [variableModal, setVariableModal] = useState<ScopedValueModalState>({
    mode: 'create',
    scope: 'team',
    name: '',
    repository: '',
    value: '',
    pending: false,
    error: 'Variable name is required.',
  });
  const [gitOpsModal, setGitOpsModal] = useState<GitOpsSecretEncryptModalState>({
    value: 'plain',
    encryptedValue: 'ENC[value]',
    pending: false,
  });
  const deleteModal: ScopedValueDeleteModalState = {
    kind: 'secret',
    scope: 'team',
    name: 'owner/repo/API_TOKEN',
    gitOpsManaged: true,
    pending: false,
  };

  return (
    <>
      <button type="button" onClick={() => setOpen('scope')}>Open scope</button>
      <button type="button" onClick={() => setOpen('variable')}>Open variable</button>
      <button
        type="button"
        onClick={() => {
          setVariableModal({
            mode: 'update',
            scope: 'team',
            originalName: 'owner/repo/API_URL',
            name: 'API_URL',
            repository: 'owner/repo',
            value: '',
            gitOpsManaged: true,
            pending: false,
          });
          setOpen('variable');
        }}
      >
        Open override variable
      </button>
      <button type="button" onClick={() => setOpen('gitops')}>Open GitOps</button>
      <button type="button" onClick={() => setOpen('delete')}>Open delete</button>
      <ScopeWorkflowModals
        scopeModal={open === 'scope' ? scopeModal : null}
        variableModal={open === 'variable' ? variableModal : null}
        secretModal={null}
        gitOpsEncryptModal={open === 'gitops' ? gitOpsModal : null}
        deleteModal={open === 'delete' ? deleteModal : null}
        canDeleteScopes
        knownRepositories={['owner/repo']}
        variableSuggestionEntries={[
          { scope: 'team', label: '/team', count: 1, preview: ['owner/repo/API_URL'] },
        ]}
        secretSuggestionEntries={[]}
        onCloseScope={() => setOpen(null)}
        onUpdateScopeName={name => setScopeModal(current => ({ ...current, name, error: undefined }))}
        onSubmitScope={onSubmitScope}
        onCloseVariable={() => setOpen(null)}
        onUpdateVariable={patch => setVariableModal(current => ({ ...current, ...patch, error: undefined }))}
        onChooseVariableSuggestion={name => setVariableModal(current => ({ ...current, name }))}
        onSubmitVariable={onSubmitVariable}
        onCloseSecret={() => setOpen(null)}
        onUpdateSecret={() => undefined}
        onChooseSecretSuggestion={() => undefined}
        onSubmitSecret={() => undefined}
        onCloseGitOpsEncrypt={() => setOpen(null)}
        onUpdateGitOpsEncryptValue={value => setGitOpsModal(current => ({ ...current, value }))}
        onEncryptGitOpsSecret={onEncrypt}
        onCopyGitOpsEncryptedValue={onCopy}
        onCloseDelete={() => setOpen(null)}
        onConfirmDelete={onDelete}
      />
    </>
  );
}

function renderHarness() {
  const callbacks = {
    onSubmitScope: vi.fn(),
    onSubmitVariable: vi.fn(),
    onEncrypt: vi.fn(),
    onCopy: vi.fn(),
    onDelete: vi.fn(),
  };
  render(<ScopeModalHarness {...callbacks} />);
  return callbacks;
}

test('labels scope fields, announces validation, traps focus, and restores the opener on Escape', async () => {
  const user = userEvent.setup();
  const { onSubmitScope } = renderHarness();
  const opener = screen.getByRole('button', { name: 'Open scope' });

  await user.click(opener);
  const dialog = screen.getByRole('dialog', { name: 'New Scope' });
  expect(screen.getByLabelText('Scope Name')).toHaveFocus();
  expect(within(dialog).getByRole('alert')).toHaveTextContent('Scope name is required.');

  await user.clear(screen.getByLabelText('Scope Name'));
  await user.type(screen.getByLabelText('Scope Name'), 'platform/dev');
  await user.click(within(dialog).getByRole('button', { name: 'Create Scope' }));
  expect(onSubmitScope).toHaveBeenCalledOnce();

  within(dialog).getByRole('button', { name: 'Create Scope' }).focus();
  await user.tab();
  expect(within(dialog).getByRole('button', { name: 'Close' })).toHaveFocus();

  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog', { name: 'New Scope' })).not.toBeInTheDocument();
  expect(opener).toHaveFocus();
});

test('associates scoped-value fields and exposes local validation errors', async () => {
  const user = userEvent.setup();
  const { onSubmitVariable } = renderHarness();

  await user.click(screen.getByRole('button', { name: 'Open variable' }));
  const dialog = screen.getByRole('dialog', { name: 'New Variable' });
  expect(screen.getByLabelText('Variable Name')).toHaveFocus();
  expect(screen.getByLabelText('Repository (optional)')).toBeVisible();
  expect(screen.getByLabelText('Value')).toBeVisible();
  expect(within(dialog).getByRole('alert')).toHaveTextContent('Variable name is required.');

  await user.click(within(dialog).getByRole('button', { name: 'Create Variable' }));
  expect(onSubmitVariable).toHaveBeenCalledOnce();
});

test('warns when editing GitOps-managed scoped values', async () => {
  const user = userEvent.setup();
  renderHarness();

  await user.click(screen.getByRole('button', { name: 'Open override variable' }));
  const dialog = screen.getByRole('dialog', { name: 'Variable' });
  expect(within(dialog).getByText(/Saving here creates a database override/)).toBeVisible();
});

test('supports GitOps encryption controls and destructive confirmation semantics', async () => {
  const user = userEvent.setup();
  const { onCopy, onDelete, onEncrypt } = renderHarness();

  await user.click(screen.getByRole('button', { name: 'Open GitOps' }));
  const gitOpsDialog = screen.getByRole('dialog', { name: 'Secret Encryption' });
  expect(screen.getByLabelText('Value')).toHaveFocus();
  expect(screen.getByLabelText('Encrypted Value')).toHaveValue('ENC[value]');
  await user.click(within(gitOpsDialog).getByRole('button', { name: 'Encrypt' }));
  await user.click(within(gitOpsDialog).getByRole('button', { name: 'Copy' }));
  expect(onEncrypt).toHaveBeenCalledOnce();
  expect(onCopy).toHaveBeenCalledOnce();
  await user.keyboard('{Escape}');

  const deleteOpener = screen.getByRole('button', { name: 'Open delete' });
  await user.click(deleteOpener);
  const deleteDialog = screen.getByRole('alertdialog', { name: 'Confirm removal' });
  expect(within(deleteDialog).getByRole('button', { name: 'Cancel' })).toHaveFocus();
  expect(within(deleteDialog).getByText(/The next GitOps sync can recreate it/)).toBeVisible();
  await user.click(within(deleteDialog).getByRole('button', { name: 'Delete' }));
  expect(onDelete).toHaveBeenCalledOnce();
});
