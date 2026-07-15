import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { KnowledgeContextDetailView } from './KnowledgeContextDetailView';
import type { KnowledgeContextDetail } from './model';

const detail: KnowledgeContextDetail = {
  id: 'runbook/platform/restart',
  uuid: '',
  kind: 'runbook',
  team: 'platform',
  name: 'restart',
  description: 'Restart the platform safely.',
  visibility: 'team',
  source: 'database',
  updated_at: '2026-07-15T10:00:00Z',
  content: '# Restart\nSteps',
  used_by: ['platform/deploy'],
  used_by_count: 1,
};

function renderDetail(overrides: Partial<Parameters<typeof KnowledgeContextDetailView>[0]> = {}) {
  const props = {
    detail,
    editorValue: detail.content,
    previewContent: detail.content,
    contentMetrics: { lines: 2, words: 3, chars: detail.content.length },
    detailError: null,
    draftID: null,
    isEditing: false,
    canEditSelected: true,
    selectedCanEdit: false,
    canWriteKnowledge: true,
    canDeleteKnowledge: true,
    saving: false,
    syncing: false,
    connections: [],
    onBackToList: vi.fn(),
    onCopy: vi.fn(),
    onDownload: vi.fn(),
    onStartEditing: vi.fn(),
    onClone: vi.fn(),
    onDiscardEditing: vi.fn(),
    onSave: vi.fn(),
    onSyncNow: vi.fn(),
    onDelete: vi.fn(),
    onDescriptionChange: vi.fn(),
    onDetailPatch: vi.fn(),
    onContentChange: vi.fn(),
    onAccessChange: vi.fn(),
    onOpenPipeline: vi.fn(),
    ...overrides,
  };
  const result = render(<KnowledgeContextDetailView {...props} />);
  return { props, ...result };
}

describe('KnowledgeContextDetailView', () => {
  it('renders metadata, content actions, and pipeline usage', () => {
    const { props } = renderDetail();

    expect(screen.getByRole('heading', { name: /restart/ })).toBeVisible();
    expect(screen.getByText('ID: runbook/platform/restart')).toBeVisible();
    expect(screen.getByText('Document Details')).toBeVisible();
    expect(screen.getByText('System Health')).toBeVisible();
    expect(screen.getAllByText('runbook/platform/restart').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Copy document ID' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Actions' })).toBeVisible();
    expect(screen.queryByRole('button', { name: 'Access' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit knowledge context' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Copy' })).not.toBeInTheDocument();
    expect(screen.queryByText('Summary')).not.toBeInTheDocument();
    expect(screen.queryByText('Document Overview')).not.toBeInTheDocument();
    expect(screen.queryByText('Document Activity')).not.toBeInTheDocument();
    expect(screen.queryByText('Content Preview')).not.toBeInTheDocument();
    expect(screen.queryByText(/Identity/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Access' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Actions' }));
    expect(screen.getByRole('button', { name: 'Access' })).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Export' }));
    fireEvent.click(screen.getByRole('button', { name: 'Clone' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

    fireEvent.click(screen.getByRole('tab', { name: 'Content' }));
    expect(screen.getByText('Content Preview')).toBeVisible();
    expect(screen.getByText((content, node) => node?.tagName.toLowerCase() === 'code' && content.includes('# Restart'))).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'Copy' }));

    fireEvent.click(screen.getByRole('tab', { name: 'Usage' }));
    expect(screen.getByText('Recent Usage')).toBeVisible();
    fireEvent.click(screen.getByRole('button', { name: 'platform/deploy' }));

    fireEvent.click(screen.getByRole('tab', { name: 'GitOps' }));
    expect(screen.getByText('Database')).toBeVisible();

    expect(props.onCopy).toHaveBeenCalledOnce();
    expect(props.onDownload).toHaveBeenCalledOnce();
    expect(props.onStartEditing).toHaveBeenCalledOnce();
    expect(props.onClone).toHaveBeenCalledOnce();
    expect(props.onDelete).toHaveBeenCalledWith(detail);
    expect(props.onOpenPipeline).toHaveBeenCalledWith('platform/deploy');
  });

  it('renders editable description and content controls', () => {
    const { props } = renderDetail({
      isEditing: true,
      selectedCanEdit: true,
    });

    fireEvent.click(screen.getByRole('tab', { name: 'Overview' }));
    fireEvent.change(screen.getByLabelText('Description'), { target: { value: 'Updated description' } });
    fireEvent.click(screen.getByRole('tab', { name: 'Content' }));
    fireEvent.change(screen.getByLabelText('Content'), { target: { value: '# Updated' } });
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));

    expect(props.onDescriptionChange).toHaveBeenCalledWith('Updated description');
    expect(props.onContentChange).toHaveBeenCalledWith('# Updated');
    expect(props.onDiscardEditing).toHaveBeenCalledOnce();
    expect(props.onSave).toHaveBeenCalledOnce();
  });

  it('renders external page sync controls and read-only cached content', () => {
    const externalDetail: KnowledgeContextDetail = {
      ...detail,
      id: 'policy/security/source-page',
      kind: 'policy',
      team: 'security',
      name: 'source-page',
      source: 'confluence',
      connection_ref: 'security/security-confluence',
      external_page_url: 'https://example.test/wiki/source-page',
      external_page_id: '12345',
      sync_mode: 'manual',
      failure_mode: 'fail',
      sync_status: 'cached',
      last_synced_at: '2026-07-15T10:30:00Z',
    };
    const connection = {
      id: 'security/security-confluence',
      team: 'security',
      name: 'security-confluence',
      display_name: 'Security Confluence',
      provider: 'confluence',
      status: 'connected',
      credential_visibility: 'configured',
    };
    const readOnlyRender = renderDetail({
      detail: externalDetail,
      editorValue: externalDetail.content,
      previewContent: externalDetail.content,
      connections: [connection],
    });

    expect(screen.getByRole('link', { name: externalDetail.external_page_url })).toHaveAttribute('href', externalDetail.external_page_url);
    fireEvent.click(screen.getByRole('button', { name: 'Actions' }));
    expect(screen.getByRole('button', { name: 'Sync now' })).toBeVisible();
    expect(screen.getByRole('link', { name: 'Open page' })).toHaveAttribute('href', externalDetail.external_page_url);
    fireEvent.click(screen.getByRole('button', { name: 'Sync now' }));
    expect(readOnlyRender.props.onSyncNow).toHaveBeenCalledOnce();
    readOnlyRender.unmount();

    const { props } = renderDetail({
      detail: externalDetail,
      editorValue: externalDetail.content,
      previewContent: externalDetail.content,
      isEditing: true,
      selectedCanEdit: true,
      connections: [connection],
    });

    fireEvent.click(screen.getByRole('tab', { name: 'Content' }));
    expect(screen.queryByLabelText('Content')).not.toBeInTheDocument();
    expect(screen.getByText(/External page content is read-only/)).toBeVisible();

    fireEvent.click(screen.getByRole('tab', { name: 'Overview' }));
    fireEvent.change(screen.getByLabelText('Page ID'), { target: { value: '67890' } });

    expect(props.onDetailPatch).toHaveBeenCalledWith({ external_page_id: '67890' });
  });
});
