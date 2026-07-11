import { Trash2 } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import {
  countTeamDocs,
  isGitManagedDocument,
  normalizeKnowledgeSource,
  splitKnowledgePath,
  type KnowledgeContextListItem,
  type KnowledgeTeamNode,
} from './model';
import { kindIconType, kindPlural, kindTitle } from './presentation';

type KnowledgeContextCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  search: string;
  visibleDocuments: KnowledgeContextListItem[];
  visibleTeams: KnowledgeTeamNode[];
  selectedID: string;
  canWriteKnowledge: boolean;
  canDeleteKnowledge: boolean;
  onOpenTeam: (team: string) => void;
  onSelectDocument: (id: string) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
};

export function KnowledgeContextCollectionList({
  listLoading,
  listError,
  search,
  visibleDocuments,
  visibleTeams,
  selectedID,
  canWriteKnowledge,
  canDeleteKnowledge,
  onOpenTeam,
  onSelectDocument,
  onDeleteDocument,
}: KnowledgeContextCollectionListProps) {
  return (
    <div id="knowledge-context-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading knowledge contexts...</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load knowledge contexts: {listError}</div>
        ) : (
          <>
            {search.trim() ? (
              <div className="triggers-search-summary">
                Showing {visibleDocuments.length} result{visibleDocuments.length === 1 ? '' : 's'} for "{search.trim()}"
              </div>
            ) : null}

            {visibleDocuments.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">
                {visibleDocuments.map(doc => (
                  <KnowledgeDocumentCard
                    key={doc.id}
                    document={doc}
                    selectedID={selectedID}
                    canDeleteKnowledge={canDeleteKnowledge}
                    onSelectDocument={onSelectDocument}
                    onDeleteDocument={onDeleteDocument}
                  />
                ))}
              </div>
            ) : null}

            {search.trim() ? null : visibleTeams.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {visibleTeams.map(team => (
                  <KnowledgeTeamCard key={`team-${team.id}`} team={team} onOpenTeam={onOpenTeam} />
                ))}
              </div>
            ) : null}

            {!visibleDocuments.length && !visibleTeams.length ? (
              <div id="knowledge-context-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No knowledge contexts found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {canWriteKnowledge ? 'Create a new document or adjust your filters.' : 'Adjust your filters or browse another team.'}
                </p>
              </div>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}

function KnowledgeTeamCard({ team, onOpenTeam }: { team: KnowledgeTeamNode; onOpenTeam: (team: string) => void }) {
  const teamDepth = team.fullPath.split('/').filter(Boolean).length;
  const iconType = teamDepth === 1 ? kindIconType(team.name) : 'team';
  const teamName = teamDepth === 1 ? kindPlural(team.name) : team.name;

  return (
    <article
      className="glass-card pipeline-card kc-team-card border border-[var(--border-primary)] rounded-xl p-4"
      onClick={() => onOpenTeam(team.fullPath)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <ObjectIcon type={iconType} />
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{teamName}</h3>
            <p className="pipeline-card-path">{team.fullPath || 'root'}</p>
            <p className="pipeline-card-description">Knowledge team</p>
          </div>
        </div>
        <span className="pipeline-team-chevron">›</span>
      </div>
      <div className="pipeline-card-meta">
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Documents</span>
          <span className="pipeline-card-meta-value">{countTeamDocs(team)}</span>
        </div>
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Sub teams</span>
          <span className="pipeline-card-meta-value">{team.children.length}</span>
        </div>
      </div>
    </article>
  );
}

function KnowledgeDocumentCard({
  document,
  selectedID,
  canDeleteKnowledge,
  onSelectDocument,
  onDeleteDocument,
}: {
  document: KnowledgeContextListItem;
  selectedID: string;
  canDeleteKnowledge: boolean;
  onSelectDocument: (id: string) => void;
  onDeleteDocument: (document: KnowledgeContextListItem) => void;
}) {
  const iconType = kindIconType(document.kind);
  const { team } = splitKnowledgePath(document.id);
  const canDeleteThisDocument = canDeleteKnowledge;

  return (
    <article
      className={`glass-card pipeline-card kc-document-card border border-[var(--border-primary)] rounded-xl p-4 ${selectedID === document.id ? 'kc-document-card--active' : ''}`}
      onClick={() => onSelectDocument(document.id)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <ObjectIcon type={iconType} />
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{document.name}</h3>
            <p className="pipeline-card-path">{team || 'root'}</p>
            <p className="pipeline-card-description">{document.description || `${kindTitle(document.kind)} knowledge context.`}</p>
          </div>
        </div>
        <div className="pipeline-card-actions">
          {canDeleteThisDocument ? (
            <button
              type="button"
              className="pipelines-delete-button"
              title={isGitManagedDocument(document) ? 'Delete database row; GitOps can recreate it on the next sync' : 'Delete knowledge context'}
              onClick={event => {
                event.stopPropagation();
                onDeleteDocument(document);
              }}
              aria-label="Delete knowledge context"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          ) : null}
        </div>
      </div>
      <div className="pipeline-card-meta">
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Source</span>
          <span className="pipeline-card-meta-value">{normalizeKnowledgeSource(document.source)}</span>
        </div>
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Used by</span>
          <span className="pipeline-card-meta-value">{document.used_by_count || 0}</span>
        </div>
      </div>
    </article>
  );
}
