import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { KnowledgeContextWorkspace } from './KnowledgeContextWorkspace';
import {
  buildKnowledgeConnectionTeamSummaries,
  buildKnowledgeTree,
  collectKnowledgeTeamDocs,
  findKnowledgeTeam,
  summarizeKnowledgeWorkspace,
  type KnowledgeConnectionListItem,
  type KnowledgeContextListItem,
} from './model';

const documents: KnowledgeContextListItem[] = [
  {
    id: 'runbook/platform/restart',
    kind: 'runbook',
    team: 'platform',
    name: 'restart',
    description: 'Restart the platform safely.',
    visibility: 'team',
    source: 'database',
    used_by_count: 1,
  },
  {
    id: 'policy/security/access',
    kind: 'policy',
    team: 'security',
    name: 'access',
    visibility: 'restricted',
    source: 'confluence-page',
    used_by_count: 2,
  },
];

function renderWorkspace(overrides: Partial<Parameters<typeof KnowledgeContextWorkspace>[0]> = {}) {
  const tree = buildKnowledgeTree(documents, []);
  const activeTeam = overrides.activeTeam ?? '';
  const activeTeamNode = findKnowledgeTeam(tree, activeTeam);
  const defaultConnectionTeams = buildKnowledgeConnectionTeamSummaries(documents);
  const connectionTeams = overrides.connectionTeams ?? defaultConnectionTeams;
  const connectionTreeTeams = overrides.connectionTreeTeams ?? connectionTeams;
  const props = {
    activeTeam,
    activeConnectionTeam: 'platform',
    activeTab: 'documents' as const,
    treeRoot: tree,
    metrics: summarizeKnowledgeWorkspace(documents),
    connectionTeams,
    connectionTreeTeams,
    listLoading: false,
    listError: null,
    search: '',
    sourceFilter: 'all' as const,
    collectionDocuments: collectKnowledgeTeamDocs(activeTeamNode),
    selectedID: '',
    selectedDetail: {
      detail: null,
      editorValue: '',
      previewContent: '',
      contentMetrics: { lines: 0, words: 0, chars: 0 },
      detailError: null,
      draftID: null,
      isEditing: false,
      canEditSelected: false,
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
    },
    detailLoading: false,
    canWriteKnowledge: true,
    canDeleteKnowledge: true,
    onSearchChange: vi.fn(),
    onSourceFilterChange: vi.fn(),
    onSwitchTab: vi.fn(),
    onOpenTeam: vi.fn(),
    onSelectConnectionTeam: vi.fn(),
    onSelectDocument: vi.fn(),
    onDeleteDocument: vi.fn(),
    onCreateDocument: vi.fn(),
    onAddConnection: vi.fn(),
    ...overrides,
  };
  render(<KnowledgeContextWorkspace {...props} />);
  return props;
}

describe('KnowledgeContextWorkspace', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('renders the demo-style document workspace and actions', () => {
    const props = renderWorkspace();

    expect(screen.getByRole('tab', { name: 'Documents' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByLabelText('Knowledge Context summary')).toBeVisible();
    expect(screen.getByRole('complementary', { name: 'Documents browser' })).toBeVisible();
    expect(screen.getByRole('heading', { name: 'All knowledge' })).toBeVisible();
    expect(screen.getByRole('columnheader', { name: 'Source' })).toBeVisible();

    fireEvent.click(screen.getByRole('button', { name: 'Expand Runbooks' }));
    expect(screen.getAllByRole('button', { name: 'Open restart' }).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole('tab', { name: 'Connections' }));
    fireEvent.change(screen.getByPlaceholderText('Search knowledge contexts'), { target: { value: 'restart' } });
    fireEvent.change(screen.getByLabelText('Filter knowledge sources'), { target: { value: 'gitops' } });
    fireEvent.click(screen.getByRole('button', { name: 'Open Runbooks' }));
    fireEvent.click(screen.getByRole('button', { name: 'New context' }));
    fireEvent.click(screen.getAllByRole('button', { name: 'Open restart' })[0]);

    expect(props.onSwitchTab).toHaveBeenCalledWith('connections');
    expect(props.onSearchChange).toHaveBeenCalledWith('restart');
    expect(props.onSourceFilterChange).toHaveBeenCalledWith('gitops');
    expect(props.onOpenTeam).toHaveBeenCalledWith('runbook');
    expect(props.onCreateDocument).toHaveBeenCalledOnce();
    expect(props.onSelectDocument).toHaveBeenCalledWith('runbook/platform/restart');
  });

  it('uses the shared resizable tree column behavior', () => {
    localStorage.setItem('treeColumnWidth:knowledge-context', '360');

    renderWorkspace();

    const resizeHandle = screen.getByRole('separator', { name: 'Resize knowledge tree' });
    expect(resizeHandle).toHaveAttribute('aria-valuemin', '240');
    expect(resizeHandle).toHaveAttribute('aria-valuemax', '520');
    expect(resizeHandle).toHaveAttribute('aria-valuenow', '360');
    expect(resizeHandle.parentElement?.style.getPropertyValue('--tree-column-width')).toBe('360px');
  });

  it('renders an empty connection workspace without document team placeholders', () => {
    const props = renderWorkspace({
      activeTab: 'connections',
      activeConnectionTeam: '',
      connectionTeams: [],
    });

    expect(screen.getByRole('tab', { name: 'Connections' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('heading', { name: 'Connection tree' })).toBeVisible();
    expect(screen.getByRole('complementary', { name: 'Connections browser' })).toBeVisible();
    expect(screen.getByText('No connections yet.')).toBeVisible();
    expect(screen.getByRole('heading', { name: 'No knowledge connections yet' })).toBeVisible();
    expect(screen.queryByText('Confluence')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add connection' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'New connection' }));

    expect(props.onAddConnection).toHaveBeenCalledWith('');
  });

  it('renders configured connection rows and management actions', () => {
    const connection: KnowledgeConnectionListItem = {
      id: 'security/security-confluence',
      team: 'security',
      name: 'security-confluence',
      display_name: 'Security Confluence',
      provider: 'confluence',
      status: 'connected',
      credential_visibility: 'configured',
      base_url: 'https://confluence.example.test',
      external_document_count: 1,
      used_by: ['policy/security/access'],
      last_checked_at: '2026-07-15T10:00:00Z',
    };
    const platformConnection: KnowledgeConnectionListItem = {
      id: 'platform/platform-notion',
      team: 'platform',
      name: 'platform-notion',
      display_name: 'Platform Notion',
      provider: 'notion',
      status: 'connected',
      credential_visibility: 'configured',
    };
    const connectionTreeTeams = buildKnowledgeConnectionTeamSummaries([], ['platform', 'security'], [platformConnection, connection]);
    const props = renderWorkspace({
      activeTab: 'connections',
      activeConnectionTeam: 'security',
      connectionTeams: buildKnowledgeConnectionTeamSummaries([], ['security'], [connection]),
      connectionTreeTeams,
      onTestConnection: vi.fn(),
      onEditConnection: vi.fn(),
      onToggleConnection: vi.fn(),
      onDeleteConnection: vi.fn(),
    });

    expect(screen.getByRole('heading', { name: 'Connections' })).toBeVisible();
    expect(screen.getByText('security/security-confluence')).toBeVisible();
    expect(screen.getByRole('row', { name: /Security Confluence/ })).toHaveTextContent('Credential configured');
    expect(screen.getByRole('row', { name: /Security Confluence/ })).toHaveTextContent('1 linked');
    expect(screen.queryByText('Configured Connections')).not.toBeInTheDocument();
    expect(screen.queryByText('Select a connection to inspect linked Knowledge Contexts and management actions.')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Expand security connections' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByRole('button', { name: 'Expand platform connections' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('button', { name: 'Open Security Confluence connection' })).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog', { name: 'Security Confluence' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'View Security Confluence details' }));

    expect(props.onSelectConnectionTeam).toHaveBeenCalledWith('security');
    expect(screen.getByRole('button', { name: 'Expand platform connections' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.getByRole('link', { name: 'Open Security Confluence base URL' })).toBeVisible();
    expect(screen.getByRole('dialog', { name: 'Security Confluence' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Open policy/security/access' })).toBeVisible();
    expect(screen.queryByRole('tab', { name: 'Providers' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Reconnect Security Confluence' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Test Security Confluence' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit Security Confluence' }));
    fireEvent.click(screen.getByRole('button', { name: 'Disable Security Confluence' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete Security Confluence' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open policy/security/access' }));

    expect(props.onTestConnection).toHaveBeenCalledWith(expect.objectContaining({ id: connection.id }));
    expect(props.onEditConnection).toHaveBeenCalledWith(expect.objectContaining({ id: connection.id }));
    expect(props.onToggleConnection).toHaveBeenCalledWith(expect.objectContaining({ id: connection.id }));
    expect(props.onDeleteConnection).toHaveBeenCalledWith(expect.objectContaining({ id: connection.id }));
    expect(props.onSelectDocument).toHaveBeenCalledWith('policy/security/access');

    fireEvent.click(screen.getByRole('button', { name: /All connections/ }));
    expect(screen.queryByRole('dialog', { name: 'Security Confluence' })).not.toBeInTheDocument();
  });

  it('keeps connection tree branches collapsed until a team is selected', () => {
    const connection: KnowledgeConnectionListItem = {
      id: 'security/security-confluence',
      team: 'security',
      name: 'security-confluence',
      display_name: 'Security Confluence',
      provider: 'confluence',
      status: 'connected',
      credential_visibility: 'configured',
    };
    const props = renderWorkspace({
      activeTab: 'connections',
      activeConnectionTeam: '',
      connectionTeams: buildKnowledgeConnectionTeamSummaries([], ['security'], [connection]),
    });

    expect(screen.getByRole('button', { name: 'Expand security connections' })).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('button', { name: 'Open Security Confluence connection' })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'View Security Confluence details' }));

    expect(props.onSelectConnectionTeam).toHaveBeenCalledWith('security');
    expect(screen.getByRole('button', { name: 'Collapse security connections' })).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('button', { name: 'Open security connections' })).not.toHaveAttribute('aria-current');
    expect(screen.getByRole('button', { name: 'Open security connections' })).not.toHaveClass('active');
    expect(screen.getByRole('button', { name: 'Open Security Confluence connection' })).toBeVisible();
    expect(screen.getByRole('button', { name: 'Open Security Confluence connection' })).toHaveAttribute('aria-current', 'page');

    fireEvent.click(screen.getByRole('button', { name: 'Open security connections' }));
    expect(screen.queryByRole('dialog', { name: 'Security Confluence' })).not.toBeInTheDocument();
  });
});
