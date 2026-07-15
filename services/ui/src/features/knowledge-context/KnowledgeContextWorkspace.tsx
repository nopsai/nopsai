import { useEffect, useRef } from 'react';
import { BookOpenText, ChevronLeft, Filter, GitBranch, Link2, MoreHorizontal, Plus, Search, UsersRound } from 'lucide-react';

import { ObjectIcon } from '../../components/ObjectIcon';
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
  knowledgeSourceFilterOptions,
  parentTeam,
  splitKnowledgePath,
  sourceLabel,
  type KnowledgeConnectionListItem,
  type KnowledgeConnectionTeamSummary,
  type KnowledgeContextListItem,
  type KnowledgeSourceFilter,
  type KnowledgeTeamNode,
  type KnowledgeWorkspaceMetrics,
  type KnowledgeWorkspaceTab,
} from './model';
import { kindIconType, kindPlural, kindTitle } from './presentation';

type KnowledgeContextWorkspaceProps = {
  activeTeam: string;
  activeConnectionTeam: string;
  activeTab: KnowledgeWorkspaceTab;
  metrics: KnowledgeWorkspaceMetrics;
  connectionTeams: KnowledgeConnectionTeamSummary[];
  listLoading: boolean;
  listError: string | null;
  search: string;
  sourceFilter: KnowledgeSourceFilter;
  collectionDocuments: KnowledgeContextListItem[];
  visibleDocuments: KnowledgeContextListItem[];
  visibleTeams: KnowledgeTeamNode[];
  selectedID: string;
  selectedDetail: KnowledgeContextDetailViewProps;
  detailLoading: boolean;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  onSearchChange: (term: string) => void;
  onSourceFilterChange: (filter: KnowledgeSourceFilter) => void;
  onSwitchTab: (tab: KnowledgeWorkspaceTab) => void;
  onOpenTeam: (team: string) => void;
  onSelectConnectionTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
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

export function KnowledgeContextWorkspace({
  activeTeam,
  activeConnectionTeam,
  activeTab,
  metrics,
  connectionTeams,
  listLoading,
  listError,
  search,
  sourceFilter,
  collectionDocuments,
  visibleDocuments,
  visibleTeams,
  selectedID,
  selectedDetail,
  detailLoading,
  canWriteKnowledge,
  canDeleteKnowledge,
  onSearchChange,
  onSourceFilterChange,
  onSwitchTab,
  onOpenTeam,
  onSelectConnectionTeam,
  onSelectDocument,
  onDeleteDocument,
  onCreateDocument,
  onAddConnection,
  onTestConnection,
  onEditConnection,
  onToggleConnection,
  onDeleteConnection,
}: KnowledgeContextWorkspaceProps) {
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const actionLabel = activeTab === 'connections' ? 'New connection' : 'New context';
  const searchPlaceholder = activeTab === 'connections' ? 'Search connections' : 'Search knowledge contexts';

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
            onClick={() => onSwitchTab('documents')}
          >
            <BookOpenText className="h-4 w-4" aria-hidden="true" />
            Documents
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === 'connections'}
            className={activeTab === 'connections' ? 'active' : ''}
            onClick={() => onSwitchTab('connections')}
          >
            <Link2 className="h-4 w-4" aria-hidden="true" />
            Connections
          </button>
        </div>
        <div className="kc-demo-top-actions">
          <label className="kc-demo-global-search">
            <Search className="h-4 w-4" aria-hidden="true" />
            <input
              ref={searchInputRef}
              type="search"
              aria-label="Search knowledge context"
              placeholder={searchPlaceholder}
              value={search}
              onChange={event => onSearchChange(event.target.value)}
            />
            <span>Ctrl K</span>
          </label>
          {activeTab === 'documents' ? (
            <label className="kc-demo-filter">
              <Filter className="h-4 w-4" aria-hidden="true" />
              <select
                aria-label="Filter knowledge sources"
                value={sourceFilter}
                onChange={event => onSourceFilterChange(event.target.value as KnowledgeSourceFilter)}
              >
                {knowledgeSourceFilterOptions.map(option => (
                  <option key={option.value} value={option.value}>{option.label}</option>
                ))}
              </select>
            </label>
          ) : null}
          <button
            type="button"
            className="kc-demo-primary-btn"
            onClick={() => {
              if (activeTab === 'connections') onAddConnection(activeConnectionTeam);
              else onCreateDocument();
            }}
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            {actionLabel}
          </button>
        </div>
      </header>

      <div className="kc-demo-workspace">
        <KnowledgeBrowserCard
          activeTab={activeTab}
          activeTeam={activeTeam}
          activeConnectionTeam={activeConnectionTeam}
          totalDocuments={metrics.documents}
          connectionTeams={connectionTeams}
          listLoading={listLoading}
          listError={listError}
          visibleDocuments={visibleDocuments}
          visibleTeams={visibleTeams}
          selectedID={selectedID}
          canDeleteKnowledge={canDeleteKnowledge}
          onOpenTeam={onOpenTeam}
          onSelectConnectionTeam={onSelectConnectionTeam}
          onSelectDocument={onSelectDocument}
          onDeleteDocument={onDeleteDocument}
        />

        {activeTab === 'connections' ? (
          <KnowledgeContextConnectionsView
            listLoading={listLoading}
            listError={listError}
            search={search}
            teams={connectionTeams}
            canWriteKnowledge={canWriteKnowledge}
            canDeleteKnowledge={canDeleteKnowledge}
            onAddConnection={onAddConnection}
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
            metrics={metrics}
            listLoading={listLoading}
            listError={listError}
            canDeleteKnowledge={canDeleteKnowledge}
            onSelectDocument={onSelectDocument}
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
  totalDocuments,
  connectionTeams,
  listLoading,
  listError,
  visibleDocuments,
  visibleTeams,
  selectedID,
  canDeleteKnowledge,
  onOpenTeam,
  onSelectConnectionTeam,
  onSelectDocument,
  onDeleteDocument,
}: {
  activeTab: KnowledgeWorkspaceTab;
  activeTeam: string;
  activeConnectionTeam: string;
  totalDocuments: number;
  connectionTeams: KnowledgeConnectionTeamSummary[];
  listLoading: boolean;
  listError: string | null;
  visibleDocuments: KnowledgeContextListItem[];
  visibleTeams: KnowledgeTeamNode[];
  selectedID: string;
  canDeleteKnowledge: boolean;
  onOpenTeam: (team: string) => void;
  onSelectConnectionTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
}) {
  const title = activeTab === 'connections' ? 'Connection tree' : 'Knowledge tree';
  const count = activeTab === 'connections'
    ? connectionTeams.reduce((total, team) => total + team.connections.length, 0)
    : totalDocuments;

  return (
    <aside className="kc-demo-tree-card" aria-label={`${activeTab === 'connections' ? 'Connections' : 'Documents'} browser`}>
      <div className="kc-demo-tree-head">
        <span className="kc-demo-tree-head-icon" aria-hidden="true">
          <ObjectIcon type={activeTab === 'connections' ? 'mcp-profile' : 'knowledge-context'} />
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
          onSelectConnectionTeam={onSelectConnectionTeam}
        />
      ) : (
        <DocumentRows
          activeTeam={activeTeam}
          totalDocuments={totalDocuments}
          visibleDocuments={visibleDocuments}
          visibleTeams={visibleTeams}
          selectedID={selectedID}
          canDeleteKnowledge={canDeleteKnowledge}
          onOpenTeam={onOpenTeam}
          onSelectDocument={onSelectDocument}
          onDeleteDocument={onDeleteDocument}
        />
      )}
    </aside>
  );
}

function DocumentRows({
  activeTeam,
  totalDocuments,
  visibleDocuments,
  visibleTeams,
  selectedID,
  canDeleteKnowledge,
  onOpenTeam,
  onSelectDocument,
  onDeleteDocument,
}: {
  activeTeam: string;
  totalDocuments: number;
  visibleDocuments: KnowledgeContextListItem[];
  visibleTeams: KnowledgeTeamNode[];
  selectedID: string;
  canDeleteKnowledge: boolean;
  onOpenTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
}) {
  if (!visibleDocuments.length && !visibleTeams.length) {
    return <p className="kc-demo-tree-state">No documents found.</p>;
  }
  const parent = parentTeam(activeTeam);

  return (
    <div className="kc-demo-tree-list">
      {activeTeam ? (
        <button
          type="button"
          className="kc-demo-tree-row"
          aria-label={`Open ${parent || 'all knowledge'}`}
          onClick={() => onOpenTeam(parent)}
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          <span>{parent || 'All knowledge'}</span>
          <span className="count">Up</span>
        </button>
      ) : (
        <button
          type="button"
          className={`kc-demo-tree-row ${!selectedID ? 'active' : ''}`}
          aria-label="Open all knowledge"
          aria-current={!selectedID ? 'page' : undefined}
          onClick={() => onOpenTeam('')}
        >
          <ObjectIcon type="knowledge-context" />
          <span>All knowledge</span>
          <span className="count">{totalDocuments}</span>
        </button>
      )}
      {visibleTeams.map(team => {
        const depth = team.fullPath.split('/').filter(Boolean).length;
        const isKind = depth === 1;
        return (
          <button
            key={team.id}
            type="button"
            className={`kc-demo-tree-row ${isKind ? '' : 'indent1'}`}
            aria-label={`Open ${isKind ? kindPlural(team.name) : team.name}`}
            onClick={() => onOpenTeam(team.fullPath)}
          >
            <ObjectIcon type={isKind ? kindIconType(team.name) : 'team'} />
            <span>{isKind ? kindPlural(team.name) : team.name}</span>
            <span className="count">{countTeamDocs(team)}</span>
          </button>
        );
      })}
      {visibleDocuments.map(document => {
        const path = splitKnowledgePath(document.id).team;
        return (
          <div key={document.id} className={`kc-demo-tree-row-wrap ${selectedID === document.id ? 'active' : ''}`}>
            <button
              type="button"
              className="kc-demo-tree-row indent2"
              aria-label={`Open ${document.name}`}
              onClick={() => onSelectDocument(document.id)}
            >
              <span className="tree-dot" aria-hidden="true" />
              <ObjectIcon type={kindIconType(document.kind)} />
              <span>{document.name}</span>
              <small>{path}</small>
              <span className={`kc-demo-source-dot ${knowledgeContentSource(document)}`} aria-label={sourceLabel(document.source)} />
            </button>
            {canDeleteKnowledge ? (
              <button type="button" className="kc-demo-row-more" aria-label={`Delete ${document.name}`} onClick={() => onDeleteDocument(document)}>
                <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function KnowledgeDocumentCollection({
  documents,
  metrics,
  listLoading,
  listError,
  canDeleteKnowledge,
  onSelectDocument,
  onDeleteDocument,
}: {
  documents: KnowledgeContextListItem[];
  metrics: KnowledgeWorkspaceMetrics;
  listLoading: boolean;
  listError: string | null;
  canDeleteKnowledge: boolean;
  onSelectDocument: (id: string) => void;
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
      <div className="kc-demo-stats" aria-label="Knowledge Context summary">
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

      <div className="kc-demo-collection">
        <div className="kc-demo-collection-head">
          <div>
            <h2>All knowledge</h2>
            <p>{sortedDocuments.length} {sortedDocuments.length === 1 ? 'document' : 'documents'} across {metrics.teams} {metrics.teams === 1 ? 'team' : 'teams'}</p>
          </div>
          <span className="kc-demo-badge purple"><span className="dot" />{metrics.externalDocuments} external pages</span>
        </div>

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
                            <span>{teamPath === 'root' ? document.name : `${teamPath}/${document.name}`}</span>
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
                        <div className="kc-demo-row-actions">
                          {canDeleteKnowledge ? (
                            <button type="button" className="kc-demo-kebab-btn" aria-label={`Delete ${document.name}`} onClick={() => onDeleteDocument(document)}>
                              <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
                            </button>
                          ) : null}
                        </div>
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
  onSelectConnectionTeam,
}: {
  teams: KnowledgeConnectionTeamSummary[];
  selectedTeamPath: string;
  onSelectConnectionTeam: (teamPath: string) => void;
}) {
  const visibleTeams = teams.filter(team => team.connections.length > 0);
  if (!visibleTeams.length) return <p className="kc-demo-tree-state">No connections yet.</p>;
  const parent = parentTeam(selectedTeamPath);

  return (
    <div className="kc-demo-tree-list">
      <div className="kc-demo-tree-group-title">Connections</div>
      {selectedTeamPath ? (
        <button
          type="button"
          className="kc-demo-tree-row"
          aria-label={`Open ${parent || 'all connections'}`}
          onClick={() => onSelectConnectionTeam(parent)}
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />
          <span>{parent || 'All connections'}</span>
          <span className="count">Up</span>
        </button>
      ) : null}
      {!selectedTeamPath ? (
        <button
          type="button"
          className="kc-demo-tree-row active"
          aria-label="Open all connections"
          aria-current="page"
          onClick={() => onSelectConnectionTeam('')}
        >
          <ObjectIcon type="mcp-profile" />
          <span>All connections</span>
          <span className="count">{visibleTeams.reduce((total, team) => total + team.connections.length, 0)}</span>
        </button>
      ) : null}
      {visibleTeams.map(team => (
        <div key={team.teamPath} className="kc-demo-tree-team-group">
          <button
            type="button"
            className={`kc-demo-tree-row ${team.teamPath === selectedTeamPath ? 'active' : ''}`}
            aria-label={`Open ${team.teamPath} connections`}
            aria-current={team.teamPath === selectedTeamPath ? 'page' : undefined}
            onClick={() => onSelectConnectionTeam(team.teamPath)}
          >
            <ObjectIcon type="team" />
            <span>{team.teamPath}</span>
            <span className="count">{team.connections.length}</span>
          </button>
          {team.connections.map(connection => (
            <button
              key={connection.id}
              type="button"
              className="kc-demo-tree-row indent1 kc-demo-tree-row--connection"
              aria-label={`Open ${knowledgeConnectionDisplayName(connection)} connection`}
              onClick={() => onSelectConnectionTeam(team.teamPath)}
            >
              <span className={`kc-demo-source-dot ${connection.disabled ? 'inline' : 'external'}`} aria-hidden="true" />
              <span>{knowledgeConnectionDisplayName(connection)}</span>
              <small>{connection.provider}</small>
            </button>
          ))}
        </div>
      ))}
    </div>
  );
}
