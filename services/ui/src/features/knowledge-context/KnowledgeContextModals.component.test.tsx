import { readFileSync } from 'node:fs';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { KnowledgeContextModals } from './KnowledgeContextModals';

describe('KnowledgeContextModals', () => {
  it('renders create source choices and external page fields', () => {
    const onUpdateForm = vi.fn();
    const onSubmitForm = vi.fn();
    const onAddConnectionFromForm = vi.fn();

    render(
      <KnowledgeContextModals
        formModal={{
          mode: 'create',
          contentSource: 'external',
          kind: 'runbook',
          team: 'platform',
          name: '',
          description: '',
          connection_id: '',
          external_page_id: '',
          external_page_url: '',
          sync_mode: 'manual',
          failure_mode: 'fail',
          content: '',
          pending: false,
        }}
        deleteModal={null}
        connectionModal={null}
        connections={[]}
        teamOptions={['platform', 'security']}
        onCloseForm={vi.fn()}
        onUpdateForm={onUpdateForm}
        onSubmitForm={onSubmitForm}
        onCloseDelete={vi.fn()}
        onConfirmDelete={vi.fn()}
        onCloseConnection={vi.fn()}
        onUpdateConnection={vi.fn()}
        onSubmitConnection={vi.fn()}
        onAddConnectionFromForm={onAddConnectionFromForm}
      />
    );

    expect(screen.getByRole('radio', { name: 'External page' })).toBeChecked();
    expect(screen.getByRole('combobox', { name: 'Team' })).toHaveValue('platform');
    expect(screen.queryByRole('option', { name: 'team-1' })).not.toBeInTheDocument();
    expect(screen.getByText('No connection is available for this team.')).toBeVisible();

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'restart' } });
    fireEvent.change(screen.getByLabelText('Page URL'), { target: { value: 'https://example.test/page' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add connection' }));
    fireEvent.change(screen.getByLabelText('Team'), { target: { value: 'security' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create' }));

    expect(onUpdateForm).toHaveBeenCalledWith({ name: 'restart' });
    expect(onUpdateForm).toHaveBeenCalledWith({ external_page_url: 'https://example.test/page' });
    expect(onUpdateForm).toHaveBeenCalledWith({ team: 'security' });
    expect(onAddConnectionFromForm).toHaveBeenCalledWith('platform');
    expect(onSubmitForm).toHaveBeenCalledOnce();
  });

  it('renders inline content fields when creating an inline document', () => {
    const onUpdateForm = vi.fn();

    render(
      <KnowledgeContextModals
        formModal={{
          mode: 'create',
          contentSource: 'inline',
          kind: 'architecture',
          team: 'platform',
          name: '',
          description: '',
          content: 'Existing inline content',
          pending: false,
        }}
        deleteModal={null}
        connectionModal={null}
        connections={[]}
        teamOptions={['platform', 'security']}
        onCloseForm={vi.fn()}
        onUpdateForm={onUpdateForm}
        onSubmitForm={vi.fn()}
        onCloseDelete={vi.fn()}
        onConfirmDelete={vi.fn()}
        onCloseConnection={vi.fn()}
        onUpdateConnection={vi.fn()}
        onSubmitConnection={vi.fn()}
        onAddConnectionFromForm={vi.fn()}
      />
    );

    expect(screen.getByRole('radio', { name: 'Inline content' })).toBeChecked();
    expect(screen.queryByLabelText('Page URL')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Content')).toHaveValue('Existing inline content');

    fireEvent.change(screen.getByLabelText('Content'), { target: { value: '# Architecture' } });
    fireEvent.change(screen.getByLabelText('Team'), { target: { value: 'security' } });

    expect(onUpdateForm).toHaveBeenCalledWith({ content: '# Architecture' });
    expect(onUpdateForm).toHaveBeenCalledWith({ team: 'security' });
  });

  it('renders the knowledge connection dialog and submits changes', () => {
    const onUpdateConnection = vi.fn();
    const onSubmitConnection = vi.fn();

    render(
      <KnowledgeContextModals
        formModal={null}
        deleteModal={null}
        connectionModal={{
          mode: 'create',
          team: 'platform',
          provider: 'notion',
          name: '',
          display_name: '',
          base_url: '',
          credential_ref: '',
          pending: false,
        }}
        connections={[]}
        teamOptions={['platform', 'security']}
        onCloseForm={vi.fn()}
        onUpdateForm={vi.fn()}
        onSubmitForm={vi.fn()}
        onCloseDelete={vi.fn()}
        onConfirmDelete={vi.fn()}
        onCloseConnection={vi.fn()}
        onUpdateConnection={onUpdateConnection}
        onSubmitConnection={onSubmitConnection}
        onAddConnectionFromForm={vi.fn()}
      />
    );

    expect(screen.getByRole('heading', { name: 'Add external page connection' })).toBeVisible();
    expect(screen.getByRole('combobox', { name: 'Team' })).toHaveValue('platform');
    expect(screen.getByText('Expected type: api_key')).toBeVisible();

    fireEvent.change(screen.getByLabelText('Team'), { target: { value: 'security' } });
    fireEvent.change(screen.getByLabelText('Provider'), { target: { value: 'confluence' } });
    fireEvent.change(screen.getByLabelText('Display name'), { target: { value: 'Platform Confluence' } });
    fireEvent.change(screen.getByLabelText('Base URL'), { target: { value: 'https://confluence.example.test' } });
    fireEvent.change(screen.getByLabelText('Credential reference'), { target: { value: 'knowledge/confluence' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create connection' }));

    expect(onUpdateConnection).toHaveBeenCalledWith({ team: 'security' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ provider: 'confluence' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ display_name: 'Platform Confluence' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ base_url: 'https://confluence.example.test' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ credential_ref: 'knowledge/confluence' });
    expect(onSubmitConnection).toHaveBeenCalledOnce();
  });

  it('keeps the connection dialog visible in the shared modal stylesheet', () => {
    const shell = readFileSync('src/components/modalShell.css', 'utf8');

    expect(shell).toContain('#knowledge-connection-modal.show');
    expect(shell).toContain('#knowledge-connection-modal.show .pipelines-modal-card');
  });

  it('sizes the document dialog without repainting the shared modal shell', () => {
    const styles = readFileSync('src/styles.css', 'utf8');
    const documentModalRule = styles.slice(
      styles.indexOf('.kc-document-modal--external {'),
      styles.indexOf('}', styles.indexOf('.kc-document-modal--external {'))
    );

    expect(documentModalRule).toContain('--modal-max-width');
    expect(documentModalRule).not.toContain('background');
    expect(documentModalRule).not.toContain('border-radius');
    expect(styles).not.toContain('.kc-document-modal .pipelines-modal-body');
  });
});
