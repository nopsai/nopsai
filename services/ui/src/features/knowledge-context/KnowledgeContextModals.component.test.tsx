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
    expect(screen.getByText('No connection is available for this team.')).toBeVisible();

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'restart' } });
    fireEvent.change(screen.getByLabelText('Page URL'), { target: { value: 'https://example.test/page' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add connection' }));
    fireEvent.click(screen.getByRole('button', { name: 'Create' }));

    expect(onUpdateForm).toHaveBeenCalledWith({ name: 'restart' });
    expect(onUpdateForm).toHaveBeenCalledWith({ external_page_url: 'https://example.test/page' });
    expect(onAddConnectionFromForm).toHaveBeenCalledWith('platform');
    expect(onSubmitForm).toHaveBeenCalledOnce();
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

    fireEvent.change(screen.getByLabelText('Provider'), { target: { value: 'confluence' } });
    fireEvent.change(screen.getByLabelText('Display name'), { target: { value: 'Platform Confluence' } });
    fireEvent.change(screen.getByLabelText('Base URL'), { target: { value: 'https://confluence.example.test' } });
    fireEvent.change(screen.getByLabelText('Credential reference'), { target: { value: 'knowledge/confluence' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create connection' }));

    expect(onUpdateConnection).toHaveBeenCalledWith({ provider: 'confluence' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ display_name: 'Platform Confluence' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ base_url: 'https://confluence.example.test' });
    expect(onUpdateConnection).toHaveBeenCalledWith({ credential_ref: 'knowledge/confluence' });
    expect(onSubmitConnection).toHaveBeenCalledOnce();
  });

  it('keeps the connection dialog visible in the shared modal stylesheet', () => {
    const styles = readFileSync('src/styles.css', 'utf8');

    expect(styles).toContain('#knowledge-connection-modal.show');
    expect(styles).toContain('#knowledge-connection-modal.show .pipelines-modal-card');
  });
});
