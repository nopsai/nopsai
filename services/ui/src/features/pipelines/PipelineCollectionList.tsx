import {
  ArrowRight,
  FilePenLine,
  GitBranch,
  Layers3,
  SearchX,
  Trash2,
  Workflow,
  type LucideIcon,
} from 'lucide-react';
import { normalizePipelineSource, splitIdentifier, type PipelineListItem } from './model';
import './PipelineCollectionList.css';

type PipelineTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: PipelineTreeNode[];
  pipelineIds: string[];
};

type PipelineCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  visiblePipelines: PipelineListItem[];
  activeTeamNode: PipelineTreeNode;
  searchTerm: string;
  canCreatePipelineHere: boolean;
  canUsePipelineDrafts: boolean;
  canDeletePipelines: boolean;
  onSelectPipeline: (id: string) => void;
  onOpenTeam: (path: string) => void;
  onDeletePipeline: (id: string, name: string) => void;
};

type PipelineSourceMeta = {
  label: string;
  description: string;
  tone: 'saved' | 'git' | 'draft';
  Icon: LucideIcon;
};

function getPipelineSourceMeta(source: string): PipelineSourceMeta {
  if (source === 'draft') {
    return {
      label: 'Draft',
      description: 'Local draft — save it before running.',
      tone: 'draft',
      Icon: FilePenLine,
    };
  }
  if (source === 'git') {
    return {
      label: 'Git managed',
      description: 'Synced from source control.',
      tone: 'git',
      Icon: GitBranch,
    };
  }
  return {
    label: 'Saved',
    description: 'Ready to review, edit, or run.',
    tone: 'saved',
    Icon: Workflow,
  };
}

function countNestedPipelines(node: PipelineTreeNode): number {
  return node.pipelineIds.length + node.children.reduce((total, child) => total + countNestedPipelines(child), 0);
}

export function PipelineCollectionList({
  listLoading,
  listError,
  visiblePipelines,
  activeTeamNode,
  searchTerm,
  canCreatePipelineHere,
  canUsePipelineDrafts,
  canDeletePipelines,
  onSelectPipeline,
  onOpenTeam,
  onDeletePipeline,
}: PipelineCollectionListProps) {
  const isSearching = Boolean(searchTerm.trim());
  const draftCount = visiblePipelines.filter(pipeline => normalizePipelineSource(pipeline.source) === 'draft').length;
  const locationLabel = activeTeamNode.fullPath
    ? activeTeamNode.fullPath.split('/').filter(Boolean).join(' / ')
    : 'Root workspace';

  return (
    <div id="pipelines-list-view" className="pipelines-view pipeline-library">
      <header className="pipeline-library__hero">
        <div className="pipeline-library__intro">
          <span className="pipeline-library__eyebrow">{isSearching ? 'Filtered view' : locationLabel}</span>
          <h1 className="pipeline-library__title">Pipelines</h1>
          <p className="pipeline-library__subtitle">
            {isSearching
              ? `Results matching “${searchTerm.trim()}” across your pipeline workspace.`
              : 'Build, organize, and run repeatable workflows from one focused workspace.'}
          </p>
        </div>
        <dl className="pipeline-library__summary" aria-label="Pipeline workspace summary">
          <div>
            <dt>Pipelines</dt>
            <dd>{visiblePipelines.length}</dd>
          </div>
          <div>
            <dt>Teams</dt>
            <dd>{isSearching ? '—' : activeTeamNode.children.length}</dd>
          </div>
          <div>
            <dt>Drafts</dt>
            <dd>{draftCount}</dd>
          </div>
        </dl>
      </header>

      {listLoading ? (
        <PipelineListSkeleton />
      ) : listError ? (
        <div className="pipeline-library__notice pipeline-library__notice--error" role="alert">
          <strong>Unable to load pipelines</strong>
          <span>{listError}</span>
        </div>
      ) : (
        <div className="pipeline-library__content">
          {!isSearching && activeTeamNode.children.length ? (
            <section className="pipeline-library__section" aria-labelledby="pipeline-teams-heading">
              <SectionHeading
                id="pipeline-teams-heading"
                title="Teams"
                description="Open a team to narrow the workspace."
                count={activeTeamNode.children.length}
              />
              <div className="pipeline-library__grid pipeline-library__grid--teams">
                {activeTeamNode.children.map(child => (
                  <PipelineTeamCard key={`team-${child.id}`} node={child} onOpenTeam={onOpenTeam} />
                ))}
              </div>
            </section>
          ) : null}

          {visiblePipelines.length ? (
            <section className="pipeline-library__section" aria-labelledby="pipeline-items-heading">
              <SectionHeading
                id="pipeline-items-heading"
                title={isSearching ? 'Results' : 'Pipelines'}
                description={isSearching ? 'Open a result to view its details.' : 'Open a pipeline to inspect, edit, or run it.'}
                count={visiblePipelines.length}
              />
              <div className="pipeline-library__grid">
                {visiblePipelines.map(pipeline => (
                  <PipelineCard
                    key={pipeline.id}
                    pipeline={pipeline}
                    canUsePipelineDrafts={canUsePipelineDrafts}
                    canDeletePipelines={canDeletePipelines}
                    onSelectPipeline={onSelectPipeline}
                    onDeletePipeline={onDeletePipeline}
                  />
                ))}
              </div>
            </section>
          ) : null}

          {!visiblePipelines.length && (isSearching || !activeTeamNode.children.length) ? (
            <div id="pipelines-empty" className="pipeline-library__empty">
              <span className="pipeline-library__empty-icon" aria-hidden="true">
                <SearchX />
              </span>
              <h2>{isSearching ? 'No matching pipelines' : 'No pipelines here yet'}</h2>
              <p>
                {isSearching
                  ? 'Try a shorter name or clear the search to browse the workspace.'
                  : canCreatePipelineHere
                    ? 'Create a pipeline with the New pipeline button above.'
                    : 'You do not have permission to create pipelines in this team.'}
              </p>
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}

function SectionHeading({
  id,
  title,
  description,
  count,
}: {
  id: string;
  title: string;
  description: string;
  count: number;
}) {
  return (
    <div className="pipeline-library__section-heading">
      <div>
        <h2 id={id}>{title}</h2>
        <p>{description}</p>
      </div>
      <span className="pipeline-library__count" aria-label={`${count} ${title.toLowerCase()}`}>
        {count}
      </span>
    </div>
  );
}

function PipelineCard({
  pipeline,
  canUsePipelineDrafts,
  canDeletePipelines,
  onSelectPipeline,
  onDeletePipeline,
}: {
  pipeline: PipelineListItem;
  canUsePipelineDrafts: boolean;
  canDeletePipelines: boolean;
  onSelectPipeline: (id: string) => void;
  onDeletePipeline: (id: string, name: string) => void;
}) {
  const { name, path } = splitIdentifier(pipeline.id);
  const source = normalizePipelineSource(pipeline.source);
  const sourceMeta = getPipelineSourceMeta(source);
  const canDeleteThisPipeline = source === 'draft' ? canUsePipelineDrafts : canDeletePipelines && source !== 'git';
  const displayName = name || pipeline.id;
  const sourceClassName = `pipeline-library-card__badge pipeline-library-card__badge--${sourceMeta.tone}`;

  return (
    <article className="pipeline-library-card pipeline-library-card--pipeline">
      <button
        type="button"
        className="pipeline-library-card__hit-area"
        aria-label={`Open pipeline ${displayName}`}
        onClick={() => onSelectPipeline(pipeline.id)}
      />
      <div className="pipeline-library-card__body" aria-hidden="true">
        <span className="pipeline-library-card__icon pipeline-library-card__icon--pipeline">
          <Workflow />
        </span>
        <div className="pipeline-library-card__content">
          <div className="pipeline-library-card__title-row">
            <h3>{displayName}</h3>
            <span className={sourceClassName}>
              <sourceMeta.Icon />
              {sourceMeta.label}
            </span>
          </div>
          <p className="pipeline-library-card__path">{path ? path.split('/').join(' / ') : 'Root workspace'}</p>
          <p className="pipeline-library-card__description">{sourceMeta.description}</p>
        </div>
        <ArrowRight className="pipeline-library-card__arrow" />
      </div>
      {canDeleteThisPipeline ? (
        <button
          type="button"
          className="pipeline-library-card__delete"
          title={source === 'draft' ? 'Discard draft' : 'Delete pipeline'}
          onClick={() => onDeletePipeline(pipeline.id, displayName)}
          aria-label={source === 'draft' ? `Discard draft ${displayName}` : `Delete pipeline ${displayName}`}
        >
          <Trash2 />
        </button>
      ) : null}
    </article>
  );
}

function PipelineTeamCard({ node, onOpenTeam }: { node: PipelineTreeNode; onOpenTeam: (path: string) => void }) {
  const nestedPipelineCount = countNestedPipelines(node);

  return (
    <article className="pipeline-library-card pipeline-library-card--team">
      <button
        type="button"
        className="pipeline-library-card__hit-area"
        aria-label={`Open team ${node.name}`}
        onClick={() => onOpenTeam(node.fullPath)}
      />
      <div className="pipeline-library-card__body" aria-hidden="true">
        <span className="pipeline-library-card__icon pipeline-library-card__icon--team">
          <Layers3 />
        </span>
        <div className="pipeline-library-card__content">
          <div className="pipeline-library-card__title-row">
            <h3>{node.name}</h3>
            <span className="pipeline-library-card__badge pipeline-library-card__badge--team">Team</span>
          </div>
          <p className="pipeline-library-card__path">{node.fullPath.split('/').join(' / ')}</p>
          <p className="pipeline-library-card__description">
            {nestedPipelineCount} {nestedPipelineCount === 1 ? 'pipeline' : 'pipelines'} · {node.children.length}{' '}
            {node.children.length === 1 ? 'subteam' : 'subteams'}
          </p>
        </div>
        <ArrowRight className="pipeline-library-card__arrow" />
      </div>
    </article>
  );
}

function PipelineListSkeleton() {
  return (
    <div className="pipeline-library__content" aria-label="Loading pipelines" aria-busy="true">
      <div className="pipeline-library__skeleton-heading" />
      <div className="pipeline-library__grid">
        {Array.from({ length: 6 }, (_, index) => (
          <div key={index} className="pipeline-library__skeleton-card" />
        ))}
      </div>
    </div>
  );
}
