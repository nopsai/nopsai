import { Trash2 } from 'lucide-react';
import { ObjectIcon } from '../../components/ObjectIcon';
import type { StepListItem } from './api';
import { normalizeSource, splitIdentifier } from './model';

type StepTreeNode = {
  id: string;
  name: string;
  fullPath: string;
  children: StepTreeNode[];
  stepIds: string[];
};

type StepCollectionListProps = {
  listLoading: boolean;
  listError: string | null;
  visibleSteps: StepListItem[];
  activeFolderNode: StepTreeNode;
  searchTerm: string;
  canCreateStepHere: boolean;
  canUseStepDrafts: boolean;
  canDeleteSteps: boolean;
  onSelectStep: (id: string) => void;
  onOpenFolder: (path: string) => void;
  onDeleteStep: (id: string, name: string) => void;
};

export function StepCollectionList({
  listLoading,
  listError,
  visibleSteps,
  activeFolderNode,
  searchTerm,
  canCreateStepHere,
  canUseStepDrafts,
  canDeleteSteps,
  onSelectStep,
  onOpenFolder,
  onDeleteStep,
}: StepCollectionListProps) {
  return (
    <div id="steps-list-view" className="pipelines-view">
      <div className="space-y-3">
        {listLoading ? (
          <div className="glass-card p-5 text-sm text-[var(--text-secondary)]">Loading steps...</div>
        ) : listError ? (
          <div className="glass-card p-5 text-sm text-red-500">Failed to load steps: {listError}</div>
        ) : (
          <>
            {visibleSteps.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines">
                {visibleSteps.map(item => (
                  <StepCard
                    key={item.id}
                    item={item}
                    canUseStepDrafts={canUseStepDrafts}
                    canDeleteSteps={canDeleteSteps}
                    onSelectStep={onSelectStep}
                    onDeleteStep={onDeleteStep}
                  />
                ))}
              </div>
            ) : null}

            {searchTerm.trim() ? null : activeFolderNode.children.length ? (
              <div className="pipelines-card-grid pipelines-card-grid--pipelines mt-4">
                {activeFolderNode.children.map(child => (
                  <StepFolderCard key={`folder-${child.id}`} node={child} onOpenFolder={onOpenFolder} />
                ))}
              </div>
            ) : null}

            {!visibleSteps.length && !activeFolderNode.children.length && (
              <div id="steps-empty" className="pipelines-empty">
                <h3 className="text-base font-semibold text-[var(--text-primary)]">No steps found</h3>
                <p className="text-sm text-[var(--text-secondary)]">
                  {canCreateStepHere ? 'Create a new step or adjust your filters.' : 'Adjust your filters or check your access.'}
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

function StepCard({
  item,
  canUseStepDrafts,
  canDeleteSteps,
  onSelectStep,
  onDeleteStep,
}: {
  item: StepListItem;
  canUseStepDrafts: boolean;
  canDeleteSteps: boolean;
  onSelectStep: (id: string) => void;
  onDeleteStep: (id: string, name: string) => void;
}) {
  const { name, path } = splitIdentifier(item.id);
  const source = normalizeSource(item.source);
  const canDeleteThisStep = source === 'draft' ? canUseStepDrafts : canDeleteSteps && source !== 'git';

  return (
    <article className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4" onClick={() => onSelectStep(item.id)}>
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <ObjectIcon type="step" />
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{name || item.id}</h3>
            <p className="pipeline-card-path">{path || 'root'}</p>
            <p className="pipeline-card-description">Reusable workflow step.</p>
          </div>
        </div>
        <div className="pipeline-card-actions">
          {canDeleteThisStep ? (
            <button
              type="button"
              className="pipelines-delete-button"
              title={source === 'draft' ? 'Discard draft' : 'Delete step'}
              onClick={event => {
                event.stopPropagation();
                onDeleteStep(item.id, name || item.id);
              }}
              aria-label={source === 'draft' ? 'Discard draft step' : 'Delete step'}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </div>
      <div className="pipeline-card-meta">
        <div className="pipeline-card-meta-row">
          <span className="pipeline-card-meta-label">Source</span>
          <span className="pipeline-card-meta-value">{source}</span>
        </div>
      </div>
    </article>
  );
}

function StepFolderCard({ node, onOpenFolder }: { node: StepTreeNode; onOpenFolder: (path: string) => void }) {
  return (
    <article
      className="glass-card pipeline-card border border-[var(--border-primary)] rounded-xl p-4"
      onClick={() => onOpenFolder(node.fullPath)}
    >
      <div className="pipeline-card-header">
        <div className="pipeline-card-info">
          <span className="pipeline-card-icon" aria-hidden="true">
            <ObjectIcon type="folder" />
          </span>
          <div className="pipeline-card-text">
            <h3 className="pipeline-card-title">{node.name}</h3>
          </div>
        </div>
        <span className="pipeline-folder-chevron">›</span>
      </div>
      <div className="pipeline-folder-meta">
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Steps:</span>
          <span className="pipeline-card-meta-value">{node.stepIds.length}</span>
        </div>
        <div className="pipeline-folder-meta-row">
          <span className="pipeline-card-meta-label">Sub groups:</span>
          <span className="pipeline-card-meta-value">{node.children.length}</span>
        </div>
      </div>
    </article>
  );
}
