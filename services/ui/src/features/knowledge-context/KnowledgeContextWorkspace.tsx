import { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { BookOpenText, ChevronRight, Download, Edit3, ExternalLink, FolderTree, GitBranch, Link2, MoreHorizontal, Plus, RotateCw, Search, Trash2, UsersRound, X } from 'lucide-react';

import ResourceAccessCard from '../../components/ResourceAccessCard';
import { ObjectIcon } from '../../components/ObjectIcon';
import { TreeColumnResizeHandle } from '../../components/resizableTreeColumn';
import { useResizableTreeColumn } from '../../components/resizableTreeColumnState';
import { useOutsideDismiss } from '../../components/useOutsideDismiss';
import { KnowledgeContextConnectionsView } from './KnowledgeContextConnectionsView';
import { KnowledgeContextDetailView, type KnowledgeContextDetailViewProps } from './KnowledgeContextDetailView';
import {
  countTeamDocs,
  documentTeamPath,
  isExternalKnowledgeDocument,
  isGitManagedDocument,
  knowledgeConnectionDisplayName,
  knowledgeContentSource,
  knowledgeSyncStatusLabel,
  normalizeTeamPath,
  splitKnowledgePath,
  sourceLabel,
  type KnowledgeConnectionListItem,
  type KnowledgeConnectionTeamSummary,
  type KnowledgeContextListItem,
  type KnowledgeTeamNode,
  type KnowledgeWorkspaceMetrics,
  type KnowledgeWorkspaceTab,
} from './model';
import { kindIconType, kindPlural, kindTitle } from './presentation';

type KnowledgeContextWorkspaceProps = {
  activeTeam: string;
  activeConnectionTeam: string;
  activeTab: KnowledgeWorkspaceTab;
  treeRoot: KnowledgeTeamNode;
  metrics: KnowledgeWorkspaceMetrics;
  connectionTeams: KnowledgeConnectionTeamSummary[];
  connectionTreeTeams: KnowledgeConnectionTeamSummary[];
  listLoading: boolean;
  listError: string | null;
  search: string;
  collectionDocuments: KnowledgeContextListItem[];
  selectedID: string;
  selectedDetail: KnowledgeContextDetailViewProps;
  detailLoading: boolean;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  onSearchChange: (term: string) => void;
  onSwitchTab: (tab: KnowledgeWorkspaceTab) => void;
  onOpenTeam: (team: string) => void;
  onSelectConnectionTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
  onDownloadDocument: (document: KnowledgeContextListItem) => void;
  onSyncDocument: (document: KnowledgeContextListItem) => void;
  onEditDocument: (document: KnowledgeContextListItem) => void;
  onCloneDocument: (document: KnowledgeContextListItem) => void;
  onAccessChange: (access: { resource_id?: string; visibility?: string }) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
  onCreateDocument: () => void;
  onAddConnection: (teamPath: string) => void;
  onTestConnection?: (connection: KnowledgeConnectionListItem) => void;
  onEditConnection?: (connection: KnowledgeConnectionListItem) => void;
  onToggleConnection?: (connection: KnowledgeConnectionListItem) => void;
  onDeleteConnection?: (connection: KnowledgeConnectionListItem) => void;
};

const statCards = [
  { key: 'documents', label: 'Documents', icon: BookOpenText, tone: 'purple' },
  { key: 'externalDocuments', label: 'External pages', icon: Link2, tone: 'blue' },
  { key: 'gitOpsManaged', label: 'GitOps', icon: GitBranch, tone: 'green' },
  { key: 'teams', label: 'Teams', icon: UsersRound, tone: 'cyan' },
] as const;

function KnowledgeWorkspaceMetricGrid({ metrics }: { metrics: KnowledgeWorkspaceMetrics }) {
  return (
    <div className="kc-demo-stats kc-demo-stats--toolbar" aria-label="Knowledge Context summary">
      {statCards.map(stat => {
        const Icon = stat.icon;
        return (
          <article key={stat.key} className="kc-demo-stat">
            <span className={`kc-demo-stat-icon kc-demo-stat-icon--${stat.tone}`} aria-hidden="true">
              <Icon className="h-4 w-4" />
            </span>
            <span className="kc-demo-stat-label">{stat.label}</span>
            <strong>{metrics[stat.key]}</strong>
          </article>
        );
      })}
    </div>
  );
}

function summarizeKnowledgeConnections(teams: KnowledgeConnectionTeamSummary[]) {
  const connections = teams.flatMap(team => team.connections);
  return {
    configured: connections.length,
    connected: connections.filter(connection => connection.status === 'connected' && !connection.disabled).length,
    authRequired: connections.filter(connection => connection.status === 'authentication_required').length,
    disabled: connections.filter(connection => connection.disabled).length,
  };
}

function KnowledgeConnectionSummaryGrid({ summary }: { summary: ReturnType<typeof summarizeKnowledgeConnections> }) {
  const cards = [
    { label: 'Configured', value: summary.configured, icon: Link2, tone: 'green' },
    { label: 'Connected', value: summary.connected, icon: UsersRound, tone: 'blue' },
    { label: 'Auth required', value: summary.authRequired, icon: BookOpenText, tone: 'purple' },
    { label: 'Disabled', value: summary.disabled, icon: UsersRound, tone: 'cyan' },
  ] as const;

  return (
    <div className="kc-demo-stats kc-demo-stats--toolbar" aria-label="Knowledge connection summary">
      {cards.map(card => {
        const Icon = card.icon;
        return (
          <article key={card.label} className="kc-demo-stat">
            <span className={`kc-demo-stat-icon kc-demo-stat-icon--${card.tone}`} aria-hidden="true">
              <Icon className="h-4 w-4" />
            </span>
            <span className="kc-demo-stat-label">{card.label}</span>
            <strong>{card.value}</strong>
          </article>
        );
      })}
    </div>
  );
}

export function KnowledgeContextWorkspace({
  activeTeam,
  activeConnectionTeam,
  activeTab,
  treeRoot,
  metrics,
  connectionTeams,
  connectionTreeTeams,
  listLoading,
  listError,
  search,
  collectionDocuments,
  selectedID,
  selectedDetail,
  detailLoading,
  canWriteKnowledge,
  canDeleteKnowledge,
  onSearchChange,
  onSwitchTab,
  onOpenTeam,
  onSelectConnectionTeam,
  onSelectDocument,
  onDownloadDocument,
  onSyncDocument,
  onEditDocument,
  onCloneDocument,
  onAccessChange,
  onDeleteDocument,
  onCreateDocument,
  onAddConnection,
  onTestConnection,
  onEditConnection,
  onToggleConnection,
  onDeleteConnection,
}: KnowledgeContextWorkspaceProps) {
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [selectedConnectionID, setSelectedConnectionID] = useState('');
  const [selectedConnectionTeamPath, setSelectedConnectionTeamPath] = useState('');
  const treeResize = useResizableTreeColumn({
    storageKey: 'knowledge-context',
    defaultWidth: 270,
    minWidth: 240,
    maxWidth: 520,
  });
  const actionLabel = activeTab === 'connections' ? 'New connection' : 'New context';
  const searchPlaceholder = activeTab === 'connections' ? 'Search connections' : 'Search knowledge contexts';
  const searchLabel = activeTab === 'connections' ? 'Search connections' : 'Search knowledge contexts';
  const searchActive = searchOpen || Boolean(search.trim());
  const activeSelectedConnectionID = activeTab === 'connections' ? selectedConnectionID : '';
  const activeSelectedConnectionTeamPath = activeTab === 'connections' ? selectedConnectionTeamPath : '';
  const connectionActionTeam = activeSelectedConnectionTeamPath || activeConnectionTeam;
  const connectionSummary = useMemo(
    () => summarizeKnowledgeConnections(connectionTeams),
    [connectionTeams]
  );

  const handleSwitchTab = (tab: KnowledgeWorkspaceTab) => {
    if (tab !== 'connections') {
      setSelectedConnectionID('');
      setSelectedConnectionTeamPath('');
    }
    onSwitchTab(tab);
  };

  const handleSelectConnectionTeam = (teamPath: string) => {
    const normalized = normalizeTeamPath(teamPath);
    setSelectedConnectionID('');
    setSelectedConnectionTeamPath(normalized);
    onSelectConnectionTeam(normalized);
  };

  const handleSelectConnection = (connectionID: string, teamPath: string) => {
    const normalized = normalizeTeamPath(teamPath);
    setSelectedConnectionID(connectionID);
    setSelectedConnectionTeamPath(normalized);
    onSelectConnectionTeam(normalized);
  };

  const handleCloseConnectionDetails = () => {
    setSelectedConnectionID('');
  };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return;
      event.preventDefault();
      searchInputRef.current?.focus();
      searchInputRef.current?.select();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  return (
    <section className="kc-demo-page" aria-label="Knowledge Context workspace">
      <header className="kc-demo-page-top">
        <div className="kc-demo-page-tabs" role="tablist" aria-label="Knowledge Context sections">
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'documents'}
            className={activeTab === 'documents' ? 'active' : ''}
            onClick={() => handleSwitchTab('documents')}
          >
            <BookOpenText className="h-4 w-4" aria-hidden="true" />
            Documents
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'connections'}
            className={activeTab === 'connections' ? 'active' : ''}
            onClick={() => handleSwitchTab('connections')}
          >
            <Link2 className="h-4 w-4" aria-hidden="true" />
            Connections
          </button>
        </div>
        {activeTab === 'connections' ? (
          <KnowledgeConnectionSummaryGrid summary={connectionSummary} />
        ) : (
          <KnowledgeWorkspaceMetricGrid metrics={metrics} />
        )}
        <div className="kc-demo-top-actions">
          <button
            type="button"
            className="kc-demo-primary-btn"
            onClick={() => {
              if (activeTab === 'connections') onAddConnection(connectionActionTeam);
              else onCreateDocument();
            }}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            {actionLabel}
          </button>
          <div className={`pipelines-search-shell kc-demo-global-search-shell ${searchActive ? 'open' : ''}`}>
            <button
              type="button"
              className="pipelines-search-toggle"
              aria-label={searchLabel}
              title={searchLabel}
              onClick={() => {
                setSearchOpen(true);
                requestAnimationFrame(() => searchInputRef.current?.focus());
              }}
            >
              <Search className="h-4 w-4" aria-hidden="true" />
            </button>
            <input
              ref={searchInputRef}
              type="search"
              className="pipelines-search-input"
              aria-label={`${searchLabel} query`}
              placeholder={searchPlaceholder}
              value={search}
              onFocus={() => setSearchOpen(true)}
              onChange={event => {
                onSearchChange(event.target.value);
                if (event.target.value && !searchOpen) setSearchOpen(true);
              }}
              onBlur={() => {
                if (!search.trim()) setSearchOpen(false);
              }}
            />
            {search || searchOpen ? (
              <button
                type="button"
                className="pipelines-search-clear"
                aria-label="Clear search"
                onMouseDown={event => event.preventDefault()}
                onClick={() => {
                  onSearchChange('');
                  setSearchOpen(false);
                  searchInputRef.current?.blur();
                }}
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
          </div>
        </div>
      </header>

      <div className="kc-demo-workspace" style={treeResize.gridStyle}>
        <KnowledgeBrowserCard
          activeTab={activeTab}
          activeTeam={activeTeam}
          activeConnectionTeam={activeSelectedConnectionTeamPath}
          treeRoot={treeRoot}
          totalDocuments={metrics.documents}
          connectionTeams={connectionTreeTeams}
          listLoading={listLoading}
          listError={listError}
          selectedID={selectedID}
          onOpenTeam={onOpenTeam}
          selectedConnectionID={activeSelectedConnectionID}
          onSelectConnectionTeam={handleSelectConnectionTeam}
          onSelectConnection={handleSelectConnection}
          onSelectDocument={onSelectDocument}
        />
        <TreeColumnResizeHandle {...treeResize} label="Resize knowledge tree" />

        {activeTab === 'connections' ? (
          <KnowledgeContextConnectionsView
            listLoading={listLoading}
            listError={listError}
            search={search}
            teams={connectionTeams}
            selectedConnectionID={activeSelectedConnectionID}
            canWriteKnowledge={canWriteKnowledge}
            canDeleteKnowledge={canDeleteKnowledge}
            onSelectConnection={handleSelectConnection}
            onCloseConnectionDetails={handleCloseConnectionDetails}
            onSelectDocument={onSelectDocument}
            onTestConnection={onTestConnection || (() => undefined)}
            onEditConnection={onEditConnection || (() => undefined)}
            onToggleConnection={onToggleConnection || (() => undefined)}
            onDeleteConnection={onDeleteConnection || (() => undefined)}
          />
        ) : detailLoading ? (
          <div className="kc-demo-detail-empty">Loading knowledge context...</div>
        ) : selectedID ? (
          <KnowledgeContextDetailView {...selectedDetail} onCreateDocument={onCreateDocument} />
        ) : (
          <KnowledgeDocumentCollection
            documents={collectionDocuments}
            listLoading={listLoading}
            listError={listError}
            canDeleteKnowledge={canDeleteKnowledge}
            canWriteKnowledge={canWriteKnowledge}
            onSelectDocument={onSelectDocument}
            onDownloadDocument={onDownloadDocument}
            onSyncDocument={onSyncDocument}
            onEditDocument={onEditDocument}
            onCloneDocument={onCloneDocument}
            onAccessChange={onAccessChange}
            onDeleteDocument={onDeleteDocument}
          />
        )}
      </div>
    </section>
  );
}

function KnowledgeBrowserCard({
  activeTab,
  activeTeam,
  activeConnectionTeam,
  treeRoot,
  totalDocuments,
  connectionTeams,
  listLoading,
  listError,
  selectedID,
  selectedConnectionID,
  onOpenTeam,
  onSelectConnectionTeam,
  onSelectConnection,
  onSelectDocument,
}: {
  activeTab: KnowledgeWorkspaceTab;
  activeTeam: string;
  activeConnectionTeam: string;
  treeRoot: KnowledgeTeamNode;
  totalDocuments: number;
  connectionTeams: KnowledgeConnectionTeamSummary[];
  listLoading: boolean;
  listError: string | null;
  selectedID: string;
  selectedConnectionID: string;
  onOpenTeam: (team: string) => void;
  onSelectConnectionTeam: (team: string) => void;
  onSelectConnection: (connectionID: string, teamPath: string) => void;
  onSelectDocument: (id: string) => void;
}) {
  const title = activeTab === 'connections' ? 'Connection tree' : 'Knowledge tree';
  const count = activeTab === 'connections'
    ? connectionTeams.reduce((total, team) => total + team.connections.length, 0)
    : totalDocuments;

  return (
    <aside className="triggers-explorer" aria-label={`${activeTab === 'connections' ? 'Connections' : 'Documents'} browser`}>
      <div className="triggers-explorer-head">
        <span className="triggers-explorer-head-icon" aria-hidden="true">
          <FolderTree className="h-4 w-4" />
        </span>
        <div>
          <h3>{title}</h3>
          <p>{count} total</p>
        </div>
      </div>

      {listLoading ? (
        <p className="kc-demo-tree-state">Loading...</p>
      ) : listError ? (
        <p className="kc-demo-tree-state kc-demo-tree-state--error">Failed to load: {listError}</p>
      ) : activeTab === 'connections' ? (
        <ConnectionRows
          teams={connectionTeams}
          selectedTeamPath={activeConnectionTeam}
          selectedConnectionID={selectedConnectionID}
          onSelectConnectionTeam={onSelectConnectionTeam}
          onSelectConnection={onSelectConnection}
        />
      ) : (
        <DocumentRows
          activeTeam={activeTeam}
          treeRoot={treeRoot}
          totalDocuments={totalDocuments}
          selectedID={selectedID}
          onOpenTeam={onOpenTeam}
          onSelectDocument={onSelectDocument}
        />
      )}
    </aside>
  );
}

function DocumentRows({
  activeTeam,
  treeRoot,
  totalDocuments,
  selectedID,
  onOpenTeam,
  onSelectDocument,
}: {
  activeTeam: string;
  treeRoot: KnowledgeTeamNode;
  totalDocuments: number;
  selectedID: string;
  onOpenTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
}) {
  const [nodeOpenOverrides, setNodeOpenOverrides] = useState<Map<string, boolean>>(() => new Map());
  const selectedPath = selectedID ? splitKnowledgePath(selectedID).team : '';
  const forcedOpenNodeIDs = useMemo(() => {
    const ids = new Set<string>(['root']);
    knowledgeTreeAncestorIDs(activeTeam).forEach(id => ids.add(id));
    knowledgeTreeAncestorIDs(selectedPath).forEach(id => ids.add(id));
    return ids;
  }, [activeTeam, selectedPath]);
  const openNodeIDs = useMemo(() => {
    const ids = new Set(forcedOpenNodeIDs);
    nodeOpenOverrides.forEach((open, id) => {
      if (open) ids.add(id);
      else ids.delete(id);
    });
    forcedOpenNodeIDs.forEach(id => ids.add(id));
    return ids;
  }, [forcedOpenNodeIDs, nodeOpenOverrides]);
  const toggleNode = (id: string) => {
    setNodeOpenOverrides(previous => {
      const next = new Map(previous);
      next.set(id, !openNodeIDs.has(id));
      return next;
    });
  };

  if (!totalDocuments) {
    return <p className="kc-demo-tree-state">No documents found.</p>;
  }

  return (
    <>
      <button
        type="button"
        className={`triggers-explorer-root ${!activeTeam && !selectedID ? 'active' : ''}`}
        aria-current={!activeTeam && !selectedID ? 'page' : undefined}
        onClick={() => onOpenTeam('')}
      >
        <span className="triggers-explorer-folder" aria-hidden="true">
          <ObjectIcon type="knowledge-context" />
        </span>
        <span>All knowledge</span>
        <strong>{totalDocuments}</strong>
      </button>
      <ul className="triggers-explorer-tree">
        {treeRoot.children.map(child => (
          <KnowledgeExplorerNode
            key={child.id}
            node={child}
            openNodeIDs={openNodeIDs}
            activeTeam={activeTeam}
            selectedID={selectedID}
            onToggleNode={toggleNode}
            onOpenTeam={onOpenTeam}
            onSelectDocument={onSelectDocument}
          />
        ))}
        {treeRoot.docs.map(document => (
          <KnowledgeExplorerLeaf
            key={document.id}
            document={document}
            selected={selectedID === document.id}
            onSelectDocument={onSelectDocument}
          />
        ))}
      </ul>
    </>
  );
}

function KnowledgeExplorerNode({
  node,
  openNodeIDs,
  activeTeam,
  selectedID,
  onToggleNode,
  onOpenTeam,
  onSelectDocument,
}: {
  node: KnowledgeTeamNode;
  openNodeIDs: Set<string>;
  activeTeam: string;
  selectedID: string;
  onToggleNode: (id: string) => void;
  onOpenTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
}) {
  const open = openNodeIDs.has(node.id);
  const active = activeTeam === node.fullPath && !selectedID;
  const isKind = node.fullPath.split('/').filter(Boolean).length === 1;
  const hasChildren = node.children.length > 0 || node.docs.length > 0;
  const label = isKind ? kindPlural(node.name) : node.name;

  return (
    <li className="triggers-explorer-node">
      <div className="triggers-explorer-node-row">
        <button
          type="button"
          className="triggers-explorer-toggle"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${label}`}
          aria-expanded={open}
          onClick={() => onToggleNode(node.id)}
          disabled={!hasChildren}
        >
          <ChevronRight className={`h-3.5 w-3.5 ${open ? 'rotate-90' : ''}`} aria-hidden="true" />
        </button>
        <button
          type="button"
          className={`triggers-explorer-owner ${active ? 'active' : ''}`}
          aria-label={`Open ${label}`}
          aria-current={active ? 'page' : undefined}
          onClick={() => onOpenTeam(node.fullPath)}
        >
          <span className="triggers-explorer-folder" aria-hidden="true">
            <ObjectIcon type={isKind ? kindIconType(node.name) : 'team'} />
          </span>
          <span className="truncate">{label}</span>
          <strong>{countTeamDocs(node)}</strong>
        </button>
      </div>
      {open ? (
        <ul className="triggers-explorer-children">
          {node.children.map(child => (
            <KnowledgeExplorerNode
              key={child.id}
              node={child}
              openNodeIDs={openNodeIDs}
              activeTeam={activeTeam}
              selectedID={selectedID}
              onToggleNode={onToggleNode}
              onOpenTeam={onOpenTeam}
              onSelectDocument={onSelectDocument}
            />
          ))}
          {node.docs.map(document => (
            <KnowledgeExplorerLeaf
              key={document.id}
              document={document}
              selected={selectedID === document.id}
              onSelectDocument={onSelectDocument}
            />
          ))}
        </ul>
      ) : null}
    </li>
  );
}

function KnowledgeExplorerLeaf({
  document,
  selected,
  onSelectDocument,
}: {
  document: KnowledgeContextListItem;
  selected: boolean;
  onSelectDocument: (id: string) => void;
}) {
  const sourceKey = knowledgeContentSource(document);
  return (
    <li className="triggers-explorer-leaf">
      <button
        type="button"
        className={`triggers-explorer-trigger ${selected ? 'active' : ''}`}
        aria-label={`Open ${document.name}`}
        aria-current={selected ? 'page' : undefined}
        onClick={() => onSelectDocument(document.id)}
      >
        <span className="triggers-explorer-trigger-icon" aria-hidden="true">
          <ObjectIcon type={kindIconType(document.kind)} />
        </span>
        <span className="truncate">{document.name || document.id}</span>
        <span
          className={`triggers-explorer-source triggers-explorer-source--${isGitManagedDocument(document) ? 'git' : sourceKey}`}
          title={sourceLabel(document.source)}
          aria-label={sourceLabel(document.source)}
        />
      </button>
    </li>
  );
}

function knowledgeTreeAncestorIDs(path: string) {
  const parts = path.split('/').filter(Boolean);
  const ids: string[] = [];
  for (let index = 0; index < parts.length; index += 1) {
    ids.push(parts.slice(0, index + 1).join('/'));
  }
  return ids;
}

function KnowledgeDocumentCollection({
  documents,
  listLoading,
  listError,
  canDeleteKnowledge,
  canWriteKnowledge,
  onSelectDocument,
  onDownloadDocument,
  onSyncDocument,
  onEditDocument,
  onCloneDocument,
  onAccessChange,
  onDeleteDocument,
}: {
  documents: KnowledgeContextListItem[];
  listLoading: boolean;
  listError: string | null;
  canDeleteKnowledge: boolean;
  canWriteKnowledge: boolean;
  onSelectDocument: (id: string) => void;
  onDownloadDocument: (document: KnowledgeContextListItem) => void;
  onSyncDocument: (document: KnowledgeContextListItem) => void;
  onEditDocument: (document: KnowledgeContextListItem) => void;
  onCloneDocument: (document: KnowledgeContextListItem) => void;
  onAccessChange: (access: { resource_id?: string; visibility?: string }) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
}) {
  const sortedDocuments = [...documents].sort(
    (a, b) =>
      a.kind.localeCompare(b.kind) ||
      documentTeamPath(a).localeCompare(documentTeamPath(b)) ||
      a.name.localeCompare(b.name)
  );

  if (listLoading) {
    return <section className="kc-demo-browser-main"><div className="kc-demo-table-empty">Loading knowledge contexts...</div></section>;
  }

  if (listError) {
    return (
      <section className="kc-demo-browser-main">
        <div className="kc-demo-table-empty kc-demo-tree-state--error">Failed to load: {listError}</div>
      </section>
    );
  }

  return (
    <section className="kc-demo-browser-main" aria-label="Knowledge contexts collection">
      <div className="kc-demo-collection">
        {sortedDocuments.length ? (
          <div className="kc-demo-table-wrap">
            <table className="kc-demo-resource-table">
              <thead>
                <tr>
                  <th>Knowledge context</th>
                  <th>Kind</th>
                  <th>Team</th>
                  <th>Source</th>
                  <th>Sync</th>
                  <th>Used by</th>
                  <th aria-label="Actions" />
                </tr>
              </thead>
              <tbody>
                {sortedDocuments.map(document => {
                  const teamPath = documentTeamPath(document) || 'root';
                  const usedByCount = document.used_by_count ?? document.used_by?.length ?? 0;
                  const source = documentSourceLabel(document);
                  const sync = documentSyncBadge(document);
                  return (
                    <tr key={document.id}>
                      <td>
                        <button type="button" className="kc-demo-resource-cell" aria-label={`Open ${document.name}`} onClick={() => onSelectDocument(document.id)}>
                          <span className="kc-demo-resource-icon" aria-hidden="true">
                            <ObjectIcon type={kindIconType(document.kind)} />
                          </span>
                          <span className="kc-demo-resource-name">
                            <strong>{document.name || document.id}</strong>
                          </span>
                        </button>
                      </td>
                      <td><span className={`kc-demo-badge ${kindBadgeTone(document.kind)}`}><span className="dot" />{kindTitle(document.kind)}</span></td>
                      <td><span className="kc-demo-mono">{teamPath}</span></td>
                      <td><span className={`kc-demo-badge ${source.tone}`}><span className="dot" />{source.label}</span></td>
                      <td>
                        {sync.label ? (
                          <span className={`kc-demo-badge ${sync.tone}`}><span className="dot" />{sync.label}</span>
                        ) : (
                          <span className="kc-demo-mono">-</span>
                        )}
                      </td>
                      <td><span className="kc-demo-mono">{usedByCount ? `${usedByCount} ${usedByCount === 1 ? 'pipeline' : 'pipelines'}` : 'None'}</span></td>
                      <td>
                        <KnowledgeDocumentRowActions
                          document={document}
                          canWriteKnowledge={canWriteKnowledge}
                          canDeleteKnowledge={canDeleteKnowledge}
                          onSelectDocument={onSelectDocument}
                          onDownloadDocument={onDownloadDocument}
                          onSyncDocument={onSyncDocument}
                          onEditDocument={onEditDocument}
                          onCloneDocument={onCloneDocument}
                          onAccessChange={onAccessChange}
                          onDeleteDocument={onDeleteDocument}
                        />
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="kc-demo-table-empty">
            <ObjectIcon type="knowledge-context" className="h-6 w-6" />
            <strong>No knowledge contexts found</strong>
            <span>Try a different search or source filter.</span>
          </div>
        )}
      </div>
    </section>
  );
}

function KnowledgeDocumentRowActions({
  document: item,
  canWriteKnowledge,
  canDeleteKnowledge,
  onSelectDocument,
  onDownloadDocument,
  onSyncDocument,
  onEditDocument,
  onCloneDocument,
  onAccessChange,
  onDeleteDocument,
}: {
  document: KnowledgeContextListItem;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  onSelectDocument: (id: string) => void;
  onDownloadDocument: (document: KnowledgeContextListItem) => void;
  onSyncDocument: (document: KnowledgeContextListItem) => void;
  onEditDocument: (document: KnowledgeContextListItem) => void;
  onCloneDocument: (document: KnowledgeContextListItem) => void;
  onAccessChange: (access: { resource_id?: string; visibility?: string }) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
}) {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<{ top: number; right: number } | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const menuID = `knowledge-row-actions-${item.id.replace(/[^a-zA-Z0-9_-]/g, '-')}`;
  const itemLabel = item.name || item.id;
  const isExternal = isExternalKnowledgeDocument(item);
  const portalHost = typeof document === 'undefined' ? null : document.querySelector('[data-page="knowledge-context"]') || document.body;
  const closeMenu = () => setOpen(false);

  const updatePosition = () => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const rect = trigger.getBoundingClientRect();
    const estimatedMenuHeight = 300;
    const belowTop = rect.bottom + 8;
    const top = belowTop + estimatedMenuHeight > window.innerHeight
      ? Math.max(8, rect.top - estimatedMenuHeight - 8)
      : belowTop;
    setPosition({
      top,
      right: Math.max(8, window.innerWidth - rect.right),
    });
  };

  useOutsideDismiss([triggerRef, menuRef], open, closeMenu, { ignore: ['#resource-access-modal'] });

  useEffect(() => {
    if (!open) return undefined;
    updatePosition();
    const handleReposition = () => updatePosition();
    window.addEventListener('resize', handleReposition);
    window.addEventListener('scroll', handleReposition, true);
    return () => {
      window.removeEventListener('resize', handleReposition);
      window.removeEventListener('scroll', handleReposition, true);
    };
  }, [open]);

  const runAction = (action: () => void) => {
    closeMenu();
    action();
  };

  const menu = open && position ? (
    <div
      ref={menuRef}
      id={menuID}
      className="kc-doc-actions-popover kc-doc-actions-popover--row kc-doc-actions-popover--portal"
      aria-label={`${itemLabel} actions`}
      style={{ top: position.top, right: position.right }}
    >
      <button type="button" className="kc-doc-menu-item kc-doc-menu-item--primary" onClick={() => runAction(() => onSelectDocument(item.id))}>
        <BookOpenText className="h-4 w-4" aria-hidden="true" />
        Open
      </button>
      <ResourceAccessCard
        resourceType="knowledge_context"
        resourceID={item.id}
        label="knowledge context"
        buttonClassName="kc-doc-menu-item"
        onAccessChange={onAccessChange}
        onDialogClose={closeMenu}
      />
      <button type="button" className="kc-doc-menu-item" onClick={() => runAction(() => onDownloadDocument(item))}>
        <Download className="h-4 w-4" aria-hidden="true" />
        Export
      </button>
      {isExternal && item.external_page_url ? (
        <a className="kc-doc-menu-item" href={item.external_page_url} target="_blank" rel="noreferrer" onClick={closeMenu}>
          <ExternalLink className="h-4 w-4" aria-hidden="true" />
          Open page
        </a>
      ) : null}
      {isExternal ? (
        <button type="button" className="kc-doc-menu-item" onClick={() => runAction(() => onSyncDocument(item))}>
          <RotateCw className="h-4 w-4" aria-hidden="true" />
          Sync now
        </button>
      ) : null}
      {canWriteKnowledge ? (
        <button type="button" className="kc-doc-menu-item kc-doc-menu-item--primary" onClick={() => runAction(() => onEditDocument(item))}>
          <Edit3 className="h-4 w-4" aria-hidden="true" />
          Edit
        </button>
      ) : null}
      {canWriteKnowledge ? (
        <button type="button" className="kc-doc-menu-item" onClick={() => runAction(() => onCloneDocument(item))}>
          <Plus className="h-4 w-4" aria-hidden="true" />
          Clone
        </button>
      ) : null}
      {canDeleteKnowledge ? (
        <button type="button" className="kc-doc-menu-item kc-doc-menu-item--danger" onClick={() => runAction(() => onDeleteDocument(item))}>
          <Trash2 className="h-4 w-4" aria-hidden="true" />
          Delete
        </button>
      ) : null}
    </div>
  ) : null;

  return (
    <div className="kc-demo-row-actions">
      <button
        ref={triggerRef}
        type="button"
        className="kc-demo-kebab-btn"
        aria-label={`Actions for ${itemLabel}`}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuID : undefined}
        onClick={() => setOpen(current => !current)}
      >
        <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
      </button>
      {menu && portalHost ? createPortal(menu, portalHost) : null}
    </div>
  );
}

function documentSourceLabel(document: KnowledgeContextListItem): { label: string; tone: string } {
  const label = sourceLabel(document.source);
  if (knowledgeContentSource(document) === 'inline' && (label === 'Database' || label === 'UI')) {
    return { label: 'Inline', tone: 'blue' };
  }
  if (label === 'GitOps' || label === 'Repo') return { label: 'GitOps', tone: 'blue' };
  if (label === 'Notion') return { label, tone: 'neutral' };
  if (label === 'Confluence' || label === 'External page') return { label, tone: 'blue' };
  return { label, tone: 'neutral' };
}

function documentSyncBadge(document: KnowledgeContextListItem): { label: string; tone: string } {
  if (isGitManagedDocument(document)) return { label: 'GitOps synced', tone: 'green' };
  if (isExternalKnowledgeDocument(document)) return knowledgeSyncStatusLabel(document.sync_status, true);
  return { label: '', tone: 'neutral' };
}

function kindBadgeTone(kind: string) {
  if (kind === 'guardrail' || kind === 'policy') return 'amber';
  if (kind === 'runbook') return 'green';
  return 'purple';
}

function ConnectionRows({
  teams,
  selectedTeamPath,
  selectedConnectionID,
  onSelectConnectionTeam,
  onSelectConnection,
}: {
  teams: KnowledgeConnectionTeamSummary[];
  selectedTeamPath: string;
  selectedConnectionID: string;
  onSelectConnectionTeam: (teamPath: string) => void;
  onSelectConnection: (connectionID: string, teamPath: string) => void;
}) {
  const visibleTeams = teams.filter(team => team.connections.length > 0);
  const [teamOpenOverrides, setTeamOpenOverrides] = useState<Map<string, boolean>>(() => new Map());
  const openTeamPaths = useMemo(() => {
    const paths = new Set<string>();
    teamOpenOverrides.forEach((open, teamPath) => {
      if (open) paths.add(teamPath);
    });
    if (selectedTeamPath && !teamOpenOverrides.has(selectedTeamPath)) paths.add(selectedTeamPath);
    return new Set(Array.from(paths).filter(teamPath => visibleTeams.some(team => team.teamPath === teamPath)));
  }, [selectedTeamPath, teamOpenOverrides, visibleTeams]);
  const toggleTeam = (teamPath: string) => {
    setTeamOpenOverrides(previous => {
      const next = new Map(previous);
      next.set(teamPath, !openTeamPaths.has(teamPath));
      return next;
    });
  };
  const selectTeam = (teamPath: string) => {
    setTeamOpenOverrides(previous => {
      const next = new Map(previous);
      next.set(teamPath, true);
      return next;
    });
    onSelectConnectionTeam(teamPath);
  };
  const selectConnection = (connectionID: string, teamPath: string) => {
    setTeamOpenOverrides(previous => {
      const next = new Map(previous);
      next.set(teamPath, true);
      return next;
    });
    onSelectConnection(connectionID, teamPath);
  };
  const totalConnections = visibleTeams.reduce((total, team) => total + team.connections.length, 0);
  if (!visibleTeams.length) return <p className="kc-demo-tree-state">No connections yet.</p>;

  return (
    <>
      <button
        type="button"
        className={`triggers-explorer-root ${!selectedTeamPath && !selectedConnectionID ? 'active' : ''}`}
        aria-current={!selectedTeamPath && !selectedConnectionID ? 'page' : undefined}
        onClick={() => onSelectConnectionTeam('')}
      >
        <span className="triggers-explorer-folder" aria-hidden="true">
          <ObjectIcon type="mcp-profile" />
        </span>
        <span>All connections</span>
        <strong>{totalConnections}</strong>
      </button>
      <ul className="triggers-explorer-tree">
        {visibleTeams.map(team => {
          const teamIsOpen = openTeamPaths.has(team.teamPath);
          const teamIsActive = team.teamPath === selectedTeamPath && !selectedConnectionID;
          return (
            <li key={team.teamPath} className="triggers-explorer-node">
              <div className="triggers-explorer-node-row">
                <button
                  type="button"
                  className="triggers-explorer-toggle"
                  aria-label={`${teamIsOpen ? 'Collapse' : 'Expand'} ${team.teamPath} connections`}
                  aria-expanded={teamIsOpen}
                  onClick={() => toggleTeam(team.teamPath)}
                >
                  <ChevronRight className={`h-3.5 w-3.5 ${teamIsOpen ? 'rotate-90' : ''}`} aria-hidden="true" />
                </button>
                <button
                  type="button"
                  className={`triggers-explorer-owner ${teamIsActive ? 'active' : ''}`}
                  aria-label={`Open ${team.teamPath} connections`}
                  aria-current={teamIsActive ? 'page' : undefined}
                  onClick={() => selectTeam(team.teamPath)}
                >
                  <span className="triggers-explorer-folder" aria-hidden="true">
                    <ObjectIcon type="team" />
                  </span>
                  <span className="truncate">{team.teamPath}</span>
                  <strong>{team.connections.length}</strong>
                </button>
              </div>
              {teamIsOpen ? (
                <ul className="triggers-explorer-children">
                  {team.connections.map(connection => (
                    <li key={connection.id} className="triggers-explorer-leaf">
                      <button
                        type="button"
                        className={`triggers-explorer-trigger ${selectedConnectionID === connection.id ? 'active' : ''}`}
                        aria-label={`Open ${knowledgeConnectionDisplayName(connection)} connection`}
                        aria-current={selectedConnectionID === connection.id ? 'page' : undefined}
                        onClick={() => selectConnection(connection.id, team.teamPath)}
                      >
                        <span className="triggers-explorer-trigger-icon" aria-hidden="true">
                          <ObjectIcon type="credential" />
                        </span>
                        <span className="truncate">{knowledgeConnectionDisplayName(connection)}</span>
                        <span
                          className={`triggers-explorer-source triggers-explorer-source--${connection.disabled ? 'disabled' : connection.status === 'connected' ? 'external' : 'inline'}`}
                          title={connection.disabled ? 'Disabled' : connection.status}
                          aria-label={connection.disabled ? 'Disabled' : connection.status}
                        />
                      </button>
                    </li>
                  ))}
                </ul>
              ) : null}
            </li>
          );
        })}
      </ul>
    </>
  );
}
