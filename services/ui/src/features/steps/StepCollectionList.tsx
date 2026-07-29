import { useMemo, type ReactNode } from 'react';
import { ArrowRight, Trash2 } from 'lucide-react';
import { ResourceCollectionWorkspace, type ResourceCollectionTreeNode } from '../editor/ResourceCollectionWorkspace';
import { formatResourceListUpdatedAt } from '../editor/resourceCollectionModel';
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
  treeRoot: StepTreeNode;
  activeTeam: string;
  canCreateStepHere: boolean;
  canUseStepDrafts: boolean;
  canDeleteSteps: boolean;
  onSelectStep: (id: string) => void;
  onOpenTeam: (path: string) => void;
  onDeleteStep: (id: string, name: string) => void;
};

export function StepCollectionList({
  listLoading,
  listError,
  visibleSteps,
  treeRoot,
  activeTeam,
  canCreateStepHere,
  canUseStepDrafts,
  canDeleteSteps,
  onSelectStep,
  onOpenTeam,
  onDeleteStep,
}: StepCollectionListProps) {
  const resourceTreeRoot = useMemo(() => toResourceCollectionTreeNode(treeRoot), [treeRoot]);
  const emptyMessage = canCreateStepHere ? 'Create a new step or adjust your filters.' : 'Adjust your filters or check your access.';

  if (listLoading) {
    return (
      <StepWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
        <ResourcePanel title="Reusable steps" countLabel="Loading" showHeader={false}><div className="pipeline-runs-empty-state">Loading steps...</div></ResourcePanel>
      </StepWorkspace>
    );
  }

  if (listError) {
    return (
      <StepWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
        <ResourcePanel title="Reusable steps" countLabel="Error" showHeader={false}><div className="pipeline-runs-empty-state text-red-500">Failed to load steps: {listError}</div></ResourcePanel>
      </StepWorkspace>
    );
  }

  return (
    <StepWorkspace treeRoot={resourceTreeRoot} activeTeam={activeTeam} onOpenTeam={onOpenTeam}>
      <div id="steps-list-view" className="pipelines-view pipeline-runs-content-grid">
        <ResourcePanel title="Reusable steps" countLabel={`${visibleSteps.length} visible`} showHeader={false}>
          {visibleSteps.length ? (
            <StepTable
              steps={visibleSteps}
              canUseStepDrafts={canUseStepDrafts}
              canDeleteSteps={canDeleteSteps}
              onSelectStep={onSelectStep}
              onDeleteStep={onDeleteStep}
            />
          ) : (
            <div id="steps-empty" className="pipeline-runs-empty-state">
              <h3 className="text-base font-semibold text-[var(--text-primary)]">No steps found</h3>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">{emptyMessage}</p>
            </div>
          )}
        </ResourcePanel>
      </div>
    </StepWorkspace>
  );
}

function StepWorkspace({
  treeRoot,
  activeTeam,
  onOpenTeam,
  children,
}: {
  treeRoot: ResourceCollectionTreeNode;
  activeTeam: string;
  onOpenTeam: (path: string) => void;
  children: ReactNode;
}) {
  return (
    <ResourceCollectionWorkspace
      treeTitle="Teams"
      rootLabel="All teams"
      searchLabel="Search step teams"
      searchPlaceholder=""
      showTreeSearch={false}
      emptyLabel="No step teams found."
      activePath={activeTeam}
      rootNode={treeRoot}
      resizeStorageKey="steps"
      resizeLabel="Resize step team tree"
      onOpenPath={onOpenTeam}
    >
      {children}
    </ResourceCollectionWorkspace>
  );
}

function ResourcePanel({
  title,
  countLabel,
  showHeader = true,
  children,
}: {
  title: string;
  countLabel: string;
  showHeader?: boolean;
  children: ReactNode;
}) {
  const titleID = `${title.toLowerCase().replace(/[^a-z0-9]+/g, '-')}-title`;
  return (
    <section
      className="pipeline-runs-panel resource-collection-panel"
      aria-labelledby={showHeader ? titleID : undefined}
      aria-label={showHeader ? undefined : title}
    >
      {showHeader ? (
        <header className="pipeline-runs-panel-head">
          <div className="pipeline-runs-panel-title">
            <h2 id={titleID}>{title}</h2>
            <span>{countLabel}</span>
          </div>
        </header>
      ) : null}
      {children}
    </section>
  );
}

function StepTable({
  steps,
  canUseStepDrafts,
  canDeleteSteps,
  onSelectStep,
  onDeleteStep,
}: {
  steps: StepListItem[];
  canUseStepDrafts: boolean;
  canDeleteSteps: boolean;
  onSelectStep: (id: string) => void;
  onDeleteStep: (id: string, name: string) => void;
}) {
  return (
    <div className="pipeline-runs-table-wrap">
      <table className="pipeline-runs-table resource-collection-table resource-collection-table--steps" data-testid="steps-resource-table">
        <thead>
          <tr>
            <th>Source</th>
            <th>Step</th>
            <th>Team</th>
            <th>Updated</th>
            <th>Identifier</th>
            <th>
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {steps.map(item => (
            <StepRow
              key={item.id}
              item={item}
              canUseStepDrafts={canUseStepDrafts}
              canDeleteSteps={canDeleteSteps}
              onSelectStep={onSelectStep}
              onDeleteStep={onDeleteStep}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StepRow({
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
  const displayName = name || item.id;
  const updatedLabel = formatResourceListUpdatedAt(item.updatedAt);

  return (
    <tr>
      <td>
        <ResourceSourcePill source={source} />
      </td>
      <td>
        <button type="button" className="pipeline-runs-table-title" onClick={() => onSelectStep(item.id)}>
          <span title={displayName}>{displayName}</span>
        </button>
      </td>
      <td>
        <span className="pipeline-runs-mono">{path || 'root'}</span>
      </td>
      <td className="pipeline-runs-mono">{updatedLabel}</td>
      <td className="pipeline-runs-mono">{item.id}</td>
      <td>
        <div className="pipeline-runs-row-actions">
          {canDeleteThisStep ? (
            <button
              type="button"
              className="pipeline-runs-icon-button pipeline-runs-icon-button--danger"
              title={source === 'draft' ? 'Discard draft' : 'Delete step'}
              onClick={() => onDeleteStep(item.id, displayName)}
              aria-label={source === 'draft' ? `Discard draft step ${displayName}` : `Delete step ${displayName}`}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
          <button
            type="button"
            className="pipeline-runs-icon-button"
            onClick={() => onSelectStep(item.id)}
            aria-label={`Open step ${displayName}`}
          >
            <ArrowRight className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </td>
    </tr>
  );
}

function ResourceSourcePill({ source }: { source: 'git' | 'database' | 'draft' }) {
  const label = source === 'git' ? 'GitOps' : source === 'draft' ? 'Draft' : 'Database';
  const status = source === 'git' ? 'success' : source === 'draft' ? 'waiting' : 'pending';
  return (
    <span className={`pipeline-runs-status pipeline-runs-status-${status}`}>
      <span aria-hidden="true" />
      {label}
    </span>
  );
}

function toResourceCollectionTreeNode(node: StepTreeNode): ResourceCollectionTreeNode {
  return {
    id: node.id,
    name: node.name,
    fullPath: node.fullPath,
    resourceIds: node.stepIds,
    children: node.children.map(toResourceCollectionTreeNode),
  };
}
